#!/usr/bin/env bash
# setup-integration.sh — 結合テストの前提 (Postgres image の事前取得、Firestore emulator 起動) を整える。
set -euo pipefail

# Testcontainers はテスト中に自動 pull するが、時間がかかりタイムアウトの原因になるため事前に取得する
docker pull postgres:16-alpine

.github/scripts/ci/start-firestore-emulator.sh

{
  echo "FIRESTORE_EMULATOR_HOST=localhost:9041"
  echo "FIRESTORE_PROJECT_ID=overload-party-test"
} >> "$GITHUB_ENV"
