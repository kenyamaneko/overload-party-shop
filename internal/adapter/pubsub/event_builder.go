package pubsub

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// EventBuilder はドメインデータから outbox 行用の OutboxEvent を構築する。
// event struct のスキーマ (apishop.*) を知っているのは pubsub adapter のみに
// 閉じ込め、postgres adapter は payload を不透明な []byte として扱う。
//
// topic 名は Publisher と共通の設定値から渡す (enqueue 時と publish 時で不一致が
// 起きないようにするため)。
type EventBuilder struct {
	factionPurchasedTopic string
	premiumUpdatedTopic   string
}

// NewEventBuilder は各イベントの送信先 topic 名を持つ EventBuilder を構築する。
func NewEventBuilder(factionPurchasedTopic, premiumUpdatedTopic string) (*EventBuilder, error) {
	if factionPurchasedTopic == "" || premiumUpdatedTopic == "" {
		return nil, errors.New("pubsub: both topic names are required")
	}
	return &EventBuilder{
		factionPurchasedTopic: factionPurchasedTopic,
		premiumUpdatedTopic:   premiumUpdatedTopic,
	}, nil
}

// BuildFactionPurchased は shop 購入起因の faction-purchased イベントを構築する。
// 常に「購入による追加所有」を意味し、onboarding 由来の初期 faction 付与は対象外。
func (b *EventBuilder) BuildFactionPurchased(playerID, faction string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("pubsub: playerID is empty")
	}
	if faction == "" {
		return port.OutboxEvent{}, errors.New("pubsub: faction is empty")
	}
	eventID := uuid.New()
	ev := apishop.FactionPurchasedEvent{
		EventType: apishop.EventTypeFactionPurchased,
		EventID:   eventID.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal faction-purchased: %w", err)
	}
	return port.OutboxEvent{
		EventID: eventID,
		Topic:   b.factionPurchasedTopic,
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
	ev := apishop.PremiumUpdatedEvent{
		EventType:        apishop.EventTypePremiumUpdated,
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
		Source:           apishop.PremiumUpdatedSourceShop,
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
