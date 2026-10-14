//go:build integration

package outbox_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

const (
	dummyPlayerID       = "00000000-0000-4000-8000-0000000000c1"
	priorFailureMessage = "TST-prior-failure"
)

var (
	firstOutboxEvent = port.OutboxEvent{
		EventID:   uuid.MustParse("00000000-0000-4000-8000-0000000000b1"),
		EventType: "TST-EVENT-TYPE-1",
		Payload:   []byte(`{"TST":"payload-1"}`),
	}
	secondOutboxEvent = port.OutboxEvent{
		EventID:   uuid.MustParse("00000000-0000-4000-8000-0000000000b2"),
		EventType: "TST-EVENT-TYPE-2",
		Payload:   []byte(`{"TST":"payload-2"}`),
	}
)

type loggedFailureCase struct {
	name          string
	priorFailures int
	message       string
	level         slog.Level
	wantAttempts  int64
}

var (
	nearThresholdFailure = loggedFailureCase{name: "今回の失敗で連続失敗回数が設定した上限に1回足りないとき", priorFailures: 1, message: "outbox publish failed", level: slog.LevelWarn, wantAttempts: 2}
	thresholdFailure     = loggedFailureCase{name: "今回の失敗で連続失敗回数が設定した上限に達するとき", priorFailures: 2, message: "outbox event stuck", level: slog.LevelError, wantAttempts: 3}
	loggedFailureCases   = []loggedFailureCase{nearThresholdFailure, thresholdFailure}
)

func prepareUnpublishedEvents(t *testing.T, events ...port.OutboxEvent) {
	t.Helper()
	pg.Truncate(t)
	for i, event := range events {
		err := pg.CreateCardPackPurchaseWithEvent(dummyPlayerID, fmt.Sprintf("TST-PACK-%d", i), fmt.Sprintf("TST-TOKEN-%d", i), event)
		require.NoError(t, err)
	}
}

func recordPriorFailures(t *testing.T, eventID uuid.UUID, count int) {
	t.Helper()
	for range count {
		require.NoError(t, pg.OutboxRepository().RecordFailure(context.Background(), eventID, priorFailureMessage))
	}
}

func readLastError(t *testing.T, eventID uuid.UUID) *string {
	t.Helper()
	var lastError *string
	err := pg.Pool.QueryRow(context.Background(), "SELECT last_error FROM shop.outbox_events WHERE event_id = $1", eventID).Scan(&lastError)
	require.NoError(t, err)
	return lastError
}

func deliverOnce(t *testing.T, publisher *fakePublisher) error {
	t.Helper()
	return newRelay(t, pg.OutboxRepository(), publisher, relayConfig).RunOnce(t.Context())
}

func deliverFailingEvent(t *testing.T, priorFailures int) *logtest.Logs {
	t.Helper()
	prepareUnpublishedEvents(t, firstOutboxEvent)
	recordPriorFailures(t, firstOutboxEvent.EventID, priorFailures)
	logs := logtest.Capture(t)
	publisher := &fakePublisher{errorsByEventType: buildPublishErrorsFor(firstOutboxEvent.EventType)}
	require.NoError(t, deliverOnce(t, publisher))
	return logs
}

func singleLogRecord(t *testing.T, logs *logtest.Logs, message string, level slog.Level) logtest.Record {
	t.Helper()
	records := logs.SelectByMessageAndLevel(message, level)
	require.Len(t, records, 1)
	return records[0]
}

