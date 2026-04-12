package port

import (
	"context"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// ShopRepository は shop ローカルの SQL 状態（products, purchases, cosmetic items）を
// 管理する。全書き込みは `shop` スキーマに閉じ、クロススキーマの状態変更
// （account.players, card.player_cards, account.player_factions）は Pub/Sub 経由で行う。
type ShopRepository interface {
	GetActiveProducts(ctx context.Context) ([]*apishop.Product, error)
	GetProductByID(ctx context.Context, productID string) (*apishop.Product, error)
	FindPurchaseByToken(ctx context.Context, playerID, purchaseToken string) (*apishop.OneTimePurchase, error)
	// CreatePurchase は one_time_purchases 行を記録する。カード/faction の付与は
	// ここでは行わず、faction-selected イベント経由で card サービスが処理する。
	CreatePurchase(ctx context.Context, purchase *apishop.OneTimePurchase) error
	CreatePurchaseWithItem(ctx context.Context, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem) error
	InsertPlayerItems(ctx context.Context, items []*apishop.PlayerItem) error
	HasPlayerItem(ctx context.Context, playerID, itemType string, itemNo int64) (bool, error)
}

// SubscriptionRepo はサブスクリプション行の CRUD を提供する。
type SubscriptionRepo interface {
	CreateSubscription(ctx context.Context, sub *apishop.Subscription) error
	GetActiveSubscription(ctx context.Context, playerID string) (*apishop.Subscription, error)
	FindSubscriptionByToken(ctx context.Context, purchaseToken string) (*apishop.Subscription, error)
	UpdateSubscription(ctx context.Context, sub *apishop.Subscription) error
}

// OwnedFactionRepo は shop 購入によるfaction 所有状態のローカル投影。
// initial_selection の所有は shop では追跡しない。faction-selected イベント発行後に
// 行を書き込み、GetProducts が IsOwned フラグに使用する write-through read model。
// テーブルは `shop` スキーマに存在するためクロススキーマ書き込みではない。
type OwnedFactionRepo interface {
	Add(ctx context.Context, playerID, faction string) error
	List(ctx context.Context, playerID string) ([]string, error)
}

// TxRunner はトランザクション内での関数実行を提供する。
type TxRunner interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
