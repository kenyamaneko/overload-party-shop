// Package appletest は apple パッケージのテストで共有する鍵と検証器の組み立てヘルパ。
package appletest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/apple"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const rsaKeyBits = 2048

// GenerateECDSAKey は指定した曲線の ECDSA 秘密鍵を生成する。
func GenerateECDSAKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	require.NoError(t, err)
	return key
}

// GenerateRSAKey は RSA 秘密鍵を生成する。
func GenerateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	require.NoError(t, err)
	return key
}

// EncodePKCS8PEM は秘密鍵を PKCS#8 形式の PEM にする。
func EncodePKCS8PEM(t *testing.T, key any) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// EncodeSEC1PEM は ECDSA 秘密鍵を SEC1 形式の PEM にする。
func EncodeSEC1PEM(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

// FakeJWSVerifier は署名を検証せず、決まった結果を返す port.AppleJWSVerifier の偽物。
type FakeJWSVerifier struct {
	Payload string
	Err     error
}

// Verify は Err があれば Err を、無ければ Payload を返す。
func (f *FakeJWSVerifier) Verify(_ string) ([]byte, error) {
	return []byte(f.Payload), f.Err
}

var _ port.AppleJWSVerifier = (*FakeJWSVerifier)(nil)

// NewVerifierFromPEM はキー ID test-key-id、発行者 ID test-issuer-id、バンドル ID com.test.shop で検証器を組み立てる。
func NewVerifierFromPEM(pemData []byte, environment string, jwsVerifier port.AppleJWSVerifier, opts ...apple.VerifierOption) (*apple.Verifier, error) {
	return apple.NewVerifierFromPEM("test-key-id", "test-issuer-id", "com.test.shop", pemData, environment, jwsVerifier, opts...)
}

// NewVerifier は endpoint を接続先にし、環境名 Sandbox の検証器を組み立てる。
func NewVerifier(t *testing.T, endpoint string, jwsVerifier port.AppleJWSVerifier) *apple.Verifier {
	t.Helper()
	pemData := EncodePKCS8PEM(t, GenerateECDSAKey(t, elliptic.P256()))
	v, err := NewVerifierFromPEM(pemData, apple.EnvironmentSandbox, jwsVerifier, apple.WithEndpoint(endpoint))
	require.NoError(t, err)
	return v
}
