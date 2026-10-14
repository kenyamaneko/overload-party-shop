// Package porttest は port のインターフェースを満たすテスト用の実装を提供する。
package porttest

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// MockReceiptVerifier はテスト用の ReceiptVerifier 実装。
type MockReceiptVerifier struct {
	VerifyPurchaseFn     func(ctx context.Context, productID, purchaseToken string) (*port.VerifyResult, error)
	VerifySubscriptionFn func(ctx context.Context, purchaseToken string) (*port.SubscriptionInfo, error)
}

var _ port.ReceiptVerifier = (*MockReceiptVerifier)(nil)

// VerifyPurchase は VerifyPurchaseFn を呼ぶ。未指定のときは有効な検証結果を返す。
func (m *MockReceiptVerifier) VerifyPurchase(ctx context.Context, productID, purchaseToken string) (*port.VerifyResult, error) {
	if m.VerifyPurchaseFn != nil {
		return m.VerifyPurchaseFn(ctx, productID, purchaseToken)
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

// MockGoogleSubVerifier はテスト用の GoogleSubVerifier 実装。設定するまで、購読情報の取得は失敗する。
type MockGoogleSubVerifier struct {
	mu           sync.Mutex
	subscription *port.GoogleSubscription
	err          error
}

var _ port.GoogleSubVerifier = (*MockGoogleSubVerifier)(nil)

// NewMockGoogleSubVerifier は、購読情報の取得が失敗する状態の MockGoogleSubVerifier を返す。
func NewMockGoogleSubVerifier() *MockGoogleSubVerifier {
	return &MockGoogleSubVerifier{err: errors.New("購読情報が設定されていない")}
}

// Return は以降の取得が、購読の状態 state・商品 productID・有効期限 expiresAt で成功するよう設定する。
func (m *MockGoogleSubVerifier) Return(state port.GoogleSubscriptionState, productID string, expiresAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscription = &port.GoogleSubscription{State: state, ProductID: productID, ExpiresAt: &expiresAt}
	m.err = nil
}

// ReturnWithoutExpiry は以降の取得が、購読の状態 state・商品 productID で、有効期限なしで成功するよう設定する。
func (m *MockGoogleSubVerifier) ReturnWithoutExpiry(state port.GoogleSubscriptionState, productID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscription = &port.GoogleSubscription{State: state, ProductID: productID}
	m.err = nil
}

// FailWith は以降の取得が、エラー err で失敗するよう設定する。
func (m *MockGoogleSubVerifier) FailWith(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscription = nil
	m.err = err
}

// GetSubscription は設定された購読情報、または設定されたエラーを返す。
func (m *MockGoogleSubVerifier) GetSubscription(_ context.Context, _ string) (*port.GoogleSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.subscription, m.err
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
