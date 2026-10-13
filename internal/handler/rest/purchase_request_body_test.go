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

type rejectedBody struct {
	name string
	body []byte
	want string
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

func describeRequestBodyReading(t *testing.T, route purchaseRoute) {
	rejectedBodies := []rejectedBody{
		{name: "本文がJSONでないnot-jsonのとき、400になる", body: []byte("not-json")},
		{name: "本文が空のとき、400になる", body: nil},
	}
	for _, tt := range missingFields {
		rejectedBodies = append(rejectedBodies, rejectedBody{name: tt.given + "、400になる", body: []byte(tt.body)})
	}
	for _, tt := range rejectedBodies {
		t.Run(tt.name, func(t *testing.T) {
			env := routertest.BuildEnv(t, pg)

			response := route.post(env, tt.body)

			assert.Equal(t, http.StatusBadRequest, response.Status)
		})
	}

	unreadableBodies := []rejectedBody{
		{name: "本文がJSONでないnot-jsonのとき、errorに本文がJSONとして読めないことを示す語が含まれる", body: []byte("not-json"), want: "request body is not valid JSON"},
		{name: "本文が空のとき、errorに本文が空であることを示す語が含まれる", body: nil, want: "request body is empty"},
	}
	for _, tt := range unreadableBodies {
		t.Run(tt.name, func(t *testing.T) {
			env := routertest.BuildEnv(t, pg)

			response := route.post(env, tt.body)

			assert.Contains(t, response.ErrorMessage(t), tt.want)
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
				describeRequestBodyReading(t, purchaseRoute{post: func(env *routertest.Env, body []byte) routertest.Response {
					return env.PostPurchase(routertest.PlayerOne, body)
				}})

				t.Run("本文が空白だけのとき、errorに本文が空であることを示す語が含まれる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)

					response := env.PostPurchase(routertest.PlayerOne, []byte("  \n "))

					assert.Contains(t, response.ErrorMessage(t), "request body is empty")
				})

				t.Run("本文がJSONでないnot-jsonのとき、ログのレベルがWarnの警告ログが1件出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					env := routertest.BuildEnv(t, pg)

					env.PostPurchase(routertest.PlayerOne, []byte("not-json"))

					assert.Len(t, logs.SelectByMessageAndLevel("request body is not valid JSON", slog.LevelWarn), 1)
				})

				t.Run("本文がJSONでないnot-jsonのとき、警告ログのpathが商品の購入のパスになる", func(t *testing.T) {
					logs := logtest.Capture(t)
					env := routertest.BuildEnv(t, pg)

					env.PostPurchase(routertest.PlayerOne, []byte("not-json"))

					records := logs.SelectByMessage("request body is not valid JSON")
					require.Len(t, records, 1)
					assert.Equal(t, "/api/v1/shop/purchase", records[0].String("path"))
				})

				t.Run("本文が空のとき、本文がJSONとして読めないことを示す警告ログは出ない", func(t *testing.T) {
					logs := logtest.Capture(t)
					env := routertest.BuildEnv(t, pg)

					env.PostPurchase(routertest.PlayerOne, nil)

					assert.Empty(t, logs.SelectByMessage("request body is not valid JSON"))
				})
			})

			t.Run("サブスクリプションの購入のとき", func(t *testing.T) {
				describeRequestBodyReading(t, purchaseRoute{post: func(env *routertest.Env, body []byte) routertest.Response {
					return env.PostSubscribe(routertest.PlayerOne, body)
				}})
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
