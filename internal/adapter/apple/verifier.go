package apple

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const (
	appleProductionURL = "https://api.storekit.itunes.apple.com"
	appleSandboxURL    = "https://api.storekit-sandbox.itunes.apple.com"
	appleAPITimeout    = 10 * time.Second
)

// Verifier は App Store Server API v2 を使用して port.ReceiptVerifier を実装する。
type Verifier struct {
	keyID      string
	issuerID   string
	bundleID   string
	privateKey *ecdsa.PrivateKey
	baseURL    string
	httpClient *http.Client
}

// NewVerifier はファイルシステムから PEM 鍵を読み込んで Apple レシート
// verifier を構築する。PEM データがメモリ上にある場合（Secret Manager 経由等）は
// NewVerifierFromPEM を使用する。
func NewVerifier(keyID, issuerID, bundleID, privateKeyPath, environment string) (*Verifier, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read apple private key: %w", err)
	}
	return NewVerifierFromPEM(keyID, issuerID, bundleID, keyData, environment)
}

// NewVerifierFromPEM はメモリ上の PEM エンコード P-256 秘密鍵バイト列
// から Apple レシート verifier を構築する。
// environment は "Production" または "Sandbox"。
func NewVerifierFromPEM(keyID, issuerID, bundleID string, pemData []byte, environment string) (*Verifier, error) {
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

	baseURL := appleProductionURL
	if environment == "Sandbox" {
		baseURL = appleSandboxURL
	}

	return &Verifier{
		keyID:      keyID,
		issuerID:   issuerID,
		bundleID:   bundleID,
		privateKey: ecKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: appleAPITimeout},
	}, nil
}

var _ port.ReceiptVerifier = (*Verifier)(nil)

// VerifyPurchase は Apple App Store Server API v2 で単発購入トランザクションを検証する。
func (v *Verifier) VerifyPurchase(ctx context.Context, purchaseToken string) (*port.VerifyResult, error) {
	token, err := v.generateJWT()
	if err != nil {
		return nil, fmt.Errorf("generate JWT: %w", err)
	}

	url := fmt.Sprintf("%s/inApps/v2/transactions/%s", v.baseURL, purchaseToken)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to Apple: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return &port.VerifyResult{IsValid: false}, fmt.Errorf("apple API returned %d (body read failed: %v)", resp.StatusCode, err)
		}
		return &port.VerifyResult{IsValid: false}, fmt.Errorf("apple API returned %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		SignedTransactionInfo string `json:"signedTransactionInfo"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode Apple response: %w", err)
	}

	txnInfo, err := decodeJWSPayload(apiResp.SignedTransactionInfo)
	if err != nil {
		return nil, fmt.Errorf("decode transaction info: %w", err)
	}

	return &port.VerifyResult{
		IsValid:       true,
		TransactionID: txnInfo.TransactionID,
		ProductID:     txnInfo.ProductID,
		PurchaseTime:  time.UnixMilli(txnInfo.PurchaseDate),
	}, nil
}

// VerifySubscription は Apple App Store Server API v2 でサブスクリプションを検証する。
func (v *Verifier) VerifySubscription(ctx context.Context, purchaseToken string) (*port.SubscriptionInfo, error) {
	token, err := v.generateJWT()
	if err != nil {
		return nil, fmt.Errorf("generate JWT: %w", err)
	}

	url := fmt.Sprintf("%s/inApps/v2/subscriptions/%s", v.baseURL, purchaseToken)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to Apple: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return &port.SubscriptionInfo{IsValid: false}, fmt.Errorf("apple API returned %d (body read failed: %v)", resp.StatusCode, err)
		}
		return &port.SubscriptionInfo{IsValid: false}, fmt.Errorf("apple API returned %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Data []struct {
			LastTransactions []struct {
				SignedTransactionInfo string `json:"signedTransactionInfo"`
				SignedRenewalInfo     string `json:"signedRenewalInfo"`
			} `json:"lastTransactions"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode Apple subscription response: %w", err)
	}

	if len(apiResp.Data) == 0 || len(apiResp.Data[0].LastTransactions) == 0 {
		return &port.SubscriptionInfo{IsValid: false}, nil
	}

	lastTxn := apiResp.Data[0].LastTransactions[0]

	txnInfo, err := decodeJWSPayload(lastTxn.SignedTransactionInfo)
	if err != nil {
		return nil, fmt.Errorf("decode transaction info: %w", err)
	}

	renewalInfo, err := decodeJWSRenewalPayload(lastTxn.SignedRenewalInfo)
	if err != nil {
		return nil, fmt.Errorf("decode renewal info: %w", err)
	}

	return &port.SubscriptionInfo{
		IsValid:        true,
		ProductID:      txnInfo.ProductID,
		TransactionID:  txnInfo.TransactionID,
		ExpiresAt:      time.UnixMilli(txnInfo.ExpiresDate),
		IsAutoRenewing: renewalInfo.AutoRenewStatus == 1,
	}, nil
}

func (v *Verifier) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    v.issuerID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(20 * time.Minute)),
		Audience:  jwt.ClaimStrings{"appstoreconnect-v1"},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = v.keyID

	return token.SignedString(v.privateKey)
}

// appleTransactionInfo は Apple JWS payload のデコード結果。
type appleTransactionInfo struct {
	TransactionID string `json:"transactionId"`
	ProductID     string `json:"productId"`
	PurchaseDate  int64  `json:"purchaseDate"`
	ExpiresDate   int64  `json:"expiresDate"`
	BundleID      string `json:"bundleId"`
}

type appleRenewalInfo struct {
	AutoRenewStatus int `json:"autoRenewStatus"`
}

// decodeJWSPayload は JWS から署名検証なしで payload を抽出する。
// TODO: 本番前に Apple root cert に対する署名検証を追加する。
// 現在は App Store Server API への認証済み HTTPS 呼び出し（JWT bearer auth）に
// 信頼を委譲している。App Store Server Notifications V2 webhook パスでは
// x5c + ECDSA 検証を行う（internal/service/jws_verify.go 参照）。
func decodeJWSPayload(jws string) (*appleTransactionInfo, error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWS format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWS payload: %w", err)
	}

	var info appleTransactionInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return nil, fmt.Errorf("unmarshal transaction info: %w", err)
	}
	return &info, nil
}

func decodeJWSRenewalPayload(jws string) (*appleRenewalInfo, error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWS format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWS payload: %w", err)
	}

	var info appleRenewalInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return nil, fmt.Errorf("unmarshal renewal info: %w", err)
	}
	return &info, nil
}
