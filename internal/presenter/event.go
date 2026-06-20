package presenter

import (
	"encoding/json"
	"fmt"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// ToCardPackPurchasedWire は domain event を wire payload に詰め替えて event_type と marshal 済み bytes を返す。
// usecase は戻り値の eventType と payload をそのまま port.OutboxEvent に格納できる (apishop の import 不要)。
func ToCardPackPurchasedWire(event domain.CardPackPurchasedEvent) (eventType string, payload []byte, err error) {
	wire := apishop.CardPackPurchasedEvent{
		EventType:  apishop.EventTypeCardPackPurchased,
		EventID:    event.EventID,
		Timestamp:  event.Timestamp,
		PlayerID:   event.PlayerID,
		CardPackID: event.CardPackID,
	}
	payload, err = json.Marshal(wire)
	if err != nil {
		return "", nil, fmt.Errorf("marshal CardPackPurchasedEvent: %w", err)
	}
	return apishop.EventTypeCardPackPurchased, payload, nil
}

// ToFactionAcquiredWire は ToCardPackPurchasedWire の FactionAcquiredEvent 版。
func ToFactionAcquiredWire(event domain.FactionAcquiredEvent) (eventType string, payload []byte, err error) {
	wire := apishop.FactionAcquiredEvent{
		EventType: apishop.EventTypeFactionAcquired,
		EventID:   event.EventID,
		Timestamp: event.Timestamp,
		PlayerID:  event.PlayerID,
		Faction:   event.Faction,
	}
	payload, err = json.Marshal(wire)
	if err != nil {
		return "", nil, fmt.Errorf("marshal FactionAcquiredEvent: %w", err)
	}
	return apishop.EventTypeFactionAcquired, payload, nil
}

// ToPremiumUpdatedWire は ToCardPackPurchasedWire の PremiumUpdatedEvent 版。Source は shop 固定。
func ToPremiumUpdatedWire(event domain.PremiumUpdatedEvent) (eventType string, payload []byte, err error) {
	wire := apishop.PremiumUpdatedEvent{
		EventType:        apishop.EventTypePremiumUpdated,
		EventID:          event.EventID,
		Timestamp:        event.Timestamp,
		PlayerID:         event.PlayerID,
		IsPremium:        event.IsPremium,
		PremiumExpiresAt: event.PremiumExpiresAt,
		Source:           apishop.PremiumUpdatedSourceShop,
	}
	payload, err = json.Marshal(wire)
	if err != nil {
		return "", nil, fmt.Errorf("marshal PremiumUpdatedEvent: %w", err)
	}
	return apishop.EventTypePremiumUpdated, payload, nil
}
