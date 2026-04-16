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

	pubsubevents "github.com/kenyamaneko/overload-party-common/packages/pubsub-events"
	"github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub/pubsubtest"
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

// Publisher を emulator に向けて構築するヘルパ。両 topic を事前に作成し、
// production コードの gpubsub.NewClient(projectID) が PUBSUB_EMULATOR_HOST
// 経由で emulator に接続することを検証する (StartEmulator が env に設定)。
func setupPublisher(t *testing.T) (*Publisher, string, string) {
	t.Helper()
	factionTopic := sharedEmulator.CreateTopic(t, "faction-selected")
	premiumTopic := sharedEmulator.CreateTopic(t, "premium-updated")

	ctx := context.Background()
	pub, err := New(ctx, sharedEmulator.ProjectID(), factionTopic, premiumTopic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })
	return pub, factionTopic, premiumTopic
}

// PublishFactionSelected は subscribe 可能な topic に正しい shape の
// FactionSelectedEvent JSON を送信することを固定する。
func TestIntegration_PublishFactionSelected(t *testing.T) {
	pub, factionTopic, _ := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, factionTopic)

	ctx := context.Background()
	require.NoError(t, pub.PublishFactionSelected(ctx, "player-123", "Tenki"))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var ev pubsubevents.FactionSelectedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &ev))

	assert.Equal(t, pubsubevents.EventTypeFactionSelected, ev.EventType)
	assert.NotEmpty(t, ev.EventID, "EventID should be generated (UUID)")
	assert.WithinDuration(t, time.Now(), ev.Timestamp, 5*time.Second)
	assert.Equal(t, "player-123", ev.PlayerID)
	assert.Equal(t, "Tenki", ev.Faction)
	assert.Equal(t, pubsubevents.FactionSourceShopPurchase, ev.Source)
}

// PublishPremiumUpdated (premium 付与): expires_at 付きで publish すると
// PremiumExpiresAt が設定された event が送信される。
func TestIntegration_PublishPremiumUpdated_Granted(t *testing.T) {
	pub, _, premiumTopic := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, premiumTopic)

	ctx := context.Background()
	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC()
	require.NoError(t, pub.PublishPremiumUpdated(ctx, "player-premium", true, &expiresAt))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var ev pubsubevents.PremiumUpdatedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &ev))

	assert.Equal(t, pubsubevents.EventTypePremiumUpdated, ev.EventType)
	assert.NotEmpty(t, ev.EventID)
	assert.WithinDuration(t, time.Now(), ev.Timestamp, 5*time.Second)
	assert.Equal(t, "player-premium", ev.PlayerID)
	assert.True(t, ev.IsPremium)
	assert.Equal(t, pubsubevents.PremiumUpdatedSourceShop, ev.Source)
	require.NotNil(t, ev.PremiumExpiresAt)
	assert.WithinDuration(t, expiresAt, *ev.PremiumExpiresAt, time.Second)
}

// PublishPremiumUpdated (premium 解除): expires_at=nil で publish すると
// PremiumExpiresAt も nil の event が送信される (omitempty で field 省略)。
func TestIntegration_PublishPremiumUpdated_Revoked(t *testing.T) {
	pub, _, premiumTopic := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, premiumTopic)

	ctx := context.Background()
	require.NoError(t, pub.PublishPremiumUpdated(ctx, "player-not-premium", false, nil))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var ev pubsubevents.PremiumUpdatedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &ev))

	assert.Equal(t, pubsubevents.EventTypePremiumUpdated, ev.EventType)
	assert.NotEmpty(t, ev.EventID)
	assert.WithinDuration(t, time.Now(), ev.Timestamp, 5*time.Second)
	assert.Equal(t, "player-not-premium", ev.PlayerID)
	assert.False(t, ev.IsPremium)
	assert.Equal(t, pubsubevents.PremiumUpdatedSourceShop, ev.Source)
	assert.Nil(t, ev.PremiumExpiresAt)
}

// negative path: publish が呼ばれなければ subscriber は timeout する。
// WaitForMessage が ErrTimeout を返すことで「publish されないこと」を
// assert できる形になっていることを確認する (他リポで consumer 条件分岐の
// 抑止を検証する時に使うパターン)。
func TestIntegration_NoPublish_SubscriberTimesOut(t *testing.T) {
	_, factionTopic, _ := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, factionTopic)

	_, err := sub.WaitForMessage(context.Background(), 500*time.Millisecond)
	assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
}
