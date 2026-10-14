//go:build integration

package purchase_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase/purchasetest"
)

type completedSubscription struct {
	env        purchasetest.Env
	snapshot   *purchasetest.OutboxSnapshot
	startedAt  time.Time
	finishedAt time.Time
	expiresAt  time.Time
}

func subscribeWithValidReceipt(t *testing.T, platform string) completedSubscription {
	t.Helper()
	e := purchasetest.NewPurchaseEnv(t, pg)
	purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)(e.Apple)
	purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)(e.Google)
	snapshot := purchasetest.TakeOutboxSnapshot(t, pg.Pool)
	startedAt := time.Now().Truncate(time.Microsecond)
	expiresAt, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, platform, purchasetest.PurchaseToken)
	finishedAt := time.Now()
	require.NoError(t, err)
	return completedSubscription{env: e, snapshot: snapshot, startedAt: startedAt, finishedAt: finishedAt, expiresAt: expiresAt}
}

func singleSubscription(t *testing.T) domain.Subscription {
	t.Helper()
	subscriptions := purchasetest.ListSubscriptions(t, pg.Pool, purchasetest.PlayerID)
	require.Len(t, subscriptions, 1)
	return subscriptions[0]
}

var failedSubscriptions = []purchasetest.FailedOperation{
	{Name: "存在しない商品を登録して失敗したあと", ProductID: purchasetest.MissingProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.AcceptReceiptFor(purchasetest.MissingProductID)},
	{Name: "subscription以外の商品を登録して失敗したあと", ProductID: purchasetest.CardPackProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)},
	{Name: "未対応のプラットフォームwindowsで登録して失敗したあと", ProductID: purchasetest.SubscriptionProductID, Platform: purchasetest.PlatformUnsupported, ArrangeReceipt: purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)},
	{Name: "ストアがレシートを無効と判定して失敗したあと", ProductID: purchasetest.SubscriptionProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.RejectReceipt},
	{Name: "ストアに接続できずに登録して失敗したあと", ProductID: purchasetest.SubscriptionProductID, Platform: purchasetest.PlatformIOS, ArrangeReceipt: purchasetest.FailReceipt},
}

