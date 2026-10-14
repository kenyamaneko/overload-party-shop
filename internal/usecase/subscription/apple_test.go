//go:build integration

package subscription_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription/subscriptiontest"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

const (
	appleBundleID      = "TST-BUNDLE-ID"
	appleOtherBundleID = "TST-OTHER-BUNDLE-ID"

	renewalExpiresAtMillis = int64(1800000000000)

	unhandledTypeMessage    = "apple unhandled notification type"
	unhandledSubtypeMessage = "apple unhandled DID_CHANGE_RENEWAL_STATUS subtype"
)

type appleEnv struct {
	notifier *subscription.AppleNotifier
	signer   *subscriptiontest.AppleSigner
}

func newAppleEnv(t *testing.T) appleEnv {
	t.Helper()
	newShop(t)
	signer := subscriptiontest.NewAppleSigner(t)
	return appleEnv{notifier: subscriptiontest.NewAppleNotifier(pg.Pool, signer.Verifier(), appleBundleID), signer: signer}
}

func newActiveAppleContract(t *testing.T) appleEnv {
	t.Helper()
	env := newAppleEnv(t)
	registerContract(t, platformIOS, appleToken)
	return env
}

func newExpiredAppleContract(t *testing.T) appleEnv {
	t.Helper()
	env := newActiveAppleContract(t)
	require.NoError(t, env.receive(t, buildAppleNotification("EXPIRED")).err)
	return env
}

func (e appleEnv) receive(t *testing.T, notification subscriptiontest.AppleNotification) outcome {
	t.Helper()
	return e.receiveSigned(t, e.signer.SignNotification(notification))
}

func (e appleEnv) receiveSigned(t *testing.T, signedPayload string) outcome {
	t.Helper()
	return process(t, platformIOS, appleToken, func() error {
		return e.notifier.HandleNotification(t.Context(), signedPayload)
	})
}

func buildAppleNotification(notificationType string) subscriptiontest.AppleNotification {
	return subscriptiontest.AppleNotification{
		NotificationType:      notificationType,
		BundleID:              appleBundleID,
		OriginalTransactionID: appleToken,
	}
}

func buildAppleRenewal(expiresAtMillis *int64) subscriptiontest.AppleNotification {
	notification := buildAppleNotification("DID_RENEW")
	notification.ExpiresAtMillis = expiresAtMillis
	return notification
}

func buildAppleRenewalStatusChange(subtype string) subscriptiontest.AppleNotification {
	notification := buildAppleNotification("DID_CHANGE_RENEWAL_STATUS")
	notification.Subtype = subtype
	return notification
}

func toInt64Pointer(value int64) *int64 {
	return &value
}

func renewExpiredContract(t *testing.T) outcome {
	t.Helper()
	received := newExpiredAppleContract(t).receive(t, buildAppleRenewal(toInt64Pointer(renewalExpiresAtMillis)))
	require.NoError(t, received.err)
	return received
}

func receiveOnActiveContract(t *testing.T, notificationType string) outcome {
	t.Helper()
	received := newActiveAppleContract(t).receive(t, buildAppleNotification(notificationType))
	require.NoError(t, received.err)
	return received
}

func receiveRenewalStatusChange(t *testing.T, subtype string) outcome {
	t.Helper()
	received := newActiveAppleContract(t).receive(t, buildAppleRenewalStatusChange(subtype))
	require.NoError(t, received.err)
	return received
}

type appleUnhandledCase struct {
	name             string
	notificationType string
	subtype          string
	message          string
}

