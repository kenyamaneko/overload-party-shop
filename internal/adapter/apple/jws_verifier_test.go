package apple_test

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

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/apple"
	"github.com/kenyamaneko/overload-party-shop/internal/adapter/apple/appletest"
)

const (
	testCertificateValidity      = 24 * time.Hour
	testCertificateStartBackdate = time.Hour
	testCertificateSerialBits    = 62
	testSignatureHalfSize        = 32
)

type trustedChain struct {
	verifier *apple.JWSVerifier
	x5c      []string
}

func encodeSegment(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func encodeCertificate(der []byte) string {
	return base64.StdEncoding.EncodeToString(der)
}

func encodeHeader(t *testing.T, header map[string]any) string {
	t.Helper()
	data, err := json.Marshal(header)
	require.NoError(t, err)
	return encodeSegment(data)
}

func newCertificateTemplate(t *testing.T, commonName string) *x509.Certificate {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), testCertificateSerialBits))
	require.NoError(t, err)
	return &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-testCertificateStartBackdate),
		NotAfter:     time.Now().Add(testCertificateValidity),
	}
}

func newSelfSignedCertificate(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	template := newCertificateTemplate(t, "test-self-signed")
	template.KeyUsage = x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return der
}

func newTrustedChain(t *testing.T, leafPublicKey any) trustedChain {
	t.Helper()
	rootKey := appletest.GenerateECDSAKey(t, elliptic.P256())
	rootTemplate := newCertificateTemplate(t, "test-root")
	rootTemplate.IsCA = true
	rootTemplate.BasicConstraintsValid = true
	rootTemplate.KeyUsage = x509.KeyUsageCertSign
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	require.NoError(t, err)
	rootCert, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)

	leafTemplate := newCertificateTemplate(t, "test-leaf")
	leafTemplate.KeyUsage = x509.KeyUsageDigitalSignature
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, leafPublicKey, rootKey)
	require.NoError(t, err)

	roots := x509.NewCertPool()
	roots.AddCert(rootCert)
	return trustedChain{
		verifier: apple.NewJWSVerifier(apple.WithTrustedRoots(roots)),
		x5c:      []string{encodeCertificate(leafDER)},
	}
}

func signES256(t *testing.T, key *ecdsa.PrivateKey, signingInput string) []byte {
	t.Helper()
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	require.NoError(t, err)
	signature := make([]byte, 2*testSignatureHalfSize)
	r.FillBytes(signature[:testSignatureHalfSize])
	s.FillBytes(signature[testSignatureHalfSize:])
	return signature
}

