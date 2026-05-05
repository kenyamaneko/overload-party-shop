//go:build integration

package purchase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testShopEnv struct {
	svc                 *Service
	productRepo         *postgres.ProductRepository
	factionPurchaseRepo *postgres.FactionPurchaseRepository
	itemPurchaseRepo    *postgres.ItemPurchaseRepository
	purchaseLookup      *postgres.PurchaseLookupRepository
	subRepo             *postgres.SubscriptionRepository
}

// shopEnvOption は newTestShopEnv に渡す依存差し替えオプション。
type shopEnvOption func(*shopEnvDeps)

type shopEnvDeps struct {
	appleVerifier  port.ReceiptVerifier
	googleVerifier port.ReceiptVerifier
}

func withAppleVerifier(v port.ReceiptVerifier) shopEnvOption {
	return func(d *shopEnvDeps) { d.appleVerifier = v }
}

func withGoogleVerifier(v port.ReceiptVerifier) shopEnvOption {
	return func(d *shopEnvDeps) { d.googleVerifier = v }
}

func newTestShopEnv(t *testing.T, opts ...shopEnvOption) *testShopEnv {
	t.Helper()
	sharedPg.Truncate(t)

	deps := &shopEnvDeps{
		appleVerifier:  &port.MockReceiptVerifier{},
		googleVerifier: &port.MockReceiptVerifier{},
	}
	for _, opt := range opts {
		opt(deps)
	}

	productRepo := postgres.NewProductRepository(sharedPg.Pool)
	factionPurchaseRepo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	itemPurchaseRepo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	purchaseLookup := postgres.NewPurchaseLookupRepository(sharedPg.Pool)
	subRepo := postgres.NewSubscriptionRepository(sharedPg.Pool)

	svc := New(
		productRepo, factionPurchaseRepo, itemPurchaseRepo, purchaseLookup, subRepo,
		deps.appleVerifier, deps.googleVerifier,
	)

	return &testShopEnv{
		svc:                 svc,
		productRepo:         productRepo,
		factionPurchaseRepo: factionPurchaseRepo,
		itemPurchaseRepo:    itemPurchaseRepo,
		purchaseLookup:      purchaseLookup,
		subRepo:             subRepo,
	}
}

func selectFactionPurchasedEvents(t *testing.T) []domain.FactionPurchasedEvent {
	t.Helper()
	rows, err := sharedPg.Pool.Query(context.Background(),
		`SELECT payload FROM shop.outbox_events WHERE event_type = $1 ORDER BY created_at`,
		domain.EventTypeFactionPurchased)
	require.NoError(t, err)
	defer rows.Close()
	var events []domain.FactionPurchasedEvent
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var ev domain.FactionPurchasedEvent
		require.NoError(t, json.Unmarshal(payload, &ev))
		events = append(events, ev)
	}
	require.NoError(t, rows.Err())
	return events
}

func selectPremiumUpdatedEvents(t *testing.T) []domain.PremiumUpdatedEvent {
	t.Helper()
	rows, err := sharedPg.Pool.Query(context.Background(),
		`SELECT payload FROM shop.outbox_events WHERE event_type = $1 ORDER BY created_at`,
		domain.EventTypePremiumUpdated)
	require.NoError(t, err)
	defer rows.Close()
	var events []domain.PremiumUpdatedEvent
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var ev domain.PremiumUpdatedEvent
		require.NoError(t, json.Unmarshal(payload, &ev))
		events = append(events, ev)
	}
	require.NoError(t, rows.Err())
	return events
}

// insertCommonProduct は shop.products に共通行を直接 INSERT する (副表は呼び元が入れる)。
func insertCommonProduct(t *testing.T, productID, name, productType string, price int64, isActive bool) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.products (product_id, name, type, price, description, image_url, is_active)
		 VALUES ($1,$2,$3,$4,NULL,NULL,$5)`,
		productID, name, productType, price, isActive)
	require.NoError(t, err)
}

// insertFactionSetProduct は faction_set 商品 (products + product_faction_grants) を seed する。
func insertFactionSetProduct(t *testing.T, productID, name string, price int64, faction string, isActive bool) {
	t.Helper()
	insertCommonProduct(t, productID, name, domain.ProductTypeFactionSet, price, isActive)
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.product_faction_grants (product_id, faction) VALUES ($1,$2)`,
		productID, faction)
	require.NoError(t, err)
}

