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

const (
	cardPackTestUser1 = "31111111-1111-1111-1111-111111111111"
	cardPackTestUser2 = "32222222-2222-2222-2222-222222222222"
)

func newCardPackPurchaseEvent() port.OutboxEvent {
	return port.OutboxEvent{
		EventID:   uuid.New(),
		EventType: apishop.EventTypeCardPackPurchased,
		Payload:   []byte(`{}`),
	}
}

func newCardPackTestPurchase(playerID string) *domain.OneTimePurchase {
	return &domain.OneTimePurchase{
		PlayerID:    playerID,
		ProductID:   "limited_2026_summer",
		PurchasedAt: time.Now().UTC(),
	}
}

type cardPackPurchaseSeed struct {
	playerID, cardPackID, platform, token string
}

func TestCardPackPurchaseRepository_CreatePurchase(t *testing.T) {
	repo := postgres.NewCardPackPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("card_pack 購入の作成", func(t *testing.T) {
		successCases := []struct {
			name              string
			seeds             []cardPackPurchaseSeed
			playerID          string
			cardPackID        string
			platform          string
			token             string
			wantCreated       bool
			wantPurchaseRows  int
			wantTokenRows     int
			wantOwnedPackRows int
			wantOutboxRows    int
		}{
			{
				name:              "新規トークンのとき、purchase / token / owned_card_pack / outbox が作成される",
				seeds:             nil,
				playerID:          cardPackTestUser1,
				cardPackID:        "limited_2026_summer",
				platform:          domain.PlatformIOS,
				token:             "apple-new-pack",
				wantCreated:       true,
				wantPurchaseRows:  1,
				wantTokenRows:     1,
				wantOwnedPackRows: 1,
				wantOutboxRows:    1,
			},
			{
				name: "同一トークンで再作成すると、べき等で created=false になり行が増えない",
				seeds: []cardPackPurchaseSeed{
					{cardPackTestUser1, "limited_2026_summer", domain.PlatformIOS, "dup-pack-token"},
				},
				playerID:          cardPackTestUser1,
				cardPackID:        "limited_2026_summer",
				platform:          domain.PlatformIOS,
				token:             "dup-pack-token",
				wantCreated:       false,
				wantPurchaseRows:  1,
				wantTokenRows:     1,
				wantOwnedPackRows: 1,
				wantOutboxRows:    1,
			},
			{
				name: "別プレイヤーが同一 pack を新規トークンで買うとき、独立して追加される",
				seeds: []cardPackPurchaseSeed{
					{cardPackTestUser1, "limited_2026_summer", domain.PlatformIOS, "tok-u1-pack"},
				},
				playerID:          cardPackTestUser2,
				cardPackID:        "limited_2026_summer",
				platform:          domain.PlatformIOS,
				token:             "tok-u2-pack",
				wantCreated:       true,
				wantPurchaseRows:  2,
				wantTokenRows:     2,
				wantOwnedPackRows: 2,
				wantOutboxRows:    2,
			},
		}

		for _, tt := range successCases {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				for _, s := range tt.seeds {
					_, err := repo.CreatePurchase(ctx, newCardPackTestPurchase(s.playerID), s.cardPackID, s.platform, s.token, newCardPackPurchaseEvent())
					require.NoError(t, err)
				}

				created, err := repo.CreatePurchase(ctx, newCardPackTestPurchase(tt.playerID), tt.cardPackID, tt.platform, tt.token, newCardPackPurchaseEvent())
				require.NoError(t, err)
				assert.Equal(t, tt.wantCreated, created)

				var purchases, tokens, ownedPacks, outboxRows int
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM shop.apple_purchase_tokens`).Scan(&tokens))
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM shop.player_owned_card_packs`).Scan(&ownedPacks))
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM shop.outbox_events`).Scan(&outboxRows))
				assert.Equal(t, tt.wantPurchaseRows, purchases)
				assert.Equal(t, tt.wantTokenRows, tokens)
				assert.Equal(t, tt.wantOwnedPackRows, ownedPacks)
				assert.Equal(t, tt.wantOutboxRows, outboxRows)
			})
		}

		t.Run("unsupported platform (windows) のとき、ErrUnsupportedPlatform になる", func(t *testing.T) {
			sharedPg.Truncate(t)
			_, err := repo.CreatePurchase(ctx, newCardPackTestPurchase(cardPackTestUser1), "limited_2026_summer", "windows", "tok", newCardPackPurchaseEvent())
			assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
		})

		t.Run("token が VARCHAR(256) 超過 (257 文字) で失敗するとき、purchase が rollback され owned_card_pack は作成されない", func(t *testing.T) {
			sharedPg.Truncate(t)
			tooLongToken := strings.Repeat("x", 257)

			created, err := repo.CreatePurchase(ctx, newCardPackTestPurchase(cardPackTestUser1), "limited_2026_summer", domain.PlatformIOS, tooLongToken, newCardPackPurchaseEvent())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "insert purchase token")
			assert.False(t, created)

			var purchases, tokens, ownedPacks int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.apple_purchase_tokens`).Scan(&tokens))
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.player_owned_card_packs`).Scan(&ownedPacks))
			assert.Equal(t, 0, purchases)
			assert.Equal(t, 0, tokens)
			assert.Equal(t, 0, ownedPacks)
		})

		t.Run("owned_card_pack の INSERT が PK 違反で失敗するとき、purchase と token が rollback される", func(t *testing.T) {
			sharedPg.Truncate(t)
			_, err := sharedPg.Pool.Exec(ctx,
				`INSERT INTO shop.player_owned_card_packs (player_id, card_pack_id) VALUES ($1, $2)`,
				cardPackTestUser1, "limited_2026_summer")
			require.NoError(t, err)

			created, err := repo.CreatePurchase(ctx, newCardPackTestPurchase(cardPackTestUser1), "limited_2026_summer", domain.PlatformIOS, "new-pack-token", newCardPackPurchaseEvent())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "insert owned card pack")
			assert.False(t, created)

			var purchases, tokens, ownedPacks int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.one_time_purchases`).Scan(&purchases))
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.apple_purchase_tokens`).Scan(&tokens))
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.player_owned_card_packs`).Scan(&ownedPacks))
			assert.Equal(t, 0, purchases)
			assert.Equal(t, 0, tokens)
			assert.Equal(t, 1, ownedPacks, "既存 seed 分のみ、新規 INSERT は反映されない")
		})
	})
}