func TestSubscribe(t *testing.T) {
	t.Run("[商品購入]サブスクリプション登録", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("有効なレシートのsubscription商品を登録したとき", func(t *testing.T) {
				t.Run("登録の結果の有効期限がストア検証の有効期限になる", func(t *testing.T) {
					result := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

					assert.WithinDuration(t, purchasetest.StoreExpiresAt, result.expiresAt, 0)
				})
				t.Run("登録のあとにプレイヤーの契約を引いたとき", func(t *testing.T) {
					t.Run("契約状態がactiveになる", func(t *testing.T) {
						subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

						assert.Equal(t, "active", singleSubscription(t).Status)
					})
					t.Run("課金期間の終了日時がストア検証の有効期限になる", func(t *testing.T) {
						subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

						assert.WithinDuration(t, purchasetest.StoreExpiresAt, singleSubscription(t).CurrentPeriodEnd, 0)
					})
					t.Run("課金期間の開始日時が、登録を実行した日時になる", func(t *testing.T) {
						result := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

						assert.WithinRange(t, singleSubscription(t).CurrentPeriodStart, result.startedAt, result.finishedAt)
					})
					t.Run("持ち主が登録したプレイヤーになる", func(t *testing.T) {
						subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

						assert.Equal(t, purchasetest.PlayerID, singleSubscription(t).PlayerID)
					})
					t.Run("商品が登録した商品になる", func(t *testing.T) {
						subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

						assert.Equal(t, purchasetest.SubscriptionProductID, singleSubscription(t).ProductID)
					})
				})
				t.Run("アウトボックスに記録されたpremium_updatedのplayer_idが登録したプレイヤーになる", func(t *testing.T) {
					result := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypePremiumUpdated))

					assert.Equal(t, purchasetest.PlayerID, event.Payload["player_id"])
				})
				t.Run("アウトボックスに記録されたpremium_updatedのis_premiumがtrueになる", func(t *testing.T) {
					result := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypePremiumUpdated))

					assert.Equal(t, true, event.Payload["is_premium"])
				})
				t.Run("アウトボックスに記録されたpremium_updatedのpremium_expires_atがストア検証の有効期限になる", func(t *testing.T) {
					result := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypePremiumUpdated))

					assert.WithinDuration(t, purchasetest.StoreExpiresAt, event.TimeField(t, "premium_expires_at"), 0)
				})
				t.Run("アウトボックスに記録されたpremium_updatedの、記録のevent_idがイベントのevent_idと一致する", func(t *testing.T) {
					result := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

					event := purchasetest.SingleEvent(t, result.snapshot.NewEvents(t, purchasetest.EventTypePremiumUpdated))

					assert.Equal(t, event.EventID, event.Payload["event_id"])
				})
			})
			platformRows := []struct {
				name     string
				platform string
			}{
				{"platformがiosで、有効なレシートのsubscription商品を登録したとき", purchasetest.PlatformIOS},
				{"platformがandroidで、有効なレシートのsubscription商品を登録したとき", purchasetest.PlatformAndroid},
			}
			for _, tt := range platformRows {
				t.Run(tt.name+"、登録に使ったトークンで契約を引くと、登録したプレイヤーの契約が1件返る", func(t *testing.T) {
					subscribeWithValidReceipt(t, tt.platform)

					var owners []string
					for _, subscription := range purchasetest.FindSubscriptionsByToken(t, pg.Pool, tt.platform, purchasetest.PurchaseToken) {
						owners = append(owners, subscription.PlayerID)
					}

					assert.Equal(t, []string{purchasetest.PlayerID}, owners)
				})
			}
			for _, tt := range platformRows {
				t.Run(tt.name+"、アウトボックスにpremium_updatedが1件記録される", func(t *testing.T) {
					result := subscribeWithValidReceipt(t, tt.platform)

					assert.Len(t, result.snapshot.NewEvents(t, purchasetest.EventTypePremiumUpdated), 1)
				})
			}
			t.Run("同じトークンで同じsubscription商品を2回続けて登録したとき", func(t *testing.T) {
				t.Run("2回目の登録の結果の有効期限が、1回目の登録の結果の有効期限と同じになる", func(t *testing.T) {
					first := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)

					secondExpiresAt, err := first.env.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
					require.NoError(t, err)

					assert.WithinDuration(t, first.expiresAt, secondExpiresAt, 0)
				})
				t.Run("プレイヤーの契約が1件だけになる", func(t *testing.T) {
					first := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)
					_, err := first.env.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
					require.NoError(t, err)

					assert.Len(t, purchasetest.ListSubscriptions(t, pg.Pool, purchasetest.PlayerID), 1)
				})
				t.Run("アウトボックスにpremium_updatedが1件だけ記録される", func(t *testing.T) {
					first := subscribeWithValidReceipt(t, purchasetest.PlatformIOS)
					_, err := first.env.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
					require.NoError(t, err)

					assert.Len(t, first.snapshot.NewEvents(t, purchasetest.EventTypePremiumUpdated), 1)
				})
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("登録できない商品や入力のとき", func(t *testing.T) {
				t.Run("存在しない商品を登録すると、商品が無いことを示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.AcceptReceiptFor(purchasetest.MissingProductID)(e.Apple)

					_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.MissingProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, port.ErrNotFound)
				})
				t.Run("subscription以外の商品を登録すると、サブスクリプション以外の商品を示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.AcceptReceiptFor(purchasetest.CardPackProductID)(e.Apple)

					_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.CardPackProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, purchase.ErrProductNotSubscription)
				})
				t.Run("未対応のプラットフォームwindowsで登録すると、未対応のプラットフォームを示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)(e.Apple)
					purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)(e.Google)

					_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformUnsupported, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
				})
			})
			t.Run("ストアのレシート検証が失敗するとき", func(t *testing.T) {
				t.Run("ストアがレシートを無効と判定すると、購読のレシートが無効であることを示すエラーで失敗する", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.RejectReceipt(e.Apple)

					_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorIs(t, err, purchase.ErrSubVerificationFailed)
				})
				t.Run("ストアに接続できないと、エラーの内容にストアの通信エラーのメッセージが含まれる", func(t *testing.T) {
					e := purchasetest.NewPurchaseEnv(t, pg)
					purchasetest.FailReceipt(e.Apple)

					_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

					assert.ErrorContains(t, err, purchasetest.StoreUnreachableMessage)
				})
			})
			t.Run("登録の失敗ごとの状態", func(t *testing.T) {
				for _, tt := range failedSubscriptions {
					t.Run(tt.Name+"、プレイヤーの契約が0件のままになる", func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						tt.ArrangeReceipt(e.Apple)

						_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, tt.ProductID, tt.Platform, purchasetest.PurchaseToken)
						require.Error(t, err)

						assert.Empty(t, purchasetest.ListSubscriptions(t, pg.Pool, purchasetest.PlayerID))
					})
				}
				for _, tt := range failedSubscriptions {
					t.Run(tt.Name+"、アウトボックスのpremium_updatedが登録の前から増えない", func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						tt.ArrangeReceipt(e.Apple)
						snapshot := purchasetest.TakeOutboxSnapshot(t, pg.Pool)

						_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, tt.ProductID, tt.Platform, purchasetest.PurchaseToken)
						require.Error(t, err)

						assert.Empty(t, snapshot.NewEvents(t, purchasetest.EventTypePremiumUpdated))
					})
				}
			})
			t.Run("登録が失敗したあとの回復", func(t *testing.T) {
				t.Run("ストアがレシートを無効と判定して失敗したあと、同じトークンで有効なレシートに変わって再び登録したとき", func(t *testing.T) {
					t.Run("登録はエラーにならない", func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						purchasetest.RejectReceipt(e.Apple)
						_, failedErr := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
						require.ErrorIs(t, failedErr, purchase.ErrSubVerificationFailed)
						purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)(e.Apple)

						_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)

						assert.NoError(t, err)
					})
					t.Run("プレイヤーの契約を引くと、契約状態がactiveになる", func(t *testing.T) {
						e := purchasetest.NewPurchaseEnv(t, pg)
						purchasetest.RejectReceipt(e.Apple)
						_, failedErr := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
						require.ErrorIs(t, failedErr, purchase.ErrSubVerificationFailed)
						purchasetest.AcceptReceiptFor(purchasetest.SubscriptionProductID)(e.Apple)
						_, retryErr := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
						require.NoError(t, retryErr)

						assert.Equal(t, "active", singleSubscription(t).Status)
					})
				})
			})
		})
	})
}
