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
		publishErrors:       buildPublishErrorsFor(firstEvent.EventType),
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
	publisher := &fakePublisher{errorsByEventType: tt.publishErrors}
	err := newRelay(t, store, publisher, relayConfig).RunOnce(t.Context())
	return recordingFailureRun{logs: logs, publisher: publisher, err: err}
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
			t.Run("1回に取得する件数が0のとき、検証は取得件数が正でないことを示すエラーで失敗する", func(t *testing.T) {
				config := relayConfig
				config.BatchSize = 0

				_, err := outbox.New(&fakeStore{}, &fakePublisher{}, config)

				assert.ErrorContains(t, err, "BatchSize must be positive")
			})
			t.Run("失敗回数の上限が0のとき、検証は失敗回数の上限が正でないことを示すエラーで失敗する", func(t *testing.T) {
				config := relayConfig
				config.FailureThreshold = 0

				_, err := outbox.New(&fakeStore{}, &fakePublisher{}, config)

				assert.ErrorContains(t, err, "FailureThreshold must be positive")
			})
			t.Run("再取得を止める期間が0のとき、検証は再取得を止める期間が正でないことを示すエラーで失敗する", func(t *testing.T) {
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
				t.Run(tt.name+"、検証に成功する", func(t *testing.T) {
					_, err := outbox.New(&fakeStore{}, &fakePublisher{}, tt.config)

					assert.NoError(t, err)
				})
			}
		})
	})
}

func TestRunOnceStoreFailure(t *testing.T) {
	t.Run("[アウトボックス配信]保存先の失敗時の実行", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("保存先が未配信イベントの取得でエラーを返すとき、実行は、保存先が返したエラーのメッセージを含むエラーで失敗する", func(t *testing.T) {
				relay := newRelay(t, &fakeStore{claimErr: errClaim}, &fakePublisher{}, relayConfig)

				err := relay.RunOnce(t.Context())

				assert.ErrorContains(t, err, "TST-claim-failed")
			})
			t.Run("保存先が、配信の成功か失敗に続く記録でエラーを返すとき", func(t *testing.T) {
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
					t.Run(tt.name+"、ログにエラーの内容として、保存先が返したエラーのメッセージが含まれる", func(t *testing.T) {
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
