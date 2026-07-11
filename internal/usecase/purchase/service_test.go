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
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testShopEnv struct {
	svc                  *Service
	productRepo          *postgres.ProductRepository
	factionPurchaseRepo  *postgres.FactionPurchaseRepository
	cardPackPurchaseRepo *postgres.CardPackPurchaseRepository
	itemPurchaseRepo     *postgres.ItemPurchaseRepository
	purchaseLookup       *postgres.PurchaseLookupRepository
	subRepo              *postgres.SubscriptionRepository
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
	cardPackPurchaseRepo := postgres.NewCardPackPurchaseRepository(sharedPg.Pool)
	itemPurchaseRepo := postgres.NewItemPurchaseRepository(sharedPg.Pool)
	purchaseLookup := postgres.NewPurchaseLookupRepository(sharedPg.Pool)
	subRepo := postgres.NewSubscriptionRepository(sharedPg.Pool)

	svc := New(
		productRepo, factionPurchaseRepo, cardPackPurchaseRepo, itemPurchaseRepo, purchaseLookup, subRepo,
		deps.appleVerifier, deps.googleVerifier,
	)

	return &testShopEnv{
		svc:                  svc,
		productRepo:          productRepo,
		factionPurchaseRepo:  factionPurchaseRepo,
		cardPackPurchaseRepo: cardPackPurchaseRepo,
		itemPurchaseRepo:     itemPurchaseRepo,
		purchaseLookup:       purchaseLookup,
		subRepo:              subRepo,
	}
}

func selectCardPackPurchasedEvents(t *testing.T) []apishop.CardPackPurchasedEvent {
	t.Helper()
	rows, err := sharedPg.Pool.Query(context.Background(),
		`SELECT payload FROM shop.outbox_events WHERE event_type = $1 ORDER BY created_at`,
		apishop.EventTypeCardPackPurchased)
	require.NoError(t, err)
	defer rows.Close()
	var events []apishop.CardPackPurchasedEvent
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var ev apishop.CardPackPurchasedEvent
		require.NoError(t, json.Unmarshal(payload, &ev))
		events = append(events, ev)
	}
	require.NoError(t, rows.Err())
	return events
}

func selectFactionAcquiredEvents(t *testing.T) []apishop.FactionAcquiredEvent {
	t.Helper()
	rows, err := sharedPg.Pool.Query(context.Background(),
		`SELECT payload FROM shop.outbox_events WHERE event_type = $1 ORDER BY created_at`,
		apishop.EventTypeFactionAcquired)
	require.NoError(t, err)
	defer rows.Close()
	var events []apishop.FactionAcquiredEvent
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var ev apishop.FactionAcquiredEvent
		require.NoError(t, json.Unmarshal(payload, &ev))
		events = append(events, ev)
	}
	require.NoError(t, rows.Err())
	return events
}

func selectPremiumUpdatedEvents(t *testing.T) []apishop.PremiumUpdatedEvent {
	t.Helper()
	rows, err := sharedPg.Pool.Query(context.Background(),
		`SELECT payload FROM shop.outbox_events WHERE event_type = $1 ORDER BY created_at`,
		apishop.EventTypePremiumUpdated)
	require.NoError(t, err)
	defer rows.Close()
	var events []apishop.PremiumUpdatedEvent
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var ev apishop.PremiumUpdatedEvent
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

// insertFactionSetProduct は faction_set 商品 (products + product_faction + product_card_pack) を seed する。
// card_pack_id は "faction_set_<faction>" の規約で生成する。
func insertFactionSetProduct(t *testing.T, productID, name string, price int64, faction string, isActive bool) {
	t.Helper()
	insertCommonProduct(t, productID, name, domain.ProductTypeFactionSet, price, isActive)
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.product_faction (product_id, faction) VALUES ($1,$2)`,
		productID, faction)
	require.NoError(t, err)
	cardPackID := "faction_set_" + faction
	_, err = sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.product_card_pack (product_id, card_pack_id) VALUES ($1,$2)`,
		productID, cardPackID)
	require.NoError(t, err)
}

