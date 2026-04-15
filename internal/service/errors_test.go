package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// エラー分類関数は transport 層が「HTTP ステータスをどう返すか / webhook を
// ack するか」を決める基礎になるので、全 sentinel を 5 つの分類に確実に
// 振り分けられていることを固定する。新しい sentinel を追加したとき、
// どの分類にも属さないと default の「一時的エラー扱い (500 / retry)」に倒れる
// 仕組みなので、分類漏れは安全側 (リトライされる方) でしか事故らない。
func TestErrorClassification(t *testing.T) {
	type classifiers struct {
		notFound      bool
		conflict      bool
		validation    bool
		paymentFailed bool
		deterministic bool
	}

	tests := []struct {
		name string
		err  error
		want classifiers
	}{
		{
			name: "ErrNotFound は NotFound",
			err:  ErrNotFound,
			want: classifiers{notFound: true},
		},
		{
			name: "ErrAlreadyOwned は Conflict",
			err:  ErrAlreadyOwned,
			want: classifiers{conflict: true},
		},
		{
			name: "ErrFactionAlreadySelected は Conflict",
			err:  ErrFactionAlreadySelected,
			want: classifiers{conflict: true},
		},
		{
			name: "ErrInvalidFaction は Validation",
			err:  ErrInvalidFaction,
			want: classifiers{validation: true},
		},
		{
			name: "ErrProductNotActive は Validation",
			err:  ErrProductNotActive,
			want: classifiers{validation: true},
		},
		{
			name: "ErrProductNotSubscription は Validation",
			err:  ErrProductNotSubscription,
			want: classifiers{validation: true},
		},
		{
			name: "ErrUnsupportedProductType は Validation",
			err:  ErrUnsupportedProductType,
			want: classifiers{validation: true},
		},
		{
			name: "ErrUnsupportedPlatform は Validation",
			err:  ErrUnsupportedPlatform,
			want: classifiers{validation: true},
		},
		{
			name: "ErrReceiptVerificationFailed は PaymentFailed",
			err:  ErrReceiptVerificationFailed,
			want: classifiers{paymentFailed: true},
		},
		{
			name: "ErrSubVerificationFailed は PaymentFailed",
			err:  ErrSubVerificationFailed,
			want: classifiers{paymentFailed: true},
		},
		{
			name: "ErrVerifyReceipt はインフラ障害扱いで分類外 (= 500 / retry)",
			err:  ErrVerifyReceipt,
			want: classifiers{},
		},
		{
			name: "ErrDecodeNotification は Deterministic",
			err:  ErrDecodeNotification,
			want: classifiers{deterministic: true},
		},
		{
			name: "ErrDecodeTransactionInfo は Deterministic",
			err:  ErrDecodeTransactionInfo,
			want: classifiers{deterministic: true},
		},
		{
			name: "ErrDecodeRTDNData は Deterministic",
			err:  ErrDecodeRTDNData,
			want: classifiers{deterministic: true},
		},
		{
			name: "ErrUnmarshalRTDNData は Deterministic",
			err:  ErrUnmarshalRTDNData,
			want: classifiers{deterministic: true},
		},
		{
			name: "ErrSubscriptionNotFound は Deterministic",
			err:  ErrSubscriptionNotFound,
			want: classifiers{deterministic: true},
		},
		{
			name: "未知のエラーはいずれの分類にも属さない",
			err:  errors.New("random infra failure"),
			want: classifiers{},
		},
		{
			name: "nil はいずれの分類にも属さない",
			err:  nil,
			want: classifiers{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want.notFound, IsNotFound(tt.err), "IsNotFound")
			assert.Equal(t, tt.want.conflict, IsConflict(tt.err), "IsConflict")
			assert.Equal(t, tt.want.validation, IsValidation(tt.err), "IsValidation")
			assert.Equal(t, tt.want.paymentFailed, IsPaymentFailed(tt.err), "IsPaymentFailed")
			assert.Equal(t, tt.want.deterministic, IsDeterministic(tt.err), "IsDeterministic")
		})
	}
}

// fmt.Errorf("%w: ...") で wrap しても分類関数が元の sentinel を認識することを
// 固定する。service 実装側は頻繁に wrap するため、errors.Is 経由の分類が
// 機能しないとリアルな呼び出しでマッチしなくなる。
func TestErrorClassification_WrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("purchase: %w", ErrAlreadyOwned)
	assert.True(t, IsConflict(wrapped))

	nested := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrDecodeNotification))
	assert.True(t, IsDeterministic(nested))
}
