package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// stubShopService は固定値を返す shop usecase スタブ。
type stubShopService struct{}

// GetProducts は商品なしを返す。
func (stubShopService) GetProducts(context.Context, string) ([]domain.ProductWithOwnership, error) {
	return []domain.ProductWithOwnership{}, nil
}

// Purchase は常に成功する。
func (stubShopService) Purchase(context.Context, string, string, string, string) error {
	return nil
}

// Subscribe は期限なしで成功する。
func (stubShopService) Subscribe(context.Context, string, string, string, string) (*time.Time, error) {
	return nil, nil
}

// fakeAppleNotifier は webhook が handler を経て notifier まで到達したかを呼び出し回数で観測する。
type fakeAppleNotifier struct {
	calls int
}

// HandleNotification は Apple webhook の到達を記録する。
func (f *fakeAppleNotifier) HandleNotification(_ context.Context, _ string) error {
	f.calls++
	return nil
}

// fakeGoogleNotifier は webhook が handler を経て notifier まで到達したかを呼び出し回数で観測する。
type fakeGoogleNotifier struct {
	calls int
}

// HandleNotification は Google webhook の到達を記録する。
func (f *fakeGoogleNotifier) HandleNotification(_ context.Context, _ subscription.GoogleRTDNMessage) error {
	f.calls++
	return nil
}

const (
	appleWebhookPath  = "/webhook/apple"
	googleWebhookPath = "/webhook/google"
	appleValidBody    = `{"signedPayload":"fake-jws"}`
	googleValidBody   = `{"message":{"data":"fake-base64"}}`
)

// postWebhook は JSON body 付き POST を router に投げ、レスポンスを返す。
func postWebhook(r http.Handler, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// TestNew_HealthEndpoint は /health が webhook の登録状態に関わらず常に 200 を返すことを固定する。
func TestNew_HealthEndpoint(t *testing.T) {
	r := New(rest.NewShopHandler(nil), nil, nil, &internalauth.MockVerifier{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestNew_AppleWebhookReachesNotifier は apple のみ登録した router で valid リクエストが
// notifier まで届き ack (200) が返ること、google 側は未登録 (404) のままであることを固定する。
func TestNew_AppleWebhookReachesNotifier(t *testing.T) {
	n := &fakeAppleNotifier{}
	r := New(rest.NewShopHandler(nil), rest.NewAppleWebhookHandler(n), nil, &internalauth.MockVerifier{})

	w := postWebhook(r, appleWebhookPath, appleValidBody)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, n.calls)

	sibling := postWebhook(r, googleWebhookPath, googleValidBody)
	assert.Equal(t, http.StatusNotFound, sibling.Code)
}

// TestNew_GoogleWebhookReachesNotifier は google のみ登録した router で valid リクエストが
// notifier まで届き ack (200) が返ること、apple 側は未登録 (404) のままであることを固定する。
func TestNew_GoogleWebhookReachesNotifier(t *testing.T) {
	n := &fakeGoogleNotifier{}
	r := New(rest.NewShopHandler(nil), nil, rest.NewGoogleWebhookHandler(n), &internalauth.MockVerifier{})

	w := postWebhook(r, googleWebhookPath, googleValidBody)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, n.calls)

	sibling := postWebhook(r, appleWebhookPath, appleValidBody)
	assert.Equal(t, http.StatusNotFound, sibling.Code)
}

// TestNew_NilWebhookHandlerLeavesRouteUnregistered は nil の webhook handler ではルート自体が
// 登録されず、valid リクエストでも 404 を返すことを固定する。
func TestNew_NilWebhookHandlerLeavesRouteUnregistered(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "apple",
			path: appleWebhookPath,
			body: appleValidBody,
		},
		{
			name: "google",
			path: googleWebhookPath,
			body: googleValidBody,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(rest.NewShopHandler(nil), nil, nil, &internalauth.MockVerifier{})
			w := postWebhook(r, tt.path, tt.body)
			assert.Equal(t, http.StatusNotFound, w.Code)
		})
	}
}

// TestNew_ApiRouteRequiresInternalAuth は /api/v1/shop 配下が auth header 欠落で
// 401 を返し handler に到達しないことを確かめる。
func TestNew_ApiRouteRequiresInternalAuth(t *testing.T) {
	// VerifyFn 未設定: header 欠落時は middleware が verifier に到達しないことの検出を兼ねる
	r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{})

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "GET /products は auth header 欠落で 401",
			method: http.MethodGet,
			path:   "/api/v1/shop/products",
		},
		{
			name:   "POST /purchase は auth header 欠落で 401",
			method: http.MethodPost,
			path:   "/api/v1/shop/purchase",
		},
		{
			name:   "POST /subscribe は auth header 欠落で 401",
			method: http.MethodPost,
			path:   "/api/v1/shop/subscribe",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

// TestNew_ApiRouteRejectsVerifierError は verifier が error を返すと 401 を返し
// handler に到達しないことを確かめる。
func TestNew_ApiRouteRejectsVerifierError(t *testing.T) {
	r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{
		VerifyFn: func(string) (string, error) { return "", errors.New("invalid token") },
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil)
	req.Header.Set(internalauth.HeaderName, "any.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestNew_ApiRouteWithValidTokenReachesHandler は verifier を通過したリクエストが
// handler の成功応答まで到達することを確かめる。
func TestNew_ApiRouteWithValidTokenReachesHandler(t *testing.T) {
	r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{
		VerifyFn: func(string) (string, error) { return "TST-PLAYER-1", nil },
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil)
	req.Header.Set(internalauth.HeaderName, "any.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
