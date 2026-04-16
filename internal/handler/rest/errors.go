package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/service/purchase"
	"github.com/kenyamaneko/overload-party-shop/internal/service/subscription"
)

// errorStatus はドメインエラーを HTTP ステータスコードに変換する。
// service 層が返す sentinel をここで「conflict / validation / payment failed / not found」
// のいずれに翻訳するかを決定する責務は transport (handler) 側に閉じる。
//
// 分類対象に該当しないエラー (DB 一時障害等) は default の 500 にフォールバックし、
// クライアント / ストア側のリトライを促す。
func errorStatus(err error) int {
	switch {
	case isNotFound(err):
		return http.StatusNotFound
	case isConflict(err):
		return http.StatusConflict
	case isValidation(err):
		return http.StatusBadRequest
	case isPaymentFailed(err):
		return http.StatusPaymentRequired
	default:
		return http.StatusInternalServerError
	}
}

func respondError(c *gin.Context, err error) {
	c.JSON(errorStatus(err), gin.H{"error": err.Error()})
}

// isNotFound は対象リソースが見つからない類のエラーか判定する。
// port.ErrNotFound は repo から bubble される。
func isNotFound(err error) bool {
	return errors.Is(err, port.ErrNotFound)
}

// isConflict は既存リソースとの衝突 (重複登録等) によるエラーか判定する。
func isConflict(err error) bool {
	return errors.Is(err, purchase.ErrAlreadyOwned) ||
		errors.Is(err, purchase.ErrFactionAlreadySelected)
}

// isValidation はクライアント入力 / 商品状態の妥当性違反によるエラーか判定する。
func isValidation(err error) bool {
	return errors.Is(err, purchase.ErrInvalidFaction) ||
		errors.Is(err, purchase.ErrProductNotActive) ||
		errors.Is(err, purchase.ErrProductNotSubscription) ||
		errors.Is(err, purchase.ErrUnsupportedProductType) ||
		errors.Is(err, purchase.ErrUnsupportedPlatform)
}

// isPaymentFailed はレシート/サブスクリプション検証がストア側で無効判定された
// エラーか判定する。verifier 自体の infra 障害 (purchase.ErrVerifyReceipt) は含まない。
func isPaymentFailed(err error) bool {
	return errors.Is(err, purchase.ErrReceiptVerificationFailed) ||
		errors.Is(err, purchase.ErrSubVerificationFailed)
}

// isDeterministic は webhook 通知処理において「再送しても結果が変わらない」
// 確定的エラーか判定する。webhook handler はこれに該当するエラーを ack (2xx) し、
// ストア側の無駄なリトライを防ぐ。該当しないエラー (DB 障害・pub/sub 失敗等) は
// 一時的な問題として 5xx で返しリトライを促す。
func isDeterministic(err error) bool {
	return errors.Is(err, subscription.ErrDecodeNotification) ||
		errors.Is(err, subscription.ErrDecodeTransactionInfo) ||
		errors.Is(err, subscription.ErrDecodeRTDNData) ||
		errors.Is(err, subscription.ErrUnmarshalRTDNData) ||
		errors.Is(err, subscription.ErrSubscriptionNotFound)
}
