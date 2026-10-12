//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

func TestPurchaseTokenPlatform(t *testing.T) {
	ctx := context.Background()

	t.Run("[購入]購入トークンのプラットフォーム別の保存先", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("購入を作成し、同じプラットフォームの同じ購入トークンで検索する", func(t *testing.T) {
				cases := []struct {
					name     string
					platform string
				}{
					{name: "プラットフォームがiosのとき、作成した購入が見つかる", platform: "ios"},
					{name: "プラットフォームがandroidのとき、作成した購入が見つかる", platform: "android"},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, tc.platform, tokenOne))

						got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, tc.platform, tokenOne)

						require.NoError(t, err)
						require.NotNil(t, got)
						assert.Equal(t, playerA, got.PlayerID)
					})
				}
			})

			t.Run("別のプラットフォームで同じ購入トークンを検索する", func(t *testing.T) {
				cases := []struct {
					name           string
					createPlatform string
					searchPlatform string
				}{
					{name: "iosで作成した購入のとき、見つからず、エラーにもならない", createPlatform: "ios", searchPlatform: "android"},
					{name: "androidで作成した購入のとき、見つからず、エラーにもならない", createPlatform: "android", searchPlatform: "ios"},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, tc.createPlatform, tokenOne))

						got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, tc.searchPlatform, tokenOne)

						assert.NoError(t, err)
						assert.Nil(t, got)
					})
				}
			})

			t.Run("プレイヤーAがiosで、プレイヤーBがandroidで、同じ文字列の購入トークンを使って購入を作成する", func(t *testing.T) {
				createBoth := func(t *testing.T) (error, error) {
					pg.Truncate(t)
					errA := pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne)
					errB := pg.CreateItemPurchase(playerB, "playmat", 1, "android", tokenOne)
					return errA, errB
				}

				t.Run("どちらの作成もエラーにならない", func(t *testing.T) {
					errA, errB := createBoth(t)

					assert.NoError(t, errA)
					assert.NoError(t, errB)
				})

				t.Run("iosで検索すると、プレイヤーAの購入が見つかる", func(t *testing.T) {
					createBoth(t)

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerA, got.PlayerID)
				})

				t.Run("androidで検索すると、プレイヤーBの購入が見つかる", func(t *testing.T) {
					createBoth(t)

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "android", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerB, got.PlayerID)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("購入を作成する", func(t *testing.T) {
				cases := []struct {
					name     string
					platform string
				}{
					{name: "プラットフォームがwindowsのとき、未対応プラットフォームのエラーになる", platform: "windows"},
					{name: "プラットフォームが空文字のとき、未対応プラットフォームのエラーになる", platform: ""},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)

						err := pg.CreateItemPurchase(playerA, "playmat", 1, tc.platform, tokenOne)

						assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
					})
				}

				t.Run("プラットフォームがwindowsで失敗したあと、続けてプラットフォームがiosで同じ購入を作成すると、購入トークンで検索して購入が見つかる", func(t *testing.T) {
					pg.Truncate(t)
					require.Error(t, pg.CreateItemPurchase(playerA, "playmat", 1, "windows", tokenOne))
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					assert.NotNil(t, got)
				})
			})

			t.Run("購入トークンで検索する", func(t *testing.T) {
				cases := []struct {
					name     string
					platform string
				}{
					{name: "プラットフォームがwindowsのとき、未対応プラットフォームのエラーになる", platform: "windows"},
					{name: "プラットフォームが空文字のとき、未対応プラットフォームのエラーになる", platform: ""},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)

						_, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, tc.platform, tokenOne)

						assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
					})
				}

				t.Run("プラットフォームがwindowsで失敗したあと、続けて保存済みの購入をiosで検索すると、購入が見つかる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))
					_, failedErr := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "windows", tokenOne)
					require.Error(t, failedErr)

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					assert.NotNil(t, got)
				})
			})
		})
	})
}

