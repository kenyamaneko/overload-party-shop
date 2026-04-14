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

var _ port.PurchaseLookupRepo = (*PgPurchaseLookupRepository)(nil)

// PgPurchaseLookupRepository は token から一回きり購入を引く cross-cutting 読み取り repo。
// faction / item どちらの aggregate にも属さず、webhook 冪等性チェック用の早期 short-circuit に使う。
type PgPurchaseLookupRepository struct {
	pool *pgxpool.Pool
}

func NewPgPurchaseLookupRepository(pool *pgxpool.Pool) *PgPurchaseLookupRepository {
	return &PgPurchaseLookupRepository{pool: pool}
}

func (r *PgPurchaseLookupRepository) FindPurchaseByToken(ctx context.Context, platform, purchaseToken string) (*apishop.OneTimePurchase, error) {
	table, err := purchaseTokenTableForPlatform(platform)
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

	var p apishop.OneTimePurchase
	if err := row.Scan(&p.PurchaseID, &p.PlayerID, &p.ProductID, &p.PurchasedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase by token: %w", err)
	}
	return &p, nil
}
