// Package platform は外部プラットフォームアダプタ（Apple App Store Server API、
// Google Play Developer API）と、それらが満たす ReceiptVerifier contract を提供する。
// Apple/Google クライアントは JWT 署名・JWS デコード等の固有プロトコルロジックを
// 持つため repository には置かず、interface と実装を同一パッケージに配置して
// service との循環参照を回避している。
package platform

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
