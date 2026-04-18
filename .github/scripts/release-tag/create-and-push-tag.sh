#!/usr/bin/env bash
# create-and-push-tag.sh — git tag を作成して push する。重複タグはエラーにする。
# 入力: TAG (env)
set -euo pipefail

if git rev-parse "${TAG}" >/dev/null 2>&1; then
  echo "::error::Tag ${TAG} already exists"
  exit 1
fi
git tag "${TAG}"
git push origin "${TAG}"
