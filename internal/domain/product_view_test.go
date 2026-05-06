package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

func ptrString(s string) *string { return &s }
func ptrInt64(n int64) *int64    { return &n }

// 正常系: type discriminator と整合する optional 入力が揃っているとき、対応する per-type 型を返す。
func TestNewProductView_Success(t *testing.T) {
	tests := []struct {
		name         string
		common       domain.Product
		cardPackID   *string
		faction      *string
		itemType     *string
		itemNo       *int64
		periodMonths *int64
		want         domain.ProductView
	}{
		{
			name:       "faction_set は CardPackID と Faction を持つ FactionSetProduct を返す",
			common:     domain.Product{ProductID: "faction_she", Type: domain.ProductTypeFactionSet},
			cardPackID: ptrString("faction_set_she"),
			faction:    ptrString("SHE"),
			want: domain.FactionSetProduct{
				Product:    domain.Product{ProductID: "faction_she", Type: domain.ProductTypeFactionSet},
				CardPackID: "faction_set_she",
				Faction:    "SHE",
			},
		},
		{
			name:       "card_pack は CardPackID を持つ CardPackProduct を返す",
			common:     domain.Product{ProductID: "limited_2026_summer", Type: domain.ProductTypeCardPack},
			cardPackID: ptrString("limited_2026_summer"),
			want: domain.CardPackProduct{
				Product:    domain.Product{ProductID: "limited_2026_summer", Type: domain.ProductTypeCardPack},
				CardPackID: "limited_2026_summer",
			},
		},
		{
			name:     "cosmetic は CosmeticProduct を返す",
			common:   domain.Product{ProductID: "playmat_01", Type: domain.ProductTypeCosmetic},
			itemType: ptrString("playmat"),
			itemNo:   ptrInt64(1),
			want: domain.CosmeticProduct{
				Product:  domain.Product{ProductID: "playmat_01", Type: domain.ProductTypeCosmetic},
				ItemType: "playmat",
				ItemNo:   1,
			},
		},
		{
			name:         "subscription は SubscriptionProduct を返す",
			common:       domain.Product{ProductID: "premium_monthly", Type: domain.ProductTypeSubscription},
			periodMonths: ptrInt64(1),
			want: domain.SubscriptionProduct{
				Product:      domain.Product{ProductID: "premium_monthly", Type: domain.ProductTypeSubscription},
				PeriodMonths: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewProductView(tt.common, tt.cardPackID, tt.faction, tt.itemType, tt.itemNo, tt.periodMonths)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// 異常系: type と入力の不整合 / 未知 type は error を返す。
func TestNewProductView_Error(t *testing.T) {
	tests := []struct {
		name         string
		common       domain.Product
		cardPackID   *string
		faction      *string
		itemType     *string
		itemNo       *int64
		periodMonths *int64
		errSubstr    string
	}{
		{
			name:      "faction_set だが card_pack_id が nil",
			common:    domain.Product{ProductID: "faction_she", Type: domain.ProductTypeFactionSet},
			faction:   ptrString("SHE"),
			errSubstr: "card_pack_id missing",
		},
		{
			name:       "faction_set だが faction が nil",
			common:     domain.Product{ProductID: "faction_she", Type: domain.ProductTypeFactionSet},
			cardPackID: ptrString("faction_set_she"),
			errSubstr:  "faction missing",
		},
		{
			name:      "card_pack だが card_pack_id が nil",
			common:    domain.Product{ProductID: "limited_2026_summer", Type: domain.ProductTypeCardPack},
			errSubstr: "card_pack_id missing",
		},
		{
			name:      "cosmetic だが item_type が nil",
			common:    domain.Product{ProductID: "playmat_01", Type: domain.ProductTypeCosmetic},
			itemNo:    ptrInt64(1),
			errSubstr: "item_type/item_no missing",
		},
		{
			name:      "cosmetic だが item_no が nil",
			common:    domain.Product{ProductID: "playmat_01", Type: domain.ProductTypeCosmetic},
			itemType:  ptrString("playmat"),
			errSubstr: "item_type/item_no missing",
		},
		{
			name:      "subscription だが period_months が nil",
			common:    domain.Product{ProductID: "premium_monthly", Type: domain.ProductTypeSubscription},
			errSubstr: "period_months missing",
		},
		{
			name:      "未知の type",
			common:    domain.Product{ProductID: "p1", Type: "totally_unknown_type"},
			errSubstr: "unknown type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewProductView(tt.common, tt.cardPackID, tt.faction, tt.itemType, tt.itemNo, tt.periodMonths)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errSubstr)
			assert.Nil(t, got)
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
			view: domain.NewFactionSetProduct(common, "faction_set_she", "SHE"),
		},
		{
			name: "CardPackProduct",
			view: domain.NewCardPackProduct(common, "limited_2026_summer"),
		},
		{
			name: "CosmeticProduct",
			view: domain.NewCosmeticProduct(common, "playmat", 1),
		},
		{
			name: "SubscriptionProduct",
			view: domain.NewSubscriptionProduct(common, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, common, tt.view.Common())
		})
	}
}
