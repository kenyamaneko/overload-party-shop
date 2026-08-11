# shop スキーマ - データ設計

> **DDL の SSoT:** `db/schema.sql`

## 設計概要

shop スキーマは商品マスター・購入履歴・コスメティクスアイテム・ファクション/カードパック所有の read model を管理する。Apple / Google の IAP webhook を受け取り、購入結果を `card-pack-purchased` / `faction-acquired` / `premium-updated` Pub/Sub イベントとして publish する。

---

## テーブル構成

### products

商品マスター。ファクションセット・コスメティクス・サブスクリプションの 3 種類。

- **PK:** `product_id` (VARCHAR(50))
- **FK:** `requires_product_id` → `products` (自己参照)

<!-- BEGIN GENERATED: products -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `product_id` | VARCHAR(50) | No | 商品ID |
| `name` | VARCHAR(100) | No | 商品名 |
| `type` | VARCHAR(20) | No | 商品タイプ discriminator (faction_set / cosmetic / subscription) |
| `price` | BIGINT | No | 価格 (JPY) |
| `requires_product_id` | VARCHAR(50) | Yes | 購入前提の商品ID（拡張セット用、NULL: なし） |
| `description` | VARCHAR(500) | Yes | 商品説明 |
| `image_url` | VARCHAR(200) | Yes | 画像URL |
| `is_active` | BOOLEAN | No | 販売中フラグ |
<!-- END GENERATED: products -->

**設計判断:**
- `requires_product_id` の自己参照 FK により、拡張セットの購入前提チェックを DB 層で整合性保証する
- type 固有属性は副表 (`product_faction` / `product_card_pack` / `product_cosmetic` / `product_subscription`) に分離し、`products` 共通表は全 type で意味を持つ列のみ保持する。詳細は [ADR-031](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/adr/031-shop-products-normalization-and-faction-purchased-decomposition.md)

### product_faction

`type='faction_set'` 商品の付帯属性。`shop.products` 1 行に対して 1 行が必ず存在する (整合性は application 層で担保)。

- **PK:** `product_id`
- **FK:** `product_id` → `products(product_id)` ON DELETE CASCADE
- **CHECK:** `faction IN ('SHE', 'Tenki', 'Sugar', 'Tuners')` (selectable faction のみ)

<!-- BEGIN GENERATED: product_faction -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `product_id` | VARCHAR(50) | No | shop.products への FK |
| `faction` | VARCHAR(20) | No | 配布対象 faction |
<!-- END GENERATED: product_faction -->

### product_card_pack

`type IN ('faction_set','card_pack')` 商品の付帯属性。`card.card_pack.pack_id` への論理参照 (FK なし、[ADR-031](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/adr/031-shop-products-normalization-and-faction-purchased-decomposition.md) の「card 側との責務分界」)。faction_set 商品は本副表と `product_faction` の両方に行を持つ。

- **PK:** `product_id`
- **FK:** `product_id` → `products(product_id)` ON DELETE CASCADE

<!-- BEGIN GENERATED: product_card_pack -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `product_id` | VARCHAR(50) | No | 商品ID |
| `card_pack_id` | VARCHAR(50) | No | カードパックID |
<!-- END GENERATED: product_card_pack -->

### product_cosmetic

`type='cosmetic'` 商品の付帯属性。`cosmetic_items` への DB レベル FK が成立する。

- **PK:** `product_id`
- **FK:** `product_id` → `products(product_id)` ON DELETE CASCADE
- **FK:** `(item_type, item_no)` → `cosmetic_items(item_type, item_no)`

<!-- BEGIN GENERATED: product_cosmetic -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `product_id` | VARCHAR(50) | No | shop.products への FK |
| `item_type` | VARCHAR(20) | No | アイテム種別 (cosmetic_items 参照) |
| `item_no` | BIGINT | No | アイテム番号 (cosmetic_items 参照) |
<!-- END GENERATED: product_cosmetic -->

### product_subscription

`type='subscription'` 商品の付帯属性。課金周期等の variant 属性を保持する。

