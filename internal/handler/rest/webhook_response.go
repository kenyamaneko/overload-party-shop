package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondWebhook は webhook 処理結果を HTTP ステータスに変換する。
// 確定的エラーは 200 ack でリトライを止め、それ以外は 500 でリトライを促す。
func respondWebhook(c *gin.Context, source string, err error) {
	if err == nil {
		c.Status(http.StatusOK)
		return
	}
	if isDeterministic(err) {
		slog.Warn("webhook deterministic failure", "source", source, "error", err)
		c.Status(http.StatusOK)
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
