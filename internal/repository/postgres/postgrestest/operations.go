package postgrestest

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
)

const (
	dummyProductID            = "TST-PRODUCT-1"
	dummyEventType            = "TST-EVENT-TYPE"
	dummyEventPayload         = `{"dummy":"value"}`
	dummyPlatform             = "ios"
	dummyTokenCharacter       = "t"
	claimAllLimit             = 1000
	claimAllFailureThreshold  = 1000
	claimAllVisibilityTimeout = 0
	pgCodeDuplicateKey        = "23505"
	pgCodeValueTooLong        = "22001"
)

var (
	dummyPurchasedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	dummyPeriodStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dummyPeriodEnd   = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	dummyCreatedAt   = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	dummyUpdatedAt   = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
)

// CardPackPurchaseRepository は接続プールを使うカードパック購入のリポジトリを返す。
func (p *Postgres) CardPackPurchaseRepository() *postgres.CardPackPurchaseRepository {
	return postgres.NewCardPackPurchaseRepository(p.Pool)
}

// FactionPurchaseRepository は接続プールを使う陣営購入のリポジトリを返す。
func (p *Postgres) FactionPurchaseRepository() *postgres.FactionPurchaseRepository {
	return postgres.NewFactionPurchaseRepository(p.Pool)
}

// ItemPurchaseRepository は接続プールを使うアイテム購入のリポジトリを返す。
func (p *Postgres) ItemPurchaseRepository() *postgres.ItemPurchaseRepository {
	return postgres.NewItemPurchaseRepository(p.Pool)
}

// PurchaseLookupRepository は接続プールを使う購入検索のリポジトリを返す。
func (p *Postgres) PurchaseLookupRepository() *postgres.PurchaseLookupRepository {
	return postgres.NewPurchaseLookupRepository(p.Pool)
}

// ProductRepository は接続プールを使う商品のリポジトリを返す。
func (p *Postgres) ProductRepository() *postgres.ProductRepository {
	return postgres.NewProductRepository(p.Pool)
}

// OutboxRepository は接続プールを使うアウトボックスのリポジトリを返す。
func (p *Postgres) OutboxRepository() *postgres.OutboxRepository {
	return postgres.NewOutboxRepository(p.Pool)
}

// SubscriptionRepository は接続プールを使うサブスクリプションのリポジトリを返す。
func (p *Postgres) SubscriptionRepository() *postgres.SubscriptionRepository {
	return postgres.NewSubscriptionRepository(p.Pool)
}

// NewOutboxEvent は新しいイベントIDを持つダミーのイベントを返す。
func NewOutboxEvent() port.OutboxEvent {
	return NewOutboxEventWithID(uuid.New())
}

// NewOutboxEventWithID は eventID を持つダミーのイベントを返す。
func NewOutboxEventWithID(eventID uuid.UUID) port.OutboxEvent {
	return port.OutboxEvent{EventID: eventID, EventType: dummyEventType, Payload: []byte(dummyEventPayload)}
}

// NewSubscription は playerID の有効なダミーのサブスクリプションを返す。
func NewSubscription(playerID string) *domain.Subscription {
	return &domain.Subscription{
		PlayerID:           playerID,
		ProductID:          dummyProductID,
		Status:             "active",
		CurrentPeriodStart: dummyPeriodStart,
		CurrentPeriodEnd:   dummyPeriodEnd,
		CreatedAt:          dummyCreatedAt,
		UpdatedAt:          dummyUpdatedAt,
	}
}

// TokenOfLength は length 文字の購入トークンを返す。
func TokenOfLength(length int) string {
	return strings.Repeat(dummyTokenCharacter, length)
}

func newDummyPurchase(playerID string) *domain.OneTimePurchase {
	return &domain.OneTimePurchase{PlayerID: playerID, ProductID: dummyProductID, PurchasedAt: dummyPurchasedAt}
}

// CreateCardPackPurchase は新しいイベントとともにカードパックの購入を作成する。
func (p *Postgres) CreateCardPackPurchase(playerID, cardPackID, token string) error {
	return p.CreateCardPackPurchaseWithEvent(playerID, cardPackID, token, NewOutboxEvent())
}

