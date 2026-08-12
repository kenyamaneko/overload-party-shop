# overload-party-shop

カードゲーム Overload Party の IAP・サブスクリプション・商品管理と Apple/Google webhook 処理を担うマイクロサービス。

## 技術スタック

| レイヤー | 技術 |
|---|---|
| 言語 | Go |
| フレームワーク | Gin |
| データベース | Cloud SQL PostgreSQL |
| NoSQL | Cloud Firestore |
| シークレット管理 | Secret Manager |
| 同期通信 | REST |
| 非同期通信 | Cloud Pub/Sub |

## ドキュメント

| ドキュメント | 内容 |
|---|---|
| [セットアップ](docs/SETUP.md) | ローカル開発環境の起動手順 |
| [API仕様書](data/openapi.yaml) | REST API のエンドポイント定義 |
| [Pub/Sub仕様書](data/asyncapi.yaml) | Pub/Sub イベントの定義 |
| [データ設計書](docs/DATA_DESIGN.md) | テーブル定義 |
| [ブランチ・CI/CD](docs/BRANCHING.md) | ブランチ運用と CI/CD の構成 |
| [運用手順書](docs/operations/IAP_SECRETS.md) | IAP シークレットの投入手順 |
| [ADR](https://github.com/kenyamaneko/overload-party-common/tree/main/docs/adr)（commonリポジトリ） | 設計判断の背景・理由・結果 |
| [システム構成図](https://github.com/kenyamaneko/overload-party-common#システム構成図)（commonリポジトリ） | Overload Party 全体の構成図 |
| [テスト観点カタログ](https://kenyamaneko.github.io/overload-party-shop/) | テスト名から自動生成した、テスト済みの観点一覧 |
