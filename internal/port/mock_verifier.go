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

// MockAppleJWSVerifier はテスト用の AppleJWSVerifier 実装。
// VerifyFn 未指定時は、JWS の payload (中央セグメント) を base64 デコードして返す
// no-op verifier として振る舞う (証明書チェーン検証をスキップ)。
type MockAppleJWSVerifier struct {
	VerifyFn func(jws string) ([]byte, error)
}

var _ AppleJWSVerifier = (*MockAppleJWSVerifier)(nil)

// Verify はテスト用の JWS 検証を実行する。
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
