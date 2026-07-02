package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
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

// TestNew_HealthEndpoint は /health が webhook の登録状態に関わらず常に 200 を返すことを固定する。
func TestNew_HealthEndpoint(t *testing.T) {
	r := New(rest.NewShopHandler(nil), nil, nil, testVerifier())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestNew_RegisteredWebhookRouteReachesNotifier は登録済 webhook ルートが valid リクエストを
// handler 経由で notifier まで届け ack (200) を返すこと、および片側登録が他方のルートを
// 巻き込まず未登録 (404) のままであることを固定する。
func TestNew_RegisteredWebhookRouteReachesNotifier(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		body        string
		siblingPath string
		build       func() (*rest.AppleWebhookHandler, *rest.GoogleWebhookHandler, func() int)
	}{
		{
			name:        "apple のみ登録",
			path:        appleWebhookPath,
			body:        appleValidBody,
			siblingPath: googleWebhookPath,
			build: func() (*rest.AppleWebhookHandler, *rest.GoogleWebhookHandler, func() int) {
				n := &fakeAppleNotifier{}
				return rest.NewAppleWebhookHandler(n), nil, func() int { return n.calls }
			},
		},
		{
			name:        "google のみ登録",
			path:        googleWebhookPath,
			body:        googleValidBody,
			siblingPath: appleWebhookPath,
			build: func() (*rest.AppleWebhookHandler, *rest.GoogleWebhookHandler, func() int) {
				n := &fakeGoogleNotifier{}
				return nil, rest.NewGoogleWebhookHandler(n), func() int { return n.calls }
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appleWH, googleWH, notifierCalls := tt.build()
			r := New(rest.NewShopHandler(nil), appleWH, googleWH, testVerifier())

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, 1, notifierCalls())

			sibling := httptest.NewRecorder()
			r.ServeHTTP(sibling, httptest.NewRequest(http.MethodPost, tt.siblingPath, nil))
			assert.Equal(t, http.StatusNotFound, sibling.Code)
		})
	}
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
			r := New(rest.NewShopHandler(nil), nil, nil, testVerifier())
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusNotFound, w.Code)
		})
	}
}
