# Shop サービス設計

本ドキュメントは **コードを読んでも一見しては分からない設計意図** だけを残す。実装詳細 (フロー順序・状態遷移の対応・エラー → HTTP ステータス変換・環境変数の一覧) は各ファイルの実装とコメントを一次情報とする。

サービス概要・起動手順は [../README.md](../README.md)、エンドポイントは [API_REFERENCE.md](API_REFERENCE.md)、テーブル定義は [DATA_DESIGN.md](DATA_DESIGN.md) を参照。

## Shop の責務境界 (SSoT と read model)

Shop は **IAP 取引そのもの** の single source of truth だが、**プレイヤーの所有状態** の authoritative owner ではない。

| 種別 | SSoT | Shop 側の扱い |
|---|---|---|
| サブスクリプション契約 | `shop.subscriptions` (shop) | 自サービス内で完結 |
| 単発購入履歴 | `shop.one_time_purchases` (shop) | 自サービス内で完結 |
| ファクション所有権 | `account.player_factions` (account) | `shop.player_owned_factions` は shop ローカルの read model |
| コスメティクス所有権 | `shop.player_items` (shop) | ドメインとして shop 配下 |

`shop.player_owned_factions` は **cross-schema 読み込みを禁じた制約下で GetProducts の IsOwned を判定するための shop ローカル射影**。authoritative なのは account 側で、イベント駆動で最終的整合する。faction 購入時の書き込みは両サービスで独立に起きる (shop は自 read model を書き、account は faction-selected イベントで書く)。

subscriber (account / card / gateway) は `faction-selected` / `premium-updated` イベントを消費して各自の read model を構築する。shop は他サービスを直接呼ばない。

## イベント配信モデル (publish-after-commit + 多層冪等性)

publish は **DB commit 後** に行う。これは shop 行を durable record として扱うための意図的な順序で、publish が失敗した場合の回復経路は以下のように分業される:

| 失敗箇所 | 回復経路 |
|---|---|
| Purchase の publish 失敗 | handler が 5xx を返す → クライアントリトライ → 既存 purchaseToken を early-return でスキップし、publish のみ再実行 |
| Webhook 起点の publish 失敗 | 5xx を返して Apple/Google のリトライ予算で再駆動 |
| Subscribe の publish 失敗 | 同上 (クライアントリトライ) |

冪等性は単一の仕組みではなく多層で担保している:

- **Shop 内**: `purchaseToken` の PK UNIQUE (`shop.*_purchase_tokens` / `shop.*_subscription_tokens`) と、事前 lookup による early-return
- **Subscriber 側**: 各サービスが `processed_events` / 複合 PK で event_id 単位の二重適用を防ぐ
- shop 側のリトライと subscriber 側のリトライは独立しており、どちらが何度発火しても副作用は冪等

2 回目の publish は新しい `event_id` を生成する。これは意図的で、subscriber 側の重複検知は event_id ではなく **業務キー + 複合 PK** で行う前提。

## エンタイトルメント維持契約

サブスクリプション解約イベント (Apple `DID_CHANGE_RENEWAL_STATUS` + `AUTO_RENEW_DISABLED` / Google `notificationType=3`) は shop 内で `status=cancelled` に更新するが、**`premium-updated` イベントは発行しない**。

これは subscriber 側 (account / gateway) との契約:

> 解約されても `current_period_end` まではエンタイトルメントを維持する。subscriber は `premium-updated(is_premium=false)` が届くまで `is_premium` を落としてはならない。期限到来時は Apple/Google から `EXPIRED` 通知が届き、そこで初めて `is_premium=false` が流れる。

両プラットフォームで同一の契約を敷くため、Apple 側と Google 側の通知ハンドラで同じ振る舞いを実装している (`internal/service/apple_notification.go` と `internal/service/google_notification.go` の `cancelled` ケース)。新プラットフォームを足すときもこの契約を維持すること。

