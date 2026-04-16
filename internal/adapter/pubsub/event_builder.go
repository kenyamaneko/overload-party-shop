package pubsub

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	pubsubevents "github.com/kenyamaneko/overload-party-common/packages/pubsub-events"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// EventBuilder はドメインデータから outbox 行用の OutboxEvent を構築する。
// event struct のスキーマ (pubsubevents.*) を知っているのは pubsub adapter のみに
// 閉じ込め、postgres adapter は payload を不透明な []byte として扱う。
//
// topic 名は Publisher と共通の設定値から渡す (enqueue 時と publish 時で不一致が
// 起きないようにするため)。
type EventBuilder struct {
	factionSelectedTopic string
	premiumUpdatedTopic  string
}

// NewEventBuilder は各イベントの送信先 topic 名を持つ EventBuilder を構築する。
func NewEventBuilder(factionSelectedTopic, premiumUpdatedTopic string) (*EventBuilder, error) {
	if factionSelectedTopic == "" || premiumUpdatedTopic == "" {
		return nil, errors.New("pubsub: both topic names are required")
	}
	return &EventBuilder{
		factionSelectedTopic: factionSelectedTopic,
		premiumUpdatedTopic:  premiumUpdatedTopic,
	}, nil
}

// BuildFactionSelected は shop 購入起因の faction-selected イベントを構築する。
// Source は常に FactionSourceShopPurchase (scenario 起因の initial-selection は
// 別 publisher が発行する)。
func (b *EventBuilder) BuildFactionSelected(playerID, faction string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("pubsub: playerID is empty")
	}
	if faction == "" {
		return port.OutboxEvent{}, errors.New("pubsub: faction is empty")
	}
	eventID := uuid.New()
	ev := pubsubevents.FactionSelectedEvent{
		EventType: pubsubevents.EventTypeFactionSelected,
		EventID:   eventID.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
		Source:    pubsubevents.FactionSourceShopPurchase,
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal faction-selected: %w", err)
	}
	return port.OutboxEvent{
		EventID: eventID,
		Topic:   b.factionSelectedTopic,
		Payload: payload,
	}, nil
}

// BuildPremiumUpdated は subscription 状態変化起因の premium-updated イベントを構築する。
// expiresAt は任意 (is_premium=false のときは nil)。
func (b *EventBuilder) BuildPremiumUpdated(playerID string, isPremium bool, expiresAt *time.Time) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("pubsub: playerID is empty")
	}
	eventID := uuid.New()
	ev := pubsubevents.PremiumUpdatedEvent{
		EventType:        pubsubevents.EventTypePremiumUpdated,
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
		Source:           pubsubevents.PremiumUpdatedSourceShop,
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal premium-updated: %w", err)
	}
	return port.OutboxEvent{
		EventID: eventID,
		Topic:   b.premiumUpdatedTopic,
		Payload: payload,
	}, nil
}