// insertCardPackProduct は card_pack 商品 (products + product_card_pack) を seed する。
func insertCardPackProduct(t *testing.T, productID, name string, price int64, cardPackID string, isActive bool) {
	t.Helper()
	insertCommonProduct(t, productID, name, domain.ProductTypeCardPack, price, isActive)
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.product_card_pack (product_id, card_pack_id) VALUES ($1,$2)`,
		productID, cardPackID)
	require.NoError(t, err)
}

// insertCosmeticProduct は cosmetic 商品 (products + cosmetic_items + product_cosmetic) を seed する。
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
		`INSERT INTO shop.product_cosmetic (product_id, item_type, item_no) VALUES ($1,$2,$3)`,
		productID, itemType, itemNo)
	require.NoError(t, err)
}

// insertSubscriptionProduct は subscription 商品 (products + product_subscription) を seed する。
// 既存テストでは課金周期を 1 か月固定で扱う。
func insertSubscriptionProduct(t *testing.T, productID, name string, price int64, isActive bool) {
	t.Helper()
	insertCommonProduct(t, productID, name, domain.ProductTypeSubscription, price, isActive)
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO shop.product_subscription (product_id, period_months) VALUES ($1, $2)`,
		productID, 1)
	require.NoError(t, err)
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

func TestPurchase(t *testing.T) {
	t.Run("購入", func(t *testing.T) {
		t.Run("faction_set 商品を購入したとき", func(t *testing.T) {
			env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
				VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
					return &port.VerifyResult{IsValid: true, TransactionID: "txn-123", ProductID: "faction_tenki"}, nil
				},
			}))
			insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

			playerID := "11111111-1111-1111-1111-111111111111"
			require.NoError(t, env.svc.Purchase(context.Background(), playerID, "faction_tenki", "ios", "receipt-token-1"))

			t.Run("card-pack-purchased event が publish される", func(t *testing.T) {
				events := selectCardPackPurchasedEvents(t)
				require.Len(t, events, 1)
				assert.Equal(t, playerID, events[0].PlayerID)
				assert.Equal(t, "faction_set_Tenki", events[0].CardPackID)
			})

			t.Run("faction-acquired event が publish される", func(t *testing.T) {
				events := selectFactionAcquiredEvents(t)
				require.Len(t, events, 1)
				assert.Equal(t, playerID, events[0].PlayerID)
				assert.Equal(t, "Tenki", events[0].Faction)
			})

			t.Run("shop 側に owned faction が記録される", func(t *testing.T) {
				factions, err := env.factionPurchaseRepo.ListOwnedFactions(context.Background(), playerID)
				require.NoError(t, err)
				assert.Contains(t, factions, "Tenki")
			})

			t.Run("shop 側に owned card pack が記録される", func(t *testing.T) {
				owned, err := env.cardPackPurchaseRepo.HasPlayerCardPack(context.Background(), playerID, "faction_set_Tenki")
				require.NoError(t, err)
				assert.True(t, owned)
			})
		})

		t.Run("同一トークンで再購入しても、card-pack-purchased と faction-acquired は各 1 回だけ publish される (冪等)", func(t *testing.T) {
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
			assert.Len(t, selectCardPackPurchasedEvents(t), 1)
			assert.Len(t, selectFactionAcquiredEvents(t), 1)
		})

		t.Run("レシート検証が IsValid=false のとき、ErrReceiptVerificationFailed になり何も publish されない", func(t *testing.T) {
			env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
				VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
					return &port.VerifyResult{IsValid: false}, nil
				},
			}))
			insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

			err := env.svc.Purchase(context.Background(), "33333333-3333-3333-3333-333333333333", "faction_tenki", "ios", "bad-receipt")
			assert.ErrorIs(t, err, ErrReceiptVerificationFailed)
			assert.Empty(t, selectCardPackPurchasedEvents(t))
			assert.Empty(t, selectFactionAcquiredEvents(t))
		})

		t.Run("card_pack 商品 (faction を伴わない pure pack) を購入したとき", func(t *testing.T) {
			env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
				VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
					return &port.VerifyResult{IsValid: true, TransactionID: "txn-limited"}, nil
				},
			}))
			insertCardPackProduct(t, "limited_2026_summer", "限定: 2026 夏パック", 480, "limited_2026_summer", true)

			playerID := "77777777-7777-7777-7777-aaaaaaaaaaaa"
			ctx := context.Background()
			require.NoError(t, env.svc.Purchase(ctx, playerID, "limited_2026_summer", "ios", "receipt-limited-1"))

			t.Run("card-pack-purchased のみ publish され faction-acquired は出ない", func(t *testing.T) {
				card := selectCardPackPurchasedEvents(t)
				require.Len(t, card, 1)
				assert.Equal(t, playerID, card[0].PlayerID)
				assert.Equal(t, "limited_2026_summer", card[0].CardPackID)
				assert.Empty(t, selectFactionAcquiredEvents(t))
			})

			// 再購入禁止は player_owned_card_packs で担保する
			t.Run("同じ card_pack を再購入すると ErrAlreadyOwned で拒否される", func(t *testing.T) {
				err := env.svc.Purchase(ctx, playerID, "limited_2026_summer", "ios", "receipt-limited-2")
				assert.ErrorIs(t, err, ErrAlreadyOwned)
			})
		})

		t.Run("cosmetic 商品を購入すると、card-pack-purchased も faction-acquired も publish されない", func(t *testing.T) {
			env := newTestShopEnv(t, withGoogleVerifier(&port.MockReceiptVerifier{
				VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
					return &port.VerifyResult{IsValid: true, TransactionID: "txn-456"}, nil
				},
			}))
			insertCosmeticProduct(t, "playmat_01", "プレイマット: サイバー", 320, "playmat", 1, true)

			err := env.svc.Purchase(context.Background(), "44444444-4444-4444-4444-444444444444", "playmat_01", "android", "cosmetic-receipt")
			require.NoError(t, err)
			assert.Empty(t, selectCardPackPurchasedEvents(t))
			assert.Empty(t, selectFactionAcquiredEvents(t))
		})

		t.Run("faction_set を所有済みで別トークン再購入すると、ErrAlreadyOwned になる", func(t *testing.T) {
			env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
				VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
					return &port.VerifyResult{IsValid: true, TransactionID: "txn-first"}, nil
				},
			}))
			insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

			playerID := "55555555-5555-5555-5555-555555555555"
			require.NoError(t, env.svc.Purchase(context.Background(), playerID, "faction_tenki", "ios", "receipt-token-1"))

			// 別トークンでの再購入は拒否 (ownedFactions 検出)
			err := env.svc.Purchase(context.Background(), playerID, "faction_tenki", "ios", "receipt-token-2")
			assert.ErrorIs(t, err, ErrAlreadyOwned)
		})

		t.Run("cosmetic を所有済みで再購入すると、ErrAlreadyOwned になる", func(t *testing.T) {
			env := newTestShopEnv(t, withGoogleVerifier(&port.MockReceiptVerifier{
				VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
					return &port.VerifyResult{IsValid: true, TransactionID: "txn-cos"}, nil
				},
			}))
			insertCosmeticProduct(t, "playmat_01", "プレイマット: サイバー", 320, "playmat", 1, true)

			playerID := "66666666-6666-6666-6666-666666666666"
			require.NoError(t, env.svc.Purchase(context.Background(), playerID, "playmat_01", "android", "cosmetic-receipt-1"))

			err := env.svc.Purchase(context.Background(), playerID, "playmat_01", "android", "cosmetic-receipt-2")
			assert.ErrorIs(t, err, ErrAlreadyOwned)
		})

		invalidCases := []struct {
			name      string
			arrange   func(t *testing.T) *testShopEnv
			playerID  string
			productID string
			platform  string
			token     string
			wantErr   error
		}{
			{
				name: "非アクティブ商品のとき、ErrProductNotActive になる",
				arrange: func(t *testing.T) *testShopEnv {
					env := newTestShopEnv(t)
					insertFactionSetProduct(t, "old_product", "旧商品", 100, "SHE", false)
					return env
				},
				playerID:  "77777777-7777-7777-7777-777777777777",
				productID: "old_product",
				platform:  "ios",
				token:     "receipt-1",
				wantErr:   ErrProductNotActive,
			},
			{
				name: "未対応 platform のとき、ErrUnsupportedPlatform になる",
				arrange: func(t *testing.T) *testShopEnv {
					env := newTestShopEnv(t)
					insertFactionSetProduct(t, "faction_she", "SHEカードセット", 980, "SHE", true)
					return env
				},
				playerID:  "88888888-8888-8888-8888-888888888888",
				productID: "faction_she",
				platform:  "windows",
				token:     "receipt-1",
				wantErr:   ErrUnsupportedPlatform,
			},
			{
				name: "verifier が infra error を返すとき、ErrVerifyReceipt になる",
				arrange: func(t *testing.T) *testShopEnv {
					env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
						VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
							return nil, fmt.Errorf("network timeout")
						},
					}))
					insertFactionSetProduct(t, "faction_she", "SHEカードセット", 980, "SHE", true)
					return env
				},
				playerID:  "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
				productID: "faction_she",
				platform:  "ios",
				token:     "receipt-err",
				wantErr:   ErrVerifyReceipt,
			},
			{
				name: "subscription 商品を Purchase 経由で買うと、ErrUnsupportedProductType になる",
				arrange: func(t *testing.T) *testShopEnv {
					env := newTestShopEnv(t, withAppleVerifier(&port.MockReceiptVerifier{
						VerifyPurchaseFn: func(ctx context.Context, token string) (*port.VerifyResult, error) {
							return &port.VerifyResult{IsValid: true, TransactionID: "txn-sub-via-purchase"}, nil
						},
					}))
					insertSubscriptionProduct(t, "premium_monthly", "プレミアム月額", 480, true)
					return env
				},
				playerID:  "ffffffff-ffff-ffff-ffff-ffffffffffff",
				productID: "premium_monthly",
				platform:  "ios",
				token:     "receipt-sub",
				wantErr:   ErrUnsupportedProductType,
			},
		}
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				env := tc.arrange(t)
				err := env.svc.Purchase(context.Background(), tc.playerID, tc.productID, tc.platform, tc.token)
				assert.ErrorIs(t, err, tc.wantErr)
			})
		}
	})
}

