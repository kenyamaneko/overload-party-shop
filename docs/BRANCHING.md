# Branching Strategy

ブランチ戦略そのものは
[repos.yaml](https://github.com/kenyamaneko/overload-party-common/blob/main/rules/repos.yaml) が
`flow: github-flow` と宣言しており、共通ルール (`keyandnotes-rules` の `rules/flow/github-flow.md`) に従う。

## ブランチ保護設定

GitHub Ruleset で `main` に以下を設定している。

- 直 push 禁止、PR マージのみ
- force push 禁止、削除禁止
- 必須ステータスチェック: `ci / lint`, `ci / test-unit`, `ci / test-integration`, `ci / image-scan`, `ci / codegen-sync`
- required reviews: 0 (一人開発のため。複数人化する際に 1 へ引き上げる)

チェック名の `ci` は `ci.yaml` の呼び出し側ジョブ名で、続く名前は common の `go-service-ci.yaml` のジョブ名。

## CI/CD パイプライン

| ワークフロー | トリガー | 役割 |
|---|---|---|
| [ci.yaml](../.github/workflows/ci.yaml) | PR: main | lint + テスト + 脆弱性スキャン + コード生成ドリフト検出。中身は common の `go-service-ci.yaml` に集約している |
| [test-catalog.yaml](../.github/workflows/test-catalog.yaml) | push: main | `ci.yaml` を呼び、そのテスト結果からテスト観点カタログを生成して GitHub Pages に公開 |
| [deploy.yaml](../.github/workflows/deploy.yaml) | push: main / タグ `v*.*.*` | Docker イメージのビルド・push |
| [publish.yaml](../.github/workflows/publish.yaml) | workflow_dispatch | `packages/api-shop` (Go) のタグ付けと公開 |

`feature/*` への直 push では CI が走らない。main 宛の PR を作ると実行され、追加 push のたびに再実行される。

サービス本体のタグ発行は common の `scripts/create-release-tag.sh` を人が手動実行する ([docs/operations/SERVICE_RELEASE.md](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/operations/SERVICE_RELEASE.md))。手動タグ付けは CLAUDE.md の禁止事項の対象外 (この手順自体が手動タグ付け)。
