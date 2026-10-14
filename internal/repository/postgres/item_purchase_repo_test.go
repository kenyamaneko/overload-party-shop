//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

func itemKey(itemType string, itemNo int64) string {
	return fmt.Sprintf("%s/%d", itemType, itemNo)
}

func TestItemPurchaseCreatePurchase(t *testing.T) {
	ctx := context.Background()

	t.Run("[アイテム購入]購入の作成", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAが新規の購入トークンでアイテムを購入する", func(t *testing.T) {
				t.Run("プレイヤーAがそのアイテムを所有している状態になる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					owned, err := pg.ItemPurchaseRepository().HasPlayerItem(ctx, playerA, "playmat", 1)

					require.NoError(t, err)
					assert.True(t, owned)
				})

				t.Run("購入トークンで検索すると、プレイヤーAの購入が見つかる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerA, got.PlayerID)
				})
			})

			t.Run("プレイヤーAがアイテムを購入済みのとき、別の購入トークンで別のアイテムを購入する", func(t *testing.T) {
				t.Run("プレイヤーAの所有アイテム一覧に2件とも含まれる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))
					require.NoError(t, pg.CreateItemPurchase(playerA, "sleeve", 2, "ios", tokenTwo))

					items, err := pg.ItemPurchaseRepository().ListPlayerItems(ctx, playerA)

					require.NoError(t, err)
					require.Len(t, items, 2)
					assert.ElementsMatch(t,
						[]string{"playmat/1", "sleeve/2"},
						[]string{itemKey(items[0].ItemType, items[0].ItemNo), itemKey(items[1].ItemType, items[1].ItemNo)},
					)
				})
			})

			t.Run("プレイヤーAがアイテムを購入済みのとき、プレイヤーBが別の購入トークンで同じアイテムを購入する", func(t *testing.T) {
				t.Run("プレイヤーAとプレイヤーBの両方がそのアイテムを所有している状態になる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))
					require.NoError(t, pg.CreateItemPurchase(playerB, "playmat", 1, "ios", tokenTwo))

					ownedByA, errA := pg.ItemPurchaseRepository().HasPlayerItem(ctx, playerA, "playmat", 1)
					ownedByB, errB := pg.ItemPurchaseRepository().HasPlayerItem(ctx, playerB, "playmat", 1)

					require.NoError(t, errA)
					require.NoError(t, errB)
					assert.True(t, ownedByA)
					assert.True(t, ownedByB)
				})
			})

			t.Run("プレイヤーAが同じ購入トークンでアイテムの購入を2回続けて行う", func(t *testing.T) {
				t.Run("プレイヤーAの所有アイテム一覧は1件のままになる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					items, err := pg.ItemPurchaseRepository().ListPlayerItems(ctx, playerA)

					require.NoError(t, err)
					assert.Len(t, items, 1)
				})
			})

			t.Run("プレイヤーAが同じ購入トークンでアイテムの購入を同時に2回行う", func(t *testing.T) {
				purchaseConcurrently := func(t *testing.T) {
					pg.Truncate(t)
					errs := postgrestest.RunConcurrently(2, func() error {
						return pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne)
					})
					require.NoError(t, errs[0])
					require.NoError(t, errs[1])
				}

				t.Run("プレイヤーAの所有アイテム一覧は1件になる", func(t *testing.T) {
					purchaseConcurrently(t)

					items, err := pg.ItemPurchaseRepository().ListPlayerItems(ctx, playerA)

					require.NoError(t, err)
					assert.Len(t, items, 1)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("購入トークンが257文字で、アイテムを購入するとき", func(t *testing.T) {
				t.Run("プレイヤーAはそのアイテムを所有していない状態のままになる", func(t *testing.T) {
					pg.Truncate(t)
					require.Error(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", postgrestest.TokenOfLength(257)))

					owned, err := pg.ItemPurchaseRepository().HasPlayerItem(ctx, playerA, "playmat", 1)

					require.NoError(t, err)
					assert.False(t, owned)
				})
			})

			t.Run("プレイヤーAがアイテムを所有しているとき、別の購入トークンで同じアイテムを購入する", func(t *testing.T) {
				purchaseOwnedItemAgain := func(t *testing.T) error {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))
					return pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenFailed)
				}

				t.Run("エラーの原因はアイテムの所有の重複になる", func(t *testing.T) {
					err := purchaseOwnedItemAgain(t)

					postgrestest.RequireDuplicateKeyError(t, err, "player_items_pkey")
				})

				t.Run("失敗した購入トークンで検索しても、購入は見つからない", func(t *testing.T) {
					purchaseOwnedItemAgain(t)

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenFailed)

					assert.NoError(t, err)
					assert.Nil(t, got)
				})

				t.Run("プレイヤーAの所有アイテム一覧は1件のままになる", func(t *testing.T) {
					purchaseOwnedItemAgain(t)

					items, err := pg.ItemPurchaseRepository().ListPlayerItems(ctx, playerA)

					require.NoError(t, err)
					assert.Len(t, items, 1)
				})

				t.Run("続けて、失敗した購入トークンで別のアイテムを購入すると、プレイヤーAがそのアイテムを所有している状態になる", func(t *testing.T) {
					require.Error(t, purchaseOwnedItemAgain(t))
					require.NoError(t, pg.CreateItemPurchase(playerA, "sleeve", 2, "ios", tokenFailed))

					owned, err := pg.ItemPurchaseRepository().HasPlayerItem(ctx, playerA, "sleeve", 2)

					require.NoError(t, err)
					assert.True(t, owned)
				})
			})
		})
	})
}

