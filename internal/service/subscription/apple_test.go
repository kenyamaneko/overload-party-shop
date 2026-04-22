//go:build integration

package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type appleTestEnv struct {
	notifier   *AppleNotifier
	subRepo    *postgres.SubscriptionRepository
	premiumPub *fakePremiumBuilder
}

func newAppleTestEnv(t *testing.T) *appleTestEnv {
	t.Helper()
	sharedPg.Truncate(t)

	subRepo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	builder := newFakeEventBuilder(nil)
	// 既定の MockAppleJWSVerifier は 3-part JWS の payload を base64 デコードして
	// 返すだけの no-verify 動作。証明書チェーン検証ロジックは adapter 側で
	// 独立にテストする。
	notifier := NewAppleNotifier(subRepo, builder, &port.MockAppleJWSVerifier{})
	return &appleTestEnv{
		notifier:   notifier,
		subRepo:    subRepo,
		premiumPub: builder.premiumPub,
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

// publish するケース: status 遷移と premium-updated イベント発火を確認する。
// publish が起きない notifType (UNKNOWN_TYPE / 自動更新トグル) は
// TestHandleAppleNotification_NoPublish 側。
func TestHandleAppleNotification_PublishesEvent(t *testing.T) {
	tests := []struct {
		name            string
		notifType       string
		initialStatus   string
		expectedStatus  string
		expectedPremium bool
	}{
		{
			name:            "更新",
			notifType:       appleNotifDIDRenew,
			initialStatus:   apishop.SubscriptionStatusActive,
			expectedStatus:  apishop.SubscriptionStatusActive,
			expectedPremium: true,
		},
		{
			name:           "期限切れ",
			notifType:      appleNotifExpired,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusExpired,
		},
		{
			name:           "猶予期間終了",
			notifType:      appleNotifGracePeriodExpired,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusExpired,
		},
		{
			name:           "返金取消",
			notifType:      appleNotifRevoke,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusRevoked,
		},
		{
			name:           "既に期限切れ状態での EXPIRED 通知",
			notifType:      appleNotifExpired,
			initialStatus:  apishop.SubscriptionStatusExpired,
			expectedStatus: apishop.SubscriptionStatusExpired,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newAppleTestEnv(t)
			token := "apple-token-" + tt.name
			playerID := fmt.Sprintf("aaaaaaaa-%04d-aaaa-aaaa-aaaaaaaaaaaa", i)
			createTestSubscription(t, env.subRepo, apishop.PlatformIOS, playerID, token, tt.initialStatus)

			notifPayload := buildAppleNotificationJWS(tt.notifType, "", token, time.Now().UnixMilli())
			require.NoError(t, env.notifier.HandleNotification(context.Background(), notifPayload))

			updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), apishop.PlatformIOS, token)
			require.NoError(t, err)
			require.NotNil(t, updatedSub)
			assert.Equal(t, tt.expectedStatus, updatedSub.Status)

			require.Len(t, env.premiumPub.calls, 1, "premium-updated を 1 回 enqueue")
			assert.Equal(t, playerID, env.premiumPub.calls[0].PlayerID)
			assert.Equal(t, tt.expectedPremium, env.premiumPub.calls[0].IsPremium)
		})
	}
}

// publish しないケース: status 遷移のみ確認し、premium-updated が enqueue されない
// ことを契約として固定する (未知 type は no-op、自動更新トグルは entitlement 維持で publish しない)。
func TestHandleAppleNotification_NoPublish(t *testing.T) {
	tests := []struct {
		name           string
		notifType      string
		subtype        string
		initialStatus  string
		expectedStatus string
	}{
		{
			name:           "未知の通知タイプは無視",
			notifType:      "UNKNOWN_TYPE",
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusActive,
		},
		{
			name:           "自動更新オン（status 変化なし）",
			notifType:      appleNotifDIDChangeRenewStatus,
			subtype:        appleSubtypeAutoRenewEnabled,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusActive,
		},
		{
			name:           "自動更新オフ（cancelled 遷移）",
			notifType:      appleNotifDIDChangeRenewStatus,
			subtype:        appleSubtypeAutoRenewDisabled,
			initialStatus:  apishop.SubscriptionStatusActive,
			expectedStatus: apishop.SubscriptionStatusCancelled,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newAppleTestEnv(t)
			token := "apple-nopub-token-" + tt.name
			playerID := fmt.Sprintf("eeeeeeee-%04d-eeee-eeee-eeeeeeeeeeee", i)
			createTestSubscription(t, env.subRepo, apishop.PlatformIOS, playerID, token, tt.initialStatus)

			notifPayload := buildAppleNotificationJWS(tt.notifType, tt.subtype, token, time.Now().UnixMilli())
			require.NoError(t, env.notifier.HandleNotification(context.Background(), notifPayload))

			updatedSub, err := env.subRepo.FindSubscriptionByToken(context.Background(), apishop.PlatformIOS, token)
			require.NoError(t, err)
			require.NotNil(t, updatedSub)
			assert.Equal(t, tt.expectedStatus, updatedSub.Status)
			assert.Empty(t, env.premiumPub.calls, "publish 無しの契約")
		})
	}
}

func TestHandleAppleNotification_SubscriptionNotFound(t *testing.T) {
	env := newAppleTestEnv(t)

	notifPayload := buildAppleNotificationJWS(appleNotifExpired, "", "nonexistent-token", time.Now().UnixMilli())
	err := env.notifier.HandleNotification(context.Background(), notifPayload)
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
	assert.Empty(t, env.premiumPub.calls, "副作用無しの契約")
}

// 通知 body 自体が JWS として parse できないケース。
// MockAppleJWSVerifier の no-verify モードでも 3-part 構造チェックは行うため
// invalid な入力はここで弾かれる。
func TestHandleAppleNotification_InvalidJWS(t *testing.T) {
	env := newAppleTestEnv(t)
	err := env.notifier.HandleNotification(context.Background(), "not-a-valid-jws")
	assert.ErrorIs(t, err, ErrDecodeNotification)
}

// 通知 body は valid JWS だが内側の signedTransactionInfo が壊れているケース。
// 内側 payload は base64 decode できても JSON unmarshal に失敗するバイト列にして
// ErrDecodeTransactionInfo を発火させる。
func TestHandleAppleNotification_InvalidTransactionInfoJWS(t *testing.T) {
	env := newAppleTestEnv(t)

	brokenInnerJWS := "h." + base64.RawURLEncoding.EncodeToString([]byte("not json {{{")) + ".s"
	notifPayload := buildAppleJWS(map[string]interface{}{
		"notificationType": appleNotifExpired,
		"data": map[string]interface{}{
			"signedTransactionInfo": brokenInnerJWS,
		},
	})

	err := env.notifier.HandleNotification(context.Background(), notifPayload)
	assert.ErrorIs(t, err, ErrDecodeTransactionInfo)
}
