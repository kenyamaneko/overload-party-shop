-- overload-party-shop - PostgreSQL DDL (service-owned)
--
-- Scope (ADR-014):
--   shop.products            - 商品マスター
--   shop.subscriptions       - サブスクリプション契約
--   shop.one_time_purchases  - 単発購入履歴
--   shop.cosmetic_items      - コスメティクスマスター
--   shop.player_items        - プレイヤー所持コスメ
--
-- psqldef 互換。shared.update_updated_at() を先に作成しておくこと。
-- Cross-schema reference（player_id -> account.players）は FK を張らない。

CREATE SCHEMA IF NOT EXISTS shop;

-- =============================================================================
-- Shop (schema: shop)
-- =============================================================================

CREATE TABLE shop.products (
  product_id          VARCHAR(50) NOT NULL,                  -- 商品ID
  name                VARCHAR(100) NOT NULL,                 -- 商品名
  type                VARCHAR(20) NOT NULL,                  -- 商品タイプ (faction_set / cosmetic / subscription)
  price               BIGINT NOT NULL,                       -- 価格 (JPY)
  content             JSONB NOT NULL,                        -- 商品内容
  faction_id          VARCHAR(20) CHECK (faction_id IS NULL OR faction_id IN ('SHE', 'Tenki', 'Sugar', 'Tuners', 'Neutral')), -- 陣営（faction_set 商品のみ、それ以外は NULL）
  requires_product_id VARCHAR(50),                           -- 購入前提の商品ID（拡張セット用、NULL: なし）
  description         VARCHAR(500),                          -- 商品説明
  image_url           VARCHAR(200),                          -- 画像URL
  is_active           BOOLEAN NOT NULL,                      -- 販売中フラグ
  PRIMARY KEY (product_id),
  FOREIGN KEY (requires_product_id) REFERENCES shop.products(product_id)
);

CREATE TABLE shop.subscriptions (
  player_id            UUID NOT NULL, -- 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK)
  subscription_id      BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY, -- 自動採番
  product_id           VARCHAR(50) NOT NULL,          -- 商品ID
  platform             VARCHAR(10) NOT NULL,          -- apple / google
  purchase_token       VARCHAR(256) NOT NULL,         -- 購入トークン（Apple: originalTransactionId / Google: purchaseToken）
  status               VARCHAR(20) NOT NULL,          -- active / grace_period / expired / refunded
  current_period_start TIMESTAMPTZ NOT NULL,          -- 課金期間開始日時
  current_period_end   TIMESTAMPTZ NOT NULL,          -- 課金期間終了日時
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(), -- 初回購入日時
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(), -- 更新日時
  PRIMARY KEY (player_id, subscription_id)
);
CREATE TRIGGER trg_subscriptions_updated_at BEFORE UPDATE ON shop.subscriptions FOR EACH ROW EXECUTE FUNCTION shared.update_updated_at();

CREATE TABLE shop.one_time_purchases (
  player_id      UUID NOT NULL, -- 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK)
  purchase_id    BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY, -- 自動採番
  product_id     VARCHAR(50) NOT NULL,               -- 商品ID
  platform       VARCHAR(10) NOT NULL,               -- apple / google
  purchase_token VARCHAR(256) NOT NULL,              -- 購入トークン（Apple: originalTransactionId / Google: purchaseToken）
  purchased_at   TIMESTAMPTZ NOT NULL DEFAULT now(), -- 購入日時
  PRIMARY KEY (player_id, purchase_id)
);

-- =============================================================================
-- Cosmetics (schema: shop)
-- =============================================================================

CREATE TABLE shop.cosmetic_items (
  item_type      VARCHAR(20) NOT NULL,               -- アイテム種別（playmat / sleeve / icon / stamp）
  item_no        BIGINT NOT NULL,                    -- アイテム番号
  item_name      VARCHAR(100) NOT NULL,              -- アイテム名
  description    VARCHAR(500),                       -- 説明文
  is_purchasable BOOLEAN NOT NULL,                   -- 購入可能フラグ
  is_active      BOOLEAN NOT NULL,                   -- 有効フラグ
  PRIMARY KEY (item_type, item_no)
);

CREATE TABLE shop.player_items (
  player_id   UUID NOT NULL, -- 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK)
  item_type   VARCHAR(20) NOT NULL,                  -- アイテム種別
  item_no     BIGINT NOT NULL,                       -- アイテム番号
  acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),    -- 獲得日時
  PRIMARY KEY (player_id, item_type, item_no)
);

-- =============================================================================
-- Shop-local read models
-- =============================================================================

-- shop.player_owned_factions は shop_purchase 経由で付与されたファクション
-- 所有状況の shop ローカル射影。authoritative な所有状況は account.player_factions
-- が持つが、shop は cross-schema 読み込みを許されないため GetProducts の
-- IsOwned 判定用に shop 内で独立した read model を保持する。
-- Purchase 成功時に書き込まれ、faction-selected イベントの publish は
-- この INSERT の後に行う。
CREATE TABLE shop.player_owned_factions (
  player_id  UUID NOT NULL,                           -- 所有プレイヤー
  faction    VARCHAR(20) NOT NULL CHECK (faction IN ('SHE', 'Tenki', 'Sugar', 'Tuners')), -- 所有ファクション
  granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),      -- 付与日時
  PRIMARY KEY (player_id, faction)
);
