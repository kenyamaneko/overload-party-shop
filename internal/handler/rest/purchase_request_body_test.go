//go:build integration

package rest_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

type purchaseRoute struct {
	post func(env *routertest.Env, body []byte) routertest.Response
}

type missingField struct {
	given string
	field string
	body  string
}

var missingFields = []missingField{
	{given: "product_idが無いとき", field: "product_id", body: `{"platform":"ios","purchase_token":"TST-PURCHASE-TOKEN-1"}`},
	{given: "platformが無いとき", field: "platform", body: `{"product_id":"TST-PRODUCT-ITEM-1","purchase_token":"TST-PURCHASE-TOKEN-1"}`},
	{given: "purchase_tokenが無いとき", field: "purchase_token", body: `{"product_id":"TST-PRODUCT-ITEM-1","platform":"ios"}`},
}

var (
	purchaseOneTimeRoute = purchaseRoute{post: func(env *routertest.Env, body []byte) routertest.Response {
		return env.PostPurchase(routertest.PlayerOne, body)
	}}
	purchaseSubscriptionRoute = purchaseRoute{post: func(env *routertest.Env, body []byte) routertest.Response {
		return env.PostSubscribe(routertest.PlayerOne, body)
	}}
)

func describeNotJSONBody(t *testing.T, route purchaseRoute) {
	t.Run("400になる", func(t *testing.T) {
		env := routertest.BuildEnv(t, pg)

		response := route.post(env, []byte("not-json"))

		assert.Equal(t, http.StatusBadRequest, response.Status)
	})

	t.Run("errorに本文がJSONとして読めないことを示す語が含まれる", func(t *testing.T) {
		env := routertest.BuildEnv(t, pg)

		response := route.post(env, []byte("not-json"))

		assert.Contains(t, response.ErrorMessage(t), "request body is not valid JSON")
	})
}

func describeEmptyBodyStatus(t *testing.T, route purchaseRoute) {
	t.Run("400になる", func(t *testing.T) {
		env := routertest.BuildEnv(t, pg)

		response := route.post(env, nil)

		assert.Equal(t, http.StatusBadRequest, response.Status)
	})
}

func describeMissingFields(t *testing.T, route purchaseRoute) {
	for _, tt := range missingFields {
		t.Run(tt.given+"、400になる", func(t *testing.T) {
			env := routertest.BuildEnv(t, pg)

			response := route.post(env, []byte(tt.body))

			assert.Equal(t, http.StatusBadRequest, response.Status)
		})
	}

	for _, tt := range missingFields {
		t.Run(tt.given+"、errorに欠落した項目名"+tt.field+"が入る", func(t *testing.T) {
			env := routertest.BuildEnv(t, pg)

			response := route.post(env, []byte(tt.body))

			assert.Contains(t, response.ErrorMessage(t), tt.field)
		})
	}
}

