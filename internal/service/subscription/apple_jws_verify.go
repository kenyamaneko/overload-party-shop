package subscription

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
)

// appleRootCAG3PEM は Apple Root CA - G3（ECDSA P-384）。
// https://www.apple.com/certificateauthority/ からダウンロー���。
//
//go:embed apple_root_ca_g3.pem
var appleRootCAG3PEM string

// jwsHeader は JWS の JOSE ヘッダを表す。
type jwsHeader struct {
	Alg string   `json:"alg"`
	X5C []string `json:"x5c"`
}

var appleRootPool *x509.CertPool

func init() {
	block, _ := pem.Decode([]byte(appleRootCAG3PEM))
	if block == nil {
		panic("Apple Root CA - G3: failed to decode PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic("Apple Root CA - G3: " + err.Error())
	}
	appleRootPool = x509.NewCertPool()
	appleRootPool.AddCert(cert)
}

// verifyAppleJWS は Apple JWS token の x5c 証明書チェーンと ECDSA 署名を検証し、
// 生の payload バイト列を返す。
func verifyAppleJWS(jws string) ([]byte, error) {
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
		Roots:         appleRootPool,
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

// jwsVerifyFunc は JWS payload の検証・抽出に使用する関数。
// テストでは Apple 証明書検証をバイパスするためにオーバーライドされる。
var jwsVerifyFunc = verifyAppleJWS

// decodeVerifiedJWSPayload は Apple JWS token の x5c 証明書チェーンと ECDSA 署名を
// 検証し、payload を T に unmarshal する。
func decodeVerifiedJWSPayload[T any](jws string) (*T, error) {
	payload, err := jwsVerifyFunc(jws)
	if err != nil {
		return nil, err
	}

	var v T
	if err := json.Unmarshal(payload, &v); err != nil {
		return nil, fmt.Errorf("unmarshal JWS payload: %w", err)
	}
	return &v, nil
}
