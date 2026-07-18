package apple

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encodeJWSHeader は JWS header (map) を JSON エンコードしてから base64url (無パディング) にする。
func encodeJWSHeader(t *testing.T, header map[string]any) string {
	t.Helper()
	data, err := json.Marshal(header)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(data)
}

// buildSelfSignedLeafJWS は Apple Root CA を根に持たない自己署名 leaf 証明書で
// 実際に ES256 署名した JWS を構築する。署名自体は正しいが、チェーン検証で
// Apple Root CA に到達しない Given を作るためのヘルパ。
func buildSelfSignedLeafJWS(t *testing.T, payload []byte) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-self-signed-leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	header := encodeJWSHeader(t, map[string]any{
		"alg": "ES256",
		"x5c": []string{base64.StdEncoding.EncodeToString(certDER)},
	})
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := header + "." + payloadB64

	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	require.NoError(t, err)

	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigB64
}

func TestJWSVerifier_Verify(t *testing.T) {
	t.Run("JWS 検証", func(t *testing.T) {
		fixedPayloadB64 := base64.RawURLEncoding.EncodeToString([]byte("payload"))

		cases := []struct {
			name            string
			buildJWS        func(t *testing.T) string
			wantErrContains string
		}{
			{
				name:            "3 分割でない (2 分割の) JWS のとき、エラーになる",
				buildJWS:        func(t *testing.T) string { return "a.b" },
				wantErrContains: "invalid JWS format",
			},
			{
				name:            "header が base64 でないとき、エラーになる",
				buildJWS:        func(t *testing.T) string { return "!!!." + fixedPayloadB64 + ".c2ln" },
				wantErrContains: "decode JWS header",
			},
			{
				name: "header が JSON でないとき、エラーになる",
				buildJWS: func(t *testing.T) string {
					header := base64.RawURLEncoding.EncodeToString([]byte("not json"))
					return header + "." + fixedPayloadB64 + ".c2ln"
				},
				wantErrContains: "unmarshal JWS header",
			},
			{
				name: "alg が ES256 でない (RS256) とき、エラーになる",
				buildJWS: func(t *testing.T) string {
					header := encodeJWSHeader(t, map[string]any{"alg": "RS256", "x5c": []string{"dummy"}})
					return header + "." + fixedPayloadB64 + ".c2ln"
				},
				wantErrContains: "unsupported JWS algorithm: RS256",
			},
			{
				name: "x5c が無いとき、エラーになる",
				buildJWS: func(t *testing.T) string {
					header := encodeJWSHeader(t, map[string]any{"alg": "ES256"})
					return header + "." + fixedPayloadB64 + ".c2ln"
				},
				wantErrContains: "missing x5c certificate chain",
			},
			{
				name: "x5c が証明書として parse できないとき、エラーになる",
				buildJWS: func(t *testing.T) string {
					header := encodeJWSHeader(t, map[string]any{
						"alg": "ES256",
						"x5c": []string{base64.StdEncoding.EncodeToString([]byte("not der"))},
					})
					return header + "." + fixedPayloadB64 + ".c2ln"
				},
				wantErrContains: "parse x5c[0]",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				v := NewJWSVerifier()
				_, err := v.Verify(tc.buildJWS(t))
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			})
		}

		t.Run("Apple Root CA を根に持たない自己署名チェーンのとき、エラーになる", func(t *testing.T) {
			v := NewJWSVerifier()
			jws := buildSelfSignedLeafJWS(t, []byte("payload"))

			_, err := v.Verify(jws)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "verify x5c certificate chain")
		})
	})
}
