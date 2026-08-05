//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
)

func TestProductRepository_GetActiveProducts(t *testing.T) {
	repo := postgres.NewProductRepository(sharedPg.Pool)
	ctx := context.Background()

	type productSeed struct {
		id, name, ptype string
		price           int64
		active          bool
	}

	t.Run("有効な商品一覧の取得", func(t *testing.T) {
		tests := []struct {
			name    string
			seeds   []productSeed
			wantIDs []string
		}{
			{
				name: "activeとinactiveが混在するとき、activeのみ返す",
				seeds: []productSeed{
					{"p1", "Active 1", domain.ProductTypeFactionSet, 100, true},
					{"p2", "Inactive", domain.ProductTypeFactionSet, 200, false},
					{"p3", "Active 2", domain.ProductTypeCosmetic, 300, true},
				},
				wantIDs: []string{"p1", "p3"},
			},
			{
				name:    "商品が無いとき、空を返す",
				seeds:   nil,
				wantIDs: nil,
			},
			{
				name: "全てinactiveのとき、空を返す",
				seeds: []productSeed{
					{"p1", "Inactive 1", domain.ProductTypeFactionSet, 100, false},
					{"p2", "Inactive 2", domain.ProductTypeCosmetic, 200, false},
				},
				wantIDs: nil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				for _, s := range tt.seeds {
					seedProduct(t, s.id, s.name, s.ptype, s.price, s.active)
				}

				got, err := repo.GetActiveProducts(ctx)
				require.NoError(t, err)

				gotIDs := make([]string, len(got))
				for i, p := range got {
					gotIDs[i] = p.Common().ProductID
				}
				assert.ElementsMatch(t, tt.wantIDs, gotIDs)
			})
		}

		t.Run("4種別の商品がactiveのとき、それぞれtype固有の属性を持つ商品として返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedProduct(t, "p_faction", "Faction Set", domain.ProductTypeFactionSet, 100, true)
			seedProduct(t, "p_card_pack", "Card Pack", domain.ProductTypeCardPack, 200, true)
			seedProduct(t, "p_cosmetic", "Cosmetic", domain.ProductTypeCosmetic, 300, true)
			seedProduct(t, "p_subscription", "Subscription", domain.ProductTypeSubscription, 400, true)

			got, err := repo.GetActiveProducts(ctx)
			require.NoError(t, err)

			byID := make(map[string]domain.ProductView, len(got))
			for _, p := range got {
				byID[p.Common().ProductID] = p
			}
			require.Len(t, byID, 4)

			factionSet, ok := byID["p_faction"].(domain.FactionSetProduct)
			require.True(t, ok, "faction_set 行は domain.FactionSetProduct になる")
			assert.Equal(t, "SHE", factionSet.Faction)
			assert.Equal(t, "faction_set_SHE", factionSet.CardPackID)

			cardPack, ok := byID["p_card_pack"].(domain.CardPackProduct)
			require.True(t, ok, "card_pack 行は domain.CardPackProduct になる")
			assert.Equal(t, "default_pack_p_card_pack", cardPack.CardPackID)

			cosmetic, ok := byID["p_cosmetic"].(domain.CosmeticProduct)
			require.True(t, ok, "cosmetic 行は domain.CosmeticProduct になる")
			assert.Equal(t, "stamp", cosmetic.ItemType)
			assert.Equal(t, int64(1), cosmetic.ItemNo)

			subscription, ok := byID["p_subscription"].(domain.SubscriptionProduct)
			require.True(t, ok, "subscription 行は domain.SubscriptionProduct になる")
			assert.Equal(t, int64(1), subscription.PeriodMonths)
		})
	})
}

func TestProductRepository_GetProductByID(t *testing.T) {
	repo := postgres.NewProductRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("product_idによる商品取得", func(t *testing.T) {
		sharedPg.Truncate(t)
		seedProduct(t, "faction_she", "SHE Pack", domain.ProductTypeFactionSet, 980, true)

		t.Run("存在するproduct_idのとき、その商品を返す", func(t *testing.T) {
			got, err := repo.GetProductByID(ctx, "faction_she")
			require.NoError(t, err)
			assert.Equal(t, "SHE Pack", got.Common().Name)
		})

		notFoundCases := []struct {
			name      string
			productID string
		}{
			{
				name:      "存在しないproduct_idのとき、ErrNotFoundになる",
				productID: "missing",
			},
			{
				name:      "空文字のproduct_idのとき、ErrNotFoundになる",
				productID: "",
			},
		}

		for _, tt := range notFoundCases {
			t.Run(tt.name, func(t *testing.T) {
				_, err := repo.GetProductByID(ctx, tt.productID)
				assert.ErrorIs(t, err, port.ErrNotFound)
			})
		}
	})
}
