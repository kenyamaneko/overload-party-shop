package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.ItemPurchaseRepo = (*ItemPurchaseRepository)(nil)

// ItemPurchaseRepository は cosmetic 購入 aggregate (purchase + token + player_item) を扱う。
type ItemPurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewItemPurchaseRepository(pool *pgxpool.Pool) *ItemPurchaseRepository {
	return &ItemPurchaseRepository{pool: pool}
}

func (r *ItemPurchaseRepository) CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, item *domain.PlayerItem, platform, purchaseToken string) (created bool, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err = insertOneTimePurchaseAndToken(ctx, tx, purchase, platform, purchaseToken)
	if err != nil {
		return false, err
	}
	if !created {
		return false, nil
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO shop.player_items (player_id, item_type, item_no, acquired_at)
		 VALUES ($1,$2,$3,$4)`,
		item.PlayerID, item.ItemType, item.ItemNo, item.AcquiredAt,
	); err != nil {
		return false, fmt.Errorf("insert player item: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}
	return true, nil
}

func (r *ItemPurchaseRepository) HasPlayerItem(ctx context.Context, playerID, itemType string, itemNo int64) (bool, error) {
	var isOwned bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM shop.player_items
		                 WHERE player_id = $1 AND item_type = $2 AND item_no = $3)`,
		playerID, itemType, itemNo).Scan(&isOwned)
	if err != nil {
		return false, fmt.Errorf("query has player item: %w", err)
	}
	return isOwned, nil
}

func (r *ItemPurchaseRepository) ListPlayerItems(ctx context.Context, playerID string) ([]*domain.PlayerItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT player_id, item_type, item_no, acquired_at
		   FROM shop.player_items
		  WHERE player_id = $1`,
		playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("query player items: %w", err)
	}
	defer rows.Close()

	var items []*domain.PlayerItem
	for rows.Next() {
		item := &domain.PlayerItem{}
		if err := rows.Scan(&item.PlayerID, &item.ItemType, &item.ItemNo, &item.AcquiredAt); err != nil {
			return nil, fmt.Errorf("scan player item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate player items: %w", err)
	}
	return items, nil
}
