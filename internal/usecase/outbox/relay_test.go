package outbox_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/outbox"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

type failureLogCase struct {
	name         string
	failureCount int
	message      string
	level        slog.Level
	wantAttempts int64
}

var (
	oneShortOfThreshold = failureLogCase{name: "今回の失敗で連続失敗回数が設定した上限に1回足りないとき", failureCount: 1, message: "outbox publish failed", level: slog.LevelWarn, wantAttempts: 2}
	reachesThreshold    = failureLogCase{name: "今回の失敗で連続失敗回数が設定した上限に達するとき", failureCount: 2, message: "outbox event stuck", level: slog.LevelError, wantAttempts: 3}
	failureLogCases     = []failureLogCase{oneShortOfThreshold, reachesThreshold}
)

type recordingFailureCase struct {
	name                string
	message             string
	publishErrors       map[string]error
	markErrors          map[uuid.UUID]error
	recordFailureErrors map[uuid.UUID]error
}

var recordingFailureCases = []recordingFailureCase{
	{
		name:       "配信に成功したイベントの配信済みの記録に失敗したとき",
		message:    "mark published failed",
		markErrors: map[uuid.UUID]error{firstEvent.EventID: errRecord},
	},
	{
		name:                "配信に失敗したイベントの失敗の記録に失敗したとき",
		message:             "record failure failed",
		publishErrors:       buildPublishErrorsFor(firstEvent),
		recordFailureErrors: map[uuid.UUID]error{firstEvent.EventID: errRecord},
	},
}

type recordingFailureRun struct {
	logs      *logtest.Logs
	publisher *fakePublisher
	err       error
}

func runRecordingFailure(t *testing.T, tt recordingFailureCase) recordingFailureRun {
	t.Helper()
	logs := logtest.Capture(t)
	store := &fakeStore{
		claimed:             []port.ClaimedOutboxEvent{firstEvent, secondEvent},
		markErrors:          tt.markErrors,
		recordFailureErrors: tt.recordFailureErrors,
	}
	publisher := &fakePublisher{errorsByPayload: tt.publishErrors}
	err := runRelay(t, store, publisher)
	return recordingFailureRun{logs: logs, publisher: publisher, err: err}
}

func runRelay(t *testing.T, store *fakeStore, publisher *fakePublisher) error {
	t.Helper()
	return newRelay(t, store, publisher, relayConfig).RunOnce(t.Context())
}

func runFailingEvent(t *testing.T, failureCount int) *logtest.Logs {
	t.Helper()
	logs := logtest.Capture(t)
	store := &fakeStore{claimed: []port.ClaimedOutboxEvent{buildEventWithFailureCount(firstEvent, failureCount)}}
	publisher := &fakePublisher{errorsByPayload: buildPublishErrorsFor(firstEvent)}
	require.NoError(t, runRelay(t, store, publisher))
	return logs
}

func findSingleRecord(t *testing.T, logs *logtest.Logs, message string, level slog.Level) logtest.Record {
	t.Helper()
	records := logs.SelectByMessageAndLevel(message, level)
	require.Len(t, records, 1)
	return records[0]
}

