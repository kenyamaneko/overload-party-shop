//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
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

	newPurchase := func(playerID string) *apishop.OneTimePurchase {
		return &apishop.OneTimePurchase{
			PlayerID:    playerID,
			ProductID:   "playmat_01",
			PurchasedAt: time.Now().UTC(),
		}
	}
	newItem := func(playerID string, itemNo int64) *apishop.PlayerItem {
		return &apishop.PlayerItem{
			PlayerID:   playerID,
			ItemType:   apishop.ItemTypePlaymat,
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

	tests := []struct {
		name               string
		seeds              []seed
		playerID           string
		itemNo             int64
		platform           string
		token              string
		wantErr            bool
		wantErrIs          error
		wantCreated        bool
		expectPurchaseRows int
		expectItemRows     int
	}{
		{
			name:               "新規トークン: purchase + token + item作成",
			playerID:           user1,
			itemNo:             1,
			platform:           apishop.PlatformIOS,
			token:              "cosmetic-new",
			wantCreated:        true,
			expectPurchaseRows: 1,
			expectItemRows:     1,
		},
		{
			name: "同一ユーザー既存トークンはべき等 (created=false, itemも追加されない)",
			seeds: []seed{
				{user1, 1, apishop.PlatformIOS, "dup-token"},
			},
			playerID:           user1,
			itemNo:             1,
			platform:           apishop.PlatformIOS,
			token:              "dup-token",
			wantCreated:        false,
			expectPurchaseRows: 1,
			expectItemRows:     1,
		},
		{
			name: "別ユーザーに同itemは別tokenなら追加される",
			seeds: []seed{
				{user1, 1, apishop.PlatformIOS, "tok-u1"},
			},
			playerID:           user2,
			itemNo:             1,
			platform:           apishop.PlatformIOS,
			token:              "tok-u2",
			wantCreated:        true,
			expectPurchaseRows: 2,
			expectItemRows:     2,
		},
		{
			name:      "unsupported platformはErrUnsupportedPlatform",
			playerID:  user1,
			itemNo:    1,
			platform:  "windows",
			token:     "tok",
			wantErrIs: port.ErrUnsupportedPlatform,
		},
		{
			name:     "player_IDが空文字(UUID不正)はエラー",
			playerID: "",
			itemNo:   1,
			platform: apishop.PlatformIOS,
			token:    "tok",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			for _, s := range tt.seeds {
				_, err := repo.CreatePurchase(ctx, newPurchase(s.playerID), newItem(s.playerID, s.itemNo), s.platform, s.token, port.OutboxEvent{})
				require.NoError(t, err)
			}

			created, err := repo.CreatePurchase(ctx, newPurchase(tt.playerID), newItem(tt.playerID, tt.itemNo), tt.platform, tt.token, port.OutboxEvent{})

			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
				return
			}
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)

			var purchases, items int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.player_items`).Scan(&items))
			assert.Equal(t, tt.expectPurchaseRows, purchases)
			assert.Equal(t, tt.expectItemRows, items)
		})
	}
}

func TestItemPurchaseRepository_CreatePurchase_AtomicRollback_PlayerItem(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const playerID = "99999999-9999-9999-9999-999999999999"
	seedPlayerItem(t, playerID, apishop.ItemTypePlaymat, 1, time.Now().UTC())

	created, err := repo.CreatePurchase(ctx,
		&apishop.OneTimePurchase{PlayerID: playerID, ProductID: "playmat_01", PurchasedAt: time.Now().UTC()},
		&apishop.PlayerItem{PlayerID: playerID, ItemType: apishop.ItemTypePlaymat, ItemNo: 1, AcquiredAt: time.Now().UTC()},
		apishop.PlatformIOS, "atomic-rollback-token", port.OutboxEvent{})
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
		&apishop.OneTimePurchase{PlayerID: playerID, ProductID: "sleeve_01", PurchasedAt: time.Now().UTC()},
		&apishop.PlayerItem{PlayerID: playerID, ItemType: apishop.ItemTypeSleeve, ItemNo: 1, AcquiredAt: time.Now().UTC()},
		apishop.PlatformIOS, tooLongToken, port.OutboxEvent{})
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
	seedPlayerItem(t, userA, apishop.ItemTypePlaymat, 1, now)
	seedPlayerItem(t, userA, apishop.ItemTypeSleeve, 2, now)
	seedPlayerItem(t, userB, apishop.ItemTypeIcon, 9, now)

	tests := []struct {
		name     string
		playerID string
		wantLen  int
		wantErr  bool
	}{
		{name: "userAは2件取得", playerID: userA, wantLen: 2},
		{name: "userBは1件取得", playerID: userB, wantLen: 1},
		{name: "所有行のないuserCは空", playerID: userC, wantLen: 0},
		{name: "player_IDが空文字(UUID不正)はエラー", playerID: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.ListPlayerItems(ctx, tt.playerID)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
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
	seedPlayerItem(t, userA, apishop.ItemTypePlaymat, 1, now)
	seedPlayerItem(t, userB, apishop.ItemTypeIcon, 9, now)

	tests := []struct {
		name     string
		playerID string
		itemType string
		itemNo   int64
		want     bool
		wantErr  bool
	}{
		{name: "所有itemはtrue", playerID: userA, itemType: apishop.ItemTypePlaymat, itemNo: 1, want: true},
		{name: "別itemNoはfalse", playerID: userA, itemType: apishop.ItemTypePlaymat, itemNo: 999, want: false},
		{name: "別item_typeはfalse", playerID: userA, itemType: apishop.ItemTypeSleeve, itemNo: 1, want: false},
		{name: "別ユーザーの所有は検出しない", playerID: userA, itemType: apishop.ItemTypeIcon, itemNo: 9, want: false},
		{name: "player_IDが空文字(UUID不正)はエラー", playerID: "", itemType: apishop.ItemTypePlaymat, itemNo: 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.HasPlayerItem(ctx, tt.playerID, tt.itemType, tt.itemNo)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
