package subscription_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

var judgedAt = time.Date(2030, 1, 15, 8, 0, 0, 0, time.UTC)

func buildSubscription(status string, periodEnd time.Time) *domain.Subscription {
	return &domain.Subscription{Status: status, CurrentPeriodEnd: periodEnd}
}

func TestIsEntitled(t *testing.T) {
	t.Run("[プレミアム特典]特典有効判定", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("契約が無いとき、特典は無効になる", func(t *testing.T) {
				isEntitled, err := subscription.IsEntitled(nil, judgedAt)

				require.NoError(t, err)
				assert.False(t, isEntitled)
			})
			t.Run("期限が判定時刻の1ナノ秒後のとき", func(t *testing.T) {
				periodEnd := judgedAt.Add(time.Nanosecond)
				entitledStatuses := []struct {
					name   string
					status string
				}{
					{"契約状態がactiveのとき", "active"},
					{"契約状態がcancelledのとき", "cancelled"},
					{"契約状態がgrace_periodのとき", "grace_period"},
				}
				for _, tt := range entitledStatuses {
					t.Run(tt.name+"、特典は有効になる", func(t *testing.T) {
						isEntitled, err := subscription.IsEntitled(buildSubscription(tt.status, periodEnd), judgedAt)

						require.NoError(t, err)
						assert.True(t, isEntitled)
					})
				}
				notEntitledStatuses := []struct {
					name   string
					status string
				}{
					{"契約状態がexpiredのとき", "expired"},
					{"契約状態がrevokedのとき", "revoked"},
				}
				for _, tt := range notEntitledStatuses {
					t.Run(tt.name+"、特典は無効になる", func(t *testing.T) {
						isEntitled, err := subscription.IsEntitled(buildSubscription(tt.status, periodEnd), judgedAt)

						require.NoError(t, err)
						assert.False(t, isEntitled)
					})
				}
			})
			t.Run("期限が判定時刻と同時刻のとき", func(t *testing.T) {
				statuses := []struct {
					name   string
					status string
				}{
					{"契約状態がactiveのとき", "active"},
					{"契約状態がcancelledのとき", "cancelled"},
					{"契約状態がgrace_periodのとき", "grace_period"},
				}
				for _, tt := range statuses {
					t.Run(tt.name+"、特典は無効になる", func(t *testing.T) {
						isEntitled, err := subscription.IsEntitled(buildSubscription(tt.status, judgedAt), judgedAt)

						require.NoError(t, err)
						assert.False(t, isEntitled)
					})
				}
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("契約状態が定義された五つ以外の値のとき、未知の契約状態を示すエラーで失敗する", func(t *testing.T) {
				_, err := subscription.IsEntitled(buildSubscription("TST-UNDEFINED-STATUS", judgedAt.Add(time.Hour)), judgedAt)

				assert.ErrorContains(t, err, "TST-UNDEFINED-STATUS")
			})
		})
	})
}
