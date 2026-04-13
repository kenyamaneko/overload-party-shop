package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
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

// HandleGoogleNotification は Google Play RTDN 通知を処理する。
func (s *SubscriptionService) HandleGoogleNotification(ctx context.Context, msg GoogleRTDNMessage) error {
	data, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		return fmt.Errorf("decode RTDN data: %w", err)
	}

	var rtdn googleRTDNData
	if err := json.Unmarshal(data, &rtdn); err != nil {
		return fmt.Errorf("unmarshal RTDN data: %w", err)
	}

	if rtdn.SubscriptionNotification == nil {
		return nil
	}

	notif := rtdn.SubscriptionNotification
	sub, err := s.subRepo.FindSubscriptionByToken(ctx, apishop.PlatformAndroid, notif.PurchaseToken)
	if err != nil {
		return fmt.Errorf("find subscription: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("subscription not found for token: %s", notif.PurchaseToken)
	}

	switch notif.NotificationType {
	case googleSubRenewed, googleSubRecovered:
		if s.googleVerifier == nil {
			return fmt.Errorf("google subscription verifier not configured")
		}
		newExpiry, err := s.googleVerifier.GetSubscriptionExpiry(ctx, notif.PurchaseToken)
		if err != nil {
			return fmt.Errorf("get subscription expiry from Google: %w", err)
		}
		sub.Status = apishop.SubscriptionStatusActive
		sub.CurrentPeriodEnd = newExpiry
		sub.UpdatedAt = time.Now()
		if err := s.applySubChangeAndPublish(ctx, sub, true, &newExpiry); err != nil {
			return err
		}

	case googleSubExpired:
		sub.Status = apishop.SubscriptionStatusExpired
		sub.UpdatedAt = time.Now()
		if err := s.applySubChangeAndPublish(ctx, sub, false, nil); err != nil {
			return err
		}

	case googleSubRevoked:
		sub.Status = apishop.SubscriptionStatusRevoked
		sub.UpdatedAt = time.Now()
		if err := s.applySubChangeAndPublish(ctx, sub, false, nil); err != nil {
			return err
		}

	case googleSubCanceled:
		sub.Status = apishop.SubscriptionStatusCancelled
		sub.UpdatedAt = time.Now()
		if err := s.subRepo.UpdateSubscription(ctx, sub); err != nil {
			return fmt.Errorf("update subscription: %w", err)
		}
		// プレミアムは current_period_end まで有効 — premium-updated イベントは発行しない。
	}

	return nil
}
