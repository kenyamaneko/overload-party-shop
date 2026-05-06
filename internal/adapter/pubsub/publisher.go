// Package pubsub は論理 eventType を物理 topic に解決して送出する Pub/Sub publisher。
package pubsub

import (
	"context"
	"errors"
	"fmt"

	gpubsub "cloud.google.com/go/pubsub/v2"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// Publisher は port.RawEventPublisher を実装する。
type Publisher struct {
	client      *gpubsub.Client
	byEventType map[string]*gpubsub.Publisher
}

// New は eventType → topic の mapping を構築する。topic 名は env から注入する。
func New(ctx context.Context, projectID, cardPackPurchasedTopic, factionAcquiredTopic, premiumUpdatedTopic string) (*Publisher, error) {
	if projectID == "" {
		return nil, errors.New("pubsub: projectID is empty")
	}
	if cardPackPurchasedTopic == "" || factionAcquiredTopic == "" || premiumUpdatedTopic == "" {
		return nil, errors.New("pubsub: all topic names are required")
	}
	client, err := gpubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new pubsub client: %w", err)
	}
	return &Publisher{
		client: client,
		byEventType: map[string]*gpubsub.Publisher{
			domain.EventTypeCardPackPurchased: client.Publisher(cardPackPurchasedTopic),
			domain.EventTypeFactionAcquired:   client.Publisher(factionAcquiredTopic),
			domain.EventTypePremiumUpdated:    client.Publisher(premiumUpdatedTopic),
		},
	}, nil
}

func (p *Publisher) Close() error {
	for _, pub := range p.byEventType {
		pub.Stop()
	}
	return p.client.Close()
}

// Publish は未登録 eventType をエラーで返す (Pub/Sub SDK に届く前に失敗させる)。
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
