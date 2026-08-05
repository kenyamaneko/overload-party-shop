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
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
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

func TestFactionPurchaseRepository_CreatePurchase(t *testing.T) {
	repo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("faction購入の作成", func(t *testing.T) {
		successCases := []struct {
			name             string
			seeds            []factionPurchaseSeed
			playerID         string
			faction          string
			cardPackID       string
			platform         string
			token            string
			wantCreated      bool
			wantPurchaseRows int
			wantOwnedRows    int
			wantCardPackRows int
		}{
			{
				name:             "新規トークンのとき、purchase / token / owned_faction / owned_card_packが作成される",
				seeds:            nil,
				playerID:         factionTestUser1,
				faction:          "Tenki",
				cardPackID:       "faction_set_tenki",
				platform:         domain.PlatformIOS,
				token:            "apple-new",
				wantCreated:      true,
				wantPurchaseRows: 1,
				wantOwnedRows:    1,
				wantCardPackRows: 1,
			},
			{
				name: "同一ユーザーの既存トークンのとき、べき等でcreated=false、owned系も増えない",
				seeds: []factionPurchaseSeed{
					{factionTestUser1, "Tenki", "faction_set_tenki", domain.PlatformIOS, "dup-token"},
				},
				playerID:         factionTestUser1,
				faction:          "Tenki",
				cardPackID:       "faction_set_tenki",
				platform:         domain.PlatformIOS,
				token:            "dup-token",
				wantCreated:      false,
				wantPurchaseRows: 1,
				wantOwnedRows:    1,
				wantCardPackRows: 1,
			},
			{
				name: "別ユーザーが同一トークンを使うとき、べき等で最初のpurchaseに紐付いたまま",
				seeds: []factionPurchaseSeed{
					{factionTestUser1, "Tenki", "faction_set_tenki", domain.PlatformIOS, "shared-token"},
				},
				playerID:         factionTestUser2,
				faction:          "Tenki",
				cardPackID:       "faction_set_tenki",
				platform:         domain.PlatformIOS,
				token:            "shared-token",
				wantCreated:      false,
				wantPurchaseRows: 1,
				wantOwnedRows:    1,
				wantCardPackRows: 1,
			},
			{
				name: "別ユーザーが同一factionを新規トークンで買うとき、独立して追加される",
				seeds: []factionPurchaseSeed{
					{factionTestUser1, "Tenki", "faction_set_tenki", domain.PlatformIOS, "tok-u1"},
				},
				playerID:         factionTestUser2,
				faction:          "Tenki",
				cardPackID:       "faction_set_tenki",
				platform:         domain.PlatformIOS,
				token:            "tok-u2",
				wantCreated:      true,
				wantPurchaseRows: 2,
				wantOwnedRows:    2,
				wantCardPackRows: 2,
			},
		}

		for _, tt := range successCases {
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
				assert.Equal(t, tt.wantPurchaseRows, purchases)
				assert.Equal(t, tt.wantOwnedRows, owned)
				assert.Equal(t, tt.wantCardPackRows, cardPacks)
			})
		}

		invalidInputCases := []struct {
			name     string
			playerID string
			faction  string
		}{
			{
				name:     "player_idが空文字 (UUID不正)のとき、エラーになる",
				playerID: "",
				faction:  "Tenki",
			},
			{
				name:     "不正なfaction文字列のとき、CHECK制約でエラーになる",
				playerID: factionTestUser1,
				faction:  "InvalidFaction",
			},
		}

		for _, tt := range invalidInputCases {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				_, err := repo.CreatePurchase(ctx, newFactionTestPurchase(tt.playerID), tt.faction, "faction_set_tenki", domain.PlatformIOS, "tok", newFactionPurchaseEvents())
				assert.Error(t, err)
			})
		}

		t.Run("unsupported platformのとき、ErrUnsupportedPlatformになる", func(t *testing.T) {
			sharedPg.Truncate(t)
			_, err := repo.CreatePurchase(ctx, newFactionTestPurchase(factionTestUser1), "Tenki", "faction_set_tenki", "windows", "tok", newFactionPurchaseEvents())
			assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
		})

		t.Run("owned_factionのINSERTがPK違反で失敗するとき、purchaseとtokenがrollbackされる", func(t *testing.T) {
			sharedPg.Truncate(t)
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
		})

		t.Run("tokenがVARCHAR(256)超過で失敗するとき、purchaseがrollbackされowned_factionはINSERTされない", func(t *testing.T) {
			sharedPg.Truncate(t)
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
		})
	})
}

func TestFactionPurchaseRepository_ListOwnedFactions(t *testing.T) {
	repo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("所有faction一覧の取得", func(t *testing.T) {
		const (
			userA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
			userB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
			userC = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		)

		sharedPg.Truncate(t)
		seedOwnedFaction(t, userA, "SHE")
		seedOwnedFaction(t, userB, "SHE")
		seedOwnedFaction(t, userB, "Tenki")

		tests := []struct {
			name     string
			playerID string
			want     []string
		}{
			{
				name:     "userAがSHEのみ所有するとき、SHEを返す",
				playerID: userA,
				want:     []string{"SHE"},
			},
			{
				name:     "userBがSHEとTenkiを所有するとき、両方を返す",
				playerID: userB,
				want:     []string{"SHE", "Tenki"},
			},
			{
				name:     "所有行の無いuserCのとき、空を返す",
				playerID: userC,
				want:     nil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := repo.ListOwnedFactions(ctx, tt.playerID)
				require.NoError(t, err)
				assert.ElementsMatch(t, tt.want, got)
			})
		}

		t.Run("player_idが空文字 (UUID不正)のとき、エラーになる", func(t *testing.T) {
			_, err := repo.ListOwnedFactions(ctx, "")
			assert.Error(t, err)
		})
	})
}
