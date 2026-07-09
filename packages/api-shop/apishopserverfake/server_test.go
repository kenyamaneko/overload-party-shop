package apishopserverfake_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopserverfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Run("サーバフェイク", func(t *testing.T) {
		t.Run("Fn 未設定の endpoint は既定応答を返す", func(t *testing.T) {
			// consumer テストが setup を書かなくても最低限の HTTP 応答が得られる契約。
			tests := []struct {
				name       string
				method     string
				path       string
				reqBody    []byte
				wantStatus int
			}{
				{
					name:       "SelectFaction は 200 で空 Response を返す",
					method:     http.MethodPost,
					path:       "/internal/v1/players/p-1/select-faction",
					reqBody:    []byte(`{"faction":"Tenki"}`),
					wantStatus: http.StatusOK,
				},
				{
					name:       "GetProducts は 200 で空配列を返す",
					method:     http.MethodGet,
					path:       "/api/v1/shop/products",
					reqBody:    nil,
					wantStatus: http.StatusOK,
				},
				{
					name:       "Purchase は 204 No Content を返す",
					method:     http.MethodPost,
					path:       "/api/v1/shop/purchase",
					reqBody:    []byte(`{}`),
					wantStatus: http.StatusNoContent,
				},
				{
					name:       "Subscribe は 200 で空 Response を返す",
					method:     http.MethodPost,
					path:       "/api/v1/shop/subscribe",
					reqBody:    []byte(`{}`),
					wantStatus: http.StatusOK,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					srv := apishopserverfake.NewServer()
					defer srv.Close()

					req, _ := http.NewRequest(tt.method, srv.URL()+tt.path, bytes.NewReader(tt.reqBody))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Player-Id", "p-1")
					resp, err := http.DefaultClient.Do(req)
					require.NoError(t, err)
					defer resp.Body.Close()

					assert.Equal(t, tt.wantStatus, resp.StatusCode)
				})
			}
		})

		t.Run("SelectFactionFn は playerID と request body を typed で受け取れる", func(t *testing.T) {
			srv := apishopserverfake.NewServer()
			defer srv.Close()

			var gotPlayerID, gotFaction string
			srv.SelectFactionFn = func(playerID string, req apishopserverfake.SelectFactionRequest) (int, any) {
				gotPlayerID = playerID
				gotFaction = req.Faction
				return http.StatusConflict, map[string]string{"error": "faction already selected"}
			}

			reqBody := []byte(`{"faction":"SHE"}`)
			req, _ := http.NewRequest(http.MethodPost, srv.URL()+"/internal/v1/players/p-9/select-faction", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusConflict, resp.StatusCode)
			assert.Equal(t, "p-9", gotPlayerID)
			assert.Equal(t, "SHE", gotFaction)
		})

		t.Run("PurchaseFn は X-Player-Id ヘッダ由来の playerID と apishop.PurchaseRequest を typed で受け取れる", func(t *testing.T) {
			srv := apishopserverfake.NewServer()
			defer srv.Close()

			var gotPlayerID string
			var gotReq apishop.PurchaseRequest
			srv.PurchaseFn = func(playerID string, req apishop.PurchaseRequest) (int, any) {
				gotPlayerID = playerID
				gotReq = req
				return http.StatusNoContent, nil
			}

			reqBody := []byte(`{"product_id":"faction_tenki","platform":"ios","purchase_token":"tok-1"}`)
			req, _ := http.NewRequest(http.MethodPost, srv.URL()+"/api/v1/shop/purchase", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Player-Id", "p-1")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
			assert.Equal(t, "p-1", gotPlayerID)
			assert.Equal(t, "faction_tenki", gotReq.ProductID)
			assert.Equal(t, apishop.PlatformIos, gotReq.Platform)
			assert.Equal(t, "tok-1", gotReq.PurchaseToken)
		})

		t.Run("SubscribeFn は ExpiresAt を *time.Time として shopclient が decode できる形で返す", func(t *testing.T) {
			srv := apishopserverfake.NewServer()
			defer srv.Close()

			expiry := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
			srv.SubscribeFn = func(_ string, _ apishop.PurchaseRequest) (int, any) {
				return http.StatusOK, apishopserverfake.SubscribeResponse{
					Message:   "subscribed",
					ExpiresAt: &expiry,
				}
			}

			reqBody := []byte(`{}`)
			req, _ := http.NewRequest(http.MethodPost, srv.URL()+"/api/v1/shop/subscribe", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Player-Id", "p-1")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			var decoded apishopserverfake.SubscribeResponse
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
			require.NotNil(t, decoded.ExpiresAt)
			assert.True(t, decoded.ExpiresAt.Equal(expiry))
		})
	})
}
