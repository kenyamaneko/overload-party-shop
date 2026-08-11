package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
)

// New は shop の HTTP ルーターを構築する。appleWebhookHandler / googleWebhookHandler が nil なら該当 webhook ルートを登録しない。
func New(shopHandler *rest.ShopHandler, appleWebhookHandler *rest.AppleWebhookHandler, googleWebhookHandler *rest.GoogleWebhookHandler, authVerifier internalauth.Verifier, pushValidator PubSubPushTokenValidator, pushServiceAccountEmail, pushAudience string) *gin.Engine {
	r := gin.New()
	r.Use(newRequestLogger(), gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1/shop")
	api.Use(internalauth.VerifyInternalAuth(authVerifier))
	{
		api.GET("/products", shopHandler.GetProducts)
		api.POST("/purchase", shopHandler.Purchase)
		api.POST("/subscribe", shopHandler.Subscribe)
	}

	// ストア webhook — 外部到達可能。Apple は payload の JWS 署名、Google は Pub/Sub push が
	// 付与する OIDC トークンでそれぞれ requester を検証するため、gateway 側の認証は不要。
	// 各 handler は独立に登録され、片側だけ有効化することも可能。
	if appleWebhookHandler != nil {
		r.POST("/webhook/apple", appleWebhookHandler.Handle)
	}
	if googleWebhookHandler != nil {
		r.POST("/webhook/google", verifyPubSubPush(pushValidator, pushServiceAccountEmail, pushAudience), googleWebhookHandler.Handle)
	}
	return r
}

// newRequestLogger はステータスコードに応じてログレベルを切り替える HTTP リクエストロガー。
func newRequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		slog.LogAttrs(c.Request.Context(), level, "http request",
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
		)
	}
}
