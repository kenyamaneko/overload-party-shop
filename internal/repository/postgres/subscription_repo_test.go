//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

const missingSubscriptionIDOffset = 1000

func createStoredSubscription(t *testing.T, playerID, token string) *domain.Subscription {
	t.Helper()
	require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerID), "ios", token, postgrestest.NewOutboxEvent()))
	return pg.FindSubscription(t, "ios", token)
}

func TestSubscriptionPlatform(t *testing.T) {
	ctx := context.Background()

	t.Run("[サブスクリプション]プラットフォーム別の保存先", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("サブスクリプションを作成し、同じプラットフォームの同じ購入トークンで検索する", func(t *testing.T) {
				cases := []struct {
					name     string
					platform string
				}{
					{name: "プラットフォームがiosのとき、作成したサブスクリプションが見つかる", platform: "ios"},
					{name: "プラットフォームがandroidのとき、作成したサブスクリプションが見つかる", platform: "android"},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), tc.platform, tokenOne, postgrestest.NewOutboxEvent()))

						got, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, tc.platform, tokenOne)

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
					{name: "iosで作成したサブスクリプションのとき、見つからず、エラーにもならない", createPlatform: "ios", searchPlatform: "android"},
					{name: "androidで作成したサブスクリプションのとき、見つからず、エラーにもならない", createPlatform: "android", searchPlatform: "ios"},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), tc.createPlatform, tokenOne, postgrestest.NewOutboxEvent()))

						got, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, tc.searchPlatform, tokenOne)

						assert.NoError(t, err)
						assert.Nil(t, got)
					})
				}
			})

			t.Run("プレイヤーAがiosで、プレイヤーBがandroidで、同じ文字列の購入トークンを使ってサブスクリプションを作成する", func(t *testing.T) {
				createBoth := func(t *testing.T) (error, error) {
					pg.Truncate(t)
					errA := pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent())
					errB := pg.CreateSubscription(postgrestest.NewSubscription(playerB), "android", tokenOne, postgrestest.NewOutboxEvent())
					return errA, errB
				}

				t.Run("どちらの作成もエラーにならない", func(t *testing.T) {
					errA, errB := createBoth(t)

					assert.NoError(t, errA)
					assert.NoError(t, errB)
				})

				t.Run("iosで検索すると、プレイヤーAのサブスクリプションが見つかる", func(t *testing.T) {
					createBoth(t)

					got, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerA, got.PlayerID)
				})

				t.Run("androidで検索すると、プレイヤーBのサブスクリプションが見つかる", func(t *testing.T) {
					createBoth(t)

					got, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, "android", tokenOne)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerB, got.PlayerID)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("サブスクリプションを作成する", func(t *testing.T) {
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

						err := pg.CreateSubscription(postgrestest.NewSubscription(playerA), tc.platform, tokenOne, postgrestest.NewOutboxEvent())

						assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
					})
				}

				t.Run("プラットフォームがwindowsで失敗したあと、続けてプラットフォームがiosで同じサブスクリプションを作成すると、購入トークンで検索してサブスクリプションが見つかる", func(t *testing.T) {
					pg.Truncate(t)
					require.Error(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "windows", tokenOne, postgrestest.NewOutboxEvent()))
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent()))

					got, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, "ios", tokenOne)

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

						_, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, tc.platform, tokenOne)

						assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
					})
				}

				t.Run("プラットフォームがwindowsで失敗したあと、続けて保存済みのサブスクリプションをiosで検索すると、サブスクリプションが見つかる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent()))
					_, failedErr := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, "windows", tokenOne)
					require.Error(t, failedErr)

					got, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, "ios", tokenOne)

					require.NoError(t, err)
					assert.NotNil(t, got)
				})
			})
		})
	})
}

