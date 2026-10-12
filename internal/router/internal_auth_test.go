package router_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

func TestInternalAuthVerification(t *testing.T) {
	routes := routertest.BuildShopRoutes(t)

	t.Run("[ショップAPI認証]内部認証の検証", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("X-Internal-Authヘッダーが無いとき", func(t *testing.T) {
				for _, route := range routes.All {
					t.Run(route.Name+"のとき、401になる", func(t *testing.T) {
						engine := routertest.BuildRouter(t)

						response := routertest.Send(engine, route.Method, route.Path, route.Body, nil)

						assert.Equal(t, http.StatusUnauthorized, response.Status)
					})
				}

				for _, route := range routes.All {
					t.Run(route.Name+"のとき、errorにヘッダーが必要であることを示すメッセージが入る", func(t *testing.T) {
						engine := routertest.BuildRouter(t)

						response := routertest.Send(engine, route.Method, route.Path, route.Body, nil)

						assert.Equal(t, "X-Internal-Auth header is required", response.ErrorMessage(t))
					})
				}

				for _, route := range routes.All {
					t.Run(route.Name+"のとき、サービスの呼び出しは記録されない", func(t *testing.T) {
						service := routertest.NewShopService()
						engine := routertest.BuildRouter(t, routertest.WithService(service))

						routertest.Send(engine, route.Method, route.Path, route.Body, nil)

						assert.Empty(t, service.Calls())
					})
				}
			})

			t.Run("トークンの検証が失敗するとき", func(t *testing.T) {
				buildFailingAuth := func() *routertest.AuthVerifier {
					return routertest.NewFailingAuthVerifier(errors.New("TST-VERIFICATION-FAILURE"))
				}
				headers := map[string]string{internalauth.HeaderName: "TST-TOKEN-1"}

				for _, route := range routes.All {
					t.Run(route.Name+"のとき、401になる", func(t *testing.T) {
						engine := routertest.BuildRouter(t, routertest.WithAuth(buildFailingAuth()))

						response := routertest.Send(engine, route.Method, route.Path, route.Body, headers)

						assert.Equal(t, http.StatusUnauthorized, response.Status)
					})
				}

				for _, route := range routes.All {
					t.Run(route.Name+"のとき、errorにトークンが不正であることを示すメッセージが入る", func(t *testing.T) {
						engine := routertest.BuildRouter(t, routertest.WithAuth(buildFailingAuth()))

						response := routertest.Send(engine, route.Method, route.Path, route.Body, headers)

						assert.Equal(t, "invalid internal auth token", response.ErrorMessage(t))
					})
				}

				for _, route := range routes.All {
					t.Run(route.Name+"のとき、サービスの呼び出しは記録されない", func(t *testing.T) {
						service := routertest.NewShopService()
						engine := routertest.BuildRouter(t, routertest.WithAuth(buildFailingAuth()), routertest.WithService(service))

						routertest.Send(engine, route.Method, route.Path, route.Body, headers)

						assert.Empty(t, service.Calls())
					})
				}
			})
		})
	})
}

func TestAuthenticatedPlayerHandover(t *testing.T) {
	routes := routertest.BuildShopRoutes(t)

	t.Run("[ショップAPI認証]認証済みプレイヤーの引き渡し", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("トークンの検証がプレイヤーIDのTST-PLAYER-1を返すとき", func(t *testing.T) {
				tests := []struct {
					name       string
					route      routertest.ShopRoute
					wantMethod string
				}{
					{name: "商品一覧の取得では、サービスの商品一覧取得がプレイヤーIDのTST-PLAYER-1で呼ばれる", route: routes.ProductList, wantMethod: "GetProducts"},
					{name: "商品の購入では、サービスの購入がプレイヤーIDのTST-PLAYER-1で呼ばれる", route: routes.Purchase, wantMethod: "Purchase"},
					{name: "サブスクリプションの購入では、サービスのサブスクリプション購入がプレイヤーIDのTST-PLAYER-1で呼ばれる", route: routes.Subscribe, wantMethod: "Subscribe"},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						service := routertest.NewShopService()
						engine := routertest.BuildRouter(t, routertest.WithAuth(routertest.NewFixedAuthVerifier("TST-PLAYER-1")), routertest.WithService(service))

						routertest.Send(engine, tt.route.Method, tt.route.Path, tt.route.Body, map[string]string{internalauth.HeaderName: "TST-TOKEN-1"})

						calls := service.Calls()
						require.Len(t, calls, 1)
						assert.Equal(t, tt.wantMethod, calls[0].Method)
						assert.Equal(t, "TST-PLAYER-1", calls[0].PlayerID)
					})
				}
			})

			t.Run("X-Internal-Authヘッダーの値がTST-TOKEN-1のとき、トークンの検証にTST-TOKEN-1がそのまま渡される", func(t *testing.T) {
				auth := routertest.NewFixedAuthVerifier("TST-PLAYER-1")
				engine := routertest.BuildRouter(t, routertest.WithAuth(auth))

				routertest.Send(engine, http.MethodGet, routertest.ProductsPath, nil, map[string]string{internalauth.HeaderName: "TST-TOKEN-1"})

				assert.Equal(t, []string{"TST-TOKEN-1"}, auth.Tokens())
			})
		})
	})
}
