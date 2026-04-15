package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// errorStatus は service の分類 API が返す値を HTTP status に変換する責務のみを
// 持つ。個別 sentinel の分類判断は service 側でテストされているので、ここでは
// 「分類 → ステータス」のマッピングのみを固定する (分類ごとに 1 sentinel で十分)。
func TestErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "NotFound 分類 → 404",
			err:        service.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Conflict 分類 → 409",
			err:        service.ErrAlreadyOwned,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "Validation 分類 → 400",
			err:        service.ErrInvalidFaction,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "PaymentFailed 分類 → 402",
			err:        service.ErrReceiptVerificationFailed,
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "分類外は 500 fallback (一時的 infra 障害を含む)",
			err:        service.ErrVerifyReceipt,
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

// respondError はステータスに加えて {"error": "..."} の body を返す責務も持つ。
// 後続の handler テストで body shape を重複検証しないよう、ここで body 形式を固定する。
func TestRespondError_WritesErrorBody(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondError(c, service.ErrAlreadyOwned)

	assert.Equal(t, http.StatusConflict, w.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, service.ErrAlreadyOwned.Error(), body["error"])
}
