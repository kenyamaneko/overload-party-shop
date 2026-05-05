//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// seedProduct はテスト用の最小 seed: shop.products 共通行 + type 別副表 (faction_set / cosmetic) のデフォルト値で 1 商品を作る。
// faction_set のデフォルト faction は "SHE"、cosmetic のデフォルト item は (stamp, 1)。
// type 別の値を指定したいテストはここを呼ばず raw SQL で seed する。
func seedProduct(t *testing.T, productID, name, productType string, price int64, isActive bool) {
	t.Helper()
	desc := name + " desc"
	img := "https://img/" + productID
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.products (product_id, name, type, price, description, image_url, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		productID, name, productType, price, desc, img, isActive)
	require.NoError(t, err)

	switch productType {
	case domain.ProductTypeFactionSet:
		_, err = sharedPg.Pool.Exec(context.Background(),
			`INSERT INTO shop.product_faction (product_id, faction) VALUES ($1, $2)`,
			productID, "SHE")
		require.NoError(t, err)
	case domain.ProductTypeCosmetic:
		_, err = sharedPg.Pool.Exec(context.Background(),
			`INSERT INTO shop.cosmetic_items (item_type, item_no, item_name, is_purchasable, is_active)
			 VALUES ('stamp', 1, 'default stamp', true, true) ON CONFLICT DO NOTHING`)
		require.NoError(t, err)
		_, err = sharedPg.Pool.Exec(context.Background(),
			`INSERT INTO shop.product_cosmetic (product_id, item_type, item_no) VALUES ($1, 'stamp', 1)`,
			productID)
		require.NoError(t, err)
	case domain.ProductTypeSubscription:
		_, err = sharedPg.Pool.Exec(context.Background(),
			`INSERT INTO shop.product_subscription (product_id, period_months) VALUES ($1, $2)`,
			productID, 1)
		require.NoError(t, err)
	}
}

func seedPlayerItem(t *testing.T, playerID, itemType string, itemNo int64, acquiredAt time.Time) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.player_items (player_id, item_type, item_no, acquired_at)
		 VALUES ($1, $2, $3, $4)`,
		playerID, itemType, itemNo, acquiredAt)
	require.NoError(t, err)
}

func seedOwnedFaction(t *testing.T, playerID, faction string) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.player_owned_factions (player_id, faction)
		 VALUES ($1, $2)`,
		playerID, faction)
	require.NoError(t, err)
}
