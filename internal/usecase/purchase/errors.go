// Package purchase はクライアント駆動の購入ユースケース
// (商品一覧取得・単発購入・サブスクリプション購入) を提供する。
package purchase

import "errors"

var (
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
	// ErrVerifyReceipt は verifier が IsValid 判定以前にインフラ的失敗
	// （ネットワーク／署名検証等）を返した場合。
	ErrVerifyReceipt = errors.New("verify receipt")
	// ErrUnsupportedProductType は Purchase で subscription 等の非対応 type が
	// 指定された場合（subscription は Subscribe を使う）。
	ErrUnsupportedProductType = errors.New("unsupported product type for purchase")
)
