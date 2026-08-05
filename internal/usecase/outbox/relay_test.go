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
	claimed              []port.ClaimedOutboxEvent
	claimErr             error
	markedPublished      []uuid.UUID
	markErr              error
	failures             []failureCall
	failErr              error
	claimCalls           int
	lastVisibilityTO     time.Duration
	lastLimit            int
	lastFailureThreshold int
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

func TestNew(t *testing.T) {
	t.Run("生成時の設定検証", func(t *testing.T) {
		tests := []struct {
			name    string
			store   port.OutboxStore
			pub     port.RawEventPublisher
			cfg     Config
			wantSub string
		}{
			{
				name:    "保存先が未指定のとき、生成に失敗する",
				store:   nil,
				pub:     &fakeRawPublisher{},
				cfg:     Config{BatchSize: 1, FailureThreshold: 1, VisibilityTimeout: time.Second},
				wantSub: "store is nil",
			},
			{
				name:    "配信先が未指定のとき、生成に失敗する",
				store:   &fakeOutboxStore{},
				pub:     nil,
				cfg:     Config{BatchSize: 1, FailureThreshold: 1, VisibilityTimeout: time.Second},
				wantSub: "publisher is nil",
			},
			{
				name:    "バッチサイズが0のとき、生成に失敗する",
				store:   &fakeOutboxStore{},
				pub:     &fakeRawPublisher{},
				cfg:     Config{BatchSize: 0, FailureThreshold: 1, VisibilityTimeout: time.Second},
				wantSub: "BatchSize must be positive",
			},
			{
				name:    "失敗回数の上限が0のとき、生成に失敗する",
				store:   &fakeOutboxStore{},
				pub:     &fakeRawPublisher{},
				cfg:     Config{BatchSize: 1, FailureThreshold: 0, VisibilityTimeout: time.Second},
				wantSub: "FailureThreshold must be positive",
			},
			{
				name:    "可視性タイムアウトが0のとき、生成に失敗する",
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
	})
}

func TestRunOnce(t *testing.T) {
	t.Run("1回の配信実行", func(t *testing.T) {
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
				name:    "未配信イベントが無いとき、何も配信されない",
				claimed: nil,
			},
			{
				name: "全件の配信に成功すると、全件が配信済みになる",
				claimed: []port.ClaimedOutboxEvent{
					{EventID: okID, EventType: apishop.EventTypeCardPackPurchased, Payload: []byte(`{}`), FailureCount: 0},
				},
				wantMarked:       []uuid.UUID{okID},
				wantPublishCalls: 1,
			},
			{
				name: "配信に失敗すると、失敗が記録され配信済みにならない",
				claimed: []port.ClaimedOutboxEvent{
					{EventID: ngID, EventType: apishop.EventTypeCardPackPurchased, Payload: []byte(`{}`), FailureCount: 0},
				},
				publishErrs:      map[string]error{apishop.EventTypeCardPackPurchased: errors.New("pubsub down")},
				wantFailed:       []uuid.UUID{ngID},
				wantPublishCalls: 1,
			},
			{
				name: "成功と失敗が混在するとき、成功分だけ配信済みになり失敗分は失敗として記録される",
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

		t.Run("未配信イベントの取得に失敗すると、そのエラーが返り何も配信されない", func(t *testing.T) {
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
		})

		t.Run("設定したバッチサイズ・可視性タイムアウト・失敗回数の上限が取得時に使われる", func(t *testing.T) {
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
		})
	})
}
