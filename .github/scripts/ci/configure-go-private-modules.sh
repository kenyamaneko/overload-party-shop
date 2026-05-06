#!/usr/bin/env bash
# configure-go-private-modules.sh — プライベート Go モジュールへのアクセスを設定する。
# 入力: COMMON_GO_MODULES_FETCH (env, organization-wide read scope の PAT を要求)
set -euo pipefail

git config --global url."https://x-access-token:${COMMON_GO_MODULES_FETCH}@github.com/kenyamaneko/".insteadOf "https://github.com/kenyamaneko/"
