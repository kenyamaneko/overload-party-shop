# Overload Party Shop

@.claude-common/base/CLAUDE.md
@.claude-common/flow-gitflow/CLAUDE.md

# このリポ固有

## [shop] 言語固有方針 (Go)

- テストコードはテーブル駆動で書く
  - 将来 `lang-go` レイヤが整備された時点で common 側に移動予定

## [shop] SSoT と生成コード

- 型コードの SSoT: `data/models.yaml` (再生成: `python3 scripts/generate_types.py`)

> **衝突解決**: `@import` した common の方針と矛盾する指示がこのファイルにある場合、リポ固有 (このファイル) を優先する。
