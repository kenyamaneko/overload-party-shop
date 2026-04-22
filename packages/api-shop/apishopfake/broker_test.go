package apishopfake_test

import (
	"context"
	"testing"
	"time"

	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Publisher に投げた payload は、同一 broker で Messages(topic) を事前に呼んだ
// Subscriber に届く — pub/sub の最も基本的な契約。
func TestBroker_DeliversPayloadToSubscriber(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		payload string
	}{
		{
			name:    "空ペイロード",
			topic:   "t",
			payload: "",
		},
		{
			name:    "json ペイロード",
			topic:   "a",
			payload: `{"k":"v"}`,
		},
		{
			name:    "topic 名に hyphen を含む",
			topic:   "faction-purchased",
			payload: `{"x":1}`,
		},
	}
	for _, tt := range tests {
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
}

// topic 別 isolation: 他 topic への publish は、別 topic の subscriber には届かない。
// 実 Pub/Sub の topic-based routing を fake でも保つための契約。
func TestBroker_IsolatesByTopic(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)

	chA := sub.Messages("topic-a")
	chB := sub.Messages("topic-b")

	require.NoError(t, pub.Publish(context.Background(), "topic-a", []byte(`a`)))

	assert.Equal(t, `a`, string(receiveWithin(t, chA, time.Second)))
	assertNoMessageWithin(t, chB, 50*time.Millisecond)
}

// fan-out: 同一 topic に複数 subscriber がある場合、全員に独立に配信される。
// gateway / account / card が同じ topic を consume する構成を 1 テスト内で
// 表現するため必要な性質。
func TestBroker_FansOutToMultipleSubscribers(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)

	ch1 := sub.Messages("topic")
	ch2 := sub.Messages("topic")

	require.NoError(t, pub.Publish(context.Background(), "topic", []byte(`x`)))

	for i, ch := range []<-chan []byte{ch1, ch2} {
		assert.Equal(t, `x`, string(receiveWithin(t, ch, time.Second)), "subscriber %d", i+1)
	}
}

// publish は subscribe より先に起こると subscriber に届かない — 実 Pub/Sub の
// 新規 subscription 挙動に揃えた意図的な仕様。この契約を破ると過去メッセージの
// 意図しない再生が subscriber 側テストで起きる。
func TestBroker_DoesNotDeliverToLateSubscriber(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)

	require.NoError(t, pub.Publish(context.Background(), "topic", []byte(`early`)))

	chLate := sub.Messages("topic")
	assertNoMessageWithin(t, chLate, 50*time.Millisecond)
}

// Publisher.Published() は publish した全メッセージを発行順に返す。送信側
// サービスの publisher 境界テストで「意図した topic/payload で publish したか」
// をアサートするための中核 API。
func TestPublisher_PublishedRecordsInOrder(t *testing.T) {
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
}

// Published() の戻り値は内部状態から切り離された snapshot で、caller が mutate
// しても Publisher 内部 / 次回呼び出し結果には影響しない。テスト間で共有状態を
// 誤って持ち越さないための契約。
func TestPublisher_PublishedSnapshotIsIndependent(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	require.NoError(t, pub.Publish(context.Background(), "t", []byte(`orig`)))

	snap := pub.Published()
	snap[0].Topic = "mutated"
	snap[0].Data[0] = 'X'

	again := pub.Published()
	assert.Equal(t, "t", again[0].Topic)
	assert.Equal(t, `orig`, string(again[0].Data))
}

// receiveWithin は channel からメッセージを timeout 付きで受信する。timeout 時は
// 即 t.Fatal。テスト内で select+time.After を書き散らさないための helper。
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

// assertNoMessageWithin は within 内は channel にメッセージが来ないことを確認する。
// isolation / late-subscribe 系のネガティブ確認で使う。
func assertNoMessageWithin(t *testing.T, ch <-chan []byte, within time.Duration) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("expected no message but received %s", got)
	case <-time.After(within):
	}
}
