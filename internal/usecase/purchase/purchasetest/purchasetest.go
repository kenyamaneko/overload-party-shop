// Package purchasetest は購入ユースケースの結合テストが共有する、サービスの組み立て・外部ストアの検証の偽物・アウトボックスと購入・契約の読み出しを提供する。
package purchasetest

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase"
)

// NewService は実 PostgreSQL のリポジトリに結線した購入サービスを、Apple と Google の検証器を指定して返す。
func NewService(pool *pgxpool.Pool, appleVerifier, googleVerifier port.ReceiptVerifier) *purchase.Service {
	return purchase.New(
		postgres.NewProductRepository(pool),
		postgres.NewFactionPurchaseRepository(pool),
		postgres.NewCardPackPurchaseRepository(pool),
		postgres.NewItemPurchaseRepository(pool),
		postgres.NewPurchaseLookupRepository(pool),
		postgres.NewSubscriptionRepository(pool),
		appleVerifier,
		googleVerifier,
	)
}

// IsOwned はプレイヤーの商品一覧で、指定した商品が所有済みかどうかを返す。
func IsOwned(t *testing.T, service *purchase.Service, playerID, productID string) bool {
	t.Helper()
	products, err := service.GetProducts(t.Context(), playerID)
	require.NoError(t, err)
	ownership := make(map[string]bool, len(products))
	for _, product := range products {
		ownership[product.ProductView.Common().ProductID] = product.IsOwned
	}
	isOwned, isListed := ownership[productID]
	require.True(t, isListed, "商品一覧に商品 %s が無い", productID)
	return isOwned
}
