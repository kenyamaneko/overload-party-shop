package presenter

import (
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// ToCardPackPurchasedEvent は与えられた eventID / playerID / cardPackID / timestamp から
// wire の CardPackPurchasedEvent を組み立てます。
// uuid 採番や時刻採取などの副作用は呼び出し側 (usecase) が担います。
func ToCardPackPurchasedEvent(eventID, playerID, cardPackID string, ts time.Time) domain.CardPackPurchasedEvent {
	return domain.CardPackPurchasedEvent{
		EventType:  domain.EventTypeCardPackPurchased,
		EventID:    eventID,
		Timestamp:  ts,
		PlayerID:   playerID,
		CardPackID: cardPackID,
	}
}

// ToFactionAcquiredEvent は与えられた eventID / playerID / faction / timestamp から
// wire の FactionAcquiredEvent を組み立てます。
// uuid 採番や時刻採取などの副作用は呼び出し側 (usecase) が担います。
func ToFactionAcquiredEvent(eventID, playerID, faction string, ts time.Time) domain.FactionAcquiredEvent {
	return domain.FactionAcquiredEvent{
		EventType: domain.EventTypeFactionAcquired,
		EventID:   eventID,
		Timestamp: ts,
		PlayerID:  playerID,
		Faction:   faction,
	}
}

// ToPremiumUpdatedEvent は wire の PremiumUpdatedEvent を組み立てます。
// uuid 採番や時刻採取などの副作用は呼び出し側 (usecase) が担います。
func ToPremiumUpdatedEvent(eventID, playerID string, isPremium bool, expiresAt *time.Time, ts time.Time) domain.PremiumUpdatedEvent {
	return domain.PremiumUpdatedEvent{
		EventType:        domain.EventTypePremiumUpdated,
		EventID:          eventID,
		Timestamp:        ts,
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
		Source:           domain.PremiumUpdatedSourceShop,
	}
}
