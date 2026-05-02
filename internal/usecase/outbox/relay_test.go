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
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// fakeOutboxStore は port.OutboxStore の簡易モック。
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

// fakeRawPublisher は eventType 単位で publish 結果を制御できるモック。
type fakeRawPublisher struct {
	calls          []string // eventType:payload
	errByEventType map[string]error
}

func (p *fakeRawPublisher) Publish(_ context.Context, eventType string, payload []byte) error {
	p.calls = append(p.calls, eventType+":"+string(payload))
	if err, ok := p.errByEventType[eventType]; ok {
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

// RunOnce の claim → publish → mark/fail のフローを各ケースで固定する。
func TestRelay_RunOnce(t *testing.T) {
	okID := uuid.New()
	ngID := uuid.New()

	tests := []struct {
		name             string
		claimed          []port.ClaimedOutboxEvent
		publishErrs      map[string]error // eventType → error
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
				{EventID: okID, EventType: domain.EventTypeFactionPurchased, Payload: []byte(`{}`), FailureCount: 0},
			},
			wantMarked:       []uuid.UUID{okID},
			wantPublishCalls: 1,
		},
		{
			name: "publish 失敗で RecordFailure を呼び、MarkPublished は呼ばない",
			claimed: []port.ClaimedOutboxEvent{
				{EventID: ngID, EventType: domain.EventTypeFactionPurchased, Payload: []byte(`{}`), FailureCount: 0},
			},
			publishErrs:      map[string]error{domain.EventTypeFactionPurchased: errors.New("pubsub down")},
			wantFailed:       []uuid.UUID{ngID},
			wantPublishCalls: 1,
		},
		{
			name: "混在バッチ: 成功行と失敗行が独立に処理される",
			claimed: []port.ClaimedOutboxEvent{
				{EventID: okID, EventType: "ok-event-type", Payload: []byte(`{}`), FailureCount: 0},
				{EventID: ngID, EventType: "ng-event-type", Payload: []byte(`{}`), FailureCount: 2},
			},
			publishErrs:      map[string]error{"ng-event-type": errors.New("nope")},
			wantMarked:       []uuid.UUID{okID},
			wantFailed:       []uuid.UUID{ngID},
			wantPublishCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeOutboxStore{claimed: tt.claimed}
			pub := &fakeRawPublisher{errByEventType: tt.publishErrs}
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

// RunOnce は store エラーのみ上位に伝播する (publish 失敗は戻り値に影響しない)。
func TestRelay_RunOnce_ClaimErrorSurfaces(t *testing.T) {
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

// Config の BatchSize / VisibilityTimeout / FailureThreshold は store.ClaimUnpublished にそのまま渡される。
func TestRelay_RunOnce_PassesConfigToStore(t *testing.T) {
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
