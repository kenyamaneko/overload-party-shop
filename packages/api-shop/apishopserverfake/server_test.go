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

// Fn 未設定の endpoint は既定応答を返す。
func TestServer_DefaultResponses(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		reqBody    []byte
		wantStatus int
	}{
		{
			name:       "SelectFaction 既定は 200 + 空 Response",
			method:     http.MethodPost,
			path:       "/internal/v1/players/p-1/select-faction",
			reqBody:    []byte(`{"faction":"Tenki"}`),
			wantStatus: http.StatusOK,
		},
		{
			name:       "GetProducts 既定は 200 + 空配列",
			method:     http.MethodGet,
			path:       "/internal/v1/players/p-1/products",
			reqBody:    nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Purchase 既定は 204 No Content",
			method:     http.MethodPost,
			path:       "/internal/v1/players/p-1/purchase",
			reqBody:    []byte(`{}`),
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "Subscribe 既定は 200 + 空 Response",
			method:     http.MethodPost,
			path:       "/internal/v1/players/p-1/subscribe",
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
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

// SelectFactionFn は playerID と request body を typed で受け取れる。
func TestServer_SelectFactionFn_ReceivesRequest(t *testing.T) {
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
}

// PurchaseFn は apishop.PurchaseRequest を typed で受け取れる。
func TestServer_PurchaseFn_ReceivesTypedRequest(t *testing.T) {
	srv := apishopserverfake.NewServer()
	defer srv.Close()

	var gotReq apishop.PurchaseRequest
	srv.PurchaseFn = func(_ string, req apishop.PurchaseRequest) (int, any) {
		gotReq = req
		return http.StatusNoContent, nil
	}

	reqBody := []byte(`{"product_id":"faction_tenki","platform":"ios","purchase_token":"tok-1"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL()+"/internal/v1/players/p-1/purchase", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "faction_tenki", gotReq.ProductID)
	assert.Equal(t, apishop.PlatformIos, gotReq.Platform)
	assert.Equal(t, "tok-1", gotReq.PurchaseToken)
}

// SubscribeResponse は ExpiresAt を *time.Time として shopclient が decode できる形で返す。
func TestServer_SubscribeFn_ReturnsTypedExpiry(t *testing.T) {
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
	req, _ := http.NewRequest(http.MethodPost, srv.URL()+"/internal/v1/players/p-1/subscribe", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var decoded apishopserverfake.SubscribeResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	require.NotNil(t, decoded.ExpiresAt)
	assert.True(t, decoded.ExpiresAt.Equal(expiry))
}
