package subscription

import (
	"context"
	"fmt"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// Apple App Store Server Notification V2 の通知種別。
const (
	appleNotifDIDRenew             = "DID_RENEW"
	appleNotifExpired              = "EXPIRED"
	appleNotifGracePeriodExpired   = "GRACE_PERIOD_EXPIRED"
	appleNotifRevoke               = "REVOKE"
	appleNotifDIDChangeRenewStatus = "DID_CHANGE_RENEWAL_STATUS"
	appleSubtypeAutoRenewDisabled  = "AUTO_RENEW_DISABLED"
)

// AppleNotificationPayload は Apple App Store Server Notifications V2 のリクエストボディ。
type AppleNotificationPayload struct {
	SignedPayload string `json:"signedPayload"`
}

type appleNotification struct {
	NotificationType string `json:"notificationType"`
	Subtype          string `json:"subtype"`
	Data             struct {
		SignedTransactionInfo string `json:"signedTransactionInfo"`
	} `json:"data"`
}

type appleNotificationTxn struct {
	OriginalTransactionID string `json:"originalTransactionId"`
	ExpiresDate           int64  `json:"expiresDate"`
}

// HandleAppleNotification は Apple App Store Server Notification V2 を処理する。
func (s *Service) HandleAppleNotification(ctx context.Context, signedPayload string) error {
	notif, err := decodeVerifiedJWSPayload[appleNotification](signedPayload)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDecodeNotification, err)
	}

	txnInfo, err := decodeVerifiedJWSPayload[appleNotificationTxn](notif.Data.SignedTransactionInfo)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDecodeTransactionInfo, err)
	}

	sub, err := s.subRepo.FindSubscriptionByToken(ctx, apishop.PlatformIOS, txnInfo.OriginalTransactionID)
	if err != nil {
		return fmt.Errorf("find subscription: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("%w: token=%s", ErrSubscriptionNotFound, txnInfo.OriginalTransactionID)
	}

	switch notif.NotificationType {
	case appleNotifDIDRenew:
		expiresAt := time.UnixMilli(txnInfo.ExpiresDate)
		sub.CurrentPeriodEnd = expiresAt
		sub.Status = apishop.SubscriptionStatusActive
		sub.UpdatedAt = time.Now()
		if err := s.applySubChangeWithEvent(ctx, sub, true, &expiresAt); err != nil {
			return err
		}

	case appleNotifExpired, appleNotifGracePeriodExpired:
		sub.Status = apishop.SubscriptionStatusExpired
		sub.UpdatedAt = time.Now()
		if err := s.applySubChangeWithEvent(ctx, sub, false, nil); err != nil {
			return err
		}

	case appleNotifRevoke:
		sub.Status = apishop.SubscriptionStatusRevoked
		sub.UpdatedAt = time.Now()
		if err := s.applySubChangeWithEvent(ctx, sub, false, nil); err != nil {
			return err
		}

	case appleNotifDIDChangeRenewStatus:
		if notif.Subtype == appleSubtypeAutoRenewDisabled {
			sub.Status = apishop.SubscriptionStatusCancelled
			sub.UpdatedAt = time.Now()
			// プレミアムは current_period_end まで有効 — premium-updated イベントは発行しない
			// (エンタイトルメント維持契約: docs/ARCHITECTURE.md)。
			if err := s.applySubChangeNoEvent(ctx, sub); err != nil {
				return err
			}
		}
	}

	return nil
}
