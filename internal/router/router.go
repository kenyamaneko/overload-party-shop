package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
)

// New は shop の HTTP ルーターを構築する。
// appleWH / googleWH は IAP_MODE=local のとき nil になり得る。その場合該当する
// `/webhook/*` ルートは登録されず、未認証 POST が nil notifier パスに到達することはない。
func New(shopH *rest.ShopHandler, appleWH *rest.AppleWebhookHandler, googleWH *rest.GoogleWebhookHandler) *gin.Engine {
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

	// ストア webhook — 外部到達可能。Apple/Google がリクエストに署名するため
	// gateway 側の認証は不要。各 handler は独立に登録され、片側だけ有効化することも可能。
	if appleWH != nil {
		r.POST("/webhook/apple", appleWH.Handle)
	}
	if googleWH != nil {
		r.POST("/webhook/google", googleWH.Handle)
	}
	return r
}
