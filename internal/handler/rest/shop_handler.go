package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// shopServicer は shop handler が依存するサービス層の狭い contract。
// カタログ・購入・サブスクリプションのドメインロジックを handler から隠蔽する。
type shopServicer interface {
	GetProducts(ctx context.Context, playerID string) ([]domain.ProductWithOwnership, error)
	Purchase(ctx context.Context, playerID, productID, pf, purchaseToken string) error
	Subscribe(ctx context.Context, playerID, productID, pf, purchaseToken string) (*time.Time, error)
}

// ShopHandler は商品カタログ・購入・サブスクリプションの REST ハンドラを提供する。
type ShopHandler struct {
	shopService shopServicer
}

// NewShopHandler は ShopService を受け取り ShopHandler を構築する。
func NewShopHandler(shopService shopServicer) *ShopHandler {
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

	c.JSON(http.StatusOK, gin.H{"products": toProductResponses(products)})
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

// toProductResponses は domain の ProductWithOwnership を REST wire の
// ProductResponse へ詰め替える。delivery 層の境界変換。
func toProductResponses(items []domain.ProductWithOwnership) []apishop.ProductResponse {
	out := make([]apishop.ProductResponse, 0, len(items))
	for _, it := range items {
		p := it.Product
		out = append(out, apishop.ProductResponse{
			ProductID:   p.ProductID,
			Name:        p.Name,
			Type:        p.Type,
			Price:       p.Price,
			Content:     p.Content,
			Description: p.Description,
			ImageURL:    p.ImageURL,
			IsActive:    p.IsActive,
			IsOwned:     it.IsOwned,
		})
	}
	return out
}
