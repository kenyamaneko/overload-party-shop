//go:build integration

package purchase_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase/purchasetest"
)

const (
	secondFactionSetProductID = "TST-FACTION-SET-2"
	unknownSubscriptionStatus = "TST-UNKNOWN"
)

var (
	pastExpiresAt       = time.Date(2020, 1, 15, 8, 0, 0, 0, time.UTC)
	unknownStatusPeriod = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
)

type cosmeticItem struct {
	itemType string
	itemNo   int64
}

func purchaseWithValidReceipt(t *testing.T, e purchasetest.Env, productID, token string) {
	t.Helper()
	e.Apple.AcceptToken(token, productID, purchasetest.StoreExpiresAt)
	err := e.Service.Purchase(t.Context(), purchasetest.PlayerID, productID, purchasetest.PlatformIOS, token)
	require.NoError(t, err)
}

func newCosmeticEnv(t *testing.T, ownedItems []cosmeticItem) purchasetest.Env {
	t.Helper()
	e := purchasetest.NewEnv(t, pg)
	pg.InsertCosmeticProduct(t, purchasetest.ActiveSeed(purchasetest.CosmeticProductID), "playmat", 1)
	for i, item := range ownedItems {
		productID := fmt.Sprintf("TST-COSMETIC-OTHER-%d", i+1)
		pg.InsertCosmeticProduct(t, purchasetest.ActiveSeed(productID), item.itemType, item.itemNo)
		purchaseWithValidReceipt(t, e, productID, fmt.Sprintf("TST-PURCHASE-TOKEN-OTHER-%d", i+1))
	}
	return e
}

func newSubscriptionEnv(t *testing.T) purchasetest.Env {
	t.Helper()
	e := purchasetest.NewEnv(t, pg)
	pg.InsertSubscriptionProduct(t, purchasetest.ActiveSeed(purchasetest.SubscriptionProductID), 1)
	return e
}

func subscribeWithExpiry(t *testing.T, e purchasetest.Env, expiresAt time.Time) {
	t.Helper()
	e.Apple.AcceptToken(purchasetest.PurchaseToken, purchasetest.SubscriptionProductID, expiresAt)
	_, err := e.Service.Subscribe(t.Context(), purchasetest.PlayerID, purchasetest.SubscriptionProductID, purchasetest.PlatformIOS, purchasetest.PurchaseToken)
	require.NoError(t, err)
}

