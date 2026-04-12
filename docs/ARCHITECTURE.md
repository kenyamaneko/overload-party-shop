# Shop サービス設計

このドキュメントは shop サービスの内部動作を説明する。サービスの概要・エンドポイント・環境変数は [README.md](../README.md) を参照。

## IAP 検証フロー

### 単発購入 (Purchase)

```
クライアント -> Gateway -> Shop POST /purchase
                             │
                             ├─ 冪等性チェック: purchaseToken で既存購入を検索
                             │   └─ 既存あり → 200 (no-op)
                             │
                             ├─ 商品取得 + is_active チェック
                             │
                             ├─ 所有ガード (商品種別による)
                             │   ├─ faction_set: player_owned_factions で既所有チェック
                             │   └─ cosmetic: player_items で既所有チェック
                             │
                             ├─ レシート検証
                             │   ├─ iOS: Apple App Store Server API (JWS 署名検証)
                             │   └─ Android: Google Play Developer API
                             │
                             ├─ トランザクション (商品種別による)
                             │   ├─ faction_set:
                             │   │   ├─ one_time_purchases INSERT
                             │   │   └─ player_owned_factions INSERT (shop-local read model)
                             │   └─ cosmetic:
                             │       ├─ one_time_purchases INSERT
                             │       └─ player_items INSERT
                             │
                             └─ COMMIT 後: faction_set の場合 Pub/Sub publish
                                 └─ faction-selected イベント (source = shop_purchase)
```

### サブスクリプション (Subscribe)

```
クライアント -> Gateway -> Shop POST /subscribe
                             │
                             ├─ 冪等性チェック: purchaseToken で既存サブスクリプション検索
                             │   └─ 既存あり → 200 with expires_at (no-op)
                             │
                             ├─ 商品取得 + type = subscription チェック
                             │
                             ├─ レシート検証 (VerifySubscription)
                             │
                             ├─ subscriptions INSERT (status = active)
                             │
                             └─ Pub/Sub publish
                                 └─ premium-updated イベント (is_premium = true)
```

### Apple JWS 検証

Apple App Store Server Notifications V2 は JWS (JSON Web Signature) 形式で署名されたペイロードを送信する。

1. `signedPayload` の JWS ヘッダーから `x5c` (証明書チェーン) を抽出
2. Apple Root CA まで証明書チェーンを検証
3. 署名を検証してペイロードをデコード
4. `notificationType` に応じて処理を分岐:
   - `DID_RENEW`: subscription を active に、`current_period_end` を更新 -> premium-updated publish (is_premium = true)
   - `EXPIRED` / `GRACE_PERIOD_EXPIRED`: subscription を expired に -> premium-updated publish (is_premium = false)
   - `REVOKE`: subscription を revoked に -> premium-updated publish (is_premium = false)
   - `DID_CHANGE_RENEWAL_STATUS` (subtype = `AUTO_RENEW_DISABLED`): subscription を cancelled に (premium は `current_period_end` まで維持、publish なし)

### Google RTDN 検証

Google Play Real-Time Developer Notifications は Pub/Sub push 経由で Base64 エンコードされた通知データを送信する。

1. `message.data` を Base64 デコード
2. `subscriptionNotification` フィールドの `notificationType` で分岐:
   - `4` (RENEWED) / `2` (RECOVERED): Google Play Developer API で最新の `expiresAt` を取得 -> subscription を active に -> premium-updated publish
   - `12` (EXPIRED): subscription を expired に -> premium-updated publish (is_premium = false)
   - `13` (REVOKED): subscription を revoked に -> premium-updated publish (is_premium = false)
   - `3` (CANCELED): subscription を cancelled に (premium は維持、publish なし)

## Pub/Sub publisher

Shop は 2 つのトピックに publish する。

| トピック | イベント | 契機 | subscriber |
|---|---|---|---|
| `faction-selected` | `FactionSelectedEvent` | faction_set 購入 COMMIT 後 | account, card, gateway |
| `premium-updated` | `PremiumUpdatedEvent` | サブスクリプション状態変化時 | account, gateway |

### publish タイミング

publish は DB COMMIT の**後**に行う。shop の DB 行が durable record であり:

- publish 失敗時は webhook リトライ予算 (Apple/Google) が再駆動する
- 冪等性ガード (purchaseToken / `player_owned_factions` ON CONFLICT) により shop 行の重複は発生しない
- 2 回目の publish は新しい `event_id` を生成するが、subscriber 側の `processed_events` / composite PK で重複適用を防止する

## IAP_MODE: local vs production

| 項目 | `production` (デフォルト) | `local` |
|---|---|---|
| Apple/Google 環境変数 | 全て必須。1 つでも欠ければ起動拒否 | 不要 |
| レシート検証 | Apple/Google API に問い合わせ | verifier = nil。purchase/subscribe で `ErrUnsupportedPlatform` |
| Webhook ルート | `/webhook/apple`, `/webhook/google` 登録 | 登録しない (router に存在しない) |

`local` モードでは webhook ルート自体が存在しないため、nil verifier が署名なしペイロードを受け入れるリスクを構造的に排除している。

## エラーハンドリング

- `DATABASE_URL` / `PUBSUB_PROJECT_ID` 未設定: 起動拒否 (fail-fast)
- `IAP_MODE=production` で Apple/Google 環境変数不足: 起動拒否
- Pub/Sub トピックが存在しない: 起動拒否
- レシート検証失敗: `ErrReceiptVerificationFailed` -> 402
- 既所有商品の再購入: `ErrAlreadyOwned` -> 409
- 未サポート platform: `ErrUnsupportedPlatform` -> 400
- publish 失敗: エラーを呼び出し元に返す (webhook の場合は Apple/Google のリトライで再駆動)
