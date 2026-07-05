# Shop 機能仕様書

このドキュメントは shop サービスがビジネス要件として満たすべき振る舞いを定義する。実装方法ではなく **何を保証するか** を記述する。テストはこの仕様に従っていることを確認する観点で書く。

関連ドキュメント:
- 内部動作・配線・本番運用設定: [ARCHITECTURE.md](ARCHITECTURE.md)
- HTTP エンドポイント契約: [../data/openapi.yaml](../data/openapi.yaml)
- DB スキーマ: [DATA_DESIGN.md](DATA_DESIGN.md)

---

## サービス責務

shop は以下の機能ドメインを所有する。

| 機能 | 主要な責務 |
|---|---|
| 商品カタログ取得 | プレイヤーごとに「所有済みフラグ」付きの商品一覧を返す |
| 単発購入 | レシート検証・所有権付与・冪等性保証 |
| サブスクリプション開始 | レシート検証・期間記録・premium 通知 |
| Apple/Google webhook 処理 | サブスクリプション状態遷移・premium 通知 |

shop は **shop スキーマの DB 行を唯一の真実とし**、他サービスへの状態同期は Pub/Sub fan-out のみで行う。account / card を直接呼び出さない。

---

## プロダクト種別

商品は `Product.Type` で 3 種に分かれ、type 固有属性は **type 別副表**に正規化されている (詳細は [DATA_DESIGN.md](DATA_DESIGN.md))。

| Type | type 固有属性の所在 | 所有判定 |
|---|---|---|
| `faction_set` | `shop.product_faction(faction)` | `player_owned_factions` に該当 faction が存在 |
| `cosmetic` | `shop.product_cosmetic(item_type, item_no)` | `player_items` に (item_type, item_no) が存在 |
| `subscription` | `shop.product_subscription(period_months)` | 現在 entitled なサブスクリプションが存在（「サブスクリプション (`Subscribe`)」参照） |

`Product.IsActive = false` の商品は購入不可で、`GetProducts` でも返さない。

---

## 商品カタログ取得 (`GetProducts`)

**入力**: `playerID`
**出力**: `[]ProductResponse`（商品メタ + `IsOwned`）

### 仕様
1. `IsActive = true` の商品のみを返す
2. プレイヤーの最新サブスクリプション・所有 faction 集合・所有 item 集合を取得
3. 各商品について種別ごとに所有判定し `IsOwned` を埋める
4. ページング・絞り込みは行わない（全件返却）

副作用なし。

---

## 単発購入 (`Purchase`)

**入力**: `playerID`, `productID`, `pf` (`ios` / `android`), `purchaseToken`
**出力**: `error`

### バリデーション順序（fail-fast）

以下の順で検証し、失敗時点で即 return する。順序自体が仕様。

1. **プラットフォーム解決**: `pf` から verifier を選択。未対応値は `ErrUnsupportedPlatform`
2. **冪等性チェック**: `(pf, purchaseToken)` で既存購入を検索。ヒットすれば `nil` で即 return（成功扱い、副作用なし）
3. **商品存在・有効性**: `productID` で取得 → `ErrNotFound` / `ErrProductNotActive`
4. **種別固有バリデーション**:
   - `faction_set`: `card_pack_id` 未所有（`ErrAlreadyOwned`）。faction の値域は `product_faction.faction` の DB CHECK 制約で担保される (selectable faction のみ)
   - `card_pack`: `card_pack_id` 未所有（`ErrAlreadyOwned`）
   - `cosmetic`: (item_type, item_no) が未所有（`ErrAlreadyOwned`）
   - `subscription` または不明種別: `ErrUnsupportedProductType`（subscription は `Subscribe` を使う）
5. **レシート検証**: verifier 呼び出し
   - インフラ失敗（ネットワーク等）: `ErrVerifyReceipt`
   - ストアが拒否: `ErrReceiptVerificationFailed`
