package router_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

func TestWebhookRouteRegistration(t *testing.T) {
	appleBody := routertest.BuildAppleWebhookBody(t, "TST-SIGNED-PAYLOAD")
	googleBody := routertest.BuildGoogleWebhookBody(t, "TST-RTDN-DATA")
	googleHeaders := routertest.BuildGooglePushHeaders()
	validPush := func() *routertest.PushValidator {
		return routertest.NewPushValidator(routertest.PushServiceAccountEmail, nil)
	}

	t.Run("[ストア通知]webhookルートの登録", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("Apple通知の受付だけが有効なとき", func(t *testing.T) {
				t.Run("/webhook/appleへのPOSTは200になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithApple(&routertest.AppleNotifier{}))

					response := routertest.Send(engine, http.MethodPost, routertest.AppleWebhookPath, appleBody, nil)

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("/webhook/googleへのPOSTは404になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithApple(&routertest.AppleNotifier{}))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusNotFound, response.Status)
				})
			})

			t.Run("GooglePlay通知の受付だけが有効なとき", func(t *testing.T) {
				t.Run("プッシュ認証を通るとき、/webhook/googleへのPOSTは200になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, validPush(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("/webhook/appleへのPOSTは404になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, validPush(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.AppleWebhookPath, appleBody, nil)

					assert.Equal(t, http.StatusNotFound, response.Status)
				})
			})

			t.Run("Apple通知もGooglePlay通知も受付が有効でないとき", func(t *testing.T) {
				t.Run("/webhook/appleへのPOSTは404になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t)

					response := routertest.Send(engine, http.MethodPost, routertest.AppleWebhookPath, appleBody, nil)

					assert.Equal(t, http.StatusNotFound, response.Status)
				})

				t.Run("/webhook/googleへのPOSTは404になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t)

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusNotFound, response.Status)
				})
			})

			t.Run("Apple通知もGooglePlay通知も受付が有効で、内部認証の検証が常に失敗するとき", func(t *testing.T) {
				buildBothEnabledWithFailingAuth := func() []routertest.RouterOption {
					return []routertest.RouterOption{
						routertest.WithApple(&routertest.AppleNotifier{}),
						routertest.WithGooglePush(&routertest.GoogleNotifier{}, validPush(), routertest.PushAudience),
						routertest.WithAuth(routertest.NewFailingAuthVerifier(errors.New("TST-VERIFICATION-FAILURE"))),
					}
				}

				t.Run("X-Internal-Authヘッダーが無いとき、/webhook/appleへのPOSTは200になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, buildBothEnabledWithFailingAuth()...)

					response := routertest.Send(engine, http.MethodPost, routertest.AppleWebhookPath, appleBody, nil)

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("X-Internal-Authヘッダーが無く、プッシュ認証を通るとき、/webhook/googleへのPOSTは200になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, buildBothEnabledWithFailingAuth()...)

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusOK, response.Status)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("GooglePlay通知の受付が有効で、プッシュ認証の検証が用意されていないとき", func(t *testing.T) {
				t.Run("webhookのルートを登録すると、エラーの内容にGooglePlay通知のプッシュ認証の検証が無いことを示す語が含まれる", func(t *testing.T) {
					_, err := routertest.NewRouter(routertest.WithGoogle(&routertest.GoogleNotifier{}))

					require.Error(t, err)
					assert.Contains(t, err.Error(), "pushValidator is required")
				})
			})
		})
	})
}