// insertCosmeticProduct は cosmetic 商品 (products + cosmetic_items + product_cosmetics) を seed する。
// cosmetic_items は ON CONFLICT DO NOTHING で多重 seed を許容する。
func insertCosmeticProduct(t *testing.T, productID, name string, price int64, itemType string, itemNo int64, isActive bool) {
	t.Helper()
	insertCommonProduct(t, productID, name, domain.ProductTypeCosmetic, price, isActive)
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.cosmetic_items (item_type, item_no, item_name, is_purchasable, is_active)
		 VALUES ($1,$2,$3,true,true) ON CONFLICT DO NOTHING`,
		itemType, itemNo, name)
	require.NoError(t, err)
	_, err = sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.product_cosmetics (product_id, item_type, item_no) VALUES ($1,$2,$3)`,
		productID, itemType, itemNo)
	require.NoError(t, err)
}

// insertSubscriptionProduct は subscription 商品 (products のみ、副表なし) を seed する。
func insertSubscriptionProduct(t *testing.T, productID, name string, price int64, isActive bool) {
	t.Helper()
	insertCommonProduct(t, productID, name, domain.ProductTypeSubscription, price, isActive)
}

// insertSubscription は業務アクションを経由せず subscription 行と token 行を seed する。
func insertSubscription(t *testing.T, sub *domain.Subscription, platform, purchaseToken string) {
	t.Helper()
	tokenTable := map[string]string{
		domain.PlatformIOS:     "shop.apple_subscription_tokens",
		domain.PlatformAndroid: "shop.google_subscription_tokens",
	}[platform]
	require.NotEmpty(t, tokenTable, "test seed: unsupported platform %q", platform)

	ctx := context.Background()
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`INSERT INTO shop.subscriptions (player_id, product_id, status, current_period_start, current_period_end, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING subscription_id`,
		sub.PlayerID, sub.ProductID, sub.Status,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CreatedAt, sub.UpdatedAt,
	).Scan(&sub.SubscriptionID))
	_, err := sharedPg.Pool.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (token, subscription_id) VALUES ($1, $2)`, tokenTable),
		purchaseToken, sub.SubscriptionID,
	)
	require.NoError(t, err)
}

func TestPurchase_FactionSet_Success(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-123", ProductID: "faction_tenki"}, nil
		},
	}))

	insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

	err := env.svc.Purchase(context.Background(), "11111111-1111-1111-1111-111111111111", "faction_tenki", "ios", "receipt-token-1")
	require.NoError(t, err)

	t.Run("publishes faction-purchased event", func(t *testing.T) {
		events := selectFactionPurchasedEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", events[0].PlayerID)
		assert.Equal(t, "Tenki", events[0].Faction)
	})

	t.Run("writes shop-local owned faction", func(t *testing.T) {
		factions, err := env.factionPurchaseRepo.ListOwnedFactions(context.Background(), "11111111-1111-1111-1111-111111111111")
		require.NoError(t, err)
		assert.Contains(t, factions, "Tenki")
	})
}

func TestPurchase_Idempotent(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-123"}, nil
		},
	}))

	insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

	playerID := "22222222-2222-2222-2222-222222222222"
	ctx := context.Background()
	require.NoError(t, env.svc.Purchase(ctx, playerID, "faction_tenki", "ios", "receipt-token-1"))

	// 同一トークンでの再購入 — べき等
	require.NoError(t, env.svc.Purchase(ctx, playerID, "faction_tenki", "ios", "receipt-token-1"))

	// publish は 1 回のみ (2 回目は既存 token 検出経路で publish 前に return)
	assert.Len(t, selectFactionPurchasedEvents(t), 1)
}

func TestPurchase_ReceiptFailed(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: false}, nil
		},
	}))

	insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

	err := env.svc.Purchase(context.Background(), "33333333-3333-3333-3333-333333333333", "faction_tenki", "ios", "bad-receipt")
	assert.ErrorIs(t, err, ErrReceiptVerificationFailed)
	assert.Empty(t, selectFactionPurchasedEvents(t))
}

func TestPurchase_CosmeticItem(t *testing.T) {
	env := newTestShopEnv(t, withGoogleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-456"}, nil
		},
	}))

	insertCosmeticProduct(t, "playmat_01", "プレイマット: サイバー", 320, "playmat", 1, true)

	err := env.svc.Purchase(context.Background(), "44444444-4444-4444-4444-444444444444", "playmat_01", "android", "cosmetic-receipt")
	require.NoError(t, err)
	assert.Empty(t, selectFactionPurchasedEvents(t))
}

func TestPurchase_AlreadyOwned_FactionSet(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-first"}, nil
		},
	}))

	insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

	playerID := "55555555-5555-5555-5555-555555555555"
	err := env.svc.Purchase(context.Background(), playerID, "faction_tenki", "ios", "receipt-token-1")
	require.NoError(t, err)

	// 別トークンでの再購入は拒否 (ownedFactions 検出)
	err = env.svc.Purchase(context.Background(), playerID, "faction_tenki", "ios", "receipt-token-2")
	assert.ErrorIs(t, err, ErrAlreadyOwned)
}

func TestPurchase_AlreadyOwned_Cosmetic(t *testing.T) {
	env := newTestShopEnv(t, withGoogleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-cos"}, nil
		},
	}))

	insertCosmeticProduct(t, "playmat_01", "プレイマット: サイバー", 320, "playmat", 1, true)

	playerID := "66666666-6666-6666-6666-666666666666"
	err := env.svc.Purchase(context.Background(), playerID, "playmat_01", "android", "cosmetic-receipt-1")
	require.NoError(t, err)

	err = env.svc.Purchase(context.Background(), playerID, "playmat_01", "android", "cosmetic-receipt-2")
	assert.ErrorIs(t, err, ErrAlreadyOwned)
}

func TestPurchase_InactiveProduct(t *testing.T) {
	env := newTestShopEnv(t)

	insertFactionSetProduct(t, "old_product", "旧商品", 100, "SHE", false)

	err := env.svc.Purchase(context.Background(), "77777777-7777-7777-7777-777777777777", "old_product", "ios", "receipt-1")
	assert.ErrorIs(t, err, ErrProductNotActive)
}

func TestPurchase_UnsupportedPlatform(t *testing.T) {
	env := newTestShopEnv(t)

	insertFactionSetProduct(t, "faction_she", "SHEカードセット", 980, "SHE", true)

	err := env.svc.Purchase(context.Background(), "88888888-8888-8888-8888-888888888888", "faction_she", "windows", "receipt-1")
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)
}

func TestSubscribe_Success(t *testing.T) {
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
			return &port.SubscriptionInfo{
				IsValid:   true,
				ProductID: "premium_monthly",
				ExpiresAt: expiresAt,
			}, nil
		},
	}))

	insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)

	playerID := "99999999-9999-9999-9999-999999999999"
	result, err := env.svc.Subscribe(context.Background(), playerID, "premium_monthly", "ios", "sub-token-1")
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Run("publishes premium-updated event", func(t *testing.T) {
		events := selectPremiumUpdatedEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, playerID, events[0].PlayerID)
		assert.True(t, events[0].IsPremium)
	})

	t.Run("creates active subscription record", func(t *testing.T) {
		sub, err := env.subRepo.GetLatestSubscription(context.Background(), playerID)
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, domain.SubscriptionStatusActive, sub.Status)
	})
}

func TestSubscribe_Errors(t *testing.T) {
	// verifier を呼ばない経路のケースでも DI の形を揃えるため default を明示する。
	defaultVerifier := &port.MockReceiptVerifier{}
	invalidSubVerifier := &port.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
			return &port.SubscriptionInfo{IsValid: false}, nil
		},
	}

	tests := []struct {
		name          string
		seedProduct   func(t *testing.T)
		appleVerifier port.ReceiptVerifier
		productID     string
		platform      string
		token         string
		wantErr       error
	}{
		{
			name: "サブスク以外の商品を Subscribe",
			seedProduct: func(t *testing.T) {
				insertFactionSetProduct(t, "faction_she", "SHEカードセット", 980, "SHE", true)
			},
			appleVerifier: defaultVerifier,
			productID:     "faction_she",
			platform:      "ios",
			token:         "sub-token-1",
			wantErr:       ErrProductNotSubscription,
		},
		{
			name: "レシート検証失敗（IsValid=false）",
			seedProduct: func(t *testing.T) {
				insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)
			},
			appleVerifier: invalidSubVerifier,
			productID:     "premium_monthly",
			platform:      "ios",
			token:         "bad-sub-token",
			wantErr:       ErrSubVerificationFailed,
		},
		{
			name: "未対応 platform",
			seedProduct: func(t *testing.T) {
				insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)
			},
			appleVerifier: defaultVerifier,
			productID:     "premium_monthly",
			platform:      "windows",
			token:         "sub-token-1",
			wantErr:       ErrUnsupportedPlatform,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestShopEnv(t, withAppleVerifier(tt.appleVerifier))
			tt.seedProduct(t)

			_, err := env.svc.Subscribe(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", tt.productID, tt.platform, tt.token)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestGetProducts_WithOwnership(t *testing.T) {
	env := newTestShopEnv(t)

	insertFactionSetProduct(t, "faction_she", "SHEカードセット", 980, "SHE", true)
	insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

	playerID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	// shop 購入で SHE faction を所有している状態をシミュレート
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.player_owned_factions (player_id, faction) VALUES ($1, $2)`,
		playerID, "SHE")
	require.NoError(t, err)

	products, err := env.svc.GetProducts(context.Background(), playerID)
	require.NoError(t, err)
	require.Len(t, products, 2)

	byID := map[string]domain.ProductWithOwnership{}
	for _, p := range products {
		byID[p.ProductView.Common().ProductID] = p
	}
	assert.True(t, byID["faction_she"].IsOwned)
	assert.False(t, byID["faction_tenki"].IsOwned)
}

