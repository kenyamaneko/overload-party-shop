package port

import (
	"context"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// ProductRepo は shop.products の読み取り専用 repo。
type ProductRepo interface {
	GetActiveProducts(ctx context.Context) ([]*apishop.Product, error)
	GetProductByID(ctx context.Context, productID string) (*apishop.Product, error)
}

// FactionPurchaseRepo は faction_set 購入 aggregate を扱う。
// shop.one_time_purchases + shop.{apple,google}_purchase_tokens + shop.player_owned_factions
// + shop.outbox_events (faction-purchased) を単一トランザクションで操作する。
type FactionPurchaseRepo interface {
	// CreatePurchase は purchase + token + owned_faction + outbox event を単一 tx で挿入する。
	// 既存 token があれば created=false で既存 purchase_id を purchase.PurchaseID に埋めて
	// no-op で返し、owned_faction / outbox 挿入もスキップする（初回の outbox 行で配信が担保される）。
	// eventOnCreate は created=true のときのみ outbox に書き込まれる。
	CreatePurchase(ctx context.Context, purchase *apishop.OneTimePurchase, faction, platform, purchaseToken string, eventOnCreate OutboxEvent) (created bool, err error)
	ListOwnedFactions(ctx context.Context, playerID string) ([]string, error)
}

// ItemPurchaseRepo は cosmetic 購入 aggregate を扱う。
// shop.one_time_purchases + shop.{apple,google}_purchase_tokens + shop.player_items
// を単一トランザクションで操作する。cosmetic 購入は現状イベントを発行しないため
// outbox には書かない。
type ItemPurchaseRepo interface {
	// CreatePurchase は purchase + token + player_item を単一 tx で挿入する。
	// 既存 token があれば created=false で既存 purchase_id を purchase.PurchaseID に埋めて
	// no-op で返し、player_item の挿入もスキップする。
	CreatePurchase(ctx context.Context, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem, platform, purchaseToken string) (created bool, err error)
	HasPlayerItem(ctx context.Context, playerID, itemType string, itemNo int64) (bool, error)
	ListPlayerItems(ctx context.Context, playerID string) ([]*apishop.PlayerItem, error)
}

// PurchaseLookupRepo は token からの一回きり購入引きを担当する cross-cutting 読み取り repo。
// faction / item どちらの aggregate にも属さず、webhook 冪等性チェック用の早期 short-circuit に使う。
type PurchaseLookupRepo interface {
	FindPurchaseByToken(ctx context.Context, platform, purchaseToken string) (*apishop.OneTimePurchase, error)
}

// SubscriptionRepo はサブスクリプション + 対応する Apple/Google トークンマッピングの CRUD。
// 「特典が有効か」のドメイン判定は service 層 (isEntitled) が担う。
// repository は status によるフィルタを行わない。
//
// CreateSubscription は subscriptions 行と {apple,google}_subscription_tokens 行 + outbox 行を
// 同一トランザクションで挿入する。FindSubscriptionByToken は platform を見て対応 token
// テーブルを引いてから subscriptions に JOIN する。
type SubscriptionRepo interface {
	// CreateSubscription は subscription + token + outbox event を単一 tx で挿入する。
	// 新規 Subscribe は常に premium-updated を発行するため event は必須。
	CreateSubscription(ctx context.Context, sub *apishop.Subscription, platform, purchaseToken string, event OutboxEvent) error
	// GetLatestSubscription は player の最新サブスクリプション 1 行を返す (platform 横断)。
	GetLatestSubscription(ctx context.Context, playerID string) (*apishop.Subscription, error)
	FindSubscriptionByToken(ctx context.Context, platform, purchaseToken string) (*apishop.Subscription, error)
	// UpdateSubscription は subscription 行のみ更新する。outbox には書かない。
	// 解約 (cancelled) 遷移など「premium-updated を発行しない」業務判断が確定している
	// 呼び出し側でのみ使う。
	UpdateSubscription(ctx context.Context, sub *apishop.Subscription) error
	// UpdateSubscriptionWithEvent は subscription 行 update と outbox 行 INSERT を
	// 同一 tx で行う。webhook 駆動の状態遷移 (renew/expire/revoke) で publish を
	// atomic に揃えるためのもの。
	UpdateSubscriptionWithEvent(ctx context.Context, sub *apishop.Subscription, event OutboxEvent) error
}

// GameConfigRepo はゲーム設定値の読み取りを抽象化するインターフェースです。
// キーが存在しない場合は ErrNotFound を返す（fail-fast）。
type GameConfigRepo interface {
	GetInt64(ctx context.Context, key string) (int64, error)
}
