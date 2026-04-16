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
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipJWSVerification はテスト用に jwsVerifyFunc を署名/証明書検証なしの
// デコード関数に差し替える。テスト終了時に元の関数を復元する。
// fake は呼び出し側が必ず 3-part JWS を渡す前提で実装する — 構造不正な入力を
// 検証したいテストは fake を使わず本物の verifyAppleJWS を通すこと。
func skipJWSVerification(t *testing.T) {
	t.Helper()
	orig := jwsVerifyFunc
	jwsVerifyFunc = func(jws string) ([]byte, error) {
		parts := strings.Split(jws, ".")
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
	subRepo        *postgres.SubscriptionRepository
	premiumPub     *fakePremiumBuilder
	builder        *fakeEventBuilder
	googleVerifier *mockGoogleSubVerifier
}

func newTestSubscriptionService(t *testing.T) *testSubEnv {
	t.Helper()
	sharedPg.Truncate(t)

	subRepo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	builder := newFakeEventBuilder(nil, nil)
	gv := &mockGoogleSubVerifier{expiry: time.Now().Add(30 * 24 * time.Hour)}
	svc := NewSubscriptionService(subRepo, builder, gv)
	return &testSubEnv{
		svc:            svc,
		subRepo:        subRepo,
		premiumPub:     builder.premiumPub,
		builder:        builder,
		googleVerifier: gv,
	}
}

// createTestSubscription はテスト用に指定の初期状態でサブスク行を作成する。
// initialStatus は各ケースが明示して受け取る — テスト内で後から mutate せず、
// Given が call 1 回で確定する形にするための必須引数。
func createTestSubscription(t *testing.T, env *testSubEnv, platform, playerID, purchaseToken, initialStatus string) *apishop.Subscription {
	t.Helper()
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)

	sub := &apishop.Subscription{
		PlayerID:           playerID,
		ProductID:          "premium_monthly",
		Status:             initialStatus,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	require.NoError(t, env.subRepo.CreateSubscription(context.Background(), sub, platform, purchaseToken, port.OutboxEvent{}))
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
		initialStatus   string
		expectedStatus  string
		expectPublish   bool
		expectedPremium bool
	}{
		{
			name:            "更新",
			notifType:       "DID_RENEW",
			initialStatus:   apishop.SubscriptionStatusActive,
			expectedStatus:  apishop.SubscriptionStatusActive,
			expectPublish:   true,
			expectedPremium: true,
		},
		{
			name:           "期限切れ",
			notifType:      "EXPIRED",
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusExpired,
			expectPublish:  true,
		},
		{
			name:           "猶予期間終了",
			notifType:      "GRACE_PERIOD_EXPIRED",
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusExpired,
			expectPublish:  true,
		},
		{
			name:           "返金取消",
			notifType:      "REVOKE",
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusRevoked,
			expectPublish:  true,
		},
		{
			name:           "未知の通知タイプは無視",
			notifType:      "UNKNOWN_TYPE",
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusActive,
		},
		{
			name:           "自動更新オン（status 変化なし）",
			notifType:      "DID_CHANGE_RENEWAL_STATUS",
			subtype:        "AUTO_RENEW_ENABLED",
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusActive,
		},
		{
			name:           "自動更新オフ（cancelled 遷移・publish なし）",
			notifType:      "DID_CHANGE_RENEWAL_STATUS",
			subtype:        "AUTO_RENEW_DISABLED",
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusCancelled,
		},
		{
			name:           "既に期限切れ状態での EXPIRED 通知",
			notifType:      "EXPIRED",
			initialStatus:  apishop.SubscriptionStatusExpired,
			expectedStatus: apishop.SubscriptionStatusExpired,
			expectPublish:  true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipJWSVerification(t)
			env := newTestSubscriptionService(t)
			token := "apple-token-" + tt.name
			playerID := fmt.Sprintf("aaaaaaaa-%04d-aaaa-aaaa-aaaaaaaaaaaa", i)
			createTestSubscription(t, env, apishop.PlatformIOS, playerID, token, tt.initialStatus)

			txnInfo := buildAppleJWS(map[string]interface{}{
				"originalTransactionId": token,
				"expiresDate":           time.Now().UnixMilli(),
			})

			notifPayload := buildAppleJWS(map[string]interface{}{
				"notificationType": tt.notifType,
				"subtype":          tt.subtype,
				"data": map[string]interface{}{
					"signedTransactionInfo": txnInfo,
				},
			})

			require.NoError(t, env.svc.HandleAppleNotification(context.Background(), notifPayload))

			updatedSub, ferr := env.subRepo.FindSubscriptionByToken(context.Background(), apishop.PlatformIOS, token)
			require.NoError(t, ferr)
			require.NotNil(t, updatedSub)
			assert.Equal(t, tt.expectedStatus, updatedSub.Status)

			if tt.expectPublish {
				require.NotEmpty(t, env.premiumPub.calls, "expected premium-updated publish")
				last := env.premiumPub.calls[len(env.premiumPub.calls)-1]
				assert.Equal(t, playerID, last.PlayerID)
				assert.Equal(t, tt.expectedPremium, last.IsPremium)
			}
		})
	}
}

