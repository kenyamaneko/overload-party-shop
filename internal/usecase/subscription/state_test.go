package subscription

import (
	"testing"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestIsEntitled(t *testing.T) {
	now := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	t.Run("特典有効判定", func(t *testing.T) {
		tests := []struct {
			name    string
			sub     *domain.Subscription
			want    bool
			wantErr bool
		}{
			{
				name:    "サブスク行が無いとき、無効になる",
				sub:     nil,
				want:    false,
				wantErr: false,
			},
			{
				name:    "activeかつ期間内のとき、有効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusActive, CurrentPeriodEnd: future},
				want:    true,
				wantErr: false,
			},
			{
				name:    "activeでも期限切れのとき、無効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusActive, CurrentPeriodEnd: past},
				want:    false,
				wantErr: false,
			},
			{
				name:    "activeで期限がちょうど判定時刻と同時刻のとき、無効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusActive, CurrentPeriodEnd: now},
				want:    false,
				wantErr: false,
			},
			{
				name:    "activeで期限が判定時刻の1ns後のとき、有効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusActive, CurrentPeriodEnd: now.Add(1)},
				want:    true,
				wantErr: false,
			},
			// cancelled は自動更新オフ後も current_period_end まで特典を維持する。
			{
				name:    "cancelledでも期間内のとき、有効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusCancelled, CurrentPeriodEnd: future},
				want:    true,
				wantErr: false,
			},
			{
				name:    "cancelledかつ期限切れのとき、無効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusCancelled, CurrentPeriodEnd: past},
				want:    false,
				wantErr: false,
			},
			// grace_period は支払い失敗中の猶予として current_period_end まで特典を維持する。
			{
				name:    "grace_periodかつ期間内のとき、有効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusGracePeriod, CurrentPeriodEnd: future},
				want:    true,
				wantErr: false,
			},
			{
				name:    "grace_periodかつ期限切れのとき、無効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusGracePeriod, CurrentPeriodEnd: past},
				want:    false,
				wantErr: false,
			},
			{
				name:    "expiredは期間内でも無効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusExpired, CurrentPeriodEnd: future},
				want:    false,
				wantErr: false,
			},
			{
				name:    "revokedは期間内でも無効になる",
				sub:     &domain.Subscription{Status: domain.SubscriptionStatusRevoked, CurrentPeriodEnd: future},
				want:    false,
				wantErr: false,
			},
			{
				name:    "未知のstatusのとき、エラーになる",
				sub:     &domain.Subscription{Status: "bogus", CurrentPeriodEnd: future},
				want:    false,
				wantErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := IsEntitled(tt.sub, now)
				assert.Equal(t, tt.want, got)
				assert.Equal(t, tt.wantErr, err != nil)
			})
		}
	})
}
