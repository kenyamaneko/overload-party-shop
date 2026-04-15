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

// IAP_MODE=local では webhookH が nil で呼ばれる。CLAUDE.md の禁止事項
// 「IAP_MODE=local は webhook ルート自体を登録しない（nil verifier で silent
// accept しない意図的設計）」を直接検証する。nil verifier を持つ WebhookHandler
// に POST が到達する状況を、そもそも router レイヤで物理的に作らせない契約。
func TestNew_RouteRegistration(t *testing.T) {
	shopH := rest.NewShopHandler(nil)

	tests := []struct {
		name            string
		webhookH        *rest.WebhookHandler
		webhookWantCode int
		healthWantCode  int
	}{
		{
			name:            "webhookH=nil なら /webhook/* は登録されず 404",
			webhookH:        nil,
			webhookWantCode: http.StatusNotFound,
			healthWantCode:  http.StatusOK,
		},
		{
			name:            "webhookH あり なら /webhook/* は登録される",
			webhookH:        rest.NewWebhookHandler(nil),
			webhookWantCode: http.StatusBadRequest, // 空 body で bind 失敗 → 登録されている証拠
			healthWantCode:  http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(shopH, tt.webhookH)

			healthW := httptest.NewRecorder()
			r.ServeHTTP(healthW, httptest.NewRequest(http.MethodGet, "/health", nil))
			assert.Equal(t, tt.healthWantCode, healthW.Code)

			for _, path := range []string{"/webhook/apple", "/webhook/google"} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
				assert.Equal(t, tt.webhookWantCode, w.Code, "path=%s", path)
			}
		})
	}
}
