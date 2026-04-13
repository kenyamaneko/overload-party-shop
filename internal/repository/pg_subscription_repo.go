package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.SubscriptionRepo = (*PgSubscriptionRepository)(nil)

// PgSubscriptionRepository は pgxpool 経由の PostgreSQL で SubscriptionRepo を実装する。
// shop.subscriptions (純粋ドメイン) と shop.{apple,google}_subscription_tokens
// (外部識別マッピング) の 2 系統を協調操作する。
type PgSubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewPgSubscriptionRepository(pool *pgxpool.Pool) *PgSubscriptionRepository {
	return &PgSubscriptionRepository{pool: pool}
}

// subscriptionTokenTableForPlatform は subscription 用の token テーブル名を返す。
func subscriptionTokenTableForPlatform(platform string) (string, error) {
	switch platform {
	case apishop.PlatformIOS:
		return "shop.apple_subscription_tokens", nil
	case apishop.PlatformAndroid:
		return "shop.google_subscription_tokens", nil
	default:
		return "", fmt.Errorf("%w: subscription platform %q", port.ErrUnsupportedPlatform, platform)
	}
}

// CreateSubscription は subscriptions + 対応 token 行をアトミックに挿入する。
func (r *PgSubscriptionRepository) CreateSubscription(ctx context.Context, sub *apishop.Subscription, platform, purchaseToken string) error {
	table, err := subscriptionTokenTableForPlatform(platform)
	if err != nil {
		return err
	}

	if txFromContext(ctx) != nil {
		return r.createSubscriptionInner(ctx, connFrom(ctx, r.pool), sub, table, purchaseToken)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := r.createSubscriptionInner(ctx, tx, sub, table, purchaseToken); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PgSubscriptionRepository) createSubscriptionInner(ctx context.Context, db dbtx, sub *apishop.Subscription, tokenTable, purchaseToken string) error {
	if err := db.QueryRow(ctx,
		`INSERT INTO shop.subscriptions (player_id, product_id, status, current_period_start, current_period_end, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING subscription_id`,
		sub.PlayerID, sub.ProductID, sub.Status,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CreatedAt, sub.UpdatedAt,
	).Scan(&sub.SubscriptionID); err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}

	if _, err := db.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (token, subscription_id) VALUES ($1, $2)`, tokenTable),
		purchaseToken, sub.SubscriptionID,
	); err != nil {
		return fmt.Errorf("insert subscription token: %w", err)
	}
	return nil
}

// GetLatestSubscription は player の最新サブスクリプション 1 行を返す。
// 純粋ドメインなので token テーブルは引かない (status / 期間判定のみが必要なため)。
func (r *PgSubscriptionRepository) GetLatestSubscription(ctx context.Context, playerID string) (*apishop.Subscription, error) {
	row := connFrom(ctx, r.pool).QueryRow(ctx,
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

// FindSubscriptionByToken は platform に応じた token テーブル → subscriptions の JOIN で引く。
func (r *PgSubscriptionRepository) FindSubscriptionByToken(ctx context.Context, platform, purchaseToken string) (*apishop.Subscription, error) {
	table, err := subscriptionTokenTableForPlatform(platform)
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
	row := connFrom(ctx, r.pool).QueryRow(ctx, q, purchaseToken)
	s, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query subscription by token: %w", err)
	}
	return s, nil
}

func scanSubscription(row pgx.Row) (*apishop.Subscription, error) {
	var s apishop.Subscription
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

// UpdateSubscription はサブスクリプションの状態・期間を更新する。
func (r *PgSubscriptionRepository) UpdateSubscription(ctx context.Context, sub *apishop.Subscription) error {
	if _, err := connFrom(ctx, r.pool).Exec(ctx,
		`UPDATE shop.subscriptions SET
			status = $1,
			current_period_start = $2,
			current_period_end = $3,
			updated_at = $4
		  WHERE subscription_id = $5`,
		sub.Status,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.UpdatedAt,
		sub.SubscriptionID,
	); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}
