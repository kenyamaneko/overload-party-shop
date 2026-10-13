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
	given        string
	headers      map[string]string
	errorSummary string
	wantError    string
}

var pushHeaderCases = []pushHeaderCase{
	{
		given:        "Authorizationヘッダーが無いとき",
		headers:      nil,
		errorSummary: "ヘッダーが無いことを示す",
		wantError:    "missing authorization header",
	},
	{
		given:        "AuthorizationヘッダーがBearer以外の方式のとき",
		headers:      map[string]string{"Authorization": "Basic TST-ID-TOKEN"},
		errorSummary: "形式が不正であることを示す",
		wantError:    "invalid authorization format",
	},
	{
		given:        "Authorizationヘッダーの方式名が小文字のbearerのとき",
		headers:      map[string]string{"Authorization": "bearer TST-ID-TOKEN"},
		errorSummary: "形式が不正であることを示す",
		wantError:    "invalid authorization format",
	},
	{
		given:        "AuthorizationヘッダーがBearerだけでトークンを区切る空白が無いとき",
		headers:      map[string]string{"Authorization": "Bearer"},
		errorSummary: "形式が不正であることを示す",
		wantError:    "invalid authorization format",
	},
}

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
					t.Run(tt.given+"、401になる", func(t *testing.T) {
						engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildValidValidator(), routertest.PushAudience))

						response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, tt.headers)

						assert.Equal(t, http.StatusUnauthorized, response.Status)
					})

					t.Run(tt.given+"、errorに"+tt.errorSummary+"メッセージが入る", func(t *testing.T) {
						engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildValidValidator(), routertest.PushAudience))

						response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, tt.headers)

						assert.Equal(t, tt.wantError, response.ErrorMessage(t))
					})
				}

				t.Run("AuthorizationヘッダーがBearerと空白だけでトークン部が空のとき、トークンの検証には空文字が渡される", func(t *testing.T) {
					validator := routertest.NewPushValidator(routertest.PushServiceAccountEmail, errors.New("TST-VERIFICATION-FAILURE"))
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, validator, routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, map[string]string{"Authorization": "Bearer "})

					assert.Equal(t, []string{""}, validator.IDTokens())
				})
			})

			t.Run("トークンの検証が失敗するとき", func(t *testing.T) {
				buildFailingValidator := func() *routertest.PushValidator {
					return routertest.NewPushValidator("", errors.New("TST-VERIFICATION-FAILURE"))
				}

				t.Run("トークンの検証が失敗するとき、401になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildFailingValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusUnauthorized, response.Status)
				})

				t.Run("トークンの検証が失敗するとき、errorにトークンが不正であることを示すメッセージが入る", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildFailingValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, "invalid token", response.ErrorMessage(t))
				})

				t.Run("トークンの検証が失敗するとき、ログのレベルがWarnのログが出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildFailingValidator(), routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					authLogs := logs.ExcludeByMessage("http request")
					require.Len(t, authLogs, 1)
					assert.Equal(t, slog.LevelWarn, authLogs[0].Level)
				})

				t.Run("トークンの検証が失敗するとき、ログのerrorにトークンの検証が失敗した原因が含まれる", func(t *testing.T) {
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

				t.Run("署名者のメールアドレスが一致しないとき、401になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildMismatchedValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusUnauthorized, response.Status)
				})

				t.Run("署名者のメールアドレスが一致しないとき、errorに想定外のサービスアカウントであることを示すメッセージが入る", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildMismatchedValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, "unexpected service account", response.ErrorMessage(t))
				})

				t.Run("署名者のメールアドレスが一致しないとき、ログのレベルがWarnのログが出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildMismatchedValidator(), routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					authLogs := logs.ExcludeByMessage("http request")
					require.Len(t, authLogs, 1)
					assert.Equal(t, slog.LevelWarn, authLogs[0].Level)
				})

				t.Run("署名者のメールアドレスが一致しないとき、ログのemailに署名者のメールアドレスが入る", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildMismatchedValidator(), routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					authLogs := logs.ExcludeByMessage("http request")
					require.Len(t, authLogs, 1)
					assert.Equal(t, "tst-other@tst.example.com", authLogs[0].String("email"))
				})
			})

			t.Run("認証に失敗するとき", func(t *testing.T) {
				tests := []struct {
					name      string
					headers   map[string]string
					validator *routertest.PushValidator
				}{
					{
						name:      "Authorizationヘッダーが無いとき、通知処理に通知は渡らない",
						headers:   nil,
						validator: buildValidValidator(),
					},
					{
						name:      "AuthorizationヘッダーがBearer以外の方式のとき、通知処理に通知は渡らない",
						headers:   map[string]string{"Authorization": "Basic TST-ID-TOKEN"},
						validator: buildValidValidator(),
					},
					{
						name:      "Authorizationヘッダーの方式名が小文字のbearerのとき、通知処理に通知は渡らない",
						headers:   map[string]string{"Authorization": "bearer TST-ID-TOKEN"},
						validator: buildValidValidator(),
					},
					{
						name:      "AuthorizationヘッダーがBearerだけでトークンを区切る空白が無いとき、通知処理に通知は渡らない",
						headers:   map[string]string{"Authorization": "Bearer"},
						validator: buildValidValidator(),
					},
					{
						name:      "トークンの検証が失敗するとき、通知処理に通知は渡らない",
						headers:   googleHeaders,
						validator: routertest.NewPushValidator("", errors.New("TST-VERIFICATION-FAILURE")),
					},
					{
						name:      "署名者のメールアドレスが一致しないとき、通知処理に通知は渡らない",
						headers:   googleHeaders,
						validator: routertest.NewPushValidator("tst-other@tst.example.com", nil),
					},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						google := &routertest.GoogleNotifier{}
						engine := routertest.BuildRouter(t, routertest.WithGooglePush(google, tt.validator, routertest.PushAudience))

						routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, tt.headers)

						assert.Empty(t, google.Data())
					})
				}
			})
		})

		t.Run("正常系", func(t *testing.T) {
			t.Run("トークンの検証が成功し、署名者のメールアドレスが想定するサービスアカウントと一致するとき", func(t *testing.T) {
				t.Run("200になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, buildValidValidator(), routertest.PushAudience))

					response := routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("通知処理に、本文のmessage.dataの値が渡る", func(t *testing.T) {
					google := &routertest.GoogleNotifier{}
					engine := routertest.BuildRouter(t, routertest.WithGooglePush(google, buildValidValidator(), routertest.PushAudience))

					routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

					assert.Equal(t, []string{"TST-RTDN-DATA"}, google.Data())
				})
			})

			t.Run("AuthorizationヘッダーがBearerとトークンTST-ID-TOKENのとき、トークンの検証には、Bearerを除いたTST-ID-TOKENが渡される", func(t *testing.T) {
				validator := buildValidValidator()
				engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, validator, routertest.PushAudience))

				routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, map[string]string{"Authorization": "Bearer TST-ID-TOKEN"})

				assert.Equal(t, []string{"TST-ID-TOKEN"}, validator.IDTokens())
			})

			t.Run("プッシュ認証のaudienceにTST-PUSH-AUDIENCEを設定したとき、トークンの検証にTST-PUSH-AUDIENCEが渡される", func(t *testing.T) {
				validator := buildValidValidator()
				engine := routertest.BuildRouter(t, routertest.WithGooglePush(&routertest.GoogleNotifier{}, validator, "TST-PUSH-AUDIENCE"))

				routertest.Send(engine, http.MethodPost, routertest.GoogleWebhookPath, googleBody, googleHeaders)

				assert.Equal(t, []string{"TST-PUSH-AUDIENCE"}, validator.Audiences())
			})
		})
	})
}
