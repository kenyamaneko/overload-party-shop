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

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

type fakeAppleNotifier struct {
	err   error
	calls int
}

func (f *fakeAppleNotifier) HandleNotification(_ context.Context, _ string) error {
	f.calls++
	return f.err
}

func TestAppleWebhookHandler(t *testing.T) {
	t.Run("Apple 通知 webhook の応答変換", func(t *testing.T) {
		validBody := []byte(`{"signedPayload":"fake-jws"}`)

		// webhook handler は usecase 層の「何が起きたか」を HTTP リトライプロトコルに
		// 変換する責務のみを持つ。確定的エラーは ack (200) してストア側のリトライを
		// 止め、一時的エラーは 5xx で返してリトライを促す。usecase のエラー分類は
		// usecase 側でテストされているので、ここでは 1 sentinel ずつで代表する。
		tests := []struct {
			name       string
			body       []byte
			svcErr     error
			wantStatus int
			wantCalls  int
		}{
			{
				name:       "notifier が成功するとき、200 で ack する",
				body:       validBody,
				svcErr:     nil,
				wantStatus: http.StatusOK,
				wantCalls:  1,
			},
			{
				name:       "usecase が ErrDecodeNotification を wrap したエラーを返すとき、200 で ack してリトライを止める",
				body:       validBody,
				svcErr:     fmt.Errorf("wrap: %w", subscription.ErrDecodeNotification),
				wantStatus: http.StatusOK,
				wantCalls:  1,
			},
			{
				name:       "usecase が ErrSubscriptionNotFound を wrap したエラーを返すとき、200 で ack する",
				body:       validBody,
				svcErr:     fmt.Errorf("wrap: %w", subscription.ErrSubscriptionNotFound),
				wantStatus: http.StatusOK,
				wantCalls:  1,
			},
			{
				name:       "usecase が ErrDecodeTransactionInfo を wrap したエラーを返すとき、200 で ack する",
				body:       validBody,
				svcErr:     fmt.Errorf("wrap: %w", subscription.ErrDecodeTransactionInfo),
				wantStatus: http.StatusOK,
				wantCalls:  1,
			},
			{
				name:       "usecase が一時的エラーを返すとき、500 でリトライを促す",
				body:       validBody,
				svcErr:     errors.New("database unavailable"),
				wantStatus: http.StatusInternalServerError,
				wantCalls:  1,
			},
			{
				name:       "body が JSON として parse できないとき、400 になり notifier は呼ばれない",
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
	})
}
