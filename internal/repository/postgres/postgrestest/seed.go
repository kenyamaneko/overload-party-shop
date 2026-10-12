package postgrestest

import (
	"context"
	"testing"
)

// ProductSeed は shop.products に直接投入する商品の共通項目。
type ProductSeed struct {
	ProductID   string
	Name        string
	Price       int64
	Description *string
	ImageURL    *string
	IsActive    bool
}

// InsertProduct は種別固有の属性を持たない商品を productType の種別で shop.products に投入する。
func (p *Postgres) InsertProduct(t *testing.T, seed ProductSeed, productType string) {
	t.Helper()
	_, err := p.Pool.Exec(context.Background(), `
		INSERT INTO shop.products (product_id, name, type, price, description, image_url, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, seed.ProductID, seed.Name, productType, seed.Price, seed.Description, seed.ImageURL, seed.IsActive)
	if err != nil {
		t.Fatalf("insert product %s: %v", seed.ProductID, err)
	}
}

// InsertProductCardPack は商品に配布するカードパックIDを shop.product_card_pack に投入する。
func (p *Postgres) InsertProductCardPack(t *testing.T, productID, cardPackID string) {
	t.Helper()
	_, err := p.Pool.Exec(context.Background(), `
		INSERT INTO shop.product_card_pack (product_id, card_pack_id) VALUES ($1, $2)
	`, productID, cardPackID)
	if err != nil {
		t.Fatalf("insert product card pack %s: %v", productID, err)
	}
}

// InsertFactionSetProduct は陣営セットの商品を、陣営とカードパックIDの副表とともに投入する。
func (p *Postgres) InsertFactionSetProduct(t *testing.T, seed ProductSeed, faction, cardPackID string) {
	t.Helper()
	p.InsertProduct(t, seed, "faction_set")
	p.InsertProductFaction(t, seed.ProductID, faction)
	p.InsertProductCardPack(t, seed.ProductID, cardPackID)
}

// InsertProductFaction は商品に付与する陣営を shop.product_faction に投入する。
func (p *Postgres) InsertProductFaction(t *testing.T, productID, faction string) {
	t.Helper()
	_, err := p.Pool.Exec(context.Background(), `
		INSERT INTO shop.product_faction (product_id, faction) VALUES ($1, $2)
	`, productID, faction)
	if err != nil {
		t.Fatalf("insert product faction %s: %v", productID, err)
	}
}

// InsertCardPackProduct はカードパックの商品を、カードパックIDの副表とともに投入する。
func (p *Postgres) InsertCardPackProduct(t *testing.T, seed ProductSeed, cardPackID string) {
	t.Helper()
	p.InsertProduct(t, seed, "card_pack")
	p.InsertProductCardPack(t, seed.ProductID, cardPackID)
}

// InsertCosmeticProduct はコスメティックの商品を、参照先のアイテムとアイテム種別・アイテム番号の副表とともに投入する。
func (p *Postgres) InsertCosmeticProduct(t *testing.T, seed ProductSeed, itemType string, itemNo int64) {
	t.Helper()
	ctx := context.Background()
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO shop.cosmetic_items (item_type, item_no, item_name, is_purchasable, is_active)
		VALUES ($1, $2, $3, TRUE, TRUE)
		ON CONFLICT (item_type, item_no) DO NOTHING
	`, itemType, itemNo, seed.Name)
	if err != nil {
		t.Fatalf("insert cosmetic item %s/%d: %v", itemType, itemNo, err)
	}
	p.InsertProduct(t, seed, "cosmetic")
	_, err = p.Pool.Exec(ctx, `
		INSERT INTO shop.product_cosmetic (product_id, item_type, item_no) VALUES ($1, $2, $3)
	`, seed.ProductID, itemType, itemNo)
	if err != nil {
		t.Fatalf("insert product cosmetic %s: %v", seed.ProductID, err)
	}
}

// InsertSubscriptionProduct はサブスクリプションの商品を、課金周期の月数の副表とともに投入する。
func (p *Postgres) InsertSubscriptionProduct(t *testing.T, seed ProductSeed, periodMonths int) {
	t.Helper()
	p.InsertProduct(t, seed, "subscription")
	_, err := p.Pool.Exec(context.Background(), `
		INSERT INTO shop.product_subscription (product_id, period_months) VALUES ($1, $2)
	`, seed.ProductID, periodMonths)
	if err != nil {
		t.Fatalf("insert product subscription %s: %v", seed.ProductID, err)
	}
}

// CountRows は table (スキーマ名つき) の行数を返す。
func (p *Postgres) CountRows(t *testing.T, table string) int {
	t.Helper()
	var count int
	err := p.Pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count)
	if err != nil {
		t.Fatalf("count rows %s: %v", table, err)
	}
	return count
}

// RunShopMain は shop のスキーマ (db/schema.sql の shop スキーマ) を適用した PostgreSQL で TestMain を実行する。
func RunShopMain(m *testing.M, out **Postgres) int {
	return RunMain(m, out, WithSchemaFile("db/schema.sql"), WithSchema("shop"))
}
