package presenter_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/domain/domaintest"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
)

func newSubscriptionItem(common domain.Product, isOwned bool) domain.ProductWithOwnership {
	return domain.ProductWithOwnership{
		ProductView: domain.SubscriptionProduct{Product: common, PeriodMonths: 1},
		IsOwned:     isOwned,
	}
}

func marshalToFields(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	return fields
}

func convertToFields(t *testing.T, item domain.ProductWithOwnership) map[string]json.RawMessage {
	t.Helper()
	response, err := presenter.ToProductResponse(item)
	require.NoError(t, err)
	return marshalToFields(t, response)
}

func convertListToProducts(t *testing.T, items []domain.ProductWithOwnership) []map[string]json.RawMessage {
	t.Helper()
	response, err := presenter.ToProductListResponse(items)
	require.NoError(t, err)

	var products []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(marshalToFields(t, response)["products"], &products))
	return products
}

type fieldCase struct {
	name string
	item domain.ProductWithOwnership
	key  string
	want string
}

func runFieldCases(t *testing.T, cases []fieldCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := convertToFields(t, tc.item)

			assert.JSONEq(t, tc.want, string(fields[tc.key]))
		})
	}
}

func TestToProductResponse(t *testing.T) {
	t.Run("[商品]商品情報の応答への変換", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("商品種別ごとのcontent", func(t *testing.T) {
				cases := []struct {
					name string
					item domain.ProductWithOwnership
					want string
				}{
					{
						name: "商品種別がfaction_setで、陣営がTST-0003のとき、contentは`{\"faction\":\"TST-0003\"}`になる",
						item: domain.ProductWithOwnership{
							ProductView: domain.FactionSetProduct{
								Product:    domaintest.NewCommonProduct("faction_set"),
								CardPackID: "TST-0002",
								Faction:    "TST-0003",
							},
						},
						want: `{"faction":"TST-0003"}`,
					},
					{
						name: "商品種別がcosmeticで、item_typeがTST-ITEM-TYPE、item_noが7のとき、contentは`{\"item_type\":\"TST-ITEM-TYPE\",\"item_no\":7}`になる",
						item: domain.ProductWithOwnership{
							ProductView: domain.CosmeticProduct{
								Product:  domaintest.NewCommonProduct("cosmetic"),
								ItemType: "TST-ITEM-TYPE",
								ItemNo:   7,
							},
						},
						want: `{"item_type":"TST-ITEM-TYPE","item_no":7}`,
					},
					{
						name: "商品種別がsubscriptionのとき、contentは`{}`になる",
						item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
						want: `{}`,
					},
					{
						name: "商品種別がcard_packで、card_pack_idがTST-0002のとき、contentは`{\"card_pack_id\":\"TST-0002\"}`になる",
						item: domain.ProductWithOwnership{
							ProductView: domain.CardPackProduct{
								Product:    domaintest.NewCommonProduct("card_pack"),
								CardPackID: "TST-0002",
							},
						},
						want: `{"card_pack_id":"TST-0002"}`,
					},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						fields := convertToFields(t, tc.item)

						assert.JSONEq(t, tc.want, string(fields["content"]))
					})
				}
			})

			t.Run("商品情報の項目の引き継ぎ", func(t *testing.T) {
				runFieldCases(t, []fieldCase{
					{
						name: "商品IDがTST-0001のとき、応答のproduct_idはTST-0001になる",
						item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
						key:  "product_id",
						want: `"TST-0001"`,
					},
					{
						name: "名称がTST-NAME-0001のとき、応答のnameはTST-NAME-0001になる",
						item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
						key:  "name",
						want: `"TST-NAME-0001"`,
					},
				})

				t.Run("商品種別の引き継ぎ", func(t *testing.T) {
					runFieldCases(t, []fieldCase{
						{
							name: "商品種別がfaction_setのとき、応答のtypeはfaction_setになる",
							item: domain.ProductWithOwnership{
								ProductView: domain.FactionSetProduct{
									Product:    domaintest.NewCommonProduct("faction_set"),
									CardPackID: "TST-0002",
									Faction:    "TST-0003",
								},
							},
							key:  "type",
							want: `"faction_set"`,
						},
						{
							name: "商品種別がcosmeticのとき、応答のtypeはcosmeticになる",
							item: domain.ProductWithOwnership{
								ProductView: domain.CosmeticProduct{
									Product:  domaintest.NewCommonProduct("cosmetic"),
									ItemType: "TST-ITEM-TYPE",
									ItemNo:   7,
								},
							},
							key:  "type",
							want: `"cosmetic"`,
						},
						{
							name: "商品種別がsubscriptionのとき、応答のtypeはsubscriptionになる",
							item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
							key:  "type",
							want: `"subscription"`,
						},
						{
							name: "商品種別がcard_packのとき、応答のtypeはcard_packになる",
							item: domain.ProductWithOwnership{
								ProductView: domain.CardPackProduct{
									Product:    domaintest.NewCommonProduct("card_pack"),
									CardPackID: "TST-0002",
								},
							},
							key:  "type",
							want: `"card_pack"`,
						},
					})
				})

				runFieldCases(t, []fieldCase{
					{
						name: "価格が980のとき、応答のpriceは980になる",
						item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
						key:  "price",
						want: `980`,
					},
					{
						name: "説明がTST-DESC-0001のとき、応答のdescriptionはTST-DESC-0001になる",
						item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
						key:  "description",
						want: `"TST-DESC-0001"`,
					},
				})

				t.Run("説明が未設定のとき、応答をJSONにするとdescriptionのキーが含まれない", func(t *testing.T) {
					common := domaintest.NewCommonProduct("subscription")
					common.Description = nil

					fields := convertToFields(t, newSubscriptionItem(common, false))

					assert.NotContains(t, fields, "description")
				})

				t.Run("画像URLがhttps://example.com/TST-0001.pngのとき、応答のimage_urlは同じURLになる", func(t *testing.T) {
					fields := convertToFields(t, newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false))

					assert.JSONEq(t, `"https://example.com/TST-0001.png"`, string(fields["image_url"]))
				})

				t.Run("画像URLが未設定のとき、応答をJSONにするとimage_urlのキーが含まれない", func(t *testing.T) {
					common := domaintest.NewCommonProduct("subscription")
					common.ImageURL = nil

					fields := convertToFields(t, newSubscriptionItem(common, false))

					assert.NotContains(t, fields, "image_url")
				})

				inactiveCommon := domaintest.NewCommonProduct("subscription")
				inactiveCommon.IsActive = false

				runFieldCases(t, []fieldCase{
					{
						name: "販売中のとき、応答のis_activeはtrueになる",
						item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
						key:  "is_active",
						want: `true`,
					},
					{
						name: "販売中でないとき、応答のis_activeはfalseになる",
						item: newSubscriptionItem(inactiveCommon, false),
						key:  "is_active",
						want: `false`,
					},
					{
						name: "所有済みのとき、応答のis_ownedはtrueになる",
						item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), true),
						key:  "is_owned",
						want: `true`,
					},
					{
						name: "所有済みでないとき、応答のis_ownedはfalseになる",
						item: newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
						key:  "is_owned",
						want: `false`,
					},
				})
			})
		})
	})
}

