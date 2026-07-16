#!/usr/bin/env bash
# codegen.sh — OpenAPI/AsyncAPI 型と DB スキーマ doc を再生成する。
set -euo pipefail

pip install \
  "overload-party-doc-tools @ git+https://github.com/kenyamaneko/overload-party-common.git@main#subdirectory=packages/doc-tools" \
  "overload-party-codegen-tools @ git+https://github.com/kenyamaneko/overload-party-common.git@main#subdirectory=packages/codegen-tools" \
  "overload-party-asyncapi-codegen-tools @ git+https://github.com/kenyamaneko/overload-party-common.git@main#subdirectory=packages/asyncapi-codegen-tools"

scripts/generate_types.sh
python3 scripts/generate_schema_doc.py
