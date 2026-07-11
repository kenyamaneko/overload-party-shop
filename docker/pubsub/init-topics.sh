#!/bin/bash
# shop が publish する Pub/Sub topic を emulator に作成する。
# 実環境では topic は基盤側で作成されるが、shop 単体起動では publish 先が無いと
# outbox の送信が失敗するため、ここで用意する。
# topic 名は compose の env から受け取り、名前の二重管理を避ける。
set -euo pipefail

PROJECT="${PUBSUB_PROJECT_ID:?compose 経由で PUBSUB_PROJECT_ID を設定すること}"
HOST="${PUBSUB_EMULATOR_HOST:?compose 経由で PUBSUB_EMULATOR_HOST を設定すること}"

# 既存 (409) は idempotent 再実行として許容し、それ以外の HTTP 失敗は abort する。
put() {
  local url="$1"
  shift
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' -X PUT "$url" "$@")
  case "$code" in
    2*|409) return 0 ;;
    *) echo "PUT $url -> HTTP $code" >&2; return 1 ;;
  esac
}

for topic in \
  "$CARD_PACK_PURCHASED_TOPIC" \
  "$FACTION_ACQUIRED_TOPIC" \
  "$PREMIUM_UPDATED_TOPIC"; do
  put "http://${HOST}/v1/projects/${PROJECT}/topics/${topic}"
  echo "topic ready: ${topic}"
done
