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
		fg.faction,
		c.item_type, c.item_no`

const productJoinClause = `
		FROM shop.products p
		LEFT JOIN shop.product_faction_grants fg ON fg.product_id = p.product_id
		LEFT JOIN shop.product_cosmetics       c  ON c.product_id  = p.product_id`

// GetActiveProducts は販売中商品を type 別 ProductView に詰めて返す。
// faction_set / cosmetic は副表 LEFT JOIN で付帯属性を引く。
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
		pv, err := scanProductView(rows)
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

	pv, err := scanProductView(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("product %s: %w", productID, port.ErrNotFound)
		}
		return nil, err
	}
	return pv, nil
}

// scanProductView は products + product_faction_grants + product_cosmetics の
// LEFT JOIN 結果 1 行を読み、type に応じた ProductView 実装を返す。
// type と副表行の整合は application 層の責務 (DDL は CHECK で縛らない)。
func scanProductView(row pgx.Row) (domain.ProductView, error) {
	var common domain.Product
	var faction *string
	var itemType *string
	var itemNo *int64

	if err := row.Scan(
		&common.ProductID, &common.Name, &common.Type, &common.Price,
		&common.Description, &common.ImageURL, &common.IsActive,
		&faction,
		&itemType, &itemNo,
	); err != nil {
		return nil, err
	}

	switch common.Type {
	case domain.ProductTypeFactionSet:
		if faction == nil {
			return nil, fmt.Errorf("product %s: type=%s but product_faction_grants missing", common.ProductID, common.Type)
		}
		return domain.FactionSetProduct{Product: common, Faction: *faction}, nil
	case domain.ProductTypeCosmetic:
		if itemType == nil || itemNo == nil {
			return nil, fmt.Errorf("product %s: type=%s but product_cosmetics missing", common.ProductID, common.Type)
		}
		return domain.CosmeticProduct{Product: common, ItemType: *itemType, ItemNo: *itemNo}, nil
	case domain.ProductTypeSubscription:
		return domain.SubscriptionProduct{Product: common}, nil
	default:
		return nil, fmt.Errorf("product %s: unknown type %q", common.ProductID, common.Type)
	}
}
