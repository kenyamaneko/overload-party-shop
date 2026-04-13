package repository

import (
	"context"
	"fmt"
	"sync"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// MockSubscriptionRepository はテスト用のインメモリ SubscriptionRepo 実装。
// pg 実装と同じくドメイン (subscriptions) と外部識別 (apple/google subscription tokens) を分離。
type MockSubscriptionRepository struct {
	mu                 sync.Mutex
	nextSubscriptionID int64
	subscriptions      map[int64]*apishop.Subscription // keyed by subscription_id
	appleSubTokens     map[string]int64                // token -> subscription_id
	googleSubTokens    map[string]int64                // token -> subscription_id
}

var _ port.SubscriptionRepo = (*MockSubscriptionRepository)(nil)

func NewMockSubscriptionRepository() *MockSubscriptionRepository {
	return &MockSubscriptionRepository{
		subscriptions:   make(map[int64]*apishop.Subscription),
		appleSubTokens:  make(map[string]int64),
		googleSubTokens: make(map[string]int64),
	}
}

func (r *MockSubscriptionRepository) tokenStore(platform string) (map[string]int64, error) {
	switch platform {
	case apishop.PlatformIOS:
		return r.appleSubTokens, nil
	case apishop.PlatformAndroid:
		return r.googleSubTokens, nil
	default:
		return nil, fmt.Errorf("unsupported subscription platform: %q", platform)
	}
}

// CreateSubscription は subscriptions + 対応 token をインメモリに記録する。
func (r *MockSubscriptionRepository) CreateSubscription(_ context.Context, sub *apishop.Subscription, platform, purchaseToken string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	store, err := r.tokenStore(platform)
	if err != nil {
		return err
	}
	r.nextSubscriptionID++
	sub.SubscriptionID = r.nextSubscriptionID
	r.subscriptions[sub.SubscriptionID] = sub
	store[purchaseToken] = sub.SubscriptionID
	return nil
}

// GetLatestSubscription は player の最新サブスクリプションを返す。
func (r *MockSubscriptionRepository) GetLatestSubscription(_ context.Context, playerID string) (*apishop.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *apishop.Subscription
	for _, s := range r.subscriptions {
		if s.PlayerID != playerID {
			continue
		}
		if latest == nil || s.CreatedAt.After(latest.CreatedAt) {
			latest = s
		}
	}
	return latest, nil
}

// FindSubscriptionByToken は platform 別 token store から subscription を引く。
func (r *MockSubscriptionRepository) FindSubscriptionByToken(_ context.Context, platform, purchaseToken string) (*apishop.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	store, err := r.tokenStore(platform)
	if err != nil {
		return nil, err
	}
	subID, ok := store[purchaseToken]
	if !ok {
		return nil, nil
	}
	return r.subscriptions[subID], nil
}

// UpdateSubscription はサブスクリプションをインメモリで更新する。
func (r *MockSubscriptionRepository) UpdateSubscription(_ context.Context, sub *apishop.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.subscriptions[sub.SubscriptionID]; !ok {
		return fmt.Errorf("subscription %d not found", sub.SubscriptionID)
	}
	r.subscriptions[sub.SubscriptionID] = sub
	return nil
}
