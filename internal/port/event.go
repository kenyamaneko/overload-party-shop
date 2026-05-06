package port

import "context"

// RawEventPublisher は eventType を物理 topic に解決して送出する。
type RawEventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}
