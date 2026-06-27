package google

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const (
	testPackageName    = "com.overloadparty.shop"
	testOrderID        = "GPA.3300-0000-0000-00000"
	purchaseTimeMillis = int64(1_700_000_000_000)
	expiryTimeMillis   = int64(1_800_000_000_000)
)

// Google Play Developer API の PurchaseState enum 値。
const (
	purchaseStatePurchased = 0
	purchaseStateCanceled  = 1
	purchaseStatePending   = 2
)

// newStubService は指定したステータスとボディを返すテストサーバへ向けた androidpublisher.Service を構築する。
func newStubService(t *testing.T, status int, body string) *androidpublisher.Service {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	svc, err := androidpublisher.NewService(context.Background(),
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL),
	)
	require.NoError(t, err)
	return svc
}

// TestSplitGoogleToken は合成トークンの分割契約を固定する。
func TestSplitGoogleToken(t *testing.T) {
	cases := []struct {
		name          string
		composite     string
		wantProductID string
		wantToken     string
		wantErr       bool
	}{
		{
			name:          "productId:token を二分割する",
			composite:     "premium_monthly:opaque-token",
			wantProductID: "premium_monthly",
			wantToken:     "opaque-token",
		},
		{
			name:          "token 部分に含まれる区切り文字は最初の : だけで分割する",
			composite:     "premium_monthly:opaque:token",
			wantProductID: "premium_monthly",
			wantToken:     "opaque:token",
		},
		{
			name:      "区切り文字が無い形式はエラー",
			composite: "no-separator",
			wantErr:   true,
		},
		{
			name:      "空文字はエラー",
			composite: "",
			wantErr:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			productID, token, err := splitGoogleToken(tc.composite)
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.wantProductID, productID)
			assert.Equal(t, tc.wantToken, token)
		})
	}
}

// TestVerifier_VerifyPurchase は購入検証の結果写像契約を固定する。
func TestVerifier_VerifyPurchase(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		status  int
		body    string
		want    *port.VerifyResult
		wantErr bool
	}{
		{
			name:   "purchased(0) はトークン由来の productID と order を持つ有効な結果になる",
			token:  "prod-1:opaque-token",
			status: http.StatusOK,
			body:   fmt.Sprintf(`{"purchaseState":%d,"orderId":%q,"purchaseTimeMillis":"%d"}`, purchaseStatePurchased, testOrderID, purchaseTimeMillis),
			want: &port.VerifyResult{
				IsValid:       true,
				TransactionID: testOrderID,
				ProductID:     "prod-1",
				PurchaseTime:  time.UnixMilli(purchaseTimeMillis),
			},
		},
		{
			name:   "canceled(1) は無効な結果になる",
			token:  "prod-1:opaque-token",
			status: http.StatusOK,
			body:   fmt.Sprintf(`{"purchaseState":%d}`, purchaseStateCanceled),
			want:   &port.VerifyResult{IsValid: false},
		},
		{
			name:   "pending(2) は無効な結果になる",
			token:  "prod-1:opaque-token",
			status: http.StatusOK,
			body:   fmt.Sprintf(`{"purchaseState":%d}`, purchaseStatePending),
			want:   &port.VerifyResult{IsValid: false},
		},
		{
			name:    "Google Play API がエラー応答を返すと無効な結果とエラーになる",
			token:   "prod-1:opaque-token",
			status:  http.StatusNotFound,
			body:    `{}`,
			want:    &port.VerifyResult{IsValid: false},
			wantErr: true,
		},
		{
			name:    "区切り文字の無いトークンは API 呼び出し前に無効な結果とエラーになる",
			token:   "no-separator",
			status:  http.StatusOK,
			body:    `{}`,
			want:    &port.VerifyResult{IsValid: false},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &Verifier{service: newStubService(t, tc.status, tc.body), packageName: testPackageName}
			got, err := v.VerifyPurchase(context.Background(), tc.token)
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestVerifier_VerifySubscription はサブスクリプション検証の結果写像契約を固定する。
func TestVerifier_VerifySubscription(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		status  int
		body    string
		want    *port.SubscriptionInfo
		wantErr bool
	}{
		{
			name:   "有効なサブスクは productID/order/有効期限/自動更新を写像する",
			token:  "premium_monthly:opaque-token",
			status: http.StatusOK,
			body:   fmt.Sprintf(`{"orderId":%q,"expiryTimeMillis":"%d","autoRenewing":true}`, testOrderID, expiryTimeMillis),
			want: &port.SubscriptionInfo{
				IsValid:        true,
				ProductID:      "premium_monthly",
				TransactionID:  testOrderID,
				ExpiresAt:      time.UnixMilli(expiryTimeMillis),
				IsAutoRenewing: true,
			},
		},
		{
			name:    "Google Play API がエラー応答を返すと無効な結果とエラーになる",
			token:   "premium_monthly:opaque-token",
			status:  http.StatusInternalServerError,
			body:    `{}`,
			want:    &port.SubscriptionInfo{IsValid: false},
			wantErr: true,
		},
		{
			name:    "区切り文字の無いトークンは API 呼び出し前に無効な結果とエラーになる",
			token:   "no-separator",
			status:  http.StatusOK,
			body:    `{}`,
			want:    &port.SubscriptionInfo{IsValid: false},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &Verifier{service: newStubService(t, tc.status, tc.body), packageName: testPackageName}
			got, err := v.VerifySubscription(context.Background(), tc.token)
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestSubVerifier_GetSubscriptionExpiry は有効期限取得の契約を固定する。
func TestSubVerifier_GetSubscriptionExpiry(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		status  int
		body    string
		want    time.Time
		wantErr bool
	}{
		{
			name:   "有効なサブスクは有効期限を返す",
			token:  "premium_monthly:opaque-token",
			status: http.StatusOK,
			body:   fmt.Sprintf(`{"expiryTimeMillis":"%d"}`, expiryTimeMillis),
			want:   time.UnixMilli(expiryTimeMillis),
		},
		{
			name:    "Google Play API がエラー応答を返すとゼロ値とエラーになる",
			token:   "premium_monthly:opaque-token",
			status:  http.StatusInternalServerError,
			body:    `{}`,
			want:    time.Time{},
			wantErr: true,
		},
		{
			name:    "区切り文字の無いトークンは API 呼び出し前にゼロ値とエラーになる",
			token:   "no-separator",
			status:  http.StatusOK,
			body:    `{}`,
			want:    time.Time{},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &SubVerifier{service: newStubService(t, tc.status, tc.body), packageName: testPackageName}
			got, err := v.GetSubscriptionExpiry(context.Background(), tc.token)
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.want, got)
		})
	}
}
