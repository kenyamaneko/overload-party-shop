//go:build integration

package purchase_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase/purchasetest"
)

func TestPurchaseReceiptVerifierSelection(t *testing.T) {
	t.Run("[商品購入]プラットフォームごとのレシート検証先", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("platformがiosのとき", func(t *testing.T) {
				t.Run("Appleのストアだけがレシートを有効と判定すると、商品一覧でその商品のis_ownedがtrueになる", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)(e.Apple)
					purchasetest.RejectReceipt(e.Google)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
					require.NoError(t, err)

					assert.True(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.CardPackProductID))
				})
			})
			t.Run("platformがandroidのとき", func(t *testing.T) {
				t.Run("Googleのストアだけがレシートを有効と判定すると、商品一覧でその商品のis_ownedがtrueになる", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.RejectReceipt(e.Apple)
					purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)(e.Google)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformAndroid, purchasetest.PurchaseToken)
					require.NoError(t, err)

					assert.True(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.CardPackProductID))
				})
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("platformがiosのとき", func(t *testing.T) {
				t.Run("Googleのストアだけがレシートを有効と判定すると、レシートが無効であることを示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.RejectReceipt(e.Apple)
					purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)(e.Google)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, purchase.ErrReceiptVerificationFailed)
				})
			})
			t.Run("platformがios・android以外のとき", func(t *testing.T) {
				tests := []struct {
					name     string
					platform string
				}{
					{"windowsを指定すると、未対応のプラットフォームを示すエラーで失敗する", purchasetest.PlatformUnsupported},
					{"空文字を指定すると、未対応のプラットフォームを示すエラーで失敗する", ""},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)(e.Apple)
						purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)(e.Google)

						err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, tt.platform, purchasetest.PurchaseToken)

						assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
					})
				}
			})
		})
	})
}
