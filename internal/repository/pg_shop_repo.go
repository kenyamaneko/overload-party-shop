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

// purchaseTokenTableForPlatform は purchase 用の token テーブル名を返す。
func purchaseTokenTableForPlatform(platform string) (string, error) {
	switch platform {
	case apishop.PlatformIOS:
		return "shop.apple_purchase_tokens", nil
	case apishop.PlatformAndroid:
		return "shop.google_purchase_tokens", nil
	default:
		return "", fmt.Errorf("%w: purchase platform %q", port.ErrUnsupportedPlatform, platform)
	}
}

// FindPurchaseByToken は platform 別 token テーブルから purchase を引く。
// playerID が token テーブル経由で得た purchase の player と一致しない場合は nil を返す。
func (r *PgShopRepository) FindPurchaseByToken(ctx context.Context, platform, purchaseToken string) (*apishop.OneTimePurchase, error) {
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
	row := connFrom(ctx, r.pool).QueryRow(ctx, q, purchaseToken)

	var p apishop.OneTimePurchase
	if err := row.Scan(&p.PurchaseID, &p.PlayerID, &p.ProductID, &p.PurchasedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase by token: %w", err)
	}
	return &p, nil
}

// CreatePurchase は one_time_purchases + 対応する token 行をアトミックに挿入する。
// 既に同じ token があれば既存 purchase_id を埋めてべき等に no-op (created は false)。
func (r *PgShopRepository) CreatePurchase(ctx context.Context, purchase *apishop.OneTimePurchase, platform, purchaseToken string) error {
	if txFromContext(ctx) != nil {
		_, err := r.createPurchaseInner(ctx, connFrom(ctx, r.pool), purchase, platform, purchaseToken)
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := r.createPurchaseInner(ctx, tx, purchase, platform, purchaseToken); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// createPurchaseInner は purchase + token を挿入する。created=true なら新規 INSERT、
// created=false なら既存 token に対するべき等 no-op 経路。
func (r *PgShopRepository) createPurchaseInner(ctx context.Context, db dbtx, purchase *apishop.OneTimePurchase, platform, purchaseToken string) (created bool, err error) {
	table, err := purchaseTokenTableForPlatform(platform)
	if err != nil {
		return false, err
	}

	var existingPurchaseID int64
	err = db.QueryRow(ctx,
		fmt.Sprintf(`SELECT purchase_id FROM %s WHERE token = $1`, table),
		purchaseToken,
	).Scan(&existingPurchaseID)
	if err == nil {
		purchase.PurchaseID = existingPurchaseID
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check existing purchase token: %w", err)
	}

	if err := db.QueryRow(ctx,
		`INSERT INTO shop.one_time_purchases (player_id, product_id, purchased_at)
		 VALUES ($1,$2,$3) RETURNING purchase_id`,
		purchase.PlayerID, purchase.ProductID, purchase.PurchasedAt,
	).Scan(&purchase.PurchaseID); err != nil {
		return false, fmt.Errorf("insert purchase: %w", err)
	}

	if _, err := db.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (token, purchase_id) VALUES ($1, $2)`, table),
		purchaseToken, purchase.PurchaseID,
	); err != nil {
		return false, fmt.Errorf("insert purchase token: %w", err)
	}
	return true, nil
}

// CreatePurchaseWithItem は purchase + token + player_items をアトミックに挿入する。
// 既存 token (べき等経路) では player_items 挿入もスキップする。
func (r *PgShopRepository) CreatePurchaseWithItem(ctx context.Context, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem, platform, purchaseToken string) error {
	if txFromContext(ctx) != nil {
		return r.createPurchaseWithItemInner(ctx, connFrom(ctx, r.pool), purchase, item, platform, purchaseToken)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := r.createPurchaseWithItemInner(ctx, tx, purchase, item, platform, purchaseToken); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PgShopRepository) createPurchaseWithItemInner(ctx context.Context, db dbtx, purchase *apishop.OneTimePurchase, item *apishop.PlayerItem, platform, purchaseToken string) error {
	created, err := r.createPurchaseInner(ctx, db, purchase, platform, purchaseToken)
	if err != nil {
		return err
	}
	if !created {
		return nil
	}
	if _, err := db.Exec(ctx,
		`INSERT INTO shop.player_items (player_id, item_type, item_no, acquired_at)
		 VALUES ($1,$2,$3,$4)`,
		item.PlayerID, item.ItemType, item.ItemNo, item.AcquiredAt,
	); err != nil {
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

func (r *PgShopRepository) ListPlayerItems(ctx context.Context, playerID string) ([]*apishop.PlayerItem, error) {
	rows, err := connFrom(ctx, r.pool).Query(ctx,
		`SELECT player_id, item_type, item_no, acquired_at
		   FROM shop.player_items
		  WHERE player_id = $1`,
		playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("query player items: %w", err)
	}
	defer rows.Close()

	var items []*apishop.PlayerItem
	for rows.Next() {
		item := &apishop.PlayerItem{}
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