func TestHandleGoogleNotification(t *testing.T) {
	tests := []struct {
		name            string
		notifType       int
		initialStatus   string
		expectedStatus  string
		expectPublish   bool
		expectedPremium bool
	}{
		{
			name:            "更新",
			notifType:       googleSubRenewed,
			initialStatus:   apishop.SubscriptionStatusActive,
			expectedStatus:  apishop.SubscriptionStatusActive,
			expectPublish:   true,
			expectedPremium: true,
		},
		{
			name:           "取消（revoke）",
			notifType:      googleSubRevoked,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusRevoked,
			expectPublish:  true,
		},
		{
			name:           "期限切れ",
			notifType:      googleSubExpired,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusExpired,
			expectPublish:  true,
		},
		{
			name:           "自動更新キャンセル（publish なし）",
			notifType:      googleSubCanceled,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusCancelled,
		},
		{
			name:            "期限切れからの復活",
			notifType:       googleSubRecovered,
			initialStatus:   apishop.SubscriptionStatusExpired,
			expectedStatus:  apishop.SubscriptionStatusActive,
			expectPublish:   true,
			expectedPremium: true,
		},
		{
			name:           "未対応の通知タイプは無視",
			notifType:      99,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusActive,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestSubscriptionService(t)
			token := "google-token-" + tt.name
			playerID := fmt.Sprintf("bbbbbbbb-%04d-bbbb-bbbb-bbbbbbbbbbbb", i)
			createTestSubscription(t, env, apishop.PlatformAndroid, playerID, token, tt.initialStatus)

			data, _ := json.Marshal(map[string]interface{}{
				"subscriptionNotification": map[string]interface{}{
					"notificationType": tt.notifType,
					"purchaseToken":    token,
					"subscriptionId":   "premium_monthly",
				},
			})

			msg := GoogleRTDNMessage{}
			msg.Message.Data = base64.StdEncoding.EncodeToString(data)

			require.NoError(t, env.svc.HandleGoogleNotification(context.Background(), msg))

			updatedSub, ferr := env.subRepo.FindSubscriptionByToken(context.Background(), apishop.PlatformAndroid, token)
			require.NoError(t, ferr)
			require.NotNil(t, updatedSub)
			assert.Equal(t, tt.expectedStatus, updatedSub.Status)

			if tt.expectPublish {
				require.NotEmpty(t, env.premiumPub.calls, "expected premium-updated publish")
				last := env.premiumPub.calls[len(env.premiumPub.calls)-1]
				assert.Equal(t, playerID, last.PlayerID)
				assert.Equal(t, tt.expectedPremium, last.IsPremium)
			}
		})
	}
}

