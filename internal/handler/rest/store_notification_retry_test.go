//go:build integration

package rest_test

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

type undeliverableNotification struct {
	given       string
	causeLabel  string
	wantSource  string
	wantMessage string
	send        func(t *testing.T, env *routertest.Env) routertest.Response
}

var (
	appleSignatureUnverifiable = undeliverableNotification{
		given: "Apple通知の署名を検証できず200にしたとき", causeLabel: "署名の検証不可を示す語",
		wantSource: "apple", wantMessage: "decode notification",
		send: func(t *testing.T, env *routertest.Env) routertest.Response {
			return env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")
		},
	}
	appleTransactionUnverifiable = undeliverableNotification{
		given: "Apple通知の取引情報を検証できず200にしたとき", causeLabel: "取引情報の検証不可を示す語",
		wantSource: "apple", wantMessage: "decode transaction info",
		send: func(t *testing.T, env *routertest.Env) routertest.Response {
			env.AppleJWS.RegisterRenewalNotification(t, "TST-SIGNED-PAYLOAD", "TST-SIGNED-TRANSACTION-INFO")
			return env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")
		},
	}
	appleSubscriptionMissing = undeliverableNotification{
		given: "Apple通知に対応するサブスクリプションが無く200にしたとき", causeLabel: "サブスクリプションが無いことを示す語",
		wantSource: "apple", wantMessage: "subscription not found",
		send: func(t *testing.T, env *routertest.Env) routertest.Response {
			env.RegisterAppleRenewal(t, routertest.FarFutureExpiry)
			return env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")
		},
	}
	googleDataNotBase64 = undeliverableNotification{
		given: "GooglePlay通知のmessage.dataがbase64として読めず200にしたとき", causeLabel: "base64として読めないことを示す語",
		wantSource: "google", wantMessage: "decode RTDN data",
		send: func(t *testing.T, env *routertest.Env) routertest.Response {
			return env.PostGoogleWebhook(t, "!!!TST-NOT-BASE64!!!")
		},
	}
	googleDataNotJSON = undeliverableNotification{
		given: "GooglePlay通知のmessage.dataがJSONとして読めず200にしたとき", causeLabel: "JSONとして読めないことを示す語",
		wantSource: "google", wantMessage: "unmarshal RTDN data",
		send: func(t *testing.T, env *routertest.Env) routertest.Response {
			return env.PostGoogleWebhook(t, routertest.EncodeBase64("not-json"))
		},
	}
	googleSubscriptionMissing = undeliverableNotification{
		given: "GooglePlay通知に対応するサブスクリプションが無く200にしたとき", causeLabel: "サブスクリプションが無いことを示す語",
		wantSource: "google", wantMessage: "subscription not found",
		send: func(t *testing.T, env *routertest.Env) routertest.Response {
			return env.PostGoogleWebhook(t, routertest.BuildGoogleRenewalData(t, "TST-GOOGLE-TOKEN-1"))
		},
	}
)

func selectWarnLogs(logs *logtest.Logs) []logtest.Record {
	return logs.SelectByLevel(slog.LevelWarn)
}

type deterministicFailure struct {
	name         string
	notification undeliverableNotification
}

