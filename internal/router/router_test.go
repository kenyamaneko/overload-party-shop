package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeRouterVerifier は router 単体テスト用の port.InternalAuthVerifier 最小 fake。
// webhook / health の検証では auth middleware の経路を通らないため Verify は呼ばれない。
type fakeRouterVerifier struct{}

func (fakeRouterVerifier) Verify(string) (string, error) { return "", nil }

func testVerifier() port.InternalAuthVerifier {
	return fakeRouterVerifier{}
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
