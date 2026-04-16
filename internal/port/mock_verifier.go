package port

import "context"

// MockReceiptVerifier はテスト用の ReceiptVerifier 実装。
type MockReceiptVerifier struct {
	VerifyPurchaseFn     func(ctx context.Context, purchaseToken string) (*VerifyResult, error)
	VerifySubscriptionFn func(ctx context.Context, purchaseToken string) (*SubscriptionInfo, error)
}

var _ ReceiptVerifier = (*MockReceiptVerifier)(nil)

// VerifyPurchase はテスト用の単発購入検証を実行する。
func (m *MockReceiptVerifier) VerifyPurchase(ctx context.Context, purchaseToken string) (*VerifyResult, error) {
	if m.VerifyPurchaseFn != nil {
		return m.VerifyPurchaseFn(ctx, purchaseToken)
	}
	return &VerifyResult{IsValid: true, TransactionID: "mock-txn-id", ProductID: "mock-product"}, nil
}

// VerifySubscription はテスト用のサブスクリプション検証を実行する。
func (m *MockReceiptVerifier) VerifySubscription(ctx context.Context, purchaseToken string) (*SubscriptionInfo, error) {
	if m.VerifySubscriptionFn != nil {
		return m.VerifySubscriptionFn(ctx, purchaseToken)
	}
	return &SubscriptionInfo{IsValid: true, ProductID: "mock-sub", TransactionID: "mock-sub-txn"}, nil
}
