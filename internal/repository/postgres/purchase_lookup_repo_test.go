//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindPurchaseByToken(t *testing.T) {
	ctx := context.Background()

	t.Run("[購入検索]購入トークンによる購入の取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("保存済みの購入トークンで検索する", func(t *testing.T) {
				t.Run("購入者は、保存した購入者になる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerA, got.PlayerID)
				})

				t.Run("商品は、保存した商品になる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, "TST-PRODUCT-1", got.ProductID)
				})

				t.Run("購入日時は、保存した購入日時になる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.True(t, got.PurchasedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)), "got %s", got.PurchasedAt)
				})
			})

			t.Run("保存されていない購入トークンで検索すると、見つからず、エラーにもならない", func(t *testing.T) {
				pg.Truncate(t)
				require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

				got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenTwo)

				assert.NoError(t, err)
				assert.Nil(t, got)
			})
		})
	})
}
