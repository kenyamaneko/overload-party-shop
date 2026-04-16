#!/usr/bin/env bash
# build-and-push.sh — Docker イメージをビルドして Artifact Registry に push する。
# 入力: IMAGE, GITHUB_SHA, COMMON_GO_MODULES_FETCH (env)
set -euo pipefail

docker build \
  --secret id=COMMON_GO_MODULES_FETCH,env=COMMON_GO_MODULES_FETCH \
  -t "${IMAGE}:${GITHUB_SHA}" -t "${IMAGE}:latest" .
docker push "${IMAGE}:${GITHUB_SHA}"
docker push "${IMAGE}:latest"