func TestNew(t *testing.T) {
	t.Run("[アウトボックス配信]配信設定の検証", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("1回に取得する件数が0のとき、生成は取得件数が正でないことを示すエラーで失敗する", func(t *testing.T) {
				config := relayConfig
				config.BatchSize = 0

				_, err := outbox.New(&fakeStore{}, &fakePublisher{}, config)

				assert.ErrorContains(t, err, "BatchSize must be positive")
			})
			t.Run("失敗回数の上限が0のとき、生成は失敗回数の上限が正でないことを示すエラーで失敗する", func(t *testing.T) {
				config := relayConfig
				config.FailureThreshold = 0

				_, err := outbox.New(&fakeStore{}, &fakePublisher{}, config)

				assert.ErrorContains(t, err, "FailureThreshold must be positive")
			})
			t.Run("再取得を止める期間が0のとき、生成は再取得を止める期間が正でないことを示すエラーで失敗する", func(t *testing.T) {
				config := relayConfig
				config.VisibilityTimeout = 0

				_, err := outbox.New(&fakeStore{}, &fakePublisher{}, config)

				assert.ErrorContains(t, err, "VisibilityTimeout must be positive")
			})
		})
		t.Run("正常系", func(t *testing.T) {
			tests := []struct {
				name   string
				config outbox.Config
			}{
				{"1回に取得する件数が1のとき", outbox.Config{BatchSize: 1, FailureThreshold: 3, VisibilityTimeout: 30 * time.Second}},
				{"失敗回数の上限が1のとき", outbox.Config{BatchSize: 10, FailureThreshold: 1, VisibilityTimeout: 30 * time.Second}},
				{"再取得を止める期間が1ナノ秒のとき", outbox.Config{BatchSize: 10, FailureThreshold: 3, VisibilityTimeout: time.Nanosecond}},
			}
			for _, tt := range tests {
				t.Run(tt.name+"、生成に成功する", func(t *testing.T) {
					_, err := outbox.New(&fakeStore{}, &fakePublisher{}, tt.config)

					assert.NoError(t, err)
				})
			}
		})
	})
}

