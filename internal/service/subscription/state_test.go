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
		name    string
		sub     *apishop.Subscription
		want    bool
		wantErr bool
	}{
		{
			name:    "サブスク行なし",
			sub:     nil,
			want:    false,
			wantErr: false,
		},
		{
			name:    "active かつ期間内",
			sub:     &apishop.Subscription{Status: apishop.SubscriptionStatusActive, CurrentPeriodEnd: future},
			want:    true,
			wantErr: false,
		},
		{
			name:    "active だが期限切れ",
			sub:     &apishop.Subscription{Status: apishop.SubscriptionStatusActive, CurrentPeriodEnd: past},
			want:    false,
			wantErr: false,
		},
		{
			name:    "cancelled だが期間内（自動更新オフ後の残存期間）",
			sub:     &apishop.Subscription{Status: apishop.SubscriptionStatusCancelled, CurrentPeriodEnd: future},
			want:    true,
			wantErr: false,
		},
		{
			name:    "cancelled で期限切れ",
			sub:     &apishop.Subscription{Status: apishop.SubscriptionStatusCancelled, CurrentPeriodEnd: past},
			want:    false,
			wantErr: false,
		},
		{
			name:    "grace_period かつ期間内（支払い失敗中の猶予）",
			sub:     &apishop.Subscription{Status: apishop.SubscriptionStatusGracePeriod, CurrentPeriodEnd: future},
			want:    true,
			wantErr: false,
		},
		{
			name:    "grace_period で期限切れ",
			sub:     &apishop.Subscription{Status: apishop.SubscriptionStatusGracePeriod, CurrentPeriodEnd: past},
			want:    false,
			wantErr: false,
		},
		{
			name:    "expired は期間内でも無効",
			sub:     &apishop.Subscription{Status: apishop.SubscriptionStatusExpired, CurrentPeriodEnd: future},
			want:    false,
			wantErr: false,
		},
		{
			name:    "revoked は期間内でも無効",
			sub:     &apishop.Subscription{Status: apishop.SubscriptionStatusRevoked, CurrentPeriodEnd: future},
			want:    false,
			wantErr: false,
		},
		{
			name:    "未知の status はエラー",
			sub:     &apishop.Subscription{Status: "bogus", CurrentPeriodEnd: future},
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
}
