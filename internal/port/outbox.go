package port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// OutboxEvent は shop.outbox_events への 1 行分の書き込み表現。
//
// EventID は payload 内 eventId と一致し、再試行でも変えない。subscriber は
// この値を冪等性キーとして使える (at-least-once)。
//
// EventType は論理イベント種別 (apishop.EventType*)。物理 topic への解決は
// pubsub adapter 内部で行う。
type OutboxEvent struct {
	EventID   uuid.UUID
	EventType string
	Payload   []byte
}

// ClaimedOutboxEvent は OutboxStore.ClaimUnpublished が返す 1 行分の情報。
// failure_count は閾値超過の alert 判定に使う。
type ClaimedOutboxEvent struct {
	EventID      uuid.UUID
	EventType    string
	Payload      []byte
	FailureCount int
}

// OutboxStore は outbox 行の消費側 (claim + mark/fail) を service 層から抽象化する。
// 書き込み側 (enqueue) は aggregate repo が担うため、この interface では扱わない。
//
// ClaimUnpublished は visibility timeout パターンで二重配信を避ける
// (claim 時に last_attempted_at を更新し、以降 visibilityTimeout の間は他 worker の
// claim から除外される)。
type OutboxStore interface {
	ClaimUnpublished(ctx context.Context, limit int, visibilityTimeout time.Duration, failureThreshold int) ([]ClaimedOutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
	RecordFailure(ctx context.Context, eventID uuid.UUID, errMsg string) error
}
