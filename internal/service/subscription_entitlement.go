package service

import (
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// isEntitled はサブスクリプションが指定時刻に特典有効かを返す。
// active / cancelled (auto-renew off だが期間内) / grace_period (支払い失敗中だが猶予内)
// が期間内である限り特典は有効。expired / revoked は無効。
func isEntitled(sub *apishop.Subscription, now time.Time) bool {
	if sub == nil {
		return false
	}
	switch sub.Status {
	case apishop.SubscriptionStatusActive,
		apishop.SubscriptionStatusCancelled,
		apishop.SubscriptionStatusGrace:
		return sub.CurrentPeriodEnd.After(now)
	}
	return false
}
