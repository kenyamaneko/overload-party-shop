package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/service"
)

// errorStatus はドメインエラー分類を HTTP ステータスコードに変換する。
// エラーが「何であるか」の判定は service 側の分類 API に委譲し、handler は
// その分類と HTTP のマッピングだけを担う。
func errorStatus(err error) int {
	switch {
	case service.IsNotFound(err):
		return http.StatusNotFound
	case service.IsConflict(err):
		return http.StatusConflict
	case service.IsValidation(err):
		return http.StatusBadRequest
	case service.IsPaymentFailed(err):
		return http.StatusPaymentRequired
	default:
		return http.StatusInternalServerError
	}
}

func respondError(c *gin.Context, err error) {
	c.JSON(errorStatus(err), gin.H{"error": err.Error()})
}
