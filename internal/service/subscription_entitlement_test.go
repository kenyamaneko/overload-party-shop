package service

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
		{"nil", nil, false},
		{"active in period", &apishop.Subscription{Status: apishop.SubscriptionStatusActive, CurrentPeriodEnd: future}, true},
		{"active expired", &apishop.Subscription{Status: apishop.SubscriptionStatusActive, CurrentPeriodEnd: past}, false},
		{"cancelled in period", &apishop.Subscription{Status: apishop.SubscriptionStatusCancelled, CurrentPeriodEnd: future}, true},
		{"cancelled expired", &apishop.Subscription{Status: apishop.SubscriptionStatusCancelled, CurrentPeriodEnd: past}, false},
		{"grace in period", &apishop.Subscription{Status: apishop.SubscriptionStatusGrace, CurrentPeriodEnd: future}, true},
		{"grace expired", &apishop.Subscription{Status: apishop.SubscriptionStatusGrace, CurrentPeriodEnd: past}, false},
		{"expired", &apishop.Subscription{Status: apishop.SubscriptionStatusExpired, CurrentPeriodEnd: future}, false},
		{"revoked", &apishop.Subscription{Status: apishop.SubscriptionStatusRevoked, CurrentPeriodEnd: future}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isEntitled(tt.sub, now))
		})
	}
}
