//go:build integration

package subscription_test

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription/subscriptiontest"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

const (
	googleSubscriptionRecovered = 1
	googleSubscriptionRenewed   = 2
	googleSubscriptionCanceled  = 3
	googleSubscriptionPurchased = 4
	googleSubscriptionRevoked   = 12
	googleSubscriptionExpired   = 13
	googleUndefinedType         = 99

	googleUnhandledTypeMessage = "google RTDN unhandled notification type"
	googleSkippedMessage       = "google RTDN skipped: no subscriptionNotification"
)

var errPlayDeveloperAPI = errors.New("TST-play-developer-api-failed")

type googleEnv struct {
	notifier *subscription.GoogleNotifier
	fetcher  *subscriptiontest.GoogleExpiryFetcher
}

func newGoogleEnv(t *testing.T) googleEnv {
	t.Helper()
	newShop(t)
	fetcher := subscriptiontest.NewGoogleExpiryFetcher()
	fetcher.ReturnExpiry(renewedExpiresAt)
	return googleEnv{notifier: subscriptiontest.NewGoogleNotifier(pg.Pool, fetcher), fetcher: fetcher}
}

func newActiveGoogleContract(t *testing.T) googleEnv {
	t.Helper()
	env := newGoogleEnv(t)
	registerContract(t, platformAndroid, googleToken)
	return env
}

func newExpiredGoogleContract(t *testing.T) googleEnv {
	t.Helper()
	env := newActiveGoogleContract(t)
	require.NoError(t, env.receiveType(t, googleSubscriptionExpired).err)
	return env
}

func (e googleEnv) receiveType(t *testing.T, notificationType int) outcome {
	t.Helper()
	return e.receive(t, subscriptiontest.BuildSubscriptionNotificationMessage(t, notificationType, googleToken))
}

func (e googleEnv) receive(t *testing.T, message subscription.GoogleRTDNMessage) outcome {
	t.Helper()
	return process(t, platformAndroid, googleToken, func() error {
		return e.notifier.HandleNotification(t.Context(), message)
	})
}

func receiveOnActiveGoogleContract(t *testing.T, notificationType int) outcome {
	t.Helper()
	received := newActiveGoogleContract(t).receiveType(t, notificationType)
	require.NoError(t, received.err)
	return received
}

func receiveOnExpiredGoogleContract(t *testing.T, notificationType int) outcome {
	t.Helper()
	received := newExpiredGoogleContract(t).receiveType(t, notificationType)
	require.NoError(t, received.err)
	return received
}

func receiveUnhandledGoogleType(t *testing.T, notificationType int) (outcome, *logtest.Logs) {
	t.Helper()
	logs := logtest.Capture(t)
	return newActiveGoogleContract(t).receiveType(t, notificationType), logs
}

func receiveUnreadableRenewal(t *testing.T, notificationType int) outcome {
	t.Helper()
	env := newExpiredGoogleContract(t)
	env.fetcher.FailWith(errPlayDeveloperAPI)
	return env.receiveType(t, notificationType)
}

func receiveRenewalAfterFetchFailure(t *testing.T) outcome {
	t.Helper()
	env := newExpiredGoogleContract(t)
	env.fetcher.FailWith(errPlayDeveloperAPI)
	failed := env.receiveType(t, googleSubscriptionRenewed)
	require.ErrorIs(t, failed.err, errPlayDeveloperAPI)
	env.fetcher.ReturnExpiry(renewedExpiresAt)
	retried := env.receiveType(t, googleSubscriptionRenewed)
	require.NoError(t, retried.err)
	return retried
}

type googleTypeCase struct {
	name             string
	notificationType int
}

var (
	renewalTypeCases = []googleTypeCase{
		{"通知種別が2(SUBSCRIPTION_RENEWED)のとき", googleSubscriptionRenewed},
		{"通知種別が1(SUBSCRIPTION_RECOVERED)のとき", googleSubscriptionRecovered},
	}
	endingTypeCases = []googleTypeCase{
		{"通知種別が13(SUBSCRIPTION_EXPIRED)のとき", googleSubscriptionExpired},
		{"通知種別が12(SUBSCRIPTION_REVOKED)のとき", googleSubscriptionRevoked},
	}
)

type unreadableDataCase struct {
	name    string
	message subscription.GoogleRTDNMessage
}

