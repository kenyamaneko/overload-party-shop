package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/service/purchase"
	"github.com/kenyamaneko/overload-party-shop/internal/service/subscription"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// 分類関数 → HTTP ステータスのマッピングを固定する。各 sentinel が
// どの分類に属するかも併せて検証する (transport が「何で 4xx/5xx を返すか」の契約)。
// 新しい sentinel を追加したとき、いずれの分類にも属さないと default の
// 「500 / retry」に倒れる仕組みなので、分類漏れは安全側でしか事故らない。
func TestErrorClassificationToStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "port.ErrNotFound は NotFound → 404",
			err:        port.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "ErrAlreadyOwned は Conflict → 409",
			err:        purchase.ErrAlreadyOwned,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "ErrFactionAlreadySelected は Conflict → 409",
			err:        purchase.ErrFactionAlreadySelected,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "ErrInvalidFaction は Validation → 400",
			err:        purchase.ErrInvalidFaction,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ErrProductNotActive は Validation → 400",
			err:        purchase.ErrProductNotActive,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ErrProductNotSubscription は Validation → 400",
			err:        purchase.ErrProductNotSubscription,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ErrUnsupportedProductType は Validation → 400",
			err:        purchase.ErrUnsupportedProductType,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ErrUnsupportedPlatform は Validation → 400",
			err:        purchase.ErrUnsupportedPlatform,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ErrReceiptVerificationFailed は PaymentFailed → 402",
			err:        purchase.ErrReceiptVerificationFailed,
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "ErrSubVerificationFailed は PaymentFailed → 402",
			err:        purchase.ErrSubVerificationFailed,
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "ErrVerifyReceipt はインフラ障害扱いで分類外 → 500 / retry",
			err:        purchase.ErrVerifyReceipt,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "未知のエラーも 500 fallback",
			err:        errors.New("unknown"),
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantStatus, errorStatus(tt.err))
		})
	}
}

// fmt.Errorf("%w: ...") で wrap しても分類関数が元の sentinel を認識することを
// 固定する。service 実装側は頻繁に wrap するため、errors.Is 経由の分類が
// 機能しないとリアルな呼び出しでマッチしなくなる。
func TestErrorClassification_WrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("purchase: %w", purchase.ErrAlreadyOwned)
	assert.True(t, isConflict(wrapped))

	nested := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", subscription.ErrDecodeNotification))
	assert.True(t, isDeterministic(nested))
}

// IsDeterministic は webhook ack 用の分類で、上記の status 変換とは独立した
// 独自グループ。各 sentinel が deterministic 判定されることを固定する。
func TestIsDeterministic(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "ErrDecodeNotification", err: subscription.ErrDecodeNotification},
		{name: "ErrDecodeTransactionInfo", err: subscription.ErrDecodeTransactionInfo},
		{name: "ErrDecodeRTDNData", err: subscription.ErrDecodeRTDNData},
		{name: "ErrUnmarshalRTDNData", err: subscription.ErrUnmarshalRTDNData},
		{name: "ErrSubscriptionNotFound", err: subscription.ErrSubscriptionNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, isDeterministic(tt.err))
		})
	}
	t.Run("未知のエラーは deterministic ではない", func(t *testing.T) {
		assert.False(t, isDeterministic(errors.New("random infra failure")))
	})
	t.Run("nil は deterministic ではない", func(t *testing.T) {
		assert.False(t, isDeterministic(nil))
	})
}

// respondError はステータスに加えて {"error": "..."} の body を返す責務も持つ。
// 後続の handler テストで body shape を重複検証しないよう、ここで body 形式を固定する。
func TestRespondError_WritesErrorBody(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondError(c, purchase.ErrAlreadyOwned)

	assert.Equal(t, http.StatusConflict, w.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, purchase.ErrAlreadyOwned.Error(), body["error"])
}
