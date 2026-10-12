package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/domain/domaintest"
)

func TestNewProductView(t *testing.T) {
	t.Run("[商品]商品種別ごとの組み立て", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			cases := []struct {
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
					name:       "商品種別がfaction_setで、card_pack_idと陣営が指定されたとき、渡した商品共通情報とcard_pack_idと陣営を持つfaction_set商品が返る",
					common:     domaintest.NewCommonProduct("faction_set"),
					cardPackID: domaintest.NewPointer("TST-0002"),
					faction:    domaintest.NewPointer("TST-0003"),
					want: domain.FactionSetProduct{
						Product:    domaintest.NewCommonProduct("faction_set"),
						CardPackID: "TST-0002",
						Faction:    "TST-0003",
					},
				},
				{
					name:       "商品種別がcard_packで、card_pack_idが指定されたとき、渡した商品共通情報とcard_pack_idを持つcard_pack商品が返る",
					common:     domaintest.NewCommonProduct("card_pack"),
					cardPackID: domaintest.NewPointer("TST-0002"),
					want: domain.CardPackProduct{
						Product:    domaintest.NewCommonProduct("card_pack"),
						CardPackID: "TST-0002",
					},
				},
				{
					name:     "商品種別がcosmeticで、item_typeとitem_noが指定されたとき、渡した商品共通情報とitem_typeとitem_noを持つcosmetic商品が返る",
					common:   domaintest.NewCommonProduct("cosmetic"),
					itemType: domaintest.NewPointer("TST-ITEM-TYPE"),
					itemNo:   domaintest.NewPointer(int64(7)),
					want: domain.CosmeticProduct{
						Product:  domaintest.NewCommonProduct("cosmetic"),
						ItemType: "TST-ITEM-TYPE",
						ItemNo:   7,
					},
				},
				{
					name:         "商品種別がsubscriptionで、課金周期(月数)が指定されたとき、渡した商品共通情報と課金周期を持つsubscription商品が返る",
					common:       domaintest.NewCommonProduct("subscription"),
					periodMonths: domaintest.NewPointer(int64(3)),
					want: domain.SubscriptionProduct{
						Product:      domaintest.NewCommonProduct("subscription"),
						PeriodMonths: 3,
					},
				},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got, err := domain.NewProductView(tc.common, tc.cardPackID, tc.faction, tc.itemType, tc.itemNo, tc.periodMonths)

					require.NoError(t, err)
					assert.Equal(t, tc.want, got)
				})
			}
		})

		t.Run("異常系", func(t *testing.T) {
			cases := []struct {
				name          string
				common        domain.Product
				cardPackID    *string
				faction       *string
				itemType      *string
				itemNo        *int64
				periodMonths  *int64
				errorCaseName string
				wantError     string
			}{
				{
					name:          "商品種別がfaction_setで、陣営は指定されcard_pack_idが指定されていないとき",
					common:        domaintest.NewCommonProduct("faction_set"),
					faction:       domaintest.NewPointer("TST-0003"),
					errorCaseName: "card_pack_idが足りないことを示すエラーになる",
					wantError:     "card_pack_id missing",
				},
				{
					name:          "商品種別がfaction_setで、card_pack_idは指定され陣営が指定されていないとき",
					common:        domaintest.NewCommonProduct("faction_set"),
					cardPackID:    domaintest.NewPointer("TST-0002"),
					errorCaseName: "陣営が足りないことを示すエラーになる",
					wantError:     "faction missing",
				},
				{
					name:          "商品種別がcard_packで、card_pack_idが指定されていないとき",
					common:        domaintest.NewCommonProduct("card_pack"),
					errorCaseName: "card_pack_idが足りないことを示すエラーになる",
					wantError:     "card_pack_id missing",
				},
				{
					name:          "商品種別がcosmeticで、item_noは指定されitem_typeが指定されていないとき",
					common:        domaintest.NewCommonProduct("cosmetic"),
					itemNo:        domaintest.NewPointer(int64(7)),
					errorCaseName: "item_typeとitem_noが足りないことを示すエラーになる",
					wantError:     "item_type/item_no missing",
				},
				{
					name:          "商品種別がcosmeticで、item_typeは指定されitem_noが指定されていないとき",
					common:        domaintest.NewCommonProduct("cosmetic"),
					itemType:      domaintest.NewPointer("TST-ITEM-TYPE"),
					errorCaseName: "item_typeとitem_noが足りないことを示すエラーになる",
					wantError:     "item_type/item_no missing",
				},
				{
					name:          "商品種別がsubscriptionで、課金周期が指定されていないとき",
					common:        domaintest.NewCommonProduct("subscription"),
					errorCaseName: "課金周期が足りないことを示すエラーになる",
					wantError:     "period_months missing",
				},
				{
					name:          "商品種別がfaction_set・card_pack・cosmetic・subscriptionのいずれでもない値(TST-UNKNOWN)のとき",
					common:        domaintest.NewCommonProduct("TST-UNKNOWN"),
					cardPackID:    domaintest.NewPointer("TST-0002"),
					faction:       domaintest.NewPointer("TST-0003"),
					errorCaseName: "商品種別が未知であることを示すエラーになる",
					wantError:     "unknown type",
				},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Run(tc.errorCaseName, func(t *testing.T) {
						_, err := domain.NewProductView(tc.common, tc.cardPackID, tc.faction, tc.itemType, tc.itemNo, tc.periodMonths)

						assert.ErrorContains(t, err, tc.wantError)
					})
					t.Run("商品は返らない", func(t *testing.T) {
						got, _ := domain.NewProductView(tc.common, tc.cardPackID, tc.faction, tc.itemType, tc.itemNo, tc.periodMonths)

						assert.Nil(t, got)
					})
				})
			}
		})
	})
}
