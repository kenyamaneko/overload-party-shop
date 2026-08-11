# アーキテクチャ概要・ローカル開発

## アーキテクチャ概要

```
Gateway
  └─ Shop (:9006)
       ├─ PostgreSQL (shop スキーマ)
       └─ Pub/Sub
            ├─ card-pack-purchased → card
            ├─ faction-acquired    → account
            └─ premium-updated     → account

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
