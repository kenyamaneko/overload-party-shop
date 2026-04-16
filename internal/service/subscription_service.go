package service

import (
	"context"
	"fmt"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// SubscriptionService は webhook 駆動のサブスクリプションライフサイクルイベントを処理する。
// Pub/Sub fan-out リファクタ以降 account.players には触れず、shop.subscriptions を
// ローカル更新し `premium-updated` イベントを発行する。
type SubscriptionService struct {
	subRepo          port.SubscriptionRepo
	premiumPublisher port.PremiumEventPublisher
	googleVerifier   port.GoogleSubVerifier
}

// NewSubscriptionService は依存を受け取り SubscriptionService を構築する。
func NewSubscriptionService(
	subRepo port.SubscriptionRepo,
	premiumPublisher port.PremiumEventPublisher,
	googleVerifier port.GoogleSubVerifier,
) *SubscriptionService {
	return &SubscriptionService{
		subRepo:          subRepo,
		premiumPublisher: premiumPublisher,
		googleVerifier:   googleVerifier,
	}
}

// applySubChangeAndPublish はサブスクリプション行を先に更新してから publish する。
// sub 行が永続記録であり、publish 失敗時は webhook リトライが再駆動する。
func (s *SubscriptionService) applySubChangeAndPublish(ctx context.Context, sub *apishop.Subscription, isPremium bool, expiresAt *time.Time) error {
	if err := s.subRepo.UpdateSubscription(ctx, sub); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	if s.premiumPublisher != nil {
		if err := s.premiumPublisher.PublishPremiumUpdated(ctx, sub.PlayerID, isPremium, expiresAt); err != nil {
			return fmt.Errorf("publish premium-updated: %w", err)
		}
	}
	return nil
}
