package port

import (
	"context"
	"time"
)

// VerifyResult は単発購入レシート検証の結果を保持する。
type VerifyResult struct {
	IsValid       bool
	TransactionID string
	ProductID     string
	PurchaseTime  time.Time
}

// SubscriptionInfo はサブスクリプション固有の検証結果を保持する。
type SubscriptionInfo struct {
	IsValid        bool
	ProductID      string
	TransactionID  string
	ExpiresAt      time.Time
	IsAutoRenewing bool
}

// ReceiptVerifier はプラットフォーム横断のレシート/購入検証を抽象化する。
type ReceiptVerifier interface {
	// VerifyPurchase は単発購入レシートを検証する。
	VerifyPurchase(ctx context.Context, purchaseToken string) (*VerifyResult, error)

	// VerifySubscription はサブスクリプションレシートを検証する。
	VerifySubscription(ctx context.Context, purchaseToken string) (*SubscriptionInfo, error)
}

// GoogleSubVerifier は Google Play Developer API からサブスクリプション有効期限を取得する。
// webhook (RTDN) 駆動の Renewed / Recovered 処理で使用する。
type GoogleSubVerifier interface {
	GetSubscriptionExpiry(ctx context.Context, purchaseToken string) (time.Time, error)
}

// AppleJWSVerifier は Apple App Store Server Notifications V2 で送られてくる
// signed JWS を Apple Root CA に対して検証し、生 payload を返す。
// webhook 駆動の通知処理で使用する。
type AppleJWSVerifier interface {
	Verify(jws string) ([]byte, error)
}
