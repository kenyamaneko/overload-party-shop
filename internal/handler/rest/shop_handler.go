package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/service"
)

// ShopHandler は商品カタログ・購入・サブスクリプションの REST ハンドラを提供する。
type ShopHandler struct {
	shopService *service.ShopService
}

// NewShopHandler は ShopService を受け取り ShopHandler を構築する。
func NewShopHandler(shopService *service.ShopService) *ShopHandler {
	return &ShopHandler{shopService: shopService}
}

// GetProducts はプレイヤー向け商品一覧を返す。
func (h *ShopHandler) GetProducts(c *gin.Context) {
	playerID := c.Param("playerId")
	if playerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "playerId is required"})
		return
	}

	products, err := h.shopService.GetProducts(c.Request.Context(), playerID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"products": products})
}

// Purchase は単発購入リクエストを処理する。
func (h *ShopHandler) Purchase(c *gin.Context) {
	playerID := c.Param("playerId")
	if playerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "playerId is required"})
		return
	}

	var req apishop.PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.shopService.Purchase(c.Request.Context(), playerID, req.ProductID, req.Platform, req.PurchaseToken); err != nil {
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
	playerID := c.Param("playerId")
	if playerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "playerId is required"})
		return
	}

	var req apishop.PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	expiresAt, err := h.shopService.Subscribe(c.Request.Context(), playerID, req.ProductID, req.Platform, req.PurchaseToken)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":    "subscription accepted",
		"expires_at": expiresAt,
	})
}
