#!/usr/bin/env bash
# start-firestore-emulator.sh — Firestore emulator をバックグラウンドで起動し
# ヘルスチェックが通るまで最大 30 秒待つ。
set -euo pipefail

gcloud emulators firestore start --host-port=localhost:9041 >/tmp/firestore.log 2>&1 &
for i in {1..30}; do
  if curl -sf http://localhost:9041 >/dev/null; then exit 0; fi
  sleep 1
done
echo "Firestore emulator failed to start"; cat /tmp/firestore.log; exit 1
