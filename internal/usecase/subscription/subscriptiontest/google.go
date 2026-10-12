package subscriptiontest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

// GoogleExpiryFetcher は Play Developer API が返すサブスクリプションの有効期限の偽物。
type GoogleExpiryFetcher struct {
	expiresAt time.Time
	err       error
}

// NewGoogleExpiryFetcher は、有効期限の取得に失敗する状態の GoogleExpiryFetcher を返す。
func NewGoogleExpiryFetcher() *GoogleExpiryFetcher {
	return &GoogleExpiryFetcher{err: errors.New("有効期限が設定されていない")}
}

// ReturnExpiry は以降の取得が、有効期限 expiresAt で成功するよう設定する。
func (f *GoogleExpiryFetcher) ReturnExpiry(expiresAt time.Time) {
	f.expiresAt = expiresAt
	f.err = nil
}

// FailWith は以降の取得が、エラー err で失敗するよう設定する。
func (f *GoogleExpiryFetcher) FailWith(err error) {
	f.err = err
}

// GetSubscriptionExpiry は設定された有効期限、または設定されたエラーを返す。
func (f *GoogleExpiryFetcher) GetSubscriptionExpiry(_ context.Context, _ string) (time.Time, error) {
	return f.expiresAt, f.err
}

// BuildSubscriptionNotificationMessage は、通知種別 notificationType・購入トークン purchaseToken の subscriptionNotification を持つ RTDN を返す。
func BuildSubscriptionNotificationMessage(t *testing.T, notificationType int, purchaseToken string) subscription.GoogleRTDNMessage {
	t.Helper()
	return messageFromJSON(t, map[string]any{
		"version":         "1.0",
		"packageName":     "TST-package",
		"eventTimeMillis": "1800000000000",
		"subscriptionNotification": map[string]any{
			"version":          "1.0",
			"notificationType": notificationType,
			"purchaseToken":    purchaseToken,
			"subscriptionId":   "TST-subscription",
		},
	})
}

// BuildVoidedPurchaseNotificationMessage は、subscriptionNotification を持たず voidedPurchaseNotification だけを持つ RTDN を返す。
func BuildVoidedPurchaseNotificationMessage(t *testing.T, purchaseToken string) subscription.GoogleRTDNMessage {
	t.Helper()
	return messageFromJSON(t, map[string]any{
		"version":         "1.0",
		"packageName":     "TST-package",
		"eventTimeMillis": "1800000000000",
		"voidedPurchaseNotification": map[string]any{
			"purchaseToken": purchaseToken,
			"orderId":       "TST-order",
			"productType":   1,
			"refundType":    1,
		},
	})
}

// BuildEncodedMessage は、body を base64 にして data に入れた RTDN を返す。
func BuildEncodedMessage(body string) subscription.GoogleRTDNMessage {
	return BuildRawMessage(base64.StdEncoding.EncodeToString([]byte(body)))
}

// BuildRawMessage は、data にそのまま data を入れた RTDN を返す。
func BuildRawMessage(data string) subscription.GoogleRTDNMessage {
	var message subscription.GoogleRTDNMessage
	message.Message.Data = data
	return message
}

func messageFromJSON(t *testing.T, body map[string]any) subscription.GoogleRTDNMessage {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	return BuildEncodedMessage(string(encoded))
}