## Token 仕様変更への耐性

ドメインテーブル (`subscriptions` / `one_time_purchases`) と外部識別テーブル (`apple_subscription_tokens` / `google_subscription_tokens` / `apple_purchase_tokens` / `google_purchase_tokens`) を分離している。

理由: Apple / Google は token の意味や発行タイミングを将来変更しうる (e.g. 「更新ごとに新 token 発行」への変更、identifier のフォーマット変更)。この層を分けておけば、ドメイン側のスキーマを触らずに token テーブルの追加・差し替えで吸収できる。新プラットフォーム追加時も同様 (新しい `*_subscription_tokens` 系テーブルを追加するだけ)。

サブスクトークンのみ `updated_at` を持つのは、将来の token 再関連付け (既存 subscription 行に新しい token を紐付け直す) に備えたため。購入トークンは append-only。

## Webhook 信頼境界

Apple と Google で信頼の引き方が違う。新プラットフォーム追加時や webhook まわりを触るときに必要な前提:

- **Apple**: payload は JWS (JSON Web Signature) で署名されている。shop 自身が `x5c` 証明書チェーンを Apple Root CA (`internal/service/apple_root_ca_g3.pem`) まで検証してからデコードする。**payload レベルの認証**。
- **Google**: Pub/Sub push delivery 経由。メッセージ本体は署名されておらず、GCP の Pub/Sub subscription auth (transport レベル) で担保する。

どちらも gateway を経由しない外部エンドポイントだが、信頼境界を引くレイヤが異なる。この前提があるため、router に gateway 認証を挟んでいない。

webhook の deterministic error (decode 失敗 / unknown subscription 等) は **200 で ack** してストア側のリトライを止める。transient error (DB・pub/sub 障害等) は 5xx を返してリトライさせる (`internal/handler/rest/webhook_handler.go` の `respondWebhook`)。

## IAP_MODE=local の構造的安全性

`IAP_MODE=local` は開発用モードで、Apple/Google verifier を初期化しない。ここで重要なのは **未認証 POST が nil verifier に到達する経路がコードの構造上存在しない** こと。これは 3 ファイルの合意で成立している:

1. `cmd/server/main.go`: `IAP_MODE!=production` のとき `webhookH = nil` のまま router に渡す
2. `internal/router/router.go`: `webhookH == nil` のとき `/webhook/*` ルートを **登録しない**
3. `internal/service/shop_service.go` の `getVerifier`: 内部 `/purchase` / `/subscribe` ルートから呼ばれ、platform 不明時は `ErrUnsupportedPlatform`

つまり local モードでは webhook エンドポイント自体が存在しないため、署名なしペイロードを受理する入口がない。`IAP_MODE` の分岐条件や webhook 登録条件を変更するときは、この構造的安全性を崩さないこと。

## 運用

### 環境変数 / Secret Manager

環境変数の一覧と必須条件は [internal/config/config.go](../internal/config/config.go) が SSoT (`loadProductionIAP` / `loadLocalIAP` が起動時に検証、欠ければ即 fail)。

運用上の注意点のみ:

- **`IAP_MODE=production`** では Apple/Google の機密情報 (`shop-apple-*` / `shop-google-package-name` 等) を Secret Manager から起動時に取得する。k8s マニフェストにシークレットは載せない。
- シークレットの追加は手動: `gcloud secrets versions add <secret-id> --data-file=-`
- ローカル開発では `make run` が env を自動注入するため shell 側 export は不要。

### Pub/Sub トピックと subscriber

| トピック | 発行契機 | subscriber |
|---|---|---|
| `faction-selected` | `faction_set` 購入の DB commit 後 | account, card, gateway |
| `premium-updated` | サブスクリプション状態変化時 (解約時は除く、上述の契約) | account, gateway |

subscriber 列はこのリポジトリからは導けないので、変更時は各サービスの購読状況も確認すること。
