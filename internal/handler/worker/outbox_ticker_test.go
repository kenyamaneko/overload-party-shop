package worker_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/handler/worker"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

const (
	tickInterval    = 10 * time.Millisecond
	awaitDeadline   = 10 * time.Second
	callsBufferSize = 1024
)

type fakeRunner struct {
	calls chan int

	mu          sync.Mutex
	count       int
	contextErrs []error
	onRun       func(ctx context.Context, call int) error
}

func newFakeRunner(onRun func(ctx context.Context, call int) error) *fakeRunner {
	return &fakeRunner{calls: make(chan int, callsBufferSize), onRun: onRun}
}

func newSucceedingRunner() *fakeRunner {
	return newFakeRunner(func(context.Context, int) error { return nil })
}

func (r *fakeRunner) RunOnce(ctx context.Context) error {
	r.mu.Lock()
	r.count++
	call := r.count
	r.mu.Unlock()
	r.calls <- call
	err := r.onRun(ctx, call)
	r.mu.Lock()
	r.contextErrs = append(r.contextErrs, ctx.Err())
	r.mu.Unlock()
	return err
}

func (r *fakeRunner) contextErrors() []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]error(nil), r.contextErrs...)
}

func (r *fakeRunner) awaitCalls(t *testing.T, want int) int {
	t.Helper()
	deadline := time.After(awaitDeadline)
	lastCall := 0
	for received := 0; received < want; received++ {
		select {
		case lastCall = <-r.calls:
		case <-deadline:
			require.FailNow(t, "実行対象の処理の呼び出しが期限内に揃わなかった", "待った回数: %d 受け取った回数: %d", want, received)
		}
	}
	return lastCall
}

type runningTicker struct {
	cancel   context.CancelFunc
	finished chan struct{}
	err      error
}

func startTicker(t *testing.T, runner *fakeRunner) *runningTicker {
	t.Helper()
	ticker, err := worker.NewOutboxTicker(runner, tickInterval)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	running := &runningTicker{cancel: cancel, finished: make(chan struct{})}
	go func() {
		running.err = ticker.Run(ctx)
		close(running.finished)
	}()
	t.Cleanup(func() { running.stopAndWait(t) })
	return running
}

func (r *runningTicker) stopAndWait(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case <-r.finished:
	case <-time.After(awaitDeadline):
		require.FailNow(t, "停止の要求の後、期限内に終了しなかった")
	}
}

func (r *runningTicker) stop(t *testing.T) error {
	t.Helper()
	r.stopAndWait(t)
	return r.err
}

func TestOutboxTickerPeriodicRun(t *testing.T) {
	t.Run("[outbox送信]周期実行", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("設定した実行間隔の3回分が経過すると、実行対象の処理の3回目の呼び出しが行われる", func(t *testing.T) {
				runner := newSucceedingRunner()
				startTicker(t, runner)

				lastCall := runner.awaitCalls(t, 3)

				assert.Equal(t, 3, lastCall)
			})

			t.Run("起動したとき", func(t *testing.T) {
				t.Run("起動を示すInfoレベルのログが出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					runner := newSucceedingRunner()
					startTicker(t, runner)
					runner.awaitCalls(t, 1)

					assert.Len(t, logs.SelectByMessageAndLevel("outbox ticker started", slog.LevelInfo), 1)
				})

				t.Run("起動を示すログのintervalが、設定した実行間隔になる", func(t *testing.T) {
					logs := logtest.Capture(t)
					runner := newSucceedingRunner()
					startTicker(t, runner)
					runner.awaitCalls(t, 1)

					records := logs.SelectByMessage("outbox ticker started")
					require.Len(t, records, 1)
					assert.Equal(t, 10*time.Millisecond, records[0].Duration("interval"))
				})
			})

			t.Run("停止の要求が出たとき", func(t *testing.T) {
				t.Run("起動した処理はエラーなしで終了する", func(t *testing.T) {
					runner := newSucceedingRunner()
					running := startTicker(t, runner)
					runner.awaitCalls(t, 1)

					err := running.stop(t)

					assert.NoError(t, err)
				})

				t.Run("停止時に、停止を示すInfoレベルのログが出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					runner := newSucceedingRunner()
					running := startTicker(t, runner)
					runner.awaitCalls(t, 1)

					running.stopAndWait(t)

					assert.Len(t, logs.SelectByMessageAndLevel("outbox ticker stopping", slog.LevelInfo), 1)
				})

				t.Run("実行対象の処理の実行中に停止の要求が出たとき、その処理に渡された停止の合図も取り消される", func(t *testing.T) {
					runner := newFakeRunner(func(ctx context.Context, _ int) error {
						<-ctx.Done()
						return nil
					})
					running := startTicker(t, runner)
					runner.awaitCalls(t, 1)

					running.stopAndWait(t)

					contextErrs := runner.contextErrors()
					require.Len(t, contextErrs, 1)
					assert.ErrorIs(t, contextErrs[0], context.Canceled)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("実行対象の処理が1回目の呼び出しでエラーを返すとき", func(t *testing.T) {
				t.Run("実行の失敗を示すErrorレベルのログが出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					runner := newFirstCallFailingRunner()
					startTicker(t, runner)
					runner.awaitCalls(t, 2)

					assert.Len(t, logs.SelectByMessageAndLevel("outbox tick failed", slog.LevelError), 1)
				})

				t.Run("実行の失敗を示すログのerrorに、実行対象の処理が返したエラーの内容が含まれる", func(t *testing.T) {
					logs := logtest.Capture(t)
					runner := newFirstCallFailingRunner()
					startTicker(t, runner)
					runner.awaitCalls(t, 2)

					records := logs.SelectByMessage("outbox tick failed")
					require.Len(t, records, 1)
					assert.Contains(t, records[0].String("error"), "TST-RUN-FAILURE")
				})

				t.Run("実行間隔が経過した後の2回目の呼び出しも行われる", func(t *testing.T) {
					runner := newFirstCallFailingRunner()
					startTicker(t, runner)

					lastCall := runner.awaitCalls(t, 2)

					assert.Equal(t, 2, lastCall)
				})
			})
		})
	})
}

func newFirstCallFailingRunner() *fakeRunner {
	failuresByCall := map[int]error{1: errors.New("TST-RUN-FAILURE")}
	return newFakeRunner(func(_ context.Context, call int) error {
		return failuresByCall[call]
	})
}
