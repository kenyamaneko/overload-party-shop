package presenter_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

func TestToCardPackPurchasedWire(t *testing.T) {
	t.Run("カードパック購入イベントの変換", func(t *testing.T) {
		t.Run("カードパック購入イベントが渡されたとき、event_typeと各フィールドが送信内容に反映される", func(t *testing.T) {
			ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			eventType, payload, err := presenter.ToCardPackPurchasedWire(domain.CardPackPurchasedEvent{
				EventID:    "evt-1",
				Timestamp:  ts,
				PlayerID:   "p1",
				CardPackID: "faction_set_tenki",
			})
			require.NoError(t, err)
			assert.Equal(t, apishop.EventTypeCardPackPurchased, eventType)

			var got apishop.CardPackPurchasedEvent
			require.NoError(t, json.Unmarshal(payload, &got))
			assert.Equal(t, apishop.EventTypeCardPackPurchased, got.EventType)
			assert.Equal(t, "evt-1", got.EventID)
			assert.Equal(t, ts, got.Timestamp)
			assert.Equal(t, "p1", got.PlayerID)
			assert.Equal(t, "faction_set_tenki", got.CardPackID)
		})
	})
}

func TestToFactionAcquiredWire(t *testing.T) {
	t.Run("faction獲得イベントの変換", func(t *testing.T) {
		t.Run("faction獲得イベントが渡されたとき、event_typeと各フィールドが送信内容に反映される", func(t *testing.T) {
			ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			eventType, payload, err := presenter.ToFactionAcquiredWire(domain.FactionAcquiredEvent{
				EventID:   "evt-2",
				Timestamp: ts,
				PlayerID:  "p1",
				Faction:   "Tenki",
			})
			require.NoError(t, err)
			assert.Equal(t, apishop.EventTypeFactionAcquired, eventType)

			var got apishop.FactionAcquiredEvent
			require.NoError(t, json.Unmarshal(payload, &got))
			assert.Equal(t, apishop.EventTypeFactionAcquired, got.EventType)
			assert.Equal(t, "evt-2", got.EventID)
			assert.Equal(t, ts, got.Timestamp)
			assert.Equal(t, "p1", got.PlayerID)
			assert.Equal(t, "Tenki", got.Faction)
		})
	})
}

func TestToPremiumUpdatedWire(t *testing.T) {
	t.Run("premium更新イベントの変換", func(t *testing.T) {
		t.Run("有効期限が設定されているとき、premium状態と有効期限が送信内容に反映される", func(t *testing.T) {
			ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			expiresAt := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
			eventType, payload, err := presenter.ToPremiumUpdatedWire(domain.PremiumUpdatedEvent{
				EventID:          "evt-2",
				Timestamp:        ts,
				PlayerID:         "p1",
				IsPremium:        true,
				PremiumExpiresAt: &expiresAt,
			})
			require.NoError(t, err)
			assert.Equal(t, apishop.EventTypePremiumUpdated, eventType)

			var got apishop.PremiumUpdatedEvent
			require.NoError(t, json.Unmarshal(payload, &got))
			assert.Equal(t, apishop.EventTypePremiumUpdated, got.EventType)
			assert.Equal(t, "evt-2", got.EventID)
			assert.Equal(t, ts, got.Timestamp)
			assert.Equal(t, "p1", got.PlayerID)
			assert.True(t, got.IsPremium)
			require.NotNil(t, got.PremiumExpiresAt)
			assert.Equal(t, expiresAt, *got.PremiumExpiresAt)
			assert.Equal(t, apishop.PremiumUpdatedSourceShop, got.Source)
		})

		t.Run("有効期限が未設定のとき、premium無効・有効期限なしとして反映される", func(t *testing.T) {
			ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			_, payload, err := presenter.ToPremiumUpdatedWire(domain.PremiumUpdatedEvent{
				EventID:   "evt-3",
				Timestamp: ts,
				PlayerID:  "p1",
				IsPremium: false,
			})
			require.NoError(t, err)

			var got apishop.PremiumUpdatedEvent
			require.NoError(t, json.Unmarshal(payload, &got))
			assert.False(t, got.IsPremium)
			assert.Nil(t, got.PremiumExpiresAt, "失効済みは expiresAt=nil で透過する")
			assert.Equal(t, apishop.PremiumUpdatedSourceShop, got.Source)
		})
	})
}
