# Shop サービス設計

本ドキュメントは **コードを読んでも一見しては分からない設計意図** だけを残す。実装詳細 (フロー順序・状態遷移の対応・エラー → HTTP ステータス変換・環境変数の一覧) は各ファイルの実装とコメントを一次情報とする。

サービス概要・起動手順は [../README.md](../README.md)、エンドポイントは [API_REFERENCE.md](API_REFERENCE.md)、テーブル定義は [DATA_DESIGN.md](DATA_DESIGN.md) を参照。

## 型のレイヤ分離 (domain / wire / persistence)

型は責務ごとに 3 つのレイヤに物理分離する。クリーンアーキテクチャの「ビジネスロジックは外部アダプターに依存しない」を構造で担保する。境界変換は handler (REST) と adapter (Pub/Sub) のみが担う。

| レイヤ | 置き場所 | 役割 |
|---|---|---|
| domain | [internal/domain/](../internal/domain/) | エンティティ・値オブジェクト・状態機械定数・ドメインイベント |
| wire | [packages/api-shop/](../packages/api-shop/) | REST request/response、webhook payload、Pub/Sub event の wire 契約 |
| persistence | repository 実装内部 | DB 行マッピング (専用の row 型は持たず pgx の positional `Scan` で domain 型へ直接読み書き) |

`packages/api-shop` は別 Go module で `internal/domain` を import できないため依存方向は物理強制される。inter-service event (`CardPackPurchasedEvent` / `FactionAcquiredEvent` / `PremiumUpdatedEvent`) のみ両レイヤに同形状で生成する (producer は domain、外部 subscriber は wire)。重複は [data/models.yaml](../data/models.yaml) からの codegen に閉じ込めている。

domain ↔ wire の境界変換は [internal/presenter/](../internal/presenter/) に集約する。

## Shop の責務境界 (SSoT と read model)

Shop は **IAP 取引そのもの** の single source of truth だが、**プレイヤーの所有状態** の authoritative owner ではない。

| 種別 | SSoT | Shop 側の扱い |
|---|---|---|
| サブスクリプション契約 | `shop.subscriptions` (shop) | 自サービス内で完結 |
| 単発購入履歴 | `shop.one_time_purchases` (shop) | 自サービス内で完結 |
| ファクション所有権 | `account.player_factions` (account) | `shop.player_owned_factions` は shop ローカルの read model |
| コスメティクス所有権 | `shop.player_items` (shop) | ドメインとして shop 配下 |

`shop.player_owned_factions` / `shop.player_owned_card_packs` は **cross-schema 読み込みを禁じた制約下で GetProducts の IsOwned 判定および再購入禁止チェックのための shop ローカル射影**。authoritative なのは account 側 (faction) / card 側 (card_pack の中身) で、イベント駆動で最終的整合する。faction 購入時の書き込みは両サービスで独立に起きる (shop は自 read model を書き、account は faction-acquired イベントで書く、card は card-pack-purchased イベントで配布する)。

subscriber は業務事実ごとに分離された ([ADR-031](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/adr/031-shop-products-normalization-and-faction-purchased-decomposition.md)):
- `card-pack-purchased`: card (`GrantPack(card_pack_id)` で配布) / gateway (副次通知)
- `faction-acquired`: account (`player_factions` INSERT、authoritative) / gateway (一次通知)
- `premium-updated`: account / gateway

shop は他サービスを直接呼ばない。

## イベント配信モデル (Transactional Outbox)

shop は **Transactional Outbox パターン** で Pub/Sub 発行を DB commit と atomic に揃える。dual-write 問題 (DB commit と Pub/Sub publish が別トランザクションになり、片方だけが成功して整合が壊れる現象) を構造的に排除するための中核機構。

### enqueue: ビジネス行と outbox 行を同一 tx で commit

各 aggregate repo (faction_purchase / item_purchase / subscription) は、ビジネステーブルの INSERT/UPDATE と同じトランザクションに `shop.outbox_events` 行の INSERT を相乗りさせる。service 層は `OutboxEventBuilder.Build*` で構築した `OutboxEvent` を repo に渡すだけで、repo 内部で同一 tx 書き込みが完結する。

```
BEGIN TX
  INSERT INTO shop.one_time_purchases ...
  INSERT INTO shop.apple_purchase_tokens ...
  INSERT INTO shop.player_owned_factions ...
  INSERT INTO shop.outbox_events ...     ← 同一 tx に相乗り
COMMIT
```

