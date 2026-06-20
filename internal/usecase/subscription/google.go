package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	googleSubscriptionRecovered = 2
	googleSubscriptionCanceled  = 3
	googleSubscriptionRenewed   = 4
	googleSubscriptionExpired   = 12
	googleSubscriptionRevoked   = 13
)

// GoogleNotifier は Google Play RTDN webhook を処理する。
// RTDN payload には expiry が含まれないため expiryFetcher (Play Developer API) で取得する。
type GoogleNotifier struct {
	subscriptionRepo port.SubscriptionRepo
	expiryFetcher    port.GoogleSubVerifier
}

func NewGoogleNotifier(
	subscriptionRepo port.SubscriptionRepo,
	expiryFetcher port.GoogleSubVerifier,
) *GoogleNotifier {
	return &GoogleNotifier{
		subscriptionRepo: subscriptionRepo,
		expiryFetcher:    expiryFetcher,
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
	case googleSubscriptionRenewed, googleSubscriptionRecovered:
		if n.expiryFetcher == nil {
			return fmt.Errorf("google subscription verifier not configured")
		}
		newExpiry, err := n.expiryFetcher.GetSubscriptionExpiry(ctx, notif.PurchaseToken)
		if err != nil {
			return fmt.Errorf("get subscription expiry from Google: %w", err)
		}
		subscription.Status = domain.SubscriptionStatusActive
		subscription.CurrentPeriodEnd = newExpiry
		subscription.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subscriptionRepo, subscription, true, &newExpiry); err != nil {
			return err
		}

	case googleSubscriptionExpired:
		subscription.Status = domain.SubscriptionStatusExpired
		subscription.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subscriptionRepo, subscription, false, nil); err != nil {
			return err
		}

	case googleSubscriptionRevoked:
		subscription.Status = domain.SubscriptionStatusRevoked
		subscription.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subscriptionRepo, subscription, false, nil); err != nil {
			return err
		}

	case googleSubscriptionCanceled:
		subscription.Status = domain.SubscriptionStatusCancelled
		subscription.UpdatedAt = time.Now()
		// プレミアムは current_period_end まで有効 — premium-updated イベントは発行しない
		// (エンタイトルメント維持契約: docs/ARCHITECTURE.md)。
		if err := writeNoEvent(ctx, n.subscriptionRepo, subscription); err != nil {
			return err
		}

	default:
		slog.Warn("google RTDN unhandled notification type",
			"notification_type", notif.NotificationType, "purchase_token", notif.PurchaseToken)
	}

	return nil
}
