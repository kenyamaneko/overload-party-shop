package port

import (
	"context"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// ProductRepo は shop.products の読み取り専用 repo。
type ProductRepo interface {
	GetActiveProducts(ctx context.Context) ([]*domain.Product, error)
	GetProductByID(ctx context.Context, productID string) (*domain.Product, error)
}

// FactionPurchaseRepo は faction_set 購入 aggregate を扱う。
// shop.one_time_purchases + shop.{apple,google}_purchase_tokens + shop.player_owned_factions
// + shop.outbox_events (faction-purchased) を単一トランザクションで操作する。
type FactionPurchaseRepo interface {
	// CreatePurchase は purchase + token + owned_faction + outbox event を単一 tx で挿入する。
	// 既存 token があれば created=false で既存 purchase_id を purchase.PurchaseID に埋めて
	// no-op で返し、owned_faction / outbox 挿入もスキップする（初回の outbox 行で配信が担保される）。
	// eventOnCreate は created=true のときのみ outbox に書き込まれる。
	CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, faction, platform, purchaseToken string, eventOnCreate OutboxEvent) (created bool, err error)
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
	CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, item *domain.PlayerItem, platform, purchaseToken string) (created bool, err error)
	HasPlayerItem(ctx context.Context, playerID, itemType string, itemNo int64) (bool, error)
	ListPlayerItems(ctx context.Context, playerID string) ([]*domain.PlayerItem, error)
}

// PurchaseLookupRepo は token からの一回きり購入引きを担当する cross-cutting 読み取り repo。
// faction / item どちらの aggregate にも属さず、webhook 冪等性チェック用の早期 short-circuit に使う。
type PurchaseLookupRepo interface {
	FindPurchaseByToken(ctx context.Context, platform, purchaseToken string) (*domain.OneTimePurchase, error)
}

// SubscriptionRepo はサブスクリプション + 対応する Apple/Google トークンマッピングの CRUD。
// 「特典が有効か」のドメイン判定は service 層 (isEntitled) が担う。
// repository は status によるフィルタを行わない。
//
// CreateSubscription は subscriptions 行と {apple,google}_subscription_tokens 行 + outbox 行を
// 同一トランザクションで挿入する。FindSubscriptionByToken は platform を見て対応 token
// テーブルを引いてから subscriptions に JOIN する。
type SubscriptionRepo interface {
	CreateSubscription(ctx context.Context, sub *domain.Subscription, platform, purchaseToken string, event OutboxEvent) error
	// GetLatestSubscription は player の最新サブスクリプション 1 行を返す (platform 横断)。
	GetLatestSubscription(ctx context.Context, playerID string) (*domain.Subscription, error)
	FindSubscriptionByToken(ctx context.Context, platform, purchaseToken string) (*domain.Subscription, error)
	// UpdateSubscription / UpdateSubscriptionWithEvent の使い分け: 解約 (cancelled)
	// 遷移はエンタイトルメント維持契約により premium-updated を発行しない。それ以外
	// の状態遷移 (renew/expire/revoke) は subscriber に状態を伝える必要がある。
	UpdateSubscription(ctx context.Context, sub *domain.Subscription) error
	UpdateSubscriptionWithEvent(ctx context.Context, sub *domain.Subscription, event OutboxEvent) error
}

// GameConfigRepo はゲーム設定値の読み取りを抽象化するインターフェースです。
// キーが存在しない場合は ErrNotFound を返す（fail-fast）。
type GameConfigRepo interface {
	GetInt64(ctx context.Context, key string) (int64, error)
}