DB commit が成功した瞬間にイベントは必ず発行される運命にある (worker が未来のどこかで publish する)。commit が失敗すれば両方巻き戻る。REST ハンドラ視点では「Purchase が 200 を返した ⇔ subscriber にイベントが届く」の保証が得られる。

### 配信: 2 層に分けた常駐 worker が未配信行を claim して publish

消費フローは Clean Architecture 上の責務ごとに 2 コンポーネントに分けている:

| ファイル | 層 | 責務 |
|---|---|---|
| [internal/usecase/outbox/relay.go](../internal/usecase/outbox/relay.go) | usecase | `RunOnce` で claim → publish → mark/fail の orchestration。`port.OutboxStore` と `port.RawEventPublisher` に依存 |
| [internal/handler/worker/outbox_ticker.go](../internal/handler/worker/outbox_ticker.go) | handler (delivery) | ticker で `outbox.Relay.RunOnce` を周期呼び出し。ctx キャンセル制御と tick 失敗時の ERROR ログだけを持つ |

依存方向: `handler/worker` → `usecase` → `port`。handler/worker は usecase の具体実装を知らず、usecase は postgres / pubsub の具体型を知らない。

repo (`postgres.OutboxRepository`) は `port.OutboxStore` を実装する pure data access 層で、orchestration は持たない。ポーリング間隔・バッチサイズ・失敗閾値・visibility timeout は env (`OUTBOX_POLL_INTERVAL` / `OUTBOX_BATCH_SIZE` / `OUTBOX_FAILURE_THRESHOLD` / `OUTBOX_VISIBILITY_TIMEOUT`) で可変。

### 二重配信の防止: visibility timeout パターン

`ClaimUnpublished` は `FOR UPDATE SKIP LOCKED` で未配信行を選ぶのと同じ SQL で `last_attempted_at = now()` を更新する。以降 `OUTBOX_VISIBILITY_TIMEOUT` の間、他 worker の claim はその行を除外する (「直近試行から N 秒経過していない行はスキップ」という WHERE 条件)。

この仕組みにより:
- 複数 pod が同時に走っても同じ行を重複処理しない
- worker が publish 途中でクラッシュしても、visibility timeout 経過で自動的に再試行対象に戻る (claim 行ロックは claim tx 終了で解放されるため、長時間の「見えない行」が発生しない)
- publish が visibility timeout より長くかかった場合は他 worker が再 claim して重複 publish が起きるが、subscriber 側の event_id 冪等性で吸収される (at-least-once 契約)

### 冪等性の契約

- 各 outbox 行の `event_id` は enqueue 時点で確定し、payload 内 `eventId` と一致する。再試行しても同じ event_id を送るため、subscriber は `processed_events` / 複合 PK で重複適用を排除できる (at-least-once)。
- shop 側の再入 (再 Purchase / 再 Subscribe) は purchaseToken 単位の `FindPurchaseByToken` / `FindSubscriptionByToken` で早期 return する。既存 purchase があれば outbox 行も既に書かれている前提で、同じ event を二重 enqueue しない。
- 失敗継続した outbox 行は `failure_count` と `last_error` を積み、閾値 (`OUTBOX_FAILURE_THRESHOLD`) 超過で worker が ERROR ログを吐く。常駐プロセスに自動復旧手段はないため監視側でアラート。

### 配信済み行は削除しない

`published_at` が埋まった行も削除せず保持する (監査・障害調査のため)。partial index `idx_outbox_events_unpublished WHERE published_at IS NULL` により worker の claim クエリは未配信行だけを見るため、積み上がっても検索性能は劣化しない。将来テーブルサイズが問題になったら別 job で古い行を DELETE する (現時点では未実装)。

## エンタイトルメント維持契約

サブスクリプション解約イベント (Apple `DID_CHANGE_RENEWAL_STATUS` + `AUTO_RENEW_DISABLED` / Google `notificationType=3`) は shop 内で `status=cancelled` に更新するが、**`premium-updated` イベントは発行しない**。

これは subscriber 側 (account / gateway) との契約:

> 解約されても `current_period_end` まではエンタイトルメントを維持する。subscriber は `premium-updated(is_premium=false)` が届くまで `is_premium` を落としてはならない。期限到来時は Apple/Google から `EXPIRED` 通知が届き、そこで初めて `is_premium=false` が流れる。

両プラットフォームで同一の契約を敷くため、Apple/Google の通知ハンドラはどちらも `SubscriptionService.applySubChangeNoEvent` (outbox 行を書かない UPDATE) を cancelled ケースで呼ぶ。新プラットフォームを足すときもこの契約を維持すること。

## Token 仕様変更への耐性

