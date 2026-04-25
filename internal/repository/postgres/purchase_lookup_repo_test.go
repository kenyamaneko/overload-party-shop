//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
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
		&apishop.OneTimePurchase{PlayerID: user1, ProductID: "faction_she", PurchasedAt: time.Now().UTC()},
		"SHE", apishop.PlatformIOS, "apple-token-1", newFactionEvent())
	require.NoError(t, err)

	tests := []struct {
		name      string
		platform  string
		token     string
		wantNil   bool
		wantErrIs error
	}{
		{
			name:     "存在するtokenは紐付くpurchaseを返す",
			platform: apishop.PlatformIOS,
			token:    "apple-token-1",
		},
		{
			name:     "存在しないtokenは(nil, nil)",
			platform: apishop.PlatformIOS,
			token:    "missing",
			wantNil:  true,
		},
		{
			name:     "別プラットフォームの同文字列tokenは見つからない",
			platform: apishop.PlatformAndroid,
			token:    "apple-token-1",
			wantNil:  true,
		},
		{
			name:      "unsupported platformはErrUnsupportedPlatform",
			platform:  "windows",
			token:     "apple-token-1",
			wantErrIs: port.ErrUnsupportedPlatform,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lookup.FindPurchaseByToken(ctx, tt.platform, tt.token)
			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
				return
			}
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, user1, got.PlayerID)
		})
	}
}
