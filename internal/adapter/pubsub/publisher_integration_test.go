//go:build integration

package pubsub

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub/pubsubtest"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
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

// publisherTopics は setupPublisher が作成して返す全 topic 名のセット。
type publisherTopics struct {
	cardPackPurchased string
	factionAcquired   string
	premiumUpdated    string
}

// Publisher を emulator に向けて構築するヘルパ。全 topic を事前作成。
func setupPublisher(t *testing.T) (*Publisher, publisherTopics) {
	t.Helper()
	topics := publisherTopics{
		cardPackPurchased: sharedEmulator.CreateTopic(t, domain.TopicCardPackPurchased),
		factionAcquired:   sharedEmulator.CreateTopic(t, domain.TopicFactionAcquired),
		premiumUpdated:    sharedEmulator.CreateTopic(t, domain.TopicPremiumUpdated),
	}

	ctx := context.Background()
	pub, err := New(ctx, sharedEmulator.ProjectID(), topics.cardPackPurchased, topics.factionAcquired, topics.premiumUpdated)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	return pub, topics
}

func buildCardPackPurchasedOutbox(t *testing.T, playerID, cardPackID string) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(domain.CardPackPurchasedEvent{
		EventType:  domain.EventTypeCardPackPurchased,
		EventID:    id.String(),
		Timestamp:  time.Now().UTC(),
		PlayerID:   playerID,
		CardPackID: cardPackID,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: domain.EventTypeCardPackPurchased,
		Payload:   payload,
	}
}

func buildFactionAcquiredOutbox(t *testing.T, playerID, faction string) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(domain.FactionAcquiredEvent{
		EventType: domain.EventTypeFactionAcquired,
		EventID:   id.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: domain.EventTypeFactionAcquired,
		Payload:   payload,
	}
}

func buildPremiumUpdatedOutbox(t *testing.T, playerID string, isPremium bool, expiresAt *time.Time) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(domain.PremiumUpdatedEvent{
		EventType:        domain.EventTypePremiumUpdated,
		EventID:          id.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
		Source:           domain.PremiumUpdatedSourceShop,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: domain.EventTypePremiumUpdated,
		Payload:   payload,
	}
}

// card-pack-purchased payload が Publisher で送信できる shape であることを固定する
// (outbox 行を worker が送出する経路の近似)。
func TestIntegration_PublishCardPackPurchased(t *testing.T) {
	pub, topics := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topics.cardPackPurchased)

	ctx := context.Background()
	ev := buildCardPackPurchasedOutbox(t, "player-123", "faction_set_tenki")
	require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded domain.CardPackPurchasedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, domain.EventTypeCardPackPurchased, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload の eventId は outbox 行の PK と一致する")
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	assert.Equal(t, "player-123", decoded.PlayerID)
	assert.Equal(t, "faction_set_tenki", decoded.CardPackID)
}

// faction-acquired payload が Publisher で送信できる shape であることを固定する。
func TestIntegration_PublishFactionAcquired(t *testing.T) {
	pub, topics := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topics.factionAcquired)

	ctx := context.Background()
	ev := buildFactionAcquiredOutbox(t, "player-456", "Tenki")
	require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded domain.FactionAcquiredEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, domain.EventTypeFactionAcquired, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID)
	assert.Equal(t, "player-456", decoded.PlayerID)
	assert.Equal(t, "Tenki", decoded.Faction)
}

// premium 付与 (expires_at あり) の送信 shape を固定。
func TestIntegration_PublishPremiumUpdated_Granted(t *testing.T) {
	pub, topics := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topics.premiumUpdated)

	ctx := context.Background()
	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC()
	ev := buildPremiumUpdatedOutbox(t, "player-premium", true, &expiresAt)
	require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded domain.PremiumUpdatedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, domain.EventTypePremiumUpdated, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID)
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	assert.Equal(t, "player-premium", decoded.PlayerID)
	assert.True(t, decoded.IsPremium)
	assert.Equal(t, domain.PremiumUpdatedSourceShop, decoded.Source)
	require.NotNil(t, decoded.PremiumExpiresAt)
	assert.WithinDuration(t, expiresAt, *decoded.PremiumExpiresAt, time.Second)
}

// premium 解除 (expires_at=nil) の送信 shape を固定。
func TestIntegration_PublishPremiumUpdated_Revoked(t *testing.T) {
	pub, topics := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topics.premiumUpdated)

	ctx := context.Background()
	ev := buildPremiumUpdatedOutbox(t, "player-not-premium", false, nil)
	require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded domain.PremiumUpdatedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, domain.EventTypePremiumUpdated, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID)
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	assert.Equal(t, "player-not-premium", decoded.PlayerID)
	assert.False(t, decoded.IsPremium)
	assert.Equal(t, domain.PremiumUpdatedSourceShop, decoded.Source)
	assert.Nil(t, decoded.PremiumExpiresAt)
}

// negative path: publish が呼ばれなければ subscriber は timeout する。
func TestIntegration_NoPublish_SubscriberTimesOut(t *testing.T) {
	_, topics := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topics.cardPackPurchased)

	_, err := sub.WaitForMessage(context.Background(), 500*time.Millisecond)
	assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
}
