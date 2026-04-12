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

var _ port.ShopRepository = (*PgShopRepository)(nil)

// PgShopRepository は pgxpool 経由の PostgreSQL で ShopRepository を実装する。
// 全ステートメントは `shop` スキーマを対象とする。Pub/Sub fan-out リファクタ以降、
// account.players / account.player_factions / card.player_cards への書き込みは
// faction-selected / premium-updated topic 経由で各サービスが処理する。
type PgShopRepository struct {
	pool *pgxpool.Pool
}

// NewPgShopRepository は pgxpool.Pool を受け取り PgShopRepository を構築する。
func NewPgShopRepository(pool *pgxpool.Pool) *PgShopRepository {
	return &PgShopRepository{pool: pool}
}

// GetActiveProducts はアクティブな商品一覧を返す。
func (r *PgShopRepository) GetActiveProducts(ctx context.Context) ([]*apishop.Product, error) {
	rows, err := connFrom(ctx, r.pool).Query(ctx,
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

// GetProductByID は指定 ID の商品を返す。
func (r *PgShopRepository) GetProductByID(ctx context.Context, productID string) (*apishop.Product, error) {
	row := connFrom(ctx, r.pool).QueryRow(ctx,
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

// FindPurchaseByToken はトークンで購入レコードを検索する。
func (r *PgShopRepository) FindPurchaseByToken(ctx context.Context, playerID, purchaseToken string) (*apishop.OneTimePurchase, error) {
	row := connFrom(ctx, r.pool).QueryRow(ctx,
		`SELECT player_id, purchase_id, product_id, platform, purchase_token, purchased_at
		 FROM shop.one_time_purchases
		 WHERE player_id = $1 AND purchase_token = $2
		 LIMIT 1`,
		playerID, purchaseToken)

	var p apishop.OneTimePurchase
	err := row.Scan(&p.PlayerID, &p.PurchaseID, &p.ProductID, &p.Platform, &p.PurchaseToken, &p.PurchasedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase by token: %w", err)
	}
	return &p, nil
}

// CreatePurchase は購入レコードを記録する。重複トークンはべき等に no-op する。
func (r *PgShopRepository) CreatePurchase(ctx context.Context, purchase *apishop.OneTimePurchase) error {
	db := connFrom(ctx, r.pool)
	var existingID int64
	err := db.QueryRow(ctx,
		`SELECT purchase_id FROM shop.one_time_purchases
		 WHERE player_id = $1 AND purchase_token = $2 LIMIT 1`,
		purchase.PlayerID, purchase.PurchaseToken,
	).Scan(&existingID)
	if err == nil {
		purchase.PurchaseID = existingID
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check existing purchase: %w", err)
	}

	err = db.QueryRow(ctx,
		`INSERT INTO shop.one_time_purchases (player_id, product_id, platform, purchase_token, purchased_at)
		 VALUES ($1,$2,$3,$4,$5) RETURNING purchase_id`,
		purchase.PlayerID, purchase.ProductID,
		purchase.Platform, purchase.PurchaseToken, purchase.PurchasedAt,
	).Scan(&purchase.PurchaseID)
	if err != nil {
		return fmt.Errorf("insert purchase: %w", err)
	}
	return nil
}

// CreatePurchaseWithItem は購入レコードとプレイヤーアイテムをアトミックに記録する。
func (r *PgShopRepository) CreatePurchaseWithItem(ctx context.Context, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem) error {
	if txFromContext(ctx) != nil {
		return r.createPurchaseWithItemInner(ctx, connFrom(ctx, r.pool), purchase, item)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := r.createPurchaseWithItemInner(ctx, tx, purchase, item); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PgShopRepository) createPurchaseWithItemInner(ctx context.Context, db dbtx, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem) error {
	var existingID int64
	err := db.QueryRow(ctx,
		`SELECT purchase_id FROM shop.one_time_purchases
		 WHERE player_id = $1 AND purchase_token = $2 LIMIT 1`,
		purchase.PlayerID, purchase.PurchaseToken,
	).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check existing purchase: %w", err)
	}

	err = db.QueryRow(ctx,
		`INSERT INTO shop.one_time_purchases (player_id, product_id, platform, purchase_token, purchased_at)
		 VALUES ($1,$2,$3,$4,$5) RETURNING purchase_id`,
		purchase.PlayerID, purchase.ProductID,
		purchase.Platform, purchase.PurchaseToken, purchase.PurchasedAt,
	).Scan(&purchase.PurchaseID)
	if err != nil {
		return fmt.Errorf("insert purchase: %w", err)
	}

	_, err = db.Exec(ctx,
		`INSERT INTO shop.player_items (player_id, item_type, item_no, acquired_at)
		 VALUES ($1,$2,$3,$4)`,
		item.PlayerID, item.ItemType, item.ItemNo, item.AcquiredAt,
	)
	if err != nil {
		return fmt.Errorf("insert player item: %w", err)
	}
	return nil
}

// InsertPlayerItems はプレイヤーアイテムをアトミックに挿入する。
// context にトランザクションがあればそれに参加し、なければ独自に開始する。
func (r *PgShopRepository) InsertPlayerItems(ctx context.Context, items []*apishop.PlayerItem) error {
	if txFromContext(ctx) != nil {
		return r.insertPlayerItemsInner(ctx, connFrom(ctx, r.pool), items)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := r.insertPlayerItemsInner(ctx, tx, items); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PgShopRepository) insertPlayerItemsInner(ctx context.Context, db dbtx, items []*apishop.PlayerItem) error {
	for _, item := range items {
		_, err := db.Exec(ctx,
			`INSERT INTO shop.player_items (player_id, item_type, item_no, acquired_at)
			 VALUES ($1,$2,$3,$4)`,
			item.PlayerID, item.ItemType, item.ItemNo, item.AcquiredAt,
		)
		if err != nil {
			return fmt.Errorf("insert player item: %w", err)
		}
	}
	return nil
}

// HasPlayerItem はプレイヤーが指定アイテムを所有しているかを返す。
func (r *PgShopRepository) HasPlayerItem(ctx context.Context, playerID, itemType string, itemNo int64) (bool, error) {
	var exists bool
	err := connFrom(ctx, r.pool).QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM shop.player_items
			WHERE player_id = $1 AND item_type = $2 AND item_no = $3
		)`,
		playerID, itemType, itemNo,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check player item: %w", err)
	}
	return exists, nil
}
