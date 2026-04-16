package rest

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondWebhook は webhook 処理結果を HTTP ステータスに変換する。
// 確定的エラー (decode 失敗・unknown subscription 等) は ack (200) で返し、
// ストア側の無駄なリトライを止める。一時的エラー (DB・pub/sub 障害等) は 500 で
// 返しリトライを促す。確定的エラーはログに残し観測可能性を担保する。
//
// AppleWebhookHandler / GoogleWebhookHandler 共通で使う package-level helper。
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
