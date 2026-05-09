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

// Expect → Publish → Wait の round-trip で typed publish/受信が繋がることを固定する。
func TestFactionAcquired_ExpectPublishWaitRoundTrip(t *testing.T) {
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
}

// PublishFactionAcquired は EventType / EventID / Timestamp の省略時にデフォルトを補完する。
func TestFactionAcquired_PublishFillsDefaults(t *testing.T) {
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
}

// Expect より先に publish されたメッセージは Wait で拾えず timeout になる。
func TestFactionAcquired_WaitTimesOutWhenPublishedBeforeExpect(t *testing.T) {
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
}

// CardPackPurchased 側の round-trip + defaults を固定する。
func TestCardPackPurchased_ExpectPublishWaitRoundTrip(t *testing.T) {
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
}

// PremiumUpdated 側の round-trip + defaults + Source 補完を固定する。
func TestPremiumUpdated_ExpectPublishWaitRoundTrip(t *testing.T) {
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
}

// Publisher.Published() は typed helper 経由の publish でも記録に残る。
func TestTyped_PublishedRecordsTopicAndPayload(t *testing.T) {
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
}
