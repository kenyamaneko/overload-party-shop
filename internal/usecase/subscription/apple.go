package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// Apple App Store Server Notification V2 の通知種別。
const (
	appleNotificationDIDRenew             = "DID_RENEW"
	appleNotificationExpired              = "EXPIRED"
	appleNotificationGracePeriodExpired   = "GRACE_PERIOD_EXPIRED"
	appleNotificationRevoke               = "REVOKE"
	appleNotificationDIDChangeRenewStatus = "DID_CHANGE_RENEWAL_STATUS"
	appleSubtypeAutoRenewEnabled          = "AUTO_RENEW_ENABLED"
	appleSubtypeAutoRenewDisabled         = "AUTO_RENEW_DISABLED"
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

// AppleNotifier は Apple App Store Server Notifications V2 の webhook を処理する。
type AppleNotifier struct {
	subscriptionRepo port.SubscriptionRepo
	jwsVerifier      port.AppleJWSVerifier
}

func NewAppleNotifier(
	subscriptionRepo port.SubscriptionRepo,
	jwsVerifier port.AppleJWSVerifier,
) *AppleNotifier {
	return &AppleNotifier{
		subscriptionRepo: subscriptionRepo,
		jwsVerifier:      jwsVerifier,
	}
}

// HandleNotification は Apple App Store Server Notification V2 を処理する。
func (n *AppleNotifier) HandleNotification(ctx context.Context, signedPayload string) error {
	notif, err := decodeAppleJWS[appleNotification](n.jwsVerifier, signedPayload)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDecodeNotification, err)
	}

	txnInfo, err := decodeAppleJWS[appleNotificationTxn](n.jwsVerifier, notif.Data.SignedTransactionInfo)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDecodeTransactionInfo, err)
	}

	subscription, err := n.subscriptionRepo.FindSubscriptionByToken(ctx, domain.PlatformIOS, txnInfo.OriginalTransactionID)
	if err != nil {
		return fmt.Errorf("find subscription: %w", err)
	}
	if subscription == nil {
		return fmt.Errorf("%w: token=%s", ErrSubscriptionNotFound, txnInfo.OriginalTransactionID)
	}

	switch notif.NotificationType {
	case appleNotificationDIDRenew:
		expiresAt := time.UnixMilli(txnInfo.ExpiresDate)
		subscription.CurrentPeriodEnd = expiresAt
		subscription.Status = domain.SubscriptionStatusActive
		subscription.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subscriptionRepo, subscription, true, &expiresAt); err != nil {
			return err
		}

	case appleNotificationExpired, appleNotificationGracePeriodExpired:
		subscription.Status = domain.SubscriptionStatusExpired
		subscription.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subscriptionRepo, subscription, false, nil); err != nil {
			return err
		}

	case appleNotificationRevoke:
		subscription.Status = domain.SubscriptionStatusRevoked
		subscription.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subscriptionRepo, subscription, false, nil); err != nil {
			return err
		}

	case appleNotificationDIDChangeRenewStatus:
		if notif.Subtype == appleSubtypeAutoRenewDisabled {
			subscription.Status = domain.SubscriptionStatusCancelled
			subscription.UpdatedAt = time.Now()
			// プレミアムは current_period_end まで有効 — premium-updated イベントは発行しない
			if err := writeNoEvent(ctx, n.subscriptionRepo, subscription); err != nil {
				return err
			}
		} else {
			slog.Warn("apple unhandled DID_CHANGE_RENEWAL_STATUS subtype",
				"subtype", notif.Subtype, "original_transaction_id", txnInfo.OriginalTransactionID)
		}

	default:
		slog.Warn("apple unhandled notification type",
			"notification_type", notif.NotificationType, "original_transaction_id", txnInfo.OriginalTransactionID)
	}

	return nil
}

// decodeAppleJWS は Apple JWS を verifier で検証してから payload を T に unmarshal する。
func decodeAppleJWS[T any](v port.AppleJWSVerifier, jws string) (*T, error) {
	payload, err := v.Verify(jws)
	if err != nil {
		return nil, err
	}
	var t T
	if err := json.Unmarshal(payload, &t); err != nil {
		return nil, fmt.Errorf("unmarshal JWS payload: %w", err)
	}
	return &t, nil
}
