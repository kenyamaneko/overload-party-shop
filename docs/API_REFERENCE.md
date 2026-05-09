# Shop Service API Reference

API 契約は OpenAPI / AsyncAPI 仕様を SSoT として管理する (ADR-034)。

## REST (gateway → shop / 外部 webhook)

仕様: [`data/openapi.yaml`](../data/openapi.yaml)

仕様を人間可読な形でレビューしたい場合は、Swagger UI / Redoc / AsyncAPI Studio 等の任意のビュアーに `data/openapi.yaml` を読み込ませる。

```bash
# 例: redocly でローカル preview
npx @redocly/cli preview-docs data/openapi.yaml
```

## Pub/Sub events (shop が発行)

仕様: [`data/asyncapi.yaml`](../data/asyncapi.yaml)

物理 topic 名は overload-party-infra (Terraform) が SSoT であり、本サービスは ConfigMap 由来 env (`CARD_PACK_PURCHASED_TOPIC` / `FACTION_ACQUIRED_TOPIC` / `PREMIUM_UPDATED_TOPIC`) で解決する。

## Go SDK

外部消費者向けの Go モジュール: [`packages/api-shop`](../packages/api-shop/)

- `openapi_gen.go` — `data/openapi.yaml` から `oapi-codegen` で再生成 (CI で drift 検知)
- `event.go` — `data/asyncapi.yaml` 由来の event 型を手書きで保守 (CI で `asyncapi-diff` により breaking change 検知)
- `apishopfake/` / `apishopserverfake/` — 手書きの test helper