- **PK:** `product_id`
- **FK:** `product_id` → `products(product_id)` ON DELETE CASCADE
- **CHECK:** `period_months > 0`

<!-- BEGIN GENERATED: product_subscription -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `product_id` | VARCHAR(50) | No | shop.products への FK |
| `period_months` | INT | No | 課金周期 (月数。e.g. 1=monthly, 12=yearly) |
<!-- END GENERATED: product_subscription -->

### subscriptions

サブスクリプション契約の管理。

- **PK:** `(player_id, subscription_id)`
- **TRIGGER:** `updated_at` 自動更新
- `player_id` は `account.players` へのクロススキーマ参照（FK 無し）

<!-- BEGIN GENERATED: subscriptions -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `subscription_id` | BIGINT (IDENTITY) | No | 自動採番 |
| `player_id` | UUID | No | 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK) |
| `product_id` | VARCHAR(50) | No | 商品ID |
| `status` | VARCHAR(20) | No | active / cancelled / grace_period / expired / revoked |
| `current_period_start` | TIMESTAMPTZ | No | 課金期間開始日時 |
| `current_period_end` | TIMESTAMPTZ | No | 課金期間終了日時 |
| `created_at` | TIMESTAMPTZ | No | 初回購入日時 |
| `updated_at` | TIMESTAMPTZ | No | 更新日時 |
<!-- END GENERATED: subscriptions -->

**設計判断:**
- ステータス変更時に `premium-updated` Pub/Sub イベントを publish し、account が `players.is_premium` / `premium_expires_at` を更新する

### one_time_purchases

買い切り購入履歴。ファクションセットやコスメティクスの購入記録。

- **PK:** `(player_id, purchase_id)`
- `player_id` は `account.players` へのクロススキーマ参照（FK 無し）

<!-- BEGIN GENERATED: one_time_purchases -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `purchase_id` | BIGINT (IDENTITY) | No | 自動採番 |
| `player_id` | UUID | No | 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK) |
| `product_id` | VARCHAR(50) | No | 商品ID |
| `purchased_at` | TIMESTAMPTZ | No | 購入日時 |
<!-- END GENERATED: one_time_purchases -->

### apple_subscription_tokens

`subscriptions` の外部識別子テーブル (Apple)。

- **PK:** `token`
- **FK:** `subscription_id` → `subscriptions(subscription_id)` ON DELETE CASCADE
- **TRIGGER:** `updated_at` 自動更新

<!-- BEGIN GENERATED: apple_subscription_tokens -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `token` | VARCHAR(256) | No | Apple originalTransactionId |
| `subscription_id` | BIGINT | No | shop.subscriptions への FK |
| `created_at` | TIMESTAMPTZ | No | 作成日時 |
| `updated_at` | TIMESTAMPTZ | No | 更新日時 |
<!-- END GENERATED: apple_subscription_tokens -->

### google_subscription_tokens

`subscriptions` の外部識別子テーブル (Google)。

- **PK:** `token`
- **FK:** `subscription_id` → `subscriptions(subscription_id)` ON DELETE CASCADE
- **TRIGGER:** `updated_at` 自動更新

<!-- BEGIN GENERATED: google_subscription_tokens -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `token` | VARCHAR(256) | No | Google purchaseToken |
| `subscription_id` | BIGINT | No | shop.subscriptions への FK |
| `created_at` | TIMESTAMPTZ | No | 作成日時 |
| `updated_at` | TIMESTAMPTZ | No | 更新日時 |
<!-- END GENERATED: google_subscription_tokens -->

**設計判断:**
- ドメインテーブル (`subscriptions`) と外部識別子テーブルを分離しているのは、Apple/Google が token の意味・発行タイミングを将来変更しても、ドメイン側のスキーマを変更せず token テーブル側の追加・差し替えだけで吸収するため。サブスクリプション用トークンのみ `updated_at` を持つのは、既存の subscription 行への token 再関連付けに備えたため。

### apple_purchase_tokens

`one_time_purchases` の外部識別子テーブル (Apple)。

