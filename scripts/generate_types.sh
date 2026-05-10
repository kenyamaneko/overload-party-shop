#!/usr/bin/env bash
# data/{openapi,asyncapi}.yaml から packages/api-shop (Go) と packages/api-shop-npm (TS) の型を再生成する。
# REST 部分は oapi-codegen / openapi-typescript、Pub/Sub 部分は asyncapi-codegen-tools を使う。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$REPO_ROOT/packages/api-shop"
oapi-codegen -config openapi-codegen.yaml ../../data/openapi.yaml

cd "$REPO_ROOT"
asyncapi-codegen \
  --input data/asyncapi.yaml \
  --output packages/api-shop/asyncapi_gen.go \
  --package apishop

npx --yes openapi-typescript@7 \
  data/openapi.yaml \
  --output packages/api-shop-npm/src/openapi.gen.ts
