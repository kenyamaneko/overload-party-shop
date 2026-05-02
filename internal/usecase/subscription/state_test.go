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

	tests := []struct {
		name    string
		sub     *domain.Subscription
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
			sub:     &domain.Subscription{Status: domain.SubscriptionStatusActive, CurrentPeriodEnd: future},
			want:    true,
			wantErr: false,
		},
		{
			name:    "active だが期限切れ",
			sub:     &domain.Subscription{Status: domain.SubscriptionStatusActive, CurrentPeriodEnd: past},
			want:    false,
			wantErr: false,
		},
		{
			name:    "cancelled だが期間内（自動更新オフ後の残存期間）",
			sub:     &domain.Subscription{Status: domain.SubscriptionStatusCancelled, CurrentPeriodEnd: future},
			want:    true,
			wantErr: false,
		},
		{
			name:    "cancelled で期限切れ",
			sub:     &domain.Subscription{Status: domain.SubscriptionStatusCancelled, CurrentPeriodEnd: past},
			want:    false,
			wantErr: false,
		},
		{
			name:    "grace_period かつ期間内（支払い失敗中の猶予）",
			sub:     &domain.Subscription{Status: domain.SubscriptionStatusGracePeriod, CurrentPeriodEnd: future},
			want:    true,
			wantErr: false,
		},
		{
			name:    "grace_period で期限切れ",
			sub:     &domain.Subscription{Status: domain.SubscriptionStatusGracePeriod, CurrentPeriodEnd: past},
			want:    false,
			wantErr: false,
		},
		{
			name:    "expired は期間内でも無効",
			sub:     &domain.Subscription{Status: domain.SubscriptionStatusExpired, CurrentPeriodEnd: future},
			want:    false,
			wantErr: false,
		},
		{
			name:    "revoked は期間内でも無効",
			sub:     &domain.Subscription{Status: domain.SubscriptionStatusRevoked, CurrentPeriodEnd: future},
			want:    false,
			wantErr: false,
		},
		{
			name:    "未知の status はエラー",
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
}
