//go:build integration

package subscription_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase/purchasetest"
)

const (
	playerID              = "00000000-0000-4000-8000-000000000001"
	subscriptionProductID = "TST-SUBSCRIPTION-1"
	subscriptionMonths    = 1
	appleToken            = "TST-APPLE-ORIGINAL-TRANSACTION-1"
	googleToken           = "TST-GOOGLE-PURCHASE-TOKEN-1"

	platformIOS     = "ios"
	platformAndroid = "android"

	eventTypePremiumUpdated = "premium_updated"
)

var (
	registeredExpiresAt = time.Date(2030, 1, 15, 8, 0, 0, 0, time.UTC)
	renewedExpiresAt    = time.Date(2027, 1, 15, 8, 0, 0, 0, time.UTC)

	errVerify = errors.New("TST-verify-failed")
)

type outcome struct {
	platform string
	token    string
	before   domain.Subscription
	snapshot *purchasetest.OutboxSnapshot
	err      error
}

func newShop(t *testing.T) {
	t.Helper()
	pg.Truncate(t)
	pg.InsertSubscriptionProduct(t, postgrestest.ProductSeed{ProductID: subscriptionProductID, Name: "TST-subscription", Price: 100, IsActive: true}, subscriptionMonths)
}

func registerContract(t *testing.T, platform, token string) {
	t.Helper()
	apple := purchasetest.NewStoreVerifier()
	google := purchasetest.NewStoreVerifier()
	apple.AcceptToken(token, subscriptionProductID, registeredExpiresAt)
	google.AcceptToken(token, subscriptionProductID, registeredExpiresAt)
	service := purchasetest.NewService(pg.Pool, apple, google)
	_, err := service.Subscribe(t.Context(), playerID, subscriptionProductID, platform, token)
	require.NoError(t, err)
}

func findContractByToken(t *testing.T, platform, token string) domain.Subscription {
	t.Helper()
	contracts := purchasetest.FindSubscriptionsByToken(t, pg.Pool, platform, token)
	require.Len(t, contracts, 1)
	return contracts[0]
}

func process(t *testing.T, platform, token string, handle func() error) outcome {
	t.Helper()
	before := findContractByToken(t, platform, token)
	snapshot := purchasetest.TakeOutboxSnapshot(t, pg.Pool)
	return outcome{platform: platform, token: token, before: before, snapshot: snapshot, err: handle()}
}

func processWithoutContract(t *testing.T, handle func() error) outcome {
	t.Helper()
	snapshot := purchasetest.TakeOutboxSnapshot(t, pg.Pool)
	return outcome{snapshot: snapshot, err: handle()}
}

func (o outcome) findContract(t *testing.T) domain.Subscription {
	t.Helper()
	return findContractByToken(t, o.platform, o.token)
}

func (o outcome) listNewPremiumEvents(t *testing.T) []purchasetest.OutboxEvent {
	t.Helper()
	return o.snapshot.NewEvents(t, eventTypePremiumUpdated)
}

func (o outcome) findSinglePremiumEvent(t *testing.T) purchasetest.OutboxEvent {
	t.Helper()
	events := o.listNewPremiumEvents(t)
	require.Len(t, events, 1)
	return events[0]
}
