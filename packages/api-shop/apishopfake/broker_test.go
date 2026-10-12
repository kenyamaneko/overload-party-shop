package apishopfake_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake/apishopfaketest"
)

const messageReceiveTimeout = time.Second

func TestPublishedMessageDelivery(t *testing.T) {
	t.Run("[イベント配信フェイク]メッセージの配信", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("発行したメッセージの配信", func(t *testing.T) {
				cases := []struct {
					name  string
					topic string
					data  []byte
				}{
					{name: "本文が空のとき、購読者に空のまま届く", topic: "TST-TOPIC", data: []byte{}},
					{name: "本文が`{\"k\":\"v\"}`のとき、購読者にそのまま届く", topic: "TST-TOPIC", data: []byte(`{"k":"v"}`)},
					{name: "トピック名がハイフンを含む`topic-with-hyphen`のとき、購読者にそのまま届く", topic: "topic-with-hyphen", data: []byte("TST-DATA-0001")},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
						messages := subscriber.Messages(tc.topic)

						require.NoError(t, publisher.Publish(context.Background(), tc.topic, tc.data))

						assert.Equal(t, string(tc.data), string(apishopfaketest.ReceiveWithin(t, messages, messageReceiveTimeout)))
					})
				}
			})

			t.Run("topic-aに発行したとき、topic-bの購読者が受信できるメッセージは無い", func(t *testing.T) {
				publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
				messages := subscriber.Messages("topic-b")

				require.NoError(t, publisher.Publish(context.Background(), "topic-a", []byte("TST-DATA-0001")))

				assert.Empty(t, messages)
			})

			t.Run("同じトピックの購読者が2人いるとき、2人とも発行したメッセージを受信できる", func(t *testing.T) {
				broker := apishopfake.NewBroker()
				publisher := apishopfake.NewPublisher(broker)
				firstMessages := apishopfake.NewSubscriber(broker).Messages("TST-TOPIC")
				secondMessages := apishopfake.NewSubscriber(broker).Messages("TST-TOPIC")

				require.NoError(t, publisher.Publish(context.Background(), "TST-TOPIC", []byte("TST-DATA-0001")))

				assert.Equal(t, []byte("TST-DATA-0001"), apishopfaketest.ReceiveWithin(t, firstMessages, messageReceiveTimeout))
				assert.Equal(t, []byte("TST-DATA-0001"), apishopfaketest.ReceiveWithin(t, secondMessages, messageReceiveTimeout))
			})

			t.Run("発行したあとに購読を始めた購読者が受信できるメッセージは無い", func(t *testing.T) {
				publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
				require.NoError(t, publisher.Publish(context.Background(), "TST-TOPIC", []byte("TST-DATA-0001")))

				messages := subscriber.Messages("TST-TOPIC")

				assert.Empty(t, messages)
			})
		})
	})
}

func TestPublishedRecords(t *testing.T) {
	t.Run("[イベント配信フェイク]発行記録", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("topic-1にa、topic-2にb、topic-1にcの順に発行したとき、発行記録は発行順にトピックと本文を並べて返る", func(t *testing.T) {
				publisher, _ := apishopfaketest.NewPublisherAndSubscriber()
				require.NoError(t, publisher.Publish(context.Background(), "topic-1", []byte("a")))
				require.NoError(t, publisher.Publish(context.Background(), "topic-2", []byte("b")))
				require.NoError(t, publisher.Publish(context.Background(), "topic-1", []byte("c")))

				got := publisher.Published()

				assert.Equal(t, []apishopfake.PublishedMessage{
					{Topic: "topic-1", Data: []byte("a")},
					{Topic: "topic-2", Data: []byte("b")},
					{Topic: "topic-1", Data: []byte("c")},
				}, got)
			})

			t.Run("返された発行記録のトピックを書き換えても、次に取得する発行記録のトピックは書き換え前のままになる", func(t *testing.T) {
				publisher, _ := apishopfaketest.NewPublisherAndSubscriber()
				require.NoError(t, publisher.Publish(context.Background(), "topic-1", []byte("a")))
				returned := publisher.Published()

				returned[0].Topic = "TST-REWRITTEN"

				assert.Equal(t, "topic-1", publisher.Published()[0].Topic)
			})

			t.Run("返された発行記録の本文を書き換えても、次に取得する発行記録の本文は書き換え前のままになる", func(t *testing.T) {
				publisher, _ := apishopfaketest.NewPublisherAndSubscriber()
				require.NoError(t, publisher.Publish(context.Background(), "topic-1", []byte("a")))
				returned := publisher.Published()

				returned[0].Data[0] = 'z'

				assert.Equal(t, []byte("a"), publisher.Published()[0].Data)
			})

			t.Run("発行した本文を、発行後に呼び出し側が書き換えても、発行記録の本文は発行時のままになる", func(t *testing.T) {
				publisher, _ := apishopfaketest.NewPublisherAndSubscriber()
				data := []byte("a")
				require.NoError(t, publisher.Publish(context.Background(), "topic-1", data))

				data[0] = 'z'

				assert.Equal(t, []byte("a"), publisher.Published()[0].Data)
			})
		})
	})
}
