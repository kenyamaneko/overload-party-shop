package router_test

import (
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

type pushHeaderCase struct {
	given   string
	headers map[string]string
}

var pushHeaderMissingCase = pushHeaderCase{given: "無い", headers: nil}

var pushHeaderMalformedCases = []pushHeaderCase{
	{given: "Bearer以外の方式の", headers: map[string]string{"Authorization": "Basic TST-ID-TOKEN"}},
	{given: "方式名が小文字のbearerの", headers: map[string]string{"Authorization": "bearer TST-ID-TOKEN"}},
	{given: "Bearerだけでトークンを区切る空白が無い", headers: map[string]string{"Authorization": "Bearer"}},
}

var pushHeaderCases = append([]pushHeaderCase{pushHeaderMissingCase}, pushHeaderMalformedCases...)

func TestPubSubPushAuthentication(t *testing.T) {
	googleBody := routertest.BuildGoogleWebhookBody(t, "TST-RTDN-DATA")
	googleHeaders := routertest.BuildGooglePushHeaders()
	buildValidValidator := func() *routertest.PushValidator {
		return routertest.NewPushValidator(routertest.PushServiceAccountEmail, nil)
	}

	t.Run("[GooglePlay通知]PubSubプッシュ認証", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("Authorizationヘッダーに問題があるとき", func(t *testing.T) {
				for _, tt := range pushHeaderCases {
					t.Run(tt.given+"とき、401になる", func(t *testing.T) {
						engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildValidValidator(), routertest.PushAudience))

						response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, tt.headers)

						assert.Equal(t, http.StatusUnauthorized, response.Status)
					})
				}

				t.Run("Authorizationヘッダーが無いとき、errorにヘッダーが無いことを示すメッセージが入る", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildValidValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, pushHeaderMissingCase.headers)

					assert.Equal(t, "missing authorization header", response.ErrorMessage(t))
				})

				for _, tt := range pushHeaderMalformedCases {
					t.Run(tt.given+"とき、errorに形式が不正であることを示すメッセージが入る", func(t *testing.T) {
						engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildValidValidator(), routertest.PushAudience))

						response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, tt.headers)

						assert.Equal(t, "invalid authorization format", response.ErrorMessage(t))
					})
				}
			})

			t.Run("トークンの検証が失敗するとき", func(t *testing.T) {
				buildFailingValidator := func() *routertest.PushValidator {
					return routertest.NewPushValidator("", errors.New("TST-VERIFICATION-FAILURE"))
				}

				t.Run("401になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildFailingValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusUnauthorized, response.Status)
				})

				t.Run("errorにトークンが不正であることを示すメッセージが入る", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildFailingValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, "invalid token", response.ErrorMessage(t))
				})

				t.Run("ログのレベルがWarnのログが出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildFailingValidator(), routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					authLogs := logs.ExcludeByMessage("http request")
					require.Len(t, authLogs, 1)
					assert.Equal(t, slog.LevelWarn, authLogs[0].Level)
				})

				t.Run("ログのerrorにトークンの検証が失敗した原因が含まれる", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildFailingValidator(), routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					authLogs := logs.ExcludeByMessage("http request")
					require.Len(t, authLogs, 1)
					assert.Contains(t, authLogs[0].String("error"), "TST-VERIFICATION-FAILURE")
				})
			})

			t.Run("署名者のメールアドレスが想定するプッシュ用サービスアカウントと一致しないとき", func(t *testing.T) {
				buildMismatchedValidator := func() *routertest.PushValidator {
					return routertest.NewPushValidator("tst-other@tst.example.com", nil)
				}

				t.Run("401になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildMismatchedValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusUnauthorized, response.Status)
				})

				t.Run("errorに想定外のサービスアカウントであることを示すメッセージが入る", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildMismatchedValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, "unexpected service account", response.ErrorMessage(t))
				})

				t.Run("ログのレベルがWarnのログが出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildMismatchedValidator(), routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					authLogs := logs.ExcludeByMessage("http request")
					require.Len(t, authLogs, 1)
					assert.Equal(t, slog.LevelWarn, authLogs[0].Level)
				})

				t.Run("ログのemailに署名者のメールアドレスが入る", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildMismatchedValidator(), routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					authLogs := logs.ExcludeByMessage("http request")
					require.Len(t, authLogs, 1)
					assert.Equal(t, "tst-other@tst.example.com", authLogs[0].String("email"))
				})
			})
		})

		t.Run("正常系", func(t *testing.T) {
			t.Run("トークンの検証が成功し、署名者のメールアドレスが想定するサービスアカウントと一致するとき、200になる", func(t *testing.T) {
				engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildValidValidator(), routertest.PushAudience))

				response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

				assert.Equal(t, http.StatusOK, response.Status)
			})
		})
	})
}
