package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// fakeOutboxStore は port.OutboxStore の簡易モック。claim 系の返り値と各メソッド
// 呼び出しを記録し、RunOnce が claim → publish → mark/fail の順序で呼ぶことを観察する。
type fakeOutboxStore struct {
	claimed          []port.ClaimedOutboxEvent
	claimErr         error
	markedPublished  []uuid.UUID
	markErr          error
	failures         []failureCall
	failErr          error
	claimCalls            int
	lastVisibilityTO      time.Duration
	lastLimit             int
	lastFailureThreshold  int
}

type failureCall struct {
	eventID uuid.UUID
	errMsg  string
}

func (f *fakeOutboxStore) ClaimUnpublished(_ context.Context, limit int, visibilityTimeout time.Duration, failureThreshold int) ([]port.ClaimedOutboxEvent, error) {
	f.claimCalls++
	f.lastLimit = limit
	f.lastVisibilityTO = visibilityTimeout
	f.lastFailureThreshold = failureThreshold
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return f.claimed, nil
}

func (f *fakeOutboxStore) MarkPublished(_ context.Context, eventID uuid.UUID) error {
	f.markedPublished = append(f.markedPublished, eventID)
	return f.markErr
}

func (f *fakeOutboxStore) RecordFailure(_ context.Context, eventID uuid.UUID, errMsg string) error {
	f.failures = append(f.failures, failureCall{eventID: eventID, errMsg: errMsg})
	return f.failErr
}

// fakeRawPublisher は topic 単位の publish 結果を制御できるモック。
// デフォルトは全 topic 成功、errByTopic で個別に失敗を指示する。
type fakeRawPublisher struct {
	calls      []string // topic:payload
	errByTopic map[string]error
}