// CreateCardPackPurchaseWithEvent は event とともにカードパックの購入を作成する。
func (p *Postgres) CreateCardPackPurchaseWithEvent(playerID, cardPackID, token string, event port.OutboxEvent) error {
	return p.CardPackPurchaseRepository().CreatePurchase(context.Background(), newDummyPurchase(playerID), cardPackID, dummyPlatform, token, event)
}

// CreateFactionPurchase は新しい 2 件のイベントとともに陣営セットの購入を作成する。
func (p *Postgres) CreateFactionPurchase(playerID, faction, cardPackID, token string) error {
	return p.CreateFactionPurchaseWithEvents(playerID, faction, cardPackID, token, []port.OutboxEvent{NewOutboxEvent(), NewOutboxEvent()})
}

// CreateFactionPurchaseWithEvents は events とともに陣営セットの購入を作成する。
func (p *Postgres) CreateFactionPurchaseWithEvents(playerID, faction, cardPackID, token string, events []port.OutboxEvent) error {
	return p.FactionPurchaseRepository().CreatePurchase(context.Background(), newDummyPurchase(playerID), faction, cardPackID, dummyPlatform, token, events)
}

// CreateItemPurchase は獲得日時を購入日時と同じ固定の時刻にしてアイテムの購入を作成する。
func (p *Postgres) CreateItemPurchase(playerID, itemType string, itemNo int64, platform, token string) error {
	return p.CreateItemPurchaseAt(playerID, itemType, itemNo, platform, token, dummyPurchasedAt)
}

// CreateItemPurchaseAt は獲得日時を acquiredAt にしてアイテムの購入を作成する。
func (p *Postgres) CreateItemPurchaseAt(playerID, itemType string, itemNo int64, platform, token string, acquiredAt time.Time) error {
	item := &domain.PlayerItem{PlayerID: playerID, ItemType: itemType, ItemNo: itemNo, AcquiredAt: acquiredAt}
	return p.ItemPurchaseRepository().CreatePurchase(context.Background(), newDummyPurchase(playerID), item, platform, token)
}

// CreateSubscription は subscription を event とともに作成する。
func (p *Postgres) CreateSubscription(subscription *domain.Subscription, platform, token string, event port.OutboxEvent) error {
	return p.SubscriptionRepository().CreateSubscription(context.Background(), subscription, platform, token, event)
}

// FindSubscription は購入トークンで保存済みのサブスクリプションを取得する。見つからないときはテストを失敗させる。
func (p *Postgres) FindSubscription(t *testing.T, platform, token string) *domain.Subscription {
	t.Helper()
	found, err := p.SubscriptionRepository().FindSubscriptionByToken(context.Background(), platform, token)
	require.NoError(t, err)
	require.NotNil(t, found)
	return found
}

// ClaimAll は保存済みの未配信イベントを、取得待ち時間 0 秒ですべて取得する。
func (p *Postgres) ClaimAll(t *testing.T) []port.ClaimedOutboxEvent {
	t.Helper()
	claimed, err := p.OutboxRepository().ClaimUnpublished(context.Background(), claimAllLimit, claimAllVisibilityTimeout, claimAllFailureThreshold)
	require.NoError(t, err)
	return claimed
}

// ClaimedEventIDs は取得したイベントのイベントIDを取得順に返す。
func ClaimedEventIDs(events []port.ClaimedOutboxEvent) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventID)
	}
	return ids
}

// RunConcurrently は operation を count 回並行に実行し、すべての完了を待って各戻り値を返す。
func RunConcurrently(count int, operation func() error) []error {
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = operation()
		}()
	}
	wg.Wait()
	return errs
}

// RequireDuplicateKeyError は err の原因が constraintName の制約への重複であることを確かめる。
func RequireDuplicateKeyError(t *testing.T, err error, constraintName string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, pgCodeDuplicateKey, pgErr.Code)
	assert.Equal(t, constraintName, pgErr.ConstraintName)
}

// RequireValueTooLongError は err の原因が列の長さの超過であることを確かめる。
func RequireValueTooLongError(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, pgCodeValueTooLong, pgErr.Code)
}
