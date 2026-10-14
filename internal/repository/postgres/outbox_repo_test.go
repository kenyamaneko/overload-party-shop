//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

const (
	unboundedClaimLimit         = 1000
	unreachableFailureThreshold = 1000
)

func createUnpublishedEvents(t *testing.T, count int) []uuid.UUID {
	t.Helper()
	eventIDs := make([]uuid.UUID, 0, count)
	for i := range count {
		eventID := uuid.New()
		err := pg.CreateCardPackPurchaseWithEvent(playerA, fmt.Sprintf("TST-PACK-%d", i), fmt.Sprintf("TST-TOKEN-%d", i), postgrestest.NewOutboxEventWithID(eventID))
		require.NoError(t, err)
		eventIDs = append(eventIDs, eventID)
	}
	return eventIDs
}

func claimWith(t *testing.T, limit int, visibilityTimeout time.Duration, failureThreshold int) []port.ClaimedOutboxEvent {
	t.Helper()
	claimed, err := pg.OutboxRepository().ClaimUnpublished(context.Background(), limit, visibilityTimeout, failureThreshold)
	require.NoError(t, err)
	return claimed
}

func recordFailures(t *testing.T, eventID uuid.UUID, count int) {
	t.Helper()
	for range count {
		require.NoError(t, pg.OutboxRepository().RecordFailure(context.Background(), eventID, "TST-ERROR"))
	}
}

func readLastError(t *testing.T, eventID uuid.UUID) *string {
	t.Helper()
	var lastError *string
	err := pg.Pool.QueryRow(context.Background(), "SELECT last_error FROM shop.outbox_events WHERE event_id = $1", eventID).Scan(&lastError)
	require.NoError(t, err)
	return lastError
}

func TestOutboxWriteEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("[アウトボックス]イベントの書き込み", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAがカードパック購入を作成する", func(t *testing.T) {
				eventID := uuid.MustParse("00000000-0000-4000-8000-0000000000e1")
				prepare := func(t *testing.T) port.ClaimedOutboxEvent {
					pg.Truncate(t)
					event := port.OutboxEvent{EventID: eventID, EventType: "TST-EVENT-TYPE-WRITE", Payload: []byte(`{"playerId":"TST-PLAYER","count":3}`)}
					require.NoError(t, pg.CreateCardPackPurchaseWithEvent(playerA, cardPackX, tokenOne, event))
					claimed := pg.ClaimAll(t)
					require.Len(t, claimed, 1)
					return claimed[0]
				}

				t.Run("取得したイベントのイベントIDは、渡したイベントIDになる", func(t *testing.T) {
					claimed := prepare(t)

					assert.Equal(t, eventID, claimed.EventID)
				})

				t.Run("取得したイベントのイベント種別は、渡したイベント種別になる", func(t *testing.T) {
					claimed := prepare(t)

					assert.Equal(t, "TST-EVENT-TYPE-WRITE", claimed.EventType)
				})

				t.Run("取得したイベントのペイロードは、渡したペイロードと同じJSONの内容になる", func(t *testing.T) {
					claimed := prepare(t)

					assert.JSONEq(t, `{"playerId":"TST-PLAYER","count":3}`, string(claimed.Payload))
				})

				t.Run("取得したイベントの失敗回数は0になる", func(t *testing.T) {
					claimed := prepare(t)

					assert.Equal(t, 0, claimed.FailureCount)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("プレイヤーBのカードパックYの購入で保存済みのイベントと同じイベントIDのイベントを持つ、プレイヤーAのカードパックXの購入を作成する", func(t *testing.T) {
				createWithDuplicateEventID := func(t *testing.T) error {
					pg.Truncate(t)
					eventID := uuid.New()
					require.NoError(t, pg.CreateCardPackPurchaseWithEvent(playerB, cardPackY, tokenOne, postgrestest.NewOutboxEventWithID(eventID)))
					return pg.CreateCardPackPurchaseWithEvent(playerA, cardPackX, tokenFailed, postgrestest.NewOutboxEventWithID(eventID))
				}

				t.Run("エラーの原因はイベントIDの重複になる", func(t *testing.T) {
					err := createWithDuplicateEventID(t)

					postgrestest.RequireDuplicateKeyError(t, err, "outbox_events_pkey")
				})

				t.Run("失敗した購入トークンで検索しても、購入は見つからない", func(t *testing.T) {
					require.Error(t, createWithDuplicateEventID(t))

					got, err := pg.PurchaseLookupRepository().FindPurchaseByToken(ctx, "ios", tokenFailed)

					assert.NoError(t, err)
					assert.Nil(t, got)
				})

				t.Run("プレイヤーAはカードパックXを所有していない状態のままになる", func(t *testing.T) {
					require.Error(t, createWithDuplicateEventID(t))

					owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackX)

					require.NoError(t, err)
					assert.False(t, owned)
				})

				t.Run("保存済みだったイベントは1件のままになる", func(t *testing.T) {
					require.Error(t, createWithDuplicateEventID(t))

					assert.Len(t, pg.ClaimAll(t), 1)
				})

				t.Run("続けて、新しいイベントIDでプレイヤーAのカードパックXの購入を作成すると、プレイヤーAがカードパックXを所有している状態になる", func(t *testing.T) {
					require.Error(t, createWithDuplicateEventID(t))
					require.NoError(t, pg.CreateCardPackPurchase(playerA, cardPackX, tokenFailed))

					owned, err := pg.CardPackPurchaseRepository().HasPlayerCardPack(ctx, playerA, cardPackX)

					require.NoError(t, err)
					assert.True(t, owned)
				})
			})
		})
	})
}

