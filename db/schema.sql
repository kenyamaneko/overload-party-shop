-- overload-party-shop - PostgreSQL DDL (service-owned)
--
-- psqldef 互換。
-- Cross-schema reference（player_id -> account.players）は FK を張らない。

CREATE SCHEMA IF NOT EXISTS shop;

-- =============================================================================
-- Schema-local helpers
-- =============================================================================

CREATE OR REPLACE FUNCTION shop.update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

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

-- ドメイン (Subscription / OneTimePurchase) と外部識別 (Apple/Google トークン) を
-- 分離する。ドメインテーブルは外部仕様に依存せず、トークンテーブルだけが Apple/Google の
-- 識別子を保持する。Apple/Google のトークン仕様変更（例: 更新ごとに新トークン発行）が
-- 起きた場合でも対応する token テーブルに行を追加するだけで吸収できる。

CREATE TABLE shop.subscriptions (
  subscription_id      BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY, -- 自動採番
  player_id            UUID NOT NULL,                                -- 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK)
  product_id           VARCHAR(50) NOT NULL,                          -- 商品ID
  status               VARCHAR(20) NOT NULL,                          -- active / cancelled / grace_period / expired / revoked
  current_period_start TIMESTAMPTZ NOT NULL,                          -- 課金期間開始日時
  current_period_end   TIMESTAMPTZ NOT NULL,                          -- 課金期間終了日時
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),            -- 初回購入日時
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),            -- 更新日時
  PRIMARY KEY (subscription_id)
);
CREATE INDEX idx_subscriptions_player_id ON shop.subscriptions (player_id);
CREATE TRIGGER trg_subscriptions_updated_at BEFORE UPDATE ON shop.subscriptions FOR EACH ROW EXECUTE FUNCTION shop.update_updated_at();

CREATE TABLE shop.one_time_purchases (
  purchase_id  BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY, -- 自動採番
  player_id    UUID NOT NULL,                                -- 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK)
  product_id   VARCHAR(50) NOT NULL,                          -- 商品ID
  purchased_at TIMESTAMPTZ NOT NULL DEFAULT now(),            -- 購入日時
  PRIMARY KEY (purchase_id)
);
CREATE INDEX idx_one_time_purchases_player_id ON shop.one_time_purchases (player_id);

-- ─── 外部識別 (Apple / Google) ───
-- token テーブルは「Apple/Google から得られる識別子」と「ドメイン FK」「監査用タイムスタンプ」だけを持つ。
-- サブスクトークンは UpdatedAt あり (将来の token 再関連付けに備える)、購入トークンは append-only。

CREATE TABLE shop.apple_subscription_tokens (
  token            VARCHAR(256) NOT NULL,                          -- Apple originalTransactionId
  subscription_id  BIGINT NOT NULL,                                 -- shop.subscriptions への FK
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (token),
  FOREIGN KEY (subscription_id) REFERENCES shop.subscriptions (subscription_id) ON DELETE CASCADE
);
CREATE TRIGGER trg_apple_subscription_tokens_updated_at BEFORE UPDATE ON shop.apple_subscription_tokens FOR EACH ROW EXECUTE FUNCTION shop.update_updated_at();

CREATE TABLE shop.google_subscription_tokens (
  token            VARCHAR(256) NOT NULL,                          -- Google purchaseToken
  subscription_id  BIGINT NOT NULL,                                 -- shop.subscriptions への FK
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (token),
  FOREIGN KEY (subscription_id) REFERENCES shop.subscriptions (subscription_id) ON DELETE CASCADE
);
CREATE TRIGGER trg_google_subscription_tokens_updated_at BEFORE UPDATE ON shop.google_subscription_tokens FOR EACH ROW EXECUTE FUNCTION shop.update_updated_at();

CREATE TABLE shop.apple_purchase_tokens (
  token        VARCHAR(256) NOT NULL,                              -- Apple transactionId
  purchase_id  BIGINT NOT NULL,                                     -- shop.one_time_purchases への FK
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (token),
  FOREIGN KEY (purchase_id) REFERENCES shop.one_time_purchases (purchase_id) ON DELETE CASCADE
);

CREATE TABLE shop.google_purchase_tokens (
  token        VARCHAR(256) NOT NULL,                              -- Google purchaseToken
  purchase_id  BIGINT NOT NULL,                                     -- shop.one_time_purchases への FK
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (token),
  FOREIGN KEY (purchase_id) REFERENCES shop.one_time_purchases (purchase_id) ON DELETE CASCADE
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
-- Purchase 成功時に書き込まれ、faction-selected イベントの outbox 行は
-- この INSERT と同一トランザクションで書き込まれる。
CREATE TABLE shop.player_owned_factions (
  player_id  UUID NOT NULL,                           -- 所有プレイヤー
  faction    VARCHAR(20) NOT NULL CHECK (faction IN ('SHE', 'Tenki', 'Sugar', 'Tuners')), -- 所有ファクション
  granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),      -- 付与日時
  PRIMARY KEY (player_id, faction)
);

-- =============================================================================
-- Transactional Outbox
-- =============================================================================

-- shop.outbox_events は Pub/Sub 発行を DB commit と atomic に揃えるための
-- outbox。ビジネス行の INSERT/UPDATE と同一トランザクションで 1 行 INSERT され、
-- 別プロセスの worker が未 publish 行を claim して Pub/Sub に送出する。
-- これにより「DB commit は成功したが publish が失敗してイベントが失われる」
-- dual-write 問題を構造的に排除する。
--
-- event_id は payload 内の eventId と一致し、再試行しても同じ値。subscriber 側の
-- 冪等性キー (processed_events / 複合 PK) と協調して at-least-once を担保する。
-- 配信済み行は削除しない (監査・障害調査用に保持)。
CREATE TABLE shop.outbox_events (
  event_id          UUID NOT NULL,                        -- payload 内 eventId と一致
  event_type        VARCHAR(100) NOT NULL,                -- 論理イベント種別 (apishop.EventType*)。adapter が物理 topic に解決する
  payload           JSONB NOT NULL,                       -- JSON Marshal 済みイベント本体
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),   -- enqueue 日時
  published_at      TIMESTAMPTZ,                          -- NULL = 未配信
  failure_count     INT NOT NULL DEFAULT 0,               -- 連続失敗回数
  last_error        TEXT,                                 -- 直近エラーメッセージ
  last_attempted_at TIMESTAMPTZ,                          -- 直近 publish 試行日時
  PRIMARY KEY (event_id)
);

-- 未配信行だけに効く partial index。publish 済み行が積み上がっても worker の
-- claim クエリ SELECT ... WHERE published_at IS NULL ORDER BY created_at が常に O(未配信数)。
CREATE INDEX idx_outbox_events_unpublished
  ON shop.outbox_events (created_at)
  WHERE published_at IS NULL;
