//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

func newSub(playerID string, now time.Time) *domain.Subscription {
	return &domain.Subscription{
		PlayerID:           playerID,
		ProductID:          "premium_monthly",
		Status:             domain.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func newPremiumEvent() port.OutboxEvent {
	return port.OutboxEvent{
		EventID:   uuid.New(),
		EventType: apishop.EventTypePremiumUpdated,
		Payload:   []byte(`{}`),
	}
}

func TestSubscriptionRepository_CreateSubscription(t *testing.T) {
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	const playerID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	t.Run("subscription の作成", func(t *testing.T) {
		platformCases := []struct {
			name     string
			platform string
			token    string
		}{
			{
				name:     "iOS のとき、apple トークンで作成され検索できる",
				platform: domain.PlatformIOS,
				token:    "apple-orig-tx-1",
			},
			{
				name:     "Android のとき、google トークンで作成され検索できる",
				platform: domain.PlatformAndroid,
				token:    "google-purchase-tok-1",
			},
		}

		for _, tt := range platformCases {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				sub := newSub(playerID, time.Now().UTC())

				require.NoError(t, repo.CreateSubscription(ctx, sub, tt.platform, tt.token, newPremiumEvent()))
				assert.NotZero(t, sub.SubscriptionID)

				found, err := repo.FindSubscriptionByToken(ctx, tt.platform, tt.token)
				require.NoError(t, err)
				require.NotNil(t, found)
				assert.Equal(t, sub.SubscriptionID, found.SubscriptionID)
				assert.Equal(t, domain.SubscriptionStatusActive, found.Status)
			})
		}

		t.Run("unsupported platform のとき、ErrUnsupportedPlatform になる", func(t *testing.T) {
			sharedPg.Truncate(t)
			sub := newSub(playerID, time.Now().UTC())
			err := repo.CreateSubscription(ctx, sub, "windows", "tok", newPremiumEvent())
			assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
		})
	})
}

func TestSubscriptionRepository_GetLatestSubscription(t *testing.T) {
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("最新 subscription の取得", func(t *testing.T) {
		t.Run("同一プレイヤーに複数あるとき、最も新しい active を返す", func(t *testing.T) {
			sharedPg.Truncate(t)
			const playerID = "dddddddd-dddd-dddd-dddd-dddddddddddd"
			base := time.Now().UTC().Add(-72 * time.Hour)

			older := newSub(playerID, base)
			older.Status = domain.SubscriptionStatusExpired
			require.NoError(t, repo.CreateSubscription(ctx, older, domain.PlatformIOS, "old-tok", newPremiumEvent()))

			newer := newSub(playerID, base.Add(48*time.Hour))
			require.NoError(t, repo.CreateSubscription(ctx, newer, domain.PlatformIOS, "new-tok", newPremiumEvent()))

			latest, err := repo.GetLatestSubscription(ctx, playerID)
			require.NoError(t, err)
			require.NotNil(t, latest)
			assert.Equal(t, newer.SubscriptionID, latest.SubscriptionID)
			assert.Equal(t, domain.SubscriptionStatusActive, latest.Status)
		})

		t.Run("subscription が無いとき、nil を返す", func(t *testing.T) {
			sharedPg.Truncate(t)
			got, err := repo.GetLatestSubscription(ctx, "ffffffff-ffff-ffff-ffff-ffffffffffff")
			require.NoError(t, err)
			assert.Nil(t, got)
		})
	})
}

func TestSubscriptionRepository_FindSubscriptionByToken(t *testing.T) {
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("token による subscription 検索", func(t *testing.T) {
		t.Run("存在しない token のとき、nil を返す", func(t *testing.T) {
			sharedPg.Truncate(t)
			got, err := repo.FindSubscriptionByToken(ctx, domain.PlatformIOS, "nonexistent-token")
			require.NoError(t, err)
			assert.Nil(t, got)
		})

		t.Run("unsupported platform のとき、ErrUnsupportedPlatform になる", func(t *testing.T) {
			sharedPg.Truncate(t)
			_, err := repo.FindSubscriptionByToken(ctx, "windows", "tok")
			assert.ErrorIs(t, err, port.ErrUnsupportedPlatform)
		})
	})
}

func TestSubscriptionRepository_UpdateSubscription(t *testing.T) {
	repo := postgres.NewSubscriptionRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("subscription の更新", func(t *testing.T) {
		t.Run("status と current_period_end を更新するとき、永続化される", func(t *testing.T) {
			sharedPg.Truncate(t)
			now := time.Now().UTC().Truncate(time.Microsecond)
			sub := newSub("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", now)
			require.NoError(t, repo.CreateSubscription(ctx, sub, domain.PlatformIOS, "upd-token", newPremiumEvent()))

			sub.Status = domain.SubscriptionStatusCancelled
			newEnd := now.Add(60 * 24 * time.Hour)
			sub.CurrentPeriodEnd = newEnd
			sub.UpdatedAt = now.Add(time.Hour)
			require.NoError(t, repo.UpdateSubscription(ctx, sub))

			got, err := repo.FindSubscriptionByToken(ctx, domain.PlatformIOS, "upd-token")
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, domain.SubscriptionStatusCancelled, got.Status)
			assert.True(t, got.CurrentPeriodEnd.Equal(newEnd))
		})
	})
}
