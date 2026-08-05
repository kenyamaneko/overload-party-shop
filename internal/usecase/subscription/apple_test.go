//go:build integration

package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type appleTestEnv struct {
	notifier *AppleNotifier
	subRepo  *postgres.SubscriptionRepository
}

func newAppleTestEnv(t *testing.T) *appleTestEnv {
	t.Helper()
	sharedPg.Truncate(t)

	subRepo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	// 既定の MockAppleJWSVerifier は 3-part JWS の payload を base64 デコードして
	// 返すだけの no-verify 動作。証明書チェーン検証ロジックは adapter 側で
	// 独立にテストする。
	notifier := NewAppleNotifier(subRepo, &port.MockAppleJWSVerifier{})
	return &appleTestEnv{
		notifier: notifier,
		subRepo:  subRepo,
	}
}

// buildAppleJWS はテスト用のフェイク JWS（header.payload.signature）を構築する。
func buildAppleJWS(payload interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256"}`))
	data, _ := json.Marshal(payload)
	body := base64.RawURLEncoding.EncodeToString(data)
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	return header + "." + body + "." + sig
}

// buildAppleNotificationJWS は notif type + subtype + 内部 txn JWS をまとめて
// 通知 body の signed JWS に組む。テストの given を 1 行で表現するための helper。
func buildAppleNotificationJWS(notifType, subtype, originalTxnID string, expiresMillis int64) string {
	txnInfo := buildAppleJWS(map[string]interface{}{
		"originalTransactionId": originalTxnID,
		"expiresDate":           expiresMillis,
	})
	return buildAppleJWS(map[string]interface{}{
		"notificationType": notifType,
		"subtype":          subtype,
		"data": map[string]interface{}{
			"signedTransactionInfo": txnInfo,
		},
	})
}