func TestToProductListResponse(t *testing.T) {
	t.Run("[商品]商品一覧の応答への変換", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("商品がfaction_set・cosmeticの順に2件あるとき、応答のproduct_idは同じ順に並ぶ", func(t *testing.T) {
				factionSetCommon := domaintest.NewCommonProduct("faction_set")
				factionSetCommon.ProductID = "TST-0001"
				cosmeticCommon := domaintest.NewCommonProduct("cosmetic")
				cosmeticCommon.ProductID = "TST-0004"
				items := []domain.ProductWithOwnership{
					{ProductView: domain.FactionSetProduct{Product: factionSetCommon, CardPackID: "TST-0002", Faction: "TST-0003"}},
					{ProductView: domain.CosmeticProduct{Product: cosmeticCommon, ItemType: "TST-ITEM-TYPE", ItemNo: 7}},
				}

				products := convertListToProducts(t, items)

				require.Len(t, products, 2)
				assert.JSONEq(t, `"TST-0001"`, string(products[0]["product_id"]))
				assert.JSONEq(t, `"TST-0004"`, string(products[1]["product_id"]))
			})

			t.Run("所有済みの商品と所有済みでない商品が1件ずつあるとき、各応答のis_ownedは対応する商品の所有状態になる", func(t *testing.T) {
				items := []domain.ProductWithOwnership{
					newSubscriptionItem(domaintest.NewCommonProduct("subscription"), true),
					newSubscriptionItem(domaintest.NewCommonProduct("subscription"), false),
				}

				products := convertListToProducts(t, items)

				require.Len(t, products, 2)
				assert.JSONEq(t, `true`, string(products[0]["is_owned"]))
				assert.JSONEq(t, `false`, string(products[1]["is_owned"]))
			})

			t.Run("商品がcard_packでcard_pack_idがTST-0002の1件だけのとき、応答はcontentが`{\"card_pack_id\":\"TST-0002\"}`の1件になる", func(t *testing.T) {
				items := []domain.ProductWithOwnership{
					{ProductView: domain.CardPackProduct{Product: domaintest.NewCommonProduct("card_pack"), CardPackID: "TST-0002"}},
				}

				products := convertListToProducts(t, items)

				require.Len(t, products, 1)
				assert.JSONEq(t, `{"card_pack_id":"TST-0002"}`, string(products[0]["content"]))
			})

			emptyCases := []struct {
				name  string
				items []domain.ProductWithOwnership
			}{
				{name: "商品が0件(指定なし)のとき、応答をJSONにすると`[]`になる", items: nil},
				{name: "商品が0件(空の一覧)のとき、応答をJSONにすると`[]`になる", items: []domain.ProductWithOwnership{}},
			}
			for _, tc := range emptyCases {
				t.Run(tc.name, func(t *testing.T) {
					response, err := presenter.ToProductListResponse(tc.items)

					require.NoError(t, err)
					assert.JSONEq(t, `[]`, string(marshalToFields(t, response)["products"]))
				})
			}
		})
	})
}
