package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
)

// New は shop の HTTP ルーターを構築する。webhookH は IAP_MODE=local のとき nil になり得る。
// その場合 `/webhook/*` ルートは一切登録されず、未認証 POST が nil verifier パスに
// 到達することはない。
func New(shopH *rest.ShopHandler, webhookH *rest.WebhookHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	players := r.Group("/internal/v1/players/:playerId")
	{
		players.GET("/products", shopH.GetProducts)
		players.POST("/purchase", shopH.Purchase)
		players.POST("/subscribe", shopH.Subscribe)
	}

	if webhookH != nil {
		// ストア webhook — 外部到達可能。Apple/Google がリクエストに署名するため
		// gateway 側の認証は不要。
		webhooks := r.Group("/webhook")
		{
			webhooks.POST("/apple", webhookH.HandleAppleWebhook)
			webhooks.POST("/google", webhookH.HandleGoogleWebhook)
		}
	}
	return r
}
