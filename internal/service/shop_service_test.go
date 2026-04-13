package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/platform"
	"github.com/kenyamaneko/overload-party-shop/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCardLister struct {
	cards []*apishop.CardView
}

func (f *fakeCardLister) ListAllCards(_ context.Context) ([]*apishop.CardView, error) {
	return f.cards, nil
}

type fakeFactionPublisher struct {
	calls []fakeFactionPubCall
	err   error
}

type fakeFactionPubCall struct {
	PlayerID string
	Faction  string
}

func (f *fakeFactionPublisher) PublishFactionSelected(_ context.Context, playerID, faction string) error {
	f.calls = append(f.calls, fakeFactionPubCall{PlayerID: playerID, Faction: faction})
	return f.err
}

type fakePremiumPublisher struct {
	calls []fakePremiumPubCall
	err   error
}

type fakePremiumPubCall struct {
	PlayerID  string
	IsPremium bool
	ExpiresAt *time.Time
}

func (f *fakePremiumPublisher) PublishPremiumUpdated(_ context.Context, playerID string, isPremium bool, expiresAt *time.Time) error {
	f.calls = append(f.calls, fakePremiumPubCall{PlayerID: playerID, IsPremium: isPremium, ExpiresAt: expiresAt})
	return f.err
}

type testShopEnv struct {
	svc              *ShopService
	shopRepo         *repository.PgShopRepository
	subRepo          *repository.PgSubscriptionRepository
	ownedFactionRepo *repository.PgOwnedFactionRepository
	factionPub       *fakeFactionPublisher
	premiumPub       *fakePremiumPublisher
}

// shopEnvOption は newTestShopEnv に渡す依存差し替えオプション。verifier 等
// テストごとに変えたい依存はここから注入し、ShopService 生成後の field 上書き
// は禁止（DI 経由のみで構築する設計を保つため）。
type shopEnvOption func(*shopEnvDeps)

type shopEnvDeps struct {
	appleVerifier  platform.ReceiptVerifier
	googleVerifier platform.ReceiptVerifier
}

func withAppleVerifier(v platform.ReceiptVerifier) shopEnvOption {
	return func(d *shopEnvDeps) { d.appleVerifier = v }
}

func withGoogleVerifier(v platform.ReceiptVerifier) shopEnvOption {
	return func(d *shopEnvDeps) { d.googleVerifier = v }
}

func newTestShopEnv(t *testing.T, opts ...shopEnvOption) *testShopEnv {
	t.Helper()
	sharedPg.Truncate(t)

	deps := &shopEnvDeps{
		appleVerifier:  &platform.MockReceiptVerifier{},
		googleVerifier: &platform.MockReceiptVerifier{},
	}
	for _, opt := range opts {
		opt(deps)
	}

	shopRepo := repository.NewPgShopRepository(sharedPg.Pool)
	subRepo := repository.NewPgSubscriptionRepository(sharedPg.Pool)
	ownedFactionRepo := repository.NewPgOwnedFactionRepository(sharedPg.Pool)
	txRunner := repository.NewTxManager(sharedPg.Pool)
	factionPub := &fakeFactionPublisher{}
	premiumPub := &fakePremiumPublisher{}

	cards := []*apishop.CardView{
		{CardID: "SH-0001", CardName: "SHE Compute", Faction: "SHE", CardType: "Compute", Restriction: "unlimited", IsActive: true},
		{CardID: "TK-0001", CardName: "Tenki VM", Faction: "Tenki", CardType: "Compute", Restriction: "unlimited", IsActive: true},
	}
	cardLister := &fakeCardLister{cards: cards}

	svc := NewShopService(shopRepo, subRepo, ownedFactionRepo, txRunner, cardLister, deps.appleVerifier, deps.googleVerifier, factionPub, premiumPub)

	return &testShopEnv{
		svc:              svc,
		shopRepo:         shopRepo,
		subRepo:          subRepo,
		ownedFactionRepo: ownedFactionRepo,
		factionPub:       factionPub,
		premiumPub:       premiumPub,
	}
}

