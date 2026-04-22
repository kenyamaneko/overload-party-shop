//go:build integration

package pubsub

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub/pubsubtest"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

var sharedEmulator *pubsubtest.Emulator

// TestMain は package 内の全 integration test で共有する emulator を起動する。
// container 起動コストは高いので per-test ではなく package scope で償却する。
// test 毎の分離は topic / subscription の UUID suffix で担保する。
func TestMain(m *testing.M) {
	ctx := context.Background()
	em, err := pubsubtest.StartEmulator(ctx, "shop-test")
	if err != nil {
		log.Fatalf("start pubsub emulator: %v", err)
	}
	sharedEmulator = em

	code := m.Run()

	if cerr := em.Close(ctx); cerr != nil {
		log.Printf("close emulator: %v", cerr)
	}
	os.Exit(code)
}

// Publisher + EventBuilder を emulator に向けて構築するヘルパ。両 topic を事前作成。
// Outbox 導入後、Publisher は topic + payload を取る低レベル送信層、EventBuilder は
// payload 構築を担う。Integration test は両者を直結して end-to-end の shape を検証する。
func setupPublisher(t *testing.T) (*Publisher, *EventBuilder, string, string) {
	t.Helper()
	factionTopic := sharedEmulator.CreateTopic(t, apishop.TopicFactionPurchased)
	premiumTopic := sharedEmulator.CreateTopic(t, apishop.TopicPremiumUpdated)

	ctx := context.Background()
	pub, err := New(ctx, sharedEmulator.ProjectID(), factionTopic, premiumTopic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	builder, err := NewEventBuilder(factionTopic, premiumTopic)
	require.NoError(t, err)

	return pub, builder, factionTopic, premiumTopic
}

// EventBuilder が構築する faction-purchased payload が Publisher で送信できる
// shape であることを固定する (outbox 行を worker が送出する経路の近似)。
func TestIntegration_PublishFactionPurchased(t *testing.T) {
	pub, builder, factionTopic, _ := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, factionTopic)

	ctx := context.Background()
	ev, err := builder.BuildFactionPurchased("player-123", "Tenki")
	require.NoError(t, err)
	require.NoError(t, pub.Publish(ctx, ev.Topic, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded apishop.FactionPurchasedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, apishop.EventTypeFactionPurchased, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload の eventId は outbox 行の PK と一致する")
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	assert.Equal(t, "player-123", decoded.PlayerID)
	assert.Equal(t, "Tenki", decoded.Faction)
}

// premium 付与 (expires_at あり) の送信 shape を固定。
func TestIntegration_PublishPremiumUpdated_Granted(t *testing.T) {
	pub, builder, _, premiumTopic := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, premiumTopic)

	ctx := context.Background()
	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC()
	ev, err := builder.BuildPremiumUpdated("player-premium", true, &expiresAt)
	require.NoError(t, err)
	require.NoError(t, pub.Publish(ctx, ev.Topic, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded apishop.PremiumUpdatedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, apishop.EventTypePremiumUpdated, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID)
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	assert.Equal(t, "player-premium", decoded.PlayerID)
	assert.True(t, decoded.IsPremium)
	assert.Equal(t, apishop.PremiumUpdatedSourceShop, decoded.Source)
	require.NotNil(t, decoded.PremiumExpiresAt)
	assert.WithinDuration(t, expiresAt, *decoded.PremiumExpiresAt, time.Second)
}

// premium 解除 (expires_at=nil) の送信 shape を固定。
func TestIntegration_PublishPremiumUpdated_Revoked(t *testing.T) {
	pub, builder, _, premiumTopic := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, premiumTopic)

	ctx := context.Background()
	ev, err := builder.BuildPremiumUpdated("player-not-premium", false, nil)
	require.NoError(t, err)
	require.NoError(t, pub.Publish(ctx, ev.Topic, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded apishop.PremiumUpdatedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, apishop.EventTypePremiumUpdated, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID)
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	assert.Equal(t, "player-not-premium", decoded.PlayerID)
	assert.False(t, decoded.IsPremium)
	assert.Equal(t, apishop.PremiumUpdatedSourceShop, decoded.Source)
	assert.Nil(t, decoded.PremiumExpiresAt)
}

// negative path: publish が呼ばれなければ subscriber は timeout する。
func TestIntegration_NoPublish_SubscriberTimesOut(t *testing.T) {
	_, _, factionTopic, _ := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, factionTopic)

	_, err := sub.WaitForMessage(context.Background(), 500*time.Millisecond)
	assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
}
