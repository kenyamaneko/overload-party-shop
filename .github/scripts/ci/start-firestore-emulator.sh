#!/usr/bin/env bash
# start-firestore-emulator.sh — Firestore emulator をバックグラウンドで起動し
# ヘルスチェックが通るまで待つ。
set -euo pipefail

EMULATOR_PORT=9041
HEALTH_CHECK_TIMEOUT_SEC=30

gcloud emulators firestore start --host-port="localhost:${EMULATOR_PORT}" >/tmp/firestore.log 2>&1 &
for i in $(seq 1 "${HEALTH_CHECK_TIMEOUT_SEC}"); do
  if curl -sf "http://localhost:${EMULATOR_PORT}" >/dev/null; then exit 0; fi
  sleep 1
done
echo "Firestore emulator failed to start"; cat /tmp/firestore.log; exit 1
