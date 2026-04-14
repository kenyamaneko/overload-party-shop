package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository"
)

func TestPgFactionPurchaseRepository_CreatePurchase(t *testing.T) {
	repo := repository.NewPgFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const (
		user1 = "11111111-1111-1111-1111-111111111111"
		user2 = "22222222-2222-2222-2222-222222222222"
	)

	newPurchase := func(playerID string) *apishop.OneTimePurchase {
		return &apishop.OneTimePurchase{
			PlayerID:    playerID,
			ProductID:   "faction_tenki",
			PurchasedAt: time.Now().UTC(),
		}
	}

	type seed struct {
		playerID, faction, platform, token string
	}

	tests := []struct {
		name               string
		seeds              []seed
		playerID           string
		faction            string
		platform           string
		token              string
		wantErr            bool
		wantErrIs          error
		wantCreated        bool
		expectPurchaseRows int
		expectOwnedRows    int
	}{
		{
			name:               "新規トークン: purchase + token + owned_faction作成",
			playerID:           user1,
			faction:            "Tenki",
			platform:           apishop.PlatformIOS,
			token:              "apple-new",
			wantCreated:        true,
			expectPurchaseRows: 1,
			expectOwnedRows:    1,
		},
		{
			name: "同一ユーザー既存トークンはべき等 (created=false, owned_factionも追加されない)",
			seeds: []seed{
				{user1, "Tenki", apishop.PlatformIOS, "dup-token"},
			},
			playerID:           user1,
			faction:            "Tenki",
			platform:           apishop.PlatformIOS,
			token:              "dup-token",
			wantCreated:        false,
			expectPurchaseRows: 1,
			expectOwnedRows:    1,
		},
		{
			name: "別ユーザー既存トークンもべき等 (first purchaseに紐付いたまま)",
			seeds: []seed{
				{user1, "Tenki", apishop.PlatformIOS, "shared-token"},
			},
			playerID:           user2,
			faction:            "Tenki",
			platform:           apishop.PlatformIOS,
			token:              "shared-token",
			wantCreated:        false,
			expectPurchaseRows: 1,
			expectOwnedRows:    1,
		},
		{
			name: "異なるユーザーに同じfaction新規トークンで追加可",
			seeds: []seed{
				{user1, "Tenki", apishop.PlatformIOS, "tok-u1"},
			},
			playerID:           user2,
			faction:            "Tenki",
			platform:           apishop.PlatformIOS,
			token:              "tok-u2",
			wantCreated:        true,
			expectPurchaseRows: 2,
			expectOwnedRows:    2,
		},
		{
			name:      "unsupported platformはErrUnsupportedPlatform",
			playerID:  user1,
			faction:   "Tenki",
			platform:  "windows",
			token:     "tok",
			wantErrIs: port.ErrUnsupportedPlatform,
		},
		{
			name:     "player_IDが空文字(UUID不正)はエラー",
			playerID: "",
			faction:  "Tenki",
			platform: apishop.PlatformIOS,
			token:    "tok",
			wantErr:  true,
		},
		{
			name:     "不正なfaction文字列はCHECK制約でエラー",
			playerID: user1,
			faction:  "InvalidFaction",
			platform: apishop.PlatformIOS,
			token:    "tok",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			for _, s := range tt.seeds {
				_, err := repo.CreatePurchase(ctx, newPurchase(s.playerID), s.faction, s.platform, s.token)
				require.NoError(t, err)
			}

			created, err := repo.CreatePurchase(ctx, newPurchase(tt.playerID), tt.faction, tt.platform, tt.token)

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

			var purchases, owned int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.player_owned_factions`).Scan(&owned)) //nolint
			assert.Equal(t, tt.expectPurchaseRows, purchases)
			assert.Equal(t, tt.expectOwnedRows, owned)
		})
	}
}

func TestPgFactionPurchaseRepository_CreatePurchase_AtomicRollback_OwnedFaction(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const playerID = "aaaaaaaa-1111-1111-1111-aaaaaaaaaaaa"
	seedOwnedFaction(t, playerID, "Tenki")

	purchase := &apishop.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   "faction_tenki",
		PurchasedAt: time.Now().UTC(),
	}
	created, err := repo.CreatePurchase(ctx, purchase, "Tenki", apishop.PlatformIOS, "new-token")
	require.Error(t, err, "player_owned_factions PK違反で失敗するはず")
	require.Contains(t, err.Error(), "insert owned faction",
		"owned_faction INSERTで落ちている (purchase/token INSERT後の失敗)")
	assert.False(t, created)

	var purchases, tokens, owned int
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.apple_purchase_tokens`).Scan(&tokens))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.player_owned_factions`).Scan(&owned)) //nolint

	assert.Equal(t, 0, purchases, "purchaseはrollbackされているはず")
	assert.Equal(t, 0, tokens, "tokenもrollbackされているはず")
	assert.Equal(t, 1, owned, "既存seed分のみ、新規INSERTは反映されない")
}

func TestPgFactionPurchaseRepository_CreatePurchase_AtomicRollback_Token(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const playerID = "bbbbbbbb-1111-1111-1111-bbbbbbbbbbbb"
	tooLongToken := strings.Repeat("x", 257)

	purchase := &apishop.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   "faction_sugar",
		PurchasedAt: time.Now().UTC(),
	}
	created, err := repo.CreatePurchase(ctx, purchase, "Sugar", apishop.PlatformIOS, tooLongToken)
	require.Error(t, err, "VARCHAR(256)超えでtoken INSERTが失敗するはず")
	require.Contains(t, err.Error(), "insert purchase token",
		"token INSERTで落ちている (purchase INSERT後の失敗)")
	assert.False(t, created)

	var purchases, tokens, owned int
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.apple_purchase_tokens`).Scan(&tokens))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.player_owned_factions`).Scan(&owned)) //nolint

	assert.Equal(t, 0, purchases)
	assert.Equal(t, 0, tokens)
	assert.Equal(t, 0, owned, "owned_factionは手前で短絡してINSERTされないはず")
}

func TestPgFactionPurchaseRepository_ListOwnedFactions(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const (
		userA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		userB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		userC = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)

	seedOwnedFaction(t, userA, "SHE")
	seedOwnedFaction(t, userB, "SHE")
	seedOwnedFaction(t, userB, "Tenki")

	tests := []struct {
		name     string
		playerID string
		want     []string
		wantErr  bool
	}{
		{name: "userAはSHEのみ取得", playerID: userA, want: []string{"SHE"}},
		{name: "userBはSHEとTenkiを取得", playerID: userB, want: []string{"SHE", "Tenki"}},
		{name: "所有行のないuserCは空", playerID: userC, want: nil},
		{name: "player_IDが空文字(UUID不正)はエラー", playerID: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.ListOwnedFactions(ctx, tt.playerID)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
