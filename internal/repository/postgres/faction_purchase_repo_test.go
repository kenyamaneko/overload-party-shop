//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

func failPurchaseOfOwnedFaction(t *testing.T) error {
	t.Helper()
	pg.Truncate(t)
	require.NoError(t, pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne))
	return pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenFailed)
}

func failPurchaseWithOnlyCardPackOwned(t *testing.T) error {
	t.Helper()
	pg.Truncate(t)
	require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackTenki, tokenOne))
	return pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenFailed)
}

func failPurchaseWithDuplicateEventID(t *testing.T) error {
	t.Helper()
	pg.Truncate(t)
	eventID := uuid.New()
	events := []port.OutboxEvent{postgrestest.NewOutboxEventWithID(eventID), postgrestest.NewOutboxEventWithID(eventID)}
	return pg.CreateFactionPurchaseWithEvents(playerA, "Tenki", cardPackTenki, tokenFailed, events)
}

func failPurchaseWithTooLongToken(t *testing.T) error {
	t.Helper()
	pg.Truncate(t)
	return pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, postgrestest.TokenOfLength(257))
}

func TestFactionPurchaseCreatePurchase(t *testing.T) {
	ctx := context.Background()

	t.Run("[陣営購入]購入の作成", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAが新規の購入トークンで陣営Tenkiのセットを購入する", func(t *testing.T) {
				t.Run("プレイヤーAの所有陣営一覧にTenkiが含まれる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne))

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Contains(t, factions, "Tenki")
				})

				t.Run("プレイヤーAが陣営Tenkiのセットのカードパックを所有している状態になる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne))

					owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackTenki)

					require.NoError(t, err)
					assert.True(t, owned)
				})

				t.Run("購入トークンで検索すると、プレイヤーAの購入が見つかる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerA, got.PlayerID)
				})

				t.Run("取得したイベントは、購入時に渡した2件のイベントのイベントIDになる", func(t *testing.T) {
					pg.Truncate(t)
					events := []port.OutboxEvent{postgrestest.NewOutboxEvent(), postgrestest.NewOutboxEvent()}
					require.NoError(t, pg.CreateFactionPurchaseWithEvents(playerA, "Tenki", cardPackTenki, tokenOne, events))

					claimed := pg.ClaimAll(t)

					assert.ElementsMatch(t, []uuid.UUID{events[0].EventID, events[1].EventID}, postgrestest.ClaimedEventIDs(claimed))
				})
			})

			t.Run("プレイヤーAが陣営Tenkiを購入済みのとき、別の購入トークンで陣営Sugarのセットを購入する", func(t *testing.T) {
				t.Run("プレイヤーAの所有陣営一覧にTenkiとSugarの両方が含まれる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne))
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Sugar", cardPackSugar, tokenTwo))

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.ElementsMatch(t, []string{"Tenki", "Sugar"}, factions)
				})
			})

			t.Run("プレイヤーAが陣営Tenkiを購入済みのとき、プレイヤーBが別の購入トークンで陣営Tenkiのセットを購入する", func(t *testing.T) {
				t.Run("プレイヤーAとプレイヤーBの両方の所有陣営一覧にTenkiが含まれる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne))
					require.NoError(t, pg.CreateFactionPurchase(playerB, "Tenki", cardPackTenki, tokenTwo))

					factionsOfA, errA := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)
					factionsOfB, errB := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerB)

					require.NoError(t, errA)
					require.NoError(t, errB)
					assert.Contains(t, factionsOfA, "Tenki")
					assert.Contains(t, factionsOfB, "Tenki")
				})
			})

			t.Run("プレイヤーAが同じ購入トークンで陣営Tenkiのセットの購入を2回続けて行う", func(t *testing.T) {
				purchaseTwice := func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne))
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne))
				}

				t.Run("所有陣営は1件のままになる", func(t *testing.T) {
					purchaseTwice(t)

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Len(t, factions, 1)
				})

				t.Run("カードパックの所有は1件のままになる", func(t *testing.T) {
					purchaseTwice(t)

					assert.Equal(t, 1, pg.CountRows(t, "shop.player_owned_card_packs"))
				})

				t.Run("イベントは2件のままになる", func(t *testing.T) {
					purchaseTwice(t)

					assert.Len(t, pg.ClaimAll(t), 2)
				})
			})

			t.Run("プレイヤーAが同じ購入トークンで陣営Tenkiのセットの購入を同時に2回行う", func(t *testing.T) {
				purchaseConcurrently := func(t *testing.T) {
					pg.Truncate(t)
					errs := postgrestest.RunConcurrently(2, func() error {
						return pg.CreateFactionPurchase(playerA, "Tenki", cardPackTenki, tokenOne)
					})
					require.NoError(t, errs[0])
					require.NoError(t, errs[1])
				}

				t.Run("所有陣営は1件になる", func(t *testing.T) {
					purchaseConcurrently(t)

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Len(t, factions, 1)
				})

				t.Run("カードパックの所有は1件になる", func(t *testing.T) {
					purchaseConcurrently(t)

					assert.Equal(t, 1, pg.CountRows(t, "shop.player_owned_card_packs"))
				})

				t.Run("イベントは2件になる", func(t *testing.T) {
					purchaseConcurrently(t)

					assert.Len(t, pg.ClaimAll(t), 2)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("購入トークンが257文字のとき", func(t *testing.T) {
				t.Run("プレイヤーAは陣営Tenkiのセットのカードパックを所有していない状態のままになる", func(t *testing.T) {
					require.Error(t, failPurchaseWithTooLongToken(t))

					owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackTenki)

					require.NoError(t, err)
					assert.False(t, owned)
				})

				t.Run("プレイヤーAの所有陣営一覧は空のままになる", func(t *testing.T) {
					require.Error(t, failPurchaseWithTooLongToken(t))

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Empty(t, factions)
				})

				t.Run("イベントは保存されない", func(t *testing.T) {
					require.Error(t, failPurchaseWithTooLongToken(t))

					assert.Empty(t, pg.ClaimAll(t))
				})
			})

			t.Run("プレイヤーAが陣営Tenkiを所有しているとき、別の購入トークンで陣営Tenkiのセットを購入する", func(t *testing.T) {
				t.Run("エラーの原因は陣営の所有の重複になる", func(t *testing.T) {
					err := failPurchaseOfOwnedFaction(t)

					postgrestest.RequireDuplicateKeyError(t, err, "player_owned_factions_pkey")
				})

				t.Run("失敗した購入トークンで検索しても、購入は見つからない", func(t *testing.T) {
					require.Error(t, failPurchaseOfOwnedFaction(t))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenFailed)

					assert.NoError(t, err)
					assert.Nil(t, got)
				})

				t.Run("カードパックの所有は1件のままになる", func(t *testing.T) {
					require.Error(t, failPurchaseOfOwnedFaction(t))

					assert.Equal(t, 1, pg.CountRows(t, "shop.player_owned_card_packs"))
				})

				t.Run("イベントは増えない", func(t *testing.T) {
					require.Error(t, failPurchaseOfOwnedFaction(t))

					assert.Len(t, pg.ClaimAll(t), 2)
				})

				t.Run("続けて、失敗した購入トークンで陣営Sugarのセットを購入すると、プレイヤーAの所有陣営一覧にSugarが含まれる", func(t *testing.T) {
					require.Error(t, failPurchaseOfOwnedFaction(t))
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Sugar", cardPackSugar, tokenFailed))

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Contains(t, factions, "Sugar")
				})
			})

			t.Run("プレイヤーAが陣営Tenkiのセットのカードパックだけを所有し、陣営Tenkiは所有していないとき、別の購入トークンで陣営Tenkiのセットを購入する", func(t *testing.T) {
				t.Run("エラーの原因はカードパックの所有の重複になる", func(t *testing.T) {
					err := failPurchaseWithOnlyCardPackOwned(t)

					postgrestest.RequireDuplicateKeyError(t, err, "player_owned_card_packs_pkey")
				})

				t.Run("プレイヤーAの所有陣営一覧は空のままになる", func(t *testing.T) {
					require.Error(t, failPurchaseWithOnlyCardPackOwned(t))

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Empty(t, factions)
				})

				t.Run("失敗した購入トークンで検索しても、購入は見つからない", func(t *testing.T) {
					require.Error(t, failPurchaseWithOnlyCardPackOwned(t))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenFailed)

					assert.NoError(t, err)
					assert.Nil(t, got)
				})

				t.Run("続けて、失敗した購入トークンで陣営Sugarのセットを購入すると、プレイヤーAの所有陣営一覧にSugarが含まれる", func(t *testing.T) {
					require.Error(t, failPurchaseWithOnlyCardPackOwned(t))
					require.NoError(t, pg.CreateFactionPurchase(playerA, "Sugar", cardPackSugar, tokenFailed))

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Contains(t, factions, "Sugar")
				})
			})

			t.Run("2件目のイベントのイベントIDが1件目のイベントのイベントIDと同じとき", func(t *testing.T) {
				t.Run("エラーの原因はイベントIDの重複になる", func(t *testing.T) {
					err := failPurchaseWithDuplicateEventID(t)

					postgrestest.RequireDuplicateKeyError(t, err, "outbox_events_pkey")
				})

				t.Run("1件目のイベントも保存されない", func(t *testing.T) {
					require.Error(t, failPurchaseWithDuplicateEventID(t))

					assert.Empty(t, pg.ClaimAll(t))
				})

				t.Run("プレイヤーAの所有陣営一覧は空のままになる", func(t *testing.T) {
					require.Error(t, failPurchaseWithDuplicateEventID(t))

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Empty(t, factions)
				})

				t.Run("失敗した購入トークンで検索しても、購入は見つからない", func(t *testing.T) {
					require.Error(t, failPurchaseWithDuplicateEventID(t))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenFailed)

					assert.NoError(t, err)
					assert.Nil(t, got)
				})
			})
		})
	})
}

