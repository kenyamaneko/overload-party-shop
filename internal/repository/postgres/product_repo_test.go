//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

func stringPointer(value string) *string {
	return &value
}

func activeProductSeed(productID string) postgrestest.ProductSeed {
	return postgrestest.ProductSeed{
		ProductID:   productID,
		Name:        "TST-NAME",
		Price:       100,
		Description: stringPointer("TST-DESCRIPTION"),
		ImageURL:    stringPointer("TST-IMAGE-URL"),
		IsActive:    true,
	}
}

func inactiveProductSeed(productID string) postgrestest.ProductSeed {
	seed := activeProductSeed(productID)
	seed.IsActive = false
	return seed
}

func listActiveProductIDs(t *testing.T) []string {
	t.Helper()
	products, err := pg.ProductRepository().GetActiveProducts(context.Background())
	require.NoError(t, err)
	productIDs := make([]string, 0, len(products))
	for _, product := range products {
		productIDs = append(productIDs, product.Common().ProductID)
	}
	return productIDs
}

func insertFactionSetProductWithoutFaction(t *testing.T, seed postgrestest.ProductSeed) {
	t.Helper()
	pg.InsertProduct(t, seed, "faction_set")
	pg.InsertProductCardPack(t, seed.ProductID, "TST-PACK-X")
}

