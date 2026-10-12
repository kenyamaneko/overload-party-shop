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

// AuthVerifier は受け取ったトークンを記録し、決まった結果を返す内部認証の検証器の偽物。
type AuthVerifier struct {
	mu       sync.Mutex
	verifyFn func(token string) (string, error)
	tokens   []string
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

// Verify はトークンを記録し、設定された結果を返す。
func (v *AuthVerifier) Verify(token string) (string, error) {
	v.mu.Lock()
	v.tokens = append(v.tokens, token)
	v.mu.Unlock()
	return v.verifyFn(token)
}

// Tokens は検証に渡されたトークンを渡された順に返す。
func (v *AuthVerifier) Tokens() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.tokens...)
}

// PushValidator は検証に渡された値を記録し、決まった結果を返すプッシュ認証の検証器の偽物。
type PushValidator struct {
	mu        sync.Mutex
	email     string
	err       error
	idTokens  []string
	audiences []string
}

// NewPushValidator は署名者のメールアドレス email を返し、err が nil でなければ検証に失敗する偽物を返す。
func NewPushValidator(email string, err error) *PushValidator {
	return &PushValidator{email: email, err: err}
}

// Validate は渡された値を記録し、設定された結果を返す。
func (v *PushValidator) Validate(_ context.Context, idToken, audience string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.idTokens = append(v.idTokens, idToken)
	v.audiences = append(v.audiences, audience)
	return v.email, v.err
}

// IDTokens は検証に渡された ID トークンを渡された順に返す。
func (v *PushValidator) IDTokens() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.idTokens...)
}

// Audiences は検証に渡された audience を渡された順に返す。
func (v *PushValidator) Audiences() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.audiences...)
}

// ShopCall はショップのサービスへの 1 回の呼び出しの記録。
type ShopCall struct {
	Method        string
	PlayerID      string
	ProductID     string
	Platform      string
	PurchaseToken string
}

// ShopService は呼び出しを記録するショップのサービスの偽物。
type ShopService struct {
	mu          sync.Mutex
	calls       []ShopCall
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

// GetProducts は呼び出しを記録し、設定された結果を返す。
func (s *ShopService) GetProducts(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error) {
	s.record(ShopCall{Method: "GetProducts", PlayerID: playerID})
	return s.getProducts(ctx, playerID)
}

// Purchase は呼び出しを記録し、成功を返す。
func (s *ShopService) Purchase(_ context.Context, playerID, productID, platform, purchaseToken string) error {
	s.record(ShopCall{Method: "Purchase", PlayerID: playerID, ProductID: productID, Platform: platform, PurchaseToken: purchaseToken})
	return nil
}

// Subscribe は呼び出しを記録し、有効期限のゼロ値で成功を返す。
func (s *ShopService) Subscribe(_ context.Context, playerID, productID, platform, purchaseToken string) (time.Time, error) {
	s.record(ShopCall{Method: "Subscribe", PlayerID: playerID, ProductID: productID, Platform: platform, PurchaseToken: purchaseToken})
	return time.Time{}, nil
}

// Calls は記録された呼び出しを呼ばれた順に返す。
func (s *ShopService) Calls() []ShopCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ShopCall(nil), s.calls...)
}

func (s *ShopService) record(call ShopCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

// AppleNotifier は受け取った署名付きデータを記録する Apple 通知の処理の偽物。
type AppleNotifier struct {
	mu       sync.Mutex
	payloads []string

	// Err が nil でないとき、通知の処理はこのエラーで失敗する。
	Err error
	// Delay は通知の処理が結果を返すまでに待つ時間。
	Delay time.Duration
}

// HandleNotification は署名付きデータを記録し、設定された結果を返す。
func (n *AppleNotifier) HandleNotification(_ context.Context, signedPayload string) error {
	n.mu.Lock()
	n.payloads = append(n.payloads, signedPayload)
	n.mu.Unlock()
	time.Sleep(n.Delay)
	return n.Err
}

// SignedPayloads は通知の処理に渡された署名付きデータを渡された順に返す。
func (n *AppleNotifier) SignedPayloads() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.payloads...)
}

// GoogleNotifier は受け取った通知の data を記録する GooglePlay 通知の処理の偽物。
type GoogleNotifier struct {
	mu   sync.Mutex
	data []string
}

// HandleNotification は通知の data を記録し、成功を返す。
func (n *GoogleNotifier) HandleNotification(_ context.Context, msg subscription.GoogleRTDNMessage) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.data = append(n.data, msg.Message.Data)
	return nil
}

// Data は通知の処理に渡された data を渡された順に返す。
func (n *GoogleNotifier) Data() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.data...)
}

// ReceiptVerifier は渡された購入トークンを記録し、決まった結果を返すストアのレシート検証の偽物。
type ReceiptVerifier struct {
	mu                 sync.Mutex
	mock               porttest.MockReceiptVerifier
	purchaseValid      bool
	purchaseErr        error
	subscriptionValid  bool
	subscriptionErr    error
	subscriptionExpiry time.Time
	purchaseTokens     []string
	subscriptionTokens []string
}

// NewReceiptVerifier は購入も購読も有効と判定する偽物を返す。
func NewReceiptVerifier() *ReceiptVerifier {
	v := &ReceiptVerifier{
		purchaseValid:      true,
		subscriptionValid:  true,
		subscriptionExpiry: FarFutureExpiry,
	}
	v.mock.VerifyPurchaseFn = func(_ context.Context, purchaseToken string) (*port.VerifyResult, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		v.purchaseTokens = append(v.purchaseTokens, purchaseToken)
		return &port.VerifyResult{IsValid: v.purchaseValid, ProductID: verifiedProductID}, v.purchaseErr
	}
	v.mock.VerifySubscriptionFn = func(_ context.Context, purchaseToken string) (*port.SubscriptionInfo, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		v.subscriptionTokens = append(v.subscriptionTokens, purchaseToken)
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

// PurchaseTokens は購入の検証に渡された購入トークンを渡された順に返す。
func (v *ReceiptVerifier) PurchaseTokens() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.purchaseTokens...)
}

// SubscriptionTokens は購読の検証に渡された購入トークンを渡された順に返す。
func (v *ReceiptVerifier) SubscriptionTokens() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.subscriptionTokens...)
}

// AppleJWSVerifier は登録された署名付きデータだけを検証に成功させる Apple の署名付きデータの検証の偽物。
type AppleJWSVerifier struct {
	mu       sync.Mutex
	mock     porttest.MockAppleJWSVerifier
	payloads map[string][]byte
	received []string
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

// Received は検証に渡された署名付きデータを渡された順に返す。
func (v *AppleJWSVerifier) Received() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.received...)
}

func (v *AppleJWSVerifier) verify(signed string) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.received = append(v.received, signed)
	payload, ok := v.payloads[signed]
	if !ok {
		return nil, errUnregisteredSignature
	}
	return payload, nil
}

// GoogleExpiryFetcher は渡された購入トークンを記録し、決まった有効期限を返す GooglePlay の有効期限取得の偽物。
type GoogleExpiryFetcher struct {
	mu     sync.Mutex
	expiry time.Time
	err    error
	tokens []string
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

// GetSubscriptionExpiry は購入トークンを記録し、設定された結果を返す。
func (f *GoogleExpiryFetcher) GetSubscriptionExpiry(_ context.Context, purchaseToken string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = append(f.tokens, purchaseToken)
	return f.expiry, f.err
}

// Tokens は有効期限の取得に渡された購入トークンを渡された順に返す。
func (f *GoogleExpiryFetcher) Tokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tokens...)
}