// insertProduct はテスト用に商品を shop.products に直接 INSERT する。
// PgShopRepository には商品作成 API がないため SQL で書き込む。
func insertProduct(t *testing.T, p *apishop.Product) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.products (product_id, name, type, price, content, description, image_url, is_active)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ProductID, p.Name, p.Type, p.Price, p.Content, p.Description, p.ImageURL, p.IsActive)
	require.NoError(t, err)
}

func TestPurchase_FactionSet_Success(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-123", ProductID: "faction_tenki"}, nil
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "11111111-1111-1111-1111-111111111111", "faction_tenki", "ios", "receipt-token-1")
	require.NoError(t, err)

	t.Run("publishes faction-selected event", func(t *testing.T) {
		require.Len(t, env.factionPub.calls, 1)
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", env.factionPub.calls[0].PlayerID)
		assert.Equal(t, "Tenki", env.factionPub.calls[0].Faction)
	})

	t.Run("writes shop-local owned faction", func(t *testing.T) {
		factions, err := env.ownedFactionRepo.List(context.Background(), "11111111-1111-1111-1111-111111111111")
		require.NoError(t, err)
		assert.Contains(t, factions, "Tenki")
	})
}

func TestPurchase_Idempotent(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-123"}, nil
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	playerID := "22222222-2222-2222-2222-222222222222"
	ctx := context.Background()
	require.NoError(t, env.svc.Purchase(ctx, playerID, "faction_tenki", "ios", "receipt-token-1"))

	// 同一トークンでの再購入 — べき等
	require.NoError(t, env.svc.Purchase(ctx, playerID, "faction_tenki", "ios", "receipt-token-1"))

	// publish は 1 回のみ (2 回目は既存 token 検出経路で publish 前に return)
	assert.Len(t, env.factionPub.calls, 1)
}

func TestPurchase_ReceiptFailed(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: false}, nil
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "33333333-3333-3333-3333-333333333333", "faction_tenki", "ios", "bad-receipt")
	assert.ErrorIs(t, err, ErrReceiptVerificationFailed)
	assert.Len(t, env.factionPub.calls, 0)
}

