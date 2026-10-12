package routertest

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"

	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
	"github.com/kenyamaneko/overload-party-shop/internal/router"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

// 要求の送り先のパス。
const (
	ProductsPath      = "/api/v1/shop/products"
	PurchasePath      = "/api/v1/shop/purchase"
	SubscribePath     = "/api/v1/shop/subscribe"
	AppleWebhookPath  = "/webhook/apple"
	GoogleWebhookPath = "/webhook/google"
)

// プッシュ認証の設定値。
const (
	PushServiceAccountEmail = "tst-push@tst.example.com"
	PushAudience            = "TST-PUSH-AUDIENCE"
)

// 結合テストのプレイヤー ID。shop の保存先が UUID を要求するため UUID の形で書く。
const (
	PlayerOne = "11111111-1111-4111-8111-111111111111"
	PlayerTwo = "22222222-2222-4222-8222-222222222222"
)

// ShopRoute はショップ API の 1 ルートへの要求。
type ShopRoute struct {
	Name   string
	Method string
	Path   string
	Body   []byte
}

// ShopRoutes はショップ API の 3 ルート。
type ShopRoutes struct {
	ProductList ShopRoute
	Purchase    ShopRoute
	Subscribe   ShopRoute
	All         []ShopRoute
}

// BuildShopRoutes はショップ API の 3 ルートへの要求を組み立てる。
func BuildShopRoutes(t *testing.T) ShopRoutes {
	t.Helper()
	body := BuildPurchaseRequestBody(t, "TST-PRODUCT-1", "ios", "TST-PURCHASE-TOKEN-1")
	routes := ShopRoutes{
		ProductList: ShopRoute{Name: "商品一覧の取得", Method: http.MethodGet, Path: ProductsPath},
		Purchase:    ShopRoute{Name: "商品の購入", Method: http.MethodPost, Path: PurchasePath, Body: body},
		Subscribe:   ShopRoute{Name: "サブスクリプションの購入", Method: http.MethodPost, Path: SubscribePath, Body: body},
	}
	routes.All = []ShopRoute{routes.ProductList, routes.Purchase, routes.Subscribe}
	return routes
}

type routerConfig struct {
	auth         internalauth.Verifier
	service      *ShopService
	apple        *rest.AppleWebhookHandler
	google       *rest.GoogleWebhookHandler
	push         router.PubSubPushTokenValidator
	pushEmail    string
	pushAudience string
}

// RouterOption は偽物だけで組み立てるルーターの構成を変える。
type RouterOption func(*routerConfig)

// WithAuth は内部認証の検証器を auth にする。指定しないときは、どのトークンもプレイヤー ID TST-PLAYER-1 として検証する。
func WithAuth(auth internalauth.Verifier) RouterOption {
	return func(c *routerConfig) { c.auth = auth }
}

// WithService はショップのサービスを service にする。指定しないときは商品なしを返す偽物を使う。
func WithService(service *ShopService) RouterOption {
	return func(c *routerConfig) { c.service = service }
}

// WithApple は Apple 通知の受付を有効にし、通知の処理を notifier にする。
func WithApple(notifier *AppleNotifier) RouterOption {
	return func(c *routerConfig) { c.apple = rest.NewAppleWebhookHandler(notifier) }
}

// WithGoogle は GooglePlay 通知の受付を有効にし、通知の処理を notifier にする。
func WithGoogle(notifier *GoogleNotifier) RouterOption {
	return func(c *routerConfig) { c.google = rest.NewGoogleWebhookHandler(notifier) }
}

// WithPushAuth はプッシュ認証の検証器と、想定するサービスアカウントのメールアドレス、audience を与える。
func WithPushAuth(validator router.PubSubPushTokenValidator, email, audience string) RouterOption {
	return func(c *routerConfig) {
		c.push = validator
		c.pushEmail = email
		c.pushAudience = audience
	}
}

// WithGooglePush は GooglePlay 通知の受付を有効にし、プッシュ認証を validator と audience で行う。
func WithGooglePush(notifier *GoogleNotifier, validator router.PubSubPushTokenValidator, audience string) RouterOption {
	return func(c *routerConfig) {
		WithGoogle(notifier)(c)
		WithPushAuth(validator, PushServiceAccountEmail, audience)(c)
	}
}

// NewRouter は偽物だけでルーターを組み立てる。
func NewRouter(options ...RouterOption) (*gin.Engine, error) {
	config := &routerConfig{
		auth:    NewFixedAuthVerifier("TST-PLAYER-1"),
		service: NewShopService(),
	}
	for _, option := range options {
		option(config)
	}
	return router.New(rest.NewShopHandler(config.service), config.apple, config.google, config.auth, config.push, config.pushEmail, config.pushAudience)
}

// BuildRouter は偽物だけでルーターを組み立て、組み立てに失敗したらテストを失敗させる。
func BuildRouter(t *testing.T, options ...RouterOption) *gin.Engine {
	t.Helper()
	engine, err := NewRouter(options...)
	require.NoError(t, err)
	return engine
}

// Env は実 PostgreSQL と本物の handler・usecase・repository を通し、外部境界だけを偽物にしたルーターの一式。
type Env struct {
	Engine        *gin.Engine
	Auth          *AuthVerifier
	AppleReceipt  *ReceiptVerifier
	GoogleReceipt *ReceiptVerifier
	AppleJWS      *AppleJWSVerifier
	GoogleExpiry  *GoogleExpiryFetcher
	Push          *PushValidator
	pg            *postgrestest.Postgres
}

