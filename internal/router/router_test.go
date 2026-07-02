package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeRouterVerifier は router 単体テスト用の internalauth.Verifier 最小 fake。
type fakeRouterVerifier struct {
	playerID string
	err      error
}

// Verify は固定の playerID / err を返す。
func (f fakeRouterVerifier) Verify(string) (string, error) { return f.playerID, f.err }

func testVerifier() internalauth.Verifier {
	return fakeRouterVerifier{}
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

// /health は webhook の登録状態に関わらず常に 200 を返す。
func TestNew_HealthEndpoint(t *testing.T) {
	r := New(rest.NewShopHandler(nil), nil, nil, testVerifier())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// nil handler の webhook はルート自体を登録しないこと、登録時は handler に到達することを固定する。
// (未登録は 404、登録済は body 空で 400 が返るため登録/未登録が区別できる)
func TestNew_WebhookRouteRegistration(t *testing.T) {
	registeredApple := rest.NewAppleWebhookHandler(nil)
	registeredGoogle := rest.NewGoogleWebhookHandler(nil)

	tests := []struct {
		name     string
		appleWH  *rest.AppleWebhookHandler
		googleWH *rest.GoogleWebhookHandler
		path     string
		wantCode int
	}{
		{
			name:     "両方 nil: /webhook/apple は未登録で 404",
			path:     "/webhook/apple",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "両方 nil: /webhook/google は未登録で 404",
			path:     "/webhook/google",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "apple のみ登録: /webhook/apple は handler 到達で 400",
			appleWH:  registeredApple,
			path:     "/webhook/apple",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "apple のみ登録: /webhook/google は未登録で 404",
			appleWH:  registeredApple,
			path:     "/webhook/google",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "google のみ登録: /webhook/google は handler 到達で 400",
			googleWH: registeredGoogle,
			path:     "/webhook/google",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "両方登録: /webhook/apple は handler 到達で 400",
			appleWH:  registeredApple,
			googleWH: registeredGoogle,
			path:     "/webhook/apple",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "両方登録: /webhook/google は handler 到達で 400",
			appleWH:  registeredApple,
			googleWH: registeredGoogle,
			path:     "/webhook/google",
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(rest.NewShopHandler(nil), tt.appleWH, tt.googleWH, testVerifier())
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tt.path, nil))
			assert.Equal(t, tt.wantCode, w.Code)
		})
	}
}

// TestNew_ApiRouteRequiresInternalAuth は /api/v1/shop 配下が auth header 欠落で
// 401 を返し handler に到達しないことを確かめる。
func TestNew_ApiRouteRequiresInternalAuth(t *testing.T) {
	r := New(rest.NewShopHandler(stubShopService{}), nil, nil, fakeRouterVerifier{playerID: "TST-PLAYER-1"})

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
	r := New(rest.NewShopHandler(stubShopService{}), nil, nil, fakeRouterVerifier{err: errors.New("invalid token")})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil)
	req.Header.Set(internalauth.HeaderName, "any.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestNew_ApiRouteWithValidTokenReachesHandler は verifier を通過したリクエストが
// handler の成功応答まで到達することを確かめる。
func TestNew_ApiRouteWithValidTokenReachesHandler(t *testing.T) {
	r := New(rest.NewShopHandler(stubShopService{}), nil, nil, fakeRouterVerifier{playerID: "TST-PLAYER-1"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil)
	req.Header.Set(internalauth.HeaderName, "any.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
