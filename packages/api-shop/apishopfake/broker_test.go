package apishopfake_test

import (
	"context"
	"testing"
	"time"

	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroker(t *testing.T) {
	t.Run("ブローカーの配送と分離", func(t *testing.T) {
		deliveryCases := []struct {
			name    string
			topic   string
			payload string
		}{
			{
				name:    "空ペイロードのとき、そのまま subscriber に届く",
				topic:   "t",
				payload: "",
			},
			{
				name:    "json ペイロードのとき、そのまま subscriber に届く",
				topic:   "a",
				payload: `{"k":"v"}`,
			},
			{
				name:    "topic 名に hyphen を含むとき、そのまま subscriber に届く",
				topic:   "faction-purchased",
				payload: `{"x":1}`,
			},
		}
		for _, tt := range deliveryCases {
			t.Run(tt.name, func(t *testing.T) {
				broker := apishopfake.NewBroker()
				pub := apishopfake.NewPublisher(broker)
				sub := apishopfake.NewSubscriber(broker)

				ch := sub.Messages(tt.topic)
				require.NoError(t, pub.Publish(context.Background(), tt.topic, []byte(tt.payload)))

				got := receiveWithin(t, ch, time.Second)
				assert.Equal(t, tt.payload, string(got))
			})
		}

		t.Run("別 topic に publish したとき、別 topic の subscriber には届かない", func(t *testing.T) {
			// 実 Pub/Sub の topic-based routing を fake でも保つ。
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)

			chA := sub.Messages("topic-a")
			chB := sub.Messages("topic-b")

			require.NoError(t, pub.Publish(context.Background(), "topic-a", []byte(`a`)))

			assert.Equal(t, `a`, string(receiveWithin(t, chA, time.Second)))
			assertNoMessageWithin(t, chB, 50*time.Millisecond)
		})

		t.Run("同一 topic に複数 subscriber がいるとき、全員に配信される", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)

			ch1 := sub.Messages("topic")
			ch2 := sub.Messages("topic")

			require.NoError(t, pub.Publish(context.Background(), "topic", []byte(`x`)))

			for i, ch := range []<-chan []byte{ch1, ch2} {
				assert.Equal(t, `x`, string(receiveWithin(t, ch, time.Second)), "subscriber %d", i+1)
			}
		})

		t.Run("subscribe より先に publish したとき、後から購読しても届かない", func(t *testing.T) {
			// 実 Pub/Sub の新規 subscription 挙動に合わせ、過去メッセージを再生しない。
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			sub := apishopfake.NewSubscriber(broker)

			require.NoError(t, pub.Publish(context.Background(), "topic", []byte(`early`)))

			chLate := sub.Messages("topic")
			assertNoMessageWithin(t, chLate, 50*time.Millisecond)
		})
	})
}

func TestPublisher(t *testing.T) {
	t.Run("Publisher の発行記録", func(t *testing.T) {
		t.Run("publish した全メッセージを発行順に Published() で返す", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			ctx := context.Background()

			require.NoError(t, pub.Publish(ctx, "t1", []byte(`a`)))
			require.NoError(t, pub.Publish(ctx, "t2", []byte(`b`)))
			require.NoError(t, pub.Publish(ctx, "t1", []byte(`c`)))

			history := pub.Published()
			require.Len(t, history, 3)
			assert.Equal(t, "t1", history[0].Topic)
			assert.Equal(t, `a`, string(history[0].Data))
			assert.Equal(t, "t2", history[1].Topic)
			assert.Equal(t, `b`, string(history[1].Data))
			assert.Equal(t, "t1", history[2].Topic)
			assert.Equal(t, `c`, string(history[2].Data))
		})

		t.Run("Published() の戻り値を caller が mutate しても、内部状態には影響しない", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			require.NoError(t, pub.Publish(context.Background(), "t", []byte(`orig`)))

			snap := pub.Published()
			snap[0].Topic = "mutated"
			snap[0].Data[0] = 'X'

			again := pub.Published()
			assert.Equal(t, "t", again[0].Topic)
			assert.Equal(t, `orig`, string(again[0].Data))
		})
	})
}

// receiveWithin は channel からメッセージを timeout 付きで受信する (timeout 時は t.Fatal)。
func receiveWithin(t *testing.T, ch <-chan []byte, timeout time.Duration) []byte {
	t.Helper()
	select {
	case data := <-ch:
		return data
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for message after %s", timeout)
		return nil
	}
}

// assertNoMessageWithin は within 内に channel へメッセージが来ないことを確認する。
func assertNoMessageWithin(t *testing.T, ch <-chan []byte, within time.Duration) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("expected no message but received %s", got)
	case <-time.After(within):
	}
}
