package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/domain/domaintest"
)

func requireProductViewType[T domain.ProductView](t *testing.T, view domain.ProductView) T {
	t.Helper()
	typed, ok := view.(T)
	require.True(t, ok, "想定と異なる商品の型: %T", view)
	return typed
}

func TestNewProductView(t *testing.T) {
	t.Run("[商品]商品種別ごとの組み立て", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("商品種別がfaction_setで、card_pack_idと陣営が指定されたとき", func(t *testing.T) {
				newView := func(t *testing.T) domain.ProductView {
					t.Helper()
					got, err := domain.NewProductView(
						domaintest.NewCommonProduct("faction_set"),
						domaintest.NewPointer("TST-0002"),
						domaintest.NewPointer("TST-0003"),
						nil, nil, nil,
					)
					require.NoError(t, err)
					return got
				}

				t.Run("返った商品はfaction_set商品になる", func(t *testing.T) {
					got := newView(t)

					assert.IsType(t, domain.FactionSetProduct{}, got)
				})

				t.Run("返った商品のcard_pack_idは指定した値になる", func(t *testing.T) {
					got := newView(t)

					assert.Equal(t, "TST-0002", requireProductViewType[domain.FactionSetProduct](t, got).CardPackID)
				})

				t.Run("返った商品の陣営は指定した値になる", func(t *testing.T) {
					got := newView(t)

					assert.Equal(t, "TST-0003", requireProductViewType[domain.FactionSetProduct](t, got).Faction)
				})
			})

			t.Run("商品種別がcard_packで、card_pack_idが指定されたとき", func(t *testing.T) {
				newView := func(t *testing.T) domain.ProductView {
					t.Helper()
					got, err := domain.NewProductView(
						domaintest.NewCommonProduct("card_pack"),
						domaintest.NewPointer("TST-0002"),
						nil, nil, nil, nil,
					)
					require.NoError(t, err)
					return got
				}

				t.Run("返った商品はcard_pack商品になる", func(t *testing.T) {
					got := newView(t)

					assert.IsType(t, domain.CardPackProduct{}, got)
				})

				t.Run("返った商品のcard_pack_idは指定した値になる", func(t *testing.T) {
					got := newView(t)

					assert.Equal(t, "TST-0002", requireProductViewType[domain.CardPackProduct](t, got).CardPackID)
				})
			})

			t.Run("商品種別がcosmeticで、item_typeとitem_noが指定されたとき", func(t *testing.T) {
				newView := func(t *testing.T) domain.ProductView {
					t.Helper()
					got, err := domain.NewProductView(
						domaintest.NewCommonProduct("cosmetic"),
						nil, nil,
						domaintest.NewPointer("TST-ITEM-TYPE"),
						domaintest.NewPointer(int64(7)),
						nil,
					)
					require.NoError(t, err)
					return got
				}

				t.Run("返った商品はcosmetic商品になる", func(t *testing.T) {
					got := newView(t)

					assert.IsType(t, domain.CosmeticProduct{}, got)
				})

				t.Run("返った商品のitem_typeは指定した値になる", func(t *testing.T) {
					got := newView(t)

					assert.Equal(t, "TST-ITEM-TYPE", requireProductViewType[domain.CosmeticProduct](t, got).ItemType)
				})

				t.Run("返った商品のitem_noは指定した値になる", func(t *testing.T) {
					got := newView(t)

					assert.Equal(t, int64(7), requireProductViewType[domain.CosmeticProduct](t, got).ItemNo)
				})
			})

			t.Run("商品種別がsubscriptionで、課金周期(月数)が指定されたとき", func(t *testing.T) {
				newView := func(t *testing.T) domain.ProductView {
					t.Helper()
					got, err := domain.NewProductView(
						domaintest.NewCommonProduct("subscription"),
						nil, nil, nil, nil,
						domaintest.NewPointer(int64(3)),
					)
					require.NoError(t, err)
					return got
				}

				t.Run("返った商品はsubscription商品になる", func(t *testing.T) {
					got := newView(t)

					assert.IsType(t, domain.SubscriptionProduct{}, got)
				})

				t.Run("返った商品の課金周期は指定した値になる", func(t *testing.T) {
					got := newView(t)

					assert.Equal(t, int64(3), requireProductViewType[domain.SubscriptionProduct](t, got).PeriodMonths)
				})
			})

			commonRows := []struct {
				name        string
				productType string
				cardPack    *string
				faction     *string
				itemType    *string
				itemNo      *int64
				period      *int64
				commonOf    func(*testing.T, domain.ProductView) domain.Product
			}{
				{
					name:        "商品種別がfaction_setで、card_pack_idと陣営が指定された",
					productType: "faction_set",
					cardPack:    domaintest.NewPointer("TST-0002"),
					faction:     domaintest.NewPointer("TST-0003"),
					commonOf: func(t *testing.T, view domain.ProductView) domain.Product {
						return requireProductViewType[domain.FactionSetProduct](t, view).Product
					},
				},
				{
					name:        "商品種別がcard_packで、card_pack_idが指定された",
					productType: "card_pack",
					cardPack:    domaintest.NewPointer("TST-0002"),
					commonOf: func(t *testing.T, view domain.ProductView) domain.Product {
						return requireProductViewType[domain.CardPackProduct](t, view).Product
					},
				},
				{
					name:        "商品種別がcosmeticで、item_typeとitem_noが指定された",
					productType: "cosmetic",
					itemType:    domaintest.NewPointer("TST-ITEM-TYPE"),
					itemNo:      domaintest.NewPointer(int64(7)),
					commonOf: func(t *testing.T, view domain.ProductView) domain.Product {
						return requireProductViewType[domain.CosmeticProduct](t, view).Product
					},
				},
				{
					name:        "商品種別がsubscriptionで、課金周期(月数)が指定された",
					productType: "subscription",
					period:      domaintest.NewPointer(int64(3)),
					commonOf: func(t *testing.T, view domain.ProductView) domain.Product {
						return requireProductViewType[domain.SubscriptionProduct](t, view).Product
					},
				},
			}
			commonItems := []struct {
				name       string
				assertSame func(t *testing.T, want, got domain.Product)
			}{
				{
					name: "返った商品の商品IDは渡した商品共通情報の商品IDと同じになる",
					assertSame: func(t *testing.T, want, got domain.Product) {
						assert.Equal(t, want.ProductID, got.ProductID)
					},
				},
				{
					name: "返った商品の名称は渡した商品共通情報の名称と同じになる",
					assertSame: func(t *testing.T, want, got domain.Product) {
						assert.Equal(t, want.Name, got.Name)
					},
				},
				{
					name: "返った商品の価格は渡した商品共通情報の価格と同じになる",
					assertSame: func(t *testing.T, want, got domain.Product) {
						assert.Equal(t, want.Price, got.Price)
					},
				},
				{
					name: "返った商品の説明は渡した商品共通情報の説明と同じになる",
					assertSame: func(t *testing.T, want, got domain.Product) {
						assert.Equal(t, want.Description, got.Description)
					},
				},
				{
					name: "返った商品の画像URLは渡した商品共通情報の画像URLと同じになる",
					assertSame: func(t *testing.T, want, got domain.Product) {
						assert.Equal(t, want.ImageURL, got.ImageURL)
					},
				},
				{
					name: "返った商品の販売中かどうかは渡した商品共通情報の販売中かどうかと同じになる",
					assertSame: func(t *testing.T, want, got domain.Product) {
						assert.Equal(t, want.IsActive, got.IsActive)
					},
				},
			}
			for _, item := range commonItems {
				for _, row := range commonRows {
					t.Run(row.name+"とき、"+item.name, func(t *testing.T) {
						common := domaintest.NewCommonProduct(row.productType)

						got, err := domain.NewProductView(common, row.cardPack, row.faction, row.itemType, row.itemNo, row.period)

						require.NoError(t, err)
						item.assertSame(t, common, row.commonOf(t, got))
					})
				}
			}
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("商品種別がfaction_setのとき", func(t *testing.T) {
				t.Run("陣営は指定されcard_pack_idが指定されていないとき", func(t *testing.T) {
					build := func() (domain.ProductView, error) {
						return domain.NewProductView(
							domaintest.NewCommonProduct("faction_set"),
							nil,
							domaintest.NewPointer("TST-0003"),
							nil, nil, nil,
						)
					}

					t.Run("card_pack_idが足りないことを示すエラーになる", func(t *testing.T) {
						_, err := build()

						assert.ErrorContains(t, err, "card_pack_id missing")
					})

					t.Run("商品は返らない", func(t *testing.T) {
						got, _ := build()

						assert.Nil(t, got)
					})
				})

				t.Run("card_pack_idは指定され陣営が指定されていないとき", func(t *testing.T) {
					build := func() (domain.ProductView, error) {
						return domain.NewProductView(
							domaintest.NewCommonProduct("faction_set"),
							domaintest.NewPointer("TST-0002"),
							nil, nil, nil, nil,
						)
					}

					t.Run("陣営が足りないことを示すエラーになる", func(t *testing.T) {
						_, err := build()

						assert.ErrorContains(t, err, "faction missing")
					})

					t.Run("商品は返らない", func(t *testing.T) {
						got, _ := build()

						assert.Nil(t, got)
					})
				})
			})

			t.Run("商品種別がcard_packで、card_pack_idが指定されていないとき", func(t *testing.T) {
				build := func() (domain.ProductView, error) {
					return domain.NewProductView(domaintest.NewCommonProduct("card_pack"), nil, nil, nil, nil, nil)
				}

				t.Run("card_pack_idが足りないことを示すエラーになる", func(t *testing.T) {
					_, err := build()

					assert.ErrorContains(t, err, "card_pack_id missing")
				})

				t.Run("商品は返らない", func(t *testing.T) {
					got, _ := build()

					assert.Nil(t, got)
				})
			})

			t.Run("商品種別がcosmeticのとき", func(t *testing.T) {
				cases := []struct {
					suffix   string
					itemType *string
					itemNo   *int64
				}{
					{
						suffix:   "item_noは指定されitem_typeが指定されていないとき",
						itemNo:   domaintest.NewPointer(int64(7)),
						itemType: nil,
					},
					{
						suffix:   "item_typeは指定されitem_noが指定されていないとき",
						itemType: domaintest.NewPointer("TST-ITEM-TYPE"),
						itemNo:   nil,
					},
				}
				for _, tc := range cases {
					t.Run(tc.suffix+"、item_typeとitem_noが足りないことを示すエラーになる", func(t *testing.T) {
						_, err := domain.NewProductView(domaintest.NewCommonProduct("cosmetic"), nil, nil, tc.itemType, tc.itemNo, nil)

						assert.ErrorContains(t, err, "item_type/item_no missing")
					})
				}
				for _, tc := range cases {
					t.Run(tc.suffix+"、商品は返らない", func(t *testing.T) {
						got, _ := domain.NewProductView(domaintest.NewCommonProduct("cosmetic"), nil, nil, tc.itemType, tc.itemNo, nil)

						assert.Nil(t, got)
					})
				}
			})

			t.Run("商品種別がsubscriptionで、課金周期が指定されていないとき", func(t *testing.T) {
				build := func() (domain.ProductView, error) {
					return domain.NewProductView(domaintest.NewCommonProduct("subscription"), nil, nil, nil, nil, nil)
				}

				t.Run("課金周期が足りないことを示すエラーになる", func(t *testing.T) {
					_, err := build()

					assert.ErrorContains(t, err, "period_months missing")
				})

				t.Run("商品は返らない", func(t *testing.T) {
					got, _ := build()

					assert.Nil(t, got)
				})
			})

			t.Run("商品種別がfaction_set・card_pack・cosmetic・subscriptionのいずれでもない値(TST-UNKNOWN)のとき", func(t *testing.T) {
				build := func() (domain.ProductView, error) {
					return domain.NewProductView(
						domaintest.NewCommonProduct("TST-UNKNOWN"),
						domaintest.NewPointer("TST-0002"),
						domaintest.NewPointer("TST-0003"),
						nil, nil, nil,
					)
				}

				t.Run("商品種別が未知であることを示すエラーになる", func(t *testing.T) {
					_, err := build()

					assert.ErrorContains(t, err, "unknown type")
				})

				t.Run("商品は返らない", func(t *testing.T) {
					got, _ := build()

					assert.Nil(t, got)
				})
			})
		})
	})
}
