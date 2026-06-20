package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.SubscriptionRepo = (*SubscriptionRepository)(nil)

// SubscriptionRepository は subscriptions と {apple,google}_subscription_tokens の CRUD。
type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

func resolveSubscriptionTokenTable(platform string) (string, error) {
	switch platform {
	case domain.PlatformIOS:
		return "shop.apple_subscription_tokens", nil
	case domain.PlatformAndroid:
		return "shop.google_subscription_tokens", nil
	default:
		return "", fmt.Errorf("%w: subscription platform %q", port.ErrUnsupportedPlatform, platform)
	}
}

func (r *SubscriptionRepository) CreateSubscription(ctx context.Context, subscription *domain.Subscription, platform, purchaseToken string, event port.OutboxEvent) error {
	tokenTable, err := resolveSubscriptionTokenTable(platform)
	if err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.QueryRow(ctx,
		`INSERT INTO shop.subscriptions (player_id, product_id, status, current_period_start, current_period_end, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING subscription_id`,
		subscription.PlayerID, subscription.ProductID, subscription.Status,
		subscription.CurrentPeriodStart, subscription.CurrentPeriodEnd,
		subscription.CreatedAt, subscription.UpdatedAt,
	).Scan(&subscription.SubscriptionID); err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}

	if _, err := tx.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (token, subscription_id) VALUES ($1, $2)`, tokenTable),
		purchaseToken, subscription.SubscriptionID,
	); err != nil {
		return fmt.Errorf("insert subscription token: %w", err)
	}

	if err := writeOutboxEvent(ctx, tx, event); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// GetLatestSubscription は player の最新サブスクリプション 1 行を返す。
func (r *SubscriptionRepository) GetLatestSubscription(ctx context.Context, playerID string) (*domain.Subscription, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT subscription_id, player_id, product_id, status, current_period_start, current_period_end, created_at, updated_at
		   FROM shop.subscriptions
		  WHERE player_id = $1
		  ORDER BY created_at DESC
		  LIMIT 1`,
		playerID,
	)
	s, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest subscription: %w", err)
	}
	return s, nil
}

// FindSubscriptionByToken は platform に応じた token テーブル経由で subscription を引く。
func (r *SubscriptionRepository) FindSubscriptionByToken(ctx context.Context, platform, purchaseToken string) (*domain.Subscription, error) {
	table, err := resolveSubscriptionTokenTable(platform)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(
		`SELECT s.subscription_id, s.player_id, s.product_id, s.status, s.current_period_start, s.current_period_end, s.created_at, s.updated_at
		   FROM %s t
		   JOIN shop.subscriptions s ON s.subscription_id = t.subscription_id
		  WHERE t.token = $1`,
		table,
	)
	row := r.pool.QueryRow(ctx, q, purchaseToken)
	s, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query subscription by token: %w", err)
	}
	return s, nil
}

func scanSubscription(row pgx.Row) (*domain.Subscription, error) {
	var s domain.Subscription
	if err := row.Scan(
		&s.SubscriptionID, &s.PlayerID, &s.ProductID,
		&s.Status,
		&s.CurrentPeriodStart, &s.CurrentPeriodEnd,
		&s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateSubscription は subscriptions 行のみを更新する (outbox 行を伴わない)。
func (r *SubscriptionRepository) UpdateSubscription(ctx context.Context, subscription *domain.Subscription) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := updateSubscriptionRow(ctx, tx, subscription); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// UpdateSubscriptionWithEvent は subscriptions 更新と outbox 行 INSERT を同一 tx で行う。
func (r *SubscriptionRepository) UpdateSubscriptionWithEvent(ctx context.Context, subscription *domain.Subscription, event port.OutboxEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := updateSubscriptionRow(ctx, tx, subscription); err != nil {
		return err
	}
	if err := writeOutboxEvent(ctx, tx, event); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func updateSubscriptionRow(ctx context.Context, tx pgx.Tx, subscription *domain.Subscription) error {
	if _, err := tx.Exec(ctx,
		`UPDATE shop.subscriptions SET
			status = $1,
			current_period_start = $2,
			current_period_end = $3,
			updated_at = $4
		  WHERE subscription_id = $5`,
		subscription.Status,
		subscription.CurrentPeriodStart, subscription.CurrentPeriodEnd,
		subscription.UpdatedAt,
		subscription.SubscriptionID,
	); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}
