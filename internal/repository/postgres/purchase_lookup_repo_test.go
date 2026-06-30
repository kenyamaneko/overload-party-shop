//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
)

func TestPurchaseLookupRepository_FindPurchaseByToken(t *testing.T) {
	sharedPg.Truncate(t)
	lookup := postgres.NewPurchaseLookupRepository(sharedPg.Pool)
	factionPurchase := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const user1 = "11111111-4444-4444-4444-111111111111"

	_, err := factionPurchase.CreatePurchase(ctx,
		&domain.OneTimePurchase{PlayerID: user1, ProductID: "faction_she", PurchasedAt: time.Now().UTC()},
		"SHE", "faction_set_she", domain.PlatformIOS, "apple-token-1", newFactionPurchaseEvents())
	require.NoError(t, err)

	tests := []struct {
		name     string
		platform string
		token    string
		check    func(t *testing.T, got *domain.OneTimePurchase, err error)
	}{
		{
			name:     "存在するtokenは紐付くpurchaseを返す",
			platform: domain.PlatformIOS,
			token:    "apple-token-1",
			check: func(t *testing.T, got *domain.OneTimePurchase, err error) {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, user1, got.PlayerID)
			},
		},
		{
			name:     "存在しないtokenは(nil, nil)",
			platform: domain.PlatformIOS,
			token:    "missing",
			check: func(t *testing.T, got *domain.OneTimePurchase, err error) {
				require.NoError(t, err)
				assert.Nil(t, got)
			},
		},
		{
			name:     "別プラットフォームの同文字列tokenは見つからない",
			platform: domain.PlatformAndroid,
			token:    "apple-token-1",
			check: func(t *testing.T, got *domain.OneTimePurchase, err error) {
				require.NoError(t, err)
				assert.Nil(t, got)
			},
		},
		{
			name:     "unsupported platformはErrUnsupportedPlatform",
			platform: "windows",
			token:    "apple-token-1",
			check: func(t *testing.T, got *domain.OneTimePurchase, err error) {
				assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lookup.FindPurchaseByToken(ctx, tt.platform, tt.token)
			tt.check(t, got, err)
		})
	}
}