func TestGetProducts_SubscriptionOwnership(t *testing.T) {
	env := newTestShopEnv(t)

	insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)

	playerID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	now := time.Now()
	insertSubscription(t, &domain.Subscription{
		PlayerID:           playerID,
		ProductID:          "premium_monthly",
		Status:             domain.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}, domain.PlatformIOS, "sub-token")

	products, err := env.svc.GetProducts(context.Background(), playerID)
	require.NoError(t, err)
	require.Len(t, products, 1)
	assert.True(t, products[0].IsOwned)
}

// 各 subscription status で IsOwned が特典有効性に従って判定される。
func TestGetProducts_SubscriptionOwnershipByStatus(t *testing.T) {
	now := time.Now()
	future := now.Add(30 * 24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	tests := []struct {
		name        string
		status      string
		periodEnd   time.Time
		wantIsOwned bool
	}{
		{
			name:        "active かつ期間内",
			status:      domain.SubscriptionStatusActive,
			periodEnd:   future,
			wantIsOwned: true,
		},
		{
			name:        "cancelled だが期間内",
			status:      domain.SubscriptionStatusCancelled,
			periodEnd:   future,
			wantIsOwned: true,
		},
		{
			name:        "grace_period かつ期間内",
			status:      domain.SubscriptionStatusGracePeriod,
			periodEnd:   future,
			wantIsOwned: true,
		},
		{
			name:      "active で期限切れ",
			status:    domain.SubscriptionStatusActive,
			periodEnd: past,
		},
		{
			name:      "cancelled で期限切れ",
			status:    domain.SubscriptionStatusCancelled,
			periodEnd: past,
		},
		{
			name:      "expired は期間内でも無効",
			status:    domain.SubscriptionStatusExpired,
			periodEnd: future,
		},
		{
			name:      "revoked は期間内でも無効",
			status:    domain.SubscriptionStatusRevoked,
			periodEnd: future,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestShopEnv(t)
			insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)
			playerID := fmt.Sprintf("dddddddd-%04d-dddd-dddd-dddddddddddd", i)
			insertSubscription(t, &domain.Subscription{
				PlayerID:           playerID,
				ProductID:          "premium_monthly",
				Status:             tt.status,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   tt.periodEnd,
				CreatedAt:          now,
				UpdatedAt:          now,
			}, domain.PlatformIOS, fmt.Sprintf("sub-token-%d", i))
			products, err := env.svc.GetProducts(context.Background(), playerID)
			require.NoError(t, err)
			require.Len(t, products, 1)
			assert.Equal(t, tt.wantIsOwned, products[0].IsOwned)
		})
	}
}

