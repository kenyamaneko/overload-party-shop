//go:build integration

package rest_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
)

func TestSubscriptionPurchase(t *testing.T) {
	t.Run("[サブスクリプション購入]サブスクリプションの購入", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("サブスクリプション検証が有効と判定するとき、200になる", func(t *testing.T) {
				env := routertest.BuildEnv(t, pg)
				env.SeedSubscriptionProduct(t)

				response := env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

				assert.Equal(t, http.StatusOK, response.Status)
			})

			t.Run("サブスクリプション検証が有効と判定するとき、messageがサブスクリプションの受付を示す値になる", func(t *testing.T) {
				env := routertest.BuildEnv(t, pg)
				env.SeedSubscriptionProduct(t)

				response := env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

				assert.Equal(t, "subscription accepted", response.SubscribeBody(t).Message)
			})

			t.Run("サブスクリプション検証が有効期限2026-05-15T00:00:00Zを返すとき、応答のexpires_atが2026-05-15T00:00:00Zになる", func(t *testing.T) {
				env := routertest.BuildEnv(t, pg)
				env.SeedSubscriptionProduct(t)
				env.AppleReceipt.AcceptSubscription(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC))

				response := env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

				assert.WithinDuration(t, time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC), response.SubscribeBody(t).ExpiresAt, 0)
			})

			t.Run("サブスクリプション検証が未来の有効期限2999-12-31T00:00:00Zを返すとき、購入した後の同じプレイヤーの商品一覧でTST-PRODUCT-SUB-1のis_ownedがtrueになる", func(t *testing.T) {
				env := routertest.BuildEnv(t, pg)
				env.SeedSubscriptionProduct(t)
				env.AppleReceipt.AcceptSubscription(time.Date(2999, 12, 31, 0, 0, 0, 0, time.UTC))
				env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

				response := env.ListProducts(routertest.PlayerOne)

				assert.True(t, response.Product(t, "TST-PRODUCT-SUB-1").IsOwned)
			})

			t.Run("同じplatformと同じpurchase_tokenで2回続けて購入するとき", func(t *testing.T) {
				t.Run("2回目も200になる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedSubscriptionProduct(t)
					env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

					response := env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

					assert.Equal(t, http.StatusOK, response.Status)
				})

				t.Run("2回目の応答のexpires_atは、1回目の応答のexpires_atと同じになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedSubscriptionProduct(t)
					first := env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

					second := env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

					assert.WithinDuration(t, first.SubscribeBody(t).ExpiresAt, second.SubscribeBody(t).ExpiresAt, 0)
				})

				t.Run("購読記録は1件だけになる", func(t *testing.T) {
					env := routertest.BuildEnv(t, pg)
					env.SeedSubscriptionProduct(t)
					env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")
					env.Subscribe(t, routertest.PlayerOne, "TST-PRODUCT-SUB-1", "ios", "TST-PURCHASE-TOKEN-1")

					assert.Equal(t, 1, env.CountRows(t, "shop.subscriptions"))
				})
			})
		})
	})
}
