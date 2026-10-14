//go:build integration

package rest_test

import (
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

type failedPurchase struct {
	given      string
	wantStatus int
	wantError  string
	errorLabel string
	attempt    func(t *testing.T) routertest.Response
}

func buildPurchaseFailures() []failedPurchase {
	return []failedPurchase{
		{
			given: "存在しない商品IDのとき", wantStatus: http.StatusNotFound, wantError: "not found", errorLabel: "商品が無いことを示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				return env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-UNKNOWN", "ios", "TST-PURCHASE-TOKEN-1")
			},
		},
		{
			given: "すでに所有している商品のとき", wantStatus: http.StatusConflict, wantError: "product already owned", errorLabel: "所有済みを示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				env.SeedItemProduct(t)
				env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")
				return env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-2")
			},
		},
		{
			given: "販売停止中の商品のとき", wantStatus: http.StatusBadRequest, wantError: "product is not active", errorLabel: "販売停止中を示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				env.SeedInactiveProduct(t)
				return env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-INACTIVE", "ios", "TST-PURCHASE-TOKEN-1")
			},
		},
		{
			given: "購入に対応しない種別のサブスクリプション商品のとき", wantStatus: http.StatusBadRequest, wantError: "unsupported product type for purchase", errorLabel: "種別が非対応であることを示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				env.SeedSubscriptionProduct(t)
				return env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")
			},
		},
		{
			given: "platformがwindowsのとき", wantStatus: http.StatusBadRequest, wantError: "unsupported platform", errorLabel: "platformが非対応であることを示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				env.SeedItemProduct(t)
				return env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "windows", "TST-PURCHASE-TOKEN-1")
			},
		},
		{
			given: "レシート検証が無効と判定するとき", wantStatus: http.StatusPaymentRequired, wantError: "receipt verification failed", errorLabel: "レシート検証の失敗を示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				env.SeedItemProduct(t)
				env.AppleReceipt.RejectPurchase()
				return env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")
			},
		},
		{
			given: "platformがiosでAppleのレシート検証が通信失敗で実行できないとき", wantStatus: http.StatusInternalServerError, wantError: "verify receipt", errorLabel: "レシート検証を実行できなかったことを示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				env.SeedItemProduct(t)
				env.AppleReceipt.FailPurchase(errors.New("TST-NETWORK-FAILURE"))
				return env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")
			},
		},
	}
}

func buildSubscriptionFailures() []failedPurchase {
	return []failedPurchase{
		{
			given: "サブスクリプションでない商品のとき", wantStatus: http.StatusBadRequest, wantError: "product is not a subscription", errorLabel: "サブスクリプションでないことを示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				env.SeedItemProduct(t)
				return env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")
			},
		},
		{
			given: "サブスクリプション検証が無効と判定するとき", wantStatus: http.StatusPaymentRequired, wantError: "subscription verification failed", errorLabel: "サブスクリプション検証の失敗を示す",
			attempt: func(t *testing.T) routertest.Response {
				env := routertest.BuildEnv(t, pg)
				env.SeedSubscriptionProduct(t)
				env.AppleReceipt.RejectSubscription()
				return env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")
			},
		},
	}
}

func describeFailedPurchases(t *testing.T, failures []failedPurchase) {
	for _, tt := range failures {
		t.Run(tt.given+"、"+strconv.Itoa(tt.wantStatus)+"になる", func(t *testing.T) {
			response := tt.attempt(t)

			assert.Equal(t, tt.wantStatus, response.Status)
		})

		t.Run(tt.given+"、errorに"+tt.errorLabel+"メッセージが入る", func(t *testing.T) {
			response := tt.attempt(t)

			assert.Contains(t, response.ErrorMessage(t), tt.wantError)
		})
	}
}

