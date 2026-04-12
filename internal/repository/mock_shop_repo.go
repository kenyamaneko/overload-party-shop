package repository

import (
	"context"
	"fmt"
	"sync"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// MockShopRepository はテスト用のインメモリ ShopRepository 実装。
type MockShopRepository struct {
	mu             sync.Mutex
	nextPurchaseID int64
	products       map[string]*apishop.Product
	purchases      map[string][]*apishop.OneTimePurchase // keyed by playerID
	playerItems    map[string][]*apishop.PlayerItem      // keyed by playerID
}

var _ port.ShopRepository = (*MockShopRepository)(nil)

// NewMockShopRepository はテスト用の MockShopRepository を構築する。
func NewMockShopRepository() *MockShopRepository {
	return &MockShopRepository{
		products:    make(map[string]*apishop.Product),
		purchases:   make(map[string][]*apishop.OneTimePurchase),
		playerItems: make(map[string][]*apishop.PlayerItem),
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

// FindPurchaseByToken はトークンで購入レコードをインメモリから検索する。
func (r *MockShopRepository) FindPurchaseByToken(_ context.Context, playerID, purchaseToken string) (*apishop.OneTimePurchase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.purchases[playerID] {
		if p.PurchaseToken == purchaseToken {
			return p, nil
		}
	}
	return nil, nil
}

// CreatePurchase は購入レコードをインメモリに記録する。
func (r *MockShopRepository) CreatePurchase(_ context.Context, purchase *apishop.OneTimePurchase) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.purchases[purchase.PlayerID] {
		if p.PurchaseToken == purchase.PurchaseToken {
			purchase.PurchaseID = p.PurchaseID
			return nil
		}
	}
	r.nextPurchaseID++
	purchase.PurchaseID = r.nextPurchaseID
	r.purchases[purchase.PlayerID] = append(r.purchases[purchase.PlayerID], purchase)
	return nil
}

// CreatePurchaseWithItem は購入レコードとプレイヤーアイテムをインメモリに記録する。
func (r *MockShopRepository) CreatePurchaseWithItem(_ context.Context, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.purchases[purchase.PlayerID] {
		if p.PurchaseToken == purchase.PurchaseToken {
			return nil
		}
	}
	r.nextPurchaseID++
	purchase.PurchaseID = r.nextPurchaseID
	r.purchases[purchase.PlayerID] = append(r.purchases[purchase.PlayerID], purchase)
	r.playerItems[purchase.PlayerID] = append(r.playerItems[purchase.PlayerID], item)
	return nil
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
