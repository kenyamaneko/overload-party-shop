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

	purchaseStateCanceled = 1
	purchaseStatePending  = 2
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

func TestSplitGoogleToken(t *testing.T) {
	t.Run("合成トークンの分割", func(t *testing.T) {
		validCases := []struct {
			name          string
			composite     string
			wantProductID string
			wantToken     string
		}{
			{
				name:          "productId:token のとき、二分割する",
				composite:     "premium_monthly:opaque-token",
				wantProductID: "premium_monthly",
				wantToken:     "opaque-token",
			},
			{
				name:          "token 部分に区切り文字を含むとき、最初の : だけで分割する",
				composite:     "premium_monthly:opaque:token",
				wantProductID: "premium_monthly",
				wantToken:     "opaque:token",
			},
		}
		for _, tc := range validCases {
			t.Run(tc.name, func(t *testing.T) {
				productID, token, err := splitGoogleToken(tc.composite)
				require.NoError(t, err)
				assert.Equal(t, tc.wantProductID, productID)
				assert.Equal(t, tc.wantToken, token)
			})
		}

		invalidCases := []struct {
			name      string
			composite string
		}{
			{
				name:      "区切り文字が無いとき、エラーになる",
				composite: "no-separator",
			},
			{
				name:      "空文字のとき、エラーになる",
				composite: "",
			},
		}
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				productID, token, err := splitGoogleToken(tc.composite)
				require.Error(t, err)
				assert.Empty(t, productID)
				assert.Empty(t, token)
			})
		}
	})
}

func TestVerifier_VerifyPurchase(t *testing.T) {
	t.Run("単発購入の検証", func(t *testing.T) {
		validCases := []struct {
			name   string
			token  string
			status int
			body   string
			want   *port.VerifyResult
		}{
			{
				name:   "purchased(0) のとき、トークン由来の productID と order を持つ有効な結果になる",
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
				name:   "canceled(1) のとき、無効な結果になる",
				token:  "prod-1:opaque-token",
				status: http.StatusOK,
				body:   fmt.Sprintf(`{"purchaseState":%d}`, purchaseStateCanceled),
				want:   &port.VerifyResult{IsValid: false},
			},
			{
				name:   "pending(2) のとき、無効な結果になる",
				token:  "prod-1:opaque-token",
				status: http.StatusOK,
				body:   fmt.Sprintf(`{"purchaseState":%d}`, purchaseStatePending),
				want:   &port.VerifyResult{IsValid: false},
			},
		}
		for _, tc := range validCases {
			t.Run(tc.name, func(t *testing.T) {
				v := &Verifier{service: newStubService(t, tc.status, tc.body), packageName: testPackageName}
				got, err := v.VerifyPurchase(context.Background(), tc.token)
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			})
		}

		invalidCases := []struct {
			name   string
			token  string
			status int
			body   string
			want   *port.VerifyResult
		}{
			{
				name:   "Google Play API がエラー応答を返すとき、無効な結果とエラーになる",
				token:  "prod-1:opaque-token",
				status: http.StatusNotFound,
				body:   `{}`,
				want:   &port.VerifyResult{IsValid: false},
			},
			{
				name:   "区切り文字の無いトークンのとき、API 呼び出し前に無効な結果とエラーになる",
				token:  "no-separator",
				status: http.StatusOK,
				body:   `{}`,
				want:   &port.VerifyResult{IsValid: false},
			},
		}
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				v := &Verifier{service: newStubService(t, tc.status, tc.body), packageName: testPackageName}
				got, err := v.VerifyPurchase(context.Background(), tc.token)
				require.Error(t, err)
				assert.Equal(t, tc.want, got)
			})
		}
	})
}

func TestVerifier_VerifySubscription(t *testing.T) {
	t.Run("サブスクリプションの検証", func(t *testing.T) {
		cases := []struct {
			name    string
			token   string
			status  int
			body    string
			want    *port.SubscriptionInfo
			wantErr bool
		}{
			{
				name:   "有効なサブスクのとき、productID/order/有効期限/自動更新を写像する",
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
				name:    "Google Play API がエラー応答を返すとき、無効な結果とエラーになる",
				token:   "premium_monthly:opaque-token",
				status:  http.StatusInternalServerError,
				body:    `{}`,
				want:    &port.SubscriptionInfo{IsValid: false},
				wantErr: true,
			},
			{
				name:    "区切り文字の無いトークンのとき、API 呼び出し前に無効な結果とエラーになる",
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
	})
}

func TestSubVerifier_GetSubscriptionExpiry(t *testing.T) {
	t.Run("有効期限の取得", func(t *testing.T) {
		cases := []struct {
			name    string
			token   string
			status  int
			body    string
			want    time.Time
			wantErr bool
		}{
			{
				name:   "有効なサブスクのとき、有効期限を返す",
				token:  "premium_monthly:opaque-token",
				status: http.StatusOK,
				body:   fmt.Sprintf(`{"expiryTimeMillis":"%d"}`, expiryTimeMillis),
				want:   time.UnixMilli(expiryTimeMillis),
			},
			{
				name:    "Google Play API がエラー応答を返すとき、ゼロ値とエラーになる",
				token:   "premium_monthly:opaque-token",
				status:  http.StatusInternalServerError,
				body:    `{}`,
				want:    time.Time{},
				wantErr: true,
			},
			{
				name:    "区切り文字の無いトークンのとき、API 呼び出し前にゼロ値とエラーになる",
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
	})
}
