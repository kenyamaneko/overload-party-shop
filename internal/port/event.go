package port

import "context"

// RawEventPublisher は outbox worker が eventType + payload で Pub/Sub に送出する
// 低レベルインターフェース。eventType (apishop.EventType*) を物理 topic 名に
// 解決するのは adapter (pubsub.Publisher) の責務で、worker は outbox 行から
// 取り出した eventType / payload をそのまま渡す。
type RawEventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}
