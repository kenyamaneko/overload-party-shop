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

// publish するケース: status 遷移と premium-updated イベント発火を確認する。
// publish が起きない notifType は TestHandleGoogleNotification_NoPublish 側。
func TestHandleGoogleNotification_PublishesEvent(t *testing.T) {
	tests := []struct {
		name            string
		notifType       int
		initialStatus   string
		expectedStatus  string
		expectedPremium bool
	}{
		{
			name:            "更新",
			notifType:       googleSubRenewed,
			initialStatus:   domain.SubscriptionStatusActive,
			expectedStatus:  domain.SubscriptionStatusActive,
			expectedPremium: true,
		},
		{
			name:           "取消（revoke）",
			notifType:      googleSubRevoked,
			initialStatus:  domain.SubscriptionStatusActive,
			expectedStatus: domain.SubscriptionStatusRevoked,
		},
		{
			name:           "期限切れ",
			notifType:      googleSubExpired,
			initialStatus:  domain.SubscriptionStatusActive,
			expectedStatus: domain.SubscriptionStatusExpired,
		},
		{
			name:            "期限切れからの復活",
			notifType:       googleSubRecovered,
			initialStatus:   domain.SubscriptionStatusExpired,
			expectedStatus:  domain.SubscriptionStatusActive,
			expectedPremium: true,
		},
	}

	for i, tt := range tests {
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
}

// publish しないケース: status 遷移のみ確認し、premium-updated が enqueue されない
// ことを契約として固定する (canceled は期間内 entitlement 維持 / 未対応 type は no-op)。
func TestHandleGoogleNotification_NoPublish(t *testing.T) {
	tests := []struct {
		name           string
		notifType      int
		initialStatus  string
		expectedStatus string
	}{
		{
			name:           "自動更新キャンセル（cancelled 遷移）",
			notifType:      googleSubCanceled,
			initialStatus:  domain.SubscriptionStatusActive,
			expectedStatus: domain.SubscriptionStatusCancelled,
		},
		{
			name:           "未対応の通知タイプは無視（status 変化なし）",
			notifType:      99,
			initialStatus:  domain.SubscriptionStatusActive,
			expectedStatus: domain.SubscriptionStatusActive,
		},
	}

	for i, tt := range tests {
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
}

// HandleNotification が DB write / publish 前に早期 return する非 decode 系入力を網羅する。
// (decode 失敗は TestHandleGoogleNotification_DecodeErrors 側、message 構造そのものが
// 壊れているケースをカバー)
func TestHandleGoogleNotification_EarlyReturn(t *testing.T) {
	tests := []struct {
		name    string
		rtdn    map[string]interface{}
		wantErr error
	}{
		{
			name: "subscriptionNotification 以外の通知は silent skip",
			rtdn: map[string]interface{}{
				"voidedPurchaseNotification": map[string]interface{}{
					"orderId": "GPA.1234",
				},
			},
			wantErr: nil,
		},
		{
			name: "subscription notification だが token が DB に無い",
			rtdn: map[string]interface{}{
				"subscriptionNotification": map[string]interface{}{
					"notificationType": googleSubExpired,
					"purchaseToken":    "nonexistent-google-token",
					"subscriptionId":   "premium_monthly",
				},
			},
			wantErr: ErrSubscriptionNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newGoogleTestEnv(t)
			msg := encodeRTDN(t, tt.rtdn)

			err := env.notifier.HandleNotification(context.Background(), msg)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, selectPremiumUpdatedEvents(t), "副作用無しの契約")
		})
	}
}

// Google Renewed/Recovered は expiryFetcher への到達が必須。
// nil verifier (設定漏れ) と verifier error (Google API 障害) を分岐ごとに検証する。
// case 固有の env 差し替えは configure で受ける — if 分岐をテスト内に入れない。
func TestHandleGoogleNotification_VerifierPaths(t *testing.T) {
	withNilVerifier := func(env *googleTestEnv) {
		env.notifier = NewGoogleNotifier(env.subRepo, nil)
	}
	withVerifierError := func(env *googleTestEnv) {
		env.expiryFetcher.err = fmt.Errorf("google API 500")
	}

	tests := []struct {
		name          string
		notifType     int
		initialStatus string
		configure     func(*googleTestEnv)
		wantSubs      string
	}{
		{
			name:          "verifier 未設定＋更新通知",
			notifType:     googleSubRenewed,
			initialStatus: domain.SubscriptionStatusActive,
			configure:     withNilVerifier,
			wantSubs:      "google subscription verifier not configured",
		},
		{
			name:          "verifier 未設定＋復活通知",
			notifType:     googleSubRecovered,
			initialStatus: domain.SubscriptionStatusExpired,
			configure:     withNilVerifier,
			wantSubs:      "google subscription verifier not configured",
		},
		{
			name:          "verifier エラー＋更新通知",
			notifType:     googleSubRenewed,
			initialStatus: domain.SubscriptionStatusActive,
			configure:     withVerifierError,
			wantSubs:      "get subscription expiry from Google",
		},
		{
			name:          "verifier エラー＋復活通知",
			notifType:     googleSubRecovered,
			initialStatus: domain.SubscriptionStatusExpired,
			configure:     withVerifierError,
			wantSubs:      "get subscription expiry from Google",
		},
	}
	for i, tt := range tests {
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
			assert.Contains(t, err.Error(), tt.wantSubs)
			assert.Empty(t, selectPremiumUpdatedEvents(t), "no publish on verifier failure")
		})
	}
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
			env := newGoogleTestEnv(t)
			msg := GoogleRTDNMessage{}
			msg.Message.Data = tt.input
			err := env.notifier.HandleNotification(context.Background(), msg)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
