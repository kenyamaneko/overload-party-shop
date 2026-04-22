# overload-party-shop

IAP・サブスクリプション・商品管理と Apple/Google webhook を処理する内部マイクロサービス。ポート 9006 で起動する。

詳細は [機能仕様書](docs/FEATURE_SPEC.md) / [サービス設計書](docs/ARCHITECTURE.md) / [API仕様書](docs/API_REFERENCE.md) / [データ設計書](docs/DATA_DESIGN.md) を参照。

## アーキテクチャ概要

```
Gateway
  └─ Shop (:9006)
       ├─ PostgreSQL (shop スキーマ)
       └─ Pub/Sub
            ├─ faction-purchased → account / card / gateway
            └─ premium-updated   → account / gateway

外部 (Gateway を経由しない)
  ├─ POST /webhook/apple   ← App Store Server Notifications V2
  └─ POST /webhook/google  ← Real-Time Developer Notifications
```

サービス間の状態同期は Pub/Sub で fan-out し、shop から他サービスを直接呼び出さない。

## ローカル開発

```bash
make db-up    # postgres:16-alpine を起動
make run      # サーバー起動（db-upと環境変数の注を含む）
make test     # Testcontainers でテスト実行（Docker 必須）
make db-down  # 停止
make db-reset # volume ごと削除して再作成
```

## 公開パッケージ

[packages/api-shop/](packages/api-shop/) に REST 契約型を公開している。[data/models.yaml](data/models.yaml) を編集後に以下で再生成する。

```bash
python3 scripts/generate_types.py
```
