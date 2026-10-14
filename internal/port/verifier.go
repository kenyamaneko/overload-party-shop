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
	VerifyPurchase(ctx context.Context, productID, purchaseToken string) (*VerifyResult, error)
	VerifySubscription(ctx context.Context, purchaseToken string) (*SubscriptionInfo, error)
}

// GoogleSubscriptionState は Google Play が返す購読の状態。
type GoogleSubscriptionState string

const (
	GoogleSubscriptionStateActive                  GoogleSubscriptionState = "ACTIVE"
	GoogleSubscriptionStateCanceled                GoogleSubscriptionState = "CANCELED"
	GoogleSubscriptionStateInGracePeriod           GoogleSubscriptionState = "IN_GRACE_PERIOD"
	GoogleSubscriptionStateOnHold                  GoogleSubscriptionState = "ON_HOLD"
	GoogleSubscriptionStatePaused                  GoogleSubscriptionState = "PAUSED"
	GoogleSubscriptionStateExpired                 GoogleSubscriptionState = "EXPIRED"
	GoogleSubscriptionStatePending                 GoogleSubscriptionState = "PENDING"
	GoogleSubscriptionStatePendingPurchaseCanceled GoogleSubscriptionState = "PENDING_PURCHASE_CANCELED"
	GoogleSubscriptionStateUnspecified             GoogleSubscriptionState = "UNSPECIFIED"
)

// GoogleSubscription は Google Play が返す購読情報。
type GoogleSubscription struct {
	State     GoogleSubscriptionState
	ProductID string
	ExpiresAt *time.Time
}

// GoogleSubVerifier は Google Play Developer API から購読情報を取得する。
type GoogleSubVerifier interface {
	GetSubscription(ctx context.Context, purchaseToken string) (*GoogleSubscription, error)
}

// AppleJWSVerifier は Apple JWS を Apple Root CA に対して検証し生 payload を返す。
type AppleJWSVerifier interface {
	Verify(jws string) ([]byte, error)
}
