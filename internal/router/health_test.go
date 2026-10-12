package router_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

func TestHealthCheck(t *testing.T) {
	t.Run("[ヘルスチェック]稼働確認", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("X-Internal-Authヘッダーが無くても、/healthへのGETは200になる", func(t *testing.T) {
				engine := routertest.BuildRouter(t)

				response := routertest.Send(engine, http.MethodGet, "/health", nil, nil)

				assert.Equal(t, http.StatusOK, response.Status)
			})

			t.Run("/healthへのGETの応答本文は、statusがokのJSONになる", func(t *testing.T) {
				engine := routertest.BuildRouter(t)

				response := routertest.Send(engine, http.MethodGet, "/health", nil, nil)

				assert.JSONEq(t, `{"status":"ok"}`, string(response.Body))
			})
		})
	})
}