func TestOutboxClaimUnpublished(t *testing.T) {
	ctx := context.Background()

	t.Run("[アウトボックス]未配信イベントの取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("取得対象の絞り込み", func(t *testing.T) {
				t.Run("未配信のイベントが2件あるとき、2件とも取得される", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 2)

					claimed := pg.ClaimAll(t)

					assert.ElementsMatch(t, eventIDs, postgrestest.ClaimedEventIDs(claimed))
				})

				t.Run("未配信のイベントが無いとき、0件になる", func(t *testing.T) {
					pg.Truncate(t)

					assert.Len(t, pg.ClaimAll(t), 0)
				})

				t.Run("配信済みのイベントと未配信のイベントがあるとき、未配信のイベントだけが取得される", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 2)
					require.NoError(t, pg.OutboxRepository().MarkPublished(ctx, eventIDs[0]))

					claimed := pg.ClaimAll(t)

					assert.Equal(t, []uuid.UUID{eventIDs[1]}, postgrestest.ClaimedEventIDs(claimed))
				})
			})

			t.Run("取得したイベントを続けて取得する", func(t *testing.T) {
				t.Run("取得待ち時間より短い間隔で続けて取得すると、直前に取得したイベントは取得されない", func(t *testing.T) {
					pg.Truncate(t)
					createUnpublishedEvents(t, 2)
					require.Len(t, pg.ClaimAll(t), 2)

					claimed := claimWith(t, unboundedClaimLimit, time.Hour, unreachableFailureThreshold)

					assert.Len(t, claimed, 0)
				})

				t.Run("取得待ち時間より長い間隔を空けて取得すると、直前に取得したイベントが再び取得される", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 2)
					require.Len(t, pg.ClaimAll(t), 2)

					claimed := pg.ClaimAll(t)

					assert.ElementsMatch(t, eventIDs, postgrestest.ClaimedEventIDs(claimed))
				})
			})

			t.Run("失敗が記録されたイベントを、取得待ち時間より長い間隔を空けて取得する", func(t *testing.T) {
				cases := []struct {
					name         string
					failureCount int
					wantClaimed  int
				}{
					{name: "失敗回数が2回で、失敗回数の閾値が3のとき、取得される", failureCount: 2, wantClaimed: 1},
					{name: "失敗回数が3回で、失敗回数の閾値が3のとき、取得されない", failureCount: 3, wantClaimed: 0},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						eventIDs := createUnpublishedEvents(t, 1)
						recordFailures(t, eventIDs[0], tc.failureCount)

						claimed := claimWith(t, unboundedClaimLimit, 0, 3)

						assert.Len(t, claimed, tc.wantClaimed)
					})
				}
			})

			t.Run("未配信のイベントが3件あるとき、取得件数の上限を2にして取得する", func(t *testing.T) {
				t.Run("2件が取得される", func(t *testing.T) {
					pg.Truncate(t)
					createUnpublishedEvents(t, 3)

					claimed := claimWith(t, 2, time.Hour, unreachableFailureThreshold)

					assert.Len(t, claimed, 2)
				})

				t.Run("取得されるのは、作成日時が古いほうから2件になる", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 3)

					claimed := claimWith(t, 2, time.Hour, unreachableFailureThreshold)

					assert.Equal(t, eventIDs[:2], postgrestest.ClaimedEventIDs(claimed))
				})

				t.Run("続けて取得すると、残りの1件が取得される", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 3)
					require.Len(t, claimWith(t, 2, time.Hour, unreachableFailureThreshold), 2)

					claimed := claimWith(t, 2, time.Hour, unreachableFailureThreshold)

					assert.Equal(t, []uuid.UUID{eventIDs[2]}, postgrestest.ClaimedEventIDs(claimed))
				})
			})

			t.Run("取得したイベントの内容", func(t *testing.T) {
				eventID := uuid.MustParse("00000000-0000-4000-8000-0000000000e2")
				prepare := func(t *testing.T) port.ClaimedOutboxEvent {
					pg.Truncate(t)
					event := port.OutboxEvent{EventID: eventID, EventType: "TST-EVENT-TYPE-CLAIM", Payload: []byte(`{"key":"value","number":5}`)}
					require.NoError(t, pg.CreateCardPackPurchaseWithEvent(playerA, cardPackX, tokenOne, event))
					recordFailures(t, eventID, 2)
					claimed := pg.ClaimAll(t)
					require.Len(t, claimed, 1)
					return claimed[0]
				}

				t.Run("イベントIDは、保存したイベントIDになる", func(t *testing.T) {
					claimed := prepare(t)

					assert.Equal(t, eventID, claimed.EventID)
				})

				t.Run("イベント種別は、保存したイベント種別になる", func(t *testing.T) {
					claimed := prepare(t)

					assert.Equal(t, "TST-EVENT-TYPE-CLAIM", claimed.EventType)
				})

				t.Run("ペイロードは、保存したJSONと同じ内容になる", func(t *testing.T) {
					claimed := prepare(t)

					assert.JSONEq(t, `{"key":"value","number":5}`, string(claimed.Payload))
				})

				t.Run("失敗を2回記録したイベントの失敗回数は2になる", func(t *testing.T) {
					claimed := prepare(t)

					assert.Equal(t, 2, claimed.FailureCount)
				})
			})

			t.Run("未配信のイベントが10件あるとき、取得件数の上限を5にした2つの取得を同時に行うと、2つの取得で返ったイベントを合わせて、10件のイベントがそれぞれ1回ずつになる", func(t *testing.T) {
				pg.Truncate(t)
				eventIDs := createUnpublishedEvents(t, 10)
				results := make([][]port.ClaimedOutboxEvent, 2)
				errs := make([]error, 2)
				var wg sync.WaitGroup
				for i := range 2 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						results[i], errs[i] = pg.OutboxRepository().ClaimUnpublished(ctx, 5, time.Hour, unreachableFailureThreshold)
					}()
				}
				wg.Wait()
				require.NoError(t, errs[0])
				require.NoError(t, errs[1])

				returnedIDs := append(postgrestest.ClaimedEventIDs(results[0]), postgrestest.ClaimedEventIDs(results[1])...)

				assert.ElementsMatch(t, eventIDs, returnedIDs)
			})
		})
	})
}

