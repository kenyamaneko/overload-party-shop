package repository

import (
	"context"
	"fmt"
	"sync"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// MockSubscriptionRepository はテスト用のインメモリ SubscriptionRepo 実装。
type MockSubscriptionRepository struct {
	mu                 sync.Mutex
	nextSubscriptionID int64
	subscriptions      map[string][]*apishop.Subscription // keyed by playerID
}

var _ port.SubscriptionRepo = (*MockSubscriptionRepository)(nil)

// NewMockSubscriptionRepository はテスト用の MockSubscriptionRepository を構築する。
func NewMockSubscriptionRepository() *MockSubscriptionRepository {
	return &MockSubscriptionRepository{
		subscriptions: make(map[string][]*apishop.Subscription),
	}
}

// CreateSubscription はサブスクリプションをインメモリに記録する。
func (r *MockSubscriptionRepository) CreateSubscription(ctx context.Context, sub *apishop.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextSubscriptionID++
	sub.SubscriptionID = r.nextSubscriptionID
	r.subscriptions[sub.PlayerID] = append(r.subscriptions[sub.PlayerID], sub)
	return nil
}

// GetActiveSubscription はアクティブなサブスクリプションをインメモリから返す。
func (r *MockSubscriptionRepository) GetActiveSubscription(ctx context.Context, playerID string) (*apishop.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.subscriptions[playerID] {
		if s.Status == apishop.SubscriptionStatusActive {
			return s, nil
		}
	}
	return nil, nil
}

// FindSubscriptionByToken はトークンでサブスクリプションをインメモリから検索する。
func (r *MockSubscriptionRepository) FindSubscriptionByToken(ctx context.Context, purchaseToken string) (*apishop.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, subs := range r.subscriptions {
		for _, s := range subs {
			if s.PurchaseToken == purchaseToken {
				return s, nil
			}
		}
	}
	return nil, nil
}

// UpdateSubscription はサブスクリプションをインメモリで更新する。
func (r *MockSubscriptionRepository) UpdateSubscription(ctx context.Context, sub *apishop.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, subs := range r.subscriptions {
		for i, s := range subs {
			if s.SubscriptionID == sub.SubscriptionID {
				subs[i] = sub
				return nil
			}
		}
	}
	return fmt.Errorf("subscription %d not found", sub.SubscriptionID)
}