func TestSubscriptionCreateSubscription(t *testing.T) {
	ctx := context.Background()

	t.Run("[サブスクリプション]サブスクリプションの作成", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAがサブスクリプションを作成する", func(t *testing.T) {
				t.Run("作成したサブスクリプションにサブスクリプションIDが付与される", func(t *testing.T) {
					pg.Truncate(t)
					subscription := postgrestest.NewSubscription(playerA)

					require.NoError(t, pg.CreateSubscription(subscription, "ios", tokenOne, postgrestest.NewOutboxEvent()))

					assert.NotZero(t, subscription.SubscriptionID)
				})

				t.Run("イベントが1件保存される", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent()))

					assert.Len(t, pg.ClaimAll(t), 1)
				})
			})

			t.Run("プレイヤーAが同じ購入トークンでサブスクリプションを2回続けて作成する", func(t *testing.T) {
				t.Run("2回目はエラーにならない", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent()))

					err := pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent())

					assert.NoError(t, err)
				})

				t.Run("サブスクリプションの保存件数は1件のままになる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent()))
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent()))

					assert.Equal(t, 1, pg.CountRows(t, "shop.subscriptions"))
				})

				t.Run("イベントは1件のままになる", func(t *testing.T) {
					pg.Truncate(t)
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent()))
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent()))

					assert.Len(t, pg.ClaimAll(t), 1)
				})
			})

			t.Run("プレイヤーAが同じ購入トークンでサブスクリプションを同時に2回作成する", func(t *testing.T) {
				createConcurrently := func(t *testing.T) []error {
					pg.Truncate(t)
					return postgrestest.RunConcurrently(2, func() error {
						return pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEvent())
					})
				}

				t.Run("2回ともエラーにならない", func(t *testing.T) {
					errs := createConcurrently(t)

					assert.NoError(t, errs[0])
					assert.NoError(t, errs[1])
				})

				t.Run("サブスクリプションの保存件数は1件になる", func(t *testing.T) {
					createConcurrently(t)

					assert.Equal(t, 1, pg.CountRows(t, "shop.subscriptions"))
				})

				t.Run("イベントは1件になる", func(t *testing.T) {
					createConcurrently(t)

					assert.Len(t, pg.ClaimAll(t), 1)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("購入トークンが257文字のとき", func(t *testing.T) {
				createWithTooLongToken := func(t *testing.T) error {
					pg.Truncate(t)
					return pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", postgrestest.TokenOfLength(257), postgrestest.NewOutboxEvent())
				}

				t.Run("エラーの原因は購入トークンの長さの超過になる", func(t *testing.T) {
					err := createWithTooLongToken(t)

					postgrestest.RequireValueTooLongError(t, err)
				})

				t.Run("サブスクリプションの保存件数は0件のままになる", func(t *testing.T) {
					require.Error(t, createWithTooLongToken(t))

					assert.Equal(t, 0, pg.CountRows(t, "shop.subscriptions"))
				})

				t.Run("イベントは保存されない", func(t *testing.T) {
					require.Error(t, createWithTooLongToken(t))

					assert.Len(t, pg.ClaimAll(t), 0)
				})

				t.Run("続けて256文字の購入トークンで同じサブスクリプションを作成すると、プレイヤーAの最新のサブスクリプションとして見つかる", func(t *testing.T) {
					require.Error(t, createWithTooLongToken(t))
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", postgrestest.TokenOfLength(256), postgrestest.NewOutboxEvent()))

					got, err := pg.SubscriptionRepository().GetLatestSubscription(ctx, playerA)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, playerA, got.PlayerID)
				})
			})

			t.Run("保存済みのイベントと同じイベントIDのイベントで、別の購入トークンのサブスクリプションを作成する", func(t *testing.T) {
				createWithDuplicateEventID := func(t *testing.T) error {
					pg.Truncate(t)
					eventID := uuid.New()
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEventWithID(eventID)))
					return pg.CreateSubscription(postgrestest.NewSubscription(playerB), "ios", tokenFailed, postgrestest.NewOutboxEventWithID(eventID))
				}

				t.Run("エラーの原因はイベントIDの重複になる", func(t *testing.T) {
					err := createWithDuplicateEventID(t)

					postgrestest.RequireDuplicateKeyError(t, err, "outbox_events_pkey")
				})

				t.Run("失敗した購入トークンで検索しても、サブスクリプションは見つからない", func(t *testing.T) {
					require.Error(t, createWithDuplicateEventID(t))

					got, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, "ios", tokenFailed)

					assert.NoError(t, err)
					assert.Nil(t, got)
				})
			})
		})
	})
}

