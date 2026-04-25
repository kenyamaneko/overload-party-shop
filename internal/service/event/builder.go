// Package event はビジネスイベントを OutboxEvent (outbox 行 1 件分) に
// シリアライズする pure 関数を提供する。
//
// 役割は postgres adapter での「pgxpool に対する SQL 構築」と対称で、
// 「pubsub publisher に対する payload 構築」をサービス層のロジックとして
// 表現する。adapter ではなく service 層に置くのは、apishop の event スキーマ
// (= サービス間契約) を知っているのは service 層であるべきだから。
package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// BuildFactionPurchased は shop 購入起因の faction-purchased イベントを
// outbox 行用の OutboxEvent に組み立てる。常に「購入による追加所有」を意味し、
// onboarding 由来の初期 faction 付与は対象外。
func BuildFactionPurchased(topic, playerID, faction string) (port.OutboxEvent, error) {
	if topic == "" {
		return port.OutboxEvent{}, errors.New("event: topic is empty")
	}
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("event: playerID is empty")
	}
	if faction == "" {
		return port.OutboxEvent{}, errors.New("event: faction is empty")
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
		Topic:   topic,
		Payload: payload,
	}, nil
}

// BuildPremiumUpdated は subscription 状態変化起因の premium-updated イベントを
// 構築する。expiresAt は任意 (is_premium=false のときは nil)。
func BuildPremiumUpdated(topic, playerID string, isPremium bool, expiresAt *time.Time) (port.OutboxEvent, error) {
	if topic == "" {
		return port.OutboxEvent{}, errors.New("event: topic is empty")
	}
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("event: playerID is empty")
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
		Topic:   topic,
		Payload: payload,
	}, nil
}