func TestSubscribe(t *testing.T) {
	t.Run("サブスクリプション登録", func(t *testing.T) {
		t.Run("サブスク登録に成功したとき", func(t *testing.T) {
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

			t.Run("premium-updated event が publish される", func(t *testing.T) {
				events := selectPremiumUpdatedEvents(t)
				require.Len(t, events, 1)
				assert.Equal(t, playerID, events[0].PlayerID)
				assert.True(t, events[0].IsPremium)
			})

			t.Run("active な subscription 行が作成される", func(t *testing.T) {
				sub, err := env.subRepo.GetLatestSubscription(context.Background(), playerID)
				require.NoError(t, err)
				require.NotNil(t, sub)
				assert.Equal(t, domain.SubscriptionStatusActive, sub.Status)
			})
		})

		t.Run("同一トークンで再 Subscribe しても、premium-updated は 1 回のみ publish され既存の期限を返す (冪等)", func(t *testing.T) {
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

			// 同一トークンでの再 Subscribe は既存 CurrentPeriodEnd を返し publish しない
			second, err := env.svc.Subscribe(ctx, playerID, "premium_monthly", "ios", "sub-token-idem")
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.WithinDuration(t, *first, *second, time.Second)
			assert.Len(t, selectPremiumUpdatedEvents(t), 1, "publish only on first subscribe")
		})

		t.Run("VerifySubscription が infra error を返すと、verify subscription でラップされ premium-updated は publish されない", func(t *testing.T) {
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
		})

		// verifier を呼ばない経路のケースでも DI の形を揃えるため default を明示する。
		defaultVerifier := &port.MockReceiptVerifier{}
		invalidSubVerifier := &port.MockReceiptVerifier{
			VerifySubscriptionFn: func(ctx context.Context, token string) (*port.SubscriptionInfo, error) {
				return &port.SubscriptionInfo{IsValid: false}, nil
			},
		}

		invalidCases := []struct {
			name          string
			seedProduct   func(t *testing.T)
			appleVerifier port.ReceiptVerifier
			productID     string
			platform      string
			token         string
			wantErr       error
		}{
			{
				name: "サブスク以外の商品のとき、ErrProductNotSubscription になる",
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
				name: "レシート検証が IsValid=false のとき、ErrSubVerificationFailed になる",
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
				name: "未対応 platform のとき、ErrUnsupportedPlatform になる",
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
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				env := newTestShopEnv(t, withAppleVerifier(tc.appleVerifier))
				tc.seedProduct(t)

				_, err := env.svc.Subscribe(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", tc.productID, tc.platform, tc.token)
				assert.ErrorIs(t, err, tc.wantErr)
			})
		}
	})
}

