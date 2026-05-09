//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
)

// newFactionPurchaseEvents は faction_set 購入時に shop が enqueue する
// outbox events 2 行 (card-pack-purchased + faction-acquired) を作る。
func newFactionPurchaseEvents() []port.OutboxEvent {
	return []port.OutboxEvent{
		{
			EventID:   uuid.New(),
			EventType: apishop.EventTypeCardPackPurchased,
			Payload:   []byte(`{}`),
		},
		{
			EventID:   uuid.New(),
			EventType: apishop.EventTypeFactionAcquired,
			Payload:   []byte(`{}`),
		},
	}
}

const (
	factionTestUser1 = "11111111-1111-1111-1111-111111111111"
	factionTestUser2 = "22222222-2222-2222-2222-222222222222"
)

func newFactionTestPurchase(playerID string) *domain.OneTimePurchase {
	return &domain.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   "faction_tenki",
		PurchasedAt: time.Now().UTC(),
	}
}

type factionPurchaseSeed struct {
	playerID, faction, cardPackID, platform, token string
}

// TestFactionPurchaseRepository_CreatePurchase_Success は正常系の作成/べき等動作を確認する。
func TestFactionPurchaseRepository_CreatePurchase_Success(t *testing.T) {
	repo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	tests := []struct {
		name               string
		seeds              []factionPurchaseSeed
		playerID           string
		faction            string
		cardPackID         string
		platform           string
		token              string
		wantCreated        bool
		expectPurchaseRows int
		expectOwnedRows    int
		expectCardPackRows int
	}{
		{
			name:               "新規トークン: purchase + token + owned_faction + owned_card_pack 作成",
			seeds:              nil,
			playerID:           factionTestUser1,
			faction:            "Tenki",
			cardPackID:         "faction_set_tenki",
			platform:           domain.PlatformIOS,
			token:              "apple-new",
			wantCreated:        true,
			expectPurchaseRows: 1,
			expectOwnedRows:    1,
			expectCardPackRows: 1,
		},
		{
			name: "同一ユーザー既存トークンはべき等 (created=false、owned 系も追加されない)",
			seeds: []factionPurchaseSeed{
				{factionTestUser1, "Tenki", "faction_set_tenki", domain.PlatformIOS, "dup-token"},
			},
			playerID:           factionTestUser1,
			faction:            "Tenki",
			cardPackID:         "faction_set_tenki",
			platform:           domain.PlatformIOS,
			token:              "dup-token",
			wantCreated:        false,
			expectPurchaseRows: 1,
			expectOwnedRows:    1,
			expectCardPackRows: 1,
		},
		{
			name: "別ユーザー既存トークンもべき等 (first purchaseに紐付いたまま)",
			seeds: []factionPurchaseSeed{
				{factionTestUser1, "Tenki", "faction_set_tenki", domain.PlatformIOS, "shared-token"},
			},
			playerID:           factionTestUser2,
			faction:            "Tenki",
			cardPackID:         "faction_set_tenki",
			platform:           domain.PlatformIOS,
			token:              "shared-token",
			wantCreated:        false,
			expectPurchaseRows: 1,
			expectOwnedRows:    1,
			expectCardPackRows: 1,
		},
		{
			name: "異なるユーザーに同じfaction新規トークンで追加可",
			seeds: []factionPurchaseSeed{
				{factionTestUser1, "Tenki", "faction_set_tenki", domain.PlatformIOS, "tok-u1"},
			},
			playerID:           factionTestUser2,
			faction:            "Tenki",
			cardPackID:         "faction_set_tenki",
			platform:           domain.PlatformIOS,
			token:              "tok-u2",
			wantCreated:        true,
			expectPurchaseRows: 2,
			expectOwnedRows:    2,
			expectCardPackRows: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			for _, s := range tt.seeds {
				_, err := repo.CreatePurchase(ctx, newFactionTestPurchase(s.playerID), s.faction, s.cardPackID, s.platform, s.token, newFactionPurchaseEvents())
				require.NoError(t, err)
			}

			created, err := repo.CreatePurchase(ctx, newFactionTestPurchase(tt.playerID), tt.faction, tt.cardPackID, tt.platform, tt.token, newFactionPurchaseEvents())
			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)

			var purchases, owned, cardPacks int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.player_owned_factions`).Scan(&owned)) //nolint
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.player_owned_card_packs`).Scan(&cardPacks))
			assert.Equal(t, tt.expectPurchaseRows, purchases)
			assert.Equal(t, tt.expectOwnedRows, owned)
			assert.Equal(t, tt.expectCardPackRows, cardPacks)
		})
	}
}

