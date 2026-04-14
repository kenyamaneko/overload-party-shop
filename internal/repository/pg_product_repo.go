package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.ProductRepo = (*PgProductRepository)(nil)

type PgProductRepository struct {
	pool *pgxpool.Pool
}

func NewPgProductRepository(pool *pgxpool.Pool) *PgProductRepository {
	return &PgProductRepository{pool: pool}
}

func (r *PgProductRepository) GetActiveProducts(ctx context.Context) ([]*apishop.Product, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT product_id, name, type, price, content, description, image_url, is_active
		 FROM shop.products WHERE is_active = true`)
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var products []*apishop.Product
	for rows.Next() {
		var p apishop.Product
		var content []byte
		if err := rows.Scan(&p.ProductID, &p.Name, &p.Type, &p.Price, &content, &p.Description, &p.ImageURL, &p.IsActive); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		p.Content = json.RawMessage(content)
		products = append(products, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return products, nil
}

func (r *PgProductRepository) GetProductByID(ctx context.Context, productID string) (*apishop.Product, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT product_id, name, type, price, content, description, image_url, is_active
		 FROM shop.products WHERE product_id = $1`,
		productID)

	var p apishop.Product
	var content []byte
	err := row.Scan(&p.ProductID, &p.Name, &p.Type, &p.Price, &content, &p.Description, &p.ImageURL, &p.IsActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("product %s: %w", productID, port.ErrNotFound)
		}
		return nil, fmt.Errorf("read product: %w", err)
	}
	p.Content = json.RawMessage(content)
	return &p, nil
}
