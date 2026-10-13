package apple_test

import (
	"context"
	"crypto/elliptic"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/apple"
	"github.com/kenyamaneko/overload-party-shop/internal/adapter/apple/appletest"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func loadPrivateKeyPEM(pemData []byte) (*apple.Verifier, error) {
	return appletest.NewVerifierFromPEM(pemData, "Sandbox", &appletest.FakeJWSVerifier{})
}

func newVerifierWithEnvironment(t *testing.T, environment string, opts ...apple.VerifierOption) (*apple.Verifier, error) {
	t.Helper()
	pemData := appletest.EncodePKCS8PEM(t, appletest.GenerateECDSAKey(t, elliptic.P256()))
	jwsVerifier := &appletest.FakeJWSVerifier{Payload: `{"bundleId":"com.test.shop","productId":"prod-1"}`}
	return appletest.NewVerifierFromPEM(pemData, environment, jwsVerifier, opts...)
}

func captureRequestOrigin(t *testing.T, environment string) *url.URL {
	t.Helper()
	var captured *url.URL
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		captured = r.URL
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"signedTransactionInfo":"signed-txn"}`)),
			Request:    r,
		}, nil
	})}
	verifier, err := newVerifierWithEnvironment(t, environment, apple.WithHTTPClient(client))
	require.NoError(t, err)

	_, _ = verifier.VerifyPurchase(context.Background(), "purchase-token")

	require.NotNil(t, captured)
	return captured
}

func TestNewVerifierFromPEM(t *testing.T) {
	t.Run("[Apple購入検証]認証用秘密鍵の読み込み", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("秘密鍵のPEMを読み込むと", func(t *testing.T) {
				t.Run("P-256のPKCS#8形式の秘密鍵のとき、エラーは返らない", func(t *testing.T) {
					pemData := appletest.EncodePKCS8PEM(t, appletest.GenerateECDSAKey(t, elliptic.P256()))

					_, err := loadPrivateKeyPEM(pemData)

					require.NoError(t, err)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("秘密鍵のPEMを読み込むと", func(t *testing.T) {
				invalidPEMTests := []struct {
					name    string
					pemData []byte
				}{
					{name: "空のデータのとき、エラーの内容にPEMブロック解析失敗が含まれる", pemData: []byte{}},
					{name: "PEMブロックを含まない文字列のとき、エラーの内容にPEMブロック解析失敗が含まれる", pemData: []byte("not-a-pem-block")},
				}
				for _, tc := range invalidPEMTests {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadPrivateKeyPEM(tc.pemData)

						assert.ErrorContains(t, err, "failed to decode PEM block")
					})
				}

				t.Run("PKCS#8形式でないSEC1形式のEC秘密鍵のとき、エラーの内容に鍵解析失敗が含まれる", func(t *testing.T) {
					pemData := appletest.EncodeSEC1PEM(t, appletest.GenerateECDSAKey(t, elliptic.P256()))

					_, err := loadPrivateKeyPEM(pemData)

					assert.ErrorContains(t, err, "parse private key")
				})

				t.Run("PKCS#8形式のRSA秘密鍵のとき、エラーの内容にECDSAでない鍵が含まれる", func(t *testing.T) {
					pemData := appletest.EncodePKCS8PEM(t, appletest.GenerateRSAKey(t))

					_, err := loadPrivateKeyPEM(pemData)

					assert.ErrorContains(t, err, "private key is not ECDSA")
				})

				t.Run("PKCS#8形式のP-384の秘密鍵のとき、エラーの内容にP-256でない曲線が含まれる", func(t *testing.T) {
					pemData := appletest.EncodePKCS8PEM(t, appletest.GenerateECDSAKey(t, elliptic.P384()))

					_, err := loadPrivateKeyPEM(pemData)

					assert.ErrorContains(t, err, "P-256")
				})
			})
		})
	})

	t.Run("[Apple購入検証]環境名ごとの接続先", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("単発購入を検証すると", func(t *testing.T) {
				t.Run("環境名がProductionで接続先を差し替えないとき、AppStoreServerAPIへの要求はhttps://api.storekit.itunes.apple.comに届く", func(t *testing.T) {
					got := captureRequestOrigin(t, "Production")

					assert.Equal(t, "https://api.storekit.itunes.apple.com", got.Scheme+"://"+got.Host)
				})

				t.Run("環境名がSandboxで接続先を差し替えないとき、AppStoreServerAPIへの要求はhttps://api.storekit-sandbox.itunes.apple.comに届く", func(t *testing.T) {
					got := captureRequestOrigin(t, "Sandbox")

					assert.Equal(t, "https://api.storekit-sandbox.itunes.apple.com", got.Scheme+"://"+got.Host)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("検証器を生成すると", func(t *testing.T) {
				invalidEnvironmentTests := []struct {
					name        string
					environment string
				}{
					{name: "環境名が空文字のとき、エラーの内容に許容値のProductionとSandboxが含まれる", environment: ""},
					{name: "環境名がstagingのとき、エラーの内容に許容値のProductionとSandboxが含まれる", environment: "staging"},
				}
				for _, tc := range invalidEnvironmentTests {
					t.Run(tc.name, func(t *testing.T) {
						_, err := newVerifierWithEnvironment(t, tc.environment)

						assert.ErrorContains(t, err, "Production")
						assert.ErrorContains(t, err, "Sandbox")
					})
				}

				t.Run("環境名がstagingのとき、エラーの内容に渡した環境名stagingが含まれる", func(t *testing.T) {
					_, err := newVerifierWithEnvironment(t, "staging")

					assert.ErrorContains(t, err, "staging")
				})
			})
		})
	})
}
