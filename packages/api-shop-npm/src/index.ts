// data/openapi.yaml から openapi-typescript で生成された components / paths の re-export。
// 利用者は本ファイルが re-export する schema 型を import すること。

import type { components, paths } from "./openapi.gen";

export type { components, paths };

type Schemas = components["schemas"];

export type HealthResponse = Schemas["HealthResponse"];
export type ProductListResponse = Schemas["ProductListResponse"];
export type ProductResponse = Schemas["ProductResponse"];
export type PurchaseRequest = Schemas["PurchaseRequest"];
export type PurchaseResponse = Schemas["PurchaseResponse"];
export type SubscribeResponse = Schemas["SubscribeResponse"];
export type Platform = Schemas["Platform"];
export type ProductType = Schemas["ProductType"];
