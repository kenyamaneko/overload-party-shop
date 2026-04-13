package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	shopRepo         *repository.MockShopRepository
	subRepo          *repository.MockSubscriptionRepository
	ownedFactionRepo *repository.MockOwnedFactionRepository
	factionPub       *fakeFactionPublisher
	premiumPub       *fakePremiumPublisher
}

func newTestShopEnv() *testShopEnv {
	shopRepo := repository.NewMockShopRepository()
	subRepo := repository.NewMockSubscriptionRepository()
	ownedFactionRepo := repository.NewMockOwnedFactionRepository()
	factionPub := &fakeFactionPublisher{}
	premiumPub := &fakePremiumPublisher{}

	cards := []*apishop.CardView{
		{CardID: "SH-0001", CardName: "SHE Compute", Faction: "SHE", CardType: "Compute", Restriction: "unlimited", IsActive: true},
		{CardID: "TK-0001", CardName: "Tenki VM", Faction: "Tenki", CardType: "Compute", Restriction: "unlimited", IsActive: true},
	}
	cardLister := &fakeCardLister{cards: cards}

	verifier := &platform.MockReceiptVerifier{}

	svc := NewShopService(shopRepo, subRepo, ownedFactionRepo, &repository.MockTxRunner{}, cardLister, verifier, verifier, factionPub, premiumPub)

	return &testShopEnv{
		svc:              svc,
		shopRepo:         shopRepo,
		subRepo:          subRepo,
		ownedFactionRepo: ownedFactionRepo,
		factionPub:       factionPub,
		premiumPub:       premiumPub,
	}
}


func TestPurchase_FactionSet_Success(t *testing.T) {
	env := newTestShopEnv()

	env.svc.appleVerifier = &platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-123", ProductID: "faction_tenki"}, nil
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "p1", "faction_tenki", "ios", "receipt-token-1")
	require.NoError(t, err)

	t.Run("publishes faction-selected event", func(t *testing.T) {
		require.Len(t, env.factionPub.calls, 1)
		assert.Equal(t, "p1", env.factionPub.calls[0].PlayerID)
		assert.Equal(t, "Tenki", env.factionPub.calls[0].Faction)
	})

	t.Run("writes shop-local owned faction", func(t *testing.T) {
		factions, _ := env.ownedFactionRepo.List(context.Background(), "p1")
		assert.Contains(t, factions, "Tenki")
	})
}

func TestPurchase_Idempotent(t *testing.T) {
	env := newTestShopEnv()

	env.svc.appleVerifier = &platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-123"}, nil
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	ctx := context.Background()
	_ = env.svc.Purchase(ctx, "p1", "faction_tenki", "ios", "receipt-token-1")

	// 同一トークンでの再購入 — べき等
	err := env.svc.Purchase(ctx, "p1", "faction_tenki", "ios", "receipt-token-1")
	require.NoError(t, err)

	// publish は 1 回のみ
	assert.Len(t, env.factionPub.calls, 1)
}

func TestPurchase_ReceiptFailed(t *testing.T) {
	env := newTestShopEnv()

	env.svc.appleVerifier = &platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: false}, nil
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "p1", "faction_tenki", "ios", "bad-receipt")
	assert.ErrorIs(t, err, ErrReceiptVerificationFailed)
	assert.Len(t, env.factionPub.calls, 0)
}

func TestPurchase_CosmeticItem(t *testing.T) {
	env := newTestShopEnv()

	env.svc.googleVerifier = &platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-456"}, nil
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "playmat_01",
		Name:      "プレイマット: サイバー",
		Type:      apishop.ProductTypeCosmetic,
		Price:     320,
		Content:   json.RawMessage(`{"item_type":"playmat","item_no":1}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "p1", "playmat_01", "android", "cosmetic-receipt")
	require.NoError(t, err)
	// cosmetic では faction publish なし
	assert.Len(t, env.factionPub.calls, 0)
}

func TestPurchase_AlreadyOwned_FactionSet(t *testing.T) {
	env := newTestShopEnv()

	env.svc.appleVerifier = &platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-first"}, nil
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	// 初回購入は成功
	err := env.svc.Purchase(context.Background(), "p1", "faction_tenki", "ios", "receipt-token-1")
	require.NoError(t, err)

	// 別トークンでの再購入は拒否
	err = env.svc.Purchase(context.Background(), "p1", "faction_tenki", "ios", "receipt-token-2")
	assert.ErrorIs(t, err, ErrAlreadyOwned)
}

func TestPurchase_AlreadyOwned_Cosmetic(t *testing.T) {
	env := newTestShopEnv()

	env.svc.googleVerifier = &platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-cos"}, nil
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "playmat_01",
		Name:      "プレイマット: サイバー",
		Type:      apishop.ProductTypeCosmetic,
		Price:     320,
		Content:   json.RawMessage(`{"item_type":"playmat","item_no":1}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "p1", "playmat_01", "android", "cosmetic-receipt-1")
	require.NoError(t, err)

	err = env.svc.Purchase(context.Background(), "p1", "playmat_01", "android", "cosmetic-receipt-2")
	assert.ErrorIs(t, err, ErrAlreadyOwned)
}

