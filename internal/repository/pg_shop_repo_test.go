package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/testutil"
)

var sharedPg *testutil.Postgres

func TestMain(m *testing.M) {
	os.Exit(testutil.RunMain(m, &sharedPg,
		testutil.WithSchemaFile("db/schema.sql"),
		testutil.WithSchema("shop"),
	))
}

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

func TestPgShopRepository_GetActiveProducts(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()

	seedProduct(t, "p1", "Active 1", apishop.ProductTypeFactionSet, 100, true)
	seedProduct(t, "p2", "Inactive", apishop.ProductTypeFactionSet, 200, false)
	seedProduct(t, "p3", "Active 2", apishop.ProductTypeCosmetic, 300, true)

	got, err := repo.GetActiveProducts(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)

	ids := []string{got[0].ProductID, got[1].ProductID}
	assert.ElementsMatch(t, []string{"p1", "p3"}, ids)
}

func TestPgShopRepository_GetProductByID(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()

	seedProduct(t, "faction_she", "SHE Pack", apishop.ProductTypeFactionSet, 980, true)

	t.Run("found", func(t *testing.T) {
		got, err := repo.GetProductByID(ctx, "faction_she")
		require.NoError(t, err)
		assert.Equal(t, "SHE Pack", got.Name)
		assert.Equal(t, int64(980), got.Price)
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		_, err := repo.GetProductByID(ctx, "missing")
		assert.True(t, errors.Is(err, port.ErrNotFound))
	})
}

func TestPgShopRepository_CreatePurchase_NewToken(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()
	seedProduct(t, "faction_tenki", "Tenki", apishop.ProductTypeFactionSet, 980, true)

	purchase := &apishop.OneTimePurchase{
		PlayerID:    "11111111-1111-1111-1111-111111111111",
		ProductID:   "faction_tenki",
		PurchasedAt: time.Now().UTC(),
	}
	err := repo.CreatePurchase(ctx, purchase, apishop.PlatformIOS, "apple-token-1")
	require.NoError(t, err)
	assert.NotZero(t, purchase.PurchaseID)

	found, err := repo.FindPurchaseByToken(ctx, apishop.PlatformIOS, "apple-token-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, purchase.PurchaseID, found.PurchaseID)
	assert.Equal(t, "faction_tenki", found.ProductID)
}

func TestPgShopRepository_CreatePurchase_Idempotent(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()
	seedProduct(t, "faction_tenki", "Tenki", apishop.ProductTypeFactionSet, 980, true)

	first := &apishop.OneTimePurchase{
		PlayerID:    "22222222-2222-2222-2222-222222222222",
		ProductID:   "faction_tenki",
		PurchasedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchase(ctx, first, apishop.PlatformAndroid, "google-token-1"))

	second := &apishop.OneTimePurchase{
		PlayerID:    "22222222-2222-2222-2222-222222222222",
		ProductID:   "faction_tenki",
		PurchasedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchase(ctx, second, apishop.PlatformAndroid, "google-token-1"))

	assert.Equal(t, first.PurchaseID, second.PurchaseID, "既存トークンは同一 purchase_id を返す")

	var rowCount int
	require.NoError(t, sharedPg.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&rowCount))
	assert.Equal(t, 1, rowCount)
}

func TestPgShopRepository_CreatePurchase_UnsupportedPlatform(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()
	seedProduct(t, "faction_she", "SHE", apishop.ProductTypeFactionSet, 980, true)

	purchase := &apishop.OneTimePurchase{
		PlayerID:    "33333333-3333-3333-3333-333333333333",
		ProductID:   "faction_she",
		PurchasedAt: time.Now().UTC(),
	}
	err := repo.CreatePurchase(ctx, purchase, "windows", "tok")
	assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
}

