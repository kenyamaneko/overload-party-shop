package subscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
)

// IsEntitled はサブスクリプションが指定時刻に特典有効かを返す。未知の status はエラー。
func IsEntitled(sub *domain.Subscription, now time.Time) (bool, error) {
	if sub == nil {
		return false, nil
	}
	switch sub.Status {
	case domain.SubscriptionStatusActive,
		domain.SubscriptionStatusCancelled,
		domain.SubscriptionStatusGracePeriod:
		return sub.CurrentPeriodEnd.After(now), nil
	case domain.SubscriptionStatusExpired,
		domain.SubscriptionStatusRevoked:
		return false, nil
	default:
		return false, fmt.Errorf("subscription: unknown status %q", sub.Status)
	}
}

// writeWithEvent は subscription 行の更新と premium-updated event の outbox enqueue を同一 tx で行う。
func writeWithEvent(
	ctx context.Context,
	subRepo port.SubscriptionRepo,
	sub *domain.Subscription,
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
	domainEv := domain.PremiumUpdatedEvent{
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
	}
	eventType, payload, err := presenter.ToPremiumUpdatedWire(domainEv)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("present premium-updated: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: eventType,
		Payload:   payload,
	}, nil
}

// writeNoEvent は subscription 行を更新し、premium-updated を発行しない (cancelled 遷移用)。
func writeNoEvent(ctx context.Context, subRepo port.SubscriptionRepo, sub *domain.Subscription) error {
	if err := subRepo.UpdateSubscription(ctx, sub); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}
