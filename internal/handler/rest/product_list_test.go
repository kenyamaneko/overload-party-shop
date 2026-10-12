//go:build integration

package rest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

type productListSeed struct {
	name string
	seed func(env *routertest.Env, t *testing.T)
}

func seedNothing(*routertest.Env, *testing.T) {}

func seedItemAndCardPackProducts(env *routertest.Env, t *testing.T) {
	env.SeedItemProduct(t)
	env.SeedCardPackProduct(t)
}

func seedItemAndSubscriptionProducts(env *routertest.Env, t *testing.T) {
	env.SeedItemProduct(t)
	env.SeedSubscriptionProduct(t)
}

func TestProductList(t *testing.T) {
	t.Run("[商品一覧]販売中の商品の取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("商品一覧を取得するとき", func(t *testing.T) {
				statusCases := []productListSeed{
					{name: "販売中の商品が0件のとき、200になる", seed: seedNothing},
					{name: "販売中の商品がコスメティックのTST-PRODUCT-ITEM-1とカードパックのTST-PRODUCT-CARD-PACK-1の2件のとき、200になる", seed: seedItemAndCardPackProducts},
				}
				for _, tt := range statusCases {
					t.Run(tt.name, func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)
						tt.seed(env, t)

						response := env.ListProducts(routertest.PlayerOne)

						assert.Equal(t, http.StatusOK, response.Status)
					})
				}

				t.Run("販売中の商品が0件のとき、productsは空配列になる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)

					response := env.ListProducts(routertest.PlayerOne)

					assert.JSONEq(t, `{"products":[]}`, string(response.Body))
				})

				listedCases := []struct {
					productListSeed
					wantIDs []string
				}{
					{
						productListSeed: productListSeed{name: "販売中の商品がTST-PRODUCT-ITEM-1とTST-PRODUCT-SUB-1の2件のとき、productsにはその2件のproduct_idだけが入る", seed: seedItemAndSubscriptionProducts},
						wantIDs:         []string{"TST-PRODUCT-ITEM-1", "TST-PRODUCT-SUB-1"},
					},
					{
						productListSeed: productListSeed{name: "販売中の商品がコスメティックのTST-PRODUCT-ITEM-1とカードパックのTST-PRODUCT-CARD-PACK-1の2件のとき、productsにはその2件のproduct_idだけが入る", seed: seedItemAndCardPackProducts},
						wantIDs:         []string{"TST-PRODUCT-ITEM-1", "TST-PRODUCT-CARD-PACK-1"},
					},
				}
				for _, tt := range listedCases {
					t.Run(tt.name, func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)
						tt.seed(env, t)

						response := env.ListProducts(routertest.PlayerOne)

						assert.ElementsMatch(t, tt.wantIDs, routertest.ExtractProductIDs(response.Products(t)))
					})
				}

				t.Run("販売中の商品がカードパックのTST-PRODUCT-CARD-PACK-1の1件のとき、TST-PRODUCT-CARD-PACK-1のcontentはcard_pack_idがTST-CARD-PACK-1のオブジェクトになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedCardPackProduct(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.Equal(t, map[string]any{"card_pack_id": "TST-CARD-PACK-1"}, response.Product(t, "TST-PRODUCT-CARD-PACK-1").Content)
				})

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
		})
	})
}
