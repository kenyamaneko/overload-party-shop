package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipJWSVerification はテスト用に jwsVerifyFunc を署名/証明書検証なしの
// デコード関数に差し替える。テスト終了時に元の関数を復元する。
func skipJWSVerification(t *testing.T) {
	t.Helper()
	orig := jwsVerifyFunc
	jwsVerifyFunc = func(jws string) ([]byte, error) {
		parts := strings.Split(jws, ".")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid JWS format")
		}
		return base64.RawURLEncoding.DecodeString(parts[1])
	}
	t.Cleanup(func() { jwsVerifyFunc = orig })
}

type mockGoogleSubVerifier struct {
	expiry time.Time
	err    error
}

func (m *mockGoogleSubVerifier) GetSubscriptionExpiry(_ context.Context, _ string) (time.Time, error) {
	return m.expiry, m.err
}

type testSubEnv struct {
	svc            *SubscriptionService
	subRepo        *repository.MockSubscriptionRepository
	premiumPub     *fakePremiumPublisher
	googleVerifier *mockGoogleSubVerifier
}

func newTestSubscriptionService() *testSubEnv {
	subRepo := repository.NewMockSubscriptionRepository()
	premiumPub := &fakePremiumPublisher{}
	gv := &mockGoogleSubVerifier{expiry: time.Now().Add(30 * 24 * time.Hour)}
	svc := NewSubscriptionService(subRepo, premiumPub, gv)
	return &testSubEnv{svc: svc, subRepo: subRepo, premiumPub: premiumPub, googleVerifier: gv}
}

func createTestSubscription(env *testSubEnv, playerID, purchaseToken string) *apishop.Subscription {
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)

	sub := &apishop.Subscription{
		PlayerID:           playerID,
		ProductID:          "premium_monthly",
		Platform:           "ios",
		PurchaseToken:      purchaseToken,
		Status:             apishop.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_ = env.subRepo.CreateSubscription(context.Background(), sub)
	return sub
}

// buildAppleJWS はテスト用のフェイク JWS（header.payload.signature）を構築する。
func buildAppleJWS(payload interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256"}`))
	data, _ := json.Marshal(payload)
	body := base64.RawURLEncoding.EncodeToString(data)
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	return header + "." + body + "." + sig
}

func TestHandleAppleNotification(t *testing.T) {
	tests := []struct {
		name            string
		notifType       string
		subtype         string
		preExpire       bool
		expectedStatus  string
		expectPublish   bool
		expectedPremium bool
	}{
		{"Renewal", "DID_RENEW", "", false, apishop.SubscriptionStatusActive, true, true},
		{"Expired", "EXPIRED", "", false, apishop.SubscriptionStatusExpired, true, false},
		{"GracePeriodExpired", "GRACE_PERIOD_EXPIRED", "", false, apishop.SubscriptionStatusExpired, true, false},
		{"Revoke", "REVOKE", "", false, apishop.SubscriptionStatusRevoked, true, false},
		{"UnknownType", "UNKNOWN_TYPE", "", false, apishop.SubscriptionStatusActive, false, true},
		{"AutoRenewEnabled", "DID_CHANGE_RENEWAL_STATUS", "AUTO_RENEW_ENABLED", false, apishop.SubscriptionStatusActive, false, true},
		{"AutoRenewDisabled", "DID_CHANGE_RENEWAL_STATUS", "AUTO_RENEW_DISABLED", false, apishop.SubscriptionStatusCancelled, false, true},
		{"AlreadyExpired", "EXPIRED", "", true, apishop.SubscriptionStatusExpired, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipJWSVerification(t)
			env := newTestSubscriptionService()
			sub := createTestSubscription(env, "p1", "apple-token-"+tt.name)

			if tt.preExpire {
				sub.Status = apishop.SubscriptionStatusExpired
				_ = env.subRepo.UpdateSubscription(context.Background(), sub)
			}

			txnInfo := buildAppleJWS(map[string]interface{}{
				"originalTransactionId": sub.PurchaseToken,
				"expiresDate":           time.Now().UnixMilli(),
			})

			notifData := map[string]interface{}{
				"notificationType": tt.notifType,
				"data": map[string]interface{}{
					"signedTransactionInfo": txnInfo,
				},
			}
			if tt.subtype != "" {
				notifData["subtype"] = tt.subtype
			}
			notifPayload := buildAppleJWS(notifData)

			err := env.svc.HandleAppleNotification(context.Background(), notifPayload)
			require.NoError(t, err)

			updatedSub, _ := env.subRepo.FindSubscriptionByToken(context.Background(), sub.PurchaseToken)
			require.NotNil(t, updatedSub)
			assert.Equal(t, tt.expectedStatus, updatedSub.Status)

			if tt.expectPublish {
				require.NotEmpty(t, env.premiumPub.calls, "expected premium-updated publish")
				last := env.premiumPub.calls[len(env.premiumPub.calls)-1]
				assert.Equal(t, "p1", last.PlayerID)
				assert.Equal(t, tt.expectedPremium, last.IsPremium)
			}
		})
	}
}

