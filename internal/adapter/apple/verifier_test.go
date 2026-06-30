package apple

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const (
	testPurchaseMillis = int64(1_700_000_000_000)
	testExpiresMillis  = int64(1_800_000_000_000)
)

// newTestECDSAKey は JWT 署名に使う P-256 秘密鍵を生成する。
func newTestECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

// newTestVerifier は App Store Server API を模したテストサーバへ向けた Verifier を構築する。
// jwsVerifier をスタブ化することで、署名検証後の payload をテストが直接指定できる。
func newTestVerifier(t *testing.T, status int, body string, verifyFn func(jws string) ([]byte, error)) *Verifier {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	return &Verifier{
		keyID:       "test-key-id",
		issuerID:    "test-issuer-id",
		bundleID:    "com.overloadparty.shop",
		privateKey:  newTestECDSAKey(t),
		baseURL:     server.URL,
		httpClient:  server.Client(),
		jwsVerifier: &port.MockAppleJWSVerifier{VerifyFn: verifyFn},
	}
}

// TestVerifier_VerifyPurchase は単発購入検証の結果写像と異常系伝播を公開経路で固定する。
func TestVerifier_VerifyPurchase(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		verifyFn func(jws string) ([]byte, error)
		want     *port.VerifyResult
		wantErr  bool
	}{
		{
			name:   "200 と検証済み transaction から有効な結果を写像する",
			status: http.StatusOK,
			body:   `{"signedTransactionInfo":"signed-txn"}`,
			verifyFn: func(string) ([]byte, error) {
				return []byte(fmt.Sprintf(`{"transactionId":"txn-1","productId":"prod-1","purchaseDate":%d}`, testPurchaseMillis)), nil
			},
			want: &port.VerifyResult{
				IsValid:       true,
				TransactionID: "txn-1",
				ProductID:     "prod-1",
				PurchaseTime:  time.UnixMilli(testPurchaseMillis),
			},
			wantErr: false,
		},
		{
			name:     "非200応答は無効な結果とエラーになる",
			status:   http.StatusNotFound,
			body:     `{"errorCode":4040010}`,
			verifyFn: func(string) ([]byte, error) { return []byte("{}"), nil },
			want:     &port.VerifyResult{IsValid: false},
			wantErr:  true,
		},
		{
			name:     "JWS 署名検証に失敗すると結果を返さずエラーを伝播する",
			status:   http.StatusOK,
			body:     `{"signedTransactionInfo":"signed-txn"}`,
			verifyFn: func(string) ([]byte, error) { return nil, errors.New("x5c chain verify failed") },
			want:     nil,
			wantErr:  true,
		},
		{
			name:     "署名は通るが payload が JSON でないとエラーになる",
			status:   http.StatusOK,
			body:     `{"signedTransactionInfo":"signed-txn"}`,
			verifyFn: func(string) ([]byte, error) { return []byte("not-json"), nil },
			want:     nil,
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVerifier(t, tc.status, tc.body, tc.verifyFn)
			got, err := v.VerifyPurchase(context.Background(), "purchase-token")
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestVerifier_VerifySubscription はサブスクリプション検証の結果写像と異常系伝播を公開経路で固定する。
func TestVerifier_VerifySubscription(t *testing.T) {
	const subBody = `{"data":[{"lastTransactions":[{"signedTransactionInfo":"signed-txn","signedRenewalInfo":"signed-renewal"}]}]}`
	cases := []struct {
		name     string
		status   int
		body     string
		verifyFn func(jws string) ([]byte, error)
		want     *port.SubscriptionInfo
		wantErr  bool
	}{
		{
			name:   "200 と検証済み transaction/renewal から有効なサブスク結果を写像する",
			status: http.StatusOK,
			body:   subBody,
			verifyFn: func(string) ([]byte, error) {
				return []byte(fmt.Sprintf(`{"transactionId":"txn-sub","productId":"premium_monthly","expiresDate":%d,"autoRenewStatus":1}`, testExpiresMillis)), nil
			},
			want: &port.SubscriptionInfo{
				IsValid:        true,
				ProductID:      "premium_monthly",
				TransactionID:  "txn-sub",
				ExpiresAt:      time.UnixMilli(testExpiresMillis),
				IsAutoRenewing: true,
			},
			wantErr: false,
		},
		{
			name:   "autoRenewStatus が 1 以外なら自動更新は false になる",
			status: http.StatusOK,
			body:   subBody,
			verifyFn: func(string) ([]byte, error) {
				return []byte(fmt.Sprintf(`{"transactionId":"txn-sub","productId":"premium_monthly","expiresDate":%d,"autoRenewStatus":0}`, testExpiresMillis)), nil
			},
			want: &port.SubscriptionInfo{
				IsValid:        true,
				ProductID:      "premium_monthly",
				TransactionID:  "txn-sub",
				ExpiresAt:      time.UnixMilli(testExpiresMillis),
				IsAutoRenewing: false,
			},
			wantErr: false,
		},
		{
			name:     "非200応答は無効な結果とエラーになる",
			status:   http.StatusInternalServerError,
			body:     `{}`,
			verifyFn: func(string) ([]byte, error) { return []byte("{}"), nil },
			want:     &port.SubscriptionInfo{IsValid: false},
			wantErr:  true,
		},
		{
			name:     "lastTransactions が空なら無効な結果でエラーは返さない",
			status:   http.StatusOK,
			body:     `{"data":[]}`,
			verifyFn: func(string) ([]byte, error) { return []byte("{}"), nil },
			want:     &port.SubscriptionInfo{IsValid: false},
			wantErr:  false,
		},
		{
			name:     "JWS 署名検証に失敗すると結果を返さずエラーを伝播する",
			status:   http.StatusOK,
			body:     subBody,
			verifyFn: func(string) ([]byte, error) { return nil, errors.New("x5c chain verify failed") },
			want:     nil,
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVerifier(t, tc.status, tc.body, tc.verifyFn)
			got, err := v.VerifySubscription(context.Background(), "sub-token")
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.want, got)
		})
	}
}
