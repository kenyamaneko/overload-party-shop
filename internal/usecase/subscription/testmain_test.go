//go:build integration

package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
	"github.com/stretchr/testify/require"
)

var sharedPg *postgrestest.Postgres

func TestMain(m *testing.M) {
	os.Exit(postgrestest.RunMain(m, &sharedPg,
		postgrestest.WithSchemaFile("db/schema.sql"),
		postgrestest.WithSchema("shop"),
	))
}

func selectPremiumUpdatedEvents(t *testing.T) []apishop.PremiumUpdatedEvent {
	t.Helper()
	rows, err := sharedPg.Pool.Query(context.Background(),
		`SELECT payload FROM shop.outbox_events WHERE event_type = $1 ORDER BY created_at`,
		apishop.EventTypePremiumUpdated)
	require.NoError(t, err)
	defer rows.Close()
	var events []apishop.PremiumUpdatedEvent
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var ev apishop.PremiumUpdatedEvent
		require.NoError(t, json.Unmarshal(payload, &ev))
		events = append(events, ev)
	}
	require.NoError(t, rows.Err())
	return events
}

// createTestSubscription は業務アクションを経由せず subscription 行と token 行
// を seed する。initialStatus はケースごとに必須で渡し、後から mutate しない
// (Given を 1 call で確定させるため)。
func createTestSubscription(t *testing.T, _ *postgres.SubscriptionRepository, platform, playerID, purchaseToken, initialStatus string) *domain.Subscription {
	t.Helper()
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)

	tokenTable, err := subscriptionTokenTableForPlatform(platform)
	require.NoError(t, err)

	sub := &domain.Subscription{
		PlayerID:           playerID,
		ProductID:          "premium_monthly",
		Status:             initialStatus,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	ctx := context.Background()
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`INSERT INTO shop.subscriptions (player_id, product_id, status, current_period_start, current_period_end, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING subscription_id`,
		sub.PlayerID, sub.ProductID, sub.Status,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CreatedAt, sub.UpdatedAt,
	).Scan(&sub.SubscriptionID))
	_, err = sharedPg.Pool.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (token, subscription_id) VALUES ($1, $2)`, tokenTable),
		purchaseToken, sub.SubscriptionID,
	)
	require.NoError(t, err)
	return sub
}

func subscriptionTokenTableForPlatform(platform string) (string, error) {
	switch platform {
	case domain.PlatformIOS:
		return "shop.apple_subscription_tokens", nil
	case domain.PlatformAndroid:
		return "shop.google_subscription_tokens", nil
	default:
		return "", fmt.Errorf("test seed: unsupported platform %q", platform)
	}
}
