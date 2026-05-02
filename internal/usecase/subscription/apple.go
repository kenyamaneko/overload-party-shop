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
	appleNotifDIDRenew             = "DID_RENEW"
	appleNotifExpired              = "EXPIRED"
	appleNotifGracePeriodExpired   = "GRACE_PERIOD_EXPIRED"
	appleNotifRevoke               = "REVOKE"
	appleNotifDIDChangeRenewStatus = "DID_CHANGE_RENEWAL_STATUS"
	appleSubtypeAutoRenewEnabled   = "AUTO_RENEW_ENABLED"
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

// AppleNotifier は Apple App Store Server Notifications V2 の webhook を処理する。
// signed JWS の検証は jwsVerifier port に委譲する。Google 側の依存は持たない。
type AppleNotifier struct {
	subRepo     port.SubscriptionRepo
	jwsVerifier port.AppleJWSVerifier
}

// NewAppleNotifier は依存を受け取り AppleNotifier を構築する。
func NewAppleNotifier(
	subRepo port.SubscriptionRepo,
	jwsVerifier port.AppleJWSVerifier,
) *AppleNotifier {
	return &AppleNotifier{
		subRepo:     subRepo,
		jwsVerifier: jwsVerifier,
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

	sub, err := n.subRepo.FindSubscriptionByToken(ctx, domain.PlatformIOS, txnInfo.OriginalTransactionID)
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
		sub.Status = domain.SubscriptionStatusActive
		sub.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subRepo, sub, true, &expiresAt); err != nil {
			return err
		}

	case appleNotifExpired, appleNotifGracePeriodExpired:
		sub.Status = domain.SubscriptionStatusExpired
		sub.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subRepo, sub, false, nil); err != nil {
			return err
		}

	case appleNotifRevoke:
		sub.Status = domain.SubscriptionStatusRevoked
		sub.UpdatedAt = time.Now()
		if err := writeWithEvent(ctx, n.subRepo, sub, false, nil); err != nil {
			return err
		}

	case appleNotifDIDChangeRenewStatus:
		if notif.Subtype == appleSubtypeAutoRenewDisabled {
			sub.Status = domain.SubscriptionStatusCancelled
			sub.UpdatedAt = time.Now()
			// プレミアムは current_period_end まで有効 — premium-updated イベントは発行しない
			// (エンタイトルメント維持契約: docs/ARCHITECTURE.md)。
			if err := writeNoEvent(ctx, n.subRepo, sub); err != nil {
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
// verifier は port 経由で注入されており、テストではモックに差し替え可能。
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
