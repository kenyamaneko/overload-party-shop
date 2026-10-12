package port

import (
	"context"
	"time"
)

// VerifyResult は単発購入レシート検証の結果。
type VerifyResult struct {
	IsValid   bool
	ProductID string
}

// SubscriptionInfo はサブスクリプションレシート検証の結果。
type SubscriptionInfo struct {
	IsValid   bool
	ProductID string
	ExpiresAt time.Time
}

// ReceiptVerifier はプラットフォーム横断のレシート/購入検証を抽象化する。
type ReceiptVerifier interface {
	VerifyPurchase(ctx context.Context, purchaseToken string) (*VerifyResult, error)
	VerifySubscription(ctx context.Context, purchaseToken string) (*SubscriptionInfo, error)
}

// GoogleSubVerifier は Google Play Developer API からサブスクリプション有効期限を取得する。
type GoogleSubVerifier interface {
	GetSubscriptionExpiry(ctx context.Context, purchaseToken string) (time.Time, error)
}

// AppleJWSVerifier は Apple JWS を Apple Root CA に対して検証し生 payload を返す。
type AppleJWSVerifier interface {
	Verify(jws string) ([]byte, error)
}
