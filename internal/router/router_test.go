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

// fakeRouterVerifier は router 単体テスト用の internalauth.Verifier 最小 fake。
// webhook / health の検証では auth middleware の経路を通らないため Verify は呼ばれない。
type fakeRouterVerifier struct{}

func (fakeRouterVerifier) Verify(string) (string, error) { return "", nil }

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

func TestNew(t *testing.T) {
	t.Run("ルーターの webhook 配線", func(t *testing.T) {
		t.Run("/health は webhook の登録状態に関わらず 200 を返す", func(t *testing.T) {
			r := New(rest.NewShopHandler(nil), nil, nil, testVerifier())
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
			assert.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("apple webhook のみ登録したとき、apple は notifier に到達し google は 404 のままになる", func(t *testing.T) {
			n := &fakeAppleNotifier{}
			r := New(rest.NewShopHandler(nil), rest.NewAppleWebhookHandler(n), nil, testVerifier())

			w := postWebhook(r, appleWebhookPath, appleValidBody)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, 1, n.calls)

			sibling := postWebhook(r, googleWebhookPath, googleValidBody)
			assert.Equal(t, http.StatusNotFound, sibling.Code)
		})

		t.Run("google webhook のみ登録したとき、google は notifier に到達し apple は 404 のままになる", func(t *testing.T) {
			n := &fakeGoogleNotifier{}
			r := New(rest.NewShopHandler(nil), nil, rest.NewGoogleWebhookHandler(n), testVerifier())

			w := postWebhook(r, googleWebhookPath, googleValidBody)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, 1, n.calls)

			sibling := postWebhook(r, appleWebhookPath, appleValidBody)
			assert.Equal(t, http.StatusNotFound, sibling.Code)
		})

		t.Run("webhook handler が nil のとき、ルートが未登録のまま 404 になる", func(t *testing.T) {
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
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					r := New(rest.NewShopHandler(nil), nil, nil, testVerifier())
					w := postWebhook(r, tc.path, tc.body)
					assert.Equal(t, http.StatusNotFound, w.Code)
				})
			}
		})
	})

	t.Run("/api/v1/shop の内部認証配線", func(t *testing.T) {
		t.Run("auth header が欠落しているとき、401 になり handler に到達しない", func(t *testing.T) {
			// VerifyFn 未設定: header 欠落時は middleware が verifier に到達しないことの検出を兼ねる
			r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{})

			cases := []struct {
				name   string
				method string
				path   string
			}{
				{
					name:   "GET /products",
					method: http.MethodGet,
					path:   "/api/v1/shop/products",
				},
				{
					name:   "POST /purchase",
					method: http.MethodPost,
					path:   "/api/v1/shop/purchase",
				},
				{
					name:   "POST /subscribe",
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
		})

		t.Run("verifier がエラーを返すとき、401 になり handler に到達しない", func(t *testing.T) {
			r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{
				VerifyFn: func(string) (string, error) { return "", errors.New("invalid token") },
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("有効なトークンのとき、handler の応答まで到達する", func(t *testing.T) {
			r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{
				VerifyFn: func(string) (string, error) { return "TST-PLAYER-1", nil },
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	})
}
