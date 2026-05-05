package presenter

import (
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// ToFactionPurchasedEvent は与えられた eventID / playerID / faction / timestamp から
// wire の FactionPurchasedEvent を組み立てます。
// uuid 採番や時刻採取などの副作用は呼び出し側 (usecase) が担います。
func ToFactionPurchasedEvent(eventID, playerID, faction string, ts time.Time) domain.FactionPurchasedEvent {
	return domain.FactionPurchasedEvent{
		EventType: domain.EventTypeFactionPurchased,
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
