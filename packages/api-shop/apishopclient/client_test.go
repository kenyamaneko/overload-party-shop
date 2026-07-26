package apishopclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopclient"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopserverfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 以下の TestClient_<Endpoint>_StatusMapping 群は、SDK の固有責務である
// 「OpenAPI spec で宣言された 4xx/5xx status を sentinel error に変換する」契約を
// endpoint ごとに検証する。各テスト関数は data/openapi.yaml が宣言する error status を
// 網羅し、errors.Is で意図した sentinel に一致することを確認する。
//
// AppleWebhook / GoogleWebhook / GetHealth は apishopserverfake に handler が無く、
// newStatusError logic は他 endpoint で十分カバーされるため本テストでは扱わない。

func TestClient_ListPlayerProducts_StatusMapping(t *testing.T) {
	t.Run("ListPlayerProducts のステータスマッピング", func(t *testing.T) {
		t.Run("401 を受けたとき、ErrUnauthorized になる", func(t *testing.T) {
			srv := apishopserverfake.NewServer()
			defer srv.Close()
			srv.GetProductsFn = func(_ string) (int, any) { return http.StatusUnauthorized, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.ListPlayerProducts(context.Background(), "p1")
			assertSentinel(t, err, apishopclient.ErrUnauthorized)
		})
	})
}

func TestClient_PurchaseProduct_StatusMapping(t *testing.T) {
	t.Run("PurchaseProduct のステータスマッピング", func(t *testing.T) {
		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "400 を受けたとき、ErrBadRequest になる",
				status:     http.StatusBadRequest,
				wantTarget: apishopclient.ErrBadRequest,
			},
			{
				name:       "401 を受けたとき、ErrUnauthorized になる",
				status:     http.StatusUnauthorized,
				wantTarget: apishopclient.ErrUnauthorized,
			},
			{
				name:       "402 を受けたとき、ErrPaymentRequired になる",
				status:     http.StatusPaymentRequired,
				wantTarget: apishopclient.ErrPaymentRequired,
			},
			{
				name:       "404 を受けたとき、ErrNotFound になる",
				status:     http.StatusNotFound,
				wantTarget: apishopclient.ErrNotFound,
			},
			{
				name:       "409 を受けたとき、ErrConflict になる",
				status:     http.StatusConflict,
				wantTarget: apishopclient.ErrConflict,
			},
			{
				name:       "500 を受けたとき、ErrInternalServer になる",
				status:     http.StatusInternalServerError,
				wantTarget: apishopclient.ErrInternalServer,
			},
			{
				name:       "503 を受けたとき、ErrInternalServer になる",
				status:     http.StatusServiceUnavailable,
				wantTarget: apishopclient.ErrInternalServer,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				srv := apishopserverfake.NewServer()
				defer srv.Close()
				srv.PurchaseFn = func(_ string, _ apishop.PurchaseRequest) (int, any) { return tc.status, nil }
				c := newTestClient(t, srv.URL())
				// status mapping 検証のため request body の内容は無関係 (server fake は body を見ず tc.status を返す)。
				_, err := c.PurchaseProduct(context.Background(), "p1", apishop.PurchaseRequest{})
				assertSentinel(t, err, tc.wantTarget)
			})
		}

		unmappedCases := []struct {
			name            string
			status          int
			wantErrContains string
		}{
			{
				name:            "499 を受けたとき、unexpected status エラーになる",
				status:          499,
				wantErrContains: "unexpected status 499",
			},
			{
				name:            "403 を受けたとき、unexpected status エラーになる",
				status:          http.StatusForbidden,
				wantErrContains: "unexpected status 403",
			},
		}
		for _, tc := range unmappedCases {
			t.Run(tc.name, func(t *testing.T) {
				srv := apishopserverfake.NewServer()
				defer srv.Close()
				srv.PurchaseFn = func(_ string, _ apishop.PurchaseRequest) (int, any) { return tc.status, nil }
				c := newTestClient(t, srv.URL())
				_, err := c.PurchaseProduct(context.Background(), "p1", apishop.PurchaseRequest{})
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			})
		}
	})
}

func TestClient_SubscribeProduct_StatusMapping(t *testing.T) {
	t.Run("SubscribeProduct のステータスマッピング", func(t *testing.T) {
		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "400 を受けたとき、ErrBadRequest になる",
				status:     http.StatusBadRequest,
				wantTarget: apishopclient.ErrBadRequest,
			},
			{
				name:       "401 を受けたとき、ErrUnauthorized になる",
				status:     http.StatusUnauthorized,
				wantTarget: apishopclient.ErrUnauthorized,
			},
			{
				name:       "402 を受けたとき、ErrPaymentRequired になる",
				status:     http.StatusPaymentRequired,
				wantTarget: apishopclient.ErrPaymentRequired,
			},
			{
				name:       "404 を受けたとき、ErrNotFound になる",
				status:     http.StatusNotFound,
				wantTarget: apishopclient.ErrNotFound,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				srv := apishopserverfake.NewServer()
				defer srv.Close()
				srv.SubscribeFn = func(_ string, _ apishop.PurchaseRequest) (int, any) { return tc.status, nil }

				c := newTestClient(t, srv.URL())
				// status mapping 検証のため request body の内容は無関係 (server fake は body を見ず tc.status を返す)。
				_, err := c.SubscribeProduct(context.Background(), "p1", apishop.PurchaseRequest{})
				assertSentinel(t, err, tc.wantTarget)
			})
		}
	})
}

func TestClient_RequestEditor(t *testing.T) {
	t.Run("リクエストエディタの適用", func(t *testing.T) {
		t.Run("WithRequestEditorFn で渡した editor が全リクエストに適用される", func(t *testing.T) {
			// X-Internal-Auth header 注入の接続点として SDK が機能することを担保する。
			var gotHeader string
			spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Get("X-Internal-Auth")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"products":[]}`))
			}))
			defer spy.Close()

			c, err := apishopclient.New(spy.URL,
				apishopclient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
					req.Header.Set("X-Internal-Auth", "test-token")
					return nil
				}),
			)
			require.NoError(t, err)

			_, err = c.ListPlayerProducts(context.Background(), "p1")
			require.NoError(t, err)
			assert.Equal(t, "test-token", gotHeader)
		})
	})
}

func newTestClient(t *testing.T, baseURL string) *apishopclient.Client {
	t.Helper()
	c, err := apishopclient.New(baseURL)
	require.NoError(t, err)
	return c
}

func assertSentinel(t *testing.T, gotErr, wantTarget error) {
	t.Helper()
	require.Error(t, gotErr)
	assert.ErrorIs(t, gotErr, wantTarget)
}
