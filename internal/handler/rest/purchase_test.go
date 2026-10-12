//go:build integration

package rest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

type purchasableProduct struct {
	name      string
	productID string
	seed      func(env *routertest.Env, t *testing.T)
}

var purchasableProducts = []purchasableProduct{
	{name: "陣営セット商品", productID: "TST-PRODUCT-FACTION-SET-1", seed: (*routertest.Env).SeedFactionSetProduct},
	{name: "カードパック商品", productID: "TST-PRODUCT-CARD-PACK-1", seed: (*routertest.Env).SeedCardPackProduct},
	{name: "コスメティック商品", productID: "TST-PRODUCT-ITEM-1", seed: (*routertest.Env).SeedItemProduct},
}

func seedAndPurchase(t *testing.T, product purchasableProduct) (*routertest.Env, routertest.Response) {
	t.Helper()
	env := routertest.BuildEnv(t, pg)
	product.seed(env, t)
	return env, env.Purchase(t, routertest.PlayerOne, product.productID, "ios", "TST-PURCHASE-TOKEN-1")
}

func TestOneTimePurchase(t *testing.T) {
	t.Run("[単発購入]商品の購入", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("レシート検証が有効と判定し、購入するとき", func(t *testing.T) {
				for _, product := range purchasableProducts {
					t.Run(product.name+"のとき、200になる", func(t *testing.T) {
						_, response := seedAndPurchase(t, product)

						assert.Equal(t, http.StatusOK, response.Status)
					})
				}

				for _, product := range purchasableProducts {
					t.Run(product.name+"のとき、messageが購入の受付を示す値になる", func(t *testing.T) {
						_, response := seedAndPurchase(t, product)

						assert.Equal(t, "purchase accepted", response.PurchaseBody(t).Message)
					})
				}

				for _, product := range purchasableProducts {
					t.Run(product.name+"のとき、product_idが購入した商品IDになる", func(t *testing.T) {
						_, response := seedAndPurchase(t, product)

						assert.Equal(t, product.productID, response.PurchaseBody(t).ProductID)
					})
				}
			})

			t.Run("レシート検証が有効と判定し、購入した後の同じプレイヤーの商品一覧", func(t *testing.T) {
				for _, product := range purchasableProducts {
					t.Run(product.name+"のとき、その商品のis_ownedがtrueになる", func(t *testing.T) {
						env, _ := seedAndPurchase(t, product)

						response := env.ListProducts(routertest.PlayerOne)

						assert.True(t, response.Product(t, product.productID).IsOwned)
					})
				}
			})

			t.Run("プレイヤーAが購入した後、別のプレイヤーBの商品一覧では、その商品のis_ownedはfalseのままになる", func(t *testing.T) {
				env := routertest.BuildEnv(t, pg)
				env.SeedItemProduct(t)
				env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

				response := env.ListProducts(routertest.PlayerTwo)

				assert.False(t, response.Product(t, "TST-PRODUCT-ITEM-1").IsOwned)
			})

			t.Run("purchase_tokenがTST-PURCHASE-TOKEN-1で購入するとき", func(t *testing.T) {
				t.Run("platformがiosのとき、Appleのレシート検証にTST-PURCHASE-TOKEN-1がそのまま渡される", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)

					env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

					assert.Equal(t, []string{"TST-PURCHASE-TOKEN-1"}, env.AppleReceipt.PurchaseTokens())
				})

				t.Run("platformがandroidのとき、Googleのレシート検証にTST-PURCHASE-TOKEN-1がそのまま渡される", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)

					env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "android", "TST-PURCHASE-TOKEN-1")

					assert.Equal(t, []string{"TST-PURCHASE-TOKEN-1"}, env.GoogleReceipt.PurchaseTokens())
				})
			})

			t.Run("同じplatformと同じpurchase_tokenで2回続けて購入するとき", func(t *testing.T) {
				t.Run("2回目も200になる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)
					env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

					response := env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("購入記録は1件だけになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)
					env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")
					env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

					assert.Equal(t, 1, env.CountRows(t, "shop.one_time_purchases"))
				})
			})
		})
	})
}
