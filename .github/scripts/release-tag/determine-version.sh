#!/usr/bin/env bash
# determine-version.sh — PR のブランチ名からリリースバージョンを算出する。
# 入力: HEAD_REF (env), 出力: GITHUB_OUTPUT に "tag=..." を書き込み。
set -euo pipefail

if [[ "${HEAD_REF}" == release/v* ]]; then
  VERSION="${HEAD_REF#release/}"
elif [[ "${HEAD_REF}" == hotfix/* ]]; then
  LATEST=$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || echo "v0.0.0")
  IFS='.' read -r MAJOR MINOR PATCH <<< "${LATEST#v}"
  VERSION="v${MAJOR}.${MINOR}.$(( PATCH + 1 ))"
else
  echo "::error::Unexpected branch: ${HEAD_REF}"
  exit 1
fi
echo "tag=${VERSION}" >> "$GITHUB_OUTPUT"
echo "Resolved version: ${VERSION}"