6. **DB 書き込み**: 購入レコードと所有権レコードを **同一トランザクション** で挿入
7. **イベント発行**: 新規成立時のみ outbox に enqueue (DB commit と atomic):
   - `faction_set`: `card-pack-purchased` + `faction-acquired` の 2 行
   - `card_pack`: `card-pack-purchased` の 1 行
   - `cosmetic`: なし

### 冪等性契約

- **キー**: `(pf, purchaseToken)`
- **保証**: 同一キーで複数回呼ばれても shop の DB 行は重複しない
- **publish 重複**: webhook リトライにより 2 回以上 publish され得るが、subscriber 側 (`processed_events` / composite PK) で重複適用を防ぐ前提
- repo が `created = false` を返した場合は再 publish しない

### ownership ガードと冪等性の関係

ownership ガード（`ErrAlreadyOwned`）は **異なる token で同じ商品を再購入** したケースを弾くためにある。同一 token の再試行は「バリデーション順序（fail-fast）」の step 2 で先に吸収される。

---

## サブスクリプション (`Subscribe`)

**入力**: `playerID`, `productID`, `pf`, `purchaseToken`
**出力**: `(*time.Time, error)`（成功時は `CurrentPeriodEnd`）

### 仕様

1. プラットフォーム解決
2. `(pf, purchaseToken)` で既存サブスクリプションを検索 → ヒット時は既存の `CurrentPeriodEnd` を返す（no-op）
3. 商品取得 → `Type = subscription` でなければ `ErrProductNotSubscription`
4. verifier の `VerifySubscription` 呼び出し
   - インフラ失敗: `ErrVerifyReceipt`
   - ストア拒否: `ErrSubVerificationFailed`
   - 成功時は `ExpiresAt` を取得
5. `subscriptions` を `Status = Active`, `CurrentPeriodEnd = ExpiresAt` で INSERT
6. `premium-updated(is_premium=true, expires_at=ExpiresAt)` を publish

### Entitlement 判定

`status` がサブスクリプション自体の状態を表すのに対し、Entitlement はプレミアム利用権の有無を表す。`isEntitled(sub, now)` は以下を満たす場合に `true`:

| Status | Entitled? | 意味 |
|---|---|---|
| `active` | `CurrentPeriodEnd > now` の場合 true | 自動更新 ON、課金中 |
| `cancelled` | `CurrentPeriodEnd > now` の場合 true | ユーザーが自動更新 OFF。期間内は premium 維持 |
| `grace` | `CurrentPeriodEnd > now` の場合 true | 決済リトライ猶予期間 |
| `expired` | 常に false | 期間終了 |
| `revoked` | 常に false | 返金等で取り消し |
| `nil` (購入歴なし) | 常に false | |

**重要**: `cancelled` でも期間内は premium 維持。期間途中での突然の権限剥奪は行わない。

---

## Webhook によるサブスクリプション状態遷移

Apple App Store Server Notifications V2 と Google Play RTDN を受け、サブスクリプション状態を更新し premium 状態を fan-out する。検証手順（JWS / RTDN デコード）は [ARCHITECTURE.md](ARCHITECTURE.md) を参照。

### 共通ルール

1. ペイロードから `purchaseToken` 相当の識別子を抽出
2. `(pf, token)` でサブスクリプションを検索 → 存在しなければ `ErrSubscriptionNotFound`（後述の通り 2xx ACK）
3. 通知種別に応じて `Status` を遷移、必要に応じて `CurrentPeriodEnd` を更新
4. premium 状態に変化があれば `premium-updated` を publish

publish は **DB COMMIT 後** に行う。publish 失敗時はストアのリトライにより再駆動される（冪等性ガードあり）。

### Apple 通知の遷移表