func TestSubscriptionFindSubscriptionByToken(t *testing.T) {
	ctx := context.Background()

	t.Run("[サブスクリプション]購入トークンによるサブスクリプションの取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("保存済みの購入トークンで検索する", func(t *testing.T) {
				prepare := func(t *testing.T) *domain.Subscription {
					pg.Truncate(t)
					subscription := postgrestest.NewSubscription(playerA)
					subscription.Status = "grace_period"
					require.NoError(t, pg.CreateSubscription(subscription, "ios", tokenOne, postgrestest.NewOutboxEvent()))
					return pg.FindSubscription(t, "ios", tokenOne)
				}

				t.Run("プレイヤーは、保存したプレイヤーになる", func(t *testing.T) {
					got := prepare(t)

					assert.Equal(t, playerA, got.PlayerID)
				})

				t.Run("商品は、保存した商品になる", func(t *testing.T) {
					got := prepare(t)

					assert.Equal(t, "TST-PRODUCT-1", got.ProductID)
				})

				t.Run("状態は、保存した状態になる", func(t *testing.T) {
					got := prepare(t)

					assert.Equal(t, "grace_period", got.Status)
				})

				t.Run("課金期間の開始は、保存した時刻になる", func(t *testing.T) {
					got := prepare(t)

					assert.True(t, got.CurrentPeriodStart.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), "got %s", got.CurrentPeriodStart)
				})

				t.Run("課金期間の終了は、保存した時刻になる", func(t *testing.T) {
					got := prepare(t)

					assert.True(t, got.CurrentPeriodEnd.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)), "got %s", got.CurrentPeriodEnd)
				})

				t.Run("作成日時は、保存した時刻になる", func(t *testing.T) {
					got := prepare(t)

					assert.True(t, got.CreatedAt.Equal(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)), "got %s", got.CreatedAt)
				})

				t.Run("更新日時は、保存した時刻になる", func(t *testing.T) {
					got := prepare(t)

					assert.True(t, got.UpdatedAt.Equal(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)), "got %s", got.UpdatedAt)
				})
			})

			t.Run("保存されていない購入トークンで検索すると、見つからず、エラーにもならない", func(t *testing.T) {
				pg.Truncate(t)
				createStoredSubscription(t, playerA, tokenOne)

				got, err := pg.SubscriptionRepository().FindSubscriptionByToken(ctx, "ios", tokenTwo)

				assert.NoError(t, err)
				assert.Nil(t, got)
			})
		})
	})
}

func TestSubscriptionGetLatestSubscription(t *testing.T) {
	ctx := context.Background()

	t.Run("[サブスクリプション]最新のサブスクリプションの取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("同じプレイヤーが作成日時の異なるサブスクリプションを2件持つとき、作成日時の新しい方が返る", func(t *testing.T) {
				pg.Truncate(t)
				newer := postgrestest.NewSubscription(playerA)
				newer.Status = "expired"
				newer.CreatedAt = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
				older := postgrestest.NewSubscription(playerA)
				older.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				require.NoError(t, pg.CreateSubscription(newer, "android", tokenTwo, postgrestest.NewOutboxEvent()))
				require.NoError(t, pg.CreateSubscription(older, "ios", tokenOne, postgrestest.NewOutboxEvent()))

				got, err := pg.SubscriptionRepository().GetLatestSubscription(ctx, playerA)

				require.NoError(t, err)
				require.NotNil(t, got)
				assert.True(t, got.CreatedAt.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)), "got %s", got.CreatedAt)
			})

			t.Run("プレイヤーAの古いサブスクリプションと、プレイヤーBの新しいサブスクリプションがあるとき、プレイヤーAについて取得すると、プレイヤーAのサブスクリプションが返る", func(t *testing.T) {
				pg.Truncate(t)
				older := postgrestest.NewSubscription(playerA)
				older.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				newer := postgrestest.NewSubscription(playerB)
				newer.Status = "expired"
				newer.CreatedAt = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
				require.NoError(t, pg.CreateSubscription(older, "ios", tokenOne, postgrestest.NewOutboxEvent()))
				require.NoError(t, pg.CreateSubscription(newer, "android", tokenTwo, postgrestest.NewOutboxEvent()))

				got, err := pg.SubscriptionRepository().GetLatestSubscription(ctx, playerA)

				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, playerA, got.PlayerID)
			})

			t.Run("サブスクリプションが1件だけのとき、その1件が返る", func(t *testing.T) {
				pg.Truncate(t)
				stored := createStoredSubscription(t, playerA, tokenOne)

				got, err := pg.SubscriptionRepository().GetLatestSubscription(ctx, playerA)

				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, stored.SubscriptionID, got.SubscriptionID)
			})

			t.Run("サブスクリプションが1件も無いとき、見つからず、エラーにもならない", func(t *testing.T) {
				pg.Truncate(t)

				got, err := pg.SubscriptionRepository().GetLatestSubscription(ctx, playerA)

				assert.NoError(t, err)
				assert.Nil(t, got)
			})
		})
	})
}

