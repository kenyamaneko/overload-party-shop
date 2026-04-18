#!/usr/bin/env bash
# configure-go-private-modules.sh — プライベート Go モジュールへのアクセスを設定する。
# 入力: COMMON_GO_MODULES_FETCH (env)
set -euo pipefail

git config --global url."https://x-access-token:${COMMON_GO_MODULES_FETCH}@github.com/kenyamaneko/overload-party-common".insteadOf "https://github.com/kenyamaneko/overload-party-common"
