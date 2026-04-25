package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.SubscriptionRepo = (*SubscriptionRepository)(nil)

// SubscriptionRepository は pgxpool 経由の PostgreSQL で SubscriptionRepo を実装する。
// shop.subscriptions (純粋ドメイン) と shop.{apple,google}_subscription_tokens
// (外部識別マッピング) の 2 系統を同一 tx で協調操作する。
type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

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

// CreateSubscription は subscriptions + 対応 token 行 + outbox event をアトミックに挿入する。
// event.EventType が空のときは outbox 書き込みをスキップする。
func (r *SubscriptionRepository) CreateSubscription(ctx context.Context, sub *apishop.Subscription, platform, purchaseToken string, event port.OutboxEvent) error {
	tokenTable, err := subscriptionTokenTableForPlatform(platform)
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
		sub.PlayerID, sub.ProductID, sub.Status,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CreatedAt, sub.UpdatedAt,
	).Scan(&sub.SubscriptionID); err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}

	if _, err := tx.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (token, subscription_id) VALUES ($1, $2)`, tokenTable),
		purchaseToken, sub.SubscriptionID,
	); err != nil {
		return fmt.Errorf("insert subscription token: %w", err)
	}

	if event.EventType != "" {
		if err := writeOutboxEvent(ctx, tx, event); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// GetLatestSubscription は player の最新サブスクリプション 1 行を返す。
// 純粋ドメインなので token テーブルは引かない (status / 期間判定のみが必要なため)。
func (r *SubscriptionRepository) GetLatestSubscription(ctx context.Context, playerID string) (*apishop.Subscription, error) {
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

// FindSubscriptionByToken は platform に応じた token テーブル → subscriptions の JOIN で引く。
func (r *SubscriptionRepository) FindSubscriptionByToken(ctx context.Context, platform, purchaseToken string) (*apishop.Subscription, error) {
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

// UpdateSubscription はサブスクリプションの状態・期間を更新しつつ、
// event.EventType が空でなければ同一 tx で outbox に書き込む。
// webhook 駆動の状態遷移 (renew/expire/revoke) で DB 更新と publish を atomic に
// 揃えるためのもの。解約 (cancelled) 等の「イベントを出さない遷移」では
// event.EventType を空にして呼ぶ。
func (r *SubscriptionRepository) UpdateSubscription(ctx context.Context, sub *apishop.Subscription, event port.OutboxEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
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

	if event.EventType != "" {
		if err := writeOutboxEvent(ctx, tx, event); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
