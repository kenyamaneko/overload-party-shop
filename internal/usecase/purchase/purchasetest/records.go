package purchasetest

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// FindOneTimePurchases は platform の購入トークン purchaseToken で記録された単発購入の記録を返す。
func FindOneTimePurchases(t *testing.T, pool *pgxpool.Pool, platform, purchaseToken string) []domain.OneTimePurchase {
	t.Helper()
	tokenTables := map[string]string{
		"ios":     "shop.apple_purchase_tokens",
		"android": "shop.google_purchase_tokens",
	}
	tokenTable, isSupported := tokenTables[platform]
	require.True(t, isSupported, "未対応の platform %s", platform)
	rows, err := pool.Query(t.Context(), `
		SELECT p.purchase_id, p.player_id::text, p.product_id, p.purchased_at
		FROM shop.one_time_purchases p
		JOIN `+tokenTable+` k ON k.purchase_id = p.purchase_id
		WHERE k.token = $1
	`, purchaseToken)
	require.NoError(t, err)
	return collectRows(t, rows, scanOneTimePurchase)
}

// FindSubscriptionsByToken は platform の購入トークン purchaseToken で引ける契約を返す。
func FindSubscriptionsByToken(t *testing.T, pool *pgxpool.Pool, platform, purchaseToken string) []domain.Subscription {
	t.Helper()
	tokenTables := map[string]string{
		"ios":     "shop.apple_subscription_tokens",
		"android": "shop.google_subscription_tokens",
	}
	tokenTable, isSupported := tokenTables[platform]
	require.True(t, isSupported, "未対応の platform %s", platform)
	rows, err := pool.Query(t.Context(), subscriptionSelect+`
		FROM shop.subscriptions s
		JOIN `+tokenTable+` k ON k.subscription_id = s.subscription_id
		WHERE k.token = $1
	`, purchaseToken)
	require.NoError(t, err)
	return collectRows(t, rows, scanSubscription)
}

// ListSubscriptions はプレイヤーの契約を、作成された順に返す。
func ListSubscriptions(t *testing.T, pool *pgxpool.Pool, playerID string) []domain.Subscription {
	t.Helper()
	rows, err := pool.Query(t.Context(), subscriptionSelect+`
		FROM shop.subscriptions s
		WHERE s.player_id = $1
		ORDER BY s.subscription_id
	`, playerID)
	require.NoError(t, err)
	return collectRows(t, rows, scanSubscription)
}

// ListOwnedFactions はプレイヤーが所有する陣営を、陣営名の昇順で返す。
func ListOwnedFactions(t *testing.T, pool *pgxpool.Pool, playerID string) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT faction FROM shop.player_owned_factions WHERE player_id = $1 ORDER BY faction
	`, playerID)
	require.NoError(t, err)
	return collectRows(t, rows, func(row pgx.Row) (string, error) {
		var faction string
		err := row.Scan(&faction)
		return faction, err
	})
}

const subscriptionSelect = `
	SELECT s.subscription_id, s.player_id::text, s.product_id, s.status,
		s.current_period_start, s.current_period_end, s.created_at, s.updated_at
`

func scanOneTimePurchase(row pgx.Row) (domain.OneTimePurchase, error) {
	var purchase domain.OneTimePurchase
	err := row.Scan(&purchase.PurchaseID, &purchase.PlayerID, &purchase.ProductID, &purchase.PurchasedAt)
	return purchase, err
}

func scanSubscription(row pgx.Row) (domain.Subscription, error) {
	var subscription domain.Subscription
	err := row.Scan(
		&subscription.SubscriptionID, &subscription.PlayerID, &subscription.ProductID, &subscription.Status,
		&subscription.CurrentPeriodStart, &subscription.CurrentPeriodEnd, &subscription.CreatedAt, &subscription.UpdatedAt,
	)
	return subscription, err
}

func collectRows[T any](t *testing.T, rows pgx.Rows, scan func(pgx.Row) (T, error)) []T {
	t.Helper()
	defer rows.Close()
	var collected []T
	for rows.Next() {
		item, err := scan(rows)
		require.NoError(t, err)
		collected = append(collected, item)
	}
	require.NoError(t, rows.Err())
	return collected
}
