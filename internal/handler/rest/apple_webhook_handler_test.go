package rest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/service/subscription"
)

type fakeAppleNotifier struct {
	err   error
	calls int
}

func (f *fakeAppleNotifier) HandleNotification(_ context.Context, _ string) error {
	f.calls++
	return f.err
}

// webhook handler は service 層の「何が起きたか」を HTTP リトライプロトコルに
// 変換する責務のみを持つ。確定的エラーは ack (200) してストア側のリトライを
// 止め、一時的エラーは 5xx で返してリトライを促す。service のエラー分類は
// service 側でテストされているので、ここでは 1 sentinel ずつで代表する。
func TestAppleWebhookHandler(t *testing.T) {
	validBody := []byte(`{"signedPayload":"fake-jws"}`)

	tests := []struct {
		name       string
		body       []byte
		svcErr     error
		wantStatus int
		wantCalls  int
	}{
		{
			name:       "成功 → 200 ack",
			body:       validBody,
			svcErr:     nil,
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "確定的エラー (decode 失敗) → 200 ack してリトライを止める",
			body:       validBody,
			svcErr:     fmt.Errorf("wrap: %w", subscription.ErrDecodeNotification),
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "確定的エラー (subscription 未存在) → 200 ack",
			body:       validBody,
			svcErr:     fmt.Errorf("wrap: %w", subscription.ErrSubscriptionNotFound),
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "一時的エラー (DB 障害等) → 500 でリトライを促す",
			body:       validBody,
			svcErr:     errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantCalls:  1,
		},
		{
			name:       "JSON として parse できない body → 400 (service 未到達)",
			body:       []byte(`not json`),
			svcErr:     nil,
			wantStatus: http.StatusBadRequest,
			wantCalls:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &fakeAppleNotifier{err: tt.svcErr}
			r := gin.New()
			r.POST("/webhook/apple", NewAppleWebhookHandler(notifier).Handle)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/webhook/apple", bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCalls, notifier.calls)
		})
	}
}