func TestGetVerifier(t *testing.T) {
	env := newTestShopEnv(t)

	tests := []struct {
		name     string
		platform string
		wantErr  error
	}{
		{
			name:     "iOS",
			platform: "ios",
		},
		{
			name:     "Android",
			platform: "android",
		},
		{
			name:     "未対応 platform",
			platform: "windows",
			wantErr:  ErrUnsupportedPlatform,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := env.svc.getVerifier(tt.platform)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, v)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, v)
		})
	}
}

func TestPurchase_VerifierReturnsError(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return nil, fmt.Errorf("network timeout")
		},
	}))

	insertFactionSetProduct(t, "faction_she", "SHEカードセット", 980, "SHE", true)

	err := env.svc.Purchase(context.Background(), "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", "faction_she", "ios", "receipt-err")
	assert.ErrorIs(t, err, ErrVerifyReceipt)
}

func TestPurchase_SubscriptionTypeViaPurchase(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-sub-via-purchase"}, nil
		},
	}))

	insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)

	err := env.svc.Purchase(context.Background(), "ffffffff-ffff-ffff-ffff-ffffffffffff", "premium_monthly", "ios", "receipt-sub")
	assert.ErrorIs(t, err, ErrUnsupportedProductType)
}

// 同一トークンでの再 Subscribe は既存 CurrentPeriodEnd を返し publish しない。
func TestSubscribe_Idempotent(t *testing.T) {
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
			return &port.SubscriptionInfo{IsValid: true, ProductID: "premium_monthly", ExpiresAt: expiresAt}, nil
		},
	}))
	insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)

	playerID := "22222222-aaaa-bbbb-cccc-dddddddddddd"
	ctx := context.Background()
	first, err := env.svc.Subscribe(ctx, playerID, "premium_monthly", "ios", "sub-token-idem")
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := env.svc.Subscribe(ctx, playerID, "premium_monthly", "ios", "sub-token-idem")
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.WithinDuration(t, *first, *second, time.Second)
	assert.Len(t, selectPremiumUpdatedEvents(t), 1, "publish only on first subscribe")
}

