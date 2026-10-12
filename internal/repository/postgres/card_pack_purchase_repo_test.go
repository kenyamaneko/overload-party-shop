//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

func TestCardPackPurchaseCreatePurchase(t *testing.T) {
	ctx := context.Background()

	t.Run("[カードパック購入]購入の作成", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("購入を作成する", func(t *testing.T) {
				cases := []struct {
					name   string
					prior  func(t *testing.T)
					create func() error
				}{
					{
						name:   "プレイヤーAが新規の購入トークンでカードパックXを購入するとき、エラーにならない",
						prior:  func(t *testing.T) {},
						create: func() error { return pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne) },
					},
					{
						name: "プレイヤーAがカードパックXを購入済みで、プレイヤーBが別の購入トークンでカードパックXを購入するとき、エラーにならない",
						prior: func(t *testing.T) {
							require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))
						},
						create: func() error { return pg.CreateCardPackPurchase(playerB, cardPackX, tokenTwo) },
					},
					{
						name: "プレイヤーAがカードパックXを購入済みで、別の購入トークンでカードパックYを購入するとき、エラーにならない",
						prior: func(t *testing.T) {
							require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))
						},
						create: func() error { return pg.CreateCardPackPurchase(playerA, cardPackY, tokenTwo) },
					},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						tc.prior(t)

						err := tc.create()

						assert.NoError(t, err)
					})
				}
			})

			t.Run("プレイヤーAが新規の購入トークンでカードパックXを購入する", func(t *testing.T) {
				t.Run("プレイヤーAがカードパックXを所有している状態になる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))

					owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackX)

					require.NoError(t, err)
					assert.True(t, owned)
				})

				t.Run("購入トークンで検索すると、プレイヤーAの購入が見つかる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerA, got.PlayerID)
				})

				t.Run("イベントが1件保存される", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))

					assert.Len(t, pg.ClaimAll(t), 1)
				})
			})

			t.Run("プレイヤーAがカードパックXを購入済みのとき、プレイヤーBが別の購入トークンでカードパックXを購入する", func(t *testing.T) {
				purchaseAsBoth := func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))
					require.NoError(t, pg.CreateCardPackPurchase(playerB, cardPackX, tokenTwo))
				}

				t.Run("プレイヤーAとプレイヤーBの両方がカードパックXを所有している状態になる", func(t *testing.T) {
					purchaseAsBoth(t)

					ownedByA, errA := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackX)
					ownedByB, errB := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerB, cardPackX)

					require.NoError(t, errA)
					require.NoError(t, errB)
					assert.True(t, ownedByA)
					assert.True(t, ownedByB)
				})

				t.Run("イベントは2件になる", func(t *testing.T) {
					purchaseAsBoth(t)

					assert.Len(t, pg.ClaimAll(t), 2)
				})
			})

			t.Run("プレイヤーAがカードパックXを購入済みのとき、別の購入トークンでカードパックYを購入する", func(t *testing.T) {
				t.Run("プレイヤーAがカードパックXとカードパックYの両方を所有している状態になる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackY, tokenTwo))

					ownedX, errX := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackX)
					ownedY, errY := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackY)

					require.NoError(t, errX)
					require.NoError(t, errY)
					assert.True(t, ownedX)
					assert.True(t, ownedY)
				})
			})

			t.Run("プレイヤーAが同じ購入トークンでカードパックXの購入を2回続けて行う", func(t *testing.T) {
				purchaseTwice := func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))
				}

				t.Run("カードパックXの所有は1件のままになる", func(t *testing.T) {
					purchaseTwice(t)

					assert.Equal(t, 1, pg.CountRows(t, "shop.player_owned_card_packs"))
				})

				t.Run("イベントは1件のままになる", func(t *testing.T) {
					purchaseTwice(t)

					assert.Len(t, pg.ClaimAll(t), 1)
				})
			})

			t.Run("プレイヤーAが同じ購入トークンでカードパックXの購入を同時に2回行う", func(t *testing.T) {
				purchaseConcurrently := func(t *testing.T) []error {
					pg.Truncate(t)
					return postgrestest.RunConcurrently(2, func() error {
						return pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne)
					})
				}

				t.Run("2回ともエラーにならない", func(t *testing.T) {
					errs := purchaseConcurrently(t)

					assert.NoError(t, errs[0])
					assert.NoError(t, errs[1])
				})

				t.Run("カードパックXの所有は1件になる", func(t *testing.T) {
					purchaseConcurrently(t)

					assert.Equal(t, 1, pg.CountRows(t, "shop.player_owned_card_packs"))
				})

				t.Run("イベントは1件になる", func(t *testing.T) {
					purchaseConcurrently(t)

					assert.Len(t, pg.ClaimAll(t), 1)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("購入トークンが257文字のとき", func(t *testing.T) {
				t.Run("プレイヤーAはカードパックXを所有していない状態のままになる", func(t *testing.T) {
					pg.Truncate(t)
					require.Error(t, pg.CreateCardPackPurchase(playerA, cardPackX, postgrestest.TokenOfLength(257)))

					owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackX)

					require.NoError(t, err)
					assert.False(t, owned)
				})

				t.Run("イベントは保存されない", func(t *testing.T) {
					pg.Truncate(t)
					require.Error(t, pg.CreateCardPackPurchase(playerA, cardPackX, postgrestest.TokenOfLength(257)))

					assert.Empty(t, pg.ClaimAll(t))
				})
			})

			t.Run("プレイヤーAがカードパックXを所有しているとき、別の購入トークンでカードパックXを購入する", func(t *testing.T) {
				purchaseOwnedPackAgain := func(t *testing.T) error {
					pg.Truncate(t)
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))
					return pg.CreateCardPackPurchase(playerA, cardPackX, tokenFailed)
				}

				t.Run("エラーの原因はカードパックの所有の重複になる", func(t *testing.T) {
					err := purchaseOwnedPackAgain(t)

					postgrestest.RequireDuplicateKeyError(t, err, "player_owned_card_packs_pkey")
				})

				t.Run("失敗した購入トークンで検索しても、購入は見つからない", func(t *testing.T) {
					purchaseOwnedPackAgain(t)

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenFailed)

					assert.NoError(t, err)
					assert.Nil(t, got)
				})

				t.Run("カードパックXの所有は1件のままになる", func(t *testing.T) {
					purchaseOwnedPackAgain(t)

					assert.Equal(t, 1, pg.CountRows(t, "shop.player_owned_card_packs"))
				})

				t.Run("イベントは増えず1件のままになる", func(t *testing.T) {
					purchaseOwnedPackAgain(t)

					assert.Len(t, pg.ClaimAll(t), 1)
				})

				t.Run("続けて、失敗した購入トークンでカードパックYを購入すると、プレイヤーAがカードパックYを所有している状態になる", func(t *testing.T) {
					require.Error(t, purchaseOwnedPackAgain(t))
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackY, tokenFailed))

					owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackY)

					require.NoError(t, err)
					assert.True(t, owned)
				})
			})
		})
	})
}