func TestGetProductsOwnership(t *testing.T) {
	t.Run("[商品一覧]所有状態の判定", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("faction_set商品の所有状態", func(t *testing.T) {
				t.Run("商品のカードパックを購入済みのプレイヤーでは、そのfaction_set商品のis_ownedがtrueになる", func(t *testing.T) {
					e := purchasetest.NewEnv(t, pg)
					pg.InsertFactionSetProduct(t, purchasetest.ActiveSeed(purchasetest.FactionSetProductID), purchasetest.FactionSetFaction, purchasetest.FactionSetCardPackID)
					pg.InsertCardPackProduct(t, purchasetest.ActiveSeed(purchasetest.CardPackProductID), purchasetest.FactionSetCardPackID)
					purchaseWithValidReceipt(t, e, purchasetest.CardPackProductID, purchasetest.PurchaseToken)

					assert.True(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.FactionSetProductID))
				})
				t.Run("ある陣営のfaction_set商品を購入済みのプレイヤーでは、別の陣営のfaction_set商品のis_ownedがfalseになる", func(t *testing.T) {
					e := purchasetest.NewEnv(t, pg)
					pg.InsertFactionSetProduct(t, purchasetest.ActiveSeed(purchasetest.FactionSetProductID), purchasetest.FactionSetFaction, purchasetest.FactionSetCardPackID)
					pg.InsertFactionSetProduct(t, purchasetest.ActiveSeed(secondFactionSetProductID), "Tenki", purchasetest.OtherCardPackID)
					purchaseWithValidReceipt(t, e, purchasetest.FactionSetProductID, purchasetest.PurchaseToken)

					assert.False(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, secondFactionSetProductID))
				})
			})
			t.Run("card_pack商品の所有状態", func(t *testing.T) {
				t.Run("商品のカードパックを購入済みのプレイヤーでは、そのcard_pack商品のis_ownedがtrueになる", func(t *testing.T) {
					e := purchasetest.NewEnv(t, pg)
					pg.InsertCardPackProduct(t, purchasetest.ActiveSeed(purchasetest.CardPackProductID), purchasetest.CardPackID)
					purchaseWithValidReceipt(t, e, purchasetest.CardPackProductID, purchasetest.PurchaseToken)

					assert.True(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.CardPackProductID))
				})
				t.Run("商品のカードパックを購入していないプレイヤーでは、そのcard_pack商品のis_ownedがfalseになる", func(t *testing.T) {
					e := purchasetest.NewEnv(t, pg)
					pg.InsertCardPackProduct(t, purchasetest.ActiveSeed(purchasetest.CardPackProductID), purchasetest.CardPackID)

					assert.False(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.CardPackProductID))
				})
			})
			t.Run("cosmetic商品(商品のアイテムは種別playmat・番号1)の所有状態", func(t *testing.T) {
				t.Run("種別playmat・番号1のアイテムを所有しているとき、そのcosmetic商品のis_ownedがtrueになる", func(t *testing.T) {
					e := newCosmeticEnv(t, nil)
					purchaseWithValidReceipt(t, e, purchasetest.CosmeticProductID, purchasetest.PurchaseToken)

					assert.True(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.CosmeticProductID))
				})
				tests := []struct {
					name       string
					ownedItems []cosmeticItem
				}{
					{"所有しているアイテムが無いとき、そのcosmetic商品のis_ownedがfalseになる", nil},
					{"種別playmat・番号99のアイテムだけを所有しているとき、そのcosmetic商品のis_ownedがfalseになる", []cosmeticItem{{"playmat", 99}}},
					{"種別sleeve・番号1のアイテムだけを所有しているとき、そのcosmetic商品のis_ownedがfalseになる", []cosmeticItem{{"sleeve", 1}}},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						e := newCosmeticEnv(t, tt.ownedItems)

						assert.False(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.CosmeticProductID))
					})
				}
			})
			t.Run("subscription商品の所有状態", func(t *testing.T) {
				t.Run("特典が有効な契約があるプレイヤーでは、subscription商品のis_ownedがtrueになる", func(t *testing.T) {
					e := newSubscriptionEnv(t)
					subscribeWithExpiry(t, e, purchasetest.StoreExpiresAt)

					assert.True(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.SubscriptionProductID))
				})
				tests := []struct {
					name    string
					arrange func(*testing.T, purchasetest.Env)
				}{
					{"サブスクリプションが無いプレイヤーでは、subscription商品のis_ownedがfalseになる", func(*testing.T, purchasetest.Env) {}},
					{"特典が無効な契約(期限が過去の契約)があるプレイヤーでは、subscription商品のis_ownedがfalseになる", func(t *testing.T, e purchasetest.Env) {
						subscribeWithExpiry(t, e, pastExpiresAt)
					}},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						e := newSubscriptionEnv(t)
						tt.arrange(t, e)

						assert.False(t, purchasetest.IsOwned(t, e.Service, purchasetest.PlayerID, purchasetest.SubscriptionProductID))
					})
				}
			})
		})
		t.Run("異常系", func(t *testing.T) {
			t.Run("契約状態が定義された値以外の契約があるプレイヤーの商品一覧取得は、未知の契約状態を示すエラーで失敗する", func(t *testing.T) {
				e := newSubscriptionEnv(t)
				_, insertErr := pg.Pool.Exec(t.Context(), `
					INSERT INTO shop.subscriptions (player_id, product_id, status, current_period_start, current_period_end)
					VALUES ($1, $2, $3, $4, $5)
				`, purchasetest.PlayerID, purchasetest.SubscriptionProductID, unknownSubscriptionStatus, unknownStatusPeriod, purchasetest.StoreExpiresAt)
				require.NoError(t, insertErr)

				_, err := e.Service.GetProducts(t.Context(), purchasetest.PlayerID)

				assert.ErrorContains(t, err, unknownSubscriptionStatus)
			})
		})
	})
}
