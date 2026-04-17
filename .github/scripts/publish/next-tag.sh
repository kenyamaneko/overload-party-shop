#!/usr/bin/env bash
# next-tag.sh — Go module の次バージョンタグを算出する。
# 入力: BUMP (patch|minor|major), 出力: GITHUB_OUTPUT に "tag=..." を書き込み。
set -euo pipefail

prefix="packages/api-shop/v"
latest=$(git tag --list "${prefix}*" --sort=-v:refname | head -n1 || true)
if [ -z "${latest}" ]; then
  next="${prefix}0.1.0"
else
  ver="${latest#${prefix}}"
  IFS='.' read -r major minor patch <<EOF
${ver}
EOF
  case "${BUMP}" in
    patch) patch=$((patch + 1)) ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    major) major=$((major + 1)); minor=0; patch=0 ;;
    *) echo "unknown bump: ${BUMP}" >&2; exit 1 ;;
  esac
  next="${prefix}${major}.${minor}.${patch}"
fi
echo "tag=${next}" >> "${GITHUB_OUTPUT}"