func TestProductRepositoryGetActiveProducts(t *testing.T) {
	ctx := context.Background()

	t.Run("[商品]販売中の商品の一覧取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("販売中の商品の絞り込み", func(t *testing.T) {
				t.Run("販売中と販売停止の商品が混在するとき、販売中の商品だけが返る", func(t *testing.T) {
					pg.Truncate(t)
					pg.InsertCardPackProduct(t, activeProductSeed("TST-PRODUCT-ACTIVE"), "TST-PACK-X")
					pg.InsertCardPackProduct(t, inactiveProductSeed("TST-PRODUCT-INACTIVE"), "TST-PACK-Y")

					assert.Equal(t, []string{"TST-PRODUCT-ACTIVE"}, listActiveProductIDs(t))
				})

				t.Run("販売中の商品が1件だけのとき、その1件が返る", func(t *testing.T) {
					pg.Truncate(t)
					pg.InsertCardPackProduct(t, activeProductSeed("TST-PRODUCT-ACTIVE"), "TST-PACK-X")

					assert.Equal(t, []string{"TST-PRODUCT-ACTIVE"}, listActiveProductIDs(t))
				})

				t.Run("販売中の商品が無い", func(t *testing.T) {
					cases := []struct {
						name    string
						prepare func(t *testing.T)
					}{
						{name: "商品が1件も無いとき、0件になる", prepare: func(t *testing.T) {}},
						{name: "すべて販売停止のとき、0件になる", prepare: func(t *testing.T) {
							pg.InsertCardPackProduct(t, inactiveProductSeed("TST-PRODUCT-INACTIVE-1"), "TST-PACK-X")
							pg.InsertCardPackProduct(t, inactiveProductSeed("TST-PRODUCT-INACTIVE-2"), "TST-PACK-Y")
						}},
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							pg.Truncate(t)
							tc.prepare(t)

							assert.Len(t, listActiveProductIDs(t), 0)
						})
					}
				})
			})

			t.Run("種別がfaction_setの商品が販売中のとき", func(t *testing.T) {
				prepare := func(t *testing.T) []domain.ProductView {
					pg.Truncate(t)
					pg.InsertFactionSetProduct(t, activeProductSeed("TST-PRODUCT-FACTION"), "Tenki", "TST-PACK-TENKI")
					products, err := pg.ProductRepository().GetActiveProducts(ctx)
					require.NoError(t, err)
					require.Len(t, products, 1)
					return products
				}

				t.Run("陣営セットの商品として返る", func(t *testing.T) {
					products := prepare(t)

					assert.IsType(t, domain.FactionSetProduct{}, products[0])
				})

				t.Run("陣営は、保存した陣営になる", func(t *testing.T) {
					products := prepare(t)

					got, ok := products[0].(domain.FactionSetProduct)
					require.True(t, ok)
					assert.Equal(t, "Tenki", got.Faction)
				})

				t.Run("カードパックIDは、保存したカードパックIDになる", func(t *testing.T) {
					products := prepare(t)

					got, ok := products[0].(domain.FactionSetProduct)
					require.True(t, ok)
					assert.Equal(t, "TST-PACK-TENKI", got.CardPackID)
				})
			})

			t.Run("種別がcard_packの商品が販売中のとき", func(t *testing.T) {
				prepare := func(t *testing.T) []domain.ProductView {
					pg.Truncate(t)
					pg.InsertCardPackProduct(t, activeProductSeed("TST-PRODUCT-PACK"), "TST-PACK-X")
					products, err := pg.ProductRepository().GetActiveProducts(ctx)
					require.NoError(t, err)
					require.Len(t, products, 1)
					return products
				}

				t.Run("カードパックの商品として返る", func(t *testing.T) {
					products := prepare(t)

					assert.IsType(t, domain.CardPackProduct{}, products[0])
				})

				t.Run("カードパックIDは、保存したカードパックIDになる", func(t *testing.T) {
					products := prepare(t)

					got, ok := products[0].(domain.CardPackProduct)
					require.True(t, ok)
					assert.Equal(t, "TST-PACK-X", got.CardPackID)
				})
			})

			t.Run("種別がcosmeticの商品が販売中のとき", func(t *testing.T) {
				prepare := func(t *testing.T) []domain.ProductView {
					pg.Truncate(t)
					pg.InsertCosmeticProduct(t, activeProductSeed("TST-PRODUCT-COSMETIC"), "sleeve", 7)
					products, err := pg.ProductRepository().GetActiveProducts(ctx)
					require.NoError(t, err)
					require.Len(t, products, 1)
					return products
				}

				t.Run("コスメティックの商品として返る", func(t *testing.T) {
					products := prepare(t)

					assert.IsType(t, domain.CosmeticProduct{}, products[0])
				})

				t.Run("アイテム種別は、保存したアイテム種別になる", func(t *testing.T) {
					products := prepare(t)

					got, ok := products[0].(domain.CosmeticProduct)
					require.True(t, ok)
					assert.Equal(t, "sleeve", got.ItemType)
				})

				t.Run("アイテム番号は、保存したアイテム番号になる", func(t *testing.T) {
					products := prepare(t)

					got, ok := products[0].(domain.CosmeticProduct)
					require.True(t, ok)
					assert.Equal(t, int64(7), got.ItemNo)
				})
			})

			t.Run("種別がsubscriptionの商品が販売中のとき", func(t *testing.T) {
				prepare := func(t *testing.T) []domain.ProductView {
					pg.Truncate(t)
					pg.InsertSubscriptionProduct(t, activeProductSeed("TST-PRODUCT-SUBSCRIPTION"), 12)
					products, err := pg.ProductRepository().GetActiveProducts(ctx)
					require.NoError(t, err)
					require.Len(t, products, 1)
					return products
				}

				t.Run("サブスクリプションの商品として返る", func(t *testing.T) {
					products := prepare(t)

					assert.IsType(t, domain.SubscriptionProduct{}, products[0])
				})

				t.Run("課金周期の月数は、保存した月数になる", func(t *testing.T) {
					products := prepare(t)

					got, ok := products[0].(domain.SubscriptionProduct)
					require.True(t, ok)
					assert.Equal(t, int64(12), got.PeriodMonths)
				})
			})

			t.Run("商品の共通項目", func(t *testing.T) {
				seed := postgrestest.ProductSeed{
					ProductID:   "TST-PRODUCT-COMMON",
					Name:        "TST-NAME-COMMON",
					Price:       1234,
					Description: stringPointer("TST-DESCRIPTION-COMMON"),
					ImageURL:    stringPointer("TST-IMAGE-URL-COMMON"),
					IsActive:    true,
				}
				prepareWith := func(t *testing.T, seed postgrestest.ProductSeed) domain.Product {
					pg.Truncate(t)
					pg.InsertCardPackProduct(t, seed, "TST-PACK-X")
					products, err := pg.ProductRepository().GetActiveProducts(ctx)
					require.NoError(t, err)
					require.Len(t, products, 1)
					return products[0].Common()
				}

				t.Run("商品IDが保存した商品IDで返る", func(t *testing.T) {
					common := prepareWith(t, seed)

					assert.Equal(t, "TST-PRODUCT-COMMON", common.ProductID)
				})

				t.Run("名称が保存した名称で返る", func(t *testing.T) {
					common := prepareWith(t, seed)

					assert.Equal(t, "TST-NAME-COMMON", common.Name)
				})

				t.Run("価格が保存した価格で返る", func(t *testing.T) {
					common := prepareWith(t, seed)

					assert.Equal(t, int64(1234), common.Price)
				})

				t.Run("説明が保存した説明で返る", func(t *testing.T) {
					common := prepareWith(t, seed)

					require.NotNil(t, common.Description)
					assert.Equal(t, "TST-DESCRIPTION-COMMON", *common.Description)
				})

				t.Run("画像URLが保存した画像URLで返る", func(t *testing.T) {
					common := prepareWith(t, seed)

					require.NotNil(t, common.ImageURL)
					assert.Equal(t, "TST-IMAGE-URL-COMMON", *common.ImageURL)
				})

				t.Run("説明が未設定の商品は、説明が未設定のまま返る", func(t *testing.T) {
					seedWithoutDescription := seed
					seedWithoutDescription.Description = nil

					common := prepareWith(t, seedWithoutDescription)

					assert.Nil(t, common.Description)
				})

				t.Run("画像URLが未設定の商品は、画像URLが未設定のまま返る", func(t *testing.T) {
					seedWithoutImageURL := seed
					seedWithoutImageURL.ImageURL = nil

					common := prepareWith(t, seedWithoutImageURL)

					assert.Nil(t, common.ImageURL)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("販売中の商品の種別固有の属性が不正なとき、一覧を取得する", func(t *testing.T) {
				listWithMissingFaction := func(t *testing.T) error {
					pg.Truncate(t)
					insertFactionSetProductWithoutFaction(t, activeProductSeed("TST-PRODUCT-BROKEN"))
					_, err := pg.ProductRepository().GetActiveProducts(ctx)
					return err
				}
				listWithUnknownType := func(t *testing.T) error {
					pg.Truncate(t)
					pg.InsertProduct(t, activeProductSeed("TST-PRODUCT-BROKEN"), "TST-UNKNOWN")
					_, err := pg.ProductRepository().GetActiveProducts(ctx)
					return err
				}

				productIDCases := []struct {
					name string
					list func(t *testing.T) error
				}{
					{name: "種別がfaction_setで陣営が欠けた商品が販売中のとき、エラーの内容に、該当商品の商品IDが示される", list: listWithMissingFaction},
					{name: "種別が未知の値の商品が販売中のとき、エラーの内容に、該当商品の商品IDが示される", list: listWithUnknownType},
				}
				for _, tc := range productIDCases {
					t.Run(tc.name, func(t *testing.T) {
						err := tc.list(t)

						require.Error(t, err)
						assert.ErrorContains(t, err, "TST-PRODUCT-BROKEN")
					})
				}

				t.Run("種別がfaction_setで陣営が欠けた商品が販売中のとき、エラーの内容に、欠けた項目として陣営が示される", func(t *testing.T) {
					err := listWithMissingFaction(t)

					require.Error(t, err)
					assert.ErrorContains(t, err, "faction")
				})

				t.Run("種別が未知の値の商品が販売中のとき、エラーの内容に、未知の種別であることが示される", func(t *testing.T) {
					err := listWithUnknownType(t)

					require.Error(t, err)
					assert.ErrorContains(t, err, "unknown type")
				})

				t.Run("種別がfaction_setで陣営が欠けた商品が販売中で失敗したあと、続けて欠けた陣営を保存すると、一覧取得はエラーにならない", func(t *testing.T) {
					require.Error(t, listWithMissingFaction(t))
					pg.InsertProductFaction(t, "TST-PRODUCT-BROKEN", "Tenki")

					_, err := pg.ProductRepository().GetActiveProducts(ctx)

					assert.NoError(t, err)
				})
			})

			t.Run("種別がfaction_setで陣営が欠けた商品が販売停止で、ほかに販売中の商品もあるとき、一覧を取得する", func(t *testing.T) {
				prepare := func(t *testing.T) {
					pg.Truncate(t)
					insertFactionSetProductWithoutFaction(t, inactiveProductSeed("TST-PRODUCT-BROKEN"))
					pg.InsertCardPackProduct(t, activeProductSeed("TST-PRODUCT-ACTIVE"), "TST-PACK-Y")
				}

				t.Run("エラーにならない", func(t *testing.T) {
					prepare(t)

					_, err := pg.ProductRepository().GetActiveProducts(ctx)

					assert.NoError(t, err)
				})

				t.Run("ほかの販売中の商品が返る", func(t *testing.T) {
					prepare(t)

					assert.Equal(t, []string{"TST-PRODUCT-ACTIVE"}, listActiveProductIDs(t))
				})
			})
		})
	})
}

