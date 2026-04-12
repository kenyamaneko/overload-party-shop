// Package pubsub は shop サービスの Pub/Sub publisher を提供する。
//
// shop は 2 種のサービス横断イベントを発行する:
//
//   - `faction-selected` — faction_set 購入 commit 時。account / card / gateway が
//     それぞれ subscribe して自スキーマを更新する。
//   - `premium-updated` — subscription 状態変更で is_premium が変わったとき。
//     account / gateway が subscribe する。
//
// publish は REST handler から見て同期的（PublishResult.Get で Pub/Sub ack を待つ）。
// 失敗時は webhook リトライ（Apple/Google RTDN）がリトライ予算を管理する。
package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	gpubsub "cloud.google.com/go/pubsub"
	"github.com/google/uuid"

	pubsubevents "github.com/kenyamaneko/overload-party-common/packages/pubsub-events"
)

// Publisher は shop が publish する全 topic をまとめる。起動時に一度だけ構築し、
// Close で in-flight メッセージを flush して gRPC client を解放する。
// port.FactionEventPublisher と port.PremiumEventPublisher の両方を実装する。
type Publisher struct {
	client               *gpubsub.Client
	factionSelectedTopic *gpubsub.Topic
	premiumUpdatedTopic  *gpubsub.Topic
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
	fs := client.Topic(factionSelectedTopic)
	if ok, err := fs.Exists(ctx); err != nil || !ok {
		_ = client.Close()
		if err != nil {
			return nil, fmt.Errorf("pubsub: check topic %q: %w", factionSelectedTopic, err)
		}
		return nil, fmt.Errorf("pubsub: topic %q does not exist", factionSelectedTopic)
	}
	pu := client.Topic(premiumUpdatedTopic)
	if ok, err := pu.Exists(ctx); err != nil || !ok {
		_ = client.Close()
		if err != nil {
			return nil, fmt.Errorf("pubsub: check topic %q: %w", premiumUpdatedTopic, err)
		}
		return nil, fmt.Errorf("pubsub: topic %q does not exist", premiumUpdatedTopic)
	}
	return &Publisher{
		client:               client,
		factionSelectedTopic: fs,
		premiumUpdatedTopic:  pu,
	}, nil
}

// Close は in-flight メッセージを flush し Pub/Sub client を閉じる。
func (p *Publisher) Close() error {
	p.factionSelectedTopic.Stop()
	p.premiumUpdatedTopic.Stop()
	return p.client.Close()
}

// PublishFactionSelected は shop 購入による faction-selected イベントを発行する。
// Source は常に FactionSourceShopPurchase。scenario は独自の publisher で
// initial-selection イベントを発行する。
func (p *Publisher) PublishFactionSelected(ctx context.Context, playerID, faction string) error {
	if playerID == "" {
		return errors.New("pubsub: playerID is empty")
	}
	if faction == "" {
		return errors.New("pubsub: faction is empty")
	}
	ev := pubsubevents.FactionSelectedEvent{
		EventType: pubsubevents.EventTypeFactionSelected,
		EventID:   uuid.NewString(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
		Source:    pubsubevents.FactionSourceShopPurchase,
	}
	return publishJSON(ctx, p.factionSelectedTopic, ev)
}

// PublishPremiumUpdated は subscription 状態変更で is_premium / premium_expires_at
// が変わったときに premium-updated イベントを発行する。
// expiresAt は任意。is_premium が false になる場合は nil を渡す。
func (p *Publisher) PublishPremiumUpdated(ctx context.Context, playerID string, isPremium bool, expiresAt *time.Time) error {
	if playerID == "" {
		return errors.New("pubsub: playerID is empty")
	}
	ev := pubsubevents.PremiumUpdatedEvent{
		EventType:        pubsubevents.EventTypePremiumUpdated,
		EventID:          uuid.NewString(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
		Source:           pubsubevents.PremiumUpdatedSourceShop,
	}
	return publishJSON(ctx, p.premiumUpdatedTopic, ev)
}

func publishJSON(ctx context.Context, topic *gpubsub.Topic, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("pubsub: marshal %T: %w", event, err)
	}
	result := topic.Publish(ctx, &gpubsub.Message{Data: data})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish to %s: %w", topic.ID(), err)
	}
	return nil
}
