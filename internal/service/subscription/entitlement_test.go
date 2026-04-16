package subscription

import (
	"testing"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/stretchr/testify/assert"
)

func TestIsEntitled(t *testing.T) {
	now := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	tests := []struct {
		name string
		sub  *apishop.Subscription
		want bool
	}{
		{
			name: "サブスク行なし",
			sub:  nil,
			want: false,
		},
		{
			name: "active かつ期間内",
			sub:  &apishop.Subscription{Status: apishop.SubscriptionStatusActive, CurrentPeriodEnd: future},
			want: true,
		},
		{
			name: "active だが期限切れ",
			sub:  &apishop.Subscription{Status: apishop.SubscriptionStatusActive, CurrentPeriodEnd: past},
			want: false,
		},
		{
			name: "cancelled だが期間内（自動更新オフ後の残存期間）",
			sub:  &apishop.Subscription{Status: apishop.SubscriptionStatusCancelled, CurrentPeriodEnd: future},
			want: true,
		},
		{
			name: "cancelled で期限切れ",
			sub:  &apishop.Subscription{Status: apishop.SubscriptionStatusCancelled, CurrentPeriodEnd: past},
			want: false,
		},
		{
			name: "grace_period かつ期間内（支払い失敗中の猶予）",
			sub:  &apishop.Subscription{Status: apishop.SubscriptionStatusGrace, CurrentPeriodEnd: future},
			want: true,
		},
		{
			name: "grace_period で期限切れ",
			sub:  &apishop.Subscription{Status: apishop.SubscriptionStatusGrace, CurrentPeriodEnd: past},
			want: false,
		},
		{
			name: "expired は期間内でも無効",
			sub:  &apishop.Subscription{Status: apishop.SubscriptionStatusExpired, CurrentPeriodEnd: future},
			want: false,
		},
		{
			name: "revoked は期間内でも無効",
			sub:  &apishop.Subscription{Status: apishop.SubscriptionStatusRevoked, CurrentPeriodEnd: future},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsEntitled(tt.sub, now))
		})
	}
}
