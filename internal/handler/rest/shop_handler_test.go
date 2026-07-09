package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testPlayerID は handler テストで context に注入する固定 player_id。
const testPlayerID = "player-1"

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

// newShopTestServer は handler 単体テスト用に gin engine を組み立てる。
// 認証 middleware は通さず context に testPlayerID を直接注入することで、
// handler が context 経由で player_id を読むことを検証する。
func newShopTestServer(svc shopServicer) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(internalauth.PlayerIDContextKey, testPlayerID)
		c.Next()
	})
	h := NewShopHandler(svc)
	api := r.Group("/api/v1/shop")
	api.GET("/products", h.GetProducts)
	api.POST("/purchase", h.Purchase)
	api.POST("/subscribe", h.Subscribe)
	return r
}

func TestGetProducts(t *testing.T) {
	t.Run("商品一覧取得", func(t *testing.T) {
		t.Run("取得に成功するとき、200 で is_owned 付き商品一覧を返し context の player_id を usecase に伝える", func(t *testing.T) {
			var observedPlayerID string
			svc := &fakeShopServicer{
				getProductsFn: func(_ context.Context, playerID string) ([]domain.ProductWithOwnership, error) {
					observedPlayerID = playerID
					return []domain.ProductWithOwnership{
						{
							ProductView: domain.SubscriptionProduct{
								Product: domain.Product{ProductID: "p1", Name: "商品1", Type: domain.ProductTypeSubscription},
							},
							IsOwned: true,
						},
					}, nil
				},
			}
			r := newShopTestServer(svc)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil))

			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, testPlayerID, observedPlayerID, "context 経由の player_id が usecase に伝搬する")
			var resp struct {
				Products []apishop.ProductResponse `json:"products"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Products, 1)
			assert.Equal(t, "p1", resp.Products[0].ProductID)
			assert.True(t, resp.Products[0].IsOwned)
		})

		t.Run("usecase がエラーを返すとき、500 になる", func(t *testing.T) {
			svc := &fakeShopServicer{
				getProductsFn: func(_ context.Context, _ string) ([]domain.ProductWithOwnership, error) {
					return nil, purchase.ErrVerifyReceipt
				},
			}
			r := newShopTestServer(svc)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil))

			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})
}

func TestPurchase(t *testing.T) {
	t.Run("商品購入", func(t *testing.T) {
		t.Run("usecase が成功するとき、202 で message「purchase accepted」と product_id を返す", func(t *testing.T) {
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/shop/purchase", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusAccepted, w.Code)
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "purchase accepted", resp["message"])
			assert.Equal(t, "faction_tenki", resp["product_id"])
		})

		validPurchaseBody, _ := json.Marshal(apishop.PurchaseRequest{
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
				name: "usecase が ErrAlreadyOwned を wrap したエラーを返すとき、errors.Is 経由で 409 になる",
				body: validPurchaseBody,
				svc: &fakeShopServicer{
					purchaseFn: func(_ context.Context, _, _, _, _ string) error {
						return fmt.Errorf("purchase faction_tenki: %w", purchase.ErrAlreadyOwned)
					},
				},
				wantStatus: http.StatusConflict,
			},
			{
				name: "usecase が ErrReceiptVerificationFailed を返すとき、402 になる",
				body: validPurchaseBody,
				svc: &fakeShopServicer{
					purchaseFn: func(_ context.Context, _, _, _, _ string) error { return purchase.ErrReceiptVerificationFailed },
				},
				wantStatus: http.StatusPaymentRequired,
			},
			{
				name:       "body が JSON として parse できないとき、400 になる",
				body:       []byte(`not json`),
				svc:        &fakeShopServicer{},
				wantStatus: http.StatusBadRequest,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				r := newShopTestServer(tt.svc)
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/shop/purchase", bytes.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(w, req)

				assert.Equal(t, tt.wantStatus, w.Code)
			})
		}

		t.Run("usecase が port.ErrNotFound を返すとき、404 で error フィールドを返す", func(t *testing.T) {
			svc := &fakeShopServicer{
				purchaseFn: func(_ context.Context, _, _, _, _ string) error { return port.ErrNotFound },
			}
			body, _ := json.Marshal(apishop.PurchaseRequest{
				ProductID:     "faction_tenki",
				Platform:      "ios",
				PurchaseToken: "receipt-1",
			})
			r := newShopTestServer(svc)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/shop/purchase", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusNotFound, w.Code)
			var resp map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, port.ErrNotFound.Error(), resp["error"])
		})
	})
}

func TestSubscribe(t *testing.T) {
	t.Run("サブスクリプション購入", func(t *testing.T) {
		t.Run("usecase が成功するとき、202 で message「subscription accepted」と expires_at を返す", func(t *testing.T) {
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/shop/subscribe", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusAccepted, w.Code)
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "subscription accepted", resp["message"])
			assert.Equal(t, expiresAt.Format(time.RFC3339Nano), resp["expires_at"])
		})

		validSubscribeBody, _ := json.Marshal(apishop.PurchaseRequest{
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
				name: "usecase が ErrProductNotSubscription を返すとき、400 になる",
				body: validSubscribeBody,
				svc: &fakeShopServicer{
					subscribeFn: func(_ context.Context, _, _, _, _ string) (*time.Time, error) {
						return nil, purchase.ErrProductNotSubscription
					},
				},
				wantStatus: http.StatusBadRequest,
			},
			{
				name:       "body が JSON として parse できないとき、400 になる",
				body:       []byte(`{broken`),
				svc:        &fakeShopServicer{},
				wantStatus: http.StatusBadRequest,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				r := newShopTestServer(tt.svc)
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/shop/subscribe", bytes.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(w, req)

				assert.Equal(t, tt.wantStatus, w.Code)
			})
		}
	})
}
