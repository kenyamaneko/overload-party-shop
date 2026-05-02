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
func TestFactionPurchased_ExpectPublishWaitRoundTrip(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)
	ctx := context.Background()

	exp := apishopfake.ExpectFactionPurchased(sub)

	require.NoError(t, apishopfake.PublishFactionPurchased(ctx, pub, apishop.FactionPurchasedEvent{
		PlayerID: "player-1",
		Faction:  "Tenki",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "player-1", got.PlayerID)
	assert.Equal(t, "Tenki", got.Faction)
}

// PublishFactionPurchased は EventType / EventID / Timestamp の省略時にデフォルトを補完する。
func TestFactionPurchased_PublishFillsDefaults(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)
	ctx := context.Background()

	before := time.Now().UTC()
	exp := apishopfake.ExpectFactionPurchased(sub)

	require.NoError(t, apishopfake.PublishFactionPurchased(ctx, pub, apishop.FactionPurchasedEvent{
		PlayerID: "p", Faction: "SHE",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, apishop.EventTypeFactionPurchased, got.EventType, "EventType は契約で固定")
	assert.NotEmpty(t, got.EventID, "EventID は未指定なら自動生成される")
	assert.False(t, got.Timestamp.Before(before), "Timestamp は未指定なら現在時刻以降")
}

// Expect より先に publish されたメッセージは Wait で拾えず timeout になる。
func TestFactionPurchased_WaitTimesOutWhenPublishedBeforeExpect(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)
	ctx := context.Background()

	require.NoError(t, apishopfake.PublishFactionPurchased(ctx, pub, apishop.FactionPurchasedEvent{
		PlayerID: "p", Faction: "Tenki",
	}))

	exp := apishopfake.ExpectFactionPurchased(sub)
	_, err := exp.Wait(50 * time.Millisecond)
	require.ErrorContains(t, err, "timeout")
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

	require.NoError(t, apishopfake.PublishFactionPurchased(ctx, pub, apishop.FactionPurchasedEvent{
		PlayerID: "p", Faction: "Sugar",
	}))
	require.NoError(t, apishopfake.PublishPremiumUpdated(ctx, pub, apishop.PremiumUpdatedEvent{
		PlayerID: "p", IsPremium: false,
	}))

	history := pub.Published()
	require.Len(t, history, 2)
	assert.Equal(t, apishop.TopicFactionPurchased, history[0].Topic)
	assert.Equal(t, apishop.TopicPremiumUpdated, history[1].Topic)
	assert.NotEmpty(t, history[0].Data)
	assert.NotEmpty(t, history[1].Data)
}
