//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
)

func newSub(playerID string, now time.Time) *apishop.Subscription {
	return &apishop.Subscription{
		PlayerID:           playerID,
		ProductID:          "premium_monthly",
		Status:             apishop.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func TestSubscriptionRepository_CreateSubscription_Apple(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	sub := newSub("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", now)

	require.NoError(t, repo.CreateSubscription(ctx, sub, apishop.PlatformIOS, "apple-orig-tx-1", port.OutboxEvent{}))
	assert.NotZero(t, sub.SubscriptionID)

	found, err := repo.FindSubscriptionByToken(ctx, apishop.PlatformIOS, "apple-orig-tx-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, sub.SubscriptionID, found.SubscriptionID)
	assert.Equal(t, apishop.SubscriptionStatusActive, found.Status)
}

func TestSubscriptionRepository_CreateSubscription_Google(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	sub := newSub("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", time.Now().UTC())
	require.NoError(t, repo.CreateSubscription(ctx, sub, apishop.PlatformAndroid, "google-purchase-tok-1", port.OutboxEvent{}))

	found, err := repo.FindSubscriptionByToken(ctx, apishop.PlatformAndroid, "google-purchase-tok-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, sub.SubscriptionID, found.SubscriptionID)
}

func TestSubscriptionRepository_CreateSubscription_UnsupportedPlatform(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	sub := newSub("cccccccc-cccc-cccc-cccc-cccccccccccc", time.Now().UTC())
	err := repo.CreateSubscription(ctx, sub, "windows", "tok", port.OutboxEvent{})
	assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
}

func TestSubscriptionRepository_GetLatestSubscription(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	playerID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	base := time.Now().UTC().Add(-72 * time.Hour)

	older := newSub(playerID, base)
	older.Status = apishop.SubscriptionStatusExpired
	require.NoError(t, repo.CreateSubscription(ctx, older, apishop.PlatformIOS, "old-tok", port.OutboxEvent{}))

	newer := newSub(playerID, base.Add(48*time.Hour))
	require.NoError(t, repo.CreateSubscription(ctx, newer, apishop.PlatformIOS, "new-tok", port.OutboxEvent{}))

	latest, err := repo.GetLatestSubscription(ctx, playerID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, newer.SubscriptionID, latest.SubscriptionID)
	assert.Equal(t, apishop.SubscriptionStatusActive, latest.Status)
}

func TestSubscriptionRepository_GetLatestSubscription_NotFound(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	got, err := repo.GetLatestSubscription(ctx, "ffffffff-ffff-ffff-ffff-ffffffffffff")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSubscriptionRepository_FindSubscriptionByToken_NotFound(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	got, err := repo.FindSubscriptionByToken(ctx, apishop.PlatformIOS, "nonexistent-token")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSubscriptionRepository_FindSubscriptionByToken_UnsupportedPlatform(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	_, err := repo.FindSubscriptionByToken(ctx, "windows", "tok")
	assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
}

func TestSubscriptionRepository_UpdateSubscription(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	sub := newSub("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", now)
	require.NoError(t, repo.CreateSubscription(ctx, sub, apishop.PlatformIOS, "upd-token", port.OutboxEvent{}))

	sub.Status = apishop.SubscriptionStatusCancelled
	newEnd := now.Add(60 * 24 * time.Hour)
	sub.CurrentPeriodEnd = newEnd
	sub.UpdatedAt = now.Add(time.Hour)
	require.NoError(t, repo.UpdateSubscription(ctx, sub, port.OutboxEvent{}))

	got, err := repo.FindSubscriptionByToken(ctx, apishop.PlatformIOS, "upd-token")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, apishop.SubscriptionStatusCancelled, got.Status)
	assert.True(t, got.CurrentPeriodEnd.Equal(newEnd))
}