func TestStoreNotificationRetryDecision(t *testing.T) {
	t.Run("[ストア通知]再送要否の判定", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("再送しても結果が変わらない失敗のとき", func(t *testing.T) {
				t.Run("Apple通知を受信するとき", func(t *testing.T) {
					tests := []deterministicFailure{
						{name: "通知本文の署名を検証できないとき、200になる", notification: appleSignatureUnverifiable},
						{name: "通知本文の署名は検証できるが、通知内の取引情報を検証できないとき、200になる", notification: appleTransactionUnverifiable},
						{name: "通知に対応するサブスクリプションが無いとき、200になる", notification: appleSubscriptionMissing},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							env := routertest.BuildEnv(t, pg)

							response := tt.notification.send(t, env)

							assert.Equal(t, http.StatusOK, response.Status)
						})
					}
				})

				t.Run("GooglePlay通知を受信するとき", func(t *testing.T) {
					tests := []deterministicFailure{
						{name: "message.dataがbase64として読めないとき、200になる", notification: googleDataNotBase64},
						{name: "message.dataのbase64を戻した内容がJSONとして読めないとき、200になる", notification: googleDataNotJSON},
						{name: "通知に対応するサブスクリプションが無いとき、200になる", notification: googleSubscriptionMissing},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							env := routertest.BuildEnv(t, pg)

							response := tt.notification.send(t, env)

							assert.Equal(t, http.StatusOK, response.Status)
						})
					}
				})

				t.Run("200にするときのログのレベルと通知元", func(t *testing.T) {
					notifications := []undeliverableNotification{appleSignatureUnverifiable, googleDataNotBase64}

					for _, tt := range notifications {
						t.Run(tt.given+"、ログのレベルがWarnのログが1件出る", func(t *testing.T) {
							logs := logtest.Capture(t)
							env := routertest.BuildEnv(t, pg)

							tt.send(t, env)

							assert.Len(t, selectWarnLogs(logs), 1)
						})
					}

					for _, tt := range notifications {
						t.Run(tt.given+"、ログの通知元が"+tt.wantSource+"になる", func(t *testing.T) {
							logs := logtest.Capture(t)
							env := routertest.BuildEnv(t, pg)

							tt.send(t, env)

							records := selectWarnLogs(logs)
							require.Len(t, records, 1)
							assert.Equal(t, tt.wantSource, records[0].String("source"))
						})
					}
				})

				t.Run("200にするときのログに含まれる原因", func(t *testing.T) {
					causes := []undeliverableNotification{
						appleSignatureUnverifiable, appleTransactionUnverifiable, appleSubscriptionMissing,
						googleDataNotBase64, googleDataNotJSON, googleSubscriptionMissing,
					}
					for _, tt := range causes {
						t.Run(tt.given+"、ログのerrorに"+tt.causeLabel+"が含まれる", func(t *testing.T) {
							logs := logtest.Capture(t)
							env := routertest.BuildEnv(t, pg)

							tt.send(t, env)

							records := selectWarnLogs(logs)
							require.Len(t, records, 1)
							assert.Contains(t, records[0].String("error"), tt.wantMessage)
						})
					}
				})
			})

			t.Run("再送すれば結果が変わりうる失敗のとき", func(t *testing.T) {
				sendAppleWhileDatabaseUnavailable := func(t *testing.T) routertest.Response {
					env := routertest.BuildEnv(t, pg)
					env.RegisterAppleRenewal(t, routertest.FarFutureExpiry)
					env.UseClosedPool(t)
					return env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")
				}
				sendGoogleWhileDatabaseUnavailable := func(t *testing.T) routertest.Response {
					env := routertest.BuildEnv(t, pg)
					env.UseClosedPool(t)
					return env.PostGoogleWebhook(t, routertest.BuildGoogleRenewalData(t, "TST-GOOGLE-TOKEN-1"))
				}
				sendGoogleWhileExpiryUnavailable := func(t *testing.T) routertest.Response {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredGoogleSubscription(t)
					env.GoogleExpiry.FailWith(errors.New("TST-PLAY-FAILURE"))
					return env.PostGoogleWebhook(t, routertest.BuildGoogleRenewalData(t, "TST-GOOGLE-TOKEN-1"))
				}

				t.Run("Apple通知の受信中にデータベースを使えないとき、500になる", func(t *testing.T) {
					response := sendAppleWhileDatabaseUnavailable(t)

					assert.Equal(t, http.StatusInternalServerError, response.Status)
				})

				t.Run("GooglePlay通知の受信中にデータベースを使えないとき、500になる", func(t *testing.T) {
					response := sendGoogleWhileDatabaseUnavailable(t)

					assert.Equal(t, http.StatusInternalServerError, response.Status)
				})

				t.Run("GooglePlay通知の更新通知で、GooglePlayから有効期限を取得できないとき、500になる", func(t *testing.T) {
					response := sendGoogleWhileExpiryUnavailable(t)

					assert.Equal(t, http.StatusInternalServerError, response.Status)
				})

				t.Run("Apple通知の受信中にデータベースを使えないとき、errorにサブスクリプションの検索に失敗したことを示す語が含まれる", func(t *testing.T) {
					response := sendAppleWhileDatabaseUnavailable(t)

					assert.Contains(t, response.ErrorMessage(t), "find subscription")
				})

				t.Run("GooglePlay通知の受信中にデータベースを使えないとき、errorにサブスクリプションの検索に失敗したことを示す語が含まれる", func(t *testing.T) {
					response := sendGoogleWhileDatabaseUnavailable(t)

					assert.Contains(t, response.ErrorMessage(t), "find subscription")
				})

				t.Run("GooglePlay通知の更新通知で、GooglePlayから有効期限を取得できないとき、errorに有効期限の取得に失敗したことを示す語が含まれる", func(t *testing.T) {
					response := sendGoogleWhileExpiryUnavailable(t)

					assert.Contains(t, response.ErrorMessage(t), "get subscription expiry from Google")
				})
			})

			t.Run("データベースを使えず500になった後", func(t *testing.T) {
				buildEnvAfterAppleDatabaseFailure := func(t *testing.T) *routertest.Env {
					env := routertest.BuildEnv(t, pg)
					env.CreateExpiredAppleSubscription(t)
					env.RegisterAppleRenewal(t, routertest.FarFutureExpiry)
					env.UseClosedPool(t)
					env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")
					return env
				}

				t.Run("データベースを使えるようになってから同じApple通知をもう一度送ると、200になる", func(t *testing.T) {
					env := buildEnvAfterAppleDatabaseFailure(t)
					env.UseOpenPool(t)

					response := env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("データベースを使えるようになってから同じApple通知をもう一度送ると、商品一覧でサブスクリプション商品のis_ownedがtrueになる", func(t *testing.T) {
					env := buildEnvAfterAppleDatabaseFailure(t)
					env.UseOpenPool(t)
					env.PostAppleWebhook(t, "TST-SIGNED-PAYLOAD")

					response := env.ListProducts(routertest.PlayerOne)

					assert.True(t, response.Product(t, "TST-PRODUCT-SUB-1").IsOwned)
				})
			})
		})
	})
}
