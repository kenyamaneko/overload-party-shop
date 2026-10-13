// Package routertest は router の入口から shop を確かめるテストの共有部品を提供する。
package routertest

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/port/porttest"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

const verifiedProductID = "TST-VERIFIED-PRODUCT"

// FarFutureExpiry は期限切れにならない購読の有効期限。
var FarFutureExpiry = time.Date(2999, 12, 31, 0, 0, 0, 0, time.UTC)

// ExpiredAt は期限切れの購読の有効期限。
var ExpiredAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

var errUnregisteredSignature = errors.New("TST-UNREGISTERED-SIGNATURE")

// AuthVerifier は決まった結果を返す内部認証の検証器の偽物。
type AuthVerifier struct {
	verifyFn func(token string) (string, error)
}

// NewFixedAuthVerifier はどのトークンもプレイヤー ID playerID として検証する偽物を返す。
func NewFixedAuthVerifier(playerID string) *AuthVerifier {
	return &AuthVerifier{verifyFn: func(string) (string, error) { return playerID, nil }}
}

// NewEchoAuthVerifier はトークンの値をそのままプレイヤー ID として検証する偽物を返す。
func NewEchoAuthVerifier() *AuthVerifier {
	return &AuthVerifier{verifyFn: func(token string) (string, error) { return token, nil }}
}

// NewFailingAuthVerifier はどのトークンも err で検証に失敗させる偽物を返す。
func NewFailingAuthVerifier(err error) *AuthVerifier {
	return &AuthVerifier{verifyFn: func(string) (string, error) { return "", err }}
}

// Verify は設定された結果を返す。
func (v *AuthVerifier) Verify(token string) (string, error) {
	return v.verifyFn(token)
}

// PushValidator は決まった結果を返すプッシュ認証の検証器の偽物。
type PushValidator struct {
	email string
	err   error
}

// NewPushValidator は署名者のメールアドレス email を返し、err が nil でなければ検証に失敗する偽物を返す。
func NewPushValidator(email string, err error) *PushValidator {
	return &PushValidator{email: email, err: err}
}

// Validate は設定された結果を返す。
func (v *PushValidator) Validate(context.Context, string, string) (string, error) {
	return v.email, v.err
}

// ShopService はショップのサービスの偽物。
type ShopService struct {
	getProducts func(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error)
}

// NewShopService は商品一覧の取得で商品なしを返す偽物を返す。
func NewShopService() *ShopService {
	return &ShopService{getProducts: func(context.Context, string) ([]domain.ProductWithOwnership, error) {
		return nil, nil
	}}
}

// NewPanickingShopService は商品一覧の取得の処理が途中で異常終了する偽物を返す。
func NewPanickingShopService() *ShopService {
	return &ShopService{getProducts: func(context.Context, string) ([]domain.ProductWithOwnership, error) {
		panic("TST-PANIC")
	}}
}

// GetProducts は設定された結果を返す。
func (s *ShopService) GetProducts(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error) {
	return s.getProducts(ctx, playerID)
}

// Purchase は成功を返す。
func (s *ShopService) Purchase(context.Context, string, string, string, string) error {
	return nil
}

// Subscribe は有効期限のゼロ値で成功を返す。
func (s *ShopService) Subscribe(context.Context, string, string, string, string) (time.Time, error) {
	return time.Time{}, nil
}

// AppleNotifier は決まった結果を返す Apple 通知の処理の偽物。
type AppleNotifier struct {
	// Err が nil でないとき、通知の処理はこのエラーで失敗する。
	Err error
	// Delay は通知の処理が結果を返すまでに待つ時間。
	Delay time.Duration
}

// HandleNotification は設定された結果を返す。
func (n *AppleNotifier) HandleNotification(context.Context, string) error {
	time.Sleep(n.Delay)
	return n.Err
}

// GoogleNotifier は成功を返す GooglePlay 通知の処理の偽物。
type GoogleNotifier struct{}

// HandleNotification は成功を返す。
func (n *GoogleNotifier) HandleNotification(context.Context, subscription.GoogleRTDNMessage) error {
	return nil
}

