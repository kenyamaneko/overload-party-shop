package apishopfake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStream(t *testing.T) {
	t.Run("Streamのconsumeとhandler結果の公開", func(t *testing.T) {
		t.Run("handlerがnilを返すとき、handledにnilが流れる", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			stream := apishopfake.NewStream(apishopfake.NewSubscriber(broker), "t")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _ = stream.Consume(ctx, func(_ context.Context, _ []byte) error { return nil }) }()

			require.NoError(t, pub.Publish(ctx, "t", []byte(`{"k":"v"}`)))

			got := stream.ExpectHandled(t, time.Second)
			assert.NoError(t, got)
		})

		t.Run("handlerがerrorを返すとき、handledに同じerrorが流れる", func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			stream := apishopfake.NewStream(apishopfake.NewSubscriber(broker), "t")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				_ = stream.Consume(ctx, func(_ context.Context, _ []byte) error { return errors.New("boom") })
			}()

			require.NoError(t, pub.Publish(ctx, "t", []byte(`{}`)))

			got := stream.ExpectHandled(t, time.Second)
			assert.EqualError(t, got, "boom")
		})

		t.Run("ctxがキャンセルされたとき、Consumeはnilを返す", func(t *testing.T) {
			// consumer ランナー側で「ctx キャンセル = 正常終了」として扱える契約。
			broker := apishopfake.NewBroker()
			stream := apishopfake.NewStream(apishopfake.NewSubscriber(broker), "t")

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				done <- stream.Consume(ctx, func(_ context.Context, _ []byte) error { return nil })
			}()

			cancel()

			select {
			case err := <-done:
				assert.NoError(t, err, "ctx キャンセルは nil 終了")
			case <-time.After(time.Second):
				t.Fatal("Consume did not return after ctx cancel")
			}
		})

		t.Run("Consume開始前にpublishしても、eager subscribeでメッセージは失われない", func(t *testing.T) {
			// subscribe は NewStream の時点で行うため「NewStream → publish → Consume 開始」順序でも届く。
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			stream := apishopfake.NewStream(apishopfake.NewSubscriber(broker), "t")

			require.NoError(t, pub.Publish(context.Background(), "t", []byte(`x`)))

			var received []byte
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				_ = stream.Consume(ctx, func(_ context.Context, data []byte) error {
					received = data
					return nil
				})
			}()

			stream.ExpectHandled(t, time.Second)
			assert.Equal(t, `x`, string(received))
		})
	})
}
