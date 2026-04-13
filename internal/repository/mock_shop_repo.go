package repository

import (
	"context"
	"fmt"
	"sync"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// MockShopRepository はテスト用のインメモリ ShopRepository 実装。
// pg 実装と同じくドメイン (purchases) と外部識別 (apple/google purchase tokens) を分離して持つ。
type MockShopRepository struct {
	mu             sync.Mutex
	nextPurchaseID int64
	products       map[string]*apishop.Product
	purchases      map[int64]*apishop.OneTimePurchase  // keyed by purchase_id
	applePurchaseTokens  map[string]int64              // token -> purchase_id
	googlePurchaseTokens map[string]int64              // token -> purchase_id
	playerItems    map[string][]*apishop.PlayerItem    // keyed by playerID
}

var _ port.ShopRepository = (*MockShopRepository)(nil)

// NewMockShopRepository はテスト用の MockShopRepository を構築する。
func NewMockShopRepository() *MockShopRepository {
	return &MockShopRepository{
		products:             make(map[string]*apishop.Product),
		purchases:            make(map[int64]*apishop.OneTimePurchase),
		applePurchaseTokens:  make(map[string]int64),
		googlePurchaseTokens: make(map[string]int64),
		playerItems:          make(map[string][]*apishop.PlayerItem),
	}
}

// AddProduct はテスト用に商品をプリセットする。
func (r *MockShopRepository) AddProduct(product *apishop.Product) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.products[product.ProductID] = product
}

// GetActiveProducts はアクティブな商品一覧をインメモリから返す。
func (r *MockShopRepository) GetActiveProducts(_ context.Context) ([]*apishop.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*apishop.Product
	for _, p := range r.products {
		if p.IsActive {
			result = append(result, p)
		}
	}
	return result, nil
}

// GetProductByID は指定 ID の商品をインメモリから返す。
func (r *MockShopRepository) GetProductByID(_ context.Context, productID string) (*apishop.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.products[productID]
	if !ok {
		return nil, fmt.Errorf("product %s: %w", productID, port.ErrNotFound)
	}
	return p, nil
}

func (r *MockShopRepository) purchaseTokenStore(platform string) (map[string]int64, error) {
	switch platform {
	case apishop.PlatformIOS:
		return r.applePurchaseTokens, nil
	case apishop.PlatformAndroid:
		return r.googlePurchaseTokens, nil
	default:
		return nil, fmt.Errorf("unsupported purchase platform: %q", platform)
	}
}

// FindPurchaseByToken はトークンで purchase をインメモリから検索する。
func (r *MockShopRepository) FindPurchaseByToken(_ context.Context, platform, purchaseToken string) (*apishop.OneTimePurchase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	store, err := r.purchaseTokenStore(platform)
	if err != nil {
		return nil, err
	}
	purchaseID, ok := store[purchaseToken]
	if !ok {
		return nil, nil
	}
	return r.purchases[purchaseID], nil
}

// CreatePurchase は purchase + 対応 token をインメモリに記録する。既存トークンはべき等 no-op。
func (r *MockShopRepository) CreatePurchase(_ context.Context, purchase *apishop.OneTimePurchase, platform, purchaseToken string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.createPurchaseLocked(purchase, platform, purchaseToken)
	return err
}

// CreatePurchaseWithItem は purchase + token + player_items をインメモリに記録する。
// 既存トークン (べき等) では player_items 挿入もスキップ。
func (r *MockShopRepository) CreatePurchaseWithItem(_ context.Context, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem, platform, purchaseToken string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	created, err := r.createPurchaseLocked(purchase, platform, purchaseToken)
	if err != nil {
		return err
	}
	if !created {
		return nil
	}
	r.playerItems[item.PlayerID] = append(r.playerItems[item.PlayerID], item)
	return nil
}

func (r *MockShopRepository) createPurchaseLocked(purchase *apishop.OneTimePurchase, platform, purchaseToken string) (bool, error) {
	store, err := r.purchaseTokenStore(platform)
	if err != nil {
		return false, err
	}
	if existingID, ok := store[purchaseToken]; ok {
		purchase.PurchaseID = existingID
		return false, nil
	}
	r.nextPurchaseID++
	purchase.PurchaseID = r.nextPurchaseID
	r.purchases[purchase.PurchaseID] = purchase
	store[purchaseToken] = purchase.PurchaseID
	return true, nil
}

// InsertPlayerItems はプレイヤーアイテムをインメモリに記録する。
func (r *MockShopRepository) InsertPlayerItems(_ context.Context, items []*apishop.PlayerItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range items {
		r.playerItems[item.PlayerID] = append(r.playerItems[item.PlayerID], item)
	}
	return nil
}

func (r *MockShopRepository) ListPlayerItems(_ context.Context, playerID string) ([]*apishop.PlayerItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.playerItems[playerID]
	out := make([]*apishop.PlayerItem, len(items))
	copy(out, items)
	return out, nil
}

// HasPlayerItem はプレイヤーが指定アイテムを所有しているかをインメモリで確認する。
func (r *MockShopRepository) HasPlayerItem(_ context.Context, playerID, itemType string, itemNo int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.playerItems[playerID] {
		if item.ItemType == itemType && item.ItemNo == itemNo {
			return true, nil
		}
	}
	return false, nil
}