func TestHandleGoogleNotification(t *testing.T) {
	tests := []struct {
		name            string
		notifType       int
		preExpire       bool
		expectedStatus  string
		expectPublish   bool
		expectedPremium bool
	}{
		{"Renewed", googleSubRenewed, false, apishop.SubscriptionStatusActive, true, true},
		{"Revoked", googleSubRevoked, false, apishop.SubscriptionStatusRevoked, true, false},
		{"Expired", googleSubExpired, false, apishop.SubscriptionStatusExpired, true, false},
		{"Canceled", googleSubCanceled, false, apishop.SubscriptionStatusCancelled, false, true},
		{"Recovered", googleSubRecovered, true, apishop.SubscriptionStatusActive, true, true},
		{"UnhandledType", 99, false, apishop.SubscriptionStatusActive, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestSubscriptionService()
			sub := createTestSubscription(env, "p1", "google-token-"+tt.name)

			if tt.preExpire {
				sub.Status = apishop.SubscriptionStatusExpired
				_ = env.subRepo.UpdateSubscription(context.Background(), sub)
			}

			data, _ := json.Marshal(map[string]interface{}{
				"subscriptionNotification": map[string]interface{}{
					"notificationType": tt.notifType,
					"purchaseToken":    sub.PurchaseToken,
					"subscriptionId":   "premium_monthly",
				},
			})

			msg := GoogleRTDNMessage{}
			msg.Message.Data = base64.StdEncoding.EncodeToString(data)

			err := env.svc.HandleGoogleNotification(context.Background(), msg)
			require.NoError(t, err)

			updatedSub, _ := env.subRepo.FindSubscriptionByToken(context.Background(), sub.PurchaseToken)
			require.NotNil(t, updatedSub)
			assert.Equal(t, tt.expectedStatus, updatedSub.Status)

			if tt.expectPublish {
				require.NotEmpty(t, env.premiumPub.calls, "expected premium-updated publish")
				last := env.premiumPub.calls[len(env.premiumPub.calls)-1]
				assert.Equal(t, "p1", last.PlayerID)
				assert.Equal(t, tt.expectedPremium, last.IsPremium)
			}
		})
	}
}

func TestHandleGoogleNotification_NonSubscription(t *testing.T) {
	env := newTestSubscriptionService()

	data, _ := json.Marshal(map[string]interface{}{
		"voidedPurchaseNotification": map[string]interface{}{
			"orderId": "GPA.1234",
		},
	})

	msg := GoogleRTDNMessage{}
	msg.Message.Data = base64.StdEncoding.EncodeToString(data)

	err := env.svc.HandleGoogleNotification(context.Background(), msg)
	require.NoError(t, err)
}

func TestHandleNotification_SubscriptionNotFound(t *testing.T) {
	tests := []struct {
		name     string
		platform string
	}{
		{name: "Apple", platform: "apple"},
		{name: "Google", platform: "google"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipJWSVerification(t)
			env := newTestSubscriptionService()

			var err error
			if tt.platform == "apple" {
				txnInfo := buildAppleJWS(map[string]interface{}{
					"originalTransactionId": "nonexistent-token",
					"expiresDate":           time.Now().UnixMilli(),
				})

				notifPayload := buildAppleJWS(map[string]interface{}{
					"notificationType": "EXPIRED",
					"data": map[string]interface{}{
						"signedTransactionInfo": txnInfo,
					},
				})

				err = env.svc.HandleAppleNotification(context.Background(), notifPayload)
			} else {
				data, _ := json.Marshal(map[string]interface{}{
					"subscriptionNotification": map[string]interface{}{
						"notificationType": googleSubExpired,
						"purchaseToken":    "nonexistent-google-token",
						"subscriptionId":   "premium_monthly",
					},
				})

				msg := GoogleRTDNMessage{}
				msg.Message.Data = base64.StdEncoding.EncodeToString(data)

				err = env.svc.HandleGoogleNotification(context.Background(), msg)
			}

			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), "subscription not found"))
		})
	}
}

func TestHandleNotification_DecodeErrors(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		input       string
		errContains string
	}{
		{
			name:        "Apple/InvalidJWS",
			platform:    "apple",
			input:       "not-a-valid-jws",
			errContains: "decode notification",
		},
		{
			name:        "Apple/InvalidTransactionInfoJWS",
			platform:    "apple",
			errContains: "decode transaction info",
		},
		{
			name:        "Google/InvalidBase64",
			platform:    "google",
			input:       "!!! not base64 !!!",
			errContains: "decode RTDN data",
		},
		{
			name:        "Google/InvalidJSON",
			platform:    "google",
			input:       base64.StdEncoding.EncodeToString([]byte("not valid json {{{")),
			errContains: "unmarshal RTDN data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipJWSVerification(t)
			env := newTestSubscriptionService()

			var err error
			if tt.platform == "apple" {
				input := tt.input
				if input == "" {
					input = buildAppleJWS(map[string]interface{}{
						"notificationType": "EXPIRED",
						"data": map[string]interface{}{
							"signedTransactionInfo": "not-a-valid-jws",
						},
					})
				}
				err = env.svc.HandleAppleNotification(context.Background(), input)
			} else {
				msg := GoogleRTDNMessage{}
				msg.Message.Data = tt.input
				err = env.svc.HandleGoogleNotification(context.Background(), msg)
			}

			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tt.errContains))
		})
	}
}
