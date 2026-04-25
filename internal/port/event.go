package port

import "context"

// RawEventPublisher は outbox worker が呼ぶ Pub/Sub 送出の低レベル interface。
// eventType (apishop.EventType*) → 物理 topic への解決は adapter 内部で行う。
type RawEventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}
