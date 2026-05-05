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

	tests := []struct {
		name    string
		seeds   []productSeed
		wantIDs []string
	}{
		{
			name: "active商品のみ返りinactiveは除外",
			seeds: []productSeed{
				{"p1", "Active 1", domain.ProductTypeFactionSet, 100, true},
				{"p2", "Inactive", domain.ProductTypeFactionSet, 200, false},
				{"p3", "Active 2", domain.ProductTypeCosmetic, 300, true},
			},
			wantIDs: []string{"p1", "p3"},
		},
		{
			name:    "シードなしなら空",
			seeds:   nil,
			wantIDs: nil,
		},
		{
			name: "全てinactiveなら空",
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
}

func TestProductRepository_GetProductByID(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewProductRepository(sharedPg.Pool)
	ctx := context.Background()

	seedProduct(t, "faction_she", "SHE Pack", domain.ProductTypeFactionSet, 980, true)

	tests := []struct {
		name      string
		productID string
		wantName  string
		wantErrIs error
	}{
		{
			name:      "存在するIDは取得成功",
			productID: "faction_she",
			wantName:  "SHE Pack",
		},
		{
			name:      "存在しないIDはErrNotFound",
			productID: "missing",
			wantErrIs: port.ErrNotFound,
		},
		{
			name:      "空文字IDはErrNotFound",
			productID: "",
			wantErrIs: port.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.GetProductByID(ctx, tt.productID)
			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, got.Common().Name)
		})
	}
}
