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
	// purchaseToken は "{productId}:{token}" 形式
	productID, token, err := splitGoogleToken(purchaseToken)
	if err != nil {
		return &port.VerifyResult{IsValid: false}, err
	}

	result, err := v.service.Purchases.Products.Get(v.packageName, productID, token).Context(ctx).Do()
	if err != nil {
		return &port.VerifyResult{IsValid: false}, fmt.Errorf("google Play API: %w", err)
	}

	// PurchaseState: 0=購入済み, 1=キャンセル, 2=保留中
	if result.PurchaseState != 0 {
		return &port.VerifyResult{IsValid: false}, nil
	}

	purchaseTime := time.UnixMilli(result.PurchaseTimeMillis)

	return &port.VerifyResult{
		IsValid:       true,
		TransactionID: result.OrderId,
		ProductID:     productID,
		PurchaseTime:  purchaseTime,
	}, nil
}

// VerifySubscription は Google Play Developer API でサブスクリプションを検証する。
func (v *Verifier) VerifySubscription(ctx context.Context, purchaseToken string) (*port.SubscriptionInfo, error) {
	// purchaseToken は "{subscriptionId}:{token}" 形式
	subscriptionID, token, err := splitGoogleToken(purchaseToken)
	if err != nil {
		return &port.SubscriptionInfo{IsValid: false}, err
	}

	result, err := v.service.Purchases.Subscriptions.Get(v.packageName, subscriptionID, token).Context(ctx).Do()
	if err != nil {
		return &port.SubscriptionInfo{IsValid: false}, fmt.Errorf("google Play API: %w", err)
	}

	expiresAt := time.UnixMilli(result.ExpiryTimeMillis)

	return &port.SubscriptionInfo{
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

// GetSubscriptionExpiry は Google Play からサブスクリプションの有効期限を取得する。
func (v *SubVerifier) GetSubscriptionExpiry(ctx context.Context, purchaseToken string) (time.Time, error) {
	subscriptionID, token, err := splitGoogleToken(purchaseToken)
	if err != nil {
		return time.Time{}, err
	}

	result, err := v.service.Purchases.Subscriptions.Get(v.packageName, subscriptionID, token).Context(ctx).Do()
	if err != nil {
		return time.Time{}, fmt.Errorf("google Play API get subscription: %w", err)
	}

	return time.UnixMilli(result.ExpiryTimeMillis), nil
}
