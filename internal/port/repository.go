package port

import (
	"context"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// ProductRepo は shop.products および付帯副表 (product_faction / product_cosmetic) の読み取り専用 repo。
// 戻り値は type 別 per-type 型を束ねる ProductView interface。
type ProductRepo interface {
	GetActiveProducts(ctx context.Context) ([]domain.ProductView, error)
	GetProductByID(ctx context.Context, productID string) (domain.ProductView, error)
}

// FactionPurchaseRepo は faction_set 購入 aggregate を扱う。
type FactionPurchaseRepo interface {
	// CreatePurchase は購入確定に伴う行と outbox events を単一 tx で挿入する。
	// 既存 token があれば created=false で既存 purchase_id を埋めて no-op return する。
	CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, faction, cardPackID, platform, purchaseToken string, eventsOnCreate []OutboxEvent) (created bool, err error)
	ListOwnedFactions(ctx context.Context, playerID string) ([]string, error)
}

// CardPackPurchaseRepo は card_pack 商品 (faction を伴わない pure pack 購入) aggregate を扱う。
type CardPackPurchaseRepo interface {
	// CreatePurchase は購入確定に伴う行と outbox event を単一 tx で挿入する。
	// 既存 token があれば created=false で no-op return する。
	CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, cardPackID, platform, purchaseToken string, eventOnCreate OutboxEvent) (created bool, err error)
	HasPlayerCardPack(ctx context.Context, playerID, cardPackID string) (bool, error)
}

// ItemPurchaseRepo は cosmetic 購入 aggregate を扱う。
type ItemPurchaseRepo interface {
	// CreatePurchase は purchase + token + player_item を単一 tx で挿入する。
	// 既存 token があれば created=false で既存 purchase_id を埋めて no-op return する。
	CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, item *domain.PlayerItem, platform, purchaseToken string) (created bool, err error)
	HasPlayerItem(ctx context.Context, playerID, itemType string, itemNo int64) (bool, error)
	ListPlayerItems(ctx context.Context, playerID string) ([]*domain.PlayerItem, error)
}

// PurchaseLookupRepo は token から一回きり購入を引く読み取り専用 repo。
type PurchaseLookupRepo interface {
	FindPurchaseByToken(ctx context.Context, platform, purchaseToken string) (*domain.OneTimePurchase, error)
}

// SubscriptionRepo はサブスクリプション + Apple/Google トークンマッピングの CRUD。
type SubscriptionRepo interface {
	CreateSubscription(ctx context.Context, sub *domain.Subscription, platform, purchaseToken string, event OutboxEvent) error
	// GetLatestSubscription は player の最新サブスクリプション 1 行を platform 横断で返す。
	GetLatestSubscription(ctx context.Context, playerID string) (*domain.Subscription, error)
	FindSubscriptionByToken(ctx context.Context, platform, purchaseToken string) (*domain.Subscription, error)
	// UpdateSubscription は outbox 行を伴わない更新 (cancelled 遷移用)。
	UpdateSubscription(ctx context.Context, sub *domain.Subscription) error
	// UpdateSubscriptionWithEvent は subscriptions 更新と outbox 行 INSERT を同一 tx で行う。
	UpdateSubscriptionWithEvent(ctx context.Context, sub *domain.Subscription, event OutboxEvent) error
}

// GameConfigRepo はゲーム設定値を読み取る。キー不在は ErrNotFound。
type GameConfigRepo interface {
	GetInt64(ctx context.Context, key string) (int64, error)
}
