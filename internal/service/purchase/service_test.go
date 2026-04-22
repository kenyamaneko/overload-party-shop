package purchase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFactionBuilder は BuildFactionPurchased 呼び出しを記録するテスト用ダブル。
// 既存テストが期待する .calls スライスのインタフェースを維持するため、
// OutboxEventBuilder の「faction-purchased 側」を担当する小さな型として切り分けている。
type fakeFactionBuilder struct {
	calls []fakeFactionPubCall
	err   error
}

type fakeFactionPubCall struct {
	PlayerID string
	Faction  string
}

func (f *fakeFactionBuilder) Build(playerID, faction string) (port.OutboxEvent, error) {
	f.calls = append(f.calls, fakeFactionPubCall{PlayerID: playerID, Faction: faction})
	if f.err != nil {
		return port.OutboxEvent{}, f.err
	}
	return port.OutboxEvent{EventID: uuid.New(), Topic: apishop.TopicFactionPurchased, Payload: []byte(`{}`)}, nil
}

// fakePremiumBuilder は BuildPremiumUpdated 呼び出しを記録するテスト用ダブル。
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

// fakeEventBuilder は OutboxEventBuilder を満たし、内部に faction/premium の
// サブビルダーを保持する。テストは env.factionPub.calls / env.premiumPub.calls で
// 旧来の publisher テストと同じ観点の観察を行える。
type fakeEventBuilder struct {
	factionPub *fakeFactionBuilder
	premiumPub *fakePremiumBuilder
}

func newFakeEventBuilder(factionErr, premiumErr error) *fakeEventBuilder {
	return &fakeEventBuilder{
		factionPub: &fakeFactionBuilder{err: factionErr},
		premiumPub: &fakePremiumBuilder{err: premiumErr},
	}
}

func (f *fakeEventBuilder) BuildFactionPurchased(playerID, faction string) (port.OutboxEvent, error) {
	return f.factionPub.Build(playerID, faction)
}

func (f *fakeEventBuilder) BuildPremiumUpdated(playerID string, isPremium bool, expiresAt *time.Time) (port.OutboxEvent, error) {
	return f.premiumPub.Build(playerID, isPremium, expiresAt)
}

type testShopEnv struct {
	svc                 *Service
	productRepo         *postgres.ProductRepository
	factionPurchaseRepo *postgres.FactionPurchaseRepository
	itemPurchaseRepo    *postgres.ItemPurchaseRepository
	purchaseLookup      *postgres.PurchaseLookupRepository
	subRepo             *postgres.SubscriptionRepository
	// factionPub / premiumPub は旧 publisher 時代の観測 API を踏襲した
	// builder 側のスパイ。テストは env.factionPub.calls / env.premiumPub.calls で
	// 「どのイベントが enqueue 要求されたか」を検証する。
	factionPub *fakeFactionBuilder
	premiumPub *fakePremiumBuilder
}

// shopEnvOption は newTestShopEnv に渡す依存差し替えオプション。verifier 等
// テストごとに変えたい依存はここから注入し、Service 生成後の field 上書き
// は禁止（DI 経由のみで構築する設計を保つため）。
type shopEnvOption func(*shopEnvDeps)

type shopEnvDeps struct {
	appleVerifier  port.ReceiptVerifier
	googleVerifier port.ReceiptVerifier
	factionPubErr  error
	premiumPubErr  error
}

func withAppleVerifier(v port.ReceiptVerifier) shopEnvOption {
	return func(d *shopEnvDeps) { d.appleVerifier = v }
}

func withGoogleVerifier(v port.ReceiptVerifier) shopEnvOption {
	return func(d *shopEnvDeps) { d.googleVerifier = v }
}

func withFactionPubErr(err error) shopEnvOption {
	return func(d *shopEnvDeps) { d.factionPubErr = err }
}

