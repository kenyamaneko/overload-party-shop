package rest

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

// appleNotifier は Apple webhook handler が依存する usecase の狭い contract。
type appleNotifier interface {
	HandleNotification(ctx context.Context, signedPayload string) error
}

// AppleWebhookHandler は Apple App Store Server Notifications V2 を処理する。
type AppleWebhookHandler struct {
	notifier appleNotifier
}

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
