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

// Publisher を emulator に向けて構築するヘルパ。全 topic を事前作成。
// 物理 topic 名は infra (Terraform) が SSoT であり、test は production と同じ env 由来で解決する。
func setupPublisher(t *testing.T) (*Publisher, publisherTopics) {
	t.Helper()
	envTopics := loadTopicsFromEnv(t)
	topics := publisherTopics{
		cardPackPurchased: sharedEmulator.CreateTopic(t, envTopics.cardPackPurchased),
		factionAcquired:   sharedEmulator.CreateTopic(t, envTopics.factionAcquired),
		premiumUpdated:    sharedEmulator.CreateTopic(t, envTopics.premiumUpdated),
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
	payload, err := json.Marshal(apishop.CardPackPurchasedEvent{
		EventType:  apishop.EventTypeCardPackPurchased,
		EventID:    id.String(),
		Timestamp:  time.Now().UTC(),
		PlayerID:   playerID,
		CardPackID: cardPackID,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: apishop.EventTypeCardPackPurchased,
		Payload:   payload,
	}
}

func buildFactionAcquiredOutbox(t *testing.T, playerID, faction string) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(apishop.FactionAcquiredEvent{
		EventType: apishop.EventTypeFactionAcquired,
		EventID:   id.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: apishop.EventTypeFactionAcquired,
		Payload:   payload,
	}
}

func buildPremiumUpdatedOutbox(t *testing.T, playerID string, isPremium bool, expiresAt *time.Time) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(apishop.PremiumUpdatedEvent{
		EventType:        apishop.EventTypePremiumUpdated,
		EventID:          id.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		IsPremium:        isPremium,
		PremiumExpiresAt: expiresAt,
		Source:           apishop.PremiumUpdatedSourceShop,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: apishop.EventTypePremiumUpdated,
		Payload:   payload,
	}
}

func TestPublishIntegration(t *testing.T) {
	t.Run("Pub/Sub への配信", func(t *testing.T) {
		t.Run("card-pack-purchased を配信すると、購読側に送信内容がそのまま届く", func(t *testing.T) {
			// outbox 行を worker が送出する経路の近似。
			pub, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.cardPackPurchased)

			ctx := context.Background()
			ev := buildCardPackPurchasedOutbox(t, "player-123", "faction_set_tenki")
			require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

			msg, err := sub.WaitForMessage(ctx, 5*time.Second)
			require.NoError(t, err)

			var decoded apishop.CardPackPurchasedEvent
			require.NoError(t, json.Unmarshal(msg.Data, &decoded))

			assert.Equal(t, apishop.EventTypeCardPackPurchased, decoded.EventType)
			assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload の eventId は outbox 行の PK と一致する")
			assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
			assert.Equal(t, "player-123", decoded.PlayerID)
			assert.Equal(t, "faction_set_tenki", decoded.CardPackID)
		})

		t.Run("faction-acquired を配信すると、送信内容が保たれる", func(t *testing.T) {
			pub, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.factionAcquired)

			ctx := context.Background()
			ev := buildFactionAcquiredOutbox(t, "player-456", "Tenki")
			require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

			msg, err := sub.WaitForMessage(ctx, 5*time.Second)
			require.NoError(t, err)

			var decoded apishop.FactionAcquiredEvent
			require.NoError(t, json.Unmarshal(msg.Data, &decoded))

			assert.Equal(t, apishop.EventTypeFactionAcquired, decoded.EventType)
			assert.Equal(t, ev.EventID.String(), decoded.EventID)
			assert.Equal(t, "player-456", decoded.PlayerID)
			assert.Equal(t, "Tenki", decoded.Faction)
		})

		t.Run("premium 付与 (expires_at あり) を配信すると、送信内容が保たれる", func(t *testing.T) {
			pub, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.premiumUpdated)

			ctx := context.Background()
			expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC()
			ev := buildPremiumUpdatedOutbox(t, "player-premium", true, &expiresAt)
			require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

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
		})

		t.Run("premium 解除 (expires_at=nil) を配信すると、送信内容が保たれる", func(t *testing.T) {
			pub, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.premiumUpdated)

			ctx := context.Background()
			ev := buildPremiumUpdatedOutbox(t, "player-not-premium", false, nil)
			require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

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
		})

		t.Run("配信しなければ、購読側はタイムアウトする", func(t *testing.T) {
			// 正例テストの偽陽性除け。
			_, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.cardPackPurchased)

			_, err := sub.WaitForMessage(context.Background(), 500*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
		})
	})
}
