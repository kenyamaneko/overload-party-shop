package subscription

import (
	"context"
	"fmt"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// IsEntitled はサブスクリプションが指定時刻に特典有効かを返す。
// active / cancelled (auto-renew off だが期間内) / grace_period (支払い失敗中だが猶予内)
// が期間内である限り特典は有効。expired / revoked は無効。
func IsEntitled(sub *apishop.Subscription, now time.Time) bool {
	if sub == nil {
		return false
	}
	switch sub.Status {
	case apishop.SubscriptionStatusActive,
		apishop.SubscriptionStatusCancelled,
		apishop.SubscriptionStatusGrace:
		return sub.CurrentPeriodEnd.After(now)
	}
	return false
}

// writeWithEvent はサブスクリプション行の更新と premium-updated 行の outbox enqueue を
// 同一 tx で行う (dual-write 問題を避ける契約)。AppleNotifier / GoogleNotifier 共通。
func writeWithEvent(
	ctx context.Context,
	subRepo port.SubscriptionRepo,
	eventBuilder port.OutboxEventBuilder,
	sub *apishop.Subscription,
	isPremium bool,
	expiresAt *time.Time,
) error {
	ev, err := eventBuilder.BuildPremiumUpdated(sub.PlayerID, isPremium, expiresAt)
	if err != nil {
		return fmt.Errorf("build premium-updated: %w", err)
	}
	if err := subRepo.UpdateSubscription(ctx, sub, ev); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}

// writeNoEvent は premium-updated を発行しない状態遷移 (解約時の cancelled 遷移など、
// エンタイトルメント維持契約により publish しないケース) で使う。
func writeNoEvent(ctx context.Context, subRepo port.SubscriptionRepo, sub *apishop.Subscription) error {
	if err := subRepo.UpdateSubscription(ctx, sub, port.OutboxEvent{}); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}
