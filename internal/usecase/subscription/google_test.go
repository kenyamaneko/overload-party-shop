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
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGoogleSubVerifier struct {
	expiry time.Time
	err    error
}

func (m *mockGoogleSubVerifier) GetSubscriptionExpiry(_ context.Context, _ string) (time.Time, error) {
	return m.expiry, m.err
}

type googleTestEnv struct {
	notifier      *GoogleNotifier
	subRepo       *postgres.SubscriptionRepository
	expiryFetcher *mockGoogleSubVerifier
}

func newGoogleTestEnv(t *testing.T) *googleTestEnv {
	t.Helper()
	sharedPg.Truncate(t)

	subRepo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	gv := &mockGoogleSubVerifier{expiry: time.Now().Add(30 * 24 * time.Hour)}
	notifier := NewGoogleNotifier(subRepo, gv)
	return &googleTestEnv{
		notifier:      notifier,
		subRepo:       subRepo,
		expiryFetcher: gv,
	}
}

// encodeRTDN は RTDN message data フィールド (base64-encoded JSON) を構築する。
func encodeRTDN(t *testing.T, payload map[string]interface{}) GoogleRTDNMessage {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	msg := GoogleRTDNMessage{}
	msg.Message.Data = base64.StdEncoding.EncodeToString(data)
	return msg
}