func TestHandleGoogleNotification_NonSubscription(t *testing.T) {
	env := newTestSubscriptionService(t)

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

func TestHandleAppleNotification_SubscriptionNotFound(t *testing.T) {
	skipJWSVerification(t)
	env := newTestSubscriptionService(t)

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

	err := env.svc.HandleAppleNotification(context.Background(), notifPayload)
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestHandleGoogleNotification_SubscriptionNotFound(t *testing.T) {
	env := newTestSubscriptionService(t)

	data, _ := json.Marshal(map[string]interface{}{
		"subscriptionNotification": map[string]interface{}{
			"notificationType": googleSubExpired,
			"purchaseToken":    "nonexistent-google-token",
			"subscriptionId":   "premium_monthly",
		},
	})
	msg := GoogleRTDNMessage{}
	msg.Message.Data = base64.StdEncoding.EncodeToString(data)

	err := env.svc.HandleGoogleNotification(context.Background(), msg)
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

// Google Renewed/Recovered は googleVerifier への到達が必須。
// nil verifier (設定漏れ) と verifier error (Google API 障害) を分岐ごとに検証する。
// case 固有の env 差し替えは configure で受ける — if 分岐をテスト内に入れない。
func TestHandleGoogleNotification_VerifierPaths(t *testing.T) {
	withNilVerifier := func(env *testSubEnv) {
		env.svc = NewSubscriptionService(env.subRepo, env.builder, nil)
	}
	withVerifierError := func(env *testSubEnv) {
		env.googleVerifier.err = fmt.Errorf("google API 500")
	}

	tests := []struct {
		name          string
		notifType     int
		initialStatus string
		configure     func(*testSubEnv)
		wantSubs      string
	}{
		{
			name:          "verifier 未設定＋更新通知",
			notifType:     googleSubRenewed,
			initialStatus: apishop.SubscriptionStatusActive,
			configure:     withNilVerifier,
			wantSubs:      "google subscription verifier not configured",
		},
		{
			name:          "verifier 未設定＋復活通知",
			notifType:     googleSubRecovered,
			initialStatus: apishop.SubscriptionStatusExpired,
			configure:     withNilVerifier,
			wantSubs:      "google subscription verifier not configured",
		},
		{
			name:          "verifier エラー＋更新通知",
			notifType:     googleSubRenewed,
			initialStatus: apishop.SubscriptionStatusActive,
			configure:     withVerifierError,
			wantSubs:      "get subscription expiry from Google",
		},
		{
			name:          "verifier エラー＋復活通知",
			notifType:     googleSubRecovered,
			initialStatus: apishop.SubscriptionStatusExpired,
			configure:     withVerifierError,
			wantSubs:      "get subscription expiry from Google",
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestSubscriptionService(t)
			token := fmt.Sprintf("google-verify-token-%d", i)
			playerID := fmt.Sprintf("cccccccc-%04d-cccc-cccc-cccccccccccc", i)
			createTestSubscription(t, env, apishop.PlatformAndroid, playerID, token, tt.initialStatus)
			tt.configure(env)

			data, _ := json.Marshal(map[string]interface{}{
				"subscriptionNotification": map[string]interface{}{
					"notificationType": tt.notifType,
					"purchaseToken":    token,
					"subscriptionId":   "premium_monthly",
				},
			})
			msg := GoogleRTDNMessage{}
			msg.Message.Data = base64.StdEncoding.EncodeToString(data)

			err := env.svc.HandleGoogleNotification(context.Background(), msg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSubs)
			assert.Empty(t, env.premiumPub.calls, "no publish on verifier failure")
		})
	}
}

// 通知 body 自体が JWS として parse できないケース。
// fake を通さず本物の verifyAppleJWS に構造不正を拒否させる。
func TestHandleAppleNotification_InvalidJWS(t *testing.T) {
	env := newTestSubscriptionService(t)
	err := env.svc.HandleAppleNotification(context.Background(), "not-a-valid-jws")
	assert.ErrorIs(t, err, ErrDecodeNotification)
}

// 通知 body は valid JWS だが内側の signedTransactionInfo が壊れているケース。
// fake を使う (署名検証スキップ) が、内側 payload は base64 decode できても
// JSON unmarshal に失敗するバイト列にして ErrDecodeTransactionInfo を発火させる。
func TestHandleAppleNotification_InvalidTransactionInfoJWS(t *testing.T) {
	skipJWSVerification(t)
	env := newTestSubscriptionService(t)

	brokenInnerJWS := "h." + base64.RawURLEncoding.EncodeToString([]byte("not json {{{")) + ".s"
	notifPayload := buildAppleJWS(map[string]interface{}{
		"notificationType": "EXPIRED",
		"data": map[string]interface{}{
			"signedTransactionInfo": brokenInnerJWS,
		},
	})

	err := env.svc.HandleAppleNotification(context.Background(), notifPayload)
	assert.ErrorIs(t, err, ErrDecodeTransactionInfo)
}

func TestHandleGoogleNotification_DecodeErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "base64 として壊れている",
			input:   "!!! not base64 !!!",
			wantErr: ErrDecodeRTDNData,
		},
		{
			name:    "base64 は valid だが中身が JSON でない",
			input:   base64.StdEncoding.EncodeToString([]byte("not valid json {{{")),
			wantErr: ErrUnmarshalRTDNData,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestSubscriptionService(t)
			msg := GoogleRTDNMessage{}
			msg.Message.Data = tt.input
			err := env.svc.HandleGoogleNotification(context.Background(), msg)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
