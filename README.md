# overload-party-shop

IAP・サブスクリプション・商品管理と Apple/Google webhook を処理する内部マイクロサービス。ポート 9006 で起動する。

詳細は [API契約](data/openapi.yaml) / [データ設計書](docs/DATA_DESIGN.md) / [ブランチ・CI/CD](docs/BRANCHING.md) を参照。設計判断 (Why) は [common の ADR](https://github.com/kenyamaneko/overload-party-common/tree/main/docs/adr) に記録する。アーキテクチャ概要・ローカル開発は [docs/SETUP.md](docs/SETUP.md) を参照。

IAP シークレットの投入手順は [運用手順書](docs/operations/IAP_SECRETS.md) を参照。

[テスト観点カタログ](https://kenyamaneko.github.io/overload-party-shop/): テスト名から生成した、テスト済みの観点の一覧。
