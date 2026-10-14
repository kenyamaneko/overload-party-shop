package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/outbox"
)

var (
	errClaim   = errors.New("TST-claim-failed")
	errPublish = errors.New("TST-publish-failed")
	errRecord  = errors.New("TST-record-failed")
)

var relayConfig = outbox.Config{BatchSize: 10, FailureThreshold: 3, VisibilityTimeout: time.Nanosecond}

var (
	firstEvent = port.ClaimedOutboxEvent{
		EventID:   uuid.MustParse("00000000-0000-4000-8000-0000000000a1"),
		EventType: "TST-EVENT-TYPE-1",
		Payload:   []byte(`{"TST":"payload-1"}`),
	}
	secondEvent = port.ClaimedOutboxEvent{
		EventID:   uuid.MustParse("00000000-0000-4000-8000-0000000000a2"),
		EventType: "TST-EVENT-TYPE-2",
		Payload:   []byte(`{"TST":"payload-2"}`),
	}
)

type publishedEvent struct {
	EventType string
	Payload   string
}

type fakeStore struct {
	claimed             []port.ClaimedOutboxEvent
	claimErr            error
	markErrors          map[uuid.UUID]error
	recordFailureErrors map[uuid.UUID]error
}

func (s *fakeStore) ClaimUnpublished(_ context.Context, _ int, _ time.Duration, _ int) ([]port.ClaimedOutboxEvent, error) {
	return s.claimed, s.claimErr
}

func (s *fakeStore) MarkPublished(_ context.Context, eventID uuid.UUID) error {
	return s.markErrors[eventID]
}

func (s *fakeStore) RecordFailure(_ context.Context, eventID uuid.UUID, _ string) error {
	return s.recordFailureErrors[eventID]
}

type fakePublisher struct {
	errorsByEventType map[string]error
	published         []publishedEvent
}

func (p *fakePublisher) Publish(_ context.Context, eventType string, payload []byte) error {
	p.published = append(p.published, publishedEvent{EventType: eventType, Payload: string(payload)})
	return p.errorsByEventType[eventType]
}

func newRelay(t *testing.T, store port.OutboxStore, publisher port.RawEventPublisher, config outbox.Config) *outbox.Relay {
	t.Helper()
	relay, err := outbox.New(store, publisher, config)
	require.NoError(t, err)
	return relay
}

func buildPublishErrorsFor(eventType string) map[string]error {
	return map[string]error{eventType: errPublish}
}
