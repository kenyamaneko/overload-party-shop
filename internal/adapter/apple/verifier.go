package apple

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// Apple の環境名。設定値と Verifier の接続先の選択で共有する。
const (
	EnvironmentProduction = "Production"
	EnvironmentSandbox    = "Sandbox"
)

const (
	appleProductionURL = "https://api.storekit.itunes.apple.com"
	appleSandboxURL    = "https://api.storekit-sandbox.itunes.apple.com"
	appleAPITimeout    = 10 * time.Second

	appleTransactionPath  = "/inApps/v2/transactions/"
	appleSubscriptionPath = "/inApps/v1/subscriptions/"

	appleJWTAudience = "appstoreconnect-v1"
	appleJWTLifetime = 20 * time.Minute
)

// Verifier は App Store Server API を使用して port.ReceiptVerifier を実装する。
type Verifier struct {
	keyID       string
	issuerID    string
	bundleID    string
	privateKey  *ecdsa.PrivateKey
	baseURL     string
	httpClient  *http.Client
	jwsVerifier port.AppleJWSVerifier
}

// VerifierOption は Verifier の構築時の設定を差し替える。
type VerifierOption func(*Verifier)

// WithEndpoint は App Store Server API の接続先 URL を環境名から決まる既定の接続先から差し替える。
func WithEndpoint(baseURL string) VerifierOption {
	return func(v *Verifier) {
		v.baseURL = baseURL
	}
}

// NewVerifierFromPEM はメモリ上の P-256 秘密鍵から Apple verifier を構築する。environment は EnvironmentProduction / EnvironmentSandbox。
func NewVerifierFromPEM(keyID, issuerID, bundleID string, pemData []byte, environment string, jwsVerifier port.AppleJWSVerifier, opts ...VerifierOption) (*Verifier, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ECDSA")
	}
	if ecKey.Curve != elliptic.P256() {
		return nil, fmt.Errorf("private key curve is %s, expected P-256", ecKey.Curve.Params().Name)
	}

	var baseURL string
	switch environment {
	case EnvironmentProduction:
		baseURL = appleProductionURL
	case EnvironmentSandbox:
		baseURL = appleSandboxURL
	default:
		return nil, fmt.Errorf("unknown apple environment %q: expected %q or %q", environment, EnvironmentProduction, EnvironmentSandbox)
	}

	v := &Verifier{
		keyID:       keyID,
		issuerID:    issuerID,
		bundleID:    bundleID,
		privateKey:  ecKey,
		baseURL:     baseURL,
		httpClient:  &http.Client{Timeout: appleAPITimeout},
		jwsVerifier: jwsVerifier,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v, nil
}

var _ port.ReceiptVerifier = (*Verifier)(nil)

// VerifyPurchase は App Store Server API で単発購入トランザクションを検証する。
func (v *Verifier) VerifyPurchase(ctx context.Context, purchaseToken string) (*port.VerifyResult, error) {
	var apiResp struct {
		SignedTransactionInfo string `json:"signedTransactionInfo"`
	}
	if err := v.getJSON(ctx, appleTransactionPath, purchaseToken, "Apple response", &apiResp); err != nil {
		return nil, err
	}

	txnInfo, err := v.decodeTransactionInfo(apiResp.SignedTransactionInfo)
	if err != nil {
		return nil, fmt.Errorf("decode transaction info: %w", err)
	}
	if txnInfo.BundleID != v.bundleID {
		return &port.VerifyResult{IsValid: false}, nil
	}

	return &port.VerifyResult{
		IsValid:   true,
		ProductID: txnInfo.ProductID,
	}, nil
}

// VerifySubscription は App Store Server API でサブスクリプションを検証する。
func (v *Verifier) VerifySubscription(ctx context.Context, purchaseToken string) (*port.SubscriptionInfo, error) {
	var apiResp struct {
		Data []struct {
			LastTransactions []struct {
				SignedTransactionInfo string `json:"signedTransactionInfo"`
			} `json:"lastTransactions"`
		} `json:"data"`
	}
	if err := v.getJSON(ctx, appleSubscriptionPath, purchaseToken, "Apple subscription response", &apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Data) == 0 || len(apiResp.Data[0].LastTransactions) == 0 {
		return &port.SubscriptionInfo{IsValid: false}, nil
	}

	txnInfo, err := v.decodeTransactionInfo(apiResp.Data[0].LastTransactions[0].SignedTransactionInfo)
	if err != nil {
		return nil, fmt.Errorf("decode transaction info: %w", err)
	}
	if txnInfo.BundleID != v.bundleID {
		return &port.SubscriptionInfo{IsValid: false}, nil
	}
	if txnInfo.ExpiresDate == nil || *txnInfo.ExpiresDate == 0 {
		return nil, fmt.Errorf("decode transaction info: expiresDate is missing")
	}

	return &port.SubscriptionInfo{
		IsValid:   true,
		ProductID: txnInfo.ProductID,
		ExpiresAt: time.UnixMilli(*txnInfo.ExpiresDate),
	}, nil
}

// getJSON は認証付きで basePath に transactionID を連結した URL へ GET し、200 の応答本文を out に復号する。responseName は復号失敗のエラーに含める応答の呼び名。
func (v *Verifier) getJSON(ctx context.Context, basePath, transactionID, responseName string, out any) error {
	token, err := v.generateJWT()
	if err != nil {
		return fmt.Errorf("generate JWT: %w", err)
	}

	requestURL := v.baseURL + basePath + url.PathEscape(transactionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request to Apple: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("apple API returned %d (body read failed: %v)", resp.StatusCode, err)
		}
		return fmt.Errorf("apple API returned %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", responseName, err)
	}
	return nil
}

type appleAPIClaims struct {
	jwt.RegisteredClaims
	// Apple は aud を文字列で定めるため、RegisteredClaims の配列の aud を同名の文字列フィールドで隠す。
	Audience string `json:"aud"`
	BundleID string `json:"bid"`
}

func (v *Verifier) generateJWT() (string, error) {
	now := time.Now()
	claims := appleAPIClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    v.issuerID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(appleJWTLifetime)),
		},
		Audience: appleJWTAudience,
		BundleID: v.bundleID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = v.keyID

	return token.SignedString(v.privateKey)
}

type appleTransactionInfo struct {
	ProductID   string `json:"productId"`
	ExpiresDate *int64 `json:"expiresDate"`
	BundleID    string `json:"bundleId"`
}

// decodeTransactionInfo は Apple JWS を検証してから transaction payload を抽出する。
func (v *Verifier) decodeTransactionInfo(jws string) (*appleTransactionInfo, error) {
	payload, err := v.jwsVerifier.Verify(jws)
	if err != nil {
		return nil, fmt.Errorf("verify JWS: %w", err)
	}

	var info appleTransactionInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return nil, fmt.Errorf("unmarshal transaction info: %w", err)
	}
	return &info, nil
}
