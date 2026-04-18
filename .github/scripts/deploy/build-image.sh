#!/usr/bin/env bash
# build-image.sh — Docker イメージをビルドする。
# 入力: IMAGE, SHA_TAG, LATEST_TAG, COMMON_GO_MODULES_FETCH (env)
set -euo pipefail

docker build \
  --secret id=COMMON_GO_MODULES_FETCH,env=COMMON_GO_MODULES_FETCH \
  -t "${IMAGE}:${SHA_TAG}" \
  -t "${IMAGE}:${LATEST_TAG}" \
  .
