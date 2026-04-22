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

// Stream.Consume は publish されたメッセージを handler に渡し、戻り値を
// handled channel に流す。Publish → ExpectHandled の同期観測が成り立つ
// 基本契約を固定する。
func TestStream_ConsumesAndExposesHandlerResult(t *testing.T) {
	tests := []struct {
		name        string
		handlerFn   func(ctx context.Context, data []byte) error
		payload     []byte
		wantHandled error
	}{
		{
			name: "handler nil 戻り: ack 相当を handled に流す",
			handlerFn: func(_ context.Context, _ []byte) error {
				return nil
			},
			payload:     []byte(`{"k":"v"}`),
			wantHandled: nil,
		},
		{
			name: "handler error 戻り: nack 相当を handled に流す",
			handlerFn: func(_ context.Context, _ []byte) error {
				return errors.New("boom")
			},
			payload:     []byte(`{}`),
			wantHandled: errors.New("boom"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := apishopfake.NewBroker()
			pub := apishopfake.NewPublisher(broker)
			stream := apishopfake.NewStream(apishopfake.NewSubscriber(broker), "t")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() { _ = stream.Consume(ctx, tt.handlerFn) }()

			require.NoError(t, pub.Publish(ctx, "t", tt.payload))

			got := stream.ExpectHandled(t, time.Second)
			if tt.wantHandled == nil {
				assert.NoError(t, got)
			} else {
				assert.EqualError(t, got, tt.wantHandled.Error())
			}
		})
	}
}

// ctx キャンセル時 Consume は nil を返す (consumer ランナー側で「ctx キャンセル
// = 正常終了」として扱える契約)。
func TestStream_ConsumeReturnsNilOnContextCancel(t *testing.T) {
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
}

// subscribe は NewStream の時点で eager に行う (Consume 前に publish しても届く)。
// このテストが通ることで「NewStream → publish → Consume 開始」順序でも
// メッセージが失われない契約を固定する。
func TestStream_EagerSubscribeSurvivesPublishBeforeConsume(t *testing.T) {
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
}
