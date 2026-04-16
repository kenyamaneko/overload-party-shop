package rest

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/service/subscription"
)

// subscriptionNotifier は webhook handler が依存するサービス層の狭い contract。
// handler は JWS 処理・状態遷移等のドメインロジックを知らず、通知ペイロードを
// 渡して成否だけ受け取る。
type subscriptionNotifier interface {
	HandleAppleNotification(ctx context.Context, signedPayload string) error
	HandleGoogleNotification(ctx context.Context, msg subscription.GoogleRTDNMessage) error
}

// WebhookHandler は Apple/Google ストアからの webhook 通知を処理する。
type WebhookHandler struct {
	subscriptionService subscriptionNotifier
}

// NewWebhookHandler は subscription service を受け取り WebhookHandler を構築する。
func NewWebhookHandler(subscriptionService subscriptionNotifier) *WebhookHandler {
	return &WebhookHandler{subscriptionService: subscriptionService}
}

// HandleAppleWebhook は Apple App Store Server Notifications V2 を受信する。
func (h *WebhookHandler) HandleAppleWebhook(c *gin.Context) {
	var payload subscription.AppleNotificationPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	respondWebhook(c, "apple", h.subscriptionService.HandleAppleNotification(c.Request.Context(), payload.SignedPayload))
}

// HandleGoogleWebhook は Google Play RTDN（Real-Time Developer Notifications）を受信する。
func (h *WebhookHandler) HandleGoogleWebhook(c *gin.Context) {
	var msg subscription.GoogleRTDNMessage
	if err := c.ShouldBindJSON(&msg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	respondWebhook(c, "google", h.subscriptionService.HandleGoogleNotification(c.Request.Context(), msg))
}

// respondWebhook は webhook 処理結果を HTTP ステータスに変換する。
// 確定的エラー (decode 失敗・unknown subscription 等) は ack (200) で返し、
// ストア側の無駄なリトライを止める。一時的エラー (DB・pub/sub 障害等) は 500 で
// 返しリトライを促す。確定的エラーはログに残し観測可能性を担保する。
func respondWebhook(c *gin.Context, source string, err error) {
	if err == nil {
		c.Status(http.StatusOK)
		return
	}
	if isDeterministic(err) {
		log.Printf("webhook %s deterministic failure (acked): %v", source, err)
		c.Status(http.StatusOK)
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
