//go:build integration

package rest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

func TestGooglePlayNotificationReceipt(t *testing.T) {
	t.Run("[GooglePlay通知]通知の受信", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("有効期限が過去の契約に更新通知が届き、GooglePlayが返す購読の状態がACTIVEで、有効期限が2999-12-31T00:00:00Zのとき", func(t *testing.T) {
				t.Run("200になる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredGoogleSubscription(t)

					response := env.PostGoogleWebhook(t, routertest.BuildGoogleRenewalData(t, "TST-GOOGLE-TOKEN-1"))

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("通知が届いた後、商品一覧でサブスクリプション商品のis_ownedがtrueになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredGoogleSubscription(t)
					env.PostGoogleWebhook(t, routertest.BuildGoogleRenewalData(t, "TST-GOOGLE-TOKEN-1"))

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
					{name: "本文にmessageが無い", body: []byte("{}")},
					{name: "本文のmessageにdataが無い", body: []byte(`{"message":{}}`)},
				}
				for _, tt := range unreadableBodyTests {
					t.Run(tt.name+"とき、400になる", func(t *testing.T) {
						env := routertest.BuildEnv(t, pg)

						response := env.PostGoogleWebhookBody(tt.body)

						assert.Equal(t, http.StatusBadRequest, response.Status)
					})
				}

				t.Run("本文がJSONでないnot-jsonのとき、errorに本文がJSONとして読めないことを示す語が含まれる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)

					response := env.PostGoogleWebhookBody([]byte("not-json"))

					assert.Contains(t, response.ErrorMessage(t), "request body is not valid JSON")
				})

				t.Run("本文がJSONでないnot-jsonで、有効期限が過去の契約があるとき、商品一覧でサブスクリプション商品のis_ownedはfalseのままになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredGoogleSubscription(t)
					env.PostGoogleWebhookBody([]byte("not-json"))

					response := env.ListProducts(routertest.PlayerOne)

					assert.False(t, response.Product(t, "TST-PRODUCT-SUB-1").IsOwned)
				})

				t.Run("本文にmessageが無いとき、errorに欠落した項目名messageが入る", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)

					response := env.PostGoogleWebhookBody([]byte("{}"))

					assert.Contains(t, response.ErrorMessage(t), "message")
				})

				t.Run("本文のmessageにdataが無いとき、errorに欠落した項目名dataが入る", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)

					response := env.PostGoogleWebhookBody([]byte(`{"message":{}}`))

					assert.Contains(t, response.ErrorMessage(t), "data")
				})
			})

			t.Run("本文を読めず400になった後", func(t *testing.T) {
				buildEnvAfterUnreadableBody := func(t *testing.T) *routertest.Env {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredGoogleSubscription(t)
					env.PostGoogleWebhookBody([]byte("not-json"))
					return env
				}

				t.Run("有効期限が過去の契約に、GooglePlayが返す購読の状態がACTIVEで、有効期限が2999-12-31T00:00:00Zの更新通知をもう一度送ると", func(t *testing.T) {
					t.Run("200になる", func(t *testing.T) {
						env := buildEnvAfterUnreadableBody(t)

						response := env.PostGoogleWebhook(t, routertest.BuildGoogleRenewalData(t, "TST-GOOGLE-TOKEN-1"))

						assert.Equal(t, http.StatusOK, response.Status)
					})

					t.Run("商品一覧でサブスクリプション商品のis_ownedがtrueになる", func(t *testing.T) {
						env := buildEnvAfterUnreadableBody(t)
						env.PostGoogleWebhook(t, routertest.BuildGoogleRenewalData(t, "TST-GOOGLE-TOKEN-1"))

						response := env.ListProducts(routertest.PlayerOne)

						assert.True(t, response.Product(t, "TST-PRODUCT-SUB-1").IsOwned)
					})
				})
			})
		})
	})
}