func TestPgShopRepository_CreatePurchaseWithItem(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()
	seedProduct(t, "playmat_01", "Playmat", apishop.ProductTypeCosmetic, 320, true)

	playerID := "44444444-4444-4444-4444-444444444444"
	purchase := &apishop.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   "playmat_01",
		PurchasedAt: time.Now().UTC(),
	}
	item := &apishop.PlayerItem{
		PlayerID:   playerID,
		ItemType:   apishop.ItemTypePlaymat,
		ItemNo:     1,
		AcquiredAt: time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchaseWithItem(ctx, purchase, item, apishop.PlatformIOS, "cosmetic-token-1"))

	t.Run("player_items inserted", func(t *testing.T) {
		has, err := repo.HasPlayerItem(ctx, playerID, apishop.ItemTypePlaymat, 1)
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("idempotent: re-call same token does not duplicate item", func(t *testing.T) {
		dup := &apishop.OneTimePurchase{PlayerID: playerID, ProductID: "playmat_01", PurchasedAt: time.Now().UTC()}
		dupItem := &apishop.PlayerItem{PlayerID: playerID, ItemType: apishop.ItemTypePlaymat, ItemNo: 1, AcquiredAt: time.Now().UTC()}
		require.NoError(t, repo.CreatePurchaseWithItem(ctx, dup, dupItem, apishop.PlatformIOS, "cosmetic-token-1"))

		items, err := repo.ListPlayerItems(ctx, playerID)
		require.NoError(t, err)
		assert.Len(t, items, 1)
	})
}

// 同じ token を別プレイヤーで再 POST した場合、token は最初の purchase に
// 紐付き続け、二度目の呼び出しは既存 purchase_id を返す（store の側で player
// 検証を行う責務であり、repo は token 単位でしか idempotency を見ない契約）。
func TestPgShopRepository_CreatePurchase_ExistingTokenIsBoundToFirstPurchase(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()
	seedProduct(t, "faction_sugar", "Sugar", apishop.ProductTypeFactionSet, 980, true)

	first := &apishop.OneTimePurchase{
		PlayerID:    "55555555-5555-5555-5555-555555555555",
		ProductID:   "faction_sugar",
		PurchasedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchase(ctx, first, apishop.PlatformIOS, "shared-token"))

	second := &apishop.OneTimePurchase{
		PlayerID:    "66666666-6666-6666-6666-666666666666",
		ProductID:   "faction_sugar",
		PurchasedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.CreatePurchase(ctx, second, apishop.PlatformIOS, "shared-token"))

	assert.Equal(t, first.PurchaseID, second.PurchaseID, "既存 token は最初の purchase_id を返す")

	found, err := repo.FindPurchaseByToken(ctx, apishop.PlatformIOS, "shared-token")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, first.PlayerID, found.PlayerID, "token は最初の購入者に永続的に紐付く")

	var rowCount int
	require.NoError(t, sharedPg.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&rowCount))
	assert.Equal(t, 1, rowCount)
}

func TestPgShopRepository_PlayerItems(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()

	playerID := "77777777-7777-7777-7777-777777777777"
	other := "88888888-8888-8888-8888-888888888888"

	now := time.Now().UTC()
	require.NoError(t, repo.InsertPlayerItems(ctx, []*apishop.PlayerItem{
		{PlayerID: playerID, ItemType: apishop.ItemTypePlaymat, ItemNo: 1, AcquiredAt: now},
		{PlayerID: playerID, ItemType: apishop.ItemTypeSleeve, ItemNo: 2, AcquiredAt: now},
		{PlayerID: other, ItemType: apishop.ItemTypeIcon, ItemNo: 9, AcquiredAt: now},
	}))

	t.Run("ListPlayerItems scopes by player_id", func(t *testing.T) {
		items, err := repo.ListPlayerItems(ctx, playerID)
		require.NoError(t, err)
		assert.Len(t, items, 2)
	})

	t.Run("HasPlayerItem true/false", func(t *testing.T) {
		has, err := repo.HasPlayerItem(ctx, playerID, apishop.ItemTypePlaymat, 1)
		require.NoError(t, err)
		assert.True(t, has)

		has, err = repo.HasPlayerItem(ctx, playerID, apishop.ItemTypeIcon, 9)
		require.NoError(t, err)
		assert.False(t, has, "別プレイヤーのアイテムは検出しない")
	})
}

func TestPgShopRepository_FindPurchaseByToken_NotFound(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgShopRepository(sharedPg.Pool)
	ctx := context.Background()

	got, err := repo.FindPurchaseByToken(ctx, apishop.PlatformIOS, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}
