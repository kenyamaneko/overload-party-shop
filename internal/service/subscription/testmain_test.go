package subscription

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
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

// fakePremiumBuilder は BuildPremiumUpdated 呼び出しを記録するテスト用ダブル。
// AppleNotifier / GoogleNotifier 共通で premium-updated イベントの enqueue を観測する。
type fakePremiumBuilder struct {
	calls []fakePremiumPubCall
	err   error
}

type fakePremiumPubCall struct {
	PlayerID  string
	IsPremium bool
	ExpiresAt *time.Time
}

func (f *fakePremiumBuilder) Build(playerID string, isPremium bool, expiresAt *time.Time) (port.OutboxEvent, error) {
	f.calls = append(f.calls, fakePremiumPubCall{PlayerID: playerID, IsPremium: isPremium, ExpiresAt: expiresAt})
	if f.err != nil {
		return port.OutboxEvent{}, f.err
	}
	return port.OutboxEvent{EventID: uuid.New(), Topic: apishop.TopicPremiumUpdated, Payload: []byte(`{}`)}, nil
}

// fakeEventBuilder は port.OutboxEventBuilder を満たす。subscription テストでは
// faction 側は呼ばれないので no-op。
type fakeEventBuilder struct {
	premiumPub *fakePremiumBuilder
}

func newFakeEventBuilder(premiumErr error) *fakeEventBuilder {
	return &fakeEventBuilder{
		premiumPub: &fakePremiumBuilder{err: premiumErr},
	}
}

func (f *fakeEventBuilder) BuildFactionPurchased(_, _ string) (port.OutboxEvent, error) {
	return port.OutboxEvent{}, nil
}

func (f *fakeEventBuilder) BuildPremiumUpdated(playerID string, isPremium bool, expiresAt *time.Time) (port.OutboxEvent, error) {
	return f.premiumPub.Build(playerID, isPremium, expiresAt)
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
