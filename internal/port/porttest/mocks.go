// Package porttest は port のインターフェースを満たすテスト用の実装を提供する。
package porttest

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// MockReceiptVerifier はテスト用の ReceiptVerifier 実装。
type MockReceiptVerifier struct {
	VerifyPurchaseFn     func(ctx context.Context, purchaseToken string) (*port.VerifyResult, error)
	VerifySubscriptionFn func(ctx context.Context, purchaseToken string) (*port.SubscriptionInfo, error)
}

var _ port.ReceiptVerifier = (*MockReceiptVerifier)(nil)

// VerifyPurchase は VerifyPurchaseFn を呼ぶ。未指定のときは有効な検証結果を返す。
func (m *MockReceiptVerifier) VerifyPurchase(ctx context.Context, purchaseToken string) (*port.VerifyResult, error) {
	if m.VerifyPurchaseFn != nil {
		return m.VerifyPurchaseFn(ctx, purchaseToken)
	}
	return &port.VerifyResult{IsValid: true, ProductID: "mock-product"}, nil
}

// VerifySubscription は VerifySubscriptionFn を呼ぶ。未指定のときは有効な検証結果を返す。
func (m *MockReceiptVerifier) VerifySubscription(ctx context.Context, purchaseToken string) (*port.SubscriptionInfo, error) {
	if m.VerifySubscriptionFn != nil {
		return m.VerifySubscriptionFn(ctx, purchaseToken)
	}
	return &port.SubscriptionInfo{IsValid: true, ProductID: "mock-sub"}, nil
}

// MockAppleJWSVerifier はテスト用の AppleJWSVerifier 実装。
type MockAppleJWSVerifier struct {
	VerifyFn func(jws string) ([]byte, error)
}

var _ port.AppleJWSVerifier = (*MockAppleJWSVerifier)(nil)

// Verify は VerifyFn を呼ぶ。未指定のときは署名を検証せずペイロードを返す。
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
