package port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// OutboxEvent は shop.outbox_events への 1 行分の書き込み表現。
type OutboxEvent struct {
	EventID   uuid.UUID
	EventType string
	Payload   []byte
}

// ClaimedOutboxEvent は OutboxStore.ClaimUnpublished が返す 1 行分の情報。
type ClaimedOutboxEvent struct {
	EventID      uuid.UUID
	EventType    string
	Payload      []byte
	FailureCount int
}

// OutboxStore は outbox 行の消費側 (claim + mark/fail) を抽象化する。
type OutboxStore interface {
	ClaimUnpublished(ctx context.Context, limit int, visibilityTimeout time.Duration, failureThreshold int) ([]ClaimedOutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
	RecordFailure(ctx context.Context, eventID uuid.UUID, errMsg string) error
}
