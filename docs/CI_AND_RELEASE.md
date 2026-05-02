# CI/CD と Release 自動化 (shop 固有)

本リポジトリ固有の CI/CD ワークフローと、ブランチ戦略を実体化する自動化の仕組みを定義する。
ブランチ戦略の一般論は [BRANCHING.md](BRANCHING.md) を参照。

## CI/CD パイプライン

| ワークフロー | トリガー | 役割 |
|---|---|---|
| `ci.yaml` | PR: main, develop, release/* | lint + test + 脆弱性スキャン + コード生成ドリフト検出 + (main のみ) マージ元ブランチ制限 |
| `deploy.yaml` | push: main, develop, release/* | Docker イメージのビルド・push |
| `release-tag.yaml` | PR close (→ main) | release/hotfix ブランチから SemVer タグを自動生成 |
| `publish.yaml` | workflow_dispatch (手動) | api-shop Go モジュールのタグ付け・公開 |

### CI と CD の連携

CI (lint/test) の成功は、各ブランチの保護ルール (required status check) で担保する。
deploy.yaml は CI と独立して push 時に発火するが、保護ブランチへの push は
CI が通った PR のマージ経由でしか行えないため、CI を経由しないデプロイは発生しない。

### feature / hotfix ブランチの CI

feature/* や hotfix/* ブランチへの push では CI は走らない。
これらのブランチで CI を実行するには、対象ブランチ (develop / main) 宛の PR を作成する。
PR 更新時 (追加 push) にも CI が再実行される。

## タグ自動生成

サービス本体のタグは `release-tag.yaml` が自動で打つ。

- release マージ時: ブランチ名からバージョンを取得（`release/v1.2.0` → `v1.2.0`）
- hotfix マージ時: 最新タグから patch を自動 bump（`v1.2.0` → `v1.2.1`）

手動タグ付けは禁止 ([CLAUDE.md](../CLAUDE.md) の禁止事項に明記)。

## packages/api-shop の Go module

`packages/api-shop` は Go module として独立のバージョンを持つ。

- タグ発行: `publish.yaml` で手動発行する（`workflow_dispatch`）
- バージョンを上げるタイミングは人が判断する
- サービス本体のバージョンと必ずしも一致しないが、破壊的変更を含む release では api-shop も major bump することを推奨する

## ブランチ保護: 必須ステータスチェック (実体)

[BRANCHING.md](BRANCHING.md) の「ブランチ保護設定」を本リポでは以下の具体的なチェックで実装する。

### main

- `CI / lint`
- `CI / test`
- `CI / check-source-branch` (release/* と hotfix/* のみ許可を機械的に強制)

### release/*

- `CI / lint`
- `CI / test`

### develop

- `CI / lint`
- `CI / test`
