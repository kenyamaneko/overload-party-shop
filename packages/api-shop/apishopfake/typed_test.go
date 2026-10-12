package apishopfake_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake/apishopfaketest"
)

const (
	eventWaitTimeout        = time.Second
	eventShortWaitTimeout   = 50 * time.Millisecond
	uuidV4Pattern           = `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	specifiedTimestampLabel = "2026-01-02T03:04:05Z"
)

func newSpecifiedTimestamp() time.Time {
	return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
}

func publishAndReceiveFactionAcquired(t *testing.T, event apishop.FactionAcquiredEvent) (apishop.FactionAcquiredEvent, *apishopfake.Publisher) {
	t.Helper()
	publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
	expecter := apishopfake.ExpectFactionAcquired(subscriber)
	require.NoError(t, apishopfake.PublishFactionAcquired(context.Background(), publisher, event))
	received, err := expecter.Wait(eventWaitTimeout)
	require.NoError(t, err)
	return received, publisher
}

func publishAndReceiveCardPackPurchased(t *testing.T, event apishop.CardPackPurchasedEvent) (apishop.CardPackPurchasedEvent, *apishopfake.Publisher) {
	t.Helper()
	publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
	expecter := apishopfake.ExpectCardPackPurchased(subscriber)
	require.NoError(t, apishopfake.PublishCardPackPurchased(context.Background(), publisher, event))
	received, err := expecter.Wait(eventWaitTimeout)
	require.NoError(t, err)
	return received, publisher
}

func publishAndReceivePremiumUpdated(t *testing.T, event apishop.PremiumUpdatedEvent) (apishop.PremiumUpdatedEvent, *apishopfake.Publisher) {
	t.Helper()
	publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
	expecter := apishopfake.ExpectPremiumUpdated(subscriber)
	require.NoError(t, apishopfake.PublishPremiumUpdated(context.Background(), publisher, event))
	received, err := expecter.Wait(eventWaitTimeout)
	require.NoError(t, err)
	return received, publisher
}

func newFactionAcquiredEvent() apishop.FactionAcquiredEvent {
	return apishop.FactionAcquiredEvent{PlayerID: "TST-PLAYER-0001", Faction: "TST-0003"}
}

func newCardPackPurchasedEvent() apishop.CardPackPurchasedEvent {
	return apishop.CardPackPurchasedEvent{PlayerID: "TST-PLAYER-0001", CardPackID: "TST-0002"}
}

func newPremiumUpdatedEvent() apishop.PremiumUpdatedEvent {
	return apishop.PremiumUpdatedEvent{PlayerID: "TST-PLAYER-0001", IsPremium: true}
}

func TestPublishFactionAcquired(t *testing.T) {
	t.Run("[陣営獲得イベント]発行と待受", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーIDがTST-PLAYER-0001、陣営がTST-0003のイベントを発行したとき、待受側で受け取ったイベントの内容は発行した内容と同じになる", func(t *testing.T) {
				received, _ := publishAndReceiveFactionAcquired(t, newFactionAcquiredEvent())

				assert.Equal(t, "TST-PLAYER-0001", received.PlayerID)
				assert.Equal(t, "TST-0003", received.Faction)
			})

			t.Run("イベントを発行したとき", func(t *testing.T) {
				eventTypeCases := []struct {
					name      string
					eventType string
				}{
					{name: "イベント種別を指定していないとき、待受側で受け取ったイベントの種別はfaction_acquiredになる", eventType: ""},
					{name: "イベント種別にTST-OTHERを指定したとき、待受側で受け取ったイベントの種別はfaction_acquiredになる", eventType: "TST-OTHER"},
				}
				for _, tc := range eventTypeCases {
					t.Run(tc.name, func(t *testing.T) {
						event := newFactionAcquiredEvent()
						event.EventType = tc.eventType

						received, _ := publishAndReceiveFactionAcquired(t, event)

						assert.Equal(t, "faction_acquired", received.EventType)
					})
				}
			})

			t.Run("イベントIDを指定せずに発行したとき、待受側で受け取ったイベントのIDは空でない", func(t *testing.T) {
				received, _ := publishAndReceiveFactionAcquired(t, newFactionAcquiredEvent())

				assert.NotEmpty(t, received.EventID)
			})

			t.Run("発行日時を指定せずに発行したとき、待受側で受け取ったイベントの発行日時は発行を始めた時刻以降になる", func(t *testing.T) {
				startedAt := time.Now()

				received, _ := publishAndReceiveFactionAcquired(t, newFactionAcquiredEvent())

				assert.False(t, received.Timestamp.Before(startedAt))
			})

			t.Run("イベントIDにTST-0001を指定して発行したとき、待受側で受け取ったイベントのIDはTST-0001になる", func(t *testing.T) {
				event := newFactionAcquiredEvent()
				event.EventID = "TST-0001"

				received, _ := publishAndReceiveFactionAcquired(t, event)

				assert.Equal(t, "TST-0001", received.EventID)
			})

			t.Run("発行日時に2026-01-02T03:04:05Zを指定して発行したとき、待受側で受け取ったイベントの発行日時は2026-01-02T03:04:05Zになる", func(t *testing.T) {
				event := newFactionAcquiredEvent()
				event.Timestamp = newSpecifiedTimestamp()

				received, _ := publishAndReceiveFactionAcquired(t, event)

				assert.Equal(t, specifiedTimestampLabel, received.Timestamp.UTC().Format(time.RFC3339))
			})

			t.Run("発行したとき、発行記録のトピックはfaction-acquiredになる", func(t *testing.T) {
				_, publisher := publishAndReceiveFactionAcquired(t, newFactionAcquiredEvent())

				published := publisher.Published()
				require.Len(t, published, 1)
				assert.Equal(t, "faction-acquired", published[0].Topic)
			})
		})
	})
}

func TestPublishCardPackPurchased(t *testing.T) {
	t.Run("[カードパック購入イベント]発行と待受", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーIDがTST-PLAYER-0001、card_pack_idがTST-0002のイベントを発行したとき、待受側で受け取ったイベントの内容は発行した内容と同じになる", func(t *testing.T) {
				received, _ := publishAndReceiveCardPackPurchased(t, newCardPackPurchasedEvent())

				assert.Equal(t, "TST-PLAYER-0001", received.PlayerID)
				assert.Equal(t, "TST-0002", received.CardPackID)
			})

			t.Run("イベントを発行したとき", func(t *testing.T) {
				eventTypeCases := []struct {
					name      string
					eventType string
				}{
					{name: "イベント種別を指定していないとき、待受側で受け取ったイベントの種別はcard_pack_purchasedになる", eventType: ""},
					{name: "イベント種別にTST-OTHERを指定したとき、待受側で受け取ったイベントの種別はcard_pack_purchasedになる", eventType: "TST-OTHER"},
				}
				for _, tc := range eventTypeCases {
					t.Run(tc.name, func(t *testing.T) {
						event := newCardPackPurchasedEvent()
						event.EventType = tc.eventType

						received, _ := publishAndReceiveCardPackPurchased(t, event)

						assert.Equal(t, "card_pack_purchased", received.EventType)
					})
				}
			})

			t.Run("イベントIDを指定せずに発行したとき、待受側で受け取ったイベントのIDは空でない", func(t *testing.T) {
				received, _ := publishAndReceiveCardPackPurchased(t, newCardPackPurchasedEvent())

				assert.NotEmpty(t, received.EventID)
			})

			t.Run("発行日時を指定せずに発行したとき、待受側で受け取ったイベントの発行日時は発行を始めた時刻以降になる", func(t *testing.T) {
				startedAt := time.Now()

				received, _ := publishAndReceiveCardPackPurchased(t, newCardPackPurchasedEvent())

				assert.False(t, received.Timestamp.Before(startedAt))
			})

			t.Run("イベントIDにTST-0001を指定して発行したとき、待受側で受け取ったイベントのIDはTST-0001になる", func(t *testing.T) {
				event := newCardPackPurchasedEvent()
				event.EventID = "TST-0001"

				received, _ := publishAndReceiveCardPackPurchased(t, event)

				assert.Equal(t, "TST-0001", received.EventID)
			})

			t.Run("発行日時に2026-01-02T03:04:05Zを指定して発行したとき、待受側で受け取ったイベントの発行日時は2026-01-02T03:04:05Zになる", func(t *testing.T) {
				event := newCardPackPurchasedEvent()
				event.Timestamp = newSpecifiedTimestamp()

				received, _ := publishAndReceiveCardPackPurchased(t, event)

				assert.Equal(t, specifiedTimestampLabel, received.Timestamp.UTC().Format(time.RFC3339))
			})

			t.Run("発行したとき、発行記録のトピックはcard-pack-purchasedになる", func(t *testing.T) {
				_, publisher := publishAndReceiveCardPackPurchased(t, newCardPackPurchasedEvent())

				published := publisher.Published()
				require.Len(t, published, 1)
				assert.Equal(t, "card-pack-purchased", published[0].Topic)
			})
		})
	})
}

func TestPublishPremiumUpdated(t *testing.T) {
	t.Run("[プレミアム更新イベント]発行と待受", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーIDがTST-PLAYER-0001、プレミアムが有効、有効期限が2026-02-02T00:00:00Zのイベントを発行したとき、待受側で受け取ったイベントの内容は発行した内容と同じになる", func(t *testing.T) {
				expiresAt := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
				event := newPremiumUpdatedEvent()
				event.PremiumExpiresAt = &expiresAt

				received, _ := publishAndReceivePremiumUpdated(t, event)

				assert.Equal(t, "TST-PLAYER-0001", received.PlayerID)
				assert.True(t, received.IsPremium)
				require.NotNil(t, received.PremiumExpiresAt)
				assert.Equal(t, "2026-02-02T00:00:00Z", received.PremiumExpiresAt.UTC().Format(time.RFC3339))
			})

			t.Run("イベントを発行したとき", func(t *testing.T) {
				eventTypeCases := []struct {
					name      string
					eventType string
				}{
					{name: "イベント種別を指定していないとき、待受側で受け取ったイベントの種別はpremium_updatedになる", eventType: ""},
					{name: "イベント種別にTST-OTHERを指定したとき、待受側で受け取ったイベントの種別はpremium_updatedになる", eventType: "TST-OTHER"},
				}
				for _, tc := range eventTypeCases {
					t.Run(tc.name, func(t *testing.T) {
						event := newPremiumUpdatedEvent()
						event.EventType = tc.eventType

						received, _ := publishAndReceivePremiumUpdated(t, event)

						assert.Equal(t, "premium_updated", received.EventType)
					})
				}
			})

			t.Run("イベントIDを指定せずに発行したとき、待受側で受け取ったイベントのIDは空でない", func(t *testing.T) {
				received, _ := publishAndReceivePremiumUpdated(t, newPremiumUpdatedEvent())

				assert.NotEmpty(t, received.EventID)
			})

			t.Run("発行日時を指定せずに発行したとき、待受側で受け取ったイベントの発行日時は発行を始めた時刻以降になる", func(t *testing.T) {
				startedAt := time.Now()

				received, _ := publishAndReceivePremiumUpdated(t, newPremiumUpdatedEvent())

				assert.False(t, received.Timestamp.Before(startedAt))
			})

			t.Run("イベントIDにTST-0001を指定して発行したとき、待受側で受け取ったイベントのIDはTST-0001になる", func(t *testing.T) {
				event := newPremiumUpdatedEvent()
				event.EventID = "TST-0001"

				received, _ := publishAndReceivePremiumUpdated(t, event)

				assert.Equal(t, "TST-0001", received.EventID)
			})

			t.Run("発行日時に2026-01-02T03:04:05Zを指定して発行したとき、待受側で受け取ったイベントの発行日時は2026-01-02T03:04:05Zになる", func(t *testing.T) {
				event := newPremiumUpdatedEvent()
				event.Timestamp = newSpecifiedTimestamp()

				received, _ := publishAndReceivePremiumUpdated(t, event)

				assert.Equal(t, specifiedTimestampLabel, received.Timestamp.UTC().Format(time.RFC3339))
			})

			t.Run("sourceを指定せずに発行したとき、待受側で受け取ったイベントのsourceはshopになる", func(t *testing.T) {
				received, _ := publishAndReceivePremiumUpdated(t, newPremiumUpdatedEvent())

				assert.Equal(t, "shop", received.Source)
			})

			t.Run("sourceにTST-SOURCEを指定して発行したとき、待受側で受け取ったイベントのsourceはTST-SOURCEになる", func(t *testing.T) {
				event := newPremiumUpdatedEvent()
				event.Source = "TST-SOURCE"

				received, _ := publishAndReceivePremiumUpdated(t, event)

				assert.Equal(t, "TST-SOURCE", received.Source)
			})

			t.Run("発行したとき、発行記録のトピックはpremium-updatedになる", func(t *testing.T) {
				_, publisher := publishAndReceivePremiumUpdated(t, newPremiumUpdatedEvent())

				published := publisher.Published()
				require.Len(t, published, 1)
				assert.Equal(t, "premium-updated", published[0].Topic)
			})
		})
	})
}

func TestExpectEvent(t *testing.T) {
	t.Run("[イベント配信フェイク]イベントの待受", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("待受を開始する前にだけイベントが発行され、待受の制限時間50msの間に何も発行されないとき、待受は時間切れのエラーになる", func(t *testing.T) {
				publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
				require.NoError(t, apishopfake.PublishFactionAcquired(context.Background(), publisher, newFactionAcquiredEvent()))
				expecter := apishopfake.ExpectFactionAcquired(subscriber)

				_, err := expecter.Wait(eventShortWaitTimeout)

				assert.ErrorContains(t, err, "timeout")
			})

			t.Run("待受中のトピックにJSONでない本文`not-json`が発行されたとき、待受は解釈に失敗したエラーになる", func(t *testing.T) {
				publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
				expecter := apishopfake.ExpectFactionAcquired(subscriber)
				require.NoError(t, publisher.Publish(context.Background(), "faction-acquired", []byte("not-json")))

				_, err := expecter.Wait(eventWaitTimeout)

				assert.ErrorContains(t, err, "unmarshal")
				assert.ErrorContains(t, err, "faction-acquired")
			})
		})
	})
}

func TestAssignEventID(t *testing.T) {
	t.Run("[イベント配信フェイク]イベントIDの採番", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("イベントIDを指定せずに発行したとき、受け取ったイベントのIDはUUIDv4の形式になる", func(t *testing.T) {
				received, _ := publishAndReceiveFactionAcquired(t, newFactionAcquiredEvent())

				assert.Regexp(t, uuidV4Pattern, received.EventID)
			})

			t.Run("イベントIDを指定せずに2回発行したとき、2回で受け取ったイベントのIDは異なる", func(t *testing.T) {
				first, _ := publishAndReceiveFactionAcquired(t, newFactionAcquiredEvent())
				second, _ := publishAndReceiveFactionAcquired(t, newFactionAcquiredEvent())

				assert.NotEqual(t, first.EventID, second.EventID)
			})
		})
	})
}