var (
	unhandledType = appleUnhandledCase{
		name: "通知種別がAppleの定義にない値のとき", notificationType: "TST-UNDEFINED-TYPE", subtype: "", message: unhandledTypeMessage,
	}
	unhandledEmptySubtype = appleUnhandledCase{
		name: "通知種別がDID_CHANGE_RENEWAL_STATUSでsubtypeが空文字のとき", notificationType: "DID_CHANGE_RENEWAL_STATUS", subtype: "", message: unhandledSubtypeMessage,
	}
	unhandledUndefinedSubtype = appleUnhandledCase{
		name: "通知種別がDID_CHANGE_RENEWAL_STATUSでsubtypeがAppleの定義にない値のとき", notificationType: "DID_CHANGE_RENEWAL_STATUS", subtype: "TST-UNDEFINED-SUBTYPE", message: unhandledSubtypeMessage,
	}
	unhandledCases        = []appleUnhandledCase{unhandledType, unhandledEmptySubtype, unhandledUndefinedSubtype}
	unhandledSubtypeCases = []appleUnhandledCase{unhandledEmptySubtype, unhandledUndefinedSubtype}
)

type unhandledRun struct {
	outcome outcome
	logs    *logtest.Logs
}

func receiveUnhandled(t *testing.T, tt appleUnhandledCase) unhandledRun {
	t.Helper()
	logs := logtest.Capture(t)
	notification := buildAppleNotification(tt.notificationType)
	notification.Subtype = tt.subtype
	return unhandledRun{outcome: newActiveAppleContract(t).receive(t, notification), logs: logs}
}

type applePayloadCase struct {
	name         string
	signedResult func(signer *subscriptiontest.AppleSigner) string
}

var (
	unreadableNotifications = []applePayloadCase{
		{"通知本体の署名検証に失敗したとき", func(signer *subscriptiontest.AppleSigner) string {
			return signer.SignInvalid(errVerify)
		}},
		{"通知本体が署名検証を通ってもJSONとして読めないとき", func(signer *subscriptiontest.AppleSigner) string {
			return signer.Sign([]byte("TST-NOT-JSON"))
		}},
	}
	unreadableTransactionInfos = []applePayloadCase{
		{"取引情報の署名検証に失敗したとき", func(signer *subscriptiontest.AppleSigner) string {
			return signer.SignNotificationWithTransactionInfo(buildAppleNotification("EXPIRED"), signer.SignInvalid(errVerify))
		}},
		{"取引情報が署名検証を通ってもJSONとして読めないとき", func(signer *subscriptiontest.AppleSigner) string {
			return signer.SignNotificationWithTransactionInfo(buildAppleNotification("EXPIRED"), signer.Sign([]byte("TST-NOT-JSON")))
		}},
	}
)

type appleExpiryCase struct {
	name            string
	expiresAtMillis *int64
}

var unreadableExpiries = []appleExpiryCase{
	{"通知の有効期限が欠落しているとき", nil},
	{"通知の有効期限が0のとき", toInt64Pointer(0)},
}