func TestCardPackPurchaseRepository_HasPlayerCardPack(t *testing.T) {
	repo := postgres.NewCardPackPurchaseRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("card_pack 所有判定", func(t *testing.T) {
		const (
			owningPlayer = "33333333-3333-3333-3333-333333333333"
			otherPlayer  = "34444444-4444-4444-4444-444444444444"
			ownedPack    = "limited_2026_summer"
			otherPack    = "limited_2026_winter"
		)

		tests := []struct {
			name        string
			seed        func(t *testing.T)
			playerID    string
			cardPackID  string
			wantIsOwned bool
		}{
			{
				name: "所有プレイヤー・所有 pack のとき、所有ありになる",
				seed: func(t *testing.T) {
					_, err := sharedPg.Pool.Exec(ctx,
						`INSERT INTO shop.player_owned_card_packs (player_id, card_pack_id) VALUES ($1, $2)`,
						owningPlayer, ownedPack)
					require.NoError(t, err)
				},
				playerID:    owningPlayer,
				cardPackID:  ownedPack,
				wantIsOwned: true,
			},
			{
				name:        "所有行が無いとき、所有なしになる",
				seed:        func(t *testing.T) {},
				playerID:    owningPlayer,
				cardPackID:  ownedPack,
				wantIsOwned: false,
			},
			{
				name: "別プレイヤーの所有行しか無いとき、所有なしになる",
				seed: func(t *testing.T) {
					_, err := sharedPg.Pool.Exec(ctx,
						`INSERT INTO shop.player_owned_card_packs (player_id, card_pack_id) VALUES ($1, $2)`,
						otherPlayer, ownedPack)
					require.NoError(t, err)
				},
				playerID:    owningPlayer,
				cardPackID:  ownedPack,
				wantIsOwned: false,
			},
			{
				name: "別 pack の所有行しか無いとき、所有なしになる",
				seed: func(t *testing.T) {
					_, err := sharedPg.Pool.Exec(ctx,
						`INSERT INTO shop.player_owned_card_packs (player_id, card_pack_id) VALUES ($1, $2)`,
						owningPlayer, otherPack)
					require.NoError(t, err)
				},
				playerID:    owningPlayer,
				cardPackID:  ownedPack,
				wantIsOwned: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				tt.seed(t)

				got, err := repo.HasPlayerCardPack(ctx, tt.playerID, tt.cardPackID)
				require.NoError(t, err)
				assert.Equal(t, tt.wantIsOwned, got)
			})
		}
	})
}
