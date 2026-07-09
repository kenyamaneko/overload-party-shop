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
	t.Run("Apple 通知の処理", func(t *testing.T) {
		publishCases := []struct {
			name            string
			notifType       string
			initialStatus   string
			expectedStatus  string
			expectedPremium bool
		}{
			{
				name:            "active のとき DID_RENEW 通知で、active のまま isPremium=true が publish される",
				notifType:       appleNotificationDIDRenew,
				initialStatus:   domain.SubscriptionStatusActive,
				expectedStatus:  domain.SubscriptionStatusActive,
				expectedPremium: true,
			},
			{
				name:           "active のとき EXPIRED 通知で、expired になり isPremium=false が publish される",
				notifType:      appleNotificationExpired,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusExpired,
			},
			{
				name:           "active のとき GRACE_PERIOD_EXPIRED 通知で、expired になり isPremium=false が publish される",
				notifType:      appleNotificationGracePeriodExpired,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusExpired,
			},
			{
				name:           "active のとき REVOKE 通知で、revoked になり isPremium=false が publish される",
				notifType:      appleNotificationRevoke,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusRevoked,
			},
			{
				name:           "expired のとき EXPIRED 通知で、expired のまま isPremium=false が publish される",
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
				name:           "active のとき未知の通知タイプでは、active のまま publish されない",
				notifType:      "UNKNOWN_TYPE",
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusActive,
			},
			{
				name:           "active のとき DID_CHANGE_RENEWAL_STATUS 通知の AUTO_RENEW_ENABLED では、active のまま publish されない",
				notifType:      appleNotificationDIDChangeRenewStatus,
				subtype:        appleSubtypeAutoRenewEnabled,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusActive,
			},
			// AUTO_RENEW_DISABLED は cancelled 遷移だが、current_period_end まで特典維持のため premium-updated を publish しない。
			{
				name:           "active のとき DID_CHANGE_RENEWAL_STATUS 通知の AUTO_RENEW_DISABLED では、cancelled になり publish されない",
				notifType:      appleNotificationDIDChangeRenewStatus,
				subtype:        appleSubtypeAutoRenewDisabled,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusCancelled,
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

		t.Run("存在しない token の通知のとき、ErrSubscriptionNotFound になり publish されない", func(t *testing.T) {
			env := newAppleTestEnv(t)

			notifPayload := buildAppleNotificationJWS(appleNotificationExpired, "", "nonexistent-token", time.Now().UnixMilli())
			err := env.notifier.HandleNotification(context.Background(), notifPayload)
			assert.ErrorIs(t, err, ErrSubscriptionNotFound)
			assert.Empty(t, selectPremiumUpdatedEvents(t), "副作用無しの契約")
		})

		t.Run("JWS として parse できない通知のとき、ErrDecodeNotification になる", func(t *testing.T) {
			// MockAppleJWSVerifier は no-verify でも 3-part 構造チェックを行うため、不正な入力はここで弾かれる。
			env := newAppleTestEnv(t)
			err := env.notifier.HandleNotification(context.Background(), "not-a-valid-jws")
			assert.ErrorIs(t, err, ErrDecodeNotification)
		})

		t.Run("内側の signedTransactionInfo が壊れているとき、ErrDecodeTransactionInfo になる", func(t *testing.T) {
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
