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
				name: "active と inactive が混在するとき、active のみ返す",
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
				name: "全て inactive のとき、空を返す",
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
	})
}

func TestProductRepository_GetProductByID(t *testing.T) {
	repo := postgres.NewProductRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("product_id による商品取得", func(t *testing.T) {
		sharedPg.Truncate(t)
		seedProduct(t, "faction_she", "SHE Pack", domain.ProductTypeFactionSet, 980, true)

		t.Run("存在する product_id のとき、その商品を返す", func(t *testing.T) {
			got, err := repo.GetProductByID(ctx, "faction_she")
			require.NoError(t, err)
			assert.Equal(t, "SHE Pack", got.Common().Name)
		})

		notFoundCases := []struct {
			name      string
			productID string
		}{
			{
				name:      "存在しない product_id のとき、ErrNotFound になる",
				productID: "missing",
			},
			{
				name:      "空文字の product_id のとき、ErrNotFound になる",
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