func (p *fakeRawPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	p.calls = append(p.calls, topic+":"+string(payload))
	if err, ok := p.errByTopic[topic]; ok {
		return err
	}
	return nil
}

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name    string
		store   port.OutboxStore
		pub     port.RawEventPublisher
		cfg     Config
		wantSub string
	}{
		{
			name:    "store が nil",
			store:   nil,
			pub:     &fakeRawPublisher{},
			cfg:     Config{BatchSize: 1, FailureThreshold: 1, VisibilityTimeout: time.Second},
			wantSub: "store is nil",
		},
		{
			name:    "publisher が nil",
			store:   &fakeOutboxStore{},
			pub:     nil,
			cfg:     Config{BatchSize: 1, FailureThreshold: 1, VisibilityTimeout: time.Second},
			wantSub: "publisher is nil",
		},
		{
			name:    "BatchSize が 0",
			store:   &fakeOutboxStore{},
			pub:     &fakeRawPublisher{},
			cfg:     Config{BatchSize: 0, FailureThreshold: 1, VisibilityTimeout: time.Second},
			wantSub: "BatchSize must be positive",
		},
		{
			name:    "FailureThreshold が 0",
			store:   &fakeOutboxStore{},
			pub:     &fakeRawPublisher{},
			cfg:     Config{BatchSize: 1, FailureThreshold: 0, VisibilityTimeout: time.Second},
			wantSub: "FailureThreshold must be positive",
		},
		{
			name:    "VisibilityTimeout が 0",
			store:   &fakeOutboxStore{},
			pub:     &fakeRawPublisher{},
			cfg:     Config{BatchSize: 1, FailureThreshold: 1, VisibilityTimeout: 0},
			wantSub: "VisibilityTimeout must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.store, tt.pub, tt.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

// RunOnce の各ケースを claim 返却 + publish 結果指定 + 期待する mark/fail 呼び出し
// で表現する。各ケースは自己完結 (runner に if を入れない)。
func TestPublisher_RunOnce(t *testing.T) {
	okID := uuid.New()
	ngID := uuid.New()

	tests := []struct {
		name             string
		claimed          []port.ClaimedOutboxEvent
		publishErrs      map[string]error // topic → error
		wantMarked       []uuid.UUID
		wantFailed       []uuid.UUID
		wantPublishCalls int
	}{
		{
			name:    "claim 0 件なら publish も mark も呼ばれない",
			claimed: nil,
		},
		{
			name: "全件 publish 成功で全件 MarkPublished",
			claimed: []port.ClaimedOutboxEvent{
				{EventID: okID, Topic: apishop.TopicFactionPurchased, Payload: []byte(`{}`), FailureCount: 0},
			},
			wantMarked:       []uuid.UUID{okID},
			wantPublishCalls: 1,
		},
		{
			name: "publish 失敗で RecordFailure を呼び、MarkPublished は呼ばない",
			claimed: []port.ClaimedOutboxEvent{
				{EventID: ngID, Topic: apishop.TopicFactionPurchased, Payload: []byte(`{}`), FailureCount: 0},
			},
			publishErrs:      map[string]error{apishop.TopicFactionPurchased: errors.New("pubsub down")},
			wantFailed:       []uuid.UUID{ngID},
			wantPublishCalls: 1,
		},
		{
			name: "混在バッチ: 成功行と失敗行が独立に処理される",
			claimed: []port.ClaimedOutboxEvent{
				{EventID: okID, Topic: "ok-topic", Payload: []byte(`{}`), FailureCount: 0},
				{EventID: ngID, Topic: "ng-topic", Payload: []byte(`{}`), FailureCount: 2},
			},
			publishErrs:      map[string]error{"ng-topic": errors.New("nope")},
			wantMarked:       []uuid.UUID{okID},
			wantFailed:       []uuid.UUID{ngID},
			wantPublishCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeOutboxStore{claimed: tt.claimed}
			pub := &fakeRawPublisher{errByTopic: tt.publishErrs}
			s, err := New(store, pub, Config{
				BatchSize: 10, FailureThreshold: 5, VisibilityTimeout: 30 * time.Second,
			})
			require.NoError(t, err)

			require.NoError(t, s.RunOnce(context.Background()))

			assert.Len(t, pub.calls, tt.wantPublishCalls)
			assert.ElementsMatch(t, tt.wantMarked, store.markedPublished)

			gotFailed := make([]uuid.UUID, 0, len(store.failures))
			for _, f := range store.failures {
				gotFailed = append(gotFailed, f.eventID)
			}
			assert.ElementsMatch(t, tt.wantFailed, gotFailed)
		})
	}
}

// RunOnce は store が返したエラーだけを上位に伝播する (ticker 側で ERROR ログ化されるため)。
// publish 単独の失敗は RunOnce の戻り値に影響しない。
func TestPublisher_RunOnce_ClaimErrorSurfaces(t *testing.T) {
	store := &fakeOutboxStore{claimErr: errors.New("db down")}
	pub := &fakeRawPublisher{}
	s, err := New(store, pub, Config{
		BatchSize: 10, FailureThreshold: 5, VisibilityTimeout: 30 * time.Second,
	})
	require.NoError(t, err)

	err = s.RunOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
	assert.Empty(t, pub.calls)
	assert.Empty(t, store.markedPublished)
	assert.Empty(t, store.failures)
}

// Config の BatchSize / VisibilityTimeout は store.ClaimUnpublished にそのまま渡される
// (env 可変設定の到達検証)。
func TestPublisher_RunOnce_PassesConfigToStore(t *testing.T) {
	store := &fakeOutboxStore{}
	pub := &fakeRawPublisher{}
	s, err := New(store, pub, Config{
		BatchSize: 42, FailureThreshold: 5, VisibilityTimeout: 17 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, s.RunOnce(context.Background()))
	assert.Equal(t, 42, store.lastLimit)
	assert.Equal(t, 17*time.Second, store.lastVisibilityTO)
	assert.Equal(t, 5, store.lastFailureThreshold)
}
