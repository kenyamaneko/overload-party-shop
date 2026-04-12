package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/service"
)

// WebhookHandler は Apple/Google ストアからの webhook 通知を処理する。
type WebhookHandler struct {
	subscriptionService *service.SubscriptionService
}

// NewWebhookHandler は SubscriptionService を受け取り WebhookHandler を構築する。
func NewWebhookHandler(subscriptionService *service.SubscriptionService) *WebhookHandler {
	return &WebhookHandler{subscriptionService: subscriptionService}
}

// HandleAppleWebhook は Apple App Store Server Notifications V2 を受信する。
func (h *WebhookHandler) HandleAppleWebhook(c *gin.Context) {
	var payload service.AppleNotificationPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.subscriptionService.HandleAppleNotification(c.Request.Context(), payload.SignedPayload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// HandleGoogleWebhook は Google Play RTDN（Real-Time Developer Notifications）を受信する。
func (h *WebhookHandler) HandleGoogleWebhook(c *gin.Context) {
	var msg service.GoogleRTDNMessage
	if err := c.ShouldBindJSON(&msg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.subscriptionService.HandleGoogleNotification(c.Request.Context(), msg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}
