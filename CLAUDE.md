# Overload Party Shop

@.claude-common/base/CLAUDE.md
@.claude-common/flow-gitflow/CLAUDE.md

# このリポ固有

## [shop] 言語固有方針 (Go)

- テストコードはテーブル駆動で書く
  - 将来 `lang-go` レイヤが整備された時点で common 側に移動予定

## [shop] 禁止事項

- 生成済み型コードを手で書き換えない。型は `data/models.yaml` を SSoT とし、変更後は `python3 scripts/generate_types.py` で再生成する
- このファイル (リポルートの CLAUDE.md) を Claude が書き換えない。ルール変更は人間が明示的に指示した場合のみ

## [shop] 参考リンク

- 本リポ固有の CI/CD と release tag 自動生成: [docs/CI_AND_RELEASE.md](docs/CI_AND_RELEASE.md)

> **衝突解決**: `@import` した common の方針と矛盾する指示がこのファイルにある場合、リポ固有 (このファイル) を優先する。