func TestShopAPIErrorResponse(t *testing.T) {
	t.Run("[ショップAPI]エラー応答", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("商品の購入が失敗するとき", func(t *testing.T) {
				describeFailedPurchases(t, buildPurchaseFailures())
			})

			t.Run("サブスクリプションの購入が失敗するとき", func(t *testing.T) {
				describeFailedPurchases(t, buildSubscriptionFailures())
			})

			t.Run("商品一覧の取得中にデータベースを使えないとき、500になる", func(t *testing.T) {
				env := routertest.BuildEnv(t, pg)
				env.UseClosedPool(t)

				response := env.ListProducts(routertest.PlayerOne)

				assert.Equal(t, http.StatusInternalServerError, response.Status)
			})

			t.Run("商品一覧の取得中にデータベースを使えないとき、errorに商品の取得に失敗したことを示すメッセージが入る", func(t *testing.T) {
				env := routertest.BuildEnv(t, pg)
				env.UseClosedPool(t)

				response := env.ListProducts(routertest.PlayerOne)

				assert.Contains(t, response.ErrorMessage(t), "get products")
			})

			t.Run("レシート検証が無効で商品の購入が402になった後", func(t *testing.T) {
				buildEnvAfterRejectedPurchase := func(t *testing.T) *routertest.Env {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)
					env.AppleReceipt.RejectPurchase()
					env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")
					return env
				}

				t.Run("同じプレイヤーの商品一覧で、その商品のis_ownedはfalseのままになる", func(t *testing.T) {
					env := buildEnvAfterRejectedPurchase(t)

					response := env.ListProducts(routertest.PlayerOne)

					assert.False(t, response.Product(t, "TST-PRODUCT-ITEM-1").IsOwned)
				})

				t.Run("有効なレシートで同じ購入をもう一度送ると", func(t *testing.T) {
					t.Run("200になる", func(t *testing.T) {
						env := buildEnvAfterRejectedPurchase(t)
						env.AppleReceipt.AcceptPurchase()

						response := env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

						assert.Equal(t, http.StatusOK, response.Status)
					})

					t.Run("messageが購入の受付を示す値になる", func(t *testing.T) {
						env := buildEnvAfterRejectedPurchase(t)
						env.AppleReceipt.AcceptPurchase()

						response := env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

						assert.Equal(t, "purchase accepted", response.PurchaseBody(t).Message)
					})
				})
			})

			t.Run("platformがiosでAppleのレシート検証が通信失敗で商品の購入が500になった後", func(t *testing.T) {
				buildEnvAfterAppleCommunicationFailure := func(t *testing.T) *routertest.Env {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)
					env.AppleReceipt.FailPurchase(errors.New("TST-NETWORK-FAILURE"))
					env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")
					return env
				}

				t.Run("Appleとの通信が回復してから同じ購入をもう一度送ると", func(t *testing.T) {
					t.Run("200になる", func(t *testing.T) {
						env := buildEnvAfterAppleCommunicationFailure(t)
						env.AppleReceipt.AcceptPurchase()

						response := env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

						assert.Equal(t, http.StatusOK, response.Status)
					})

					t.Run("messageが購入の受付を示す値になる", func(t *testing.T) {
						env := buildEnvAfterAppleCommunicationFailure(t)
						env.AppleReceipt.AcceptPurchase()

						response := env.Purchase(t, routertest.PlayerOne, "TST-PRODUCT-ITEM-1", "ios", "TST-PURCHASE-TOKEN-1")

						assert.Equal(t, "purchase accepted", response.PurchaseBody(t).Message)
					})
				})
			})

			t.Run("データベースを使えず商品一覧の取得が500になった後", func(t *testing.T) {
				buildEnvAfterUnavailableListing := func(t *testing.T) *routertest.Env {
					env := routertest.BuildEnv(t, pg)
					env.SeedItemProduct(t)
					env.UseClosedPool(t)
					env.ListProducts(routertest.PlayerOne)
					return env
				}

				t.Run("データベースを使えるようになってから同じ取得をもう一度送ると", func(t *testing.T) {
					t.Run("200になる", func(t *testing.T) {
						env := buildEnvAfterUnavailableListing(t)
						env.UseOpenPool(t)

						response := env.ListProducts(routertest.PlayerOne)

						assert.Equal(t, http.StatusOK, response.Status)
					})

					t.Run("productsに販売中の商品が入る", func(t *testing.T) {
						env := buildEnvAfterUnavailableListing(t)
						env.UseOpenPool(t)

						response := env.ListProducts(routertest.PlayerOne)

						assert.Equal(t, []string{"TST-PRODUCT-ITEM-1"}, routertest.ExtractProductIDs(response.Products(t)))
					})
				})
			})
		})
	})
}
