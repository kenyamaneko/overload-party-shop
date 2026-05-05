package purchase

import "errors"

var (
	// ErrFactionAlreadySelected は faction が既に選択済みの場合に返される。
	ErrFactionAlreadySelected = errors.New("faction already selected")
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
	// ErrVerifyReceipt は verifier がインフラ的失敗 (ネットワーク／署名検証等) を返した場合。
	ErrVerifyReceipt = errors.New("verify receipt")
	// ErrUnsupportedProductType は Purchase で subscription 等の非対応 type が指定された場合。
	ErrUnsupportedProductType = errors.New("unsupported product type for purchase")
)
