package port

import "context"

// RawEventPublisher は outbox worker が topic + payload で Pub/Sub に送出する
// 低レベルインターフェース。イベント struct の構築は service/event の build 関数
// が担い、worker は構築済み payload を流すだけ。
type RawEventPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}
