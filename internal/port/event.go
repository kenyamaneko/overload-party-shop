package port

import (
	"context"
	"time"
)

// FactionEventPublisher は faction-selected イベントをイベントバスに発行する。
type FactionEventPublisher interface {
	PublishFactionSelected(ctx context.Context, playerID, faction string) error
}

// PremiumEventPublisher は premium-updated イベントをイベントバスに発行する。
type PremiumEventPublisher interface {
	PublishPremiumUpdated(ctx context.Context, playerID string, isPremium bool, expiresAt *time.Time) error
}
