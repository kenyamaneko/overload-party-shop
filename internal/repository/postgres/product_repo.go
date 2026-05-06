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

var _ port.ProductRepo = (*ProductRepository)(nil)

type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

const productSelectColumns = `
		p.product_id, p.name, p.type, p.price, p.description, p.image_url, p.is_active,
		cp.card_pack_id,
		f.faction,
		c.item_type, c.item_no,
		s.period_months`

const productJoinClause = `
		FROM shop.products p
		LEFT JOIN shop.product_card_pack    cp ON cp.product_id = p.product_id
		LEFT JOIN shop.product_faction      f  ON f.product_id  = p.product_id
		LEFT JOIN shop.product_cosmetic     c  ON c.product_id  = p.product_id
		LEFT JOIN shop.product_subscription s  ON s.product_id  = p.product_id`

// GetActiveProducts は販売中商品を type 別 ProductView に詰めて返す。
// 副表 LEFT JOIN で得た optional 列はそのまま domain.NewProductView に渡し、
// 型 dispatch / 不変条件検査は domain 層に委譲する。
func (r *ProductRepository) GetActiveProducts(ctx context.Context) ([]domain.ProductView, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT`+productSelectColumns+productJoinClause+`
		 WHERE p.is_active = true`)
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var products []domain.ProductView
	for rows.Next() {
		pv, err := scanProductRow(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, pv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return products, nil
}

// GetProductByID は指定 ID の商品を type 別 ProductView として返す。
func (r *ProductRepository) GetProductByID(ctx context.Context, productID string) (domain.ProductView, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT`+productSelectColumns+productJoinClause+`
		 WHERE p.product_id = $1`,
		productID)

	pv, err := scanProductRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("product %s: %w", productID, port.ErrNotFound)
		}
		return nil, err
	}
	return pv, nil
}

// scanProductRow は LEFT JOIN 結果 1 行を Scan して domain factory に委譲する。
// 型 dispatch / 不変条件検査は持たず、純粋に DB 行 → primitive の取り出しのみを行う。
func scanProductRow(row pgx.Row) (domain.ProductView, error) {
	var common domain.Product
	var cardPackID, faction, itemType *string
	var itemNo, periodMonths *int64

	if err := row.Scan(
		&common.ProductID, &common.Name, &common.Type, &common.Price,
		&common.Description, &common.ImageURL, &common.IsActive,
		&cardPackID,
		&faction,
		&itemType, &itemNo,
		&periodMonths,
	); err != nil {
		return nil, err
	}
	return domain.NewProductView(common, cardPackID, faction, itemType, itemNo, periodMonths)
}