func TestRunOnce(t *testing.T) {
	t.Run("[アウトボックス配信]1回の配信実行", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("未配信イベントが0件のとき", func(t *testing.T) {
				t.Run("配信先に送られるイベントが0件になる", func(t *testing.T) {
					prepareUnpublishedEvents(t)
					publisher := &fakePublisher{}

					require.NoError(t, deliverOnce(t, publisher))

					assert.Empty(t, publisher.published)
				})
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					prepareUnpublishedEvents(t)

					err := deliverOnce(t, &fakePublisher{})

					assert.NoError(t, err)
				})
			})
			t.Run("配信に成功する未配信イベントが1件のとき", func(t *testing.T) {
				t.Run("配信先に、そのイベントの種別が送られる", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent)
					publisher := &fakePublisher{}

					require.NoError(t, deliverOnce(t, publisher))

					require.Len(t, publisher.published, 1)
					assert.Equal(t, "TST-EVENT-TYPE-1", publisher.published[0].EventType)
				})
				t.Run("配信先に、そのイベントの本文が保存したJSONと同じ内容で送られる", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent)
					publisher := &fakePublisher{}

					require.NoError(t, deliverOnce(t, publisher))

					require.Len(t, publisher.published, 1)
					assert.JSONEq(t, `{"TST":"payload-1"}`, publisher.published[0].Payload)
				})
				t.Run("未配信イベントを取得し直しても、そのイベントは取得されない", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent)

					require.NoError(t, deliverOnce(t, &fakePublisher{}))

					assert.Empty(t, pg.ClaimAll(t))
				})
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent)

					err := deliverOnce(t, &fakePublisher{})

					assert.NoError(t, err)
				})
			})
			t.Run("配信に成功する未配信イベントが2件のとき", func(t *testing.T) {
				t.Run("2件とも配信先に送られる", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent, secondOutboxEvent)
					publisher := &fakePublisher{}

					require.NoError(t, deliverOnce(t, publisher))

					require.Len(t, publisher.published, 2)
					assert.Equal(t, "TST-EVENT-TYPE-1", publisher.published[0].EventType)
					assert.JSONEq(t, `{"TST":"payload-1"}`, publisher.published[0].Payload)
					assert.Equal(t, "TST-EVENT-TYPE-2", publisher.published[1].EventType)
					assert.JSONEq(t, `{"TST":"payload-2"}`, publisher.published[1].Payload)
				})
				t.Run("未配信イベントを取得し直しても、2件とも取得されない", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent, secondOutboxEvent)

					require.NoError(t, deliverOnce(t, &fakePublisher{}))

					assert.Empty(t, pg.ClaimAll(t))
				})
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("配信先がエラーを返す未配信イベントが1件のとき", func(t *testing.T) {
				t.Run("未配信イベントを取得し直すと、そのイベントの失敗回数が1になる", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent)
					publisher := &fakePublisher{errorsByEventType: buildPublishErrorsFor(firstOutboxEvent.EventType)}

					require.NoError(t, deliverOnce(t, publisher))

					claimed := pg.ClaimAll(t)
					require.Len(t, claimed, 1)
					assert.Equal(t, 1, claimed[0].FailureCount)
				})
				t.Run("そのイベントの直近のエラーが、配信先が返したエラーのメッセージになる", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent)
					publisher := &fakePublisher{errorsByEventType: buildPublishErrorsFor(firstOutboxEvent.EventType)}

					require.NoError(t, deliverOnce(t, publisher))

					lastError := readLastError(t, firstOutboxEvent.EventID)
					require.NotNil(t, lastError)
					assert.Equal(t, "TST-publish-failed", *lastError)
				})
				t.Run("未配信イベントを取得し直すと、そのイベントが取得される", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent)
					publisher := &fakePublisher{errorsByEventType: buildPublishErrorsFor(firstOutboxEvent.EventType)}

					require.NoError(t, deliverOnce(t, publisher))

					assert.Equal(t, []uuid.UUID{firstOutboxEvent.EventID}, postgrestest.ClaimedEventIDs(pg.ClaimAll(t)))
				})
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent)
					publisher := &fakePublisher{errorsByEventType: buildPublishErrorsFor(firstOutboxEvent.EventType)}

					err := deliverOnce(t, publisher)

					assert.NoError(t, err)
				})
			})
			t.Run("配信先がエラーを返すイベントのあとに、配信に成功するイベントが続けて取得されたとき", func(t *testing.T) {
				t.Run("後ろのイベントが配信先に送られる", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent, secondOutboxEvent)
					publisher := &fakePublisher{errorsByEventType: buildPublishErrorsFor(firstOutboxEvent.EventType)}

					require.NoError(t, deliverOnce(t, publisher))

					require.Len(t, publisher.published, 2)
					assert.Equal(t, "TST-EVENT-TYPE-2", publisher.published[1].EventType)
					assert.JSONEq(t, `{"TST":"payload-2"}`, publisher.published[1].Payload)
				})
				t.Run("未配信イベントを取得し直しても、後ろのイベントは取得されない", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent, secondOutboxEvent)
					publisher := &fakePublisher{errorsByEventType: buildPublishErrorsFor(firstOutboxEvent.EventType)}

					require.NoError(t, deliverOnce(t, publisher))

					assert.NotContains(t, postgrestest.ClaimedEventIDs(pg.ClaimAll(t)), secondOutboxEvent.EventID)
				})
				t.Run("未配信イベントを取得し直すと、前のイベントの失敗回数が1になる", func(t *testing.T) {
					prepareUnpublishedEvents(t, firstOutboxEvent, secondOutboxEvent)
					publisher := &fakePublisher{errorsByEventType: buildPublishErrorsFor(firstOutboxEvent.EventType)}

					require.NoError(t, deliverOnce(t, publisher))

					claimed := pg.ClaimAll(t)
					require.Len(t, claimed, 1)
					assert.Equal(t, 1, claimed[0].FailureCount)
				})
			})
			t.Run("配信に失敗したときのログ", func(t *testing.T) {
				t.Run("今回の失敗で連続失敗回数が、設定した上限に1回足りないとき", func(t *testing.T) {
					t.Run("Warnのログが1件だけ出力される", func(t *testing.T) {
						logs := deliverFailingEvent(t, nearThresholdFailure.priorFailures)

						records := logs.Records()
						require.Len(t, records, 1)
						assert.Equal(t, slog.LevelWarn, records[0].Level)
						assert.Equal(t, "outbox publish failed", records[0].Message)
					})
				})
				t.Run("今回の失敗で連続失敗回数が、設定した上限に達するとき", func(t *testing.T) {
					t.Run("Errorのログが1件だけ出力される", func(t *testing.T) {
						logs := deliverFailingEvent(t, thresholdFailure.priorFailures)

						records := logs.Records()
						require.Len(t, records, 1)
						assert.Equal(t, slog.LevelError, records[0].Level)
						assert.Equal(t, "outbox event stuck", records[0].Message)
					})
				})
				t.Run("連続失敗回数ごとのログの内容", func(t *testing.T) {
					for _, tt := range loggedFailureCases {
						t.Run(tt.name+"、ログにイベントの識別子が含まれる", func(t *testing.T) {
							logs := deliverFailingEvent(t, tt.priorFailures)

							record := singleLogRecord(t, logs, tt.message, tt.level)

							assert.Equal(t, "00000000-0000-4000-8000-0000000000b1", record.String("event_id"))
						})
					}
					for _, tt := range loggedFailureCases {
						t.Run(tt.name+"、ログにイベントの種別が含まれる", func(t *testing.T) {
							logs := deliverFailingEvent(t, tt.priorFailures)

							record := singleLogRecord(t, logs, tt.message, tt.level)

							assert.Equal(t, "TST-EVENT-TYPE-1", record.String("event_type"))
						})
					}
					for _, tt := range loggedFailureCases {
						t.Run(tt.name+"、ログの失敗回数が、これまでの連続失敗回数に今回の失敗を足した回数になる", func(t *testing.T) {
							logs := deliverFailingEvent(t, tt.priorFailures)

							record := singleLogRecord(t, logs, tt.message, tt.level)

							assert.Equal(t, tt.wantAttempts, record.Int("attempts"))
						})
					}
					for _, tt := range loggedFailureCases {
						t.Run(tt.name+"、ログにエラーの内容として、配信先が返したエラーのメッセージが含まれる", func(t *testing.T) {
							logs := deliverFailingEvent(t, tt.priorFailures)

							record := singleLogRecord(t, logs, tt.message, tt.level)

							assert.Contains(t, record.String("error"), "TST-publish-failed")
						})
					}
				})
			})
		})
	})
}
