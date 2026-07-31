#!/usr/bin/env bash
# CI で codegen を再実行する。common の生成ツールを取得してから、型とスキーマドキュメントを
# 生成する。生成後の差分検証は check-codegen-drift.sh が行う。
set -euo pipefail

COMMON_REPO="git+https://github.com/kenyamaneko/overload-party-common.git@main"

pip install \
  "overload-party-doc-tools @ ${COMMON_REPO}#subdirectory=packages/doc-tools" \
  "overload-party-codegen-tools @ ${COMMON_REPO}#subdirectory=packages/codegen-tools" \
  "overload-party-asyncapi-codegen-tools @ ${COMMON_REPO}#subdirectory=packages/asyncapi-codegen-tools"

scripts/generate_types.sh
python3 scripts/generate_schema_doc.py
