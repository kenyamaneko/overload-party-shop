package stub

import (
	"context"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// Verifier は全レシートを valid 扱いする port.ReceiptVerifier の stub 実装。
// 実ストアの認証情報なしで購入・サブスクリプションフローを通すために使う。
type Verifier struct{}

// NewVerifier は stub の Verifier を構築する。
func NewVerifier() *Verifier {
	return &Verifier{}
}

var _ port.ReceiptVerifier = (*Verifier)(nil)

// VerifyPurchase はレシートを検証せず常に有効として返す。
func (v *Verifier) VerifyPurchase(_ context.Context, purchaseToken string) (*port.VerifyResult, error) {
	return &port.VerifyResult{IsValid: true, TransactionID: purchaseToken}, nil
}

// VerifySubscription はレシートを検証せず常に有効として返す。
func (v *Verifier) VerifySubscription(_ context.Context, purchaseToken string) (*port.SubscriptionInfo, error) {
	return &port.SubscriptionInfo{IsValid: true, TransactionID: purchaseToken}, nil
}