func TestOutboxMarkPublished(t *testing.T) {
	ctx := context.Background()

	t.Run("[アウトボックス]配信済みの記録", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("イベントを配信済みにする", func(t *testing.T) {
				cases := []struct {
					name      string
					markCount int
				}{
					{name: "未配信のイベントのとき、以後、取得待ち時間0秒で取得しても、そのイベントは取得されない", markCount: 1},
					{name: "すでに配信済みのイベントのとき、以後、取得待ち時間0秒で取得しても、そのイベントは取得されない", markCount: 2},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						eventIDs := createUnpublishedEvents(t, 1)
						for range tc.markCount {
							require.NoError(t, pg.OutboxRepository().MarkPublished(ctx, eventIDs[0]))
						}

						assert.Len(t, pg.ClaimAll(t), 0)
					})
				}
			})

			t.Run("失敗を記録したイベントを配信済みにする", func(t *testing.T) {
				t.Run("直近のエラーが空になる", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 1)
					recordFailures(t, eventIDs[0], 1)
					require.NotNil(t, readLastError(t, eventIDs[0]))
					require.NoError(t, pg.OutboxRepository().MarkPublished(ctx, eventIDs[0]))

					assert.Nil(t, readLastError(t, eventIDs[0]))
				})
			})

			t.Run("未配信のイベントが2件あるとき、1件を配信済みにする", func(t *testing.T) {
				t.Run("もう1件のイベントは、引き続き取得される", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 2)
					require.NoError(t, pg.OutboxRepository().MarkPublished(ctx, eventIDs[0]))

					assert.Equal(t, []uuid.UUID{eventIDs[1]}, postgrestest.ClaimedEventIDs(pg.ClaimAll(t)))
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("未配信のイベントが1件あるとき、保存されていないイベントIDを配信済みにする", func(t *testing.T) {
				t.Run("更新対象が見つからないエラーになる", func(t *testing.T) {
					pg.Truncate(t)
					createUnpublishedEvents(t, 1)

					err := pg.OutboxRepository().MarkPublished(ctx, uuid.New())

					assert.ErrorIs(t, err, port.ErrNotFound)
				})

				t.Run("保存済みのイベントは、引き続き取得される", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 1)
					require.Error(t, pg.OutboxRepository().MarkPublished(ctx, uuid.New()))

					assert.Equal(t, eventIDs, postgrestest.ClaimedEventIDs(pg.ClaimAll(t)))
				})

				t.Run("続けて、保存済みのイベントを配信済みにすると、以後、取得待ち時間0秒で取得しても、そのイベントは取得されない", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 1)
					require.Error(t, pg.OutboxRepository().MarkPublished(ctx, uuid.New()))
					require.NoError(t, pg.OutboxRepository().MarkPublished(ctx, eventIDs[0]))

					assert.Len(t, pg.ClaimAll(t), 0)
				})
			})
		})
	})
}

func TestOutboxRecordFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("[アウトボックス]失敗の記録", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("失敗を記録する", func(t *testing.T) {
				cases := []struct {
					name          string
					priorFailures int
					wantFailures  int
				}{
					{name: "失敗の記録が0回のイベントのとき、失敗回数は1になる", priorFailures: 0, wantFailures: 1},
					{name: "失敗の記録が1回のイベントのとき、失敗回数は2になる", priorFailures: 1, wantFailures: 2},
					{name: "失敗の記録が2回のイベントのとき、失敗回数は3になる", priorFailures: 2, wantFailures: 3},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						pg.Truncate(t)
						eventIDs := createUnpublishedEvents(t, 1)
						recordFailures(t, eventIDs[0], tc.priorFailures)
						require.NoError(t, pg.OutboxRepository().RecordFailure(ctx, eventIDs[0], "TST-ERROR"))

						claimed := pg.ClaimAll(t)

						require.Len(t, claimed, 1)
						assert.Equal(t, tc.wantFailures, claimed[0].FailureCount)
					})
				}
			})

			t.Run("直近のエラー", func(t *testing.T) {
				t.Run("失敗を記録すると、直近のエラーは記録したエラーの内容になる", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 1)
					require.NoError(t, pg.OutboxRepository().RecordFailure(ctx, eventIDs[0], "TST-ERROR-FIRST"))

					lastError := readLastError(t, eventIDs[0])

					require.NotNil(t, lastError)
					assert.Equal(t, "TST-ERROR-FIRST", *lastError)
				})

				t.Run("続けて別のエラーの内容で失敗を記録すると、直近のエラーは新しいエラーの内容に置き換わる", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 1)
					require.NoError(t, pg.OutboxRepository().RecordFailure(ctx, eventIDs[0], "TST-ERROR-FIRST"))
					require.NoError(t, pg.OutboxRepository().RecordFailure(ctx, eventIDs[0], "TST-ERROR-SECOND"))

					lastError := readLastError(t, eventIDs[0])

					require.NotNil(t, lastError)
					assert.Equal(t, "TST-ERROR-SECOND", *lastError)
				})
			})

			t.Run("失敗を記録したイベントを、取得待ち時間0秒で取得すると、取得される", func(t *testing.T) {
				pg.Truncate(t)
				eventIDs := createUnpublishedEvents(t, 1)
				require.NoError(t, pg.OutboxRepository().RecordFailure(ctx, eventIDs[0], "TST-ERROR"))

				assert.Equal(t, eventIDs, postgrestest.ClaimedEventIDs(pg.ClaimAll(t)))
			})

			t.Run("失敗を記録した直後に、取得待ち時間より短い間隔で取得すると、そのイベントは取得されない", func(t *testing.T) {
				pg.Truncate(t)
				eventIDs := createUnpublishedEvents(t, 1)
				require.NoError(t, pg.OutboxRepository().RecordFailure(ctx, eventIDs[0], "TST-ERROR"))

				claimed := claimWith(t, unboundedClaimLimit, time.Hour, unreachableFailureThreshold)

				assert.Len(t, claimed, 0)
			})

			t.Run("2件のイベントのうち1件に失敗を記録しても、もう1件の失敗回数は0のままになる", func(t *testing.T) {
				pg.Truncate(t)
				eventIDs := createUnpublishedEvents(t, 2)
				require.NoError(t, pg.OutboxRepository().RecordFailure(ctx, eventIDs[0], "TST-ERROR"))

				claimed := pg.ClaimAll(t)

				require.Len(t, claimed, 2)
				failureCounts := map[uuid.UUID]int{}
				for _, event := range claimed {
					failureCounts[event.EventID] = event.FailureCount
				}
				assert.Equal(t, 0, failureCounts[eventIDs[1]])
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("未配信のイベントが1件あるとき、保存されていないイベントIDに失敗を記録する", func(t *testing.T) {
				t.Run("更新対象が見つからないエラーになる", func(t *testing.T) {
					pg.Truncate(t)
					createUnpublishedEvents(t, 1)

					err := pg.OutboxRepository().RecordFailure(ctx, uuid.New(), "TST-ERROR")

					assert.ErrorIs(t, err, port.ErrNotFound)
				})

				t.Run("保存済みのイベントの失敗回数は0のままになる", func(t *testing.T) {
					pg.Truncate(t)
					createUnpublishedEvents(t, 1)
					require.Error(t, pg.OutboxRepository().RecordFailure(ctx, uuid.New(), "TST-ERROR"))

					claimed := pg.ClaimAll(t)

					require.Len(t, claimed, 1)
					assert.Equal(t, 0, claimed[0].FailureCount)
				})

				t.Run("続けて、保存済みのイベントに失敗を記録すると、失敗回数は1になる", func(t *testing.T) {
					pg.Truncate(t)
					eventIDs := createUnpublishedEvents(t, 1)
					require.Error(t, pg.OutboxRepository().RecordFailure(ctx, uuid.New(), "TST-ERROR"))
					require.NoError(t, pg.OutboxRepository().RecordFailure(ctx, eventIDs[0], "TST-ERROR"))

					claimed := pg.ClaimAll(t)

					require.Len(t, claimed, 1)
					assert.Equal(t, 1, claimed[0].FailureCount)
				})
			})
		})
	})
}
