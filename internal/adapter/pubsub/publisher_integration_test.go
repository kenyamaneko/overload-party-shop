//go:build integration

package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub/pubsubtest"
)

const (
	testProjectID  = "test-project"
	receiveTimeout = 20 * time.Second
	publishTimeout = 30 * time.Second
)

type publisherFixture struct {
	publisher     *pubsub.Publisher
	subscriptions map[string]*pubsubtest.Subscription
}

func newPublisherFixture(t *testing.T, emulator *pubsubtest.Emulator) publisherFixture {
	t.Helper()
	cardPackTopic := emulator.CreateTopic(t, "test-card-pack")
	factionTopic := emulator.CreateTopic(t, "test-faction")
	premiumTopic := emulator.CreateTopic(t, "test-premium")
	subscriptions := map[string]*pubsubtest.Subscription{
		"card_pack_purchased": emulator.Subscribe(t, cardPackTopic),
		"faction_acquired":    emulator.Subscribe(t, factionTopic),
		"premium_updated":     emulator.Subscribe(t, premiumTopic),
	}
	publisher, err := pubsub.New(context.Background(), emulator.ProjectID(), cardPackTopic, factionTopic, premiumTopic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = publisher.Close() })
	return publisherFixture{publisher: publisher, subscriptions: subscriptions}
}

func TestPublisher_Publish(t *testing.T) {
	emulator, err := pubsubtest.StartEmulator(context.Background(), testProjectID)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, emulator.Close(context.Background())) })

	t.Run("[shopイベントのパブリッシュ]event_typeに対応するトピックへのパブリッシュ", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("パブリッシュすると", func(t *testing.T) {
				deliveryTests := []struct {
					name      string
					eventType string
					data      []byte
				}{
					{
						name:      "event_typeがcard_pack_purchasedのとき、そのevent_type用に指定したトピックのサブスクライブ側に、パブリッシュしたメッセージのデータがバイト列のまま届く",
						eventType: "card_pack_purchased",
						data:      []byte{0x01, 0x00, 0xfe, 0xff},
					},
					{
						name:      "event_typeがfaction_acquiredのとき、そのevent_type用に指定したトピックのサブスクライブ側に、パブリッシュしたメッセージのデータがバイト列のまま届く",
						eventType: "faction_acquired",
						data:      []byte{0x02, 0x00, 0xfe, 0xff},
					},
					{
						name:      "event_typeがpremium_updatedのとき、そのevent_type用に指定したトピックのサブスクライブ側に、パブリッシュしたメッセージのデータがバイト列のまま届く",
						eventType: "premium_updated",
						data:      []byte{0x03, 0x00, 0xfe, 0xff},
					},
				}
				for _, tc := range deliveryTests {
					t.Run(tc.name, func(t *testing.T) {
						fixture := newPublisherFixture(t, emulator)

						err := fixture.publisher.Publish(context.Background(), tc.eventType, tc.data)

						require.NoError(t, err)
						got, err := fixture.subscriptions[tc.eventType].WaitForMessage(context.Background(), receiveTimeout)
						require.NoError(t, err)
						assert.Equal(t, tc.data, got.Data)
					})
				}
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("event_typeが未登録のとき", func(t *testing.T) {
				t.Run("event_typeがunknown_eventのとき、パブリッシュするとエラーの内容に未登録のevent_typeとunknown_eventが含まれる", func(t *testing.T) {
					fixture := newPublisherFixture(t, emulator)

					err := fixture.publisher.Publish(context.Background(), "unknown_event", []byte("test-data"))

					assert.ErrorContains(t, err, `unknown event type "unknown_event"`)
				})

				t.Run("event_typeがunknown_eventのパブリッシュに失敗したあと、card_pack_purchasedをパブリッシュすると、card_pack_purchased用に指定したトピックのサブスクライブ側に届く", func(t *testing.T) {
					fixture := newPublisherFixture(t, emulator)
					_ = fixture.publisher.Publish(context.Background(), "unknown_event", []byte("test-data"))

					err := fixture.publisher.Publish(context.Background(), "card_pack_purchased", []byte("test-data-after-failure"))

					require.NoError(t, err)
					got, err := fixture.subscriptions["card_pack_purchased"].WaitForMessage(context.Background(), receiveTimeout)
					require.NoError(t, err)
					assert.Equal(t, []byte("test-data-after-failure"), got.Data)
				})
			})

			t.Run("event_typeに対応するトピックがエミュレータ上に存在しないとき", func(t *testing.T) {
				t.Run("card_pack_purchasedをパブリッシュすると、エラーの内容にパブリッシュ失敗とcard_pack_purchasedが含まれる", func(t *testing.T) {
					publisher, err := pubsub.New(context.Background(), emulator.ProjectID(), "missing-card-pack-error", "missing-faction-error", "missing-premium-error")
					require.NoError(t, err)
					t.Cleanup(func() { _ = publisher.Close() })
					ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
					defer cancel()

					err = publisher.Publish(ctx, "card_pack_purchased", []byte("test-data"))

					assert.ErrorContains(t, err, "publish event_type=card_pack_purchased")
				})

				t.Run("card_pack_purchasedのパブリッシュに失敗したあと、トピックとサブスクリプションを作成し、同じパブリッシャーでcard_pack_purchasedを再度パブリッシュすると、サブスクライブ側に届く", func(t *testing.T) {
					const missingTopic = "missing-card-pack-recovery"
					publisher, err := pubsub.New(context.Background(), emulator.ProjectID(), missingTopic, "missing-faction-recovery", "missing-premium-recovery")
					require.NoError(t, err)
					t.Cleanup(func() { _ = publisher.Close() })
					ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
					defer cancel()
					_ = publisher.Publish(ctx, "card_pack_purchased", []byte("test-data"))
					emulator.CreateTopicWithID(t, missingTopic)
					subscription := emulator.Subscribe(t, missingTopic)

					err = publisher.Publish(ctx, "card_pack_purchased", []byte("test-data-after-failure"))

					require.NoError(t, err)
					got, err := subscription.WaitForMessage(context.Background(), receiveTimeout)
					require.NoError(t, err)
					assert.Equal(t, []byte("test-data-after-failure"), got.Data)
				})
			})
		})
	})
}