func TestGetProducts(t *testing.T) {
	t.Run("商品一覧の所有判定", func(t *testing.T) {
		t.Run("faction を所有しているとき、その faction_set 商品だけ IsOwned=true になる", func(t *testing.T) {
			env := newTestShopEnv(t)
			insertFactionSetProduct(t, "faction_she", "SHEカードセット", 980, "SHE", true)
			insertFactionSetProduct(t, "faction_tenki", "Tenkiカードセット", 980, "Tenki", true)

			playerID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
			// shop 購入で SHE faction を所有している状態をシミュレート
			// (faction_set 商品の IsOwned 判定は player_owned_card_packs 経由)
			_, err := sharedPg.Pool.Exec(context.Background(),
				`INSERT INTO shop.player_owned_factions (player_id, faction) VALUES ($1, $2)`,
				playerID, "SHE")
			require.NoError(t, err)
			_, err = sharedPg.Pool.Exec(context.Background(),
				`INSERT INTO shop.player_owned_card_packs (player_id, card_pack_id) VALUES ($1, $2)`,
				playerID, "faction_set_SHE")
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
		})

		t.Run("active な subscription があるとき、subscription 商品が IsOwned=true になる", func(t *testing.T) {
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
		})

		now := time.Now()
		future := now.Add(30 * 24 * time.Hour)
		past := now.Add(-24 * time.Hour)

		statusCases := []struct {
			name        string
			status      string
			periodEnd   time.Time
			wantIsOwned bool
		}{
			{
				name:        "active かつ期間内のとき、IsOwned=true になる",
				status:      domain.SubscriptionStatusActive,
				periodEnd:   future,
				wantIsOwned: true,
			},
			{
				name:        "cancelled でも期間内のとき、IsOwned=true になる",
				status:      domain.SubscriptionStatusCancelled,
				periodEnd:   future,
				wantIsOwned: true,
			},
			{
				name:        "grace_period かつ期間内のとき、IsOwned=true になる",
				status:      domain.SubscriptionStatusGracePeriod,
				periodEnd:   future,
				wantIsOwned: true,
			},
			{
				name:      "active でも期限切れのとき、IsOwned=false になる",
				status:    domain.SubscriptionStatusActive,
				periodEnd: past,
			},
			{
				name:      "cancelled で期限切れのとき、IsOwned=false になる",
				status:    domain.SubscriptionStatusCancelled,
				periodEnd: past,
			},
			{
				name:      "expired は期間内でも IsOwned=false になる",
				status:    domain.SubscriptionStatusExpired,
				periodEnd: future,
			},
			{
				name:      "revoked は期間内でも IsOwned=false になる",
				status:    domain.SubscriptionStatusRevoked,
				periodEnd: future,
			},
		}
		for i, tt := range statusCases {
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

		seedOwnedCosmetic := func(itemType string, itemNo int) func(t *testing.T, playerID string) {
			return func(t *testing.T, playerID string) {
				_, err := sharedPg.Pool.Exec(context.Background(),
					`INSERT INTO shop.cosmetic_items (item_type, item_no, item_name, is_purchasable, is_active) VALUES ($1,$2,'extra',true,true) ON CONFLICT DO NOTHING`,
					itemType, itemNo)
				require.NoError(t, err)
				_, err = sharedPg.Pool.Exec(context.Background(),
					`INSERT INTO shop.player_items (player_id, item_type, item_no, acquired_at) VALUES ($1,$2,$3,now())`,
					playerID, itemType, itemNo)
				require.NoError(t, err)
			}
		}

		cosmeticCases := []struct {
			name        string
			seedItem    func(t *testing.T, playerID string)
			wantIsOwned bool
		}{
			{
				name:        "item_type と item_no が完全一致のとき、IsOwned=true になる",
				seedItem:    seedOwnedCosmetic("playmat", 1),
				wantIsOwned: true,
			},
			{
				name:        "所有アイテムが無いとき、IsOwned=false になる",
				seedItem:    func(_ *testing.T, _ string) {},
				wantIsOwned: false,
			},
			{
				name:        "item_type 一致・item_no 不一致のとき、IsOwned=false になる",
				seedItem:    seedOwnedCosmetic("playmat", 99),
				wantIsOwned: false,
			},
			{
				name:        "item_no 一致・item_type 不一致のとき、IsOwned=false になる",
				seedItem:    seedOwnedCosmetic("sleeve", 1),
				wantIsOwned: false,
			},
		}
		for i, tt := range cosmeticCases {
			t.Run(tt.name, func(t *testing.T) {
				env := newTestShopEnv(t)
				insertCosmeticProduct(t, "playmat_01", "プレイマット", 320, "playmat", 1, true)

				playerID := fmt.Sprintf("55555555-%04d-bbbb-cccc-dddddddddddd", i)
				tt.seedItem(t, playerID)

				products, err := env.svc.GetProducts(context.Background(), playerID)
				require.NoError(t, err)
				require.Len(t, products, 1)
				assert.Equal(t, tt.wantIsOwned, products[0].IsOwned)
			})
		}
	})
}
