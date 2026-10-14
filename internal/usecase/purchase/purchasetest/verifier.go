package purchasetest

import (
	"context"
	"fmt"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/port/porttest"
)

type receipt struct {
	isValid   bool
	productID string
	expiresAt time.Time
	err       error
}

// StoreVerifier は購入トークンごとに検証結果を設定できる、外部ストアのレシート検証の偽物。
type StoreVerifier struct {
	*porttest.MockReceiptVerifier
	receipts map[string]receipt
}

// NewStoreVerifier は、どの購入トークンの結果も設定していない StoreVerifier を返す。
func NewStoreVerifier() *StoreVerifier {
	verifier := &StoreVerifier{
		MockReceiptVerifier: &porttest.MockReceiptVerifier{},
		receipts:            map[string]receipt{},
	}
	verifier.VerifyPurchaseFn = func(_ context.Context, _, purchaseToken string) (*port.VerifyResult, error) {
		found, err := verifier.lookup(purchaseToken)
		if err != nil {
			return nil, err
		}
		return &port.VerifyResult{IsValid: found.isValid, ProductID: found.productID}, nil
	}
	verifier.VerifySubscriptionFn = func(_ context.Context, purchaseToken string) (*port.SubscriptionInfo, error) {
		found, err := verifier.lookup(purchaseToken)
		if err != nil {
			return nil, err
		}
		return &port.SubscriptionInfo{IsValid: found.isValid, ProductID: found.productID, ExpiresAt: found.expiresAt}, nil
	}
	return verifier
}

func (v *StoreVerifier) lookup(purchaseToken string) (receipt, error) {
	found, isRegistered := v.receipts[purchaseToken]
	if !isRegistered {
		return receipt{}, fmt.Errorf("購入トークン %s の検証結果が設定されていない", purchaseToken)
	}
	return found, found.err
}

// AcceptToken は購入トークンの検証が、商品 productID の有効なレシートで、有効期限が expiresAt であると判定するよう設定する。
func (v *StoreVerifier) AcceptToken(purchaseToken, productID string, expiresAt time.Time) {
	v.receipts[purchaseToken] = receipt{isValid: true, productID: productID, expiresAt: expiresAt}
}

// RejectToken は購入トークンの検証が、無効なレシートと判定するよう設定する。
func (v *StoreVerifier) RejectToken(purchaseToken string) {
	v.receipts[purchaseToken] = receipt{isValid: false}
}

// FailToken は購入トークンの検証が、通信エラー err を返すよう設定する。
func (v *StoreVerifier) FailToken(purchaseToken string, err error) {
	v.receipts[purchaseToken] = receipt{err: err}
}