func TestFactionPurchaseListOwnedFactions(t *testing.T) {
	ctx := context.Background()

	t.Run("[陣営購入]所有陣営の一覧取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAがSHEだけを所有し、プレイヤーBがSHEとTenkiを所有し、プレイヤーCは何も所有していないとき", func(t *testing.T) {
				prepare := func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateFactionPurchase(playerA, "SHE", cardPackSHE, "TST-TOKEN-A1"))
					require.NoError(t, pg.CreateFactionPurchase(playerB, "SHE", cardPackSHE, "TST-TOKEN-B1"))
					require.NoError(t, pg.CreateFactionPurchase(playerB, "Tenki", cardPackTenki, "TST-TOKEN-B2"))
				}

				t.Run("プレイヤーAの一覧はSHEだけになる", func(t *testing.T) {
					prepare(t)

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerA)

					require.NoError(t, err)
					assert.Equal(t, []string{"SHE"}, factions)
				})

				t.Run("プレイヤーBの一覧はSHEとTenkiの2件になる", func(t *testing.T) {
					prepare(t)

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerB)

					require.NoError(t, err)
					assert.ElementsMatch(t, []string{"SHE", "Tenki"}, factions)
				})

				t.Run("プレイヤーCの一覧は0件になる", func(t *testing.T) {
					prepare(t)

					factions, err := pg.FactionPurchaseRepository().ListOwnedFactions(ctx, playerC)

					require.NoError(t, err)
					assert.Len(t, factions, 0)
				})
			})
		})
	})
}
