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

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/service/purchase"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

type fakeShopServicer struct {
	getProductsFn func(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error)
	purchaseFn    func(ctx context.Context, playerID, productID, pf, purchaseToken string) error
	subscribeFn   func(ctx context.Context, playerID, productID, pf, purchaseToken string) (*time.Time, error)
}

func (f *fakeShopServicer) GetProducts(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error) {
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

// GetProducts の成功パスは service の戻り値を {"products":[...]} で wrap する。
// body shape 検証が本テストの主目的。
func TestGetProducts_Success(t *testing.T) {
	svc := &fakeShopServicer{
		getProductsFn: func(_ context.Context, _ string) ([]domain.ProductWithOwnership, error) {
			return []domain.ProductWithOwnership{
				{Product: domain.Product{ProductID: "p1", Name: "商品1"}, IsOwned: true},
			}, nil
		},
	}
	r := newShopTestServer(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/players/player-1/products", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Products []apishop.ProductResponse `json:"products"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Products, 1)
	assert.Equal(t, "p1", resp.Products[0].ProductID)
	assert.True(t, resp.Products[0].IsOwned)
}

// GetProducts の service エラーは errorStatus マッピング経由で HTTP ステータスに
// 変換される (分類 → ステータスの網羅は rest/errors_test.go 側)。ここは
// 「handler が service のエラーを respondError 経由で流すこと」を代表ケースで固定する。
func TestGetProducts_PropagatesServiceError(t *testing.T) {
	svc := &fakeShopServicer{
		getProductsFn: func(_ context.Context, _ string) ([]domain.ProductWithOwnership, error) {
			return nil, purchase.ErrVerifyReceipt
		},
	}
	r := newShopTestServer(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/players/player-1/products", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// Purchase の成功パスは 202 Accepted と product_id を返す。
func TestPurchase_Success(t *testing.T) {
	svc := &fakeShopServicer{
		purchaseFn: func(_ context.Context, _, _, _, _ string) error { return nil },
	}
	body, _ := json.Marshal(apishop.PurchaseRequest{
		ProductID:     "faction_tenki",
		Platform:      "ios",
		PurchaseToken: "receipt-1",
	})
	r := newShopTestServer(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/players/player-1/purchase", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "purchase accepted", resp["message"])
	assert.Equal(t, "faction_tenki", resp["product_id"])
}

// Purchase は service のエラー分類と body bind 失敗をそれぞれの HTTP ステータスに
// 変換する。body shape は検証対象外 (status 変換の契約を固定する)。
func TestPurchase_ErrorResponses(t *testing.T) {
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
	}{
		{
			name: "Conflict 分類エラー (既に所有) は 409",
			body: validBody,
			svc: &fakeShopServicer{
				purchaseFn: func(_ context.Context, _, _, _, _ string) error { return purchase.ErrAlreadyOwned },
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "PaymentFailed 分類エラー (レシート無効) は 402",
			body: validBody,
			svc: &fakeShopServicer{
				purchaseFn: func(_ context.Context, _, _, _, _ string) error { return purchase.ErrReceiptVerificationFailed },
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
		})
	}
}

// Subscribe の成功パスは 202 Accepted と expires_at を返す。
func TestSubscribe_Success(t *testing.T) {
	expiresAt := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	svc := &fakeShopServicer{
		subscribeFn: func(_ context.Context, _, _, _, _ string) (*time.Time, error) {
			return &expiresAt, nil
		},
	}
	body, _ := json.Marshal(apishop.PurchaseRequest{
		ProductID:     "premium_monthly",
		Platform:      "ios",
		PurchaseToken: "sub-1",
	})
	r := newShopTestServer(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/players/player-1/subscribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "subscription accepted", resp["message"])
	assert.Equal(t, expiresAt.Format(time.RFC3339Nano), resp["expires_at"])
}

// Subscribe のエラーレスポンス: service エラー分類 + body bind 失敗 の HTTP status 変換。
func TestSubscribe_ErrorResponses(t *testing.T) {
	validBody, _ := json.Marshal(apishop.PurchaseRequest{
		ProductID:     "premium_monthly",
		Platform:      "ios",
		PurchaseToken: "sub-1",
	})

	tests := []struct {
		name       string
		body       []byte
		svc        *fakeShopServicer
		wantStatus int
	}{
		{
			name: "Validation 分類エラー (non-subscription 商品) は 400",
			body: validBody,
			svc: &fakeShopServicer{
				subscribeFn: func(_ context.Context, _, _, _, _ string) (*time.Time, error) {
					return nil, purchase.ErrProductNotSubscription
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
		})
	}
}
