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
)