// VerifySubscription が infra error (ネットワーク等) を返した場合のラップ。
func TestSubscribe_VerifierReturnsError(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
			return nil, fmt.Errorf("network timeout")
		},
	}))
	insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)

	_, err := env.svc.Subscribe(context.Background(), "33333333-aaaa-bbbb-cccc-dddddddddddd", "premium_monthly", "ios", "sub-token-err")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify subscription")
	assert.Contains(t, err.Error(), "network timeout")
	assert.Empty(t, selectPremiumUpdatedEvents(t))
}

// GetProducts の cosmetic 所有判定 — item_type/item_no が一致するときのみ IsOwned。
func TestGetProducts_CosmeticOwnership(t *testing.T) {
	tests := []struct {
		name        string
		insertItem  bool
		itemType    string
		itemNo      int
		wantIsOwned bool
	}{
		{
			name:        "item_type と item_no が完全一致で所有",
			insertItem:  true,
			itemType:    "playmat",
			itemNo:      1,
			wantIsOwned: true,
		},
		{
			name: "未所有",
		},
		{
			name:       "item_type 一致・item_no 不一致は未所有扱い",
			insertItem: true,
			itemType:   "playmat",
			itemNo:     99,
		},
		{
			name:       "item_no 一致・item_type 不一致は未所有扱い",
			insertItem: true,
			itemType:   "sleeve",
			itemNo:     1,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestShopEnv(t)
			insertCosmeticProduct(t, "playmat_01", "プレイマット", 320, "playmat", 1, true)

			playerID := fmt.Sprintf("55555555-%04d-bbbb-cccc-dddddddddddd", i)
			if tt.insertItem {
				_, err := sharedPg.Pool.Exec(context.Background(),
					`INSERT INTO shop.cosmetic_items (item_type, item_no, item_name, is_purchasable, is_active) VALUES ($1,$2,'extra',true,true) ON CONFLICT DO NOTHING`,
					tt.itemType, tt.itemNo)
				require.NoError(t, err)
				_, err = sharedPg.Pool.Exec(context.Background(),
					`INSERT INTO shop.player_items (player_id, item_type, item_no, acquired_at) VALUES ($1,$2,$3,now())`,
					playerID, tt.itemType, tt.itemNo)
				require.NoError(t, err)
			}

			products, err := env.svc.GetProducts(context.Background(), playerID)
			require.NoError(t, err)
			require.Len(t, products, 1)
			assert.Equal(t, tt.wantIsOwned, products[0].IsOwned)
		})
	}
}
