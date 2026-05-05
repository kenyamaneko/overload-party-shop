package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

func ptrString(s string) *string { return &s }
func ptrInt64(n int64) *int64    { return &n }

func TestNewProductView(t *testing.T) {
	common := func(productID, productType string) domain.Product {
		return domain.Product{ProductID: productID, Type: productType}
	}

	tests := []struct {
		name      string
		common    domain.Product
		faction   *string
		itemType  *string
		itemNo    *int64
		want      domain.ProductView
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "faction_set: faction が揃っていれば FactionSetProduct を返す",
			common:  common("faction_she", domain.ProductTypeFactionSet),
			faction: ptrString("SHE"),
			want:    domain.FactionSetProduct{Product: common("faction_she", domain.ProductTypeFactionSet), Faction: "SHE"},
		},
		{
			name:      "faction_set: faction が nil なら error",
			common:    common("faction_she", domain.ProductTypeFactionSet),
			faction:   nil,
			wantErr:   true,
			errSubstr: "faction missing",
		},
		{
			name:     "cosmetic: item_type / item_no が揃っていれば CosmeticProduct を返す",
			common:   common("playmat_01", domain.ProductTypeCosmetic),
			itemType: ptrString("playmat"),
			itemNo:   ptrInt64(1),
			want:     domain.CosmeticProduct{Product: common("playmat_01", domain.ProductTypeCosmetic), ItemType: "playmat", ItemNo: 1},
		},
		{
			name:      "cosmetic: item_type が nil なら error",
			common:    common("playmat_01", domain.ProductTypeCosmetic),
			itemType:  nil,
			itemNo:    ptrInt64(1),
			wantErr:   true,
			errSubstr: "item_type/item_no missing",
		},
		{
			name:      "cosmetic: item_no が nil なら error",
			common:    common("playmat_01", domain.ProductTypeCosmetic),
			itemType:  ptrString("playmat"),
			itemNo:    nil,
			wantErr:   true,
			errSubstr: "item_type/item_no missing",
		},
		{
			name:   "subscription: type 固有属性なしで SubscriptionProduct を返す",
			common: common("premium_monthly", domain.ProductTypeSubscription),
			want:   domain.SubscriptionProduct{Product: common("premium_monthly", domain.ProductTypeSubscription)},
		},
		{
			name:      "未知の type は error",
			common:    common("p1", "totally_unknown_type"),
			wantErr:   true,
			errSubstr: "unknown type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewProductView(tt.common, tt.faction, tt.itemType, tt.itemNo)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// 個別 constructor は trivial だが ProductView interface を満たすことを最低限保証する。
func TestPerTypeConstructors_ImplementProductView(t *testing.T) {
	common := domain.Product{ProductID: "p1"}

	tests := []struct {
		name string
		view domain.ProductView
	}{
		{
			name: "FactionSetProduct",
			view: domain.NewFactionSetProduct(common, "SHE"),
		},
		{
			name: "CosmeticProduct",
			view: domain.NewCosmeticProduct(common, "playmat", 1),
		},
		{
			name: "SubscriptionProduct",
			view: domain.NewSubscriptionProduct(common),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, common, tt.view.Common())
		})
	}
}
