package port

import (
	"time"

	"github.com/google/uuid"
)

// OutboxEvent は shop.outbox_events への 1 行分の書き込み表現。
//
// event_id は enqueue 時点で確定し、payload 内の eventId と一致する。再試行時も
// 同じ event_id / payload を worker が Pub/Sub に送出するため、subscriber 側は
// event_id または複合 PK を冪等性キーとして使える (at-least-once)。
//
// Payload は pubsubevents スキーマの struct を JSON Marshal した生バイトで、
// adapter/pubsub/event_builder が構築する。postgres adapter は payload の
// スキーマを知らず、単に bytes として書き込む。
//
// ゼロ値 (Topic == "") は「イベントを書かない」を意味し、repo 層はこの値を
// 受け取ったとき outbox への INSERT をスキップする (解約遷移など publish
// しない状態遷移で使う)。
type OutboxEvent struct {
	EventID uuid.UUID
	Topic   string
	Payload []byte
}

// OutboxEventBuilder は service 層が発行したいビジネスイベントを OutboxEvent に
// シリアライズする。pubsubevents スキーマの詳細は adapter/pubsub 側に閉じ込め、
// service 層は「何を発行したいか」だけを述語で伝える。
type OutboxEventBuilder interface {
	BuildFactionSelected(playerID, faction string) (OutboxEvent, error)
	BuildPremiumUpdated(playerID string, isPremium bool, expiresAt *time.Time) (OutboxEvent, error)
}