func TestHandleAppleNotification(t *testing.T) {
	t.Run("Apple通知の処理", func(t *testing.T) {
		publishCases := []struct {
			name            string
			notifType       string
			initialStatus   string
			expectedStatus  string
			expectedPremium bool
		}{
			{
				name:            "activeのときDID_RENEW通知で、activeのままisPremium=trueがpublishされる",
				notifType:       appleNotificationDIDRenew,
				initialStatus:   domain.SubscriptionStatusActive,
				expectedStatus:  domain.SubscriptionStatusActive,
				expectedPremium: true,
			},
			{
				name:           "activeのときEXPIRED通知で、expiredになりisPremium=falseがpublishされる",
				notifType:      appleNotificationExpired,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusExpired,
			},
			{
				name:           "activeのときGRACE_PERIOD_EXPIRED通知で、expiredになりisPremium=falseがpublishされる",
				notifType:      appleNotificationGracePeriodExpired,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusExpired,
			},
			{
				name:           "activeのときREVOKE通知で、revokedになりisPremium=falseがpublishされる",
				notifType:      appleNotificationRevoke,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusRevoked,
			},
			{
				name:           "expiredのときEXPIRED通知で、expiredのままisPremium=falseがpublishされる",
				notifType:      appleNotificationExpired,
				initialStatus:  domain.SubscriptionStatusExpired,
				expectedStatus: domain.SubscriptionStatusExpired,
			},
		}
		for i, tt := range publishCases {
			t.Run(tt.name, func(t *testing.T) {
				env := newAppleTestEnv(t)
				token := "apple-token-" + tt.name
				playerID := fmt.Sprintf("aaaaaaaa-%04d-aaaa-aaaa-aaaaaaaaaaaa", i)
				createTestSubscription(t, env.subRepo, domain.PlatformIOS, playerID, token, tt.initialStatus)

				notifPayload := buildAppleNotificationJWS(tt.notifType, "", token, time.Now().UnixMilli())
				require.NoError(t, env.notifier.HandleNotification(context.Background(), notifPayload))

				updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), domain.PlatformIOS, token)
				require.NoError(t, err)
				require.NotNil(t, updatedSub)
				assert.Equal(t, tt.expectedStatus, updatedSub.Status)

				events := selectPremiumUpdatedEvents(t)
				require.Len(t, events, 1, "premium-updated を 1 回 enqueue")
				assert.Equal(t, playerID, events[0].PlayerID)
				assert.Equal(t, tt.expectedPremium, events[0].IsPremium)
			})
		}

		noPublishCases := []struct {
			name           string
			notifType      string
			subtype        string
			initialStatus  string
			expectedStatus string
		}{
			{
				name:           "activeのとき未知の通知タイプでは、activeのままpublishされない",
				notifType:      "UNKNOWN_TYPE",
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusActive,
			},
			{
				name:           "activeのときDID_CHANGE_RENEWAL_STATUS通知のAUTO_RENEW_ENABLEDでは、activeのままpublishされない",
				notifType:      appleNotificationDIDChangeRenewStatus,
				subtype:        appleSubtypeAutoRenewEnabled,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusActive,
			},
			// AUTO_RENEW_DISABLED は cancelled 遷移だが、current_period_end まで特典維持のため premium-updated を publish しない。
			{
				name:           "activeのときDID_CHANGE_RENEWAL_STATUS通知のAUTO_RENEW_DISABLEDでは、cancelledになりpublishされない",
				notifType:      appleNotificationDIDChangeRenewStatus,
				subtype:        appleSubtypeAutoRenewDisabled,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusCancelled,
			},
			{
				name:           "activeのときDID_CHANGE_RENEWAL_STATUS通知のsubtypeが空のとき、activeのままpublishされない",
				notifType:      appleNotificationDIDChangeRenewStatus,
				subtype:        "",
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusActive,
			},
			{
				name:           "activeのときDID_CHANGE_RENEWAL_STATUS通知のsubtypeが未知 (UNKNOWN_SUBTYPE)のとき、activeのままpublishされない",
				notifType:      appleNotificationDIDChangeRenewStatus,
				subtype:        "UNKNOWN_SUBTYPE",
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusActive,
			},
		}
		for i, tt := range noPublishCases {
			t.Run(tt.name, func(t *testing.T) {
				env := newAppleTestEnv(t)
				token := "apple-nopub-token-" + tt.name
				playerID := fmt.Sprintf("eeeeeeee-%04d-eeee-eeee-eeeeeeeeeeee", i)
				createTestSubscription(t, env.subRepo, domain.PlatformIOS, playerID, token, tt.initialStatus)

				notifPayload := buildAppleNotificationJWS(tt.notifType, tt.subtype, token, time.Now().UnixMilli())
				require.NoError(t, env.notifier.HandleNotification(context.Background(), notifPayload))

				updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), domain.PlatformIOS, token)
				require.NoError(t, err)
				require.NotNil(t, updatedSub)
				assert.Equal(t, tt.expectedStatus, updatedSub.Status)
				assert.Empty(t, selectPremiumUpdatedEvents(t), "publish 無しの契約")
			})
		}

		t.Run("activeのときDID_RENEW通知で、premium-updatedがpublishされ期間終了が通知の有効期限に更新される", func(t *testing.T) {
			env := newAppleTestEnv(t)
			token := "apple-renew-expiry-token"
			playerID := "18181818-1818-1818-1818-181818181818"
			createTestSubscription(t, env.subRepo, domain.PlatformIOS, playerID, token, domain.SubscriptionStatusActive)

			const expiresMillis = int64(1_800_000_000_000)
			notifPayload := buildAppleNotificationJWS(appleNotificationDIDRenew, "", token, expiresMillis)
			require.NoError(t, env.notifier.HandleNotification(context.Background(), notifPayload))

			updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), domain.PlatformIOS, token)
			require.NoError(t, err)
			require.NotNil(t, updatedSub)
			assert.True(t, time.UnixMilli(expiresMillis).Equal(updatedSub.CurrentPeriodEnd))

			events := selectPremiumUpdatedEvents(t)
			require.Len(t, events, 1)
			require.NotNil(t, events[0].PremiumExpiresAt)
			assert.True(t, time.UnixMilli(expiresMillis).Equal(*events[0].PremiumExpiresAt))
		})

		t.Run("存在しないtokenの通知のとき、ErrSubscriptionNotFoundになりpublishされない", func(t *testing.T) {
			env := newAppleTestEnv(t)

			notifPayload := buildAppleNotificationJWS(appleNotificationExpired, "", "nonexistent-token", time.Now().UnixMilli())
			err := env.notifier.HandleNotification(context.Background(), notifPayload)
			assert.ErrorIs(t, err, ErrSubscriptionNotFound)
			assert.Empty(t, selectPremiumUpdatedEvents(t), "副作用無しの契約")
		})

		t.Run("JWSとしてparseできない通知のとき、ErrDecodeNotificationになる", func(t *testing.T) {
			// MockAppleJWSVerifier は no-verify でも 3-part 構造チェックを行うため、不正な入力はここで弾かれる。
			env := newAppleTestEnv(t)
			err := env.notifier.HandleNotification(context.Background(), "not-a-valid-jws")
			assert.ErrorIs(t, err, ErrDecodeNotification)
		})

		t.Run("内側のsignedTransactionInfoが壊れているとき、ErrDecodeTransactionInfoになる", func(t *testing.T) {
			// 通知 body は valid JWS だが、内側 payload は base64 decode できても JSON unmarshal に失敗するバイト列にする。
			env := newAppleTestEnv(t)

			brokenInnerJWS := "h." + base64.RawURLEncoding.EncodeToString([]byte("not json {{{")) + ".s"
			notifPayload := buildAppleJWS(map[string]interface{}{
				"notificationType": appleNotificationExpired,
				"data": map[string]interface{}{
					"signedTransactionInfo": brokenInnerJWS,
				},
			})

			err := env.notifier.HandleNotification(context.Background(), notifPayload)
			assert.ErrorIs(t, err, ErrDecodeTransactionInfo)
		})
	})
}
