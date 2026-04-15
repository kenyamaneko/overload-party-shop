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

// エラー分類 — ドメインで「何が起きたか」を表現する contract。
// transport 層 (handler) はこれを経由してステータスコードやリトライ挙動を決める。
// service に新しい sentinel を追加したら、該当する分類関数も更新すること。

// IsNotFound は対象リソースが見つからない類のエラーか判定する。
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsConflict は既存リソースとの衝突 (重複登録等) によるエラーか判定する。
func IsConflict(err error) bool {
	return errors.Is(err, ErrAlreadyOwned) ||
		errors.Is(err, ErrFactionAlreadySelected)
}

// IsValidation はクライアント入力 / 商品状態の妥当性違反によるエラーか判定する。
func IsValidation(err error) bool {
	return errors.Is(err, ErrInvalidFaction) ||
		errors.Is(err, ErrProductNotActive) ||
		errors.Is(err, ErrProductNotSubscription) ||
		errors.Is(err, ErrUnsupportedProductType) ||
		errors.Is(err, ErrUnsupportedPlatform)
}

// IsPaymentFailed はレシート/サブスクリプション検証がストア側で無効判定された
// エラーか判定する。verifier 自体の infra 障害 (ErrVerifyReceipt) は含まない。
func IsPaymentFailed(err error) bool {
	return errors.Is(err, ErrReceiptVerificationFailed) ||
		errors.Is(err, ErrSubVerificationFailed)
}

// IsDeterministic は webhook 通知処理において「再送しても結果が変わらない」
// 確定的エラーか判定する。webhook handler はこれに該当するエラーを ack (2xx) し、
// ストア側の無駄なリトライを防ぐ。該当しないエラー (DB 障害・pub/sub 失敗等) は
// 一時的な問題として 5xx で返しリトライを促す。
func IsDeterministic(err error) bool {
	return errors.Is(err, ErrDecodeNotification) ||
		errors.Is(err, ErrDecodeTransactionInfo) ||
		errors.Is(err, ErrDecodeRTDNData) ||
		errors.Is(err, ErrUnmarshalRTDNData) ||
		errors.Is(err, ErrSubscriptionNotFound)
}
