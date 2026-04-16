package service

import (
	"context"
	"fmt"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// SubscriptionService は webhook 駆動のサブスクリプションライフサイクルイベントを処理する。
// account.players には触れず、shop.subscriptions をローカル更新して `premium-updated`
// イベントを outbox に enqueue する (DB 更新 + outbox 行は repo 層の単一 tx で atomic に
// commit される)。publish は worker が行う。
type SubscriptionService struct {
	subRepo        port.SubscriptionRepo
	eventBuilder   port.OutboxEventBuilder
	googleVerifier port.GoogleSubVerifier
}

// NewSubscriptionService は依存を受け取り SubscriptionService を構築する。
func NewSubscriptionService(
	subRepo port.SubscriptionRepo,
	eventBuilder port.OutboxEventBuilder,
	googleVerifier port.GoogleSubVerifier,
) *SubscriptionService {
	return &SubscriptionService{
		subRepo:        subRepo,
		eventBuilder:   eventBuilder,
		googleVerifier: googleVerifier,
	}
}

// applySubChangeWithEvent はサブスクリプション行の更新と premium-updated 行の
// outbox enqueue を同一 tx で行う。DB commit 成功時点で publish は worker に
// 委ねられ、dual-write 問題が起きない。
func (s *SubscriptionService) applySubChangeWithEvent(ctx context.Context, sub *apishop.Subscription, isPremium bool, expiresAt *time.Time) error {
	ev, err := s.eventBuilder.BuildPremiumUpdated(sub.PlayerID, isPremium, expiresAt)
	if err != nil {
		return fmt.Errorf("build premium-updated: %w", err)
	}
	if err := s.subRepo.UpdateSubscription(ctx, sub, ev); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}

// applySubChangeNoEvent は premium-updated を発行しない状態遷移 (解約時の
// cancelled 遷移など、エンタイトルメント維持契約により publish しないケース) で使う。
func (s *SubscriptionService) applySubChangeNoEvent(ctx context.Context, sub *apishop.Subscription) error {
	if err := s.subRepo.UpdateSubscription(ctx, sub, port.OutboxEvent{}); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}
