package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// /health は webhook の登録状態に関わらず常に 200 を返す (router 自体が機能
// していることの sanity check)。webhook 登録の spec とは独立したテスト。
func TestNew_HealthEndpoint(t *testing.T) {
	r := New(rest.NewShopHandler(nil), nil, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// CLAUDE.md 禁止事項「IAP_MODE=local は webhook ルート自体を登録しない
// (nil verifier で silent accept しない意図的設計)」を spec 単位で固定する。
// 各行 = (apple/google handler 設定, path, 期待 status) の 1 spec。
//
// registered 側で 400 が返るのは body 空で ShouldBindJSON が失敗するため。
// 「ルートが登録されていて handler に到達した」ことの証拠として使う
// (未登録なら 404 になるため、登録/未登録の区別がつく)。
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
			r := New(rest.NewShopHandler(nil), tt.appleWH, tt.googleWH)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tt.path, nil))
			assert.Equal(t, tt.wantCode, w.Code)
		})
	}
}