// ReceiptVerifier は決まった結果を返すストアのレシート検証の偽物。
type ReceiptVerifier struct {
	mu                 sync.Mutex
	mock               porttest.MockReceiptVerifier
	purchaseValid      bool
	purchaseErr        error
	subscriptionValid  bool
	subscriptionErr    error
	subscriptionExpiry time.Time
}

// NewReceiptVerifier は購入も購読も有効と判定する偽物を返す。
func NewReceiptVerifier() *ReceiptVerifier {
	v := &ReceiptVerifier{
		purchaseValid:      true,
		subscriptionValid:  true,
		subscriptionExpiry: FarFutureExpiry,
	}
	v.mock.VerifyPurchaseFn = func(context.Context, string) (*port.VerifyResult, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		return &port.VerifyResult{IsValid: v.purchaseValid, ProductID: verifiedProductID}, v.purchaseErr
	}
	v.mock.VerifySubscriptionFn = func(context.Context, string) (*port.SubscriptionInfo, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		return &port.SubscriptionInfo{IsValid: v.subscriptionValid, ProductID: verifiedProductID, ExpiresAt: v.subscriptionExpiry}, v.subscriptionErr
	}
	return v
}

// Port は usecase に渡すレシート検証を返す。
func (v *ReceiptVerifier) Port() port.ReceiptVerifier {
	return &v.mock
}

// AcceptPurchase は以降の購入を有効と判定させる。
func (v *ReceiptVerifier) AcceptPurchase() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.purchaseValid = true
	v.purchaseErr = nil
}

// RejectPurchase は以降の購入を無効と判定させる。
func (v *ReceiptVerifier) RejectPurchase() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.purchaseValid = false
	v.purchaseErr = nil
}

// FailPurchase は以降の購入の検証を err で実行できなくさせる。
func (v *ReceiptVerifier) FailPurchase(err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.purchaseErr = err
}

// AcceptSubscription は以降の購読を有効と判定させ、有効期限を expiresAt にする。
func (v *ReceiptVerifier) AcceptSubscription(expiresAt time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.subscriptionValid = true
	v.subscriptionErr = nil
	v.subscriptionExpiry = expiresAt
}

// RejectSubscription は以降の購読を無効と判定させる。
func (v *ReceiptVerifier) RejectSubscription() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.subscriptionValid = false
	v.subscriptionErr = nil
}

// AppleJWSVerifier は登録された署名付きデータだけを検証に成功させる Apple の署名付きデータの検証の偽物。
type AppleJWSVerifier struct {
	mu       sync.Mutex
	mock     porttest.MockAppleJWSVerifier
	payloads map[string][]byte
}

// NewAppleJWSVerifier は何も登録されていない偽物を返す。登録されていない署名付きデータは検証に失敗する。
func NewAppleJWSVerifier() *AppleJWSVerifier {
	v := &AppleJWSVerifier{payloads: map[string][]byte{}}
	v.mock.VerifyFn = v.verify
	return v
}

// Port は usecase に渡す署名付きデータの検証を返す。
func (v *AppleJWSVerifier) Port() port.AppleJWSVerifier {
	return &v.mock
}

// Register は署名付きデータ signed を検証に成功させ、検証済みの本文を payload にする。
func (v *AppleJWSVerifier) Register(signed string, payload []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.payloads[signed] = payload
}

func (v *AppleJWSVerifier) verify(signed string) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	payload, ok := v.payloads[signed]
	if !ok {
		return nil, errUnregisteredSignature
	}
	return payload, nil
}

// GoogleExpiryFetcher は決まった有効期限を返す GooglePlay の有効期限取得の偽物。
type GoogleExpiryFetcher struct {
	mu     sync.Mutex
	expiry time.Time
	err    error
}

// NewGoogleExpiryFetcher は有効期限 expiry を返す偽物を返す。
func NewGoogleExpiryFetcher(expiry time.Time) *GoogleExpiryFetcher {
	return &GoogleExpiryFetcher{expiry: expiry}
}

// FailWith は以降の有効期限の取得を err で失敗させる。
func (f *GoogleExpiryFetcher) FailWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// GetSubscriptionExpiry は設定された結果を返す。
func (f *GoogleExpiryFetcher) GetSubscriptionExpiry(context.Context, string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.expiry, f.err
}
