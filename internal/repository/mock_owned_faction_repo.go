package repository

import (
	"context"
	"sync"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.OwnedFactionRepo = (*MockOwnedFactionRepository)(nil)

// MockOwnedFactionRepository はテスト用のインメモリ OwnedFactionRepo 実装。
type MockOwnedFactionRepository struct {
	mu   sync.Mutex
	data map[string]map[string]struct{}
}

// NewMockOwnedFactionRepository はテスト用の MockOwnedFactionRepository を構築する。
func NewMockOwnedFactionRepository() *MockOwnedFactionRepository {
	return &MockOwnedFactionRepository{data: make(map[string]map[string]struct{})}
}

// Add はプレイヤーの faction 所有をインメモリに記録する。
func (r *MockOwnedFactionRepository) Add(_ context.Context, playerID, faction string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[playerID]; !ok {
		r.data[playerID] = make(map[string]struct{})
	}
	r.data[playerID][faction] = struct{}{}
	return nil
}

// List はプレイヤーが所有する faction 一覧をインメモリから返す。
func (r *MockOwnedFactionRepository) List(_ context.Context, playerID string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := r.data[playerID]
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	return out, nil
}