var unreadableDataCases = []unreadableDataCase{
	{"通知データがbase64として読めないとき", subscriptiontest.BuildRawMessage("TST-!!!-NOT-BASE64")},
	{"通知データがJSONとして読めないとき", subscriptiontest.BuildEncodedMessage("TST-NOT-JSON")},
}

func TestGoogleNotifier(t *testing.T) {
	t.Run("[サブスクリプション]Google通知の処理", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("契約状態がexpiredの契約に、更新または復旧の通知を受け取ったとき", func(t *testing.T) {
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、契約状態がactiveになる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt.notificationType)

						assert.Equal(t, "active", o.findContract(t).Status)
					})
				}
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、課金期間の終了日時が有効期限になる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt.notificationType)

						assert.WithinDuration(t, renewedExpiresAt, o.findContract(t).CurrentPeriodEnd, 0)
					})
				}
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、アウトボックスにpremium-updatedが1件加わる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt.notificationType)

						assert.Len(t, o.listNewPremiumEvents(t), 1)
					})
				}
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、アウトボックスに加わったpremium-updatedのis_premiumがtrueになる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt.notificationType)

						assert.Equal(t, true, o.findSinglePremiumEvent(t).Payload["is_premium"])
					})
				}
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、アウトボックスに加わったpremium-updatedのpremium_expires_atが有効期限になる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt.notificationType)

						assert.WithinDuration(t, renewedExpiresAt, o.findSinglePremiumEvent(t).TimeField(t, "premium_expires_at"), 0)
					})
				}
			})
			t.Run("契約状態がactiveの契約に、期限切れまたは取り消しの通知を受け取ったとき", func(t *testing.T) {
				t.Run("通知種別が13(SUBSCRIPTION_EXPIRED)のとき、契約状態がexpiredになる", func(t *testing.T) {
					o := receiveOnActiveGoogleContract(t, googleSubscriptionExpired)

					assert.Equal(t, "expired", o.findContract(t).Status)
				})
				t.Run("通知種別が12(SUBSCRIPTION_REVOKED)のとき、契約状態がrevokedになる", func(t *testing.T) {
					o := receiveOnActiveGoogleContract(t, googleSubscriptionRevoked)

					assert.Equal(t, "revoked", o.findContract(t).Status)
				})
				for _, tt := range endingTypeCases {
					t.Run(tt.name+"、アウトボックスにpremium-updatedが1件加わる", func(t *testing.T) {
						o := receiveOnActiveGoogleContract(t, tt.notificationType)

						assert.Len(t, o.listNewPremiumEvents(t), 1)
					})
				}
				for _, tt := range endingTypeCases {
					t.Run(tt.name+"、アウトボックスに加わったpremium-updatedのis_premiumがfalseになる", func(t *testing.T) {
						o := receiveOnActiveGoogleContract(t, tt.notificationType)

						assert.Equal(t, false, o.findSinglePremiumEvent(t).Payload["is_premium"])
					})
				}
				for _, tt := range endingTypeCases {
					t.Run(tt.name+"、アウトボックスに加わったpremium-updatedにpremium_expires_atが含まれない", func(t *testing.T) {
						o := receiveOnActiveGoogleContract(t, tt.notificationType)

						assert.NotContains(t, o.findSinglePremiumEvent(t).Payload, "premium_expires_at")
					})
				}
			})
			t.Run("契約状態がactiveの契約に、通知種別が3(SUBSCRIPTION_CANCELED)の通知を受け取ったとき", func(t *testing.T) {
				t.Run("契約状態がcancelledになる", func(t *testing.T) {
					o := receiveOnActiveGoogleContract(t, googleSubscriptionCanceled)

					assert.Equal(t, "cancelled", o.findContract(t).Status)
				})
				t.Run("課金期間の終了日時が通知の前と同じになる", func(t *testing.T) {
					o := receiveOnActiveGoogleContract(t, googleSubscriptionCanceled)

					assert.WithinDuration(t, o.before.CurrentPeriodEnd, o.findContract(t).CurrentPeriodEnd, 0)
				})
				t.Run("アウトボックスのpremium-updatedが通知の前から増えない", func(t *testing.T) {
					o := receiveOnActiveGoogleContract(t, googleSubscriptionCanceled)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
			})
			t.Run("subscriptionNotificationを持たない通知(voidedPurchaseNotificationなど)を受け取ったとき", func(t *testing.T) {
				receiveVoidedPurchase := func(t *testing.T) outcome {
					return newActiveGoogleContract(t).receive(t, subscriptiontest.BuildVoidedPurchaseNotificationMessage(t, googleToken))
				}
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					o := receiveVoidedPurchase(t)

					assert.NoError(t, o.err)
				})
				t.Run("アウトボックスのpremium-updatedが通知の前から増えない", func(t *testing.T) {
					o := receiveVoidedPurchase(t)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
				t.Run("Infoのログが1件出力される", func(t *testing.T) {
					logs := logtest.Capture(t)

					receiveVoidedPurchase(t)

					assert.Len(t, logs.SelectByMessageAndLevel(googleSkippedMessage, slog.LevelInfo), 1)
				})
			})
			t.Run("契約状態がactiveの契約に、通知種別が4(SUBSCRIPTION_PURCHASED)の通知を受け取ったとき", func(t *testing.T) {
				t.Run("契約状態が通知の前と同じになる", func(t *testing.T) {
					o, _ := receiveUnhandledGoogleType(t, googleSubscriptionPurchased)

					assert.Equal(t, o.before.Status, o.findContract(t).Status)
				})
				t.Run("アウトボックスのpremium-updatedが通知の前から増えない", func(t *testing.T) {
					o, _ := receiveUnhandledGoogleType(t, googleSubscriptionPurchased)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					o, _ := receiveUnhandledGoogleType(t, googleSubscriptionPurchased)

					assert.NoError(t, o.err)
				})
				t.Run("Warnのログが1件出力される", func(t *testing.T) {
					_, logs := receiveUnhandledGoogleType(t, googleSubscriptionPurchased)

					assert.Len(t, logs.SelectByMessageAndLevel(googleUnhandledTypeMessage, slog.LevelWarn), 1)
				})
				t.Run("ログに通知種別が含まれる", func(t *testing.T) {
					_, logs := receiveUnhandledGoogleType(t, googleSubscriptionPurchased)

					records := logs.SelectByMessageAndLevel(googleUnhandledTypeMessage, slog.LevelWarn)
					require.Len(t, records, 1)
					assert.Equal(t, "4", records[0].String("notification_type"))
				})
				t.Run("ログに購入トークンが含まれる", func(t *testing.T) {
					_, logs := receiveUnhandledGoogleType(t, googleSubscriptionPurchased)

					records := logs.SelectByMessageAndLevel(googleUnhandledTypeMessage, slog.LevelWarn)
					require.Len(t, records, 1)
					assert.Equal(t, googleToken, records[0].String("purchase_token"))
				})
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("契約状態がactiveの契約に、未対応の通知種別(Googleが定義していない値99)の通知を受け取ったとき", func(t *testing.T) {
				t.Run("契約状態が通知の前と同じになる", func(t *testing.T) {
					o, _ := receiveUnhandledGoogleType(t, googleUndefinedType)

					assert.Equal(t, o.before.Status, o.findContract(t).Status)
				})
				t.Run("アウトボックスのpremium-updatedが通知の前から増えない", func(t *testing.T) {
					o, _ := receiveUnhandledGoogleType(t, googleUndefinedType)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					o, _ := receiveUnhandledGoogleType(t, googleUndefinedType)

					assert.NoError(t, o.err)
				})
				t.Run("Warnのログが1件出力される", func(t *testing.T) {
					_, logs := receiveUnhandledGoogleType(t, googleUndefinedType)

					assert.Len(t, logs.SelectByMessageAndLevel(googleUnhandledTypeMessage, slog.LevelWarn), 1)
				})
				t.Run("ログに通知種別が含まれる", func(t *testing.T) {
					_, logs := receiveUnhandledGoogleType(t, googleUndefinedType)

					records := logs.SelectByMessageAndLevel(googleUnhandledTypeMessage, slog.LevelWarn)
					require.Len(t, records, 1)
					assert.Equal(t, "99", records[0].String("notification_type"))
				})
				t.Run("ログに購入トークンが含まれる", func(t *testing.T) {
					_, logs := receiveUnhandledGoogleType(t, googleUndefinedType)

					records := logs.SelectByMessageAndLevel(googleUnhandledTypeMessage, slog.LevelWarn)
					require.Len(t, records, 1)
					assert.Equal(t, googleToken, records[0].String("purchase_token"))
				})
			})
			t.Run("通知データが読み取れないとき", func(t *testing.T) {
				t.Run("通知データがbase64として読めないと、通知データの復号に失敗するエラーで失敗する", func(t *testing.T) {
					o := newActiveGoogleContract(t).receive(t, unreadableDataCases[0].message)

					assert.ErrorIs(t, o.err, subscription.ErrDecodeRTDNData)
				})
				t.Run("通知データがbase64として読めてもJSONとして読めないと、通知データの解釈に失敗するエラーで失敗する", func(t *testing.T) {
					o := newActiveGoogleContract(t).receive(t, unreadableDataCases[1].message)

					assert.ErrorIs(t, o.err, subscription.ErrUnmarshalRTDNData)
				})
				for _, tt := range unreadableDataCases {
					t.Run(tt.name+"、アウトボックスのpremium-updatedが通知の前から増えない", func(t *testing.T) {
						o := newActiveGoogleContract(t).receive(t, tt.message)

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
			t.Run("通知の購入トークンに対応する契約が無いとき", func(t *testing.T) {
				receiveWithoutContract := func(t *testing.T) outcome {
					env := newGoogleEnv(t)
					message := subscriptiontest.BuildSubscriptionNotificationMessage(t, googleSubscriptionExpired, googleToken)
					return processWithoutContract(t, func() error {
						return env.notifier.HandleNotification(t.Context(), message)
					})
				}
				t.Run("契約が見つからないエラーで失敗する", func(t *testing.T) {
					o := receiveWithoutContract(t)

					assert.ErrorIs(t, o.err, subscription.ErrSubscriptionNotFound)
				})
				t.Run("エラーの内容に通知の購入トークンが含まれる", func(t *testing.T) {
					o := receiveWithoutContract(t)

					assert.ErrorContains(t, o.err, googleToken)
				})
				t.Run("アウトボックスのpremium-updatedが通知の前から増えない", func(t *testing.T) {
					o := receiveWithoutContract(t)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
			})
			t.Run("更新または復旧の通知で、PlayDeveloperAPIが有効期限の取得に失敗したとき", func(t *testing.T) {
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、エラーの内容にPlayDeveloperAPIのエラーのメッセージが含まれる", func(t *testing.T) {
						o := receiveUnreadableRenewal(t, tt.notificationType)

						assert.ErrorContains(t, o.err, "TST-play-developer-api-failed")
					})
				}
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、契約状態が通知の前と同じになる", func(t *testing.T) {
						o := receiveUnreadableRenewal(t, tt.notificationType)

						assert.Equal(t, o.before.Status, o.findContract(t).Status)
					})
				}
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、アウトボックスのpremium-updatedが通知の前から増えない", func(t *testing.T) {
						o := receiveUnreadableRenewal(t, tt.notificationType)

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
			t.Run("通知データが読み取れない失敗、契約が見つからない失敗のあとの回復", func(t *testing.T) {
				t.Run("読み取れない通知で失敗したあとに、同じ契約への有効な通知を受け取ると、その通知の内容どおりに契約状態が変わる", func(t *testing.T) {
					env := newActiveGoogleContract(t)
					failed := env.receive(t, unreadableDataCases[0].message)
					require.ErrorIs(t, failed.err, subscription.ErrDecodeRTDNData)

					recovered := env.receiveType(t, googleSubscriptionExpired)
					require.NoError(t, recovered.err)

					assert.Equal(t, "expired", recovered.findContract(t).Status)
				})
			})
			t.Run("有効期限の取得失敗のあとの回復", func(t *testing.T) {
				t.Run("有効期限の取得に失敗したあと、同じ通知を再び受け取り取得に成功したとき", func(t *testing.T) {
					t.Run("契約状態がactiveになる", func(t *testing.T) {
						o := receiveRenewalAfterFetchFailure(t)

						assert.Equal(t, "active", o.findContract(t).Status)
					})
					t.Run("課金期間の終了日時が有効期限になる", func(t *testing.T) {
						o := receiveRenewalAfterFetchFailure(t)

						assert.WithinDuration(t, renewedExpiresAt, o.findContract(t).CurrentPeriodEnd, 0)
					})
				})
			})
		})
	})
}
