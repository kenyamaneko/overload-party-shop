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
	// 同じ token が既に保存されているときは何も保存せず成功を返す。
	CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, faction, cardPackID, platform, purchaseToken string, eventsOnCreate []OutboxEvent) error
	ListOwnedFactions(ctx context.Context, playerID string) ([]string, error)
}

// CardPackPurchaseRepo は card_pack 商品 (faction を伴わない pure pack 購入) aggregate を扱う。
type CardPackPurchaseRepo interface {
	// CreatePurchase は購入確定に伴う行と outbox event を単一 tx で挿入する。
	// 同じ token が既に保存されているときは何も保存せず成功を返す。
	CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, cardPackID, platform, purchaseToken string, eventOnCreate OutboxEvent) error
	HasPlayerCardPack(ctx context.Context, playerID, cardPackID string) (bool, error)
}

// ItemPurchaseRepo は cosmetic 購入 aggregate を扱う。
type ItemPurchaseRepo interface {
	// CreatePurchase は purchase + token + player_item を単一 tx で挿入する。
	// 同じ token が既に保存されているときは何も保存せず成功を返す。
	CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, item *domain.PlayerItem, platform, purchaseToken string) error
	HasPlayerItem(ctx context.Context, playerID, itemType string, itemNo int64) (bool, error)
	ListPlayerItems(ctx context.Context, playerID string) ([]*domain.PlayerItem, error)
}

// PurchaseLookupRepo は token から一回きり購入を引く読み取り専用 repo。
type PurchaseLookupRepo interface {
	FindPurchaseByToken(ctx context.Context, platform, purchaseToken string) (*domain.OneTimePurchase, error)
}

// SubscriptionRepo はサブスクリプション + Apple/Google トークンマッピングの CRUD。
type SubscriptionRepo interface {
	// CreateSubscription は subscriptions 行・token 行・outbox 行を同一 tx で挿入する。
	// 同じ token が既に保存されているときは何も保存せず成功を返す。
	CreateSubscription(ctx context.Context, subscription *domain.Subscription, platform, purchaseToken string, event OutboxEvent) error
	// GetLatestSubscription は player の最新サブスクリプション 1 行を platform 横断で返す。
	GetLatestSubscription(ctx context.Context, playerID string) (*domain.Subscription, error)
	FindSubscriptionByToken(ctx context.Context, platform, purchaseToken string) (*domain.Subscription, error)
	// UpdateSubscription は outbox 行を伴わない更新 (cancelled 遷移用)。対象の行が無ければ ErrNotFound を返す。
	UpdateSubscription(ctx context.Context, subscription *domain.Subscription) error
	// UpdateSubscriptionWithEvent は subscriptions 更新と outbox 行 INSERT を同一 tx で行う。対象の行が無ければ outbox 行を書かず ErrNotFound を返す。
	UpdateSubscriptionWithEvent(ctx context.Context, subscription *domain.Subscription, event OutboxEvent) error
}
