#!/usr/bin/env bash
# generate_types.sh — data/openapi.yaml から packages/api-shop/openapi_gen.go を再生成する。
#
# AsyncAPI 由来の event 型 (packages/api-shop/event.go) は手書きで保守する。
# 仕様変更時は data/asyncapi.yaml を先に更新し、event.go を同期させること
# (CI の asyncapi-diff ジョブで breaking change は検知される)。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT/packages/api-shop"

oapi-codegen -config openapi-codegen.yaml ../../data/openapi.yaml
