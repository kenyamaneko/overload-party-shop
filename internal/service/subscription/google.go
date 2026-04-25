package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
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
	googleSubRecovered = 2
	googleSubCanceled  = 3
	googleSubRenewed   = 4
	googleSubExpired   = 12
	googleSubRevoked   = 13
)

// GoogleNotifier は Google Play RTDN webhook を処理する。
// RTDN payload には expiry が含まれないため expiryFetcher (Play Developer API) で取得する。
// Apple 側の依存は持たない。
type GoogleNotifier struct {
	subRepo       port.SubscriptionRepo
	expiryFetcher port.GoogleSubVerifier
}

// NewGoogleNotifier は依存を受け取り GoogleNotifier を構築する。
func NewGoogleNotifier(
	subRepo port.SubscriptionRepo,
	expiryFetcher port.GoogleSubVerifier,
) *GoogleNotifier {
	return &GoogleNotifier{
		subRepo:       subRepo,
		expiryFetcher: expiryFetcher,
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
	sub, err := n.subRepo.FindSubscriptionByToken(ctx, apishop.PlatformAndroid, notif.PurchaseToken)
	if err != nil {
		return fmt.Errorf("find subscription: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("%w: token=%s", ErrSubscriptionNotFound, notif.PurchaseToken)
	}

	switch notif.NotificationType {
	case googleSubRenewed, googleSubRecovered:
		if n.expiryFetcher == nil {
			return fmt.Errorf("google subscription verifier not configured")
		}
		newExpiry, err := n.expiryFetcher.GetSubscriptionExpiry(ctx, notif.PurchaseToken)
		if err != nil {
			return fmt.Errorf("get subscription expiry from Google: %w", err)
		}
		sub.Status = apishop.SubscriptionStatusActive
		sub.CurrentPeriodEnd = newExpiry
		sub.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subRepo, sub, true, &newExpiry); err != nil {
			return err
		}

	case googleSubExpired:
		sub.Status = apishop.SubscriptionStatusExpired
		sub.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subRepo, sub, false, nil); err != nil {
			return err
		}

	case googleSubRevoked:
		sub.Status = apishop.SubscriptionStatusRevoked
		sub.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subRepo, sub, false, nil); err != nil {
			return err
		}

	case googleSubCanceled:
		sub.Status = apishop.SubscriptionStatusCancelled
		sub.UpdatedAt = time.Now()
		// プレミアムは current_period_end まで有効 — premium-updated イベントは発行しない
		// (エンタイトルメント維持契約: docs/ARCHITECTURE.md)。
		if err := writeNoEvent(ctx, n.subRepo, sub); err != nil {
			return err
		}

	default:
		slog.Warn("google RTDN unhandled notification type",
			"notification_type", notif.NotificationType, "purchase_token", notif.PurchaseToken)
	}

	return nil
}