func TestItemPurchaseListPlayerItems(t *testing.T) {
	ctx := context.Background()

	t.Run("[アイテム購入]所有アイテムの一覧取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAがplaymat番号1とsleeve番号2を所有し、プレイヤーBがicon番号9を所有し、プレイヤーCは何も所有していないとき", func(t *testing.T) {
				playmatAcquiredAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
				sleeveAcquiredAt := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
				prepare := func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchaseAt(playerA, "playmat", 1, "ios", "TST-TOKEN-A1", playmatAcquiredAt))
					require.NoError(t, pg.CreateItemPurchaseAt(playerA, "sleeve", 2, "ios", "TST-TOKEN-A2", sleeveAcquiredAt))
					require.NoError(t, pg.CreateItemPurchaseAt(playerB, "icon", 9, "ios", "TST-TOKEN-B1", playmatAcquiredAt))
				}

				t.Run("プレイヤーAの一覧は、playmat番号1とsleeve番号2の2件になる", func(t *testing.T) {
					prepare(t)

					items, err := pg.ItemPurchaseRepository().ListPlayerItems(ctx, playerA)

					require.NoError(t, err)
					require.Len(t, items, 2)
					assert.ElementsMatch(t,
						[]string{"playmat/1", "sleeve/2"},
						[]string{itemKey(items[0].ItemType, items[0].ItemNo), itemKey(items[1].ItemType, items[1].ItemNo)},
					)
				})

				t.Run("プレイヤーAの一覧の各アイテムの獲得日時は、購入時に渡した獲得日時になる", func(t *testing.T) {
					prepare(t)

					items, err := pg.ItemPurchaseRepository().ListPlayerItems(ctx, playerA)

					require.NoError(t, err)
					require.Len(t, items, 2)
					acquiredAtByKey := map[string]time.Time{
						itemKey(items[0].ItemType, items[0].ItemNo): items[0].AcquiredAt,
						itemKey(items[1].ItemType, items[1].ItemNo): items[1].AcquiredAt,
					}
					assert.True(t, acquiredAtByKey["playmat/1"].Equal(playmatAcquiredAt), "playmat/1 acquired at %s", acquiredAtByKey["playmat/1"])
					assert.True(t, acquiredAtByKey["sleeve/2"].Equal(sleeveAcquiredAt), "sleeve/2 acquired at %s", acquiredAtByKey["sleeve/2"])
				})

				t.Run("プレイヤーBの一覧は、icon番号9の1件になる", func(t *testing.T) {
					prepare(t)

					items, err := pg.ItemPurchaseRepository().ListPlayerItems(ctx, playerB)

					require.NoError(t, err)
					require.Len(t, items, 1)
					assert.Equal(t, "icon/9", itemKey(items[0].ItemType, items[0].ItemNo))
				})

				t.Run("プレイヤーCの一覧は0件になる", func(t *testing.T) {
					prepare(t)

					items, err := pg.ItemPurchaseRepository().ListPlayerItems(ctx, playerC)

					require.NoError(t, err)
					assert.Len(t, items, 0)
				})
			})
		})
	})
}

func TestItemPurchaseHasPlayerItem(t *testing.T) {
	ctx := context.Background()

	t.Run("[アイテム購入]アイテムの所有判定", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAがplaymat番号1を所有し、プレイヤーBがicon番号9を所有しているとき", func(t *testing.T) {
				prepare := func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))
					require.NoError(t, pg.CreateItemPurchase(playerB, "icon", 9, "ios", tokenTwo))
				}

				t.Run("プレイヤーAについてplaymat番号1を判定すると、所有ありになる", func(t *testing.T) {
					prepare(t)

					owned, err := pg.ItemPurchaseRepository().HasPlayerItem(ctx, playerA, "playmat", 1)

					require.NoError(t, err)
					assert.True(t, owned)
				})

				t.Run("プレイヤーAについて判定する", func(t *testing.T) {
					cases := []struct {
						name     string
						itemType string
						itemNo   int64
					}{
						{name: "playmatの別の番号999のとき、所有なしになる", itemType: "playmat", itemNo: 999},
						{name: "sleeve番号1のとき、所有なしになる", itemType: "sleeve", itemNo: 1},
						{name: "プレイヤーBだけが所有するicon番号9のとき、所有なしになる", itemType: "icon", itemNo: 9},
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							prepare(t)

							owned, err := pg.ItemPurchaseRepository().HasPlayerItem(ctx, playerA, tc.itemType, tc.itemNo)

							require.NoError(t, err)
							assert.False(t, owned)
						})
					}
				})
			})
		})
	})
}
