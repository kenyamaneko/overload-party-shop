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
	t.Run("ProductView 構築", func(t *testing.T) {
		validCases := []struct {
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
				name:       "faction_set の type で card_pack_id と faction が両方指定されるとき、CardPackID と Faction を持つ FactionSetProduct が返る",
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
				name:       "card_pack の type で card_pack_id が指定されるとき、CardPackID を持つ CardPackProduct が返る",
				common:     domain.Product{ProductID: "limited_2026_summer", Type: domain.ProductTypeCardPack},
				cardPackID: ptrString("limited_2026_summer"),
				want: domain.CardPackProduct{
					Product:    domain.Product{ProductID: "limited_2026_summer", Type: domain.ProductTypeCardPack},
					CardPackID: "limited_2026_summer",
				},
			},
			{
				name:     "cosmetic の type で item_type と item_no が指定されるとき、CosmeticProduct が返る",
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
				name:         "subscription の type で period_months が指定されるとき、SubscriptionProduct が返る",
				common:       domain.Product{ProductID: "premium_monthly", Type: domain.ProductTypeSubscription},
				periodMonths: ptrInt64(1),
				want: domain.SubscriptionProduct{
					Product:      domain.Product{ProductID: "premium_monthly", Type: domain.ProductTypeSubscription},
					PeriodMonths: 1,
				},
			},
		}
		for _, tc := range validCases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := domain.NewProductView(tc.common, tc.cardPackID, tc.faction, tc.itemType, tc.itemNo, tc.periodMonths)
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			})
		}

		invalidCases := []struct {
			name         string
			common       domain.Product
			cardPackID   *string
			faction      *string
			itemType     *string
			itemNo       *int64
			periodMonths *int64
			wantErr      string
		}{
			{
				name:    "faction_set の type で card_pack_id が nil のとき、エラーになる",
				common:  domain.Product{ProductID: "faction_she", Type: domain.ProductTypeFactionSet},
				faction: ptrString("SHE"),
				wantErr: "card_pack_id missing",
			},
			{
				name:       "faction_set の type で faction が nil のとき、エラーになる",
				common:     domain.Product{ProductID: "faction_she", Type: domain.ProductTypeFactionSet},
				cardPackID: ptrString("faction_set_she"),
				wantErr:    "faction missing",
			},
			{
				name:    "card_pack の type で card_pack_id が nil のとき、エラーになる",
				common:  domain.Product{ProductID: "limited_2026_summer", Type: domain.ProductTypeCardPack},
				wantErr: "card_pack_id missing",
			},
			{
				name:    "cosmetic の type で item_type が nil のとき、エラーになる",
				common:  domain.Product{ProductID: "playmat_01", Type: domain.ProductTypeCosmetic},
				itemNo:  ptrInt64(1),
				wantErr: "item_type/item_no missing",
			},
			{
				name:     "cosmetic の type で item_no が nil のとき、エラーになる",
				common:   domain.Product{ProductID: "playmat_01", Type: domain.ProductTypeCosmetic},
				itemType: ptrString("playmat"),
				wantErr:  "item_type/item_no missing",
			},
			{
				name:    "subscription の type で period_months が nil のとき、エラーになる",
				common:  domain.Product{ProductID: "premium_monthly", Type: domain.ProductTypeSubscription},
				wantErr: "period_months missing",
			},
			{
				name:    "type が未知の値 (totally_unknown_type) のとき、エラーになる",
				common:  domain.Product{ProductID: "p1", Type: "totally_unknown_type"},
				wantErr: "unknown type",
			},
		}
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := domain.NewProductView(tc.common, tc.cardPackID, tc.faction, tc.itemType, tc.itemNo, tc.periodMonths)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Nil(t, got)
			})
		}
	})
}