func TestAppleNotifier(t *testing.T) {
	t.Run("[サブスクリプション]Apple通知の処理", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("契約状態がexpiredの契約に、通知種別DID_RENEWで通知の有効期限が2027-01-15T08:00:00Zの通知を受け取ったとき", func(t *testing.T) {
				t.Run("契約状態がactiveになる", func(t *testing.T) {
					o := renewExpiredContract(t)

					assert.Equal(t, "active", o.findContract(t).Status)
				})
				t.Run("課金期間の終了日時が2027-01-15T08:00:00Zになる", func(t *testing.T) {
					o := renewExpiredContract(t)

					assert.WithinDuration(t, time.Date(2027, 1, 15, 8, 0, 0, 0, time.UTC), o.findContract(t).CurrentPeriodEnd, 0)
				})
				t.Run("アウトボックスにpremium_updatedが1件加わる", func(t *testing.T) {
					o := renewExpiredContract(t)

					assert.Len(t, o.listNewPremiumEvents(t), 1)
				})
				t.Run("アウトボックスに加わったpremium_updatedのplayer_idが契約の持ち主になる", func(t *testing.T) {
					o := renewExpiredContract(t)

					assert.Equal(t, playerID, o.findSinglePremiumEvent(t).Payload["player_id"])
				})
				t.Run("アウトボックスに加わったpremium_updatedのis_premiumがtrueになる", func(t *testing.T) {
					o := renewExpiredContract(t)

					assert.Equal(t, true, o.findSinglePremiumEvent(t).Payload["is_premium"])
				})
				t.Run("アウトボックスに加わったpremium_updatedのpremium_expires_atが2027-01-15T08:00:00Zになる", func(t *testing.T) {
					o := renewExpiredContract(t)

					assert.WithinDuration(t, time.Date(2027, 1, 15, 8, 0, 0, 0, time.UTC), o.findSinglePremiumEvent(t).TimeField(t, "premium_expires_at"), 0)
				})
				t.Run("アウトボックスに加わったpremium_updatedの、記録のevent_idがイベントのevent_idと一致する", func(t *testing.T) {
					o := renewExpiredContract(t)

					event := o.findSinglePremiumEvent(t)

					assert.Equal(t, event.EventID, event.Payload["event_id"])
				})
			})
			t.Run("契約状態がactiveの契約に、期限切れまたは取り消しの通知を受け取ったとき", func(t *testing.T) {
				expiryTypes := []string{"EXPIRED", "GRACE_PERIOD_EXPIRED"}
				allTypes := []string{"EXPIRED", "GRACE_PERIOD_EXPIRED", "REVOKE"}
				for _, notificationType := range expiryTypes {
					t.Run("通知種別が"+notificationType+"のとき、契約状態がexpiredになる", func(t *testing.T) {
						o := receiveOnActiveContract(t, notificationType)

						assert.Equal(t, "expired", o.findContract(t).Status)
					})
				}
				t.Run("通知種別がREVOKEのとき、契約状態がrevokedになる", func(t *testing.T) {
					o := receiveOnActiveContract(t, "REVOKE")

					assert.Equal(t, "revoked", o.findContract(t).Status)
				})
				for _, notificationType := range allTypes {
					t.Run("通知種別が"+notificationType+"のとき、アウトボックスにpremium_updatedが1件加わる", func(t *testing.T) {
						o := receiveOnActiveContract(t, notificationType)

						assert.Len(t, o.listNewPremiumEvents(t), 1)
					})
				}
				for _, notificationType := range allTypes {
					t.Run("通知種別が"+notificationType+"のとき、アウトボックスに加わったpremium_updatedのis_premiumがfalseになる", func(t *testing.T) {
						o := receiveOnActiveContract(t, notificationType)

						assert.Equal(t, false, o.findSinglePremiumEvent(t).Payload["is_premium"])
					})
				}
				for _, notificationType := range allTypes {
					t.Run("通知種別が"+notificationType+"のとき、アウトボックスに加わったpremium_updatedにpremium_expires_atが含まれない", func(t *testing.T) {
						o := receiveOnActiveContract(t, notificationType)

						assert.NotContains(t, o.findSinglePremiumEvent(t).Payload, "premium_expires_at")
					})
				}
			})
			t.Run("契約状態がactiveの契約に、通知種別DID_CHANGE_RENEWAL_STATUSの通知を受け取ったとき", func(t *testing.T) {
				t.Run("subtypeがAUTO_RENEW_DISABLEDのとき", func(t *testing.T) {
					t.Run("契約状態がcancelledになる", func(t *testing.T) {
						o := receiveRenewalStatusChange(t, "AUTO_RENEW_DISABLED")

						assert.Equal(t, "cancelled", o.findContract(t).Status)
					})
					t.Run("課金期間の終了日時が通知の前と同じになる", func(t *testing.T) {
						o := receiveRenewalStatusChange(t, "AUTO_RENEW_DISABLED")

						assert.WithinDuration(t, o.before.CurrentPeriodEnd, o.findContract(t).CurrentPeriodEnd, 0)
					})
				})
				t.Run("subtypeがAUTO_RENEW_ENABLEDのとき", func(t *testing.T) {
					t.Run("契約状態が通知の前と同じになる", func(t *testing.T) {
						o := receiveRenewalStatusChange(t, "AUTO_RENEW_ENABLED")

						assert.Equal(t, o.before.Status, o.findContract(t).Status)
					})
					t.Run("実行はエラーを返さない", func(t *testing.T) {
						o := newActiveAppleContract(t).receive(t, buildAppleRenewalStatusChange("AUTO_RENEW_ENABLED"))

						assert.NoError(t, o.err)
					})
					t.Run("Warnのログが1件出力される", func(t *testing.T) {
						logs := logtest.Capture(t)

						receiveRenewalStatusChange(t, "AUTO_RENEW_ENABLED")

						assert.Len(t, logs.SelectByMessageAndLevel(unhandledSubtypeMessage, slog.LevelWarn), 1)
					})
					t.Run("ログに通知のsubtypeが含まれる", func(t *testing.T) {
						logs := logtest.Capture(t)

						receiveRenewalStatusChange(t, "AUTO_RENEW_ENABLED")

						records := logs.SelectByMessageAndLevel(unhandledSubtypeMessage, slog.LevelWarn)
						require.Len(t, records, 1)
						assert.Equal(t, "AUTO_RENEW_ENABLED", records[0].String("subtype"))
					})
					t.Run("ログに取引識別子が含まれる", func(t *testing.T) {
						logs := logtest.Capture(t)

						receiveRenewalStatusChange(t, "AUTO_RENEW_ENABLED")

						records := logs.SelectByMessageAndLevel(unhandledSubtypeMessage, slog.LevelWarn)
						require.Len(t, records, 1)
						assert.Equal(t, appleToken, records[0].String("original_transaction_id"))
					})
				})
				for _, subtype := range []string{"AUTO_RENEW_DISABLED", "AUTO_RENEW_ENABLED"} {
					t.Run("subtypeが"+subtype+"のとき、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
						o := receiveRenewalStatusChange(t, subtype)

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("契約状態がactiveの契約に、未対応の通知を受け取ったとき", func(t *testing.T) {
				for _, tt := range unhandledCases {
					t.Run(tt.name+"、契約状態が通知の前と同じになる", func(t *testing.T) {
						run := receiveUnhandled(t, tt)

						assert.Equal(t, run.outcome.before.Status, run.outcome.findContract(t).Status)
					})
				}
				for _, tt := range unhandledCases {
					t.Run(tt.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
						run := receiveUnhandled(t, tt)

						assert.Empty(t, run.outcome.listNewPremiumEvents(t))
					})
				}
				for _, tt := range unhandledCases {
					t.Run(tt.name+"、実行はエラーを返さない", func(t *testing.T) {
						run := receiveUnhandled(t, tt)

						assert.NoError(t, run.outcome.err)
					})
				}
				for _, tt := range unhandledCases {
					t.Run(tt.name+"、Warnのログが1件出力される", func(t *testing.T) {
						run := receiveUnhandled(t, tt)

						assert.Len(t, run.logs.SelectByMessageAndLevel(tt.message, slog.LevelWarn), 1)
					})
				}
				for _, tt := range unhandledCases {
					t.Run(tt.name+"、ログに取引識別子が含まれる", func(t *testing.T) {
						run := receiveUnhandled(t, tt)

						records := run.logs.SelectByMessageAndLevel(tt.message, slog.LevelWarn)
						require.Len(t, records, 1)
						assert.Equal(t, appleToken, records[0].String("original_transaction_id"))
					})
				}
				t.Run("通知種別がAppleの定義にない値のとき、ログに通知種別が含まれる", func(t *testing.T) {
					run := receiveUnhandled(t, unhandledType)

					records := run.logs.SelectByMessageAndLevel(unhandledType.message, slog.LevelWarn)
					require.Len(t, records, 1)
					assert.Equal(t, "TST-UNDEFINED-TYPE", records[0].String("notification_type"))
				})
				for _, tt := range unhandledSubtypeCases {
					t.Run(tt.name+"、ログに通知のsubtypeが含まれる", func(t *testing.T) {
						run := receiveUnhandled(t, tt)

						records := run.logs.SelectByMessageAndLevel(tt.message, slog.LevelWarn)
						require.Len(t, records, 1)
						assert.Equal(t, tt.subtype, records[0].String("subtype"))
					})
				}
			})
			t.Run("通知本体が読み取れないとき", func(t *testing.T) {
				for _, tt := range unreadableNotifications {
					t.Run(tt.name+"、通知の復号に失敗するエラーで失敗する", func(t *testing.T) {
						env := newActiveAppleContract(t)

						o := env.receiveSigned(t, tt.signedResult(env.signer))

						assert.ErrorIs(t, o.err, subscription.ErrDecodeNotification)
					})
				}
				t.Run("通知本体の署名検証に失敗すると、エラーの内容に署名検証のエラーのメッセージが含まれる", func(t *testing.T) {
					env := newActiveAppleContract(t)

					o := env.receiveSigned(t, unreadableNotifications[0].signedResult(env.signer))

					assert.ErrorContains(t, o.err, "TST-verify-failed")
				})
				t.Run("通知本体が署名検証を通ってもJSONとして読めないと、エラーの内容にJSONの解釈に失敗したことが含まれる", func(t *testing.T) {
					env := newActiveAppleContract(t)

					o := env.receiveSigned(t, unreadableNotifications[1].signedResult(env.signer))

					assert.ErrorContains(t, o.err, "unmarshal JWS payload")
				})
				for _, tt := range unreadableNotifications {
					t.Run(tt.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
						env := newActiveAppleContract(t)

						o := env.receiveSigned(t, tt.signedResult(env.signer))

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
			t.Run("通知内の取引情報が読み取れないとき", func(t *testing.T) {
				for _, tt := range unreadableTransactionInfos {
					t.Run(tt.name+"、取引情報の復号に失敗するエラーで失敗する", func(t *testing.T) {
						env := newActiveAppleContract(t)

						o := env.receiveSigned(t, tt.signedResult(env.signer))

						assert.ErrorIs(t, o.err, subscription.ErrDecodeTransactionInfo)
					})
				}
				t.Run("取引情報の署名検証に失敗すると、エラーの内容に署名検証のエラーのメッセージが含まれる", func(t *testing.T) {
					env := newActiveAppleContract(t)

					o := env.receiveSigned(t, unreadableTransactionInfos[0].signedResult(env.signer))

					assert.ErrorContains(t, o.err, "TST-verify-failed")
				})
				t.Run("取引情報が署名検証を通ってもJSONとして読めないと、エラーの内容にJSONの解釈に失敗したことが含まれる", func(t *testing.T) {
					env := newActiveAppleContract(t)

					o := env.receiveSigned(t, unreadableTransactionInfos[1].signedResult(env.signer))

					assert.ErrorContains(t, o.err, "unmarshal JWS payload")
				})
				for _, tt := range unreadableTransactionInfos {
					t.Run(tt.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
						env := newActiveAppleContract(t)

						o := env.receiveSigned(t, tt.signedResult(env.signer))

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
			t.Run("契約状態がexpiredの契約に、通知種別DID_RENEWで通知の有効期限が読み取れない通知を受け取ったとき", func(t *testing.T) {
				for _, tt := range unreadableExpiries {
					t.Run(tt.name+"、有効期限が読み取れないことを示すエラーで失敗する", func(t *testing.T) {
						o := newExpiredAppleContract(t).receive(t, buildAppleRenewal(tt.expiresAtMillis))

						assert.ErrorIs(t, o.err, subscription.ErrDecodeTransactionInfo)
					})
				}
				for _, tt := range unreadableExpiries {
					t.Run(tt.name+"、エラーの内容に有効期限が無いことが含まれる", func(t *testing.T) {
						o := newExpiredAppleContract(t).receive(t, buildAppleRenewal(tt.expiresAtMillis))

						assert.ErrorContains(t, o.err, "expiresDate is missing")
					})
				}
				for _, tt := range unreadableExpiries {
					t.Run(tt.name+"、契約状態が通知の前と同じになる", func(t *testing.T) {
						o := newExpiredAppleContract(t).receive(t, buildAppleRenewal(tt.expiresAtMillis))

						assert.Equal(t, o.before.Status, o.findContract(t).Status)
					})
				}
				for _, tt := range unreadableExpiries {
					t.Run(tt.name+"、課金期間の終了日時が通知の前と同じになる", func(t *testing.T) {
						o := newExpiredAppleContract(t).receive(t, buildAppleRenewal(tt.expiresAtMillis))

						assert.WithinDuration(t, o.before.CurrentPeriodEnd, o.findContract(t).CurrentPeriodEnd, 0)
					})
				}
				for _, tt := range unreadableExpiries {
					t.Run(tt.name+"、アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
						o := newExpiredAppleContract(t).receive(t, buildAppleRenewal(tt.expiresAtMillis))

						assert.Empty(t, o.listNewPremiumEvents(t))
					})
				}
			})
			t.Run("契約状態がactiveの契約に、通知種別EXPIREDでbundleIdが設定したbundleIDと異なる通知を受け取ったとき", func(t *testing.T) {
				otherBundleNotification := buildAppleNotification("EXPIRED")
				otherBundleNotification.BundleID = appleOtherBundleID
				t.Run("通知のbundleIdが設定した値と異なることを示すエラーで失敗する", func(t *testing.T) {
					o := newActiveAppleContract(t).receive(t, otherBundleNotification)

					assert.ErrorIs(t, o.err, subscription.ErrDecodeNotification)
				})
				t.Run("エラーの内容に通知のbundleIdが含まれる", func(t *testing.T) {
					o := newActiveAppleContract(t).receive(t, otherBundleNotification)

					assert.ErrorContains(t, o.err, "TST-OTHER-BUNDLE-ID")
				})
				t.Run("契約状態が通知の前と同じになる", func(t *testing.T) {
					o := newActiveAppleContract(t).receive(t, otherBundleNotification)

					assert.Equal(t, o.before.Status, o.findContract(t).Status)
				})
				t.Run("アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
					o := newActiveAppleContract(t).receive(t, otherBundleNotification)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
			})
			t.Run("通知の取引識別子に対応する契約が無いとき", func(t *testing.T) {
				receiveWithoutContract := func(t *testing.T) outcome {
					env := newAppleEnv(t)
					signedPayload := env.signer.SignNotification(buildAppleNotification("EXPIRED"))
					return processWithoutContract(t, func() error {
						return env.notifier.HandleNotification(t.Context(), signedPayload)
					})
				}
				t.Run("契約が見つからないエラーで失敗する", func(t *testing.T) {
					o := receiveWithoutContract(t)

					assert.ErrorIs(t, o.err, subscription.ErrSubscriptionNotFound)
				})
				t.Run("エラーの内容に通知の取引識別子が含まれる", func(t *testing.T) {
					o := receiveWithoutContract(t)

					assert.ErrorContains(t, o.err, appleToken)
				})
				t.Run("アウトボックスのpremium_updatedが通知の前から増えない", func(t *testing.T) {
					o := receiveWithoutContract(t)

					assert.Empty(t, o.listNewPremiumEvents(t))
				})
			})
			t.Run("通知本体が読み取れない失敗のあとの回復", func(t *testing.T) {
				t.Run("読み取れない通知で失敗したあとに、同じ契約への有効な通知を受け取ると、その通知の内容どおりに契約状態が変わる", func(t *testing.T) {
					env := newActiveAppleContract(t)
					failed := env.receiveSigned(t, env.signer.SignInvalid(errVerify))
					require.ErrorIs(t, failed.err, subscription.ErrDecodeNotification)

					recovered := env.receive(t, buildAppleNotification("EXPIRED"))
					require.NoError(t, recovered.err)

					assert.Equal(t, "expired", recovered.findContract(t).Status)
				})
			})
		})
	})
}
