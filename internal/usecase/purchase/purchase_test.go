//go:build integration

package purchase_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase/purchasetest"
)

type completedPurchase struct {
	env        purchasetest.Env
	snapshot   *purchasetest.OutboxSnapshot
	startedAt  time.Time
	finishedAt time.Time
}

func purchaseValidProduct(t *testing.T, productID string) completedPurchase {
	t.Helper()
	e := purchasetest.NewPurchaseEnv(t, pg)
	purchasetest.AcceptReceiptFor(productID)(e.Apple)
	snapshot := purchasetest.TakeOutboxSnapshot(t, pg.Pool)
	startedAt := time.Now().Truncate(time.Microsecond)
	err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, productID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
	finishedAt := time.Now()
	require.NoError(t, err)
	return completedPurchase{env: e, snapshot: snapshot, startedAt: startedAt, finishedAt: finishedAt}
}

func purchaseTwiceWithSameToken(t *testing.T, productID string) (completedPurchase, error) {
	t.Helper()
	first := purchaseValidProduct(t, productID)
	secondErr := first.env.Service.Purchase(t.Context(), purchasetest.PlayerID, productID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
	return first, secondErr
}

var failedPurchasesLeavingRecords = []purchasetest.FailedOperation{
	{Name: "販売停止中の商品を購入して失敗したあと", ProductID: purchasetest.InactiveProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.AcceptReceiptFor(purchasetest.InactiveProductID)},
	{Name: "存在しない商品を購入して失敗したあと", ProductID: purchasetest.MissingProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.AcceptReceiptFor(purchasetest.MissingProductID)},
	{Name: "subscription商品を購入して失敗したあと", ProductID: purchasetest.SubscriptionProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)},
	{Name: "ストアがレシートを無効と判定して失敗したあと", ProductID: purchasetest.CardPackProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.RejectReceipt},
	{Name: "ストアに接続できずに購入して失敗したあと", ProductID: purchasetest.CardPackProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.FailReceipt},
}

var unsupportedPlatformPurchase = purchasetest.FailedOperation{
	Name: "未対応のプラットフォームwindowsで購入して失敗したあと", ProductID: purchasetest.CardPackProductID, Platform: purchasetest.PlatformUnsupported, ArrangeReceipt: purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID),
}

var failedPurchasesLeavingOutbox = append(append([]purchasetest.FailedOperation{}, failedPurchasesLeavingRecords...), unsupportedPlatformPurchase)

