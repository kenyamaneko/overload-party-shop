// Package pubsub は shop サービスの Pub/Sub publisher を提供する。
//
// shop は 2 種のサービス横断イベントを発行する:
//
//   - `faction-selected` — faction_set 購入 commit 時。account / card / gateway が
//     それぞれ subscribe して自スキーマを更新する。
//   - `premium-updated` — subscription 状態変更で is_premium が変わったとき。
//     account / gateway が subscribe する。
//
// Publisher は worker (outbox) から topic + payload 指定で呼ばれる低レベル送信層。
// ビジネス要件に応じたイベント struct の構築は EventBuilder が担う。
package pubsub

import (
	"context"
	"errors"
	"fmt"

	gpubsub "cloud.google.com/go/pubsub/v2"
)

// Publisher は shop が publish する全 topic をまとめる。起動時に一度だけ構築し、
// Close で in-flight メッセージを flush して gRPC client を解放する。
// port.RawEventPublisher を実装する。
type Publisher struct {
	client  *gpubsub.Client
	byTopic map[string]*gpubsub.Publisher
}

// New は指定 project + topic 名に紐づく Publisher を構築する。
// 両 topic は Terraform（modules/pubsub）で事前作成されている前提。
func New(ctx context.Context, projectID, factionSelectedTopic, premiumUpdatedTopic string) (*Publisher, error) {
	if projectID == "" {
		return nil, errors.New("pubsub: projectID is empty")
	}
	if factionSelectedTopic == "" || premiumUpdatedTopic == "" {
		return nil, errors.New("pubsub: both topic names are required")
	}
	client, err := gpubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new pubsub client: %w", err)
	}
	return &Publisher{
		client: client,
		byTopic: map[string]*gpubsub.Publisher{
			factionSelectedTopic: client.Publisher(factionSelectedTopic),
			premiumUpdatedTopic:  client.Publisher(premiumUpdatedTopic),
		},
	}, nil
}

// Close は in-flight メッセージを flush し Pub/Sub client を閉じる。
func (p *Publisher) Close() error {
	for _, pub := range p.byTopic {
		pub.Stop()
	}
	return p.client.Close()
}

// Publish は topic 名で指定された Pub/Sub topic に payload をそのまま送る。
// worker が outbox 行から topic + payload を読み出して呼ぶ想定。
// 未登録 topic はエラーを返す (outbox 行の topic 設定ミスを alert 経路に載せるため)。
func (p *Publisher) Publish(ctx context.Context, topic string, payload []byte) error {
	pub, ok := p.byTopic[topic]
	if !ok {
		return fmt.Errorf("pubsub: unknown topic %q", topic)
	}
	result := pub.Publish(ctx, &gpubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish to %s: %w", topic, err)
	}
	return nil
}
