package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner は outboxRunner (outbox.Relay のテスト代替)。
// RunOnce の呼び出し回数を原子的にカウントし、指定エラーを返す。
type fakeRunner struct {
	calls int32
	err   error
}

func (f *fakeRunner) RunOnce(_ context.Context) error {
	atomic.AddInt32(&f.calls, 1)
	return f.err
}

func (f *fakeRunner) Calls() int32 { return atomic.LoadInt32(&f.calls) }

func TestNewOutboxTicker_Validation(t *testing.T) {
	tests := []struct {
		name     string
		runner   outboxRunner
		interval time.Duration
		wantSub  string
	}{
		{
			name:     "runner が nil",
			runner:   nil,
			interval: time.Second,
			wantSub:  "runner is nil",
		},
		{
			name:     "interval が 0",
			runner:   &fakeRunner{},
			interval: 0,
			wantSub:  "interval must be positive",
		},
		{
			name:     "interval が負",
			runner:   &fakeRunner{},
			interval: -time.Second,
			wantSub:  "interval must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOutboxTicker(tt.runner, tt.interval)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

// Run は ctx の状態と runner の挙動で 2 つの不変条件を満たす:
//  1. ctx cancel で return する
//  2. runner が tick ごとに呼ばれる (エラーを返しても停止しない)
// 各ケースを runner 設定 + 期待する最小呼び出し回数で固定する。
func TestOutboxTicker_Run(t *testing.T) {
	tests := []struct {
		name          string
		runnerErr     error
		sleep         time.Duration
		wantMinCalls  int32
	}{
		{
			name:         "正常系: tick ごとに RunOnce を呼び、cancel で停止する",
			runnerErr:    nil,
			sleep:        35 * time.Millisecond,
			wantMinCalls: 1,
		},
		{
			name:         "RunOnce がエラーを返しても ticker は継続する",
			runnerErr:    errors.New("transient"),
			sleep:        35 * time.Millisecond,
			wantMinCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{err: tt.runnerErr}
			ticker, err := NewOutboxTicker(runner, 10*time.Millisecond)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- ticker.Run(ctx) }()

			time.Sleep(tt.sleep)
			cancel()

			select {
			case err := <-done:
				require.NoError(t, err, "ctx cancel では正常終了")
			case <-time.After(500 * time.Millisecond):
				t.Fatal("ticker did not stop after cancel")
			}
			assert.GreaterOrEqual(t, runner.Calls(), tt.wantMinCalls, "tick が期待回数以上発火している")
		})
	}
}