- **PK:** `token`
- **FK:** `purchase_id` → `one_time_purchases(purchase_id)` ON DELETE CASCADE

<!-- BEGIN GENERATED: apple_purchase_tokens -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `token` | VARCHAR(256) | No | Apple transactionId |
| `purchase_id` | BIGINT | No | shop.one_time_purchases への FK |
| `created_at` | TIMESTAMPTZ | No | 作成日時 |
<!-- END GENERATED: apple_purchase_tokens -->

### google_purchase_tokens

`one_time_purchases` の外部識別子テーブル (Google)。

- **PK:** `token`
- **FK:** `purchase_id` → `one_time_purchases(purchase_id)` ON DELETE CASCADE

<!-- BEGIN GENERATED: google_purchase_tokens -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `token` | VARCHAR(256) | No | Google purchaseToken |
| `purchase_id` | BIGINT | No | shop.one_time_purchases への FK |
| `created_at` | TIMESTAMPTZ | No | 作成日時 |
<!-- END GENERATED: google_purchase_tokens -->

**設計判断:**
- 単発購入は再関連付けが発生しない前提のため `updated_at` を持たず append-only。

### cosmetic_items

コスメティクスアイテムマスター（プレイマット・スリーブ・アイコン・スタンプ）。

- **PK:** `(item_type, item_no)`

<!-- BEGIN GENERATED: cosmetic_items -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `item_type` | VARCHAR(20) | No | アイテム種別（playmat / sleeve / icon / stamp） |
| `item_no` | BIGINT | No | アイテム番号 |
| `item_name` | VARCHAR(100) | No | アイテム名 |
| `description` | VARCHAR(500) | Yes | 説明文 |
| `is_purchasable` | BOOLEAN | No | 購入可能フラグ |
| `is_active` | BOOLEAN | No | 有効フラグ |
<!-- END GENERATED: cosmetic_items -->

### player_items

プレイヤーのコスメティクスアイテム所持。

- **PK:** `(player_id, item_type, item_no)`
- `player_id` は `account.players` へのクロススキーマ参照（FK 無し）

<!-- BEGIN GENERATED: player_items -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `player_id` | UUID | No | 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK) |
| `item_type` | VARCHAR(20) | No | アイテム種別 |
| `item_no` | BIGINT | No | アイテム番号 |
| `acquired_at` | TIMESTAMPTZ | No | 獲得日時 |
<!-- END GENERATED: player_items -->

**装備状態の管理:**

装備中のアイテムは使用時に即座に参照できるよう、所持テーブルではなく使用先テーブルに直接保持する。

| アイテム種別 | 装備先 | カラム |
|---|---|---|
| アイコン | `account.players` | `equipped_icon_no` |
| プレイマット | `card.decks` | `playmat_no` |
| スリーブ | `card.decks` | `sleeve_no` |

### player_owned_factions

shop 購入経由で付与されたファクション所有状況の shop ローカル read model。

- **PK:** `(player_id, faction)`
- **CHECK:** `faction IN ('SHE', 'Tenki', 'Sugar', 'Tuners')`

<!-- BEGIN GENERATED: player_owned_factions -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `player_id` | UUID | No | 所有プレイヤー |
| `faction` | VARCHAR(20) | No | 所有ファクション |
| `granted_at` | TIMESTAMPTZ | No | 付与日時 |
<!-- END GENERATED: player_owned_factions -->

**設計判断:**
- authoritative な所有状況は `account.player_factions` が持つが、shop は cross-schema 読み込みを許されないため、shop 内で独立した read model を保持する
- Purchase 成功時に INSERT し、その後 `faction-acquired` イベントを publish する (account が購読)

### player_owned_card_packs

shop 購入経由で付与された card_pack 所有状況の shop ローカル read model。再購入禁止チェック ([ADR-031](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/adr/031-shop-products-normalization-and-faction-purchased-decomposition.md) §1) に使う。

- **PK:** `(player_id, card_pack_id)`

