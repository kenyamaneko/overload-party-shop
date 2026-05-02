package rest

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

// googleNotifier は Google webhook handler が依存するサービス層の狭い contract。
type googleNotifier interface {
	HandleNotification(ctx context.Context, msg subscription.GoogleRTDNMessage) error
}

// GoogleWebhookHandler は Google Play RTDN (Real-Time Developer Notifications) を処理する。
type GoogleWebhookHandler struct {
	notifier googleNotifier
}

// NewGoogleWebhookHandler は GoogleNotifier を受け取り handler を構築する。
func NewGoogleWebhookHandler(n googleNotifier) *GoogleWebhookHandler {
	return &GoogleWebhookHandler{notifier: n}
}

// Handle は Google Play RTDN を受信する。
func (h *GoogleWebhookHandler) Handle(c *gin.Context) {
	var msg subscription.GoogleRTDNMessage
	if err := c.ShouldBindJSON(&msg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	respondWebhook(c, "google", h.notifier.HandleNotification(c.Request.Context(), msg))
}
