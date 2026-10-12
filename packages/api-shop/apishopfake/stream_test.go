package apishopfake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake/apishopfaketest"
)

const (
	streamTopic       = "TST-TOPIC"
	streamWaitTimeout = time.Second
)

func startConsuming(
	ctx context.Context,
	stream *apishopfake.Stream,
	handler func(ctx context.Context, data []byte) error,
) <-chan error {
	finished := make(chan error, 1)
	go func() {
		finished <- stream.Consume(ctx, handler)
	}()
	return finished
}

func succeedHandler(context.Context, []byte) error {
	return nil
}

func TestStreamConsumption(t *testing.T) {
	t.Run("[イベント配信フェイク]ストリームの消費", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("メッセージの処理が成功したとき、そのメッセージの処理結果を確認すると成功(エラーなし)になる", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
				stream := apishopfake.NewStream(subscriber, streamTopic)
				startConsuming(ctx, stream, succeedHandler)

				require.NoError(t, publisher.Publish(ctx, streamTopic, []byte("TST-DATA-0001")))

				assert.NoError(t, stream.ExpectHandled(t, streamWaitTimeout))
			})

			t.Run("メッセージの処理がエラーboomで失敗したとき、そのメッセージの処理結果を確認すると同じエラーboomになる", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				boom := errors.New("boom")
				publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
				stream := apishopfake.NewStream(subscriber, streamTopic)
				startConsuming(ctx, stream, func(context.Context, []byte) error {
					return boom
				})

				require.NoError(t, publisher.Publish(ctx, streamTopic, []byte("TST-DATA-0001")))

				assert.Equal(t, boom, stream.ExpectHandled(t, streamWaitTimeout))
			})

			t.Run("ストリームを作ったあと、消費を始める前に発行したメッセージは、消費を始めたあとにメッセージの処理がそのまま受け取る", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				publisher, subscriber := apishopfaketest.NewPublisherAndSubscriber()
				stream := apishopfake.NewStream(subscriber, streamTopic)
				require.NoError(t, publisher.Publish(ctx, streamTopic, []byte("TST-DATA-0001")))
				handled := make(chan []byte, 1)

				startConsuming(ctx, stream, func(_ context.Context, data []byte) error {
					handled <- data
					return nil
				})

				assert.Equal(t, []byte("TST-DATA-0001"), apishopfaketest.ReceiveWithin(t, handled, streamWaitTimeout))
			})

			t.Run("消費中に消費の中止を指示したとき、消費はエラーなしで終わる", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				_, subscriber := apishopfaketest.NewPublisherAndSubscriber()
				stream := apishopfake.NewStream(subscriber, streamTopic)
				finished := startConsuming(ctx, stream, succeedHandler)

				cancel()

				assert.NoError(t, apishopfaketest.ReceiveWithin(t, finished, streamWaitTimeout))
			})
		})
	})
}