func TestPurchase(t *testing.T) {
	t.Run("[商品購入]単発購入", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("有効なレシートのfaction_set商品を購入したとき", func(t *testing.T) {
				t.Run("購入記録を、購入に使ったトークンから引いたとき", func(t *testing.T) {
					t.Run("購入したプレイヤーの記録になる", func(t *testing.T) {
						purchaseValidProduct(t, purchasetest.FactionSetProductID)

						records := purchasetest.FindOneTimePurchases(t, pg.Pool, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

						require.Len(t, records, 1)
						assert.Equal(t, purchasetest.PlayerID, records[0].PlayerID)
					})
					t.Run("購入した商品の記録になる", func(t *testing.T) {
						purchaseValidProduct(t, purchasetest.FactionSetProductID)

						records := purchasetest.FindOneTimePurchases(t, pg.Pool, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

						require.Len(t, records, 1)
						assert.Equal(t, purchasetest.FactionSetProductID, records[0].ProductID)
					})
					t.Run("購入日時が、購入を実行した日時になる", func(t *testing.T) {
						result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

						records := purchasetest.FindOneTimePurchases(t, pg.Pool, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

						require.Len(t, records, 1)
						assert.WithinRange(t, records[0].PurchasedAt, result.startedAt, result.finishedAt)
					})
				})
				t.Run("商品一覧で、その商品のis_ownedがtrueになる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					assert.True(t, purchasetest.IsOwned(t, result.env.Service, purchasetest.PlayerID, purchasetest.FactionSetProductID))
				})
				t.Run("所有陣営に、商品の陣営が加わる", func(t *testing.T) {
					purchaseValidProduct(t, purchasetest.FactionSetProductID)

					assert.Contains(t, purchasetest.ListOwnedFactions(t, pg.Pool, purchasetest.PlayerID), purchasetest.FactionSetFaction)
				})
				t.Run("アウトボックスにcard-pack-purchasedが1件記録される", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					assert.Len(t, result.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased), 1)
				})
				t.Run("アウトボックスに記録されたcard-pack-purchasedのplayer_idが購入したプレイヤーになる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased))

					assert.Equal(t, purchasetest.PlayerID, event.Payload["player_id"])
				})
				t.Run("アウトボックスに記録されたcard-pack-purchasedのcard_pack_idが商品に紐づくカードパックになる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased))

					assert.Equal(t, purchasetest.FactionSetCardPackID, event.Payload["card_pack_id"])
				})
				t.Run("アウトボックスに記録されたcard-pack-purchasedの、記録のevent_idがイベントのevent_idと一致する", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased))

					assert.Equal(t, event.EventID, event.Payload["event_id"])
				})
				t.Run("アウトボックスにfaction-acquiredが1件記録される", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					assert.Len(t, result.snapshot.NewEvents(t, purchasetest.EventTypeFactionAcquired), 1)
				})
				t.Run("アウトボックスに記録されたfaction-acquiredのplayer_idが購入したプレイヤーになる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypeFactionAcquired))

					assert.Equal(t, purchasetest.PlayerID, event.Payload["player_id"])
				})
				t.Run("アウトボックスに記録されたfaction-acquiredのfactionが商品の陣営になる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypeFactionAcquired))

					assert.Equal(t, purchasetest.FactionSetFaction, event.Payload["faction"])
				})
				t.Run("アウトボックスに記録されたfaction-acquiredの、記録のevent_idがイベントのevent_idと一致する", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.FactionSetProductID)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypeFactionAcquired))

					assert.Equal(t, event.EventID, event.Payload["event_id"])
				})
			})
			t.Run("有効なレシートのcard_pack商品を購入したとき", func(t *testing.T) {
				t.Run("商品一覧で、その商品のis_ownedがtrueになる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.CardPackProductID)

					assert.True(t, purchasetest.IsOwned(t, result.env.Service, purchasetest.PlayerID, purchasetest.CardPackProductID))
				})
				t.Run("アウトボックスにcard-pack-purchasedが1件記録される", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.CardPackProductID)

					assert.Len(t, result.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased), 1)
				})
				t.Run("アウトボックスに記録されたcard-pack-purchasedのcard_pack_idが商品のカードパックになる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.CardPackProductID)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased))

					assert.Equal(t, purchasetest.CardPackID, event.Payload["card_pack_id"])
				})
				t.Run("アウトボックスのfaction-acquiredが0件になる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.CardPackProductID)

					assert.Empty(t, result.snapshot.NewEvents(t, purchasetest.EventTypeFactionAcquired))
				})
			})
			t.Run("有効なレシートのcosmetic商品を購入したとき", func(t *testing.T) {
				t.Run("商品一覧で、その商品のis_ownedがtrueになる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.CosmeticProductID)

					assert.True(t, purchasetest.IsOwned(t, result.env.Service, purchasetest.PlayerID, purchasetest.CosmeticProductID))
				})
				t.Run("アウトボックスのcard-pack-purchasedが0件になる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.CosmeticProductID)

					assert.Empty(t, result.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased))
				})
				t.Run("アウトボックスのfaction-acquiredが0件になる", func(t *testing.T) {
					result := purchaseValidProduct(t, purchasetest.CosmeticProductID)

					assert.Empty(t, result.snapshot.NewEvents(t, purchasetest.EventTypeFactionAcquired))
				})
			})
			t.Run("同じ購入トークンで同じfaction_set商品を2回続けて購入したとき", func(t *testing.T) {
				t.Run("2回目の購入はエラーにならない", func(t *testing.T) {
					_, secondErr := purchaseTwiceWithSameToken(t, purchasetest.FactionSetProductID)

					assert.NoError(t, secondErr)
				})
				t.Run("購入記録が1件だけになる", func(t *testing.T) {
					_, secondErr := purchaseTwiceWithSameToken(t, purchasetest.FactionSetProductID)
					require.NoError(t, secondErr)

					assert.Equal(t, 1, pg.CountRows(t, "shop.one_time_purchases"))
				})
				t.Run("アウトボックスにcard-pack-purchasedが1件だけ記録される", func(t *testing.T) {
					first, secondErr := purchaseTwiceWithSameToken(t, purchasetest.FactionSetProductID)
					require.NoError(t, secondErr)

					assert.Len(t, first.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased), 1)
				})
				t.Run("アウトボックスにfaction-acquiredが1件だけ記録される", func(t *testing.T) {
					first, secondErr := purchaseTwiceWithSameToken(t, purchasetest.FactionSetProductID)
					require.NoError(t, secondErr)

					assert.Len(t, first.snapshot.NewEvents(t, purchasetest.EventTypeFactionAcquired), 1)
				})
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("使用済みの購入トークンで、別のcard_pack商品を購入したとき", func(t *testing.T) {
				t.Run("購入はエラーにならない", func(t *testing.T) {
					first := purchaseValidProduct(t, purchasetest.CardPackProductID)

					err := first.env.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.OtherCardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.NoError(t, err)
				})
				t.Run("商品一覧で、別のcard_pack商品のis_ownedがfalseのままになる", func(t *testing.T) {
					first := purchaseValidProduct(t, purchasetest.CardPackProductID)
					err := first.env.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.OtherCardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
					require.NoError(t, err)

					assert.False(t, purchasetest.IsOwned(t, first.env.Service, purchasetest.PlayerID, purchasetest.OtherCardPackProductID))
				})
				t.Run("アウトボックスのcard-pack-purchasedが、最初の購入の1件のままになる", func(t *testing.T) {
					first := purchaseValidProduct(t, purchasetest.CardPackProductID)
					err := first.env.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.OtherCardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
					require.NoError(t, err)

					assert.Len(t, first.snapshot.NewEvents(t, purchasetest.EventTypeCardPackPurchased), 1)
				})
			})
			t.Run("購入済みの商品を、別の購入トークンで再び購入したとき", func(t *testing.T) {
				const otherPurchaseToken = "TST-PURCHASE-TOKEN-2"
				tests := []struct {
					name      string
					productID string
				}{
					{"商品がfaction_set商品のとき、購入済みを示すエラーで失敗する", purchasetest.FactionSetProductID},
					{"商品がcard_pack商品のとき、購入済みを示すエラーで失敗する", purchasetest.CardPackProductID},
					{"商品がcosmetic商品のとき、購入済みを示すエラーで失敗する", purchasetest.CosmeticProductID},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						first := purchaseValidProduct(t, tt.productID)
						first.env.Apple.AcceptToken(otherPurchaseToken, tt.productID, purchasetest.StoreExpiresAt)

						err := first.env.Service.Purchase(t.Context(), purchasetest.PlayerID, tt.productID, purchasetest.PlatformIOS, otherPurchaseToken)

						assert.ErrorIs(t, err, purchase.ErrAlreadyOwned)
					})
				}
			})
			t.Run("購入できない商品や入力のとき", func(t *testing.T) {
				t.Run("販売停止中の商品を購入すると、販売停止を示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.AcceptReceiptFor(purchasetest.InactiveProductID)(e.Apple)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.InactiveProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, purchase.ErrProductNotActive)
				})
				t.Run("存在しない商品を購入すると、商品が無いことを示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.AcceptReceiptFor(purchasetest.MissingProductID)(e.Apple)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.MissingProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, port.ErrNotFound)
				})
				t.Run("subscription商品を購入すると、単発購入に対応しない商品種別を示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)(e.Apple)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, purchase.ErrUnsupportedProductType)
				})
				t.Run("subscription商品を購入したときのエラーの内容に、商品種別subscriptionが含まれる", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)(e.Apple)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorContains(t, err, "subscription")
				})
			})
			t.Run("ストアのレシート検証が失敗するとき", func(t *testing.T) {
				t.Run("ストアがレシートを無効と判定すると、レシートが無効であることを示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.RejectReceipt(e.Apple)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, purchase.ErrReceiptVerificationFailed)
				})
				t.Run("ストアに接続できないと、レシートの検証に失敗したことを示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.FailReceipt(e.Apple)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, purchase.ErrVerifyReceipt)
				})
				t.Run("ストアに接続できないときのエラーの内容に、ストアの通信エラーのメッセージが含まれる", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.FailReceipt(e.Apple)

					err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorContains(t, err, purchasetest.StoreUnreachableMessage)
				})
			})
			t.Run("購入の失敗ごとの状態", func(t *testing.T) {
				for _, tt := range failedPurchasesLeavingOutbox {
					t.Run(tt.Name+"、アウトボックスのイベントが購入の前から増えない", func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						tt.ArrangeReceipt(e.Apple)
						snapshot := purchasetest.TakeOutboxSnapshot(t, pg.Pool)

						err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, tt.ProductID, tt.Platform, purchasetest.PurchaseToken)
						require.Error(t, err)

						assert.Empty(t, snapshot.AllNewEvents(t))
					})
				}
				for _, tt := range failedPurchasesLeavingRecords {
					t.Run(tt.Name+"、購入に使ったトークンから購入記録を引くと、記録が無い", func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						tt.ArrangeReceipt(e.Apple)

						err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, tt.ProductID, tt.Platform, purchasetest.PurchaseToken)
						require.Error(t, err)

						assert.Empty(t, purchasetest.FindOneTimePurchases(t, pg.Pool, tt.Platform, purchasetest.PurchaseToken))
					})
				}
			})
			t.Run("購入が失敗したあとの回復", func(t *testing.T) {
				t.Run("ストアがレシートを無効と判定して失敗したあと、同じトークンで有効なレシートに変わって再び購入したとき", func(t *testing.T) {
					t.Run("購入はエラーにならない", func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						purchasetest.RejectReceipt(e.Apple)
						failedErr := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
						require.ErrorIs(t, failedErr, purchase.ErrReceiptVerificationFailed)
						purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)(e.Apple)

						err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

						assert.NoError(t, err)
					})
					t.Run("商品一覧で、その商品のis_ownedがtrueになる", func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						purchasetest.RejectReceipt(e.Apple)
						failedErr := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
						require.ErrorIs(t, failedErr, purchase.ErrReceiptVerificationFailed)
						purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)(e.Apple)
						retryErr := e.Service.Purchase(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
						require.NoError(t, retryErr)

						assert.True(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.CardPackProductID))
					})
				})
			})
		})
	})
}
