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

type fakeGoogleNotifier struct {
	err   error
	calls int
}

func (f *fakeGoogleNotifier) HandleNotification(_ context.Context, _ subscription.GoogleRTDNMessage) error {
	f.calls++
	return f.err
}

func TestGoogleWebhookHandler(t *testing.T) {
	validBody := []byte(`{"message":{"data":"fake-base64"}}`)

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
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "確定的エラー (RTDN base64 壊れ) → 200 ack",
			body:       validBody,
			svcErr:     fmt.Errorf("wrap: %w", subscription.ErrDecodeRTDNData),
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "確定的エラー (RTDN JSON 壊れ) → 200 ack",
			body:       validBody,
			svcErr:     fmt.Errorf("wrap: %w", subscription.ErrUnmarshalRTDNData),
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "一時的エラー → 500",
			body:       validBody,
			svcErr:     errors.New("pubsub timeout"),
			wantStatus: http.StatusInternalServerError,
			wantCalls:  1,
		},
		{
			name:       "JSON として parse できない body → 400",
			body:       []byte(`not json`),
			wantStatus: http.StatusBadRequest,
			wantCalls:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &fakeGoogleNotifier{err: tt.svcErr}
			r := gin.New()
			r.POST("/webhook/google", NewGoogleWebhookHandler(notifier).Handle)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/webhook/google", bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCalls, notifier.calls)
		})
	}
}