func TestPurchaseRequestBody(t *testing.T) {
	t.Run("[購入リクエスト]本文の読み取り", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("商品の購入のとき", func(t *testing.T) {
				t.Run("本文がJSONでないnot-jsonのとき", func(t *testing.T) {
					describeNotJSONBody(t, purchaseOneTimeRoute)

					t.Run("ログのレベルがWarnの警告ログが1件出る", func(t *testing.T) {
						logs := logtest.Capture(t)
						env := routertest.BuildEnv(t, pg)

						env.PostPurchase(routertest.PlayerOne, []byte("not-json"))

						assert.Len(t, logs.SelectByMessageAndLevel("request body is not valid JSON", slog.LevelWarn), 1)
					})

					t.Run("警告ログのpathが商品の購入のパスになる", func(t *testing.T) {
						logs := logtest.Capture(t)
						env := routertest.BuildEnv(t, pg)

						env.PostPurchase(routertest.PlayerOne, []byte("not-json"))

						records := logs.SelectByMessage("request body is not valid JSON")
						require.Len(t, records, 1)
						assert.Equal(t, "/api/v1/shop/purchase", records[0].String("path"))
					})
				})

				t.Run("本文が空のとき", func(t *testing.T) {
					describeEmptyBodyStatus(t, purchaseOneTimeRoute)

					tests := []struct {
						name string
						body []byte
					}{
						{name: "本文が長さ0のとき、errorに本文が空であることを示す語が含まれる", body: nil},
						{name: "本文が空白だけのとき、errorに本文が空であることを示す語が含まれる", body: []byte("  \n ")},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							env := routertest.BuildEnv(t, pg)

							response := env.PostPurchase(routertest.PlayerOne, tt.body)

							assert.Contains(t, response.ErrorMessage(t), "request body is empty")
						})
					}

					t.Run("本文がJSONとして読めないことを示す警告ログは出ない", func(t *testing.T) {
						logs := logtest.Capture(t)
						env := routertest.BuildEnv(t, pg)

						env.PostPurchase(routertest.PlayerOne, nil)

						assert.Empty(t, logs.SelectByMessage("request body is not valid JSON"))
					})
				})

				describeMissingFields(t, purchaseOneTimeRoute)

				t.Run("本文を読めず400になった後", func(t *testing.T) {
					buildEnvAfterUnreadableBody := func(t *testing.T) *routertest.Env {
						env := routertest.BuildEnv(t, pg)
						env.SeedItemProduct(t)
						env.PostPurchase(routertest.PlayerOne, []byte("not-json"))
						return env
					}

					t.Run("レシート検証が有効と判定するコスメティック商品TST-PRODUCT-ITEM-1の購入を、正しい本文でもう一度送ると", func(t *testing.T) {
						t.Run("200になる", func(t *testing.T) {
							env := buildEnvAfterUnreadableBody(t)

							response := env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

							assert.Equal(t, http.StatusOK, response.Status)
						})

						t.Run("商品一覧でTST-PRODUCT-ITEM-1のis_ownedがtrueになる", func(t *testing.T) {
							env := buildEnvAfterUnreadableBody(t)
							env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

							response := env.ListProducts(routertest.PlayerOne)

							assert.True(t, response.Product(t, "TST-PRODUCT-ITEM-1").IsOwned)
						})
					})
				})
			})

			t.Run("サブスクリプションの購入のとき", func(t *testing.T) {
				t.Run("本文がJSONでないnot-jsonのとき", func(t *testing.T) {
					describeNotJSONBody(t, purchaseSubscriptionRoute)
				})

				t.Run("本文が空のとき", func(t *testing.T) {
					describeEmptyBodyStatus(t, purchaseSubscriptionRoute)

					t.Run("errorに本文が空であることを示す語が含まれる", func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)

						response := purchaseSubscriptionRoute.post(env, nil)

						assert.Contains(t, response.ErrorMessage(t), "request body is empty")
					})
				})

				describeMissingFields(t, purchaseSubscriptionRoute)

				t.Run("本文を読めず400になった後", func(t *testing.T) {
					buildEnvAfterUnreadableBody := func(t *testing.T) *routertest.Env {
						env := routertest.BuildEnv(t, pg)
						env.SeedSubscriptionProduct(t)
						env.PostSubscribe(routertest.PlayerOne, []byte("not-json"))
						return env
					}

					t.Run("サブスクリプション検証が有効と判定するTST-PRODUCT-SUB-1の購入を、正しい本文でもう一度送ると", func(t *testing.T) {
						t.Run("200になる", func(t *testing.T) {
							env := buildEnvAfterUnreadableBody(t)

							response := env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

							assert.Equal(t, http.StatusOK, response.Status)
						})

						t.Run("商品一覧でTST-PRODUCT-SUB-1のis_ownedがtrueになる", func(t *testing.T) {
							env := buildEnvAfterUnreadableBody(t)
							env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

							response := env.ListProducts(routertest.PlayerOne)

							assert.True(t, response.Product(t, "TST-PRODUCT-SUB-1").IsOwned)
						})
					})
				})
			})

			t.Run("本文がJSONでない商品の購入の後、プレイヤーの商品一覧でその商品のis_ownedはfalseのままになる", func(t *testing.T) {
				env := routertest.BuildEnv(t, pg)
				env.SeedItemProduct(t)
				env.PostPurchase(routertest.PlayerOne, []byte("not-json"))

				response := env.ListProducts(routertest.PlayerOne)

				assert.False(t, response.Product(t, "TST-PRODUCT-ITEM-1").IsOwned)
			})
		})
	})
}