| NotificationType | Subtype | New Status | `premium-updated` |
|---|---|---|---|
| `DID_RENEW` | — | `active`（`CurrentPeriodEnd` を更新） | `is_premium=true, expires_at=新` |
| `EXPIRED` | — | `expired` | `is_premium=false` |
| `GRACE_PERIOD_EXPIRED` | — | `expired` | `is_premium=false` |
| `REVOKE` | — | `revoked` | `is_premium=false` |
| `DID_CHANGE_RENEWAL_STATUS` | `AUTO_RENEW_DISABLED` | `cancelled` | **publish しない**（期間内 premium 維持） |
| その他 | — | 変更なし | publish しない |

### Google 通知の遷移表

Google RTDN は `expiresAt` をペイロードに含まないため、`active` に戻す系の通知では Google Play Developer API に問い合わせて期限を取得する。

| NotificationType (int) | 意味 | New Status | 期限取得 | `premium-updated` |
|---|---|---|---|---|
| 2 | RECOVERED | `active` | API 取得 | `is_premium=true, expires_at=新` |
| 4 | RENEWED | `active` | API 取得 | `is_premium=true, expires_at=新` |
| 12 | EXPIRED | `expired` | 不要 | `is_premium=false` |
| 13 | REVOKED | `revoked` | 不要 | `is_premium=false` |
| 3 | CANCELED | `cancelled` | 不要 | **publish しない** |
| その他 | — | 変更なし | — | publish しない |

### cancellation で publish しない理由

`cancelled` は「自動更新が止まった」だけで、`CurrentPeriodEnd` までは entitlement が継続する（「Entitlement 判定」参照）。ここで `is_premium=false` を流すと subscriber 側で premium が即剥奪され、「課金期間内なのに権限が消えた」というユーザー体験事故になる。期限到達時の `EXPIRED` 通知で初めて剥奪する。

---

## エラーセマンティクス

usecase 層は HTTP ステータスを知らない。エラーはセンチネルとして返し、handler が `errors.Is` ベースの分類関数で transport 層のステータスに変換する（既存の振る舞いは [errors.go](../internal/handler/rest/errors.go) 参照）。

### 分類

| 分類関数 | 対象エラー | 用途 |
|---|---|---|
| `IsNotFound` | `ErrNotFound`, `ErrSubscriptionNotFound` | 404 |
| `IsConflict` | `ErrAlreadyOwned`, `ErrFactionAlreadySelected` | 409 |
| `IsValidation` | `ErrProductNotActive`, `ErrProductNotSubscription`, `ErrUnsupportedProductType`, `ErrUnsupportedPlatform` | 400 |
| `IsPaymentFailed` | `ErrReceiptVerificationFailed`, `ErrSubVerificationFailed` | 402（ストアが拒否） |
| `IsDeterministic` | デコード系全般, `ErrSubscriptionNotFound` | webhook で 2xx ACK（リトライ無意味） |

### `ErrVerifyReceipt` と `ErrReceiptVerificationFailed` の区別

- `ErrVerifyReceipt`: verifier 自体が失敗（ネットワーク、cert 検証失敗など）→ インフラ起因なので 5xx
- `ErrReceiptVerificationFailed`: verifier は応答したがストアが「無効」と判定 → 402

webhook は `IsDeterministic` で「リトライしても結果が変わらないか」を判定し、変わらないものは 2xx で確定 ACK する。これによりストアからの無限リトライを防ぐ。

---

## イベント発行

| トピック | ペイロード | 発行契機 |
|---|---|---|
| `card-pack-purchased` | `{event_type, event_id, timestamp, player_id, card_pack_id}` | `faction_set` または `card_pack` 単発購入が新規成立した COMMIT 後 |
| `faction-acquired` | `{event_type, event_id, timestamp, player_id, faction}` | `faction_set` 単発購入が新規成立した COMMIT 後 (`card-pack-purchased` と 2 行同時 publish) |
| `premium-updated` | `{event_type, event_id, timestamp, player_id, is_premium, expires_at?, source}` | サブスクリプション開始時、および webhook で premium 状態が変化した時 |

publish タイミング・冪等性の詳細は [ARCHITECTURE.md](ARCHITECTURE.md) の「イベント配信モデル (Transactional Outbox)」を参照。