func TestPurchase_CosmeticItem(t *testing.T) {
	env := newTestShopEnv(t, withGoogleVerifier(&platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-456"}, nil
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "playmat_01",
		Name:      "プレイマット: サイバー",
		Type:      apishop.ProductTypeCosmetic,
		Price:     320,
		Content:   json.RawMessage(`{"item_type":"playmat","item_no":1}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "44444444-4444-4444-4444-444444444444", "playmat_01", "android", "cosmetic-receipt")
	require.NoError(t, err)
	assert.Len(t, env.factionPub.calls, 0)
}

func TestPurchase_AlreadyOwned_FactionSet(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-first"}, nil
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	playerID := "55555555-5555-5555-5555-555555555555"
	err := env.svc.Purchase(context.Background(), playerID, "faction_tenki", "ios", "receipt-token-1")
	require.NoError(t, err)

	// 別トークンでの再購入は拒否 (ownedFactions 検出)
	err = env.svc.Purchase(context.Background(), playerID, "faction_tenki", "ios", "receipt-token-2")
	assert.ErrorIs(t, err, ErrAlreadyOwned)
}

func TestPurchase_AlreadyOwned_Cosmetic(t *testing.T) {
	env := newTestShopEnv(t, withGoogleVerifier(&platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-cos"}, nil
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "playmat_01",
		Name:      "プレイマット: サイバー",
		Type:      apishop.ProductTypeCosmetic,
		Price:     320,
		Content:   json.RawMessage(`{"item_type":"playmat","item_no":1}`),
		IsActive:  true,
	})

	playerID := "66666666-6666-6666-6666-666666666666"
	err := env.svc.Purchase(context.Background(), playerID, "playmat_01", "android", "cosmetic-receipt-1")
	require.NoError(t, err)

	err = env.svc.Purchase(context.Background(), playerID, "playmat_01", "android", "cosmetic-receipt-2")
	assert.ErrorIs(t, err, ErrAlreadyOwned)
}

func TestPurchase_InactiveProduct(t *testing.T) {
	env := newTestShopEnv(t)

	insertProduct(t, &apishop.Product{
		ProductID: "old_product",
		Name:      "旧商品",
		Type:      apishop.ProductTypeFactionSet,
		Price:     100,
		Content:   json.RawMessage(`{"faction":"SHE"}`),
		IsActive:  false,
	})

	err := env.svc.Purchase(context.Background(), "77777777-7777-7777-7777-777777777777", "old_product", "ios", "receipt-1")
	assert.ErrorIs(t, err, ErrProductNotActive)
}

func TestPurchase_UnsupportedPlatform(t *testing.T) {
	env := newTestShopEnv(t)

	insertProduct(t, &apishop.Product{
		ProductID: "faction_she",
		Name:      "SHEカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"SHE"}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "88888888-8888-8888-8888-888888888888", "faction_she", "windows", "receipt-1")
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)
}

func TestSubscribe_Success(t *testing.T) {
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	env := newTestShopEnv(t, withAppleVerifier(&platform.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*platform.SubscriptionInfo, error) {
			return &platform.SubscriptionInfo{
				IsValid:   true,
				ProductID: "premium_monthly",
				ExpiresAt: expiresAt,
			}, nil
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "premium_monthly",
		Name:      "プレミアム月額",
		Type:      apishop.ProductTypeSubscription,
		Price:     480,
		Content:   json.RawMessage(`{}`),
		IsActive:  true,
	})

	playerID := "99999999-9999-9999-9999-999999999999"
	result, err := env.svc.Subscribe(context.Background(), playerID, "premium_monthly", "ios", "sub-token-1")
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Run("publishes premium-updated event", func(t *testing.T) {
		require.Len(t, env.premiumPub.calls, 1)
		assert.Equal(t, playerID, env.premiumPub.calls[0].PlayerID)
		assert.True(t, env.premiumPub.calls[0].IsPremium)
	})

	t.Run("creates active subscription record", func(t *testing.T) {
		sub, err := env.subRepo.GetLatestSubscription(context.Background(), playerID)
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, apishop.SubscriptionStatusActive, sub.Status)
	})
}

func TestSubscribe_Errors(t *testing.T) {
	tests := []struct {
		name          string
		product       *apishop.Product
		appleVerifier platform.ReceiptVerifier
		productID     string
		platform      string
		token         string
		wantErr       error
	}{
		{
			name: "NotSubscriptionProduct",
			product: &apishop.Product{
				ProductID: "faction_she",
				Name:      "SHEカードセット",
				Type:      apishop.ProductTypeFactionSet,
				Price:     980,
				Content:   json.RawMessage(`{"faction":"SHE"}`),
				IsActive:  true,
			},
			productID: "faction_she",
			platform:  "ios",
			token:     "sub-token-1",
			wantErr:   ErrProductNotSubscription,
		},
		{
			name: "VerificationFailed",
			appleVerifier: &platform.MockReceiptVerifier{
				VerifySubscriptionFn: func(ctx context.Context, token string) (*platform.SubscriptionInfo, error) {
					return &platform.SubscriptionInfo{IsValid: false}, nil
				},
			},
			product: &apishop.Product{
				ProductID: "premium_monthly",
				Name:      "プレミアム月額",
				Type:      apishop.ProductTypeSubscription,
				Price:     480,
				Content:   json.RawMessage(`{}`),
				IsActive:  true,
			},
			productID: "premium_monthly",
			platform:  "ios",
			token:     "bad-sub-token",
			wantErr:   ErrSubVerificationFailed,
		},
		{
			name: "UnsupportedPlatform",
			product: &apishop.Product{
				ProductID: "premium_monthly",
				Name:      "プレミアム月額",
				Type:      apishop.ProductTypeSubscription,
				Price:     480,
				Content:   json.RawMessage(`{}`),
				IsActive:  true,
			},
			productID: "premium_monthly",
			platform:  "windows",
			token:     "sub-token-1",
			wantErr:   ErrUnsupportedPlatform,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []shopEnvOption
			if tt.appleVerifier != nil {
				opts = append(opts, withAppleVerifier(tt.appleVerifier))
			}
			env := newTestShopEnv(t, opts...)
			insertProduct(t, tt.product)

			_, err := env.svc.Subscribe(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", tt.productID, tt.platform, tt.token)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestGetProducts_WithOwnership(t *testing.T) {
	env := newTestShopEnv(t)

	insertProduct(t, &apishop.Product{
		ProductID: "faction_she",
		Name:      "SHEカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"SHE"}`),
		IsActive:  true,
	})
	insertProduct(t, &apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	playerID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	// shop 購入で SHE faction を所有している状態をシミュレート
	require.NoError(t, env.ownedFactionRepo.Add(context.Background(), playerID, "SHE"))

	products, err := env.svc.GetProducts(context.Background(), playerID)
	require.NoError(t, err)
	require.Len(t, products, 2)

	byID := map[string]apishop.ProductResponse{}
	for _, p := range products {
		byID[p.ProductID] = p
	}
	assert.True(t, byID["faction_she"].IsOwned)
	assert.False(t, byID["faction_tenki"].IsOwned)
}

func TestGetProducts_SubscriptionOwnership(t *testing.T) {
	env := newTestShopEnv(t)

	insertProduct(t, &apishop.Product{
		ProductID: "premium_monthly",
		Name:      "プレミアム月額",
		Type:      apishop.ProductTypeSubscription,
		Price:     480,
		Content:   json.RawMessage(`{}`),
		IsActive:  true,
	})

	playerID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	now := time.Now()
	require.NoError(t, env.subRepo.CreateSubscription(context.Background(), &apishop.Subscription{
		PlayerID:           playerID,
		ProductID:          "premium_monthly",
		Status:             apishop.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}, apishop.PlatformIOS, "sub-token"))

	products, err := env.svc.GetProducts(context.Background(), playerID)
	require.NoError(t, err)
	require.Len(t, products, 1)
	assert.True(t, products[0].IsOwned)
}

// 解約済み・支払い猶予中・期限切れ・revoke の各状態で IsOwned が
// 特典有効性に従って判定されることを検証する。
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
		{"active in period", apishop.SubscriptionStatusActive, future, true},
		{"cancelled in period", apishop.SubscriptionStatusCancelled, future, true},
		{"grace in period", apishop.SubscriptionStatusGrace, future, true},
		{"active expired", apishop.SubscriptionStatusActive, past, false},
		{"cancelled expired", apishop.SubscriptionStatusCancelled, past, false},
		{"expired status", apishop.SubscriptionStatusExpired, future, false},
		{"revoked", apishop.SubscriptionStatusRevoked, future, false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestShopEnv(t)
			insertProduct(t, &apishop.Product{
				ProductID: "premium_monthly",
				Name:      "プレミアム月額",
				Type:      apishop.ProductTypeSubscription,
				Price:     480,
				Content:   json.RawMessage(`{}`),
				IsActive:  true,
			})
			playerID := fmt.Sprintf("dddddddd-%04d-dddd-dddd-dddddddddddd", i)
			require.NoError(t, env.subRepo.CreateSubscription(context.Background(), &apishop.Subscription{
				PlayerID:           playerID,
				ProductID:          "premium_monthly",
				Status:             tt.status,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   tt.periodEnd,
				CreatedAt:          now,
				UpdatedAt:          now,
			}, apishop.PlatformIOS, fmt.Sprintf("sub-token-%d", i)))
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
		platform string
		wantNil  bool
	}{
		{"ios", false},
		{"android", false},
		{"windows", true},
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			v := env.svc.getVerifier(tt.platform)
			if tt.wantNil {
				assert.Nil(t, v)
			} else {
				assert.NotNil(t, v)
			}
		})
	}
}

func TestPurchase_VerifierReturnsError(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return nil, fmt.Errorf("network timeout")
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "faction_she",
		Name:      "SHEカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"SHE"}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", "faction_she", "ios", "receipt-err")
	assert.ErrorIs(t, err, ErrVerifyReceipt)
}

func TestPurchase_SubscriptionTypeViaPurchase(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-sub-via-purchase"}, nil
		},
	}))

	insertProduct(t, &apishop.Product{
		ProductID: "premium_monthly",
		Name:      "プレミアム月額",
		Type:      apishop.ProductTypeSubscription,
		Price:     480,
		Content:   json.RawMessage(`{}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "ffffffff-ffff-ffff-ffff-ffffffffffff", "premium_monthly", "ios", "receipt-sub")
	assert.ErrorIs(t, err, ErrUnsupportedProductType)
}
