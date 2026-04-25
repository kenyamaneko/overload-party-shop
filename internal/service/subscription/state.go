package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// IsEntitled はサブスクリプションが指定時刻に特典有効かを返す。
// active / cancelled (auto-renew off だが期間内) / grace_period (支払い失敗中だが猶予内)
// が期間内である限り特典は有効。expired / revoked は無効。
// 未知の status は上流の不整合なのでエラーで返す。
func IsEntitled(sub *apishop.Subscription, now time.Time) (bool, error) {
	if sub == nil {
		return false, nil
	}
	switch sub.Status {
	case apishop.SubscriptionStatusActive,
		apishop.SubscriptionStatusCancelled,
		apishop.SubscriptionStatusGracePeriod:
		return sub.CurrentPeriodEnd.After(now), nil
	case apishop.SubscriptionStatusExpired,
		apishop.SubscriptionStatusRevoked:
		return false, nil
	default:
		return false, fmt.Errorf("subscription: unknown status %q", sub.Status)
	}
}

// writeWithEvent / writeNoEvent は dual-write 問題を避けるため subscription 行
// 更新と premium-updated 発行 (またはその不発行) を同一 tx に揃える共通経路。
// AppleNotifier / GoogleNotifier 共通。
func writeWithEvent(
	ctx context.Context,
	subRepo port.SubscriptionRepo,
	sub *apishop.Subscription,
	isPremium bool,
	expiresAt *time.Time,
) error {
	ev, err := buildPremiumUpdatedEvent(sub.PlayerID, isPremium, expiresAt)
	if err != nil {
		return fmt.Errorf("build premium-updated: %w", err)
	}
	if err := subRepo.UpdateSubscriptionWithEvent(ctx, sub, ev); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}

func buildPremiumUpdatedEvent(playerID string, isPremium bool, expiresAt *time.Time) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("subscription: playerID is empty")
	}
	eventID := uuid.New()
	ev := apishop.PremiumUpdatedEvent{
		EventType:        apishop.EventTypePremiumUpdated,
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
		Source:           apishop.PremiumUpdatedSourceShop,
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal premium-updated: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: apishop.EventTypePremiumUpdated,
		Payload:   payload,
	}, nil
}

// writeNoEvent は cancelled 遷移用 (エンタイトルメント維持契約 — 期限到来までは
// is_premium=false を subscriber に伝えない)。
func writeNoEvent(ctx context.Context, subRepo port.SubscriptionRepo, sub *apishop.Subscription) error {
	if err := subRepo.UpdateSubscription(ctx, sub); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}
