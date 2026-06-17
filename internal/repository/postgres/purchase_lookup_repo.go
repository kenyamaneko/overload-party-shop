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

var _ port.PurchaseLookupRepo = (*PurchaseLookupRepository)(nil)

// PurchaseLookupRepository は token から一回きり購入を引く読み取り専用 repo。
type PurchaseLookupRepository struct {
	pool *pgxpool.Pool
}

func NewPurchaseLookupRepository(pool *pgxpool.Pool) *PurchaseLookupRepository {
	return &PurchaseLookupRepository{pool: pool}
}

func (r *PurchaseLookupRepository) FindPurchaseByToken(ctx context.Context, platform, purchaseToken string) (*domain.OneTimePurchase, error) {
	table, err := resolvePurchaseTokenTable(platform)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(
		`SELECT p.purchase_id, p.player_id, p.product_id, p.purchased_at
		   FROM %s t
		   JOIN shop.one_time_purchases p ON p.purchase_id = t.purchase_id
		  WHERE t.token = $1`,
		table,
	)
	row := r.pool.QueryRow(ctx, q, purchaseToken)

	var p domain.OneTimePurchase
	if err := row.Scan(&p.PurchaseID, &p.PlayerID, &p.ProductID, &p.PurchasedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase by token: %w", err)
	}
	return &p, nil
}
