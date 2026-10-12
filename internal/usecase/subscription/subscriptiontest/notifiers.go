package subscriptiontest

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

// NewAppleNotifier は実 PostgreSQL のリポジトリに結線した Apple 通知の処理を、署名検証の偽物と bundle ID を指定して返す。
func NewAppleNotifier(pool *pgxpool.Pool, verifier port.AppleJWSVerifier, bundleID string) *subscription.AppleNotifier {
	return subscription.NewAppleNotifier(postgres.NewSubscriptionRepository(pool), verifier, bundleID)
}

// NewGoogleNotifier は実 PostgreSQL のリポジトリに結線した Google 通知の処理を、有効期限の取得の偽物を指定して返す。
func NewGoogleNotifier(pool *pgxpool.Pool, expiryFetcher port.GoogleSubVerifier) *subscription.GoogleNotifier {
	return subscription.NewGoogleNotifier(postgres.NewSubscriptionRepository(pool), expiryFetcher)
}