func TestPurchase_InactiveProduct(t *testing.T) {
	env := newTestShopEnv()

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "old_product",
		Name:      "旧商品",
		Type:      apishop.ProductTypeFactionSet,
		Price:     100,
		Content:   json.RawMessage(`{"faction":"SHE"}`),
		IsActive:  false,
	})

	err := env.svc.Purchase(context.Background(), "p1", "old_product", "ios", "receipt-1")
	assert.ErrorIs(t, err, ErrProductNotActive)
}

func TestPurchase_UnsupportedPlatform(t *testing.T) {
	env := newTestShopEnv()

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "faction_she",
		Name:      "SHEカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"SHE"}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "p1", "faction_she", "windows", "receipt-1")
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)
}


func TestSubscribe_Success(t *testing.T) {
	env := newTestShopEnv()

	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	env.svc.appleVerifier = &platform.MockReceiptVerifier{
		VerifySubscriptionFn: func(ctx context.Context, token string) (*platform.SubscriptionInfo, error) {
			return &platform.SubscriptionInfo{
				IsValid:   true,
				ProductID: "premium_monthly",
				ExpiresAt: expiresAt,
			}, nil
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "premium_monthly",
		Name:      "プレミアム月額",
		Type:      apishop.ProductTypeSubscription,
		Price:     480,
		Content:   json.RawMessage(`{}`),
		IsActive:  true,
	})

	result, err := env.svc.Subscribe(context.Background(), "p1", "premium_monthly", "ios", "sub-token-1")
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Run("publishes premium-updated event", func(t *testing.T) {
		require.Len(t, env.premiumPub.calls, 1)
		assert.Equal(t, "p1", env.premiumPub.calls[0].PlayerID)
		assert.True(t, env.premiumPub.calls[0].IsPremium)
	})

	t.Run("creates active subscription record", func(t *testing.T) {
		sub, _ := env.subRepo.GetLatestSubscription(context.Background(), "p1")
		require.NotNil(t, sub)
		assert.Equal(t, apishop.SubscriptionStatusActive, sub.Status)
	})
}