func TestSubscriptionUpdateSubscription(t *testing.T) {
	ctx := context.Background()

	t.Run("[サブスクリプション]サブスクリプションの更新", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("作成済みのサブスクリプションを更新する", func(t *testing.T) {
				t.Run("状態をcancelledに更新すると、検索した状態がcancelledになる", func(t *testing.T) {
					pg.Truncate(t)
					update := *createStoredSubscription(t, playerA, tokenOne)
					update.Status = "cancelled"
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscription(ctx, &update))

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.Equal(t, "cancelled", got.Status)
				})

				t.Run("課金期間の終了を更新すると、検索した課金期間の終了が更新後の時刻になる", func(t *testing.T) {
					pg.Truncate(t)
					update := *createStoredSubscription(t, playerA, tokenOne)
					update.CurrentPeriodEnd = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscription(ctx, &update))

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.True(t, got.CurrentPeriodEnd.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)), "got %s", got.CurrentPeriodEnd)
				})

				t.Run("課金期間の開始を更新すると、検索した課金期間の開始が更新後の時刻になる", func(t *testing.T) {
					pg.Truncate(t)
					update := *createStoredSubscription(t, playerA, tokenOne)
					update.CurrentPeriodStart = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscription(ctx, &update))

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.True(t, got.CurrentPeriodStart.Equal(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)), "got %s", got.CurrentPeriodStart)
				})

				t.Run("プレイヤーBのサブスクリプションがあるとき、プレイヤーAのサブスクリプションを更新しても、プレイヤーBのサブスクリプションの状態はactiveのままになる", func(t *testing.T) {
					pg.Truncate(t)
					update := *createStoredSubscription(t, playerA, tokenOne)
					createStoredSubscription(t, playerB, tokenTwo)
					update.Status = "cancelled"
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscription(ctx, &update))

					got := pg.FindSubscription(t, "ios", tokenTwo)

					assert.Equal(t, "active", got.Status)
				})

				t.Run("更新しても、イベントは増えない", func(t *testing.T) {
					pg.Truncate(t)
					update := *createStoredSubscription(t, playerA, tokenOne)
					update.Status = "cancelled"
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscription(ctx, &update))

					assert.Len(t, pg.ClaimAll(t), 1)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("プレイヤーBのサブスクリプションがあるとき、保存されていないサブスクリプションIDのサブスクリプションを更新する", func(t *testing.T) {
				updateMissing := func(t *testing.T) error {
					pg.Truncate(t)
					stored := createStoredSubscription(t, playerB, tokenOne)
					missing := *stored
					missing.SubscriptionID = stored.SubscriptionID + missingSubscriptionIDOffset
					missing.Status = "cancelled"
					return pg.SubscriptionRepository().UpdateSubscription(ctx, &missing)
				}

				t.Run("更新対象が見つからないエラーになる", func(t *testing.T) {
					err := updateMissing(t)

					assert.ErrorIs(t, err, port.ErrNotFound)
				})

				t.Run("プレイヤーBのサブスクリプションの状態はactiveのままになる", func(t *testing.T) {
					require.Error(t, updateMissing(t))

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.Equal(t, "active", got.Status)
				})

				t.Run("続けて、プレイヤーBのサブスクリプションの状態をcancelledに更新すると、検索した状態がcancelledになる", func(t *testing.T) {
					require.Error(t, updateMissing(t))
					update := *pg.FindSubscription(t, "ios", tokenOne)
					update.Status = "cancelled"
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscription(ctx, &update))

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.Equal(t, "cancelled", got.Status)
				})
			})
		})
	})
}