func TestPurchaseTokenDuplicateAndLength(t *testing.T) {
	ctx := context.Background()

	t.Run("[購入]購入トークンの重複判定と長さ", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("購入トークンが256文字のとき、購入を作成する", func(t *testing.T) {
				t.Run("エラーにならない", func(t *testing.T) {
					pg.Truncate(t)

					err := pg.CreateItemPurchase(playerA, "playmat", 1, "ios", postgrestest.TokenOfLength(256))

					assert.NoError(t, err)
				})

				t.Run("購入トークンで検索すると、購入が見つかる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", postgrestest.TokenOfLength(256)))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", postgrestest.TokenOfLength(256))

					require.NoError(t, err)
					assert.NotNil(t, got)
				})
			})

			t.Run("プレイヤーAが同じ購入トークンで同じ購入を2回続けて作成する", func(t *testing.T) {
				t.Run("2回目はエラーにならない", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					err := pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne)

					assert.NoError(t, err)
				})

				t.Run("購入の保存件数は1件のままになる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))

					assert.Equal(t, 1, pg.CountRows(t, "shop.one_time_purchases"))
				})
			})

			t.Run("プレイヤーAが同じ購入トークンで同じ購入を同時に2回作成する", func(t *testing.T) {
				createConcurrently := func(t *testing.T) []error {
					pg.Truncate(t)
					return postgrestest.RunConcurrently(2, func() error {
						return pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne)
					})
				}

				t.Run("2回ともエラーにならない", func(t *testing.T) {
					errs := createConcurrently(t)

					assert.NoError(t, errs[0])
					assert.NoError(t, errs[1])
				})

				t.Run("購入の保存件数は1件になる", func(t *testing.T) {
					createConcurrently(t)

					assert.Equal(t, 1, pg.CountRows(t, "shop.one_time_purchases"))
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("プレイヤーBが、プレイヤーAの保存済みの購入トークンと同じ購入トークンで購入を作成する", func(t *testing.T) {
				createDuplicateOfPlayerA := func(t *testing.T) error {
					pg.Truncate(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", tokenOne))
					return pg.CreateItemPurchase(playerB, "playmat", 1, "ios", tokenOne)
				}

				t.Run("エラーにならない", func(t *testing.T) {
					err := createDuplicateOfPlayerA(t)

					assert.NoError(t, err)
				})

				t.Run("購入トークンで検索すると、プレイヤーAの購入が見つかる", func(t *testing.T) {
					createDuplicateOfPlayerA(t)

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerA, got.PlayerID)
				})

				t.Run("購入の保存件数は1件のままになる", func(t *testing.T) {
					createDuplicateOfPlayerA(t)

					assert.Equal(t, 1, pg.CountRows(t, "shop.one_time_purchases"))
				})

				t.Run("続けて、プレイヤーBが別の購入トークンで同じ購入を作成すると、その購入トークンで検索してプレイヤーBの購入が見つかる", func(t *testing.T) {
					createDuplicateOfPlayerA(t)
					require.NoError(t, pg.CreateItemPurchase(playerB, "playmat", 1, "ios", tokenTwo))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenTwo)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerB, got.PlayerID)
				})
			})

			t.Run("購入トークンが257文字のとき、購入を作成する", func(t *testing.T) {
				createWithTooLongToken := func(t *testing.T) error {
					pg.Truncate(t)
					return pg.CreateItemPurchase(playerA, "playmat", 1, "ios", postgrestest.TokenOfLength(257))
				}

				t.Run("エラーの原因は購入トークンの長さの超過になる", func(t *testing.T) {
					err := createWithTooLongToken(t)

					postgrestest.RequireValueTooLongError(t, err)
				})

				t.Run("購入の保存件数は0件のままになる", func(t *testing.T) {
					createWithTooLongToken(t)

					assert.Equal(t, 0, pg.CountRows(t, "shop.one_time_purchases"))
				})

				t.Run("続けて256文字の購入トークンで同じ購入を作成すると、エラーにならない", func(t *testing.T) {
					createWithTooLongToken(t)

					err := pg.CreateItemPurchase(playerA, "playmat", 1, "ios", postgrestest.TokenOfLength(256))

					assert.NoError(t, err)
				})

				t.Run("続けて256文字の購入トークンで同じ購入を作成すると、購入の保存件数は1件になる", func(t *testing.T) {
					createWithTooLongToken(t)
					require.NoError(t, pg.CreateItemPurchase(playerA, "playmat", 1, "ios", postgrestest.TokenOfLength(256)))

					assert.Equal(t, 1, pg.CountRows(t, "shop.one_time_purchases"))
				})
			})
		})
	})
}
