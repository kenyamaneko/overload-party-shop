package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
)

// fakeStore は outboxStore の簡易モック。ProcessBatch が呼ばれた回数と、
// 最後に渡された publish 関数を記録する。worker のポーリングと publish 委譲だけを
// 観察するのが目的で、本物の postgres は使わない。
type fakeStore struct {
	calls       int
	succeeded   int
	failed      int
	err         error
	onPublish   func(ctx context.Context, ev postgres.ClaimedOutboxEvent) error
	claimedRows []postgres.ClaimedOutboxEvent
}

func (f *fakeStore) ProcessBatch(
	ctx context.Context,
	_ int,
	publish func(ctx context.Context, ev postgres.ClaimedOutboxEvent) error,
) (int, int, error) {
	f.calls++
	f.onPublish = publish
	for _, row := range f.claimedRows {
		if err := publish(ctx, row); err != nil {
			f.failed++
			continue
		}
		f.succeeded++
	}
	return f.succeeded, f.failed, f.err
}

type fakePub struct {
	calls []string // topic-payload 文字列
	err   error
}

func (p *fakePub) Publish(_ context.Context, topic string, payload []byte) error {
	p.calls = append(p.calls, topic+":"+string(payload))
	return p.err
}

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"PollInterval 0", Config{PollInterval: 0, BatchSize: 1, FailureThreshold: 1}},
		{"BatchSize 0", Config{PollInterval: time.Second, BatchSize: 0, FailureThreshold: 1}},
		{"FailureThreshold 0", Config{PollInterval: time.Second, BatchSize: 1, FailureThreshold: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(postgres.NewOutboxRepository(nil), &fakePub{}, tt.cfg)
			require.Error(t, err)
		})
	}
}

// Run は ctx キャンセルで return する。tick が一度も発火しなくても停止できる。
func TestOutboxPublisher_Run_StopsOnContextCancel(t *testing.T) {
	store := &fakeStore{}
	pub := &fakePub{}
	w := &OutboxPublisher{
		store: store, pub: pub,
		pollInterval: 10 * time.Millisecond, batchSize: 10, failureThreshold: 3,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not stop after cancel")
	}
	assert.GreaterOrEqual(t, store.calls, 1, "tick が少なくとも 1 回発火している")
}

// worker は claim した行を Publisher に委譲する (topic + payload そのまま)。
func TestOutboxPublisher_DelegatesPublishToPubSub(t *testing.T) {
	store := &fakeStore{
		claimedRows: []postgres.ClaimedOutboxEvent{
			{EventID: uuid.New(), Topic: "faction-selected", Payload: []byte(`{"k":"v"}`), FailureCount: 0},
		},
	}
	pub := &fakePub{}
	w := &OutboxPublisher{
		store: store, pub: pub,
		pollInterval: time.Second, batchSize: 10, failureThreshold: 3,
	}

	require.NoError(t, w.runOnce(context.Background()))
	assert.Equal(t, []string{`faction-selected:{"k":"v"}`}, pub.calls)
}

// Publisher が失敗するとエラーは ProcessBatch 内の publish 関数 → fakeStore の failed 集計
// に落ち、runOnce 自体はエラーにならない (DB エラーではないため)。
func TestOutboxPublisher_PublishFailure_DoesNotReturnError(t *testing.T) {
	store := &fakeStore{
		claimedRows: []postgres.ClaimedOutboxEvent{
			{EventID: uuid.New(), Topic: "faction-selected", Payload: []byte(`{}`), FailureCount: 0},
		},
	}
	pub := &fakePub{err: errors.New("pubsub down")}
	w := &OutboxPublisher{
		store: store, pub: pub,
		pollInterval: time.Second, batchSize: 10, failureThreshold: 3,
	}

	err := w.runOnce(context.Background())
	require.NoError(t, err, "publish 失敗は worker エラーに昇格しない (次 tick で自動再試行)")
	assert.Equal(t, 1, store.failed)
}

// DB エラー (store.ProcessBatch 自体が失敗) は runOnce の戻りエラーになる。
func TestOutboxPublisher_StoreError_Surfaces(t *testing.T) {
	store := &fakeStore{err: errors.New("db down")}
	w := &OutboxPublisher{
		store: store, pub: &fakePub{},
		pollInterval: time.Second, batchSize: 10, failureThreshold: 3,
	}

	err := w.runOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