func TestSubscriptionUpdateSubscriptionWithEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("[サブスクリプション]更新とイベントの同時書き込み", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("作成済みのサブスクリプションを、イベントとともに更新する", func(t *testing.T) {
				t.Run("状態をcancelledに更新すると、検索した状態がcancelledになる", func(t *testing.T) {
					pg.Truncate(t)
					update := *createStoredSubscription(t, playerA, tokenOne)
					update.Status = "cancelled"
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscriptionWithEvent(ctx, &update, postgrestest.NewOutboxEvent()))

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.Equal(t, "cancelled", got.Status)
				})

				t.Run("課金期間の終了を更新すると、検索した課金期間の終了が更新後の時刻になる", func(t *testing.T) {
					pg.Truncate(t)
					update := *createStoredSubscription(t, playerA, tokenOne)
					update.CurrentPeriodEnd = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscriptionWithEvent(ctx, &update, postgrestest.NewOutboxEvent()))

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.True(t, got.CurrentPeriodEnd.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)), "got %s", got.CurrentPeriodEnd)
				})

				t.Run("渡したイベントIDのイベントが1件追加される", func(t *testing.T) {
					pg.Truncate(t)
					createEventID := uuid.New()
					updateEventID := uuid.New()
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEventWithID(createEventID)))
					update := *pg.FindSubscription(t, "ios", tokenOne)
					update.Status = "cancelled"
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscriptionWithEvent(ctx, &update, postgrestest.NewOutboxEventWithID(updateEventID)))

					assert.ElementsMatch(t, []uuid.UUID{createEventID, updateEventID}, postgrestest.ClaimedEventIDs(pg.ClaimAll(t)))
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("保存済みのイベントと同じイベントIDのイベントで更新する", func(t *testing.T) {
				updateWithDuplicateEventID := func(t *testing.T) (domain.Subscription, error) {
					pg.Truncate(t)
					eventID := uuid.New()
					require.NoError(t, pg.CreateSubscription(postgrestest.NewSubscription(playerA), "ios", tokenOne, postgrestest.NewOutboxEventWithID(eventID)))
					update := *pg.FindSubscription(t, "ios", tokenOne)
					update.Status = "cancelled"
					update.CurrentPeriodEnd = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
					return update, pg.SubscriptionRepository().UpdateSubscriptionWithEvent(ctx, &update, postgrestest.NewOutboxEventWithID(eventID))
				}

				t.Run("エラーの原因はイベントIDの重複になる", func(t *testing.T) {
					_, err := updateWithDuplicateEventID(t)

					postgrestest.RequireDuplicateKeyError(t, err, "outbox_events_pkey")
				})

				t.Run("検索した状態は、更新前のactiveのままになる", func(t *testing.T) {
					_, err := updateWithDuplicateEventID(t)
					require.Error(t, err)

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.Equal(t, "active", got.Status)
				})

				t.Run("検索した課金期間の終了は、更新前のままになる", func(t *testing.T) {
					_, err := updateWithDuplicateEventID(t)
					require.Error(t, err)

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.True(t, got.CurrentPeriodEnd.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)), "got %s", got.CurrentPeriodEnd)
				})

				t.Run("続けて、新しいイベントIDで同じ更新をやり直すと、検索した状態が更新後になる", func(t *testing.T) {
					update, err := updateWithDuplicateEventID(t)
					require.Error(t, err)
					require.NoError(t, pg.SubscriptionRepository().UpdateSubscriptionWithEvent(ctx, &update, postgrestest.NewOutboxEvent()))

					got := pg.FindSubscription(t, "ios", tokenOne)

					assert.Equal(t, "cancelled", got.Status)
				})
			})

			t.Run("プレイヤーBのサブスクリプションがあるとき、保存されていないサブスクリプションIDのサブスクリプションをイベントとともに更新する", func(t *testing.T) {
				updateMissing := func(t *testing.T) error {
					pg.Truncate(t)
					stored := createStoredSubscription(t, playerB, tokenOne)
					missing := *stored
					missing.SubscriptionID = stored.SubscriptionID + missingSubscriptionIDOffset
					missing.Status = "cancelled"
					return pg.SubscriptionRepository().UpdateSubscriptionWithEvent(ctx, &missing, postgrestest.NewOutboxEvent())
				}

				t.Run("更新対象が見つからないエラーになる", func(t *testing.T) {
					err := updateMissing(t)

					assert.ErrorIs(t, err, port.ErrNotFound)
				})

				t.Run("イベントは増えず、プレイヤーBのサブスクリプションの作成時の1件のままになる", func(t *testing.T) {
					require.Error(t, updateMissing(t))

					assert.Len(t, pg.ClaimAll(t), 1)
				})
			})
		})
	})
}
