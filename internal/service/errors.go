package service

import (
	"errors"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var (
	// ErrNotFound は対象が見つからない場合に返される。
	ErrNotFound = port.ErrNotFound
	// ErrFactionAlreadySelected は faction が既に選択済みの場合に返される。
	ErrFactionAlreadySelected = errors.New("faction already selected")
	// ErrInvalidFaction は無効な faction が指定された場合に返される。
	ErrInvalidFaction = errors.New("invalid faction")
	// ErrAlreadyOwned は商品が既に所有されている場合に返される。
	ErrAlreadyOwned = errors.New("product already owned")
	// ErrProductNotActive は非アクティブ商品への操作で返される。
	ErrProductNotActive = errors.New("product is not active")
	// ErrProductNotSubscription はサブスクリプションではない商品への Subscribe で返される。
	ErrProductNotSubscription = errors.New("product is not a subscription")
	// ErrUnsupportedPlatform は未対応プラットフォームが指定された場合に返される。
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	// ErrReceiptVerificationFailed はレシート検証失敗時に返される。
	ErrReceiptVerificationFailed = errors.New("receipt verification failed")
	// ErrSubVerificationFailed はサブスクリプション検証失敗時に返される。
	ErrSubVerificationFailed = errors.New("subscription verification failed")
	// ErrSubscriptionNotFound は webhook 通知に対応する subscription が
	// shop.subscriptions に存在しない場合に返される。
	ErrSubscriptionNotFound = errors.New("subscription not found")
	// ErrDecodeNotification は webhook 通知本体のデコード失敗。
	ErrDecodeNotification = errors.New("decode notification")
	// ErrDecodeTransactionInfo は通知内 signedTransactionInfo のデコード失敗。
	ErrDecodeTransactionInfo = errors.New("decode transaction info")
	// ErrDecodeRTDNData は Google RTDN メッセージ data フィールドの base64 デコード失敗。
	ErrDecodeRTDNData = errors.New("decode RTDN data")
	// ErrUnmarshalRTDNData は Google RTDN data の JSON unmarshal 失敗。
	ErrUnmarshalRTDNData = errors.New("unmarshal RTDN data")
	// ErrVerifyReceipt は verifier が IsValid 判定以前にインフラ的失敗
	// （ネットワーク／署名検証等）を返した場合。
	ErrVerifyReceipt = errors.New("verify receipt")
	// ErrUnsupportedProductType は Purchase で subscription 等の非対応 type が
	// 指定された場合（subscription は Subscribe を使う）。
	ErrUnsupportedProductType = errors.New("unsupported product type for purchase")
)
