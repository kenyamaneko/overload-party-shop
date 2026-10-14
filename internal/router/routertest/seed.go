package routertest

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

const (
	appleTransactionID    = "TST-APPLE-TXN-1"
	googlePurchaseToken   = "TST-GOOGLE-TOKEN-1"
	subscriptionProductID = "TST-PRODUCT-SUB-1"
)

func buildProductSeed(productID string, isActive bool) postgrestest.ProductSeed {
	return postgrestest.ProductSeed{ProductID: productID, Name: "TST-NAME-" + productID, Price: 100, IsActive: isActive}
}

// BuildItemProductSeed は販売中のコスメティック商品 TST-PRODUCT-ITEM-1 の登録内容を返す。
func BuildItemProductSeed() postgrestest.ProductSeed {
	return buildProductSeed("TST-PRODUCT-ITEM-1", true)
}

// SeedItemProductFrom は seed の内容でコスメティック商品を、item_type stamp、item_no 1 で登録する。
func (e *Env) SeedItemProductFrom(t *testing.T, seed postgrestest.ProductSeed) {
	t.Helper()
	e.pg.InsertCosmeticProduct(t, seed, "stamp", 1)
}

// SeedItemProduct は販売中のコスメティック商品 TST-PRODUCT-ITEM-1 を登録する。
func (e *Env) SeedItemProduct(t *testing.T) {
	t.Helper()
	e.SeedItemProductFrom(t, BuildItemProductSeed())
}

// SeedInactiveProduct は販売停止中のコスメティック商品 TST-PRODUCT-INACTIVE を登録する。
func (e *Env) SeedInactiveProduct(t *testing.T) {
	t.Helper()
	e.pg.InsertCosmeticProduct(t, buildProductSeed("TST-PRODUCT-INACTIVE", false), "stamp", 2)
}

// SeedSubscriptionProduct は販売中のサブスクリプション商品 TST-PRODUCT-SUB-1 を登録する。
func (e *Env) SeedSubscriptionProduct(t *testing.T) {
	t.Helper()
	e.pg.InsertSubscriptionProduct(t, buildProductSeed(subscriptionProductID, true), 1)
}

// SeedCardPackProduct はカードパック TST-CARD-PACK-1 を配る販売中の商品 TST-PRODUCT-CARD-PACK-1 を登録する。
func (e *Env) SeedCardPackProduct(t *testing.T) {
	t.Helper()
	e.pg.InsertCardPackProduct(t, buildProductSeed("TST-PRODUCT-CARD-PACK-1", true), "TST-CARD-PACK-1")
}

// SeedFactionSetProduct は販売中の陣営セット商品 TST-PRODUCT-FACTION-SET-1 を登録する。
func (e *Env) SeedFactionSetProduct(t *testing.T) {
	t.Helper()
	e.pg.InsertFactionSetProduct(t, buildProductSeed("TST-PRODUCT-FACTION-SET-1", true), "SHE", "TST-CARD-PACK-2")
}

// CreateExpiredAppleSubscription は購入トークン TST-APPLE-TXN-1 の、有効期限が過去のサブスクリプションをプレイヤー PlayerOne に作る。
func (e *Env) CreateExpiredAppleSubscription(t *testing.T) {
	t.Helper()
	e.SeedSubscriptionProduct(t)
	e.AppleReceipt.AcceptSubscription(ExpiredAt)
	response := e.Subscribe(t, PlayerOne, subscriptionProductID, "ios", appleTransactionID)
	require.Equal(t, http.StatusOK, response.Status, "応答本文: %s", response.Body)
}

// RegisterAppleRenewal は署名付きデータ TST-SIGNED-PAYLOAD を、有効期限 expiresAt の更新通知として検証に成功させる。
func (e *Env) RegisterAppleRenewal(t *testing.T, expiresAt time.Time) {
	t.Helper()
	e.AppleJWS.RegisterRenewalNotification(t, "TST-SIGNED-PAYLOAD", "TST-SIGNED-TRANSACTION-INFO")
	e.AppleJWS.RegisterTransaction(t, "TST-SIGNED-TRANSACTION-INFO", appleTransactionID, expiresAt)
}

// CreateExpiredGoogleSubscription は購入トークン TST-GOOGLE-TOKEN-1 の、有効期限が過去のサブスクリプションをプレイヤー PlayerOne に作る。
func (e *Env) CreateExpiredGoogleSubscription(t *testing.T) {
	t.Helper()
	e.SeedSubscriptionProduct(t)
	e.GoogleReceipt.AcceptSubscription(ExpiredAt)
	response := e.Subscribe(t, PlayerOne, subscriptionProductID, "android", googlePurchaseToken)
	require.Equal(t, http.StatusOK, response.Status, "応答本文: %s", response.Body)
}
