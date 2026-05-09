#!/usr/bin/env bash
# asyncapi-diff.sh — base ブランチ (PR の merge 先) と PR head の data/asyncapi.yaml を比較し、
# 後方非互換な変更があれば exit 1 で CI を fail させる。
#
# base に data/asyncapi.yaml が存在しない場合はエラー扱い (本仕様への移行後は必ず存在する前提)。
set -euo pipefail

BASE_REF="${1:?base ref is required (e.g. origin/main)}"
SPEC_PATH="data/asyncapi.yaml"
BASE_TMP="/tmp/base-asyncapi.yaml"

if ! git show "${BASE_REF}:${SPEC_PATH}" >"${BASE_TMP}" 2>/dev/null; then
  echo "::error::${BASE_REF} に ${SPEC_PATH} が存在しない。base ブランチに spec が無いケースは想定外のため失敗扱い。"
  exit 1
fi

asyncapi diff "${BASE_TMP}" "${SPEC_PATH}" --type breaking
