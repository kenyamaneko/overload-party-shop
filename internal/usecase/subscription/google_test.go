//go:build integration

package subscription_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/port/porttest"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription/subscriptiontest"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

const (
	googleSubscriptionRecovered = 1
	googleSubscriptionRenewed   = 2
	googleSubscriptionRevoked   = 12
	googleSubscriptionExpired   = 13
	googleUndefinedType         = 99

	googleUnhandledTypeMessage = "google RTDN unhandled notification type"
	googleSkippedMessage       = "google RTDN skipped: no subscriptionNotification"

	googleOtherProductID = "TST-GOOGLE-OTHER-PRODUCT"
)

var (
	errPlayDeveloperAPI = errors.New("TST-play-developer-api-failed")

	priorExpiresAt = time.Date(2028, 1, 15, 8, 0, 0, 0, time.UTC)
)

type googleEnv struct {
	notifier *subscription.GoogleNotifier
	sub      *porttest.MockGoogleSubVerifier
}

func newGoogleEnv(t *testing.T) googleEnv {
	t.Helper()
	newShop(t)
	sub := porttest.NewMockGoogleSubVerifier()
	sub.Return(port.GoogleSubscriptionStateActive, subscriptionProductID, renewedExpiresAt)
	return googleEnv{notifier: subscriptiontest.NewGoogleNotifier(pg.Pool, sub), sub: sub}
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
	env.sub.Return(port.GoogleSubscriptionStateExpired, subscriptionProductID, priorExpiresAt)
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

type googleStateCase struct {
	name             string
	state            port.GoogleSubscriptionState
	notificationType int
}

var (
	googleActive = googleStateCase{
		name: "GooglePlayが返す購読の状態がACTIVEのとき", state: port.GoogleSubscriptionStateActive, notificationType: googleSubscriptionRenewed,
	}
	googleInGracePeriod = googleStateCase{
		name: "GooglePlayが返す購読の状態がIN_GRACE_PERIODのとき", state: port.GoogleSubscriptionStateInGracePeriod, notificationType: googleSubscriptionRenewed,
	}
	googleCanceled = googleStateCase{
		name: "GooglePlayが返す購読の状態がCANCELEDのとき", state: port.GoogleSubscriptionStateCanceled, notificationType: googleSubscriptionRenewed,
	}
	googleOnHold = googleStateCase{
		name: "GooglePlayが返す購読の状態がON_HOLDのとき", state: port.GoogleSubscriptionStateOnHold, notificationType: googleSubscriptionRenewed,
	}
	googlePaused = googleStateCase{
		name: "GooglePlayが返す購読の状態がPAUSEDのとき", state: port.GoogleSubscriptionStatePaused, notificationType: googleSubscriptionRenewed,
	}
	googleExpiredByExpiration = googleStateCase{
		name: "GooglePlayが返す購読の状態がEXPIREDで、通知種別が13(SUBSCRIPTION_EXPIRED)のとき", state: port.GoogleSubscriptionStateExpired, notificationType: googleSubscriptionExpired,
	}
	googleExpiredByRevocation = googleStateCase{
		name: "GooglePlayが返す購読の状態がEXPIREDで、通知種別が12(SUBSCRIPTION_REVOKED)のとき", state: port.GoogleSubscriptionStateExpired, notificationType: googleSubscriptionRevoked,
	}
)

var (
	activeOrGracePeriodCases = []googleStateCase{googleActive, googleInGracePeriod}
	expiredStatusCases       = []googleStateCase{googleOnHold, googlePaused, googleExpiredByExpiration}
	periodEndCases           = []googleStateCase{googleCanceled, googleOnHold, googlePaused, googleExpiredByExpiration, googleExpiredByRevocation}
	invalidatingEventCases   = []googleStateCase{googleOnHold, googlePaused, googleExpiredByExpiration, googleExpiredByRevocation}
)

type unsupportedStateCase struct {
	name       string
	state      port.GoogleSubscriptionState
	stateValue string
}

var unsupportedStateCases = []unsupportedStateCase{
	{name: "GooglePlayが返す購読の状態がPENDINGのとき", state: port.GoogleSubscriptionStatePending, stateValue: "PENDING"},
	{name: "GooglePlayが返す購読の状態がPENDING_PURCHASE_CANCELEDのとき", state: port.GoogleSubscriptionStatePendingPurchaseCanceled, stateValue: "PENDING_PURCHASE_CANCELED"},
	{name: "GooglePlayが返す購読の状態がUNSPECIFIEDのとき", state: port.GoogleSubscriptionStateUnspecified, stateValue: "UNSPECIFIED"},
}

var (
	mismatchActive = googleStateCase{
		name: "GooglePlayが返す購読の状態がACTIVEのとき", state: port.GoogleSubscriptionStateActive, notificationType: googleSubscriptionRenewed,
	}
	mismatchCanceled = googleStateCase{
		name: "GooglePlayが返す購読の状態がCANCELEDのとき", state: port.GoogleSubscriptionStateCanceled, notificationType: googleSubscriptionRenewed,
	}
	mismatchInGracePeriod = googleStateCase{
		name: "GooglePlayが返す購読の状態がIN_GRACE_PERIODのとき", state: port.GoogleSubscriptionStateInGracePeriod, notificationType: googleSubscriptionRenewed,
	}
	mismatchOnHold = googleStateCase{
		name: "GooglePlayが返す購読の状態がON_HOLDのとき", state: port.GoogleSubscriptionStateOnHold, notificationType: googleSubscriptionRenewed,
	}
	mismatchPaused = googleStateCase{
		name: "GooglePlayが返す購読の状態がPAUSEDのとき", state: port.GoogleSubscriptionStatePaused, notificationType: googleSubscriptionRenewed,
	}
	mismatchExpired = googleStateCase{
		name: "GooglePlayが返す購読の状態がEXPIREDのとき", state: port.GoogleSubscriptionStateExpired, notificationType: googleSubscriptionExpired,
	}

	productMismatchCases      = []googleStateCase{mismatchActive, mismatchCanceled, mismatchInGracePeriod, mismatchOnHold, mismatchPaused, mismatchExpired}
	productMismatchEventCases = []googleStateCase{mismatchActive, mismatchInGracePeriod, mismatchOnHold, mismatchPaused, mismatchExpired}
)

func receiveOnExpiredGoogleContract(t *testing.T, tc googleStateCase) outcome {
	t.Helper()
	env := newExpiredGoogleContract(t)
	env.sub.Return(tc.state, subscriptionProductID, renewedExpiresAt)
	received := env.receiveType(t, tc.notificationType)
	require.NoError(t, received.err)
	return received
}

func receiveOnActiveGoogleContract(t *testing.T, tc googleStateCase) outcome {
	t.Helper()
	env := newActiveGoogleContract(t)
	env.sub.Return(tc.state, subscriptionProductID, renewedExpiresAt)
	received := env.receiveType(t, tc.notificationType)
	require.NoError(t, received.err)
	return received
}

func receiveUnsupportedState(t *testing.T, tc unsupportedStateCase) outcome {
	t.Helper()
	env := newActiveGoogleContract(t)
	env.sub.Return(tc.state, subscriptionProductID, renewedExpiresAt)
	return env.receiveType(t, googleSubscriptionRenewed)
}

func receiveOtherProduct(t *testing.T, tc googleStateCase) outcome {
	t.Helper()
	env := newActiveGoogleContract(t)
	env.sub.Return(tc.state, googleOtherProductID, renewedExpiresAt)
	return env.receiveType(t, tc.notificationType)
}

func receiveWithoutExpiry(t *testing.T) outcome {
	t.Helper()
	env := newExpiredGoogleContract(t)
	env.sub.ReturnWithoutExpiry(port.GoogleSubscriptionStateActive, subscriptionProductID)
	return env.receiveType(t, googleSubscriptionRenewed)
}

func receiveUnhandledGoogleType(t *testing.T, notificationType int) (outcome, *logtest.Logs) {
	t.Helper()
	logs := logtest.Capture(t)
	return newActiveGoogleContract(t).receiveType(t, notificationType), logs
}

func receiveRenewalWhileFetchFails(t *testing.T, notificationType int) outcome {
	t.Helper()
	env := newExpiredGoogleContract(t)
	env.sub.FailWith(errPlayDeveloperAPI)
	return env.receiveType(t, notificationType)
}

func receiveRenewalAfterFetchFailure(t *testing.T) outcome {
	t.Helper()
	env := newExpiredGoogleContract(t)
	env.sub.FailWith(errPlayDeveloperAPI)
	failed := env.receiveType(t, googleSubscriptionRenewed)
	require.ErrorIs(t, failed.err, errPlayDeveloperAPI)
	env.sub.Return(port.GoogleSubscriptionStateActive, subscriptionProductID, renewedExpiresAt)
	retried := env.receiveType(t, googleSubscriptionRenewed)
	require.NoError(t, retried.err)
	return retried
}

func receiveActiveAfterUnreadableData(t *testing.T) outcome {
	t.Helper()
	env := newExpiredGoogleContract(t)
	failed := env.receive(t, unreadableDataCases[1].message)
	require.ErrorIs(t, failed.err, subscription.ErrUnmarshalRTDNData)
	env.sub.Return(port.GoogleSubscriptionStateActive, subscriptionProductID, renewedExpiresAt)
	recovered := env.receiveType(t, googleSubscriptionRenewed)
	require.NoError(t, recovered.err)
	return recovered
}

type googleTypeCase struct {
	name             string
	notificationType int
}

var renewalTypeCases = []googleTypeCase{
	{"通知種別が2(SUBSCRIPTION_RENEWED)のとき", googleSubscriptionRenewed},
	{"通知種別が1(SUBSCRIPTION_RECOVERED)のとき", googleSubscriptionRecovered},
}

type unreadableDataCase struct {
	name    string
	message subscription.GoogleRTDNMessage
}

var unreadableDataCases = []unreadableDataCase{
	{"通知データがbase64として読めないとき", subscriptiontest.BuildRawMessage("TST-!!!-NOT-BASE64")},
	{"通知データがJSONとして読めないとき", subscriptiontest.BuildEncodedMessage("TST-NOT-JSON")},
}

func TestGoogleNotifier(t *testing.T) {
	t.Run("[サブスクリプション]GooglePlay通知の処理", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("契約状態がexpiredの契約に、GooglePlayが返す購読の状態がACTIVEまたはIN_GRACE_PERIODの通知を受け取ったとき", func(t *testing.T) {
				t.Run(googleActive.name+"、契約状態がactiveになる", func(t *testing.T) {
					o := receiveOnExpiredGoogleContract(t, googleActive)

					assert.Equal(t, "active", o.findContract(t).Status)
				})
				t.Run(googleInGracePeriod.name+"、契約状態がgrace_periodになる", func(t *testing.T) {
					o := receiveOnExpiredGoogleContract(t, googleInGracePeriod)

					assert.Equal(t, "grace_period", o.findContract(t).Status)
				})
				for _, tt := range activeOrGracePeriodCases {
					t.Run(tt.name+"、課金期間の終了日時が有効期限になる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt)

						assert.WithinDuration(t, renewedExpiresAt, o.findContract(t).CurrentPeriodEnd, 0)
					})
				}
				for _, tt := range activeOrGracePeriodCases {
					t.Run(tt.name+"、アウトボックスにpremium_updatedが1件加わる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt)

						assert.Len(t, o.listNewPremiumEvents(t), 1)
					})
				}
				for _, tt := range activeOrGracePeriodCases {
					t.Run(tt.name+"、アウトボックスに加わったpremium_updatedのis_premiumがtrueになる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt)

						assert.Equal(t, true, o.findSinglePremiumEvent(t).Payload["is_premium"])
					})
				}
				for _, tt := range activeOrGracePeriodCases {
					t.Run(tt.name+"、アウトボックスに加わったpremium_updatedのpremium_expires_atが有効期限になる", func(t *testing.T) {
						o := receiveOnExpiredGoogleContract(t, tt)

						assert.WithinDuration(t, renewedExpiresAt, o.findSinglePremiumEvent(t).TimeField(t, "premium_expires_at"), 0)
					})
				}
			})
			t.Run("契約状態がactiveの契約に、GooglePlayが返す購読の状態がCANCELED・ON_HOLD・PAUSED・EXPIREDのいずれかの通知を受け取ったとき", func(t *testing.T) {
				t.Run(googleCanceled.name+"、契約状態がcancelledになる", func(t *testing.T) {
					o := receiveOnActiveGoogleContract(t, googleCanceled)

					assert.Equal(t, "cancelled", o.findContract(t).Status)
				})
				for _, tt := range expiredStatusCases {
					t.Run(tt.name+"、契約状態がexpiredになる", func(t *testing.T) {
						o := receiveOnActiveGoogleContract(t, tt)

						assert.Equal(t, "expired", o.findContract(t).Status)
					})
				}
				t.Run(googleExpiredByRevocation.name+"、契約状態がrevokedになる", func(t *testing.T) {
					o := receiveOnActiveGoogleContract(t, googleExpiredByRevocation)

					assert.Equal(t, "revoked", o.findContract(t).Status)
				})
				for _, tt := range periodEndCases {
					t.Run(tt.name+"、課金期間の終了日時が有効期限になる", func(t *testing.T) {
						o := receiveOnActiveGoogleContract(t, tt)

						assert.WithinDuration(t, renewedExpiresAt, o.findContract(t).CurrentPeriodEnd, 0)
					})
				}
				t.Run(googleCanceled.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
					o := receiveOnActiveGoogleContract(t, googleCanceled)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
				for _, tt := range invalidatingEventCases {
					t.Run(tt.name+"、アウトボックスにpremium_updatedが1件加わる", func(t *testing.T) {
						o := receiveOnActiveGoogleContract(t, tt)

						assert.Len(t, o.listNewPremiumEvents(t), 1)
					})
				}
				for _, tt := range invalidatingEventCases {
					t.Run(tt.name+"、アウトボックスに加わったpremium_updatedのis_premiumがfalseになる", func(t *testing.T) {
						o := receiveOnActiveGoogleContract(t, tt)

						assert.Equal(t, false, o.findSinglePremiumEvent(t).Payload["is_premium"])
					})
				}
				for _, tt := range invalidatingEventCases {
					t.Run(tt.name+"、アウトボックスに加わったpremium_updatedにpremium_expires_atが含まれない", func(t *testing.T) {
						o := receiveOnActiveGoogleContract(t, tt)

						assert.NotContains(t, o.findSinglePremiumEvent(t).Payload, "premium_expires_at")
					})
				}
			})
			t.Run("subscriptionNotificationを持たない通知(voidedPurchaseNotificationなど)を受け取ったとき", func(t *testing.T) {
				receiveVoidedPurchase := func(t *testing.T) outcome {
					return newActiveGoogleContract(t).receive(t, subscriptiontest.BuildVoidedPurchaseNotificationMessage(t, googleToken))
				}
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					o := receiveVoidedPurchase(t)

					assert.NoError(t, o.err)
				})
				t.Run("アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
					o := receiveVoidedPurchase(t)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
				t.Run("Infoのログが1件出力される", func(t *testing.T) {
					logs := logtest.Capture(t)

					receiveVoidedPurchase(t)

					assert.Len(t, logs.SelectByMessageAndLevel(googleSkippedMessage, slog.LevelInfo), 1)
				})
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("契約状態がactiveの契約に、未対応の通知種別(GooglePlayが定義していない値99)の通知を受け取ったとき", func(t *testing.T) {
				t.Run("契約状態が通知の前と同じになる", func(t *testing.T) {
					o, _ := receiveUnhandledGoogleType(t, googleUndefinedType)

					assert.Equal(t, o.before.Status, o.findContract(t).Status)
				})
				t.Run("アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
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
					t.Run(tt.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
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
				t.Run("アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
					o := receiveWithoutContract(t)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
			})
			t.Run("更新または復旧の通知で、GooglePlayが購読情報の取得に失敗したとき", func(t *testing.T) {
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、エラーの内容にGooglePlayのエラーのメッセージが含まれる", func(t *testing.T) {
						o := receiveRenewalWhileFetchFails(t, tt.notificationType)

						assert.ErrorContains(t, o.err, "TST-play-developer-api-failed")
					})
				}
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、契約状態が通知の前と同じになる", func(t *testing.T) {
						o := receiveRenewalWhileFetchFails(t, tt.notificationType)

						assert.Equal(t, o.before.Status, o.findContract(t).Status)
					})
				}
				for _, tt := range renewalTypeCases {
					t.Run(tt.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
						o := receiveRenewalWhileFetchFails(t, tt.notificationType)

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
			t.Run("通知データが読み取れない失敗のあとの回復", func(t *testing.T) {
				t.Run("契約状態がexpiredの契約に、通知データがJSONとして読めない通知を受け取って失敗したあと、同じ契約へ、GooglePlayが返す購読の状態がACTIVEの通知を受け取ったとき", func(t *testing.T) {
					t.Run("契約状態がactiveになる", func(t *testing.T) {
						o := receiveActiveAfterUnreadableData(t)

						assert.Equal(t, "active", o.findContract(t).Status)
					})
					t.Run("課金期間の終了日時が有効期限になる", func(t *testing.T) {
						o := receiveActiveAfterUnreadableData(t)

						assert.WithinDuration(t, renewedExpiresAt, o.findContract(t).CurrentPeriodEnd, 0)
					})
				})
			})
			t.Run("購読情報の取得失敗のあとの回復", func(t *testing.T) {
				t.Run("契約状態がexpiredの契約に、通知種別が2(SUBSCRIPTION_RENEWED)の通知を受け取ってGooglePlayが購読情報の取得に失敗したあと、通知種別が2(SUBSCRIPTION_RENEWED)の通知を再び受け取り、GooglePlayが返す購読の状態がACTIVEの取得に成功したとき", func(t *testing.T) {
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
			t.Run("契約状態がexpiredの契約に、GooglePlayが返す購読の状態がACTIVEで、購読情報に有効期限が無い通知を受け取ったとき", func(t *testing.T) {
				t.Run("有効期限が無いことを示すエラーで失敗する", func(t *testing.T) {
					o := receiveWithoutExpiry(t)

					assert.ErrorContains(t, o.err, "expiry is missing")
				})
				t.Run("契約状態が通知の前と同じになる", func(t *testing.T) {
					o := receiveWithoutExpiry(t)

					assert.Equal(t, o.before.Status, o.findContract(t).Status)
				})
				t.Run("課金期間の終了日時が通知の前と同じになる", func(t *testing.T) {
					o := receiveWithoutExpiry(t)

					assert.WithinDuration(t, o.before.CurrentPeriodEnd, o.findContract(t).CurrentPeriodEnd, 0)
				})
				t.Run("アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
					o := receiveWithoutExpiry(t)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
			})
			t.Run("契約状態がactiveの契約に、GooglePlayが返す購読の状態がPENDING・PENDING_PURCHASE_CANCELED・UNSPECIFIEDのいずれかの通知を受け取ったとき", func(t *testing.T) {
				for _, tt := range unsupportedStateCases {
					t.Run(tt.name+"、購読の状態を扱えないことを示すエラーで失敗する", func(t *testing.T) {
						o := receiveUnsupportedState(t, tt)

						assert.ErrorContains(t, o.err, tt.stateValue)
					})
				}
				for _, tt := range unsupportedStateCases {
					t.Run(tt.name+"、契約状態が通知の前と同じになる", func(t *testing.T) {
						o := receiveUnsupportedState(t, tt)

						assert.Equal(t, o.before.Status, o.findContract(t).Status)
					})
				}
				for _, tt := range unsupportedStateCases {
					t.Run(tt.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
						o := receiveUnsupportedState(t, tt)

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
			t.Run("契約状態がactiveの契約に、GooglePlayが返す商品が契約の商品と異なる通知を受け取ったとき", func(t *testing.T) {
				for _, tt := range productMismatchCases {
					t.Run(tt.name+"、商品が一致しないことを示すエラーで失敗する", func(t *testing.T) {
						o := receiveOtherProduct(t, tt)

						assert.ErrorContains(t, o.err, subscriptionProductID)
						assert.ErrorContains(t, o.err, googleOtherProductID)
					})
				}
				for _, tt := range productMismatchCases {
					t.Run(tt.name+"、課金期間の終了日時が通知の前と同じになる", func(t *testing.T) {
						o := receiveOtherProduct(t, tt)

						assert.WithinDuration(t, o.before.CurrentPeriodEnd, o.findContract(t).CurrentPeriodEnd, 0)
					})
				}
				for _, tt := range productMismatchEventCases {
					t.Run(tt.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
						o := receiveOtherProduct(t, tt)

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
		})
	})
}