func TestRunOnce(t *testing.T) {
	t.Run("[アウトボックス配信]1回の配信実行", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("未配信イベントが0件のとき", func(t *testing.T) {
				t.Run("配信先に送られるイベントが0件になる", func(t *testing.T) {
					publisher := &fakePublisher{}

					require.NoError(t, runRelay(t, &fakeStore{}, publisher))

					assert.Empty(t, publisher.published)
				})
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					err := runRelay(t, &fakeStore{}, &fakePublisher{})

					assert.NoError(t, err)
				})
			})
			t.Run("配信に成功する未配信イベントが1件のとき", func(t *testing.T) {
				t.Run("配信先に、そのイベントの種別が送られる", func(t *testing.T) {
					publisher := &fakePublisher{}

					require.NoError(t, runRelay(t, &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}, publisher))

					require.Len(t, publisher.published, 1)
					assert.Equal(t, "TST-EVENT-TYPE-1", publisher.published[0].EventType)
				})
				t.Run("配信先に、そのイベントの本文がそのまま送られる", func(t *testing.T) {
					publisher := &fakePublisher{}

					require.NoError(t, runRelay(t, &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}, publisher))

					require.Len(t, publisher.published, 1)
					assert.Equal(t, `{"TST":"payload-1"}`, publisher.published[0].Payload)
				})
				t.Run("そのイベントが配信済みとして記録される", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}

					require.NoError(t, runRelay(t, store, &fakePublisher{}))

					assert.Equal(t, []uuid.UUID{uuid.MustParse("00000000-0000-4000-8000-0000000000a1")}, store.markedPublished)
				})
				t.Run("失敗として記録されるイベントが0件になる", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}

					require.NoError(t, runRelay(t, store, &fakePublisher{}))

					assert.Empty(t, store.failures)
				})
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					err := runRelay(t, &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}, &fakePublisher{})

					assert.NoError(t, err)
				})
			})
			t.Run("配信に成功する未配信イベントが2件のとき", func(t *testing.T) {
				t.Run("2件とも配信先に送られる", func(t *testing.T) {
					publisher := &fakePublisher{}

					require.NoError(t, runRelay(t, &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent, secondEvent}}, publisher))

					assert.ElementsMatch(t, []publishedEvent{
						{EventType: "TST-EVENT-TYPE-1", Payload: `{"TST":"payload-1"}`},
						{EventType: "TST-EVENT-TYPE-2", Payload: `{"TST":"payload-2"}`},
					}, publisher.published)
				})
				t.Run("2件とも配信済みとして記録される", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent, secondEvent}}

					require.NoError(t, runRelay(t, store, &fakePublisher{}))

					assert.ElementsMatch(t, []uuid.UUID{
						uuid.MustParse("00000000-0000-4000-8000-0000000000a1"),
						uuid.MustParse("00000000-0000-4000-8000-0000000000a2"),
					}, store.markedPublished)
				})
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("未配信イベントの取得に失敗したとき", func(t *testing.T) {
				t.Run("実行は、取得のエラーのメッセージを含むエラーで失敗する", func(t *testing.T) {
					err := runRelay(t, &fakeStore{claimErr: errClaim}, &fakePublisher{})

					assert.ErrorContains(t, err, "TST-claim-failed")
				})
			})
			t.Run("配信先がエラーを返す未配信イベントが1件のとき", func(t *testing.T) {
				t.Run("そのイベントが失敗として記録される", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}
					publisher := &fakePublisher{errorsByPayload: buildPublishErrorsFor(firstEvent)}

					require.NoError(t, runRelay(t, store, publisher))

					require.Len(t, store.failures, 1)
					assert.Equal(t, uuid.MustParse("00000000-0000-4000-8000-0000000000a1"), store.failures[0].EventID)
				})
				t.Run("失敗の内容として、配信先が返したエラーのメッセージが記録される", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}
					publisher := &fakePublisher{errorsByPayload: buildPublishErrorsFor(firstEvent)}

					require.NoError(t, runRelay(t, store, publisher))

					require.Len(t, store.failures, 1)
					assert.Equal(t, "TST-publish-failed", store.failures[0].Message)
				})
				t.Run("そのイベントは配信済みとして記録されない", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}
					publisher := &fakePublisher{errorsByPayload: buildPublishErrorsFor(firstEvent)}

					require.NoError(t, runRelay(t, store, publisher))

					assert.Empty(t, store.markedPublished)
				})
				t.Run("実行はエラーを返さない", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent}}
					publisher := &fakePublisher{errorsByPayload: buildPublishErrorsFor(firstEvent)}

					err := runRelay(t, store, publisher)

					assert.NoError(t, err)
				})
			})
			t.Run("配信先がエラーを返すイベントのあとに、配信に成功するイベントが続けて取得されたとき", func(t *testing.T) {
				t.Run("後ろのイベントが配信先に送られる", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent, secondEvent}}
					publisher := &fakePublisher{errorsByPayload: buildPublishErrorsFor(firstEvent)}

					require.NoError(t, runRelay(t, store, publisher))

					assert.Contains(t, publisher.published, publishedEvent{EventType: "TST-EVENT-TYPE-2", Payload: `{"TST":"payload-2"}`})
				})
				t.Run("後ろのイベントが配信済みとして記録される", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent, secondEvent}}
					publisher := &fakePublisher{errorsByPayload: buildPublishErrorsFor(firstEvent)}

					require.NoError(t, runRelay(t, store, publisher))

					assert.Equal(t, []uuid.UUID{uuid.MustParse("00000000-0000-4000-8000-0000000000a2")}, store.markedPublished)
				})
				t.Run("前のイベントが失敗として記録される", func(t *testing.T) {
					store := &fakeStore{claimed: []port.ClaimedOutboxEvent{firstEvent, secondEvent}}
					publisher := &fakePublisher{errorsByPayload: buildPublishErrorsFor(firstEvent)}

					require.NoError(t, runRelay(t, store, publisher))

					require.Len(t, store.failures, 1)
					assert.Equal(t, uuid.MustParse("00000000-0000-4000-8000-0000000000a1"), store.failures[0].EventID)
				})
			})
			t.Run("配信に失敗したときのログ", func(t *testing.T) {
				t.Run("今回の失敗で連続失敗回数が、設定した上限に1回足りないとき", func(t *testing.T) {
					t.Run("Warnのログが1件だけ出力される", func(t *testing.T) {
						logs := runFailingEvent(t, oneShortOfThreshold.failureCount)

						records := logs.Records()
						require.Len(t, records, 1)
						assert.Equal(t, slog.LevelWarn, records[0].Level)
						assert.Equal(t, "outbox publish failed", records[0].Message)
					})
				})
				t.Run("今回の失敗で連続失敗回数が、設定した上限に達するとき", func(t *testing.T) {
					t.Run("Errorのログが1件だけ出力される", func(t *testing.T) {
						logs := runFailingEvent(t, reachesThreshold.failureCount)

						records := logs.Records()
						require.Len(t, records, 1)
						assert.Equal(t, slog.LevelError, records[0].Level)
						assert.Equal(t, "outbox event stuck", records[0].Message)
					})
				})
				t.Run("連続失敗回数ごとのログの内容", func(t *testing.T) {
					for _, tt := range failureLogCases {
						t.Run(tt.name+"、ログにイベントの識別子が含まれる", func(t *testing.T) {
							logs := runFailingEvent(t, tt.failureCount)

							record := findSingleRecord(t, logs, tt.message, tt.level)

							assert.Equal(t, "00000000-0000-4000-8000-0000000000a1", record.String("event_id"))
						})
					}
					for _, tt := range failureLogCases {
						t.Run(tt.name+"、ログにイベントの種別が含まれる", func(t *testing.T) {
							logs := runFailingEvent(t, tt.failureCount)

							record := findSingleRecord(t, logs, tt.message, tt.level)

							assert.Equal(t, "TST-EVENT-TYPE-1", record.String("event_type"))
						})
					}
					for _, tt := range failureLogCases {
						t.Run(tt.name+"、ログの失敗回数が、これまでの連続失敗回数に今回の失敗を足した回数になる", func(t *testing.T) {
							logs := runFailingEvent(t, tt.failureCount)

							record := findSingleRecord(t, logs, tt.message, tt.level)

							assert.Equal(t, tt.wantAttempts, record.Int("attempts"))
						})
					}
					for _, tt := range failureLogCases {
						t.Run(tt.name+"、ログにエラーの内容として、配信先が返したエラーのメッセージが含まれる", func(t *testing.T) {
							logs := runFailingEvent(t, tt.failureCount)

							record := findSingleRecord(t, logs, tt.message, tt.level)

							assert.Contains(t, record.String("error"), "TST-publish-failed")
						})
					}
				})
			})
			t.Run("配信の成功か失敗に続く記録が失敗したとき", func(t *testing.T) {
				for _, tt := range recordingFailureCases {
					t.Run(tt.name+"、Errorのログが1件出力される", func(t *testing.T) {
						run := runRecordingFailure(t, tt)

						assert.Len(t, run.logs.SelectByMessageAndLevel(tt.message, slog.LevelError), 1)
					})
				}
				for _, tt := range recordingFailureCases {
					t.Run(tt.name+"、ログにイベントの識別子が含まれる", func(t *testing.T) {
						run := runRecordingFailure(t, tt)

						record := findSingleRecord(t, run.logs, tt.message, slog.LevelError)

						assert.Equal(t, "00000000-0000-4000-8000-0000000000a1", record.String("event_id"))
					})
				}
				for _, tt := range recordingFailureCases {
					t.Run(tt.name+"、ログにエラーの内容として、記録に失敗したときのエラーのメッセージが含まれる", func(t *testing.T) {
						run := runRecordingFailure(t, tt)

						record := findSingleRecord(t, run.logs, tt.message, slog.LevelError)

						assert.Contains(t, record.String("error"), "TST-record-failed")
					})
				}
				for _, tt := range recordingFailureCases {
					t.Run(tt.name+"、実行はエラーを返さない", func(t *testing.T) {
						run := runRecordingFailure(t, tt)

						assert.NoError(t, run.err)
					})
				}
				for _, tt := range recordingFailureCases {
					t.Run(tt.name+"、同じ取得で続く次のイベントも配信先に送られる", func(t *testing.T) {
						run := runRecordingFailure(t, tt)

						assert.Contains(t, run.publisher.published, publishedEvent{EventType: "TST-EVENT-TYPE-2", Payload: `{"TST":"payload-2"}`})
					})
				}
			})
		})
	})
}
