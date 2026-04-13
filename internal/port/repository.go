package port

import (
	"context"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// ShopRepository は shop ローカルの SQL 状態（products, purchases, cosmetic items）を
// 管理する。全書き込みは `shop` スキーマに閉じ、クロススキーマの状態変更
// （account.players, card.player_cards, account.player_factions）は Pub/Sub 経由で行う。
//
// CreatePurchase / CreatePurchaseWithItem は purchase 行と
// {apple,google}_purchase_tokens 行を同一トランザクションで挿入する。
// 既存トークンが見つかった場合はべき等に no-op で抜ける。
type ShopRepository interface {
	GetActiveProducts(ctx context.Context) ([]*apishop.Product, error)
	GetProductByID(ctx context.Context, productID string) (*apishop.Product, error)
	// FindPurchaseByToken は platform に応じた token テーブルから purchase を引く。
	// playerID は付加検証用 (token は token テーブル内で一意なので playerID なしでも一意特定可)。
	FindPurchaseByToken(ctx context.Context, platform, purchaseToken string) (*apishop.OneTimePurchase, error)
	// CreatePurchase は one_time_purchases + 対応 token 行を挿入する。faction set 等で使用。
	CreatePurchase(ctx context.Context, purchase *apishop.OneTimePurchase, platform, purchaseToken string) error
	// CreatePurchaseWithItem は cosmetic 購入で player_items 行も同 tx で挿入する。
	CreatePurchaseWithItem(ctx context.Context, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem, platform, purchaseToken string) error
	InsertPlayerItems(ctx context.Context, items []*apishop.PlayerItem) error
	HasPlayerItem(ctx context.Context, playerID, itemType string, itemNo int64) (bool, error)
	ListPlayerItems(ctx context.Context, playerID string) ([]*apishop.PlayerItem, error)
}

// SubscriptionRepo はサブスクリプション + 対応する Apple/Google トークンマッピングの CRUD。
// 「特典が有効か」のドメイン判定は service 層 (isEntitled) が担う。
// repository は status によるフィルタを行わない。
//
// CreateSubscription は subscriptions 行と {apple,google}_subscription_tokens 行を
// 同一トランザクションで挿入する。FindSubscriptionByToken は platform を見て対応 token
// テーブルを引いてから subscriptions に JOIN する。
type SubscriptionRepo interface {
	CreateSubscription(ctx context.Context, sub *apishop.Subscription, platform, purchaseToken string) error
	// GetLatestSubscription は player の最新サブスクリプション 1 行を返す (platform 横断)。
	GetLatestSubscription(ctx context.Context, playerID string) (*apishop.Subscription, error)
	FindSubscriptionByToken(ctx context.Context, platform, purchaseToken string) (*apishop.Subscription, error)
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

// GameConfigRepo はゲーム設定値の読み取りを抽象化するインターフェースです。
// キーが存在しない場合は ErrNotFound を返す（fail-fast）。
type GameConfigRepo interface {
	GetInt64(ctx context.Context, key string) (int64, error)
}