// BuildEnv は pg の全テーブルを空にして、pg に接続するルーターの一式を組み立てる。内部認証はトークンの値をそのままプレイヤー ID とする。
func BuildEnv(t *testing.T, pg *postgrestest.Postgres) *Env {
	t.Helper()
	pg.Truncate(t)
	env := &Env{
		Auth:          NewEchoAuthVerifier(),
		AppleReceipt:  NewReceiptVerifier(),
		GoogleReceipt: NewReceiptVerifier(),
		AppleJWS:      NewAppleJWSVerifier(),
		GoogleExpiry:  NewGoogleExpiryFetcher(FarFutureExpiry),
		Push:          NewPushValidator(PushServiceAccountEmail, nil),
		pg:            pg,
	}
	env.usePool(t, pg.Pool)
	return env
}

// UseClosedPool は偽物の状態を保ったまま、接続を閉じた接続プールに接続するルーターへ組み直す。
func (e *Env) UseClosedPool(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	dsn, err := e.pg.Container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	pool.Close()
	e.usePool(t, pool)
}

// UseOpenPool は偽物の状態を保ったまま、接続が健全な接続プールに接続するルーターへ組み直す。
func (e *Env) UseOpenPool(t *testing.T) {
	t.Helper()
	e.usePool(t, e.pg.Pool)
}

func (e *Env) usePool(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	productRepo := postgres.NewProductRepository(pool)
	subscriptionRepo := postgres.NewSubscriptionRepository(pool)
	service := purchase.New(
		productRepo,
		postgres.NewFactionPurchaseRepository(pool),
		postgres.NewCardPackPurchaseRepository(pool),
		postgres.NewItemPurchaseRepository(pool),
		postgres.NewPurchaseLookupRepository(pool),
		subscriptionRepo,
		e.AppleReceipt.Port(),
		e.GoogleReceipt.Port(),
	)
	appleNotifier := subscription.NewAppleNotifier(subscriptionRepo, e.AppleJWS.Port(), BundleID)
	googleNotifier := subscription.NewGoogleNotifier(subscriptionRepo, e.GoogleExpiry)
	engine, err := router.New(
		rest.NewShopHandler(service),
		rest.NewAppleWebhookHandler(appleNotifier),
		rest.NewGoogleWebhookHandler(googleNotifier),
		e.Auth,
		e.Push,
		PushServiceAccountEmail,
		PushAudience,
	)
	require.NoError(t, err)
	e.Engine = engine
}

// CountRows は table (スキーマ名つき) の行数を返す。
func (e *Env) CountRows(t *testing.T, table string) int {
	t.Helper()
	return e.pg.CountRows(t, table)
}

// ListProducts はプレイヤー playerID の商品一覧を取得する。
func (e *Env) ListProducts(playerID string) Response {
	return Send(e.Engine, http.MethodGet, ProductsPath, nil, buildAuthHeader(playerID))
}

// PostPurchase はプレイヤー playerID として商品の購入に本文 body を送る。
func (e *Env) PostPurchase(playerID string, body []byte) Response {
	return Send(e.Engine, http.MethodPost, PurchasePath, body, buildAuthHeader(playerID))
}

// PostSubscribe はプレイヤー playerID としてサブスクリプションの購入に本文 body を送る。
func (e *Env) PostSubscribe(playerID string, body []byte) Response {
	return Send(e.Engine, http.MethodPost, SubscribePath, body, buildAuthHeader(playerID))
}

// Purchase はプレイヤー playerID として商品を購入する。
func (e *Env) Purchase(t *testing.T, playerID, productID, platform, purchaseToken string) Response {
	t.Helper()
	return e.PostPurchase(playerID, BuildPurchaseRequestBody(t, productID, platform, purchaseToken))
}

// Subscribe はプレイヤー playerID としてサブスクリプションを購入する。
func (e *Env) Subscribe(t *testing.T, playerID, productID, platform, purchaseToken string) Response {
	t.Helper()
	return e.PostSubscribe(playerID, BuildPurchaseRequestBody(t, productID, platform, purchaseToken))
}

// PostAppleWebhook は Apple 通知の受付に署名付きデータ signedPayload を送る。
func (e *Env) PostAppleWebhook(t *testing.T, signedPayload string) Response {
	t.Helper()
	return e.PostAppleWebhookBody(BuildAppleWebhookBody(t, signedPayload))
}

// PostAppleWebhookBody は Apple 通知の受付に本文 body を送る。
func (e *Env) PostAppleWebhookBody(body []byte) Response {
	return Send(e.Engine, http.MethodPost, AppleWebhookPath, body, nil)
}

// PostGoogleWebhook は GooglePlay 通知の受付に通知の data を送る。
func (e *Env) PostGoogleWebhook(t *testing.T, data string) Response {
	t.Helper()
	return e.PostGoogleWebhookBody(BuildGoogleWebhookBody(t, data))
}

// PostGoogleWebhookBody は GooglePlay 通知の受付に本文 body を送る。
func (e *Env) PostGoogleWebhookBody(body []byte) Response {
	return Send(e.Engine, http.MethodPost, GoogleWebhookPath, body, BuildGooglePushHeaders())
}

func buildAuthHeader(playerID string) map[string]string {
	return map[string]string{internalauth.HeaderName: playerID}
}
