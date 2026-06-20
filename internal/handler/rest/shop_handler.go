package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// shopServicer は shop handler が依存する usecase の狭い contract。
type shopServicer interface {
	GetProducts(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error)
	Purchase(ctx context.Context, playerID, productID, platform, purchaseToken string) error
	Subscribe(ctx context.Context, playerID, productID, platform, purchaseToken string) (*time.Time, error)
}

// ShopHandler は商品カタログ・購入・サブスクリプションの REST ハンドラを提供する。
type ShopHandler struct {
	shopService shopServicer
}

func NewShopHandler(shopService shopServicer) *ShopHandler {
	return &ShopHandler{shopService: shopService}
}

// GetProducts はプレイヤー向け商品一覧を返す。
func (h *ShopHandler) GetProducts(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)

	products, err := h.shopService.GetProducts(c.Request.Context(), playerID)
	if err != nil {
		respondError(c, err)
		return
	}

	resp, err := presenter.ToProductResponses(products)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"products": resp})
}

// Purchase は単発購入リクエストを処理する。
func (h *ShopHandler) Purchase(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)

	var req apishop.PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.shopService.Purchase(c.Request.Context(), playerID, req.ProductID, string(req.Platform), req.PurchaseToken); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":    "purchase accepted",
		"product_id": req.ProductID,
	})
}

// Subscribe はサブスクリプション購入リクエストを処理する。
func (h *ShopHandler) Subscribe(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)

	var req apishop.PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	expiresAt, err := h.shopService.Subscribe(c.Request.Context(), playerID, req.ProductID, string(req.Platform), req.PurchaseToken)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":    "subscription accepted",
		"expires_at": expiresAt,
	})
}

