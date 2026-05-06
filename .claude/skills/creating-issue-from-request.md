---
name: creating-issue-from-request
description: ユーザーの自然言語の修正・機能追加依頼を GitHub Issue に変換する手順。ユーザーが「これを Issue にして」「チケット作って」と依頼したときに使う。
---

# 修正依頼を GitHub Issue にする

ユーザーの自然言語の依頼を、重複確認とテンプレートに沿った本文で GitHub Issue として登録する。

## 手順

1. 依頼から type を判定し、タイトルを生成する
   - type: `feat` / `fix` / `refactor` / `docs` / `chore` / `test` / `perf` / `ci` のいずれか
   - タイトル形式: `[{type}] {要約}` (日本語・50 文字以内)
2. 本文を下記テンプレに沿って書く
   - 情報が不足していれば**推測せず質問する**(「確認事項」参照)
3. 重複を確認する
   - `gh issue list --search "{keyword}"` で既存 Issue を探す
4. Issue を作成する
   - `gh issue create --title "..." --body-file {path} --label {type}`
5. Issue 番号をユーザーに伝え、対応ブランチを切るか確認する
   - 切る場合は `implementing-from-issue` skill に従う

## テンプレート

```markdown
## 背景

## 変更内容

## 受け入れ基準
- [ ]

## 関連
```

`fix` の場合は以下を追加する:

- 再現手順
- 期待動作
- 実際の動作

## 確認事項

以下が依頼文から読み取れない場合は、推測せずにユーザーに質問する:

- なぜやるか(背景)
- 何ができたら完了か(受け入れ基準)
- `fix` の場合: 再現手順、発見環境(dev / stg / prod)
