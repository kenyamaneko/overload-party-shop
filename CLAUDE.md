# CLAUDE.md - overload-party-shop

## 行動制約

- エラーは握りつぶさない（verifier エラー含む）
- git tag を手動で打たない（CI が自動作成する）
- TODO スタブを追加しない
- 内部 REST (`/internal/v1/*`) にクライアント認証を行わない（gateway が唯一の呼び出し元）
- Webhook (`/webhook/*`) の JWS / 署名検証を削除しない
- DB または card サービスに到達できないときフォールバックしない（5xx で fail）
- `IAP_MODE=local` は webhook ルート自体を登録しない（nil verifier で silent accept しない意図的設計）
- 型変更時は `data/models.yaml` → `python3 scripts/generate_types.py` を実行する