<!-- BEGIN GENERATED: player_owned_card_packs -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `player_id` | UUID | No | 所有プレイヤー |
| `card_pack_id` | VARCHAR(50) | No | 所有カードパック |
| `granted_at` | TIMESTAMPTZ | No | 付与日時 |
<!-- END GENERATED: player_owned_card_packs -->

**設計判断:**
- faction_set / card_pack の両方の商品 type で本表を使う (faction_set は同時に `player_owned_factions` にも書く)
- Purchase 成功時に INSERT し、その後 `card-pack-purchased` イベントを publish する (card が購読)
- `card_pack_id` は `card.card_pack.pack_id` への論理参照 (FK なし、整合性は CI と DLQ で担保)

### outbox_events

Pub/Sub 発行を DB commit と同一トランザクションに揃えるための Transactional Outbox。

- **PK:** `event_id`

<!-- BEGIN GENERATED: outbox_events -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `event_id` | UUID | No | payload 内 eventId と一致 |
| `event_type` | VARCHAR(100) | No | 論理イベント種別 (apishop.EventType*)。adapter が物理 topic に解決する |
| `payload` | JSONB | No | JSON Marshal 済みイベント本体 |
| `created_at` | TIMESTAMPTZ | No | enqueue 日時 |
| `published_at` | TIMESTAMPTZ | Yes | NULL = 未配信 |
| `failure_count` | INT | No | 連続失敗回数 |
| `last_error` | TEXT | Yes | 直近エラーメッセージ |
| `last_attempted_at` | TIMESTAMPTZ | Yes | 直近 publish 試行日時 |
<!-- END GENERATED: outbox_events -->

**設計判断:**
- ビジネステーブルへの書き込みと同一トランザクションで INSERT し、DB commit と Pub/Sub publish の dual-write 問題を避ける ([ADR-073](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/adr/073-shop-transactional-outbox.md))。配信済み行も削除せず保持する (監査・障害調査のため)。

---

## Pub/Sub イベントと subscriber

| イベント | 発行契機 | subscriber |
|---|---|---|
| `card-pack-purchased` | `faction_set` / `card_pack` 商品購入の DB commit 後 (worker が outbox 消費) | card |
| `faction-acquired` | `faction_set` 商品購入の DB commit 後 (worker が outbox 消費) | account |
| `premium-updated` | サブスクリプション状態変化時 (解約時は除く) | account |

gateway は Pub/Sub event を client 通知に転用しないため、いずれのイベントも購読しない ([ADR-027](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/adr/027-gateway-pubsub-fanout-removal.md) / [ADR-031](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/adr/031-shop-products-normalization-and-faction-purchased-decomposition.md))。

---

## テーブル間リレーション

```
products (PK: product_id)
  │
  ├── FK: requires_product_id → products (自己参照、拡張セットの前提商品)
  ├── 1:0..1 ── product_faction      (FK: product_id, type='faction_set'           のみ)
  ├── 1:0..1 ── product_card_pack    (FK: product_id, type IN ('faction_set','card_pack'))
  ├── 1:0..1 ── product_cosmetic     (FK: product_id, type='cosmetic'              のみ)
  └── 1:0..1 ── product_subscription (FK: product_id, type='subscription'          のみ)

[account.players] ─ ─ ─ (cross-schema, app-level)
  │
  ├── 1:N ── subscriptions      (PK: player_id, subscription_id)
  ├── 1:N ── one_time_purchases (PK: player_id, purchase_id)
  ├── 1:N ── player_items       (PK: player_id, item_type, item_no)
  ├── 1:N ── player_owned_factions   (PK: player_id, faction)
  └── 1:N ── player_owned_card_packs (PK: player_id, card_pack_id)

cosmetic_items (PK: item_type, item_no)
  ├── product_cosmetic (FK: item_type, item_no)
  └── （player_items も参照するが FK は張らない）
```

---

## インデックス戦略

現時点で明示的なセカンダリインデックスは定義していない。PK の複合キーが主要クエリパスをカバーする。将来的に商品一覧のフィルタリングが必要になった場合は `products(type, is_active)` 等を検討する。
