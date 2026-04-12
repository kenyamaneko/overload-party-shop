package platform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"
)

// GoogleReceiptVerifier は Google Play Developer API を使用して ReceiptVerifier を実装する。
type GoogleReceiptVerifier struct {
	service     *androidpublisher.Service
	packageName string
}

// NewGoogleReceiptVerifier は Google Play レシート verifier を構築する。
// ADC（Application Default Credentials）で認証する。
func NewGoogleReceiptVerifier(ctx context.Context, packageName string, opts ...option.ClientOption) (*GoogleReceiptVerifier, error) {
	svc, err := androidpublisher.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create androidpublisher service: %w", err)
	}

	return &GoogleReceiptVerifier{
		service:     svc,
		packageName: packageName,
	}, nil
}

var _ ReceiptVerifier = (*GoogleReceiptVerifier)(nil)

// VerifyPurchase は Google Play Developer API で単発購入を検証する。
func (v *GoogleReceiptVerifier) VerifyPurchase(ctx context.Context, purchaseToken string) (*VerifyResult, error) {
	// purchaseToken は "{productId}:{token}" 形式
	productID, token, err := splitGoogleToken(purchaseToken)
	if err != nil {
		return &VerifyResult{IsValid: false}, err
	}

	result, err := v.service.Purchases.Products.Get(v.packageName, productID, token).Context(ctx).Do()
	if err != nil {
		return &VerifyResult{IsValid: false}, fmt.Errorf("Google Play API: %w", err)
	}

	// PurchaseState: 0=購入済み, 1=キャンセル, 2=保留中
	if result.PurchaseState != 0 {
		return &VerifyResult{IsValid: false}, nil
	}

	purchaseTime := time.UnixMilli(result.PurchaseTimeMillis)

	return &VerifyResult{
		IsValid:       true,
		TransactionID: result.OrderId,
		ProductID:     productID,
		PurchaseTime:  purchaseTime,
	}, nil
}

// VerifySubscription は Google Play Developer API でサブスクリプションを検証する。
func (v *GoogleReceiptVerifier) VerifySubscription(ctx context.Context, purchaseToken string) (*SubscriptionInfo, error) {
	// purchaseToken は "{subscriptionId}:{token}" 形式
	subscriptionID, token, err := splitGoogleToken(purchaseToken)
	if err != nil {
		return &SubscriptionInfo{IsValid: false}, err
	}

	result, err := v.service.Purchases.Subscriptions.Get(v.packageName, subscriptionID, token).Context(ctx).Do()
	if err != nil {
		return &SubscriptionInfo{IsValid: false}, fmt.Errorf("Google Play API: %w", err)
	}

	expiresAt := time.UnixMilli(result.ExpiryTimeMillis)

	return &SubscriptionInfo{
		IsValid:        true,
		ProductID:      subscriptionID,
		TransactionID:  result.OrderId,
		ExpiresAt:      expiresAt,
		IsAutoRenewing: result.AutoRenewing,
	}, nil
}

func splitGoogleToken(composite string) (string, string, error) {
	productID, token, ok := strings.Cut(composite, ":")
	if !ok {
		return "", "", fmt.Errorf("invalid Google purchase token format: expected 'productId:token'")
	}
	return productID, token, nil
}

// GooglePlaySubVerifier は Google Play Developer API からサブスクリプションの
// 実際の有効期限を取得する。
type GooglePlaySubVerifier struct {
	service     *androidpublisher.Service
	packageName string
}

// NewGooglePlaySubVerifier は Google Play にサブスクリプション有効期限を問い合わせる
// verifier を構築する。ADC で認証する。
func NewGooglePlaySubVerifier(ctx context.Context, packageName string, opts ...option.ClientOption) (*GooglePlaySubVerifier, error) {
	svc, err := androidpublisher.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create androidpublisher service: %w", err)
	}
	return &GooglePlaySubVerifier{
		service:     svc,
		packageName: packageName,
	}, nil
}

// GetSubscriptionExpiry は Google Play からサブスクリプションの有効期限を取得する。
func (v *GooglePlaySubVerifier) GetSubscriptionExpiry(ctx context.Context, purchaseToken string) (time.Time, error) {
	subscriptionID, token, err := splitGoogleToken(purchaseToken)
	if err != nil {
		return time.Time{}, err
	}

	result, err := v.service.Purchases.Subscriptions.Get(v.packageName, subscriptionID, token).Context(ctx).Do()
	if err != nil {
		return time.Time{}, fmt.Errorf("Google Play API get subscription: %w", err)
	}

	return time.UnixMilli(result.ExpiryTimeMillis), nil
}
