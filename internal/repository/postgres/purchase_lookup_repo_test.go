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
	lookup := postgres.NewPurchaseLookupRepository(sharedPg.Pool)
	factionPurchase := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("token による purchase 検索", func(t *testing.T) {
		const user1 = "11111111-4444-4444-4444-111111111111"

		sharedPg.Truncate(t)
		_, err := factionPurchase.CreatePurchase(ctx,
			&domain.OneTimePurchase{PlayerID: user1, ProductID: "faction_she", PurchasedAt: time.Now().UTC()},
			"SHE", "faction_set_she", domain.PlatformIOS, "apple-token-1", newFactionPurchaseEvents())
		require.NoError(t, err)

		t.Run("存在する token のとき、紐付く purchase を返す", func(t *testing.T) {
			got, err := lookup.FindPurchaseByToken(ctx, domain.PlatformIOS, "apple-token-1")
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, user1, got.PlayerID)
		})

		notFoundCases := []struct {
			name     string
			platform string
			token    string
		}{
			{
				name:     "存在しない token のとき、(nil, nil) を返す",
				platform: domain.PlatformIOS,
				token:    "missing",
			},
			{
				name:     "別プラットフォームで同一文字列の token のとき、見つからず (nil, nil) を返す",
				platform: domain.PlatformAndroid,
				token:    "apple-token-1",
			},
		}

		for _, tt := range notFoundCases {
			t.Run(tt.name, func(t *testing.T) {
				got, err := lookup.FindPurchaseByToken(ctx, tt.platform, tt.token)
				require.NoError(t, err)
				assert.Nil(t, got)
			})
		}

		t.Run("unsupported platform のとき、ErrUnsupportedPlatform になる", func(t *testing.T) {
			_, err := lookup.FindPurchaseByToken(ctx, "windows", "apple-token-1")
			assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
		})
	})
}