func TestHandleGoogleNotification(t *testing.T) {
	t.Run("Google通知の処理", func(t *testing.T) {
		publishCases := []struct {
			name            string
			notifType       int
			initialStatus   string
			expectedStatus  string
			expectedPremium bool
		}{
			{
				name:            "activeのときgoogleSubscriptionRenewed通知で、activeのままisPremium=trueがpublishされる",
				notifType:       googleSubscriptionRenewed,
				initialStatus:   domain.SubscriptionStatusActive,
				expectedStatus:  domain.SubscriptionStatusActive,
				expectedPremium: true,
			},
			{
				name:           "activeのときgoogleSubscriptionRevoked通知で、revokedになりisPremium=falseがpublishされる",
				notifType:      googleSubscriptionRevoked,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusRevoked,
			},
			{
				name:           "activeのときgoogleSubscriptionExpired通知で、expiredになりisPremium=falseがpublishされる",
				notifType:      googleSubscriptionExpired,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusExpired,
			},
			{
				name:            "expiredのときgoogleSubscriptionRecovered通知で、activeになりisPremium=trueがpublishされる",
				notifType:       googleSubscriptionRecovered,
				initialStatus:   domain.SubscriptionStatusExpired,
				expectedStatus:  domain.SubscriptionStatusActive,
				expectedPremium: true,
			},
		}
		for i, tt := range publishCases {
			t.Run(tt.name, func(t *testing.T) {
				env := newGoogleTestEnv(t)
				token := "google-token-" + tt.name
				playerID := fmt.Sprintf("bbbbbbbb-%04d-bbbb-bbbb-bbbbbbbbbbbb", i)
				createTestSubscription(t, env.subRepo, domain.PlatformAndroid, playerID, token, tt.initialStatus)

				msg := encodeRTDN(t, map[string]interface{}{
					"subscriptionNotification": map[string]interface{}{
						"notificationType": tt.notifType,
						"purchaseToken":    token,
						"subscriptionId":   "premium_monthly",
					},
				})

				require.NoError(t, env.notifier.HandleNotification(context.Background(), msg))

				updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), domain.PlatformAndroid, token)
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
			notifType      int
			initialStatus  string
			expectedStatus string
		}{
			// googleSubscriptionCanceled は cancelled 遷移だが、current_period_end まで特典維持のため premium-updated を publish しない。
			{
				name:           "activeのときgoogleSubscriptionCanceled通知で、cancelledになりpublishされない",
				notifType:      googleSubscriptionCanceled,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusCancelled,
			},
			{
				name:           "activeのとき未対応の通知タイプ(99)では、activeのままpublishされない",
				notifType:      99,
				initialStatus:  domain.SubscriptionStatusActive,
				expectedStatus: domain.SubscriptionStatusActive,
			},
		}
		for i, tt := range noPublishCases {
			t.Run(tt.name, func(t *testing.T) {
				env := newGoogleTestEnv(t)
				token := "google-nopub-token-" + tt.name
				playerID := fmt.Sprintf("dddddddd-%04d-dddd-dddd-dddddddddddd", i)
				createTestSubscription(t, env.subRepo, domain.PlatformAndroid, playerID, token, tt.initialStatus)

				msg := encodeRTDN(t, map[string]interface{}{
					"subscriptionNotification": map[string]interface{}{
						"notificationType": tt.notifType,
						"purchaseToken":    token,
						"subscriptionId":   "premium_monthly",
					},
				})

				require.NoError(t, env.notifier.HandleNotification(context.Background(), msg))

				updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), domain.PlatformAndroid, token)
				require.NoError(t, err)
				require.NotNil(t, updatedSub)
				assert.Equal(t, tt.expectedStatus, updatedSub.Status)
				assert.Empty(t, selectPremiumUpdatedEvents(t), "publish 無しの契約")
			})
		}

		t.Run("expiredのとき サブスクリプション復旧通知で、premium-updatedがpublishされ期間終了がPlay Developer APIの有効期限に反映される", func(t *testing.T) {
			env := newGoogleTestEnv(t)
			fixedExpiry := time.Now().UTC().Truncate(time.Microsecond).Add(30 * 24 * time.Hour)
			env.expiryFetcher.expiry = fixedExpiry
			token := "google-recovered-expiry-token"
			playerID := "19191919-1919-1919-1919-191919191919"
			createTestSubscription(t, env.subRepo, domain.PlatformAndroid, playerID, token, domain.SubscriptionStatusExpired)

			msg := encodeRTDN(t, map[string]interface{}{
				"subscriptionNotification": map[string]interface{}{
					"notificationType": googleSubscriptionRecovered,
					"purchaseToken":    token,
					"subscriptionId":   "premium_monthly",
				},
			})
			require.NoError(t, env.notifier.HandleNotification(context.Background(), msg))

			updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), domain.PlatformAndroid, token)
			require.NoError(t, err)
			require.NotNil(t, updatedSub)
			assert.True(t, fixedExpiry.Equal(updatedSub.CurrentPeriodEnd))

			events := selectPremiumUpdatedEvents(t)
			require.Len(t, events, 1)
			require.NotNil(t, events[0].PremiumExpiresAt)
			assert.True(t, fixedExpiry.Equal(*events[0].PremiumExpiresAt))
		})

		t.Run("activeのとき サブスクリプション更新通知で、premium-updatedがpublishされ期間終了がPlay Developer APIの有効期限に反映される", func(t *testing.T) {
			env := newGoogleTestEnv(t)
			fixedExpiry := time.Now().UTC().Truncate(time.Microsecond).Add(30 * 24 * time.Hour)
			env.expiryFetcher.expiry = fixedExpiry
			token := "google-renewed-expiry-token"
			playerID := "20202020-2020-2020-2020-202020202020"
			createTestSubscription(t, env.subRepo, domain.PlatformAndroid, playerID, token, domain.SubscriptionStatusActive)

			msg := encodeRTDN(t, map[string]interface{}{
				"subscriptionNotification": map[string]interface{}{
					"notificationType": googleSubscriptionRenewed,
					"purchaseToken":    token,
					"subscriptionId":   "premium_monthly",
				},
			})
			require.NoError(t, env.notifier.HandleNotification(context.Background(), msg))

			updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), domain.PlatformAndroid, token)
			require.NoError(t, err)
			require.NotNil(t, updatedSub)
			assert.True(t, fixedExpiry.Equal(updatedSub.CurrentPeriodEnd))

			events := selectPremiumUpdatedEvents(t)
			require.Len(t, events, 1)
			require.NotNil(t, events[0].PremiumExpiresAt)
			assert.True(t, fixedExpiry.Equal(*events[0].PremiumExpiresAt))
		})

		earlyReturnCases := []struct {
			name    string
			rtdn    map[string]interface{}
			wantErr error
		}{
			{
				name: "subscriptionNotificationが無い通知のとき、エラーにならずpublishされない",
				rtdn: map[string]interface{}{
					"voidedPurchaseNotification": map[string]interface{}{
						"orderId": "GPA.1234",
					},
				},
				wantErr: nil,
			},
			{
				name: "存在しないpurchaseTokenの通知のとき、ErrSubscriptionNotFoundになりpublishされない",
				rtdn: map[string]interface{}{
					"subscriptionNotification": map[string]interface{}{
						"notificationType": googleSubscriptionExpired,
						"purchaseToken":    "nonexistent-google-token",
						"subscriptionId":   "premium_monthly",
					},
				},
				wantErr: ErrSubscriptionNotFound,
			},
		}
		for _, tt := range earlyReturnCases {
			t.Run(tt.name, func(t *testing.T) {
				env := newGoogleTestEnv(t)
				msg := encodeRTDN(t, tt.rtdn)

				err := env.notifier.HandleNotification(context.Background(), msg)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, selectPremiumUpdatedEvents(t), "副作用無しの契約")
			})
		}

		withNilVerifier := func(env *googleTestEnv) {
			env.notifier = NewGoogleNotifier(env.subRepo, nil)
		}
		withVerifierError := func(env *googleTestEnv) {
			env.expiryFetcher.err = fmt.Errorf("google API 500")
		}
		verifierCases := []struct {
			name          string
			notifType     int
			initialStatus string
			configure     func(*googleTestEnv)
			wantContains  []string
		}{
			{
				name:          "verifier未設定のときgoogleSubscriptionRenewed通知で、not configuredエラーになりpublishされない",
				notifType:     googleSubscriptionRenewed,
				initialStatus: domain.SubscriptionStatusActive,
				configure:     withNilVerifier,
				wantContains:  []string{"google subscription verifier not configured"},
			},
			{
				name:          "verifier未設定のときgoogleSubscriptionRecovered通知で、not configuredエラーになりpublishされない",
				notifType:     googleSubscriptionRecovered,
				initialStatus: domain.SubscriptionStatusExpired,
				configure:     withNilVerifier,
				wantContains:  []string{"google subscription verifier not configured"},
			},
			{
				name:          "verifierがエラーを返すときgoogleSubscriptionRenewed通知で、expiry取得エラーになりpublishされない",
				notifType:     googleSubscriptionRenewed,
				initialStatus: domain.SubscriptionStatusActive,
				configure:     withVerifierError,
				wantContains:  []string{"get subscription expiry from Google"},
			},
			{
				name:          "verifierがエラーを返すときgoogleSubscriptionRecovered通知で、expiry取得エラーになりpublishされない",
				notifType:     googleSubscriptionRecovered,
				initialStatus: domain.SubscriptionStatusExpired,
				configure:     withVerifierError,
				wantContains:  []string{"get subscription expiry from Google"},
			},
		}
		for i, tt := range verifierCases {
			t.Run(tt.name, func(t *testing.T) {
				env := newGoogleTestEnv(t)
				token := fmt.Sprintf("google-verify-token-%d", i)
				playerID := fmt.Sprintf("cccccccc-%04d-cccc-cccc-cccccccccccc", i)
				createTestSubscription(t, env.subRepo, domain.PlatformAndroid, playerID, token, tt.initialStatus)
				tt.configure(env)

				msg := encodeRTDN(t, map[string]interface{}{
					"subscriptionNotification": map[string]interface{}{
						"notificationType": tt.notifType,
						"purchaseToken":    token,
						"subscriptionId":   "premium_monthly",
					},
				})

				err := env.notifier.HandleNotification(context.Background(), msg)
				require.Error(t, err)
				for _, want := range tt.wantContains {
					assert.Contains(t, err.Error(), want)
				}
				assert.Empty(t, selectPremiumUpdatedEvents(t), "no publish on verifier failure")
			})
		}

		decodeErrorCases := []struct {
			name    string
			input   string
			wantErr error
		}{
			{
				name:    "dataがbase64として壊れているとき、ErrDecodeRTDNDataになる",
				input:   "!!! not base64 !!!",
				wantErr: ErrDecodeRTDNData,
			},
			{
				name:    "dataのbase64はvalidだがJSONでないとき、ErrUnmarshalRTDNDataになる",
				input:   base64.StdEncoding.EncodeToString([]byte("not valid json {{{")),
				wantErr: ErrUnmarshalRTDNData,
			},
		}
		for _, tt := range decodeErrorCases {
			t.Run(tt.name, func(t *testing.T) {
				env := newGoogleTestEnv(t)
				msg := GoogleRTDNMessage{}
				msg.Message.Data = tt.input
				err := env.notifier.HandleNotification(context.Background(), msg)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}
	})
}
