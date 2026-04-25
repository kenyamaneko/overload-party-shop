package port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// OutboxEvent は shop.outbox_events への 1 行分の書き込み表現。
//
// event_id は enqueue 時点で確定し、payload 内の eventId と一致する。再試行時も
// 同じ event_id / payload を worker が Pub/Sub に送出するため、subscriber 側は
// event_id または複合 PK を冪等性キーとして使える (at-least-once)。
//
// Payload は apishop スキーマの struct を JSON Marshal した生バイトで、
// service/event の build 関数が構築する。postgres adapter は payload の
// スキーマを知らず、単に bytes として書き込む。
//
// EventType は論理イベント種別 (apishop.EventType*)。物理 topic 名への解決は
// pubsub adapter が内部で行うため、service / outbox / worker は EventType しか
// 触らない (Pub/Sub 固有概念は adapter に閉じ込める)。
//
// ゼロ値 (EventType == "") は「イベントを書かない」を意味し、repo 層はこの値を
// 受け取ったとき outbox への INSERT をスキップする (解約遷移など publish
// しない状態遷移で使う)。
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
