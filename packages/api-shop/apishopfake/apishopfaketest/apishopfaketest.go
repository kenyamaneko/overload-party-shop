// Package apishopfaketest は apishopfake のテストが共有する部品を提供する。
package apishopfaketest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
)

const receivePollInterval = 10 * time.Millisecond

// NewPublisherAndSubscriber は同じ配信元を共有する発行側と購読側を返す。
func NewPublisherAndSubscriber() (*apishopfake.Publisher, *apishopfake.Subscriber) {
	broker := apishopfake.NewBroker()
	return apishopfake.NewPublisher(broker), apishopfake.NewSubscriber(broker)
}

// ReceiveWithin は制限時間内にチャネルへ届いた値を1件取り出す。届かなければテストを失敗させる。
func ReceiveWithin[T any](t *testing.T, messages <-chan T, timeout time.Duration) T {
	t.Helper()
	require.Eventually(t, func() bool { return len(messages) > 0 }, timeout, receivePollInterval, "制限時間内に値を受信できなかった")
	return <-messages
}
