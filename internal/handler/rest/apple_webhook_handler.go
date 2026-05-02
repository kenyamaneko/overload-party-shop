package rest

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

// appleNotifier は Apple webhook handler が依存するサービス層の狭い contract。
// handler は JWS 検証や状態遷移を知らず、payload を渡して成否だけ受け取る。
type appleNotifier interface {
	HandleNotification(ctx context.Context, signedPayload string) error
}

// AppleWebhookHandler は Apple App Store Server Notifications V2 を処理する。
type AppleWebhookHandler struct {
	notifier appleNotifier
}

// NewAppleWebhookHandler は AppleNotifier を受け取り handler を構築する。
func NewAppleWebhookHandler(n appleNotifier) *AppleWebhookHandler {
	return &AppleWebhookHandler{notifier: n}
}

// Handle は Apple App Store Server Notifications V2 を受信する。
func (h *AppleWebhookHandler) Handle(c *gin.Context) {
	var payload subscription.AppleNotificationPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	respondWebhook(c, "apple", h.notifier.HandleNotification(c.Request.Context(), payload.SignedPayload))
}
