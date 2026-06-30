//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
)

func TestItemPurchaseRepository_CreatePurchase(t *testing.T) {
	repo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const (
		user1 = "11111111-2222-2222-2222-111111111111"
		user2 = "22222222-3333-3333-3333-222222222222"
	)

	newPurchase := func(playerID string) *domain.OneTimePurchase {
		return &domain.OneTimePurchase{
			PlayerID:    playerID,
			ProductID:   "playmat_01",
			PurchasedAt: time.Now().UTC(),
		}
	}
	newItem := func(playerID string, itemNo int64) *domain.PlayerItem {
		return &domain.PlayerItem{
			PlayerID:   playerID,
			ItemType:   domain.ItemTypePlaymat,
			ItemNo:     itemNo,
			AcquiredAt: time.Now().UTC(),
		}
	}

	type seed struct {
		playerID string
		itemNo   int64
		platform string
		token    string
	}

	assertRowCounts := func(t *testing.T, wantPurchases, wantItems int) {
		var purchases, items int
		require.NoError(t, sharedPg.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
		require.NoError(t, sharedPg.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM shop.player_items`).Scan(&items))
		assert.Equal(t, wantPurchases, purchases)
		assert.Equal(t, wantItems, items)
	}

	tests := []struct {
		name     string
		seeds    []seed
		playerID string
		itemNo   int64
		platform string
		token    string
		check    func(t *testing.T, created bool, err error)
	}{
		{
			name:     "新規トークン: purchase + token + item作成",
			playerID: user1,
			itemNo:   1,
			platform: domain.PlatformIOS,
			token:    "cosmetic-new",
			check: func(t *testing.T, created bool, err error) {
				require.NoError(t, err)
				assert.True(t, created)
				assertRowCounts(t, 1, 1)
			},
		},
		{
			name: "同一ユーザー既存トークンはべき等 (created=false, itemも追加されない)",
			seeds: []seed{
				{user1, 1, domain.PlatformIOS, "dup-token"},
			},
			playerID: user1,
			itemNo:   1,
			platform: domain.PlatformIOS,
			token:    "dup-token",
			check: func(t *testing.T, created bool, err error) {
				require.NoError(t, err)
				assert.False(t, created)
				assertRowCounts(t, 1, 1)
			},
		},
		{
			name: "別ユーザーに同itemは別tokenなら追加される",
			seeds: []seed{
				{user1, 1, domain.PlatformIOS, "tok-u1"},
			},
			playerID: user2,
			itemNo:   1,
			platform: domain.PlatformIOS,
			token:    "tok-u2",
			check: func(t *testing.T, created bool, err error) {
				require.NoError(t, err)
				assert.True(t, created)
				assertRowCounts(t, 2, 2)
			},
		},
		{
			name:     "unsupported platformはErrUnsupportedPlatform",
			playerID: user1,
			itemNo:   1,
			platform: "windows",
			token:    "tok",
			check: func(t *testing.T, _ bool, err error) {
				assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
			},
		},
		{
			name:     "player_IDが空文字(UUID不正)はエラー",
			playerID: "",
			itemNo:   1,
			platform: domain.PlatformIOS,
			token:    "tok",
			check: func(t *testing.T, _ bool, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			for _, s := range tt.seeds {
				_, err := repo.CreatePurchase(ctx, newPurchase(s.playerID), newItem(s.playerID, s.itemNo), s.platform, s.token)
				require.NoError(t, err)
			}

			created, err := repo.CreatePurchase(ctx, newPurchase(tt.playerID), newItem(tt.playerID, tt.itemNo), tt.platform, tt.token)
			tt.check(t, created, err)
		})
	}
}

func TestItemPurchaseRepository_CreatePurchase_AtomicRollback_PlayerItem(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const playerID = "99999999-9999-9999-9999-999999999999"
	seedPlayerItem(t, playerID, domain.ItemTypePlaymat, 1, time.Now().UTC())

	created, err := repo.CreatePurchase(ctx,
		&domain.OneTimePurchase{PlayerID: playerID, ProductID: "playmat_01", PurchasedAt: time.Now().UTC()},
		&domain.PlayerItem{PlayerID: playerID, ItemType: domain.ItemTypePlaymat, ItemNo: 1, AcquiredAt: time.Now().UTC()},
		domain.PlatformIOS, "atomic-rollback-token")
	require.Error(t, err, "player_items PK違反で失敗するはず")
	require.Contains(t, err.Error(), "insert player item",
		"player_item INSERTで落ちている (purchase/token INSERT後の失敗)")
	assert.False(t, created)

	var purchases, tokens, items int
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.apple_purchase_tokens`).Scan(&tokens))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.player_items`).Scan(&items))

	assert.Equal(t, 0, purchases)
	assert.Equal(t, 0, tokens)
	assert.Equal(t, 1, items, "既存seed分のみ残る")
}

func TestItemPurchaseRepository_CreatePurchase_AtomicRollback_Token(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const playerID = "88888888-8888-8888-8888-888888888888"
	tooLongToken := strings.Repeat("y", 257)

	created, err := repo.CreatePurchase(ctx,
		&domain.OneTimePurchase{PlayerID: playerID, ProductID: "sleeve_01", PurchasedAt: time.Now().UTC()},
		&domain.PlayerItem{PlayerID: playerID, ItemType: domain.ItemTypeSleeve, ItemNo: 1, AcquiredAt: time.Now().UTC()},
		domain.PlatformIOS, tooLongToken)
	require.Error(t, err, "VARCHAR(256)超えでtoken INSERTが失敗するはず")
	require.Contains(t, err.Error(), "insert purchase token")
	assert.False(t, created)

	var purchases, tokens, items int
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.apple_purchase_tokens`).Scan(&tokens))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.player_items`).Scan(&items))

	assert.Equal(t, 0, purchases)
	assert.Equal(t, 0, tokens)
	assert.Equal(t, 0, items)
}

func TestItemPurchaseRepository_ListPlayerItems(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const (
		userA = "aaaaaaaa-2222-2222-2222-aaaaaaaaaaaa"
		userB = "bbbbbbbb-2222-2222-2222-bbbbbbbbbbbb"
		userC = "cccccccc-2222-2222-2222-cccccccccccc"
	)

	now := time.Now().UTC()
	seedPlayerItem(t, userA, domain.ItemTypePlaymat, 1, now)
	seedPlayerItem(t, userA, domain.ItemTypeSleeve, 2, now)
	seedPlayerItem(t, userB, domain.ItemTypeIcon, 9, now)

	tests := []struct {
		name     string
		playerID string
		check    func(t *testing.T, got []*domain.PlayerItem, err error)
	}{
		{
			name:     "userAは2件取得",
			playerID: userA,
			check: func(t *testing.T, got []*domain.PlayerItem, err error) {
				require.NoError(t, err)
				assert.Len(t, got, 2)
			},
		},
		{
			name:     "userBは1件取得",
			playerID: userB,
			check: func(t *testing.T, got []*domain.PlayerItem, err error) {
				require.NoError(t, err)
				assert.Len(t, got, 1)
			},
		},
		{
			name:     "所有行のないuserCは空",
			playerID: userC,
			check: func(t *testing.T, got []*domain.PlayerItem, err error) {
				require.NoError(t, err)
				assert.Empty(t, got)
			},
		},
		{
			name:     "player_IDが空文字(UUID不正)はエラー",
			playerID: "",
			check: func(t *testing.T, _ []*domain.PlayerItem, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.ListPlayerItems(ctx, tt.playerID)
			tt.check(t, got, err)
		})
	}
}

func TestItemPurchaseRepository_HasPlayerItem(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const (
		userA = "aaaaaaaa-3333-3333-3333-aaaaaaaaaaaa"
		userB = "bbbbbbbb-3333-3333-3333-bbbbbbbbbbbb"
	)

	now := time.Now().UTC()
	seedPlayerItem(t, userA, domain.ItemTypePlaymat, 1, now)
	seedPlayerItem(t, userB, domain.ItemTypeIcon, 9, now)

	tests := []struct {
		name     string
		playerID string
		itemType string
		itemNo   int64
		check    func(t *testing.T, got bool, err error)
	}{
		{
			name:     "所有itemはtrue",
			playerID: userA,
			itemType: domain.ItemTypePlaymat,
			itemNo:   1,
			check: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.True(t, got)
			},
		},
		{
			name:     "別itemNoはfalse",
			playerID: userA,
			itemType: domain.ItemTypePlaymat,
			itemNo:   999,
			check: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.False(t, got)
			},
		},
		{
			name:     "別item_typeはfalse",
			playerID: userA,
			itemType: domain.ItemTypeSleeve,
			itemNo:   1,
			check: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.False(t, got)
			},
		},
		{
			name:     "別ユーザーの所有は検出しない",
			playerID: userA,
			itemType: domain.ItemTypeIcon,
			itemNo:   9,
			check: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.False(t, got)
			},
		},
		{
			name:     "player_IDが空文字(UUID不正)はエラー",
			playerID: "",
			itemType: domain.ItemTypePlaymat,
			itemNo:   1,
			check: func(t *testing.T, _ bool, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.HasPlayerItem(ctx, tt.playerID, tt.itemType, tt.itemNo)
			tt.check(t, got, err)
		})
	}
}
