// Package pubsub は shop の Pub/Sub publisher。worker (outbox) から呼ばれる
// 低レベル送信層で、論理 eventType を物理 topic に解決して送出する。
package pubsub

import (
	"context"
	"errors"
	"fmt"

	gpubsub "cloud.google.com/go/pubsub/v2"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// Publisher は port.RawEventPublisher を実装する。
type Publisher struct {
	client      *gpubsub.Client
	byEventType map[string]*gpubsub.Publisher
}

// New は物理 topic 名から eventType→topic mapping を構築する。topic 名は
// configmap / env で外から差し替えできるよう構築時に受け取る。両 topic は
// Terraform (modules/pubsub) で事前作成されている前提。
func New(ctx context.Context, projectID, factionPurchasedTopic, premiumUpdatedTopic string) (*Publisher, error) {
	if projectID == "" {
		return nil, errors.New("pubsub: projectID is empty")
	}
	if factionPurchasedTopic == "" || premiumUpdatedTopic == "" {
		return nil, errors.New("pubsub: both topic names are required")
	}
	client, err := gpubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new pubsub client: %w", err)
	}
	return &Publisher{
		client: client,
		byEventType: map[string]*gpubsub.Publisher{
			apishop.EventTypeFactionPurchased: client.Publisher(factionPurchasedTopic),
			apishop.EventTypePremiumUpdated:   client.Publisher(premiumUpdatedTopic),
		},
	}, nil
}

func (p *Publisher) Close() error {
	for _, pub := range p.byEventType {
		pub.Stop()
	}
	return p.client.Close()
}

// Publish は未登録 eventType をエラーで返し、outbox 行の設定ミスを alert
// 経路に載せる (Pub/Sub SDK に届く前に失敗させる)。
func (p *Publisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	pub, ok := p.byEventType[eventType]
	if !ok {
		return fmt.Errorf("pubsub: unknown event type %q", eventType)
	}
	result := pub.Publish(ctx, &gpubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish event_type=%s: %w", eventType, err)
	}
	return nil
}