func TestProductRepositoryGetProductByID(t *testing.T) {
	ctx := context.Background()

	t.Run("[商品]商品IDによる商品の取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("商品が1件だけのとき、その商品IDを指定すると、その商品の名称が返る", func(t *testing.T) {
				pg.Truncate(t)
				seed := activeProductSeed("TST-PRODUCT-ONLY")
				seed.Name = "TST-NAME-ONLY"
				pg.InsertCardPackProduct(t, seed, "TST-PACK-X")

				got, err := pg.ProductRepository().GetProductByID(ctx, "TST-PRODUCT-ONLY")

				require.NoError(t, err)
				assert.Equal(t, "TST-NAME-ONLY", got.Common().Name)
			})

			t.Run("商品が複数あるとき、指定した商品IDの商品だけが返る", func(t *testing.T) {
				pg.Truncate(t)
				pg.InsertCardPackProduct(t, activeProductSeed("TST-PRODUCT-1"), "TST-PACK-X")
				pg.InsertCardPackProduct(t, activeProductSeed("TST-PRODUCT-2"), "TST-PACK-Y")
				pg.InsertSubscriptionProduct(t, activeProductSeed("TST-PRODUCT-3"), 1)

				got, err := pg.ProductRepository().GetProductByID(ctx, "TST-PRODUCT-2")

				require.NoError(t, err)
				assert.Equal(t, "TST-PRODUCT-2", got.Common().ProductID)
			})

			t.Run("販売停止の商品の商品IDを指定しても、その商品が返る", func(t *testing.T) {
				pg.Truncate(t)
				pg.InsertCardPackProduct(t, inactiveProductSeed("TST-PRODUCT-INACTIVE"), "TST-PACK-X")

				got, err := pg.ProductRepository().GetProductByID(ctx, "TST-PRODUCT-INACTIVE")

				require.NoError(t, err)
				assert.Equal(t, "TST-PRODUCT-INACTIVE", got.Common().ProductID)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("指定した商品IDの商品が無い", func(t *testing.T) {
				cases := []struct {
					name      string
					productID string
				}{
					{name: "保存されていない商品IDのとき、商品が見つからないエラーになる", productID: "TST-PRODUCT-MISSING"},
					{name: "空文字の商品IDのとき、商品が見つからないエラーになる", productID: ""},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						pg.InsertCardPackProduct(t, activeProductSeed("TST-PRODUCT-1"), "TST-PACK-X")

						_, err := pg.ProductRepository().GetProductByID(ctx, tc.productID)

						assert.ErrorIs(t, err, port.ErrNotFound)
					})
				}
			})

			t.Run("種別固有の属性が不正な商品の商品IDを指定する", func(t *testing.T) {
				getWithMissingFaction := func(t *testing.T) error {
					pg.Truncate(t)
					insertFactionSetProductWithoutFaction(t, activeProductSeed("TST-PRODUCT-BROKEN"))
					_, err := pg.ProductRepository().GetProductByID(ctx, "TST-PRODUCT-BROKEN")
					return err
				}
				getWithUnknownType := func(t *testing.T) error {
					pg.Truncate(t)
					pg.InsertProduct(t, activeProductSeed("TST-PRODUCT-BROKEN"), "TST-UNKNOWN")
					_, err := pg.ProductRepository().GetProductByID(ctx, "TST-PRODUCT-BROKEN")
					return err
				}
				cases := []struct {
					name string
					get  func(t *testing.T) error
				}{
					{name: "種別がfaction_setで陣営が欠けた商品", get: getWithMissingFaction},
					{name: "種別が未知の値の商品", get: getWithUnknownType},
				}

				for _, tc := range cases {
					t.Run(tc.name+"のとき、エラーの種類は、商品が見つからないエラーではない", func(t *testing.T) {
						err := tc.get(t)

						require.Error(t, err)
						assert.NotErrorIs(t, err, port.ErrNotFound)
					})
				}

				for _, tc := range cases {
					t.Run(tc.name+"のとき、エラーの内容に、該当商品の商品IDが示される", func(t *testing.T) {
						err := tc.get(t)

						require.Error(t, err)
						assert.ErrorContains(t, err, "TST-PRODUCT-BROKEN")
					})
				}

				t.Run("種別がfaction_setで陣営が欠けた商品のとき、エラーの内容に、欠けた項目として陣営が示される", func(t *testing.T) {
					err := getWithMissingFaction(t)

					require.Error(t, err)
					assert.ErrorContains(t, err, "faction")
				})

				t.Run("種別が未知の値の商品のとき、エラーの内容に、未知の種別であることが示される", func(t *testing.T) {
					err := getWithUnknownType(t)

					require.Error(t, err)
					assert.ErrorContains(t, err, "unknown type")
				})

				t.Run("種別がfaction_setで陣営が欠けた商品で失敗したあと、続けて欠けた陣営を保存すると、その商品が返る", func(t *testing.T) {
					require.Error(t, getWithMissingFaction(t))
					pg.InsertProductFaction(t, "TST-PRODUCT-BROKEN", "Tenki")

					got, err := pg.ProductRepository().GetProductByID(ctx, "TST-PRODUCT-BROKEN")

					require.NoError(t, err)
					assert.Equal(t, "TST-PRODUCT-BROKEN", got.Common().ProductID)
				})
			})
		})
	})
}
