package subscriptiontest

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

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
