package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/service"
)

type fakeShopServicer struct {
	getProductsFn func(ctx context.Context, playerID string) ([]apishop.ProductResponse, error)
	purchaseFn    func(ctx context.Context, playerID, productID, pf, purchaseToken string) error
	subscribeFn   func(ctx context.Context, playerID, productID, pf, purchaseToken string) (*time.Time, error)
}

func (f *fakeShopServicer) GetProducts(ctx context.Context, playerID string) ([]apishop.ProductResponse, error) {
	return f.getProductsFn(ctx, playerID)
}

func (f *fakeShopServicer) Purchase(ctx context.Context, playerID, productID, pf, purchaseToken string) error {
	return f.purchaseFn(ctx, playerID, productID, pf, purchaseToken)
}

func (f *fakeShopServicer) Subscribe(ctx context.Context, playerID, productID, pf, purchaseToken string) (*time.Time, error) {
	return f.subscribeFn(ctx, playerID, productID, pf, purchaseToken)
}

func newShopTestServer(svc shopServicer) *gin.Engine {
	r := gin.New()
	h := NewShopHandler(svc)
	r.GET("/players/:playerId/products", h.GetProducts)
	r.POST("/players/:playerId/purchase", h.Purchase)
	r.POST("/players/:playerId/subscribe", h.Subscribe)
	return r
}

// GetProducts は service.GetProducts の戻り値を {"products":[...]} にラップして返す。
// サービスのエラーは errorStatus マッピング経由で HTTP status に変換される。
func TestGetProducts(t *testing.T) {
	tests := []struct {
		name       string
		svc        *fakeShopServicer
		wantStatus int
		assertBody func(t *testing.T, body []byte)
	}{
		{
			name: "成功時は products 配列を返す",
			svc: &fakeShopServicer{
				getProductsFn: func(_ context.Context, _ string) ([]apishop.ProductResponse, error) {
					return []apishop.ProductResponse{
						{ProductID: "p1", Name: "商品1", IsOwned: true},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			assertBody: func(t *testing.T, body []byte) {
				var resp struct {
					Products []apishop.ProductResponse `json:"products"`
				}
				require.NoError(t, json.Unmarshal(body, &resp))
				require.Len(t, resp.Products, 1)
				assert.Equal(t, "p1", resp.Products[0].ProductID)
				assert.True(t, resp.Products[0].IsOwned)
			},
		},
		{
			name: "service の分類外エラーは 500",
			svc: &fakeShopServicer{
				getProductsFn: func(_ context.Context, _ string) ([]apishop.ProductResponse, error) {
					return nil, service.ErrVerifyReceipt
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newShopTestServer(tt.svc)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/players/player-1/products", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.assertBody != nil {
				tt.assertBody(t, w.Body.Bytes())
			}
		})
	}
}

// Purchase のサービスエラーは分類経由で適切な HTTP status に変換される。
// エラー分類 → HTTP status のマッピング網羅は rest/errors_test.go でテスト済み。
// ここは「handler が service のエラーを respondError 経由で伝搬する」ことを
// 代表ケースで固定する。
func TestPurchase(t *testing.T) {
	validBody, _ := json.Marshal(apishop.PurchaseRequest{
		ProductID:     "faction_tenki",
		Platform:      "ios",
		PurchaseToken: "receipt-1",
	})

	tests := []struct {
		name       string
		body       []byte
		svc        *fakeShopServicer
		wantStatus int
		assertBody func(t *testing.T, body []byte)
	}{
		{
			name: "成功時は 202 Accepted と product_id を返す",
			body: validBody,
			svc: &fakeShopServicer{
				purchaseFn: func(_ context.Context, _, _, _, _ string) error { return nil },
			},
			wantStatus: http.StatusAccepted,
			assertBody: func(t *testing.T, body []byte) {
				var resp map[string]interface{}
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "purchase accepted", resp["message"])
				assert.Equal(t, "faction_tenki", resp["product_id"])
			},
		},
		{
			name: "Conflict 分類エラー (既に所有) は 409",
			body: validBody,
			svc: &fakeShopServicer{
				purchaseFn: func(_ context.Context, _, _, _, _ string) error { return service.ErrAlreadyOwned },
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "PaymentFailed 分類エラー (レシート無効) は 402",
			body: validBody,
			svc: &fakeShopServicer{
				purchaseFn: func(_ context.Context, _, _, _, _ string) error { return service.ErrReceiptVerificationFailed },
			},
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "JSON として parse できない body は 400 (service 未到達)",
			body:       []byte(`not json`),
			svc:        &fakeShopServicer{},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newShopTestServer(tt.svc)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/players/player-1/purchase", bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.assertBody != nil {
				tt.assertBody(t, w.Body.Bytes())
			}
		})
	}
}

// Subscribe は expires_at を含む body を返し、サービスエラーは Purchase と
// 同じく分類経由で HTTP status に変換される。
func TestSubscribe(t *testing.T) {
	validBody, _ := json.Marshal(apishop.PurchaseRequest{
		ProductID:     "premium_monthly",
		Platform:      "ios",
		PurchaseToken: "sub-1",
	})
	expiresAt := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		body       []byte
		svc        *fakeShopServicer
		wantStatus int
		assertBody func(t *testing.T, body []byte)
	}{
		{
			name: "成功時は 202 Accepted と expires_at を返す",
			body: validBody,
			svc: &fakeShopServicer{
				subscribeFn: func(_ context.Context, _, _, _, _ string) (*time.Time, error) {
					return &expiresAt, nil
				},
			},
			wantStatus: http.StatusAccepted,
			assertBody: func(t *testing.T, body []byte) {
				var resp map[string]interface{}
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "subscription accepted", resp["message"])
				assert.Equal(t, expiresAt.Format(time.RFC3339Nano), resp["expires_at"])
			},
		},
		{
			name: "Validation 分類エラー (non-subscription 商品) は 400",
			body: validBody,
			svc: &fakeShopServicer{
				subscribeFn: func(_ context.Context, _, _, _, _ string) (*time.Time, error) {
					return nil, service.ErrProductNotSubscription
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "JSON 不正は 400",
			body:       []byte(`{broken`),
			svc:        &fakeShopServicer{},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newShopTestServer(tt.svc)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/players/player-1/subscribe", bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.assertBody != nil {
				tt.assertBody(t, w.Body.Bytes())
			}
		})
	}
}
