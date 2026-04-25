//go:build integration

package subscription

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
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

// selectPremiumUpdatedEvents は shop.outbox_events から premium-updated の payload
// を取り出して apishop.PremiumUpdatedEvent としてデコードする。subscription
// notifier が outbox に enqueue した事実を直接検証するためのヘルパ。
func selectPremiumUpdatedEvents(t *testing.T) []apishop.PremiumUpdatedEvent {
	t.Helper()
	rows, err := sharedPg.Pool.Query(context.Background(),
		`SELECT payload FROM shop.outbox_events WHERE topic = $1 ORDER BY created_at`,
		apishop.TopicPremiumUpdated)
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

// createTestSubscription はテスト用に指定の初期状態でサブスク行を作成する。
// initialStatus は各ケースが明示して受け取る — テスト内で後から mutate せず、
// Given が call 1 回で確定する形にするための必須引数。
func createTestSubscription(t *testing.T, repo *postgres.SubscriptionRepository, platform, playerID, purchaseToken, initialStatus string) *apishop.Subscription {
	t.Helper()
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)

	sub := &apishop.Subscription{
		PlayerID:           playerID,
		ProductID:          "premium_monthly",
		Status:             initialStatus,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	require.NoError(t, repo.CreateSubscription(context.Background(), sub, platform, purchaseToken, port.OutboxEvent{}))
	return sub
}
