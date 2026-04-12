# overload-party-shop

IAP / サブスクリプション / 商品管理 + Apple/Google webhook を処理する内部マイクロサービス。

## サービス間連携

```
Gateway (主たる呼び出し元)
  ├─ GET  /players/:playerId/products
  ├─ POST /players/:playerId/purchase
  └─ POST /players/:playerId/subscribe
                │
                ▼
Shop (このサービス, :9006)
  ├─ PostgreSQL (shop スキーマ所有)
  └─ Pub/Sub publisher
       ├─ faction-selected  → account / card / gateway が subscribe
       └─ premium-updated   → account / gateway が subscribe

Apple App Store / Google Play (外部)
  ├─ POST /webhook/apple  ← App Store Server Notifications V2
  └─ POST /webhook/google ← Real-Time Developer Notifications
```

- Gateway を経由しない唯一の公開エンドポイント: `/webhook/*`
- shop から account / card を直接呼び出さない。跨サービス更新は Pub/Sub で fan-out する

エンドポイント一覧は [docs/API_REFERENCE.md](docs/API_REFERENCE.md) を参照。

## 環境変数

### Production (`IAP_MODE=production`, デフォルト)

Apple/Google IAP 資格情報は **GCP Secret Manager** から起動時に取得する。k8s マニフェストにシークレットは載せない。

| 変数名 | デフォルト | 説明 |
|---|---|---|
| `DATABASE_URL` | (必須) | PostgreSQL 接続文字列 |
| `PUBSUB_PROJECT_ID` | (必須) | Pub/Sub GCP プロジェクト |
| `GCP_PROJECT` | (production 時必須) | Secret Manager が存在する GCP プロジェクト ID |
| `PORT` | `9006` | リッスンポート |
| `ENV` | `dev` | `dev` / `stg` / `prod` |
| `FACTION_SELECTED_TOPIC` | `faction-selected` | faction-selected Pub/Sub トピック名 |
| `PREMIUM_UPDATED_TOPIC` | `premium-updated` | premium-updated Pub/Sub トピック名 |
| `IAP_MODE` | `production` | `production` / `local` |
| `APPLE_ENVIRONMENT` | `Sandbox` | `Production` / `Sandbox` |

Secret Manager から取得するシークレット:

| Secret ID | 内容 |
|---|---|
| `shop-apple-key-id` | App Store Connect API key ID |
| `shop-apple-issuer-id` | App Store Connect issuer ID |
| `shop-apple-bundle-id` | iOS アプリ bundle ID |
| `shop-apple-private-key` | `.p8` PEM 秘密鍵 (全文) |
| `shop-google-package-name` | Android パッケージ名 |

シークレットバージョンの追加は手動: `gcloud secrets versions add shop-apple-key-id --data-file=-`

### Local (`IAP_MODE=local`)

Secret Manager を使わず環境変数から直接読み込む。未設定でも起動可能 (verifier 初期化をスキップし webhook ルートを登録しない)。

| 変数名 | 説明 |
|---|---|
| `APPLE_KEY_ID` | App Store Connect API key ID |
| `APPLE_ISSUER_ID` | App Store Connect issuer ID |
| `APPLE_BUNDLE_ID` | iOS アプリ bundle ID |
| `APPLE_PRIVATE_KEY_PATH` | `.p8` 秘密鍵ファイルパス |
| `GOOGLE_PACKAGE_NAME` | Android パッケージ名 |

`DATABASE_URL` / `PUBSUB_PROJECT_ID` が未設定なら起動時に即 fail する。`IAP_MODE=production` で `GCP_PROJECT` が未設定、または Secret Manager に到達できない場合も即 fail する。

## 公開パッケージ

| パッケージ | パス | 用途 |
|---|---|---|
| Go module | `packages/api-shop/` | REST 契約型 (`apishop.PurchaseRequest` 等) |

SSoT: `data/models.yaml` -> `python3 scripts/generate_types.py` で再生成。

クライアント向け TypeScript 型は `@kenyamaneko/overload-party-api-gateway` に統合済み。
