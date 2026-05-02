// Package subscription はストア webhook 駆動のサブスクリプションライフサイクル
// 処理と、エンタイトルメント判定 (IsEntitled) を提供する。
package subscription

import "errors"

var (
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
)
