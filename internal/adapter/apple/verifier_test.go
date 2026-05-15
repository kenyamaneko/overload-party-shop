package apple

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

func TestVerifier_decodeJWSPayload(t *testing.T) {
	cases := []struct {
		name     string
		verifyFn func(jws string) ([]byte, error)
		wantErr  bool
		want     *appleTransactionInfo
	}{
		{
			name:     "JWS 署名検証に失敗したら payload を返さない",
			verifyFn: func(string) ([]byte, error) { return nil, errors.New("x5c chain verify failed") },
			wantErr:  true,
		},
		{
			name:     "検証済み payload を transaction info に展開する",
			verifyFn: func(string) ([]byte, error) { return []byte(`{"transactionId":"txn-1","productId":"prod-1"}`), nil },
			want:     &appleTransactionInfo{TransactionID: "txn-1", ProductID: "prod-1"},
		},
		{
			name:     "検証は通ったが payload が JSON でない",
			verifyFn: func(string) ([]byte, error) { return []byte("not-json"), nil },
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &Verifier{jwsVerifier: &port.MockAppleJWSVerifier{VerifyFn: tc.verifyFn}}
			got, err := v.decodeJWSPayload("")
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestVerifier_decodeJWSRenewalPayload(t *testing.T) {
	cases := []struct {
		name     string
		verifyFn func(jws string) ([]byte, error)
		wantErr  bool
		want     *appleRenewalInfo
	}{
		{
			name:     "JWS 署名検証に失敗したら payload を返さない",
			verifyFn: func(string) ([]byte, error) { return nil, errors.New("x5c chain verify failed") },
			wantErr:  true,
		},
		{
			name:     "検証済み payload を renewal info に展開する",
			verifyFn: func(string) ([]byte, error) { return []byte(`{"autoRenewStatus":1}`), nil },
			want:     &appleRenewalInfo{AutoRenewStatus: 1},
		},
		{
			name:     "検証は通ったが payload が JSON でない",
			verifyFn: func(string) ([]byte, error) { return []byte("not-json"), nil },
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &Verifier{jwsVerifier: &port.MockAppleJWSVerifier{VerifyFn: tc.verifyFn}}
			got, err := v.decodeJWSRenewalPayload("")
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.want, got)
		})
	}
}
