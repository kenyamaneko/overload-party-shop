//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func seedProduct(t *testing.T, productID, name, productType string, price int64, isActive bool) {
	t.Helper()
	desc := name + " desc"
	img := "https://img/" + productID
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.products (product_id, name, type, price, content, description, image_url, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		productID, name, productType, price, json.RawMessage(`{}`), desc, img, isActive)
	require.NoError(t, err)
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