func withPremiumPubErr(err error) shopEnvOption {
	return func(d *shopEnvDeps) { d.premiumPubErr = err }
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
	builder := newFakeEventBuilder(deps.factionPubErr, deps.premiumPubErr)

	svc := New(productRepo, factionPurchaseRepo, itemPurchaseRepo, purchaseLookup, subRepo, deps.appleVerifier, deps.googleVerifier, builder)

	return &testShopEnv{
		svc:                 svc,
		productRepo:         productRepo,
		factionPurchaseRepo: factionPurchaseRepo,
		itemPurchaseRepo:    itemPurchaseRepo,
		purchaseLookup:      purchaseLookup,
		subRepo:             subRepo,
		factionPub:          builder.factionPub,
		premiumPub:          builder.premiumPub,
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
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-123", ProductID: "faction_tenki"}, nil
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

	t.Run("publishes faction-purchased event", func(t *testing.T) {
		require.Len(t, env.factionPub.calls, 1)
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", env.factionPub.calls[0].PlayerID)
		assert.Equal(t, "Tenki", env.factionPub.calls[0].Faction)
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
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: false}, nil
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
	env := newTestShopEnv(t, withGoogleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-456"}, nil
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
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-first"}, nil
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
	env := newTestShopEnv(t, withGoogleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-cos"}, nil
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
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
			return &port.SubscriptionInfo{
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
	// verifier を呼ばない経路のケースでも DI の形を揃えるため default を明示する。
	defaultVerifier := &port.MockReceiptVerifier{}
	invalidSubVerifier := &port.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
			return &port.SubscriptionInfo{IsValid: false}, nil
		},
	}

	tests := []struct {
		name          string
		product       *apishop.Product
		appleVerifier port.ReceiptVerifier
		productID     string
		platform      string
		token         string
		wantErr       error
	}{
		{
			name: "サブスク以外の商品を Subscribe",
			product: &apishop.Product{
				ProductID: "faction_she",
				Name:      "SHEカードセット",
				Type:      apishop.ProductTypeFactionSet,
				Price:     980,
				Content:   json.RawMessage(`{"faction":"SHE"}`),
				IsActive:  true,
			},
			appleVerifier: defaultVerifier,
			productID:     "faction_she",
			platform:      "ios",
			token:         "sub-token-1",
			wantErr:       ErrProductNotSubscription,
		},
		{
			name: "レシート検証失敗（IsValid=false）",
			product: &apishop.Product{
				ProductID: "premium_monthly",
				Name:      "プレミアム月額",
				Type:      apishop.ProductTypeSubscription,
				Price:     480,
				Content:   json.RawMessage(`{}`),
				IsActive:  true,
			},
			appleVerifier: invalidSubVerifier,
			productID:     "premium_monthly",
			platform:      "ios",
			token:         "bad-sub-token",
			wantErr:       ErrSubVerificationFailed,
		},
		{
			name: "未対応 platform",
			product: &apishop.Product{
				ProductID: "premium_monthly",
				Name:      "プレミアム月額",
				Type:      apishop.ProductTypeSubscription,
				Price:     480,
				Content:   json.RawMessage(`{}`),
				IsActive:  true,
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
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.player_owned_factions (player_id, faction) VALUES ($1, $2)`,
		playerID, "SHE")
	require.NoError(t, err)

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
	}, apishop.PlatformIOS, "sub-token", port.OutboxEvent{}))

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
		{
			name:        "active かつ期間内",
			status:      apishop.SubscriptionStatusActive,
			periodEnd:   future,
			wantIsOwned: true,
		},
		{
			name:        "cancelled だが期間内",
			status:      apishop.SubscriptionStatusCancelled,
			periodEnd:   future,
			wantIsOwned: true,
		},
		{
			name:        "grace_period かつ期間内",
			status:      apishop.SubscriptionStatusGracePeriod,
			periodEnd:   future,
			wantIsOwned: true,
		},
		{
			name:      "active で期限切れ",
			status:    apishop.SubscriptionStatusActive,
			periodEnd: past,
		},
		{
			name:      "cancelled で期限切れ",
			status:    apishop.SubscriptionStatusCancelled,
			periodEnd: past,
		},
		{
			name:      "expired は期間内でも無効",
			status:    apishop.SubscriptionStatusExpired,
			periodEnd: future,
		},
		{
			name:      "revoked は期間内でも無効",
			status:    apishop.SubscriptionStatusRevoked,
			periodEnd: future,
		},
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
			}, apishop.PlatformIOS, fmt.Sprintf("sub-token-%d", i), port.OutboxEvent{}))
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
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
			return &port.VerifyResult{IsValid: true, TransactionID: "txn-sub-via-purchase"}, nil
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

// Purchase の defensive 分岐 — verifier 到達前に弾かれる入力検証エラー。
// 型 / 内容バリデーションが repo 層を呼ぶ前に return することを確認する。
func TestPurchase_DefensivePaths(t *testing.T) {
	tests := []struct {
		name     string
		product  *apishop.Product
		wantErr  error
		wantSubs string
	}{
		{
			name: "選択不可 faction が content に入っている",
			product: &apishop.Product{
				ProductID: "faction_unknown",
				Name:      "未知 faction",
				Type:      apishop.ProductTypeFactionSet,
				Price:     980,
				Content:   json.RawMessage(`{"faction":"NotARealFaction"}`),
				IsActive:  true,
			},
			wantErr: ErrInvalidFaction,
		},
		{
			name: "faction content の JSON 型不一致",
			product: &apishop.Product{
				ProductID: "faction_broken",
				Name:      "壊れた faction",
				Type:      apishop.ProductTypeFactionSet,
				Price:     980,
				Content:   json.RawMessage(`{"faction":123}`),
				IsActive:  true,
			},
			wantSubs: "parse faction set content",
		},
		{
			name: "cosmetic content の JSON 型不一致",
			product: &apishop.Product{
				ProductID: "cosmetic_broken",
				Name:      "壊れた cosmetic",
				Type:      apishop.ProductTypeCosmetic,
				Price:     320,
				Content:   json.RawMessage(`{"item_type":123,"item_no":1}`),
				IsActive:  true,
			},
			wantSubs: "parse cosmetic content",
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestShopEnv(t)
			insertProduct(t, tt.product)

			playerID := fmt.Sprintf("eeeeeeee-%04d-eeee-eeee-eeeeeeeeeeee", i)
			err := env.svc.Purchase(context.Background(), playerID, tt.product.ProductID, "ios", fmt.Sprintf("token-%d", i))
			require.Error(t, err)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			if tt.wantSubs != "" {
				assert.Contains(t, err.Error(), tt.wantSubs)
			}
			assert.Empty(t, env.factionPub.calls, "no publish on defensive failure")
		})
	}
}

// Purchase 後の event builder 経路 — ビルダーエラー時は tx ごと rollback され
// 購入行も outbox 行も残らない (dual-write 問題を避けるための atomic 契約)。
func TestPurchase_FactionPublisherPaths(t *testing.T) {
	tests := []struct {
		name          string
		opts          []shopEnvOption
		wantErr       bool
		wantErrSubs   string
		wantCalls     int
		wantFactionDB bool
	}{
		{
			// EventBuilder が失敗するケース。購入行 + outbox 行が同一 tx なので
			// ビルダーエラーは tx 開始前に露見し、購入行も書かれない。
			name:          "event builder がエラーを返す（購入行も残らない）",
			opts:          []shopEnvOption{withFactionPubErr(fmt.Errorf("build failed"))},
			wantErr:       true,
			wantErrSubs:   "build faction-purchased",
			wantCalls:     1,
			wantFactionDB: false,
		},
		{
			// 正常系: builder 成功 → 購入行 + outbox 行が同一 tx で書かれる。
			name:          "event builder が成功（購入行が永続化される）",
			opts:          nil,
			wantErr:       false,
			wantCalls:     1,
			wantFactionDB: true,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]shopEnvOption{
				withAppleVerifier(&port.MockReceiptVerifier{
					VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
						return &port.VerifyResult{IsValid: true, TransactionID: "txn-pub"}, nil
					},
				}),
			}, tt.opts...)
			env := newTestShopEnv(t, opts...)
			insertProduct(t, &apishop.Product{
				ProductID: "faction_tenki",
				Name:      "Tenki",
				Type:      apishop.ProductTypeFactionSet,
				Price:     980,
				Content:   json.RawMessage(`{"faction":"Tenki"}`),
				IsActive:  true,
			})

			playerID := fmt.Sprintf("11111111-%04d-2222-3333-444444444444", i)
			err := env.svc.Purchase(context.Background(), playerID, "faction_tenki", "ios", fmt.Sprintf("pub-token-%d", i))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSubs)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, env.factionPub.calls, tt.wantCalls)

			factions, ferr := env.factionPurchaseRepo.ListOwnedFactions(context.Background(), playerID)
			require.NoError(t, ferr)
			if tt.wantFactionDB {
				assert.Contains(t, factions, "Tenki")
			} else {
				assert.NotContains(t, factions, "Tenki", "builder 失敗時は購入行も rollback される")
			}
		})
	}
}

// 同一トークンでの再 Subscribe は既存 CurrentPeriodEnd を返し publish しない。
func TestSubscribe_Idempotent(t *testing.T) {
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
			return &port.SubscriptionInfo{IsValid: true, ProductID: "premium_monthly", ExpiresAt: expiresAt}, nil
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

	playerID := "22222222-aaaa-bbbb-cccc-dddddddddddd"
	ctx := context.Background()
	first, err := env.svc.Subscribe(ctx, playerID, "premium_monthly", "ios", "sub-token-idem")
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := env.svc.Subscribe(ctx, playerID, "premium_monthly", "ios", "sub-token-idem")
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.WithinDuration(t, *first, *second, time.Second)
	assert.Len(t, env.premiumPub.calls, 1, "publish only on first subscribe")
}

// VerifySubscription が infra error (ネットワーク等) を返した場合のラップ。
func TestSubscribe_VerifierReturnsError(t *testing.T) {
	env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
			return nil, fmt.Errorf("network timeout")
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

	_, err := env.svc.Subscribe(context.Background(), "33333333-aaaa-bbbb-cccc-dddddddddddd", "premium_monthly", "ios", "sub-token-err")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify subscription")
	assert.Contains(t, err.Error(), "network timeout")
	assert.Empty(t, env.premiumPub.calls)
}

// Subscribe 後の event builder 経路 — ビルダーエラー時は subscription 行も残らない
// (同一 tx で outbox と atomic なため)。
func TestSubscribe_PremiumPublisherPaths(t *testing.T) {
	tests := []struct {
		name        string
		opts        []shopEnvOption
		wantErr     bool
		wantErrSubs string
		wantCalls   int
		wantSubDB   bool
	}{
		{
			name:        "event builder がエラーを返す（フロー失敗、subscription 行も残らない）",
			opts:        []shopEnvOption{withPremiumPubErr(fmt.Errorf("build failed"))},
			wantErr:     true,
			wantErrSubs: "build premium-updated",
			wantCalls:   1,
			wantSubDB:   false,
		},
		{
			name:      "event builder が成功（subscription 行が永続化される）",
			opts:      nil,
			wantErr:   false,
			wantCalls: 1,
			wantSubDB: true,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiresAt := time.Now().Add(30 * 24 * time.Hour)
			opts := append([]shopEnvOption{
				withAppleVerifier(&port.MockReceiptVerifier{
					VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
						return &port.SubscriptionInfo{IsValid: true, ProductID: "premium_monthly", ExpiresAt: expiresAt}, nil
					},
				}),
			}, tt.opts...)
			env := newTestShopEnv(t, opts...)
			insertProduct(t, &apishop.Product{
				ProductID: "premium_monthly",
				Name:      "プレミアム月額",
				Type:      apishop.ProductTypeSubscription,
				Price:     480,
				Content:   json.RawMessage(`{}`),
				IsActive:  true,
			})

			playerID := fmt.Sprintf("44444444-%04d-bbbb-cccc-dddddddddddd", i)
			_, err := env.svc.Subscribe(context.Background(), playerID, "premium_monthly", "ios", fmt.Sprintf("sub-pub-%d", i))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSubs)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, env.premiumPub.calls, tt.wantCalls)

			sub, ferr := env.subRepo.GetLatestSubscription(context.Background(), playerID)
			require.NoError(t, ferr)
			if tt.wantSubDB {
				require.NotNil(t, sub)
				assert.Equal(t, apishop.SubscriptionStatusActive, sub.Status)
			} else {
				assert.Nil(t, sub, "builder 失敗時は subscription 行も rollback される")
			}
		})
	}
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
			insertProduct(t, &apishop.Product{
				ProductID: "playmat_01",
				Name:      "プレイマット",
				Type:      apishop.ProductTypeCosmetic,
				Price:     320,
				Content:   json.RawMessage(`{"item_type":"playmat","item_no":1}`),
				IsActive:  true,
			})

			playerID := fmt.Sprintf("55555555-%04d-bbbb-cccc-dddddddddddd", i)
			if tt.insertItem {
				_, err := sharedPg.Pool.Exec(context.Background(),
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

// GetProducts は壊れた content JSON を error として伝播する（log だけで握りつぶさない）。
// DB 列が json 型のため構文エラーは insert 時点で弾かれる。ここでは「JSON としては
// valid だが struct フィールド型と不一致」のケースを検証する。
func TestGetProducts_MalformedContent(t *testing.T) {
	tests := []struct {
		name        string
		productType string
		content     json.RawMessage
	}{
		{
			name:        "faction_set で faction の JSON 型不一致",
			productType: apishop.ProductTypeFactionSet,
			content:     json.RawMessage(`{"faction":123}`),
		},
		{
			name:        "cosmetic で item_type の JSON 型不一致",
			productType: apishop.ProductTypeCosmetic,
			content:     json.RawMessage(`{"item_type":123,"item_no":1}`),
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestShopEnv(t)
			insertProduct(t, &apishop.Product{
				ProductID: fmt.Sprintf("broken_%d", i),
				Name:      "broken",
				Type:      tt.productType,
				Price:     100,
				Content:   tt.content,
				IsActive:  true,
			})

			playerID := fmt.Sprintf("66666666-%04d-bbbb-cccc-dddddddddddd", i)
			_, err := env.svc.GetProducts(context.Background(), playerID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "parse product content for")
		})
	}
}
