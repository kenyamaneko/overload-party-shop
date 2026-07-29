package apishopfake_test

import (
	"context"
	"testing"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactionAcquired(t *testing.T) {
	t.Run("FactionAcquired typed helper", func(t *testing.T) {
		t.Run("Expect → Publish → Wait すると、typed publish と typed 受信が一致する", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)
			ctx := context.Background()

			exp := apishopfake.ExpectFactionAcquired(sub)

			require.NoError(t, apishopfake.PublishFactionAcquired(ctx, pub, apishop.FactionAcquiredEvent{
				PlayerID: "player-1",
				Faction:  "Tenki",
			}))

			got, err := exp.Wait(time.Second)
			require.NoError(t, err)
			assert.Equal(t, "player-1", got.PlayerID)
			assert.Equal(t, "Tenki", got.Faction)
		})

		t.Run("EventType / EventID / Timestamp を指定しないとき、補完される", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)
			ctx := context.Background()

			before := time.Now().UTC()
			exp := apishopfake.ExpectFactionAcquired(sub)

			require.NoError(t, apishopfake.PublishFactionAcquired(ctx, pub, apishop.FactionAcquiredEvent{
				PlayerID: "p", Faction: "SHE",
			}))

			got, err := exp.Wait(time.Second)
			require.NoError(t, err)
			assert.Equal(t, apishop.EventTypeFactionAcquired, got.EventType, "EventType は契約で固定")
			assert.NotEmpty(t, got.EventID, "EventID は未指定なら自動生成される")
			assert.False(t, got.Timestamp.Before(before), "Timestamp は未指定なら現在時刻以降")
		})

		t.Run("Expect より先に Publish したとき、Wait が timeout する", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)
			ctx := context.Background()

			require.NoError(t, apishopfake.PublishFactionAcquired(ctx, pub, apishop.FactionAcquiredEvent{
				PlayerID: "p", Faction: "Tenki",
			}))

			exp := apishopfake.ExpectFactionAcquired(sub)
			_, err := exp.Wait(50 * time.Millisecond)
			require.ErrorContains(t, err, "timeout")
		})

		t.Run("イベントID と発行日時を指定して publish すると、指定値のまま受信される", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)
			ctx := context.Background()

			fixedTimestamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			exp := apishopfake.ExpectFactionAcquired(sub)

			require.NoError(t, apishopfake.PublishFactionAcquired(ctx, pub, apishop.FactionAcquiredEvent{
				PlayerID:  "p",
				Faction:   "SHE",
				EventID:   "TST-0001",
				Timestamp: fixedTimestamp,
			}))

			got, err := exp.Wait(time.Second)
			require.NoError(t, err)
			assert.Equal(t, "TST-0001", got.EventID)
			assert.True(t, got.Timestamp.Equal(fixedTimestamp))
		})
	})
}

func TestCardPackPurchased(t *testing.T) {
	t.Run("CardPackPurchased typed helper", func(t *testing.T) {
		t.Run("Expect → Publish → Wait すると、typed publish と typed 受信が一致し EventType/EventID が補完される", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)
			ctx := context.Background()

			exp := apishopfake.ExpectCardPackPurchased(sub)

			require.NoError(t, apishopfake.PublishCardPackPurchased(ctx, pub, apishop.CardPackPurchasedEvent{
				PlayerID:   "player-2",
				CardPackID: "faction_set_tenki",
			}))

			got, err := exp.Wait(time.Second)
			require.NoError(t, err)
			assert.Equal(t, "player-2", got.PlayerID)
			assert.Equal(t, "faction_set_tenki", got.CardPackID)
			assert.Equal(t, apishop.EventTypeCardPackPurchased, got.EventType, "EventType は契約で固定")
			assert.NotEmpty(t, got.EventID)
		})

		t.Run("イベントID と発行日時を指定して publish すると、指定値のまま受信される", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)
			ctx := context.Background()

			fixedTimestamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			exp := apishopfake.ExpectCardPackPurchased(sub)

			require.NoError(t, apishopfake.PublishCardPackPurchased(ctx, pub, apishop.CardPackPurchasedEvent{
				PlayerID:   "player-2",
				CardPackID: "faction_set_tenki",
				EventID:    "TST-0002",
				Timestamp:  fixedTimestamp,
			}))

			got, err := exp.Wait(time.Second)
			require.NoError(t, err)
			assert.Equal(t, "TST-0002", got.EventID)
			assert.True(t, got.Timestamp.Equal(fixedTimestamp))
		})
	})
}

func TestPremiumUpdated(t *testing.T) {
	t.Run("PremiumUpdated typed helper", func(t *testing.T) {
		t.Run("Expect → Publish → Wait すると、typed publish と typed 受信が一致し EventType/EventID/Source が補完される", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)
			ctx := context.Background()

			exp := apishopfake.ExpectPremiumUpdated(sub)

			expiry := time.Now().Add(30 * 24 * time.Hour).UTC()
			require.NoError(t, apishopfake.PublishPremiumUpdated(ctx, pub, apishop.PremiumUpdatedEvent{
				PlayerID:         "player-2",
				IsPremium:        true,
				PremiumExpiresAt: &expiry,
			}))

			got, err := exp.Wait(time.Second)
			require.NoError(t, err)
			assert.Equal(t, "player-2", got.PlayerID)
			assert.True(t, got.IsPremium)
			require.NotNil(t, got.PremiumExpiresAt)
			assert.True(t, got.PremiumExpiresAt.Equal(expiry))
			assert.Equal(t, apishop.EventTypePremiumUpdated, got.EventType)
			assert.Equal(t, apishop.PremiumUpdatedSourceShop, got.Source, "Source は未指定なら shop 固定")
			assert.NotEmpty(t, got.EventID)
		})

		t.Run("イベントID と発行日時を指定して publish すると、指定値のまま受信される", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)
			ctx := context.Background()

			fixedTimestamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			exp := apishopfake.ExpectPremiumUpdated(sub)

			require.NoError(t, apishopfake.PublishPremiumUpdated(ctx, pub, apishop.PremiumUpdatedEvent{
				PlayerID:  "player-2",
				IsPremium: true,
				EventID:   "TST-0003",
				Timestamp: fixedTimestamp,
			}))

			got, err := exp.Wait(time.Second)
			require.NoError(t, err)
			assert.Equal(t, "TST-0003", got.EventID)
			assert.True(t, got.Timestamp.Equal(fixedTimestamp))
		})
	})
}

func TestTyped_PublishedRecordsTopicAndPayload(t *testing.T) {
	t.Run("typed helper 経由の publish の記録", func(t *testing.T) {
		t.Run("2 種の typed helper で publish すると、Published() に topic 順で記録される", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			ctx := context.Background()

			require.NoError(t, apishopfake.PublishFactionAcquired(ctx, pub, apishop.FactionAcquiredEvent{
				PlayerID: "p", Faction: "Sugar",
			}))
			require.NoError(t, apishopfake.PublishPremiumUpdated(ctx, pub, apishop.PremiumUpdatedEvent{
				PlayerID: "p", IsPremium: false,
			}))

			history := pub.Published()
			require.Len(t, history, 2)
			assert.Equal(t, "faction-acquired", history[0].Topic)
			assert.Equal(t, "premium-updated", history[1].Topic)
			assert.NotEmpty(t, history[0].Data)
			assert.NotEmpty(t, history[1].Data)
		})
	})
}