func TestSubscribe_Errors(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(env *testShopEnv)
		productID string
		platform  string
		token     string
		wantErr   error
	}{
		{
			name: "NotSubscriptionProduct",
			setup: func(env *testShopEnv) {
				env.shopRepo.AddProduct(&apishop.Product{
					ProductID: "faction_she",
					Name:      "SHEカードセット",
					Type:      apishop.ProductTypeFactionSet,
					Price:     980,
					Content:   json.RawMessage(`{"faction":"SHE"}`),
					IsActive:  true,
				})
			},
			productID: "faction_she",
			platform:  "ios",
			token:     "sub-token-1",
			wantErr:   ErrProductNotSubscription,
		},
		{
			name: "VerificationFailed",
			setup: func(env *testShopEnv) {
				env.svc.appleVerifier = &platform.MockReceiptVerifier{
					VerifySubscriptionFn: func(ctx context.Context, token string) (*platform.SubscriptionInfo, error) {
						return &platform.SubscriptionInfo{IsValid: false}, nil
					},
				}
				env.shopRepo.AddProduct(&apishop.Product{
					ProductID: "premium_monthly",
					Name:      "プレミアム月額",
					Type:      apishop.ProductTypeSubscription,
					Price:     480,
					Content:   json.RawMessage(`{}`),
					IsActive:  true,
				})
			},
			productID: "premium_monthly",
			platform:  "ios",
			token:     "bad-sub-token",
			wantErr:   ErrSubVerificationFailed,
		},
		{
			name: "UnsupportedPlatform",
			setup: func(env *testShopEnv) {
				env.shopRepo.AddProduct(&apishop.Product{
					ProductID: "premium_monthly",
					Name:      "プレミアム月額",
					Type:      apishop.ProductTypeSubscription,
					Price:     480,
					Content:   json.RawMessage(`{}`),
					IsActive:  true,
				})
			},
			productID: "premium_monthly",
			platform:  "windows",
			token:     "sub-token-1",
			wantErr:   ErrUnsupportedPlatform,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestShopEnv()
			tt.setup(env)

			_, err := env.svc.Subscribe(context.Background(), "p1", tt.productID, tt.platform, tt.token)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}


func TestGetProducts_WithOwnership(t *testing.T) {
	env := newTestShopEnv()

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "faction_she",
		Name:      "SHEカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"SHE"}`),
		IsActive:  true,
	})
	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "faction_tenki",
		Name:      "Tenkiカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"Tenki"}`),
		IsActive:  true,
	})

	// shop 購入で SHE faction を所有している状態をシミュレート
	_ = env.ownedFactionRepo.Add(context.Background(), "p1", "SHE")

	products, err := env.svc.GetProducts(context.Background(), "p1")
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
	env := newTestShopEnv()

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "premium_monthly",
		Name:      "プレミアム月額",
		Type:      apishop.ProductTypeSubscription,
		Price:     480,
		Content:   json.RawMessage(`{}`),
		IsActive:  true,
	})

	now := time.Now()
	_ = env.subRepo.CreateSubscription(context.Background(), &apishop.Subscription{
		PlayerID:           "p1",
		ProductID:          "premium_monthly",
		Status:             apishop.SubscriptionStatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}, apishop.PlatformIOS, "sub-token")

	products, err := env.svc.GetProducts(context.Background(), "p1")
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestShopEnv()
			env.shopRepo.AddProduct(&apishop.Product{
				ProductID: "premium_monthly",
				Name:      "プレミアム月額",
				Type:      apishop.ProductTypeSubscription,
				Price:     480,
				Content:   json.RawMessage(`{}`),
				IsActive:  true,
			})
			_ = env.subRepo.CreateSubscription(context.Background(), &apishop.Subscription{
				PlayerID:           "p1",
				ProductID:          "premium_monthly",
				Status:             tt.status,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   tt.periodEnd,
				CreatedAt:          now,
				UpdatedAt:          now,
			}, apishop.PlatformIOS, "sub-token")
			products, err := env.svc.GetProducts(context.Background(), "p1")
			require.NoError(t, err)
			require.Len(t, products, 1)
			assert.Equal(t, tt.wantIsOwned, products[0].IsOwned)
		})
	}
}

func TestGetVerifier(t *testing.T) {
	env := newTestShopEnv()

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
	env := newTestShopEnv()

	env.svc.appleVerifier = &platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return nil, fmt.Errorf("network timeout")
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "faction_she",
		Name:      "SHEカードセット",
		Type:      apishop.ProductTypeFactionSet,
		Price:     980,
		Content:   json.RawMessage(`{"faction":"SHE"}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "p1", "faction_she", "ios", "receipt-err")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "verify receipt:"))
}

func TestPurchase_SubscriptionTypeViaPurchase(t *testing.T) {
	env := newTestShopEnv()

	env.svc.appleVerifier = &platform.MockReceiptVerifier{
		VerifyPurchaseFn: func(ctx context.Context, token string) (*platform.VerifyResult, error) {
			return &platform.VerifyResult{IsValid: true, TransactionID: "txn-sub-via-purchase"}, nil
		},
	}

	env.shopRepo.AddProduct(&apishop.Product{
		ProductID: "premium_monthly",
		Name:      "プレミアム月額",
		Type:      apishop.ProductTypeSubscription,
		Price:     480,
		Content:   json.RawMessage(`{}`),
		IsActive:  true,
	})

	err := env.svc.Purchase(context.Background(), "p1", "premium_monthly", "ios", "receipt-sub")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unsupported product type"))
}