ドメインテーブル (`subscriptions` / `one_time_purchases`) と外部識別テーブル (`apple_subscription_tokens` / `google_subscription_tokens` / `apple_purchase_tokens` / `google_purchase_tokens`) を分離している。

理由: Apple / Google は token の意味や発行タイミングを将来変更しうる (e.g. 「更新ごとに新 token 発行」への変更、identifier のフォーマット変更)。この層を分けておけば、ドメイン側のスキーマを触らずに token テーブルの追加・差し替えで吸収できる。新プラットフォーム追加時も同様 (新しい `*_subscription_tokens` 系テーブルを追加するだけ)。

サブスクトークンのみ `updated_at` を持つのは、将来の token 再関連付け (既存 subscription 行に新しい token を紐付け直す) に備えたため。購入トークンは append-only。

## Webhook 信頼境界

Apple と Google で信頼の引き方が違う。新プラットフォーム追加時や webhook まわりを触るときに必要な前提:

- **Apple**: payload は JWS (JSON Web Signature) で署名されている。shop 自身が `x5c` 証明書チェーンを Apple Root CA (`internal/adapter/apple/apple_root_ca_g3.pem`) まで検証してからデコードする。**payload レベルの認証**。
- **Google**: Pub/Sub push delivery 経由。メッセージ本体は署名されておらず、Google Cloud の Pub/Sub subscription auth (transport レベル) で担保する。

どちらも gateway を経由しない外部エンドポイントだが、信頼境界を引くレイヤが異なる。この前提があるため、router に gateway 認証を挟んでいない。

webhook の deterministic error (decode 失敗 / unknown subscription 等) は **200 で ack** してストア側のリトライを止める。transient error (DB・pub/sub 障害等) は 5xx を返してリトライさせる (`internal/handler/rest/webhook_handler.go` の `respondWebhook`)。outbox 導入後は webhook 起点の DB 更新 + outbox 行も同一 tx なので、DB 失敗で 5xx が返るケースはビジネス行と outbox 行を両方巻き戻した上でストアリトライを待つ形になる。

## IAP_MODE=local の構造的安全性

`IAP_MODE=local` は開発用モードで、Apple/Google verifier を初期化しない。ここで重要なのは **未認証 POST が nil verifier に到達する経路がコードの構造上存在しない** こと。これは 3 ファイルの合意で成立している:

1. `cmd/server/main.go`: `IAP_MODE!=production` のとき verifier 系を全て nil のまま返し、webhookH も nil 構築する
2. `internal/router/router.go`: `webhookH == nil` のとき `/webhook/*` ルートを **登録しない**
3. `internal/usecase/purchase/service.go` の `getVerifier`: 内部 `/purchase` / `/subscribe` ルートから呼ばれ、platform 不明時は `ErrUnsupportedPlatform`

つまり local モードでは webhook エンドポイント自体が存在しないため、署名なしペイロードを受理する入口がない。`IAP_MODE` の分岐条件や webhook 登録条件を変更するときは、この構造的安全性を崩さないこと。

## 運用

### 環境変数 / Secret Manager

環境変数の一覧と必須条件は [internal/config/config.go](../internal/config/config.go) が SSoT (`loadProductionIAP` / `loadLocalIAP` / `loadOutboxConfig` が起動時に検証、欠ければ即 fail)。

運用上の注意点のみ:

- **`IAP_MODE=production`** では Apple/Google の機密情報 (`shop-apple-*` / `shop-google-package-name` 等) を Secret Manager から起動時に取得する。k8s マニフェストにシークレットは載せない。
- シークレットの追加は手動: `gcloud secrets versions add <secret-id> --data-file=-`
- **`OUTBOX_POLL_INTERVAL` / `OUTBOX_BATCH_SIZE` / `OUTBOX_FAILURE_THRESHOLD`** は負荷試験やインシデント時にデプロイなしで試行錯誤できるよう env で持つ。
- ローカル開発では `make run` が env を自動注入するため shell 側 export は不要。

### Pub/Sub トピックと subscriber

| トピック | 発行契機 | subscriber |
|---|---|---|
| `card-pack-purchased` | `faction_set` / `card_pack` 商品購入の DB commit 後 (worker が outbox 消費) | card, gateway |
| `faction-acquired` | `faction_set` 商品購入の DB commit 後 (worker が outbox 消費) | account, gateway |
| `premium-updated` | サブスクリプション状態変化時 (解約時は除く、上述の契約) | account, gateway |

subscriber 列はこのリポジトリからは導けないので、変更時は各サービスの購読状況も確認すること。
