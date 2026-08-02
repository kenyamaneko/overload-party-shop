#!/usr/bin/env bash
# 算出済みタグから npm パッケージをビルドし、package.json にバージョンを反映する。
set -euo pipefail

: "${TAG:?TAG env required}"
: "${PREFIX:?PREFIX env required}"

version="${TAG#"${PREFIX}"}"
npm install
npm version "${version}" --no-git-tag-version
npm run build
