package router_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

func TestInternalAuthVerification(t *testing.T) {
	routes := routertest.BuildShopRoutes(t)

	t.Run("[ショップAPI認証]内部認証の検証", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("X-Internal-Authヘッダーが無いとき", func(t *testing.T) {
				for _, route := range routes {
					t.Run(route.Name+"のとき、401になる", func(t *testing.T) {
						engine := routertest.BuildRouter(t)

						response := routertest.Send(engine, route.Method, route.Path, route.Body, nil)

						assert.Equal(t, http.StatusUnauthorized, response.Status)
					})
				}

				for _, route := range routes {
					t.Run(route.Name+"のとき、errorにヘッダーが必要であることを示すメッセージが入る", func(t *testing.T) {
						engine := routertest.BuildRouter(t)

						response := routertest.Send(engine, route.Method, route.Path, route.Body, nil)

						assert.Equal(t, "X-Internal-Auth header is required", response.ErrorMessage(t))
					})
				}
			})

			t.Run("トークンの検証が失敗するとき", func(t *testing.T) {
				buildFailingAuth := func() *routertest.AuthVerifier {
					return routertest.NewFailingAuthVerifier(errors.New("TST-VERIFICATION-FAILURE"))
				}
				headers := map[string]string{internalauth.HeaderName: "TST-TOKEN-1"}

				for _, route := range routes {
					t.Run(route.Name+"のとき、401になる", func(t *testing.T) {
						engine := routertest.BuildRouter(t, routertest.WithAuth(buildFailingAuth()))

						response := routertest.Send(engine, route.Method, route.Path, route.Body, headers)

						assert.Equal(t, http.StatusUnauthorized, response.Status)
					})
				}

				for _, route := range routes {
					t.Run(route.Name+"のとき、errorにトークンが不正であることを示すメッセージが入る", func(t *testing.T) {
						engine := routertest.BuildRouter(t, routertest.WithAuth(buildFailingAuth()))

						response := routertest.Send(engine, route.Method, route.Path, route.Body, headers)

						assert.Equal(t, "invalid internal auth token", response.ErrorMessage(t))
					})
				}
			})
		})
	})
}
