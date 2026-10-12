package apple

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// appleRootCAG3PEM は Apple Root CA - G3（ECDSA P-384）。
// https://www.apple.com/certificateauthority/ からダウンロード。
//
//go:embed apple_root_ca_g3.pem
var appleRootCAG3PEM string

// JWSVerifier は Apple JWS を信頼するルート証明書に対して検証する port.AppleJWSVerifier 実装。
type JWSVerifier struct {
	trustedRoots *x509.CertPool
}

var _ port.AppleJWSVerifier = (*JWSVerifier)(nil)

// JWSVerifierOption は JWSVerifier の構築時の設定を差し替える。
type JWSVerifierOption func(*JWSVerifier)

// WithTrustedRoots は信頼するルート証明書を Apple Root CA - G3 から差し替える。
func WithTrustedRoots(roots *x509.CertPool) JWSVerifierOption {
	return func(v *JWSVerifier) {
		v.trustedRoots = roots
	}
}

// NewJWSVerifier は Apple Root CA - G3 を信頼する JWSVerifier を構築する。
func NewJWSVerifier(opts ...JWSVerifierOption) *JWSVerifier {
	v := &JWSVerifier{trustedRoots: mustLoadAppleRootPool()}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// mustLoadAppleRootPool は埋め込みの証明書が読めないときに panic する。埋め込みの証明書の破損はビルドの不具合であり、起動を続けても検証できないため。
func mustLoadAppleRootPool() *x509.CertPool {
	block, _ := pem.Decode([]byte(appleRootCAG3PEM))
	if block == nil {
		panic("Apple Root CA - G3: failed to decode PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic("Apple Root CA - G3: " + err.Error())
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return pool
}

type jwsHeader struct {
	Alg string   `json:"alg"`
	X5C []string `json:"x5c"`
}

// Verify は JWS の x5c 証明書チェーンと ECDSA 署名を検証し、生 payload バイト列を返す。
func (v *JWSVerifier) Verify(jws string) ([]byte, error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWS format: expected 3 parts, got %d", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode JWS header: %w", err)
	}

	var header jwsHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("unmarshal JWS header: %w", err)
	}

	if header.Alg != "ES256" {
		return nil, fmt.Errorf("unsupported JWS algorithm: %s", header.Alg)
	}

	if len(header.X5C) == 0 {
		return nil, fmt.Errorf("JWS header missing x5c certificate chain")
	}

	// x5c は leaf → intermediates → root の順で並ぶ仕様。
	certs := make([]*x509.Certificate, len(header.X5C))
	for i, certB64 := range header.X5C {
		certDER, err := base64.StdEncoding.DecodeString(certB64)
		if err != nil {
			return nil, fmt.Errorf("decode x5c[%d]: %w", i, err)
		}
		cert, err := x509.ParseCertificate(certDER)
		if err != nil {
			return nil, fmt.Errorf("parse x5c[%d]: %w", i, err)
		}
		certs[i] = cert
	}

	leaf := certs[0]
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}

	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         v.trustedRoots,
		Intermediates: intermediates,
	}); err != nil {
		return nil, fmt.Errorf("verify x5c certificate chain: %w", err)
	}

	pubKey, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("leaf certificate public key is not ECDSA")
	}

	signingInput := parts[0] + "." + parts[1]
	hash := sha256.Sum256([]byte(signingInput))

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode JWS signature: %w", err)
	}

	// ES256 署名は r || s（各 32 バイト）。
	if len(sigBytes) != 64 {
		return nil, fmt.Errorf("invalid ES256 signature length: expected 64 bytes, got %d", len(sigBytes))
	}

	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])

	if !ecdsa.Verify(pubKey, hash[:], r, s) {
		return nil, fmt.Errorf("JWS signature verification failed")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWS payload: %w", err)
	}

	return payload, nil
}
