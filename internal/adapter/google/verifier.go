package google

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const purchaseStatePurchased = 0

// Verifier は Google Play Developer API を使用して port.ReceiptVerifier を実装する。
type Verifier struct {
	service     *androidpublisher.Service
	packageName string
}

// NewVerifier は ADC で認証する Google Play レシート verifier を構築する。
func NewVerifier(ctx context.Context, packageName string, opts ...option.ClientOption) (*Verifier, error) {
	svc, err := androidpublisher.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create androidpublisher service: %w", err)
	}

	return &Verifier{
		service:     svc,
		packageName: packageName,
	}, nil
}

var _ port.ReceiptVerifier = (*Verifier)(nil)

// VerifyPurchase は Google Play Developer API で単発購入を検証する。
func (v *Verifier) VerifyPurchase(ctx context.Context, purchaseToken string) (*port.VerifyResult, error) {
	productID, token, err := splitGoogleToken(purchaseToken)
	if err != nil {
		return nil, err
	}

	result, err := v.service.Purchases.Products.Get(v.packageName, productID, token).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("google Play API: %w", err)
	}

	if result.PurchaseState != purchaseStatePurchased {
		return &port.VerifyResult{IsValid: false}, nil
	}

	return &port.VerifyResult{
		IsValid:   true,
		ProductID: productID,
	}, nil
}

// VerifySubscription は Google Play Developer API でサブスクリプションを検証する。
func (v *Verifier) VerifySubscription(ctx context.Context, purchaseToken string) (*port.SubscriptionInfo, error) {
	subscriptionID, expiresAt, err := fetchSubscriptionExpiry(ctx, v.service, v.packageName, purchaseToken)
	if err != nil {
		return nil, err
	}

	return &port.SubscriptionInfo{
		IsValid:   true,
		ProductID: subscriptionID,
		ExpiresAt: expiresAt,
	}, nil
}

// splitGoogleToken は "{productId}:{token}" 形式の購入トークンを商品 ID とトークンに分ける。
func splitGoogleToken(composite string) (string, string, error) {
	productID, token, ok := strings.Cut(composite, ":")
	if !ok {
		return "", "", fmt.Errorf("invalid Google purchase token format: expected 'productId:token'")
	}
	if productID == "" || token == "" {
		return "", "", fmt.Errorf("invalid Google purchase token format: productId and token must not be empty")
	}
	return productID, token, nil
}

func fetchSubscriptionExpiry(ctx context.Context, service *androidpublisher.Service, packageName, purchaseToken string) (string, time.Time, error) {
	subscriptionID, token, err := splitGoogleToken(purchaseToken)
	if err != nil {
		return "", time.Time{}, err
	}

	result, err := service.Purchases.Subscriptions.Get(packageName, subscriptionID, token).Context(ctx).Do()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("google Play API get subscription: %w", err)
	}
	if result.ExpiryTimeMillis == 0 {
		return "", time.Time{}, fmt.Errorf("google Play API get subscription: expiryTimeMillis is missing")
	}

	return subscriptionID, time.UnixMilli(result.ExpiryTimeMillis), nil
}

// SubVerifier は Google Play Developer API からサブスクリプション有効期限を取得する。
type SubVerifier struct {
	service     *androidpublisher.Service
	packageName string
}

// NewSubVerifier は ADC で認証する SubVerifier を構築する。
func NewSubVerifier(ctx context.Context, packageName string, opts ...option.ClientOption) (*SubVerifier, error) {
	svc, err := androidpublisher.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create androidpublisher service: %w", err)
	}
	return &SubVerifier{
		service:     svc,
		packageName: packageName,
	}, nil
}

var _ port.GoogleSubVerifier = (*SubVerifier)(nil)

// GetSubscriptionExpiry は Google Play からサブスクリプションの有効期限を取得する。
func (v *SubVerifier) GetSubscriptionExpiry(ctx context.Context, purchaseToken string) (time.Time, error) {
	_, expiresAt, err := fetchSubscriptionExpiry(ctx, v.service, v.packageName, purchaseToken)
	if err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}
