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
	Subscribe(ctx context.Context, playerID, productID, platform, purchaseToken string) (time.Time, error)
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

	resp, err := presenter.ToProductListResponse(products)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Purchase は単発購入リクエストを処理する。
func (h *ShopHandler) Purchase(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)

	var req apishop.PurchaseRequest
	if err := decodeRequestBody(c, &req); err != nil {
		respondError(c, err)
		return
	}
	if err := validatePurchaseRequest(req); err != nil {
		respondBadRequest(c, err)
		return
	}

	if err := h.shopService.Purchase(c.Request.Context(), playerID, req.ProductID, string(req.Platform), req.PurchaseToken); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, presenter.ToPurchaseResponse(req.ProductID))
}

// Subscribe はサブスクリプション購入リクエストを処理する。
func (h *ShopHandler) Subscribe(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)

	var req apishop.PurchaseRequest
	if err := decodeRequestBody(c, &req); err != nil {
		respondError(c, err)
		return
	}
	if err := validatePurchaseRequest(req); err != nil {
		respondBadRequest(c, err)
		return
	}

	expiresAt, err := h.shopService.Subscribe(c.Request.Context(), playerID, req.ProductID, string(req.Platform), req.PurchaseToken)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, presenter.ToSubscribeResponse(expiresAt))
}

// validatePurchaseRequest は購入リクエストの必須項目の欠落を検査する。
func validatePurchaseRequest(req apishop.PurchaseRequest) error {
	return validateRequiredFields(
		requiredField{name: "product_id", value: req.ProductID},
		requiredField{name: "platform", value: string(req.Platform)},
		requiredField{name: "purchase_token", value: req.PurchaseToken},
	)
}
