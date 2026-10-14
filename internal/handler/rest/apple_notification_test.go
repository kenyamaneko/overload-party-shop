//go:build integration

package rest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

func TestAppleNotificationReceipt(t *testing.T) {
	t.Run("[Apple通知]通知の受信", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("有効期限が過去の契約に、更新通知が未来の有効期限2999-12-31T00:00:00Zで届くとき", func(t *testing.T) {
				t.Run("200になる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredAppleSubscription(t)
					env.RegisterAppleRenewal(t, routertest.FarFutureExpiry)

					response := env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("通知が届いた後、商品一覧でサブスクリプション商品のis_ownedがtrueになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredAppleSubscription(t)
					env.RegisterAppleRenewal(t, routertest.FarFutureExpiry)
					env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")

					response := env.ListProducts(routertest.PlayerOne)

					assert.True(t, response.Product(t, "TST-PRODUCT-SUB-1").IsOwned)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("本文を読めない通知を受信すると", func(t *testing.T) {
				unreadableBodyTests := []struct {
					name string
					body []byte
				}{
					{name: "本文がJSONでないnot-jsonの", body: []byte("not-json")},
					{name: "本文にsignedPayloadが無い", body: []byte("{}")},
				}
				for _, tt := range unreadableBodyTests {
					t.Run(tt.name+"とき、400になる", func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)

						response := env.PostAppleWebhookBody(tt.body)

						assert.Equal(t, http.StatusBadRequest, response.Status)
					})
				}

				t.Run("本文がJSONでないnot-jsonのとき、errorに本文がJSONとして読めないことを示す語が含まれる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)

					response := env.PostAppleWebhookBody([]byte("not-json"))

					assert.Contains(t, response.ErrorMessage(t), "request body is not valid JSON")
				})

				t.Run("本文がJSONでないnot-jsonで、有効期限が過去の契約があるとき、商品一覧でサブスクリプション商品のis_ownedはfalseのままになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredAppleSubscription(t)
					env.RegisterAppleRenewal(t, routertest.FarFutureExpiry)
					env.PostAppleWebhookBody([]byte("not-json"))

					response := env.ListProducts(routertest.PlayerOne)

					assert.False(t, response.Product(t, "TST-PRODUCT-SUB-1").IsOwned)
				})

				t.Run("本文にsignedPayloadが無いとき、errorに欠落した項目名signedPayloadが入る", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)

					response := env.PostAppleWebhookBody([]byte("{}"))

					assert.Contains(t, response.ErrorMessage(t), "signedPayload")
				})
			})

			t.Run("本文を読めず400になった後", func(t *testing.T) {
				buildEnvAfterUnreadableBody := func(t *testing.T) *routertest.Env {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredAppleSubscription(t)
					env.RegisterAppleRenewal(t, routertest.FarFutureExpiry)
					env.PostAppleWebhookBody([]byte("not-json"))
					return env
				}

				t.Run("有効期限が過去の契約に、更新通知が未来の有効期限2999-12-31T00:00:00Zで届くと", func(t *testing.T) {
					t.Run("200になる", func(t *testing.T) {
						env := buildEnvAfterUnreadableBody(t)

						response := env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")

						assert.Equal(t, http.StatusOK, response.Status)
					})

					t.Run("商品一覧でサブスクリプション商品のis_ownedがtrueになる", func(t *testing.T) {
						env := buildEnvAfterUnreadableBody(t)
						env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")

						response := env.ListProducts(routertest.PlayerOne)

						assert.True(t, response.Product(t, "TST-PRODUCT-SUB-1").IsOwned)
					})
				})
			})
		})
	})
}