func TestJWSVerifier_Verify(t *testing.T) {
	t.Run("[Apple購入検証]署名付きデータの署名検証", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("テスト専用のルート証明書を渡して生成した検証器で、ルート証明書に連なるx5cとES256の正しい署名を持つJWSを検証すると", func(t *testing.T) {
				newValidJWS := func(t *testing.T) (*apple.JWSVerifier, string) {
					t.Helper()
					leafKey := appletest.GenerateECDSAKey(t, elliptic.P256())
					chain := newTrustedChain(t, &leafKey.PublicKey)
					header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": chain.x5c})
					payload := encodeSegment([]byte("test-payload"))
					signature := encodeSegment(signES256(t, leafKey, header+"."+payload))
					return chain.verifier, header + "." + payload + "." + signature
				}

				t.Run("エラーは返らない", func(t *testing.T) {
					verifier, jws := newValidJWS(t)

					_, err := verifier.Verify(jws)

					require.NoError(t, err)
				})

				t.Run("JWSのペイロードのバイト列がそのまま返る", func(t *testing.T) {
					verifier, jws := newValidJWS(t)

					got, err := verifier.Verify(jws)

					require.NoError(t, err)
					assert.Equal(t, []byte("test-payload"), got)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("テスト専用のルート証明書を渡して生成した検証器で、ルート証明書に連なるx5cを持つJWSを検証すると", func(t *testing.T) {
				t.Run("葉の証明書の公開鍵がECDSAでないとき、エラーの内容にECDSAでない公開鍵が含まれる", func(t *testing.T) {
					chain := newTrustedChain(t, &appletest.GenerateRSAKey(t).PublicKey)
					header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": chain.x5c})
					jws := header + "." + encodeSegment([]byte("test-payload")) + "." + encodeSegment(make([]byte, 2*testSignatureHalfSize))

					_, err := chain.verifier.Verify(jws)

					assert.ErrorContains(t, err, "leaf certificate public key is not ECDSA")
				})

				t.Run("署名がbase64でないとき、エラーの内容に署名デコード失敗が含まれる", func(t *testing.T) {
					leafKey := appletest.GenerateECDSAKey(t, elliptic.P256())
					chain := newTrustedChain(t, &leafKey.PublicKey)
					header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": chain.x5c})
					jws := header + "." + encodeSegment([]byte("test-payload")) + ".!!!"

					_, err := chain.verifier.Verify(jws)

					assert.ErrorContains(t, err, "decode JWS signature")
				})

				t.Run("署名が64バイトで署名対象と一致しないとき、エラーの内容に署名不一致が含まれる", func(t *testing.T) {
					leafKey := appletest.GenerateECDSAKey(t, elliptic.P256())
					chain := newTrustedChain(t, &leafKey.PublicKey)
					header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": chain.x5c})
					payload := encodeSegment([]byte("test-payload"))
					signatureOfOtherInput := encodeSegment(signES256(t, leafKey, "other-signing-input"))
					jws := header + "." + payload + "." + signatureOfOtherInput

					_, err := chain.verifier.Verify(jws)

					assert.ErrorContains(t, err, "JWS signature verification failed")
				})

				t.Run("署名が正しく、ペイロードがbase64でないとき、エラーの内容にペイロードデコード失敗が含まれる", func(t *testing.T) {
					leafKey := appletest.GenerateECDSAKey(t, elliptic.P256())
					chain := newTrustedChain(t, &leafKey.PublicKey)
					header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": chain.x5c})
					payload := "!!!"
					signature := encodeSegment(signES256(t, leafKey, header+"."+payload))
					jws := header + "." + payload + "." + signature

					_, err := chain.verifier.Verify(jws)

					assert.ErrorContains(t, err, "decode JWS payload")
				})

				signatureLengthTests := []struct {
					name          string
					signatureSize int
				}{
					{name: "署名が63バイトのとき、エラーの内容に署名長不正が含まれる", signatureSize: 63},
					{name: "署名が65バイトのとき、エラーの内容に署名長不正が含まれる", signatureSize: 65},
				}
				for _, tc := range signatureLengthTests {
					t.Run(tc.name, func(t *testing.T) {
						leafKey := appletest.GenerateECDSAKey(t, elliptic.P256())
						chain := newTrustedChain(t, &leafKey.PublicKey)
						header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": chain.x5c})
						signature := encodeSegment(make([]byte, tc.signatureSize))
						jws := header + "." + encodeSegment([]byte("test-payload")) + "." + signature

						_, err := chain.verifier.Verify(jws)

						assert.ErrorContains(t, err, "invalid ES256 signature length")
					})
				}
			})

			t.Run("署名付きデータを検証すると", func(t *testing.T) {
				newDefaultVerifier := func() *apple.JWSVerifier { return apple.NewJWSVerifier() }
				payload := encodeSegment([]byte("test-payload"))
				signature := encodeSegment(make([]byte, 2*testSignatureHalfSize))

				partCountTests := []struct {
					name string
					jws  string
				}{
					{name: "JWSをドットで区切った部分が2個のとき、エラーの内容に構造不正が含まれる", jws: "part1.part2"},
					{name: "JWSをドットで区切った部分が4個のとき、エラーの内容に構造不正が含まれる", jws: "part1.part2.part3.part4"},
				}
				for _, tc := range partCountTests {
					t.Run(tc.name, func(t *testing.T) {
						_, err := newDefaultVerifier().Verify(tc.jws)

						assert.ErrorContains(t, err, "invalid JWS format")
					})
				}

				t.Run("ヘッダがbase64でないとき、エラーの内容にヘッダ読み取り失敗が含まれる", func(t *testing.T) {
					_, err := newDefaultVerifier().Verify("!!!." + payload + "." + signature)

					assert.ErrorContains(t, err, "decode JWS header")
				})

				t.Run("ヘッダがJSONでないとき、エラーの内容にヘッダ解析失敗が含まれる", func(t *testing.T) {
					header := encodeSegment([]byte("not-json"))

					_, err := newDefaultVerifier().Verify(header + "." + payload + "." + signature)

					assert.ErrorContains(t, err, "unmarshal JWS header")
				})

				t.Run("ヘッダのalgがRS256のとき、エラーの内容に未対応アルゴリズムが含まれる", func(t *testing.T) {
					header := encodeHeader(t, map[string]any{"alg": "RS256"})

					_, err := newDefaultVerifier().Verify(header + "." + payload + "." + signature)

					assert.ErrorContains(t, err, "unsupported JWS algorithm: RS256")
				})

				t.Run("ヘッダにx5cが無いとき、エラーの内容に証明書チェーン欠落が含まれる", func(t *testing.T) {
					header := encodeHeader(t, map[string]any{"alg": "ES256"})

					_, err := newDefaultVerifier().Verify(header + "." + payload + "." + signature)

					assert.ErrorContains(t, err, "missing x5c certificate chain")
				})

				t.Run("x5cの先頭の要素がbase64でないとき、エラーの内容に証明書デコード失敗が含まれる", func(t *testing.T) {
					header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": []string{"!!!"}})

					_, err := newDefaultVerifier().Verify(header + "." + payload + "." + signature)

					assert.ErrorContains(t, err, "decode x5c[0]")
				})

				t.Run("x5cの先頭の要素が証明書として解釈できないとき、エラーの内容に証明書解析失敗が含まれる", func(t *testing.T) {
					notCertificate := encodeCertificate([]byte("not-a-certificate"))
					header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": []string{notCertificate}})

					_, err := newDefaultVerifier().Verify(header + "." + payload + "." + signature)

					assert.ErrorContains(t, err, "parse x5c[0]")
				})

				t.Run("x5cがAppleRootCAを根に持たない自己署名証明書のみのとき、エラーの内容に証明書チェーン検証失敗が含まれる", func(t *testing.T) {
					key := appletest.GenerateECDSAKey(t, elliptic.P256())
					header := encodeHeader(t, map[string]any{"alg": "ES256", "x5c": []string{encodeCertificate(newSelfSignedCertificate(t, key))}})
					jws := header + "." + payload + "." + encodeSegment(signES256(t, key, header+"."+payload))

					_, err := newDefaultVerifier().Verify(jws)

					assert.ErrorContains(t, err, "verify x5c certificate chain")
				})
			})
		})
	})
}
