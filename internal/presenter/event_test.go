package presenter_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
)

func TestToCardPackPurchasedEvent(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := presenter.ToCardPackPurchasedEvent("evt-1", "p1", "faction_set_tenki", ts)

	assert.Equal(t, domain.EventTypeCardPackPurchased, got.EventType)
	assert.Equal(t, "evt-1", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.Equal(t, "faction_set_tenki", got.CardPackID)
}

func TestToFactionAcquiredEvent(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := presenter.ToFactionAcquiredEvent("evt-2", "p1", "Tenki", ts)

	assert.Equal(t, domain.EventTypeFactionAcquired, got.EventType)
	assert.Equal(t, "evt-2", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.Equal(t, "Tenki", got.Faction)
}

func TestToPremiumUpdatedEvent_Active(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	expiresAt := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	got := presenter.ToPremiumUpdatedEvent("evt-2", "p1", true, &expiresAt, ts)

	assert.Equal(t, domain.EventTypePremiumUpdated, got.EventType)
	assert.Equal(t, "evt-2", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.True(t, got.IsPremium)
	assert.Equal(t, &expiresAt, got.PremiumExpiresAt)
	assert.Equal(t, domain.PremiumUpdatedSourceShop, got.Source)
}

func TestToPremiumUpdatedEvent_NoExpiry(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := presenter.ToPremiumUpdatedEvent("evt-3", "p1", false, nil, ts)

	assert.False(t, got.IsPremium)
	assert.Nil(t, got.PremiumExpiresAt, "失効済みは expiresAt=nil で透過する")
	assert.Equal(t, domain.PremiumUpdatedSourceShop, got.Source)
}
