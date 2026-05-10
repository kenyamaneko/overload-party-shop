package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// playerIDHeader は gateway が認証後に shop へ付与する player_id ヘッダ名。
const playerIDHeader = "X-Player-Id"

// playerIDFromHeader は X-Player-Id ヘッダから player_id を取り出す。
// 欠落時は 401 を c に書き込み、第二戻り値で false を返す (呼び出し側はそのまま return すればよい)。
func playerIDFromHeader(c *gin.Context) (string, bool) {
	playerID := c.GetHeader(playerIDHeader)
	if playerID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": playerIDHeader + " header is required"})
		return "", false
	}
	return playerID, true
}
