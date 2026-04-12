package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/service"
)

// errorStatus はサービスエラーを HTTP ステータスコードに変換する。
func errorStatus(err error) int {
	switch {
	case errors.Is(err, port.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrFactionAlreadySelected),
		errors.Is(err, service.ErrAlreadyOwned):
		return http.StatusConflict
	case errors.Is(err, service.ErrInvalidFaction),
		errors.Is(err, service.ErrProductNotSubscription),
		errors.Is(err, service.ErrProductNotActive),
		errors.Is(err, service.ErrUnsupportedPlatform):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrReceiptVerificationFailed),
		errors.Is(err, service.ErrSubVerificationFailed):
		return http.StatusPaymentRequired
	default:
		return http.StatusInternalServerError
	}
}

func respondError(c *gin.Context, err error) {
	c.JSON(errorStatus(err), gin.H{"error": err.Error()})
}
