//go:build integration

package rest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

func TestProductList(t *testing.T) {
	t.Run("[商品一覧]販売中の商品の取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("商品一覧を取得するとき", func(t *testing.T) {
				t.Run("販売中の商品が0件のとき", func(t *testing.T) {
					t.Run("200になる", func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)

						response := env.ListProducts(routertest.PlayerOne)

						assert.Equal(t, http.StatusOK, response.Status)
					})

					t.Run("productsは空配列になる", func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)

						response := env.ListProducts(routertest.PlayerOne)

						assert.JSONEq(t, `{"products":[]}`, string(response.Body))
					})
				})

				twoProductCases := []struct {
					given   string
					seeds   []func(env *routertest.Env, t *testing.T)
					wantIDs []string
				}{
					{
						given:   "コスメティックのTST-PRODUCT-ITEM-1とカードパックのTST-PRODUCT-CARD-PACK-1",
						seeds:   []func(env *routertest.Env, t *testing.T){(*routertest.Env).SeedItemProduct, (*routertest.Env).SeedCardPackProduct},
						wantIDs: []string{"TST-PRODUCT-ITEM-1", "TST-PRODUCT-CARD-PACK-1"},
					},
					{
						given:   "コスメティックのTST-PRODUCT-ITEM-1とサブスクリプションのTST-PRODUCT-SUB-1",
						seeds:   []func(env *routertest.Env, t *testing.T){(*routertest.Env).SeedItemProduct, (*routertest.Env).SeedSubscriptionProduct},
						wantIDs: []string{"TST-PRODUCT-ITEM-1", "TST-PRODUCT-SUB-1"},
					},
				}

				for _, tt := range twoProductCases {
					t.Run("販売中の商品が"+tt.given+"の2件のとき、200になる", func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)
						for _, seed := range tt.seeds {
							seed(env, t)
						}

						response := env.ListProducts(routertest.PlayerOne)

						assert.Equal(t, http.StatusOK, response.Status)
					})
				}

				for _, tt := range twoProductCases {
					t.Run("販売中の商品が"+tt.given+"の2件のとき、productsにはその2件のproduct_idだけが入る", func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)
						for _, seed := range tt.seeds {
							seed(env, t)
						}

						response := env.ListProducts(routertest.PlayerOne)

						assert.ElementsMatch(t, tt.wantIDs, routertest.ExtractProductIDs(response.Products(t)))
					})
				}

				t.Run("販売停止中のTST-PRODUCT-INACTIVEが登録されているとき、productsにTST-PRODUCT-INACTIVEは含まれない", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)
					env.SeedInactiveProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.NotContains(t, routertest.ExtractProductIDs(response.Products(t)), "TST-PRODUCT-INACTIVE")
				})

				t.Run("プレイヤーがTST-PRODUCT-ITEM-1を購入していないとき、productsのTST-PRODUCT-ITEM-1のis_ownedはfalseになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.False(t, response.Product(t, "TST-PRODUCT-ITEM-1").IsOwned)
				})
			})

			t.Run("販売中の商品が陣営セットのTST-PRODUCT-FACTION-SET-1の1件で、陣営がSHEのとき", func(t *testing.T) {
				t.Run("TST-PRODUCT-FACTION-SET-1のtypeはfaction_setになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedFactionSetProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `"faction_set"`, string(response.ProductFields(t, "TST-PRODUCT-FACTION-SET-1")["type"]))
				})

				t.Run("TST-PRODUCT-FACTION-SET-1のcontentはfactionがSHEのオブジェクトになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedFactionSetProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `{"faction":"SHE"}`, string(response.ProductFields(t, "TST-PRODUCT-FACTION-SET-1")["content"]))
				})
			})

			t.Run("販売中の商品がコスメティックのTST-PRODUCT-ITEM-1の1件で、item_typeがstamp、item_noが1のとき", func(t *testing.T) {
				t.Run("TST-PRODUCT-ITEM-1のtypeはcosmeticになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `"cosmetic"`, string(response.ProductFields(t, "TST-PRODUCT-ITEM-1")["type"]))
				})

				t.Run("TST-PRODUCT-ITEM-1のcontentはitem_typeがstamp、item_noが1のオブジェクトになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `{"item_type":"stamp","item_no":1}`, string(response.ProductFields(t, "TST-PRODUCT-ITEM-1")["content"]))
				})
			})

			t.Run("販売中の商品がサブスクリプションのTST-PRODUCT-SUB-1の1件のとき", func(t *testing.T) {
				t.Run("TST-PRODUCT-SUB-1のtypeはsubscriptionになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedSubscriptionProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `"subscription"`, string(response.ProductFields(t, "TST-PRODUCT-SUB-1")["type"]))
				})

				t.Run("TST-PRODUCT-SUB-1のcontentは空のオブジェクトになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedSubscriptionProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `{}`, string(response.ProductFields(t, "TST-PRODUCT-SUB-1")["content"]))
				})
			})

			t.Run("販売中の商品がカードパックのTST-PRODUCT-CARD-PACK-1の1件のとき", func(t *testing.T) {
				t.Run("TST-PRODUCT-CARD-PACK-1のtypeはcard_packになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedCardPackProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `"card_pack"`, string(response.ProductFields(t, "TST-PRODUCT-CARD-PACK-1")["type"]))
				})

				t.Run("TST-PRODUCT-CARD-PACK-1のcontentはcard_pack_idがTST-CARD-PACK-1のオブジェクトになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedCardPackProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.Equal(t, map[string]any{"card_pack_id": "TST-CARD-PACK-1"}, response.Product(t, "TST-PRODUCT-CARD-PACK-1").Content)
				})
			})

			t.Run("販売中の商品がTST-PRODUCT-ITEM-1の1件のとき", func(t *testing.T) {
				t.Run("productsのproduct_idはTST-PRODUCT-ITEM-1だけになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.Equal(t, []string{"TST-PRODUCT-ITEM-1"}, routertest.ExtractProductIDs(response.Products(t)))
				})

				t.Run("名称がTST-NAME-0001のとき、TST-PRODUCT-ITEM-1のnameはTST-NAME-0001になる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					seed := routertest.BuildItemProductSeed()
					seed.Name = "TST-NAME-0001"
					env.SeedItemProductFrom(t, seed)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `"TST-NAME-0001"`, string(response.ProductFields(t, "TST-PRODUCT-ITEM-1")["name"]))
				})

				t.Run("価格が980のとき、TST-PRODUCT-ITEM-1のpriceは980になる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					seed := routertest.BuildItemProductSeed()
					seed.Price = 980
					env.SeedItemProductFrom(t, seed)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `980`, string(response.ProductFields(t, "TST-PRODUCT-ITEM-1")["price"]))
				})

				t.Run("説明がTST-DESC-0001のとき、TST-PRODUCT-ITEM-1のdescriptionはTST-DESC-0001になる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					seed := routertest.BuildItemProductSeed()
					description := "TST-DESC-0001"
					seed.Description = &description
					env.SeedItemProductFrom(t, seed)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `"TST-DESC-0001"`, string(response.ProductFields(t, "TST-PRODUCT-ITEM-1")["description"]))
				})

				t.Run("説明が未設定のとき、TST-PRODUCT-ITEM-1にdescriptionのキーは含まれない", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					seed := routertest.BuildItemProductSeed()
					seed.Description = nil
					env.SeedItemProductFrom(t, seed)

					response := env.ListProducts(routertest.PlayerOne)

					assert.NotContains(t, response.ProductFields(t, "TST-PRODUCT-ITEM-1"), "description")
				})

				t.Run("画像URLがhttps://example.com/TST-0001.pngのとき、TST-PRODUCT-ITEM-1のimage_urlはhttps://example.com/TST-0001.pngになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					seed := routertest.BuildItemProductSeed()
					imageURL := "https://example.com/TST-0001.png"
					seed.ImageURL = &imageURL
					env.SeedItemProductFrom(t, seed)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `"https://example.com/TST-0001.png"`, string(response.ProductFields(t, "TST-PRODUCT-ITEM-1")["image_url"]))
				})

				t.Run("画像URLが未設定のとき、TST-PRODUCT-ITEM-1にimage_urlのキーは含まれない", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					seed := routertest.BuildItemProductSeed()
					seed.ImageURL = nil
					env.SeedItemProductFrom(t, seed)

					response := env.ListProducts(routertest.PlayerOne)

					assert.NotContains(t, response.ProductFields(t, "TST-PRODUCT-ITEM-1"), "image_url")
				})

				t.Run("TST-PRODUCT-ITEM-1のis_activeはtrueになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `true`, string(response.ProductFields(t, "TST-PRODUCT-ITEM-1")["is_active"]))
				})
			})
		})
	})
}
