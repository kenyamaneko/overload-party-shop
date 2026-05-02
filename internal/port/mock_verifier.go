package port

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
)

// MockReceiptVerifier はテスト用の ReceiptVerifier 実装。
type MockReceiptVerifier struct {
	VerifyPurchaseFn     func(ctx context.Context, purchaseToken string) (*VerifyResult, error)
	VerifySubscriptionFn func(ctx context.Context, purchaseToken string) (*SubscriptionInfo, error)
}

var _ ReceiptVerifier = (*MockReceiptVerifier)(nil)

func (m *MockReceiptVerifier) VerifyPurchase(ctx context.Context, purchaseToken string) (*VerifyResult, error) {
	if m.VerifyPurchaseFn != nil {
		return m.VerifyPurchaseFn(ctx, purchaseToken)
	}
	return &VerifyResult{IsValid: true, TransactionID: "mock-txn-id", ProductID: "mock-product"}, nil
}

func (m *MockReceiptVerifier) VerifySubscription(ctx context.Context, purchaseToken string) (*SubscriptionInfo, error) {
	if m.VerifySubscriptionFn != nil {
		return m.VerifySubscriptionFn(ctx, purchaseToken)
	}
	return &SubscriptionInfo{IsValid: true, ProductID: "mock-sub", TransactionID: "mock-sub-txn"}, nil
}

// MockAppleJWSVerifier はテスト用の AppleJWSVerifier 実装。
// VerifyFn 未指定時は payload を base64 デコードして返す no-op verifier として振る舞う。
type MockAppleJWSVerifier struct {
	VerifyFn func(jws string) ([]byte, error)
}

var _ AppleJWSVerifier = (*MockAppleJWSVerifier)(nil)

func (m *MockAppleJWSVerifier) Verify(jws string) ([]byte, error) {
	if m.VerifyFn != nil {
		return m.VerifyFn(jws)
	}
	return decodeJWSPayloadNoVerify(jws)
}

func decodeJWSPayloadNoVerify(jws string) ([]byte, error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWS format: expected 3 parts")
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}
