package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// GoogleRTDNMessage は Google Play RTDN（Real-Time Developer Notifications）のメッセージ構造。
type GoogleRTDNMessage struct {
	Message struct {
		Data string `json:"data"`
	} `json:"message"`
}

type googleRTDNData struct {
	SubscriptionNotification *struct {
		NotificationType int    `json:"notificationType"`
		PurchaseToken    string `json:"purchaseToken"`
		SubscriptionID   string `json:"subscriptionId"`
	} `json:"subscriptionNotification"`
}

// Google RTDN 通知種別
const (
	googleSubscriptionRecovered = 1
	googleSubscriptionRenewed   = 2
	googleSubscriptionCanceled  = 3
	googleSubscriptionPurchased = 4
	googleSubscriptionRevoked   = 12
	googleSubscriptionExpired   = 13
)

// GoogleNotifier は Google Play RTDN webhook を処理する。
type GoogleNotifier struct {
	subscriptionRepo port.SubscriptionRepo
	subVerifier      port.GoogleSubVerifier
}

// NewGoogleNotifier は GoogleNotifier を構築する。
func NewGoogleNotifier(
	subscriptionRepo port.SubscriptionRepo,
	subVerifier port.GoogleSubVerifier,
) *GoogleNotifier {
	return &GoogleNotifier{
		subscriptionRepo: subscriptionRepo,
		subVerifier:      subVerifier,
	}
}

// HandleNotification は Google Play RTDN 通知を処理する。
func (n *GoogleNotifier) HandleNotification(ctx context.Context, msg GoogleRTDNMessage) error {
	data, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDecodeRTDNData, err)
	}

	var rtdn googleRTDNData
	if err := json.Unmarshal(data, &rtdn); err != nil {
		return fmt.Errorf("%w: %w", ErrUnmarshalRTDNData, err)
	}

	if rtdn.SubscriptionNotification == nil {
		slog.Info("google RTDN skipped: no subscriptionNotification")
		return nil
	}

	notif := rtdn.SubscriptionNotification
	subscription, err := n.subscriptionRepo.FindSubscriptionByToken(ctx, domain.PlatformAndroid, notif.PurchaseToken)
	if err != nil {
		return fmt.Errorf("find subscription: %w", err)
	}
	if subscription == nil {
		return fmt.Errorf("%w: token=%s", ErrSubscriptionNotFound, notif.PurchaseToken)
	}

	switch notif.NotificationType {
	case googleSubscriptionRecovered,
		googleSubscriptionRenewed,
		googleSubscriptionCanceled,
		googleSubscriptionPurchased,
		googleSubscriptionRevoked,
		googleSubscriptionExpired:
		return n.updateSubscriptionFromGoogle(ctx, subscription, notif.NotificationType, notif.PurchaseToken)
	default:
		slog.Warn("google RTDN unhandled notification type",
			"notification_type", notif.NotificationType, "purchase_token", notif.PurchaseToken)
		return nil
	}
}

// updateSubscriptionFromGoogle は Google Play が返す購読情報に合わせて契約の状態と有効期限を更新する。
func (n *GoogleNotifier) updateSubscriptionFromGoogle(ctx context.Context, subscription *domain.Subscription, notificationType int, purchaseToken string) error {
	// 契約状態は通知種別ではなく Google が返す購読の状態から決めるため、どの通知種別でも購読情報を問い合わせる。
	googleSubscription, err := n.subVerifier.GetSubscription(ctx, purchaseToken)
	if err != nil {
		return fmt.Errorf("get subscription expiry from Google: %w", err)
	}
	if googleSubscription.ProductID != subscription.ProductID {
		return fmt.Errorf("google subscription product mismatch: subscription=%q google=%q",
			subscription.ProductID, googleSubscription.ProductID)
	}

	transition, err := resolveGoogleTransition(googleSubscription.State, notificationType)
	if err != nil {
		return err
	}
	if googleSubscription.ExpiresAt == nil {
		return errors.New("google subscription expiry is missing")
	}

	subscription.Status = transition.status
	subscription.CurrentPeriodEnd = *googleSubscription.ExpiresAt
	if !transition.shouldSendEvent {
		return writeNoEvent(ctx, n.subscriptionRepo, subscription)
	}
	var premiumExpiresAt *time.Time
	if transition.isPremium {
		premiumExpiresAt = googleSubscription.ExpiresAt
	}
	return writeWithEvent(ctx, n.subscriptionRepo, subscription, transition.isPremium, premiumExpiresAt)
}

type googleTransition struct {
	status          string
	shouldSendEvent bool
	isPremium       bool
}

// resolveGoogleTransition は Google が返す購読の状態に対応する契約状態とプレミアム更新イベントの扱いを返す。
func resolveGoogleTransition(state port.GoogleSubscriptionState, notificationType int) (googleTransition, error) {
	switch state {
	case port.GoogleSubscriptionStateActive:
		return googleTransition{status: domain.SubscriptionStatusActive, shouldSendEvent: true, isPremium: true}, nil
	case port.GoogleSubscriptionStateCanceled:
		// 解約後も有効期限まで特典が有効なため、プレミアム更新イベントを送らない。
		return googleTransition{status: domain.SubscriptionStatusCancelled}, nil
	case port.GoogleSubscriptionStateInGracePeriod:
		return googleTransition{status: domain.SubscriptionStatusGracePeriod, shouldSendEvent: true, isPremium: true}, nil
	case port.GoogleSubscriptionStateOnHold, port.GoogleSubscriptionStatePaused:
		return googleTransition{status: domain.SubscriptionStatusExpired, shouldSendEvent: true}, nil
	case port.GoogleSubscriptionStateExpired:
		// 購読の状態だけでは期限切れと取り消しを区別できないため、通知種別が REVOKED のときは revoked にする。
		if notificationType == googleSubscriptionRevoked {
			return googleTransition{status: domain.SubscriptionStatusRevoked, shouldSendEvent: true}, nil
		}
		return googleTransition{status: domain.SubscriptionStatusExpired, shouldSendEvent: true}, nil
	default:
		return googleTransition{}, fmt.Errorf("google subscription state %q is not supported", state)
	}
}
