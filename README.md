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

`make run` はアプリ本体とインフラ (Postgres / Firestore / Pub/Sub emulator) を compose 内で起動する。
インフラはホストへ publish せず内部ネットワークのサービス名 DNS で参照するため、他リポのローカル
スタックやホスト上の他アプリとポートが衝突しない。ホストへ出るのは shop の API ポート 9006 のみ。

```bash
make run      # アプリ + インフラを compose で起動（ソース bind-mount）
make down     # 停止して volume を削除
make test     # Testcontainers でテスト実行（Docker 必須）
```

アプリはコンテナ内で `go run` する。ソースを編集して `docker compose restart shop` すれば、
イメージを作り直さずに反映される。private module は host の module cache を読み取り専用でマウント
して解決するため、`make run` は先に host 側で `go mod download` を実行する。

## 公開パッケージ

[packages/api-shop/](packages/api-shop/) に REST 契約型を公開している。[data/models.yaml](data/models.yaml) を編集後に以下で再生成する。

```bash
python3 scripts/generate_types.py
```
