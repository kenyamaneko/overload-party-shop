package presenter

import (
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

const (
	purchaseAcceptedMessage     = "purchase accepted"
	subscriptionAcceptedMessage = "subscription accepted"
)

// ToPurchaseResponse は単発購入の成功を wire の PurchaseResponse に詰め替える。
func ToPurchaseResponse(productID string) apishop.PurchaseResponse {
	return apishop.PurchaseResponse{
		Message:   purchaseAcceptedMessage,
		ProductID: productID,
	}
}

// ToSubscribeResponse はサブスクリプション購入の成功を wire の SubscribeResponse に詰め替える。
func ToSubscribeResponse(expiresAt time.Time) apishop.SubscribeResponse {
	return apishop.SubscribeResponse{
		Message:   subscriptionAcceptedMessage,
		ExpiresAt: expiresAt,
	}
}
