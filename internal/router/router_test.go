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
	r := New(rest.NewShopHandler(nil), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// CLAUDE.md 禁止事項「IAP_MODE=local は webhook ルート自体を登録しない
// (nil verifier で silent accept しない意図的設計)」を spec 単位で固定する。
// 各行 = (webhook handler 設定, path, 期待 status) の 1 spec。
//
// registered 側で 400 が返るのは body 空で ShouldBindJSON が失敗するため。
// 「ルートが登録されていて handler に到達した」ことの証拠として使う
// (未登録なら 404 になるため、登録/未登録の区別がつく)。
func TestNew_WebhookRouteRegistration(t *testing.T) {
	registered := rest.NewWebhookHandler(nil)

	tests := []struct {
		name     string
		webhookH *rest.WebhookHandler
		path     string
		wantCode int
	}{
		{
			name:     "webhookH=nil: /webhook/apple は未登録で 404",
			webhookH: nil,
			path:     "/webhook/apple",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "webhookH=nil: /webhook/google は未登録で 404",
			webhookH: nil,
			path:     "/webhook/google",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "webhookH あり: /webhook/apple は登録済み (handler 到達で 400)",
			webhookH: registered,
			path:     "/webhook/apple",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "webhookH あり: /webhook/google は登録済み (handler 到達で 400)",
			webhookH: registered,
			path:     "/webhook/google",
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(rest.NewShopHandler(nil), tt.webhookH)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tt.path, nil))
			assert.Equal(t, tt.wantCode, w.Code)
		})
	}
}