func TestCardPackPurchaseHasPlayerCardPack(t *testing.T) {
	ctx := context.Background()

	t.Run("[カードパック購入]カードパックの所有判定", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAがカードパックXを購入済みのとき、プレイヤーAについてカードパックXの所有ありになる", func(t *testing.T) {
				pg.Truncate(t)
				require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenOne))

				owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackX)

				require.NoError(t, err)
				assert.True(t, owned)
			})

			t.Run("プレイヤーAについてカードパックXの所有を判定する", func(t *testing.T) {
				cases := []struct {
					name  string
					prior func(t *testing.T)
				}{
					{
						name:  "誰も何も購入していないとき、所有なしになる",
						prior: func(t *testing.T) {},
					},
					{
						name: "プレイヤーBだけがカードパックXを購入済みのとき、所有なしになる",
						prior: func(t *testing.T) {
							require.NoError(t, pg.CreateCardPackPurchase(playerB, cardPackX, tokenOne))
						},
					},
					{
						name: "プレイヤーAがカードパックYだけを購入済みのとき、所有なしになる",
						prior: func(t *testing.T) {
							require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackY, tokenOne))
						},
					},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						tc.prior(t)

						owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackX)

						require.NoError(t, err)
						assert.False(t, owned)
					})
				}
			})
		})
	})
}
