package routertest

import (
	"encoding/base64"
	"testing"
	"time"
)

// BundleID は Apple 通知の処理が受け付けるアプリのバンドル ID。
const BundleID = "com.tst.shop"

// googleRenewalNotificationType は GooglePlay のサブスクリプション更新通知の種別。
const googleRenewalNotificationType = 2

// BuildAppleWebhookBody は Apple 通知の POST 本文を返す。
func BuildAppleWebhookBody(t *testing.T, signedPayload string) []byte {
	t.Helper()
	return MarshalJSONBody(t, map[string]string{"signedPayload": signedPayload})
}

// BuildGoogleWebhookBody は GooglePlay 通知の POST 本文を返す。
func BuildGoogleWebhookBody(t *testing.T, data string) []byte {
	t.Helper()
	return MarshalJSONBody(t, map[string]any{"message": map[string]string{"data": data}})
}

// BuildGooglePushHeaders は GooglePlay 通知のプッシュ認証を通る要求ヘッダーを返す。
func BuildGooglePushHeaders() map[string]string {
	return map[string]string{"Authorization": "Bearer TST-ID-TOKEN"}
}

// BuildPurchaseRequestBody は購入とサブスクリプション購入の POST 本文を返す。
func BuildPurchaseRequestBody(t *testing.T, productID, platform, purchaseToken string) []byte {
	t.Helper()
	return MarshalJSONBody(t, map[string]string{
		"product_id":     productID,
		"platform":       platform,
		"purchase_token": purchaseToken,
	})
}

// RegisterRenewalNotification は署名付きデータ signedPayload を、取引情報 signedTransactionInfo を持つ更新通知として検証に成功させる。
func (v *AppleJWSVerifier) RegisterRenewalNotification(t *testing.T, signedPayload, signedTransactionInfo string) {
	t.Helper()
	v.Register(signedPayload, MarshalJSONBody(t, map[string]any{
		"notificationType": "DID_RENEW",
		"data": map[string]any{
			"bundleId":              BundleID,
			"environment":           "Sandbox",
			"signedTransactionInfo": signedTransactionInfo,
		},
	}))
}

// RegisterTransaction は署名付きデータ signedTransactionInfo を、取引 transactionID の有効期限が expiresAt の取引情報として検証に成功させる。
func (v *AppleJWSVerifier) RegisterTransaction(t *testing.T, signedTransactionInfo, transactionID string, expiresAt time.Time) {
	t.Helper()
	v.Register(signedTransactionInfo, MarshalJSONBody(t, map[string]any{
		"originalTransactionId": transactionID,
		"transactionId":         transactionID,
		"bundleId":              BundleID,
		"expiresDate":           expiresAt.UnixMilli(),
	}))
}

// BuildGoogleRenewalData は購入トークン purchaseToken の更新通知を、通知の data として base64 で符号化した値を返す。
func BuildGoogleRenewalData(t *testing.T, purchaseToken string) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(MarshalJSONBody(t, map[string]any{
		"version":     "1.0",
		"packageName": "com.tst.shop",
		"subscriptionNotification": map[string]any{
			"version":          "1.0",
			"notificationType": googleRenewalNotificationType,
			"purchaseToken":    purchaseToken,
			"subscriptionId":   "tst.subscription",
		},
	}))
}

// EncodeBase64 は text を base64 で符号化した値を返す。
func EncodeBase64(text string) string {
	return base64.StdEncoding.EncodeToString([]byte(text))
}
