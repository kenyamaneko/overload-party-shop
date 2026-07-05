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

	assertRowCounts := func(t *testing.T, wantPurchases, wantItems int) {
		var purchases, items int
		require.NoError(t, sharedPg.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
		require.NoError(t, sharedPg.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM shop.player_items`).Scan(&items))
		assert.Equal(t, wantPurchases, purchases)
		assert.Equal(t, wantItems, items)
	}

	type seed struct {
		playerID string
		itemNo   int64
		platform string
		token    string
	}

	t.Run("item 購入の作成", func(t *testing.T) {
		successCases := []struct {
			name          string
			seeds         []seed
			playerID      string
			itemNo        int64
			platform      string
			token         string
			wantCreated   bool
			wantPurchases int
			wantItems     int
		}{
			{
				name:          "新規トークンのとき、purchase / token / item が作成される",
				playerID:      user1,
				itemNo:        1,
				platform:      domain.PlatformIOS,
				token:         "cosmetic-new",
				wantCreated:   true,
				wantPurchases: 1,
				wantItems:     1,
			},
			{
				name: "同一ユーザーの既存トークンのとき、べき等で created=false、item も増えない",
				seeds: []seed{
					{user1, 1, domain.PlatformIOS, "dup-token"},
				},
				playerID:      user1,
				itemNo:        1,
				platform:      domain.PlatformIOS,
				token:         "dup-token",
				wantCreated:   false,
				wantPurchases: 1,
				wantItems:     1,
			},
			{
				name: "別ユーザーが同一 item を別トークンで買うとき、追加される",
				seeds: []seed{
					{user1, 1, domain.PlatformIOS, "tok-u1"},
				},
				playerID:      user2,
				itemNo:        1,
				platform:      domain.PlatformIOS,
				token:         "tok-u2",
				wantCreated:   true,
				wantPurchases: 2,
				wantItems:     2,
			},
		}

		for _, tt := range successCases {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				for _, s := range tt.seeds {
					_, err := repo.CreatePurchase(ctx, newPurchase(s.playerID), newItem(s.playerID, s.itemNo), s.platform, s.token)
					require.NoError(t, err)
				}

				created, err := repo.CreatePurchase(ctx, newPurchase(tt.playerID), newItem(tt.playerID, tt.itemNo), tt.platform, tt.token)
				require.NoError(t, err)
				assert.Equal(t, tt.wantCreated, created)
				assertRowCounts(t, tt.wantPurchases, tt.wantItems)
			})
		}

		t.Run("unsupported platform のとき、ErrUnsupportedPlatform になる", func(t *testing.T) {
			sharedPg.Truncate(t)
			_, err := repo.CreatePurchase(ctx, newPurchase(user1), newItem(user1, 1), "windows", "tok")
			assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
		})

		t.Run("player_id が空文字 (UUID 不正) のとき、エラーになる", func(t *testing.T) {
			sharedPg.Truncate(t)
			_, err := repo.CreatePurchase(ctx, newPurchase(""), newItem("", 1), domain.PlatformIOS, "tok")
			assert.Error(t, err)
		})

		t.Run("player_item の INSERT が PK 違反で失敗するとき、purchase と token が rollback される", func(t *testing.T) {
			sharedPg.Truncate(t)
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
		})

		t.Run("token が VARCHAR(256) 超過で失敗するとき、purchase / token / item が作成されない", func(t *testing.T) {
			sharedPg.Truncate(t)
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
		})
	})
}

func TestItemPurchaseRepository_ListPlayerItems(t *testing.T) {
	repo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("所有 item 一覧の取得", func(t *testing.T) {
		const (
			userA = "aaaaaaaa-2222-2222-2222-aaaaaaaaaaaa"
			userB = "bbbbbbbb-2222-2222-2222-bbbbbbbbbbbb"
			userC = "cccccccc-2222-2222-2222-cccccccccccc"
		)

		sharedPg.Truncate(t)
		now := time.Now().UTC()
		seedPlayerItem(t, userA, domain.ItemTypePlaymat, 1, now)
		seedPlayerItem(t, userA, domain.ItemTypeSleeve, 2, now)
		seedPlayerItem(t, userB, domain.ItemTypeIcon, 9, now)

		tests := []struct {
			name      string
			playerID  string
			wantCount int
		}{
			{
				name:      "userA が 2 件所有するとき、2 件返す",
				playerID:  userA,
				wantCount: 2,
			},
			{
				name:      "userB が 1 件所有するとき、1 件返す",
				playerID:  userB,
				wantCount: 1,
			},
			{
				name:      "所有行の無い userC のとき、空を返す",
				playerID:  userC,
				wantCount: 0,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := repo.ListPlayerItems(ctx, tt.playerID)
				require.NoError(t, err)
				assert.Len(t, got, tt.wantCount)
			})
		}

		t.Run("player_id が空文字 (UUID 不正) のとき、エラーになる", func(t *testing.T) {
			_, err := repo.ListPlayerItems(ctx, "")
			assert.Error(t, err)
		})
	})
}

func TestItemPurchaseRepository_HasPlayerItem(t *testing.T) {
	repo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("item 所有判定", func(t *testing.T) {
		const (
			userA = "aaaaaaaa-3333-3333-3333-aaaaaaaaaaaa"
			userB = "bbbbbbbb-3333-3333-3333-bbbbbbbbbbbb"
		)

		sharedPg.Truncate(t)
		now := time.Now().UTC()
		seedPlayerItem(t, userA, domain.ItemTypePlaymat, 1, now)
		seedPlayerItem(t, userB, domain.ItemTypeIcon, 9, now)

		tests := []struct {
			name     string
			playerID string
			itemType string
			itemNo   int64
			want     bool
		}{
			{
				name:     "所有 item のとき、true になる",
				playerID: userA,
				itemType: domain.ItemTypePlaymat,
				itemNo:   1,
				want:     true,
			},
			{
				name:     "別 item_no のとき、false になる",
				playerID: userA,
				itemType: domain.ItemTypePlaymat,
				itemNo:   999,
				want:     false,
			},
			{
				name:     "別 item_type のとき、false になる",
				playerID: userA,
				itemType: domain.ItemTypeSleeve,
				itemNo:   1,
				want:     false,
			},
			{
				name:     "別ユーザーの所有は検出せず、false になる",
				playerID: userA,
				itemType: domain.ItemTypeIcon,
				itemNo:   9,
				want:     false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := repo.HasPlayerItem(ctx, tt.playerID, tt.itemType, tt.itemNo)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			})
		}

		t.Run("player_id が空文字 (UUID 不正) のとき、エラーになる", func(t *testing.T) {
			_, err := repo.HasPlayerItem(ctx, "", domain.ItemTypePlaymat, 1)
			assert.Error(t, err)
		})
	})
}