// TestFactionPurchaseRepository_CreatePurchase_WrappedError は wrap された
// sentinel error (ErrorIs 判定) を返すケースを確認する。
func TestFactionPurchaseRepository_CreatePurchase_WrappedError(t *testing.T) {
	repo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	tests := []struct {
		name       string
		playerID   string
		faction    string
		cardPackID string
		platform   string
		token      string
		wantErrIs  error
	}{
		{
			name:       "unsupported platformはErrUnsupportedPlatform",
			playerID:   factionTestUser1,
			faction:    "Tenki",
			cardPackID: "faction_set_tenki",
			platform:   "windows",
			token:      "tok",
			wantErrIs:  port.ErrUnsupportedPlatform,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			_, err := repo.CreatePurchase(ctx, newFactionTestPurchase(tt.playerID), tt.faction, tt.cardPackID, tt.platform, tt.token, newFactionPurchaseEvents())
			assert.ErrorIs(t, err, tt.wantErrIs)
		})
	}
}

// TestFactionPurchaseRepository_CreatePurchase_DBError は DB 制約違反などで
// 一般的にエラーが返るケースを確認する (sentinel ではないので Error のみ判定)。
func TestFactionPurchaseRepository_CreatePurchase_DBError(t *testing.T) {
	repo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	tests := []struct {
		name       string
		playerID   string
		faction    string
		cardPackID string
		platform   string
		token      string
	}{
		{
			name:       "player_IDが空文字(UUID不正)はエラー",
			playerID:   "",
			faction:    "Tenki",
			cardPackID: "faction_set_tenki",
			platform:   domain.PlatformIOS,
			token:      "tok",
		},
		{
			name:       "不正なfaction文字列はCHECK制約でエラー",
			playerID:   factionTestUser1,
			faction:    "InvalidFaction",
			cardPackID: "faction_set_tenki",
			platform:   domain.PlatformIOS,
			token:      "tok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			_, err := repo.CreatePurchase(ctx, newFactionTestPurchase(tt.playerID), tt.faction, tt.cardPackID, tt.platform, tt.token, newFactionPurchaseEvents())
			assert.Error(t, err)
		})
	}
}

func TestFactionPurchaseRepository_CreatePurchase_AtomicRollback_OwnedFaction(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const playerID = "aaaaaaaa-1111-1111-1111-aaaaaaaaaaaa"
	seedOwnedFaction(t, playerID, "Tenki")

	purchase := &domain.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   "faction_tenki",
		PurchasedAt: time.Now().UTC(),
	}
	created, err := repo.CreatePurchase(ctx, purchase, "Tenki", "faction_set_tenki", domain.PlatformIOS, "new-token", newFactionPurchaseEvents())
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

func TestFactionPurchaseRepository_CreatePurchase_AtomicRollback_Token(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	const playerID = "bbbbbbbb-1111-1111-1111-bbbbbbbbbbbb"
	tooLongToken := strings.Repeat("x", 257)

	purchase := &domain.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   "faction_sugar",
		PurchasedAt: time.Now().UTC(),
	}
	created, err := repo.CreatePurchase(ctx, purchase, "Sugar", "faction_set_sugar", domain.PlatformIOS, tooLongToken, newFactionPurchaseEvents())
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

func TestFactionPurchaseRepository_ListOwnedFactions(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
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
