package google

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const purchaseStatePurchased = 0

const googleSubscriptionStatePrefix = "SUBSCRIPTION_STATE_"

var googleSubscriptionStates = map[string]port.GoogleSubscriptionState{
	googleSubscriptionStatePrefix + "ACTIVE":                    port.GoogleSubscriptionStateActive,
	googleSubscriptionStatePrefix + "CANCELED":                  port.GoogleSubscriptionStateCanceled,
	googleSubscriptionStatePrefix + "IN_GRACE_PERIOD":           port.GoogleSubscriptionStateInGracePeriod,
	googleSubscriptionStatePrefix + "ON_HOLD":                   port.GoogleSubscriptionStateOnHold,
	googleSubscriptionStatePrefix + "PAUSED":                    port.GoogleSubscriptionStatePaused,
	googleSubscriptionStatePrefix + "EXPIRED":                   port.GoogleSubscriptionStateExpired,
	googleSubscriptionStatePrefix + "PENDING":                   port.GoogleSubscriptionStatePending,
	googleSubscriptionStatePrefix + "PENDING_PURCHASE_CANCELED": port.GoogleSubscriptionStatePendingPurchaseCanceled,
	googleSubscriptionStatePrefix + "UNSPECIFIED":               port.GoogleSubscriptionStateUnspecified,
}

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
func (v *Verifier) VerifyPurchase(ctx context.Context, productID, purchaseToken string) (*port.VerifyResult, error) {
	result, err := v.service.Purchases.Products.Get(v.packageName, productID, purchaseToken).Context(ctx).Do()
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
	subscription, err := fetchSubscription(ctx, v.service, v.packageName, purchaseToken)
	if err != nil {
		return nil, err
	}
	if subscription.ExpiresAt == nil {
		return nil, fmt.Errorf("google Play API get subscription: expiry is missing")
	}

	return &port.SubscriptionInfo{
		IsValid:   true,
		ProductID: subscription.ProductID,
		ExpiresAt: *subscription.ExpiresAt,
	}, nil
}

func fetchSubscription(ctx context.Context, service *androidpublisher.Service, packageName, purchaseToken string) (*port.GoogleSubscription, error) {
	result, err := service.Purchases.Subscriptionsv2.Get(packageName, purchaseToken).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("google Play API get subscription: %w", err)
	}
	if len(result.LineItems) != 1 {
		return nil, fmt.Errorf("google Play API get subscription: expected exactly one lineItem, got %d", len(result.LineItems))
	}
	lineItem := result.LineItems[0]

	var expiresAt *time.Time
	if lineItem.ExpiryTime != "" {
		parsed, err := time.Parse(time.RFC3339, lineItem.ExpiryTime)
		if err != nil {
			return nil, fmt.Errorf("google Play API get subscription: parse expiryTime: %w", err)
		}
		expiresAt = &parsed
	}

	state, ok := googleSubscriptionStates[result.SubscriptionState]
	if !ok {
		return nil, fmt.Errorf("google Play API get subscription: unknown subscriptionState %q", result.SubscriptionState)
	}

	return &port.GoogleSubscription{
		State:     state,
		ProductID: lineItem.ProductId,
		ExpiresAt: expiresAt,
	}, nil
}

// SubVerifier は Google Play Developer API から購読情報を取得する。
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

// GetSubscription は Google Play から購読情報を取得する。
func (v *SubVerifier) GetSubscription(ctx context.Context, purchaseToken string) (*port.GoogleSubscription, error) {
	return fetchSubscription(ctx, v.service, v.packageName, purchaseToken)
}
