package purchasetest

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase"
)

// テストで使うダミーのプレイヤー・購入トークン・商品・カードパック・陣営。
const (
	PlayerID      = "00000000-0000-4000-8000-000000000001"
	PurchaseToken = "TST-PURCHASE-TOKEN-1"

	FactionSetProductID    = "TST-FACTION-SET-1"
	FactionSetFaction      = "SHE"
	FactionSetCardPackID   = "TST-CARD-PACK-ID-1"
	CardPackProductID      = "TST-CARD-PACK-1"
	CardPackID             = "TST-CARD-PACK-ID-2"
	OtherCardPackProductID = "TST-CARD-PACK-2"
	OtherCardPackID        = "TST-CARD-PACK-ID-3"
	CosmeticProductID      = "TST-COSMETIC-1"
	InactiveProductID      = "TST-INACTIVE-1"
	SubscriptionProductID  = "TST-SUBSCRIPTION-1"
	MissingProductID       = "TST-MISSING-1"
)

// 購入と登録に指定する platform。
const (
	PlatformIOS         = "ios"
	PlatformAndroid     = "android"
	PlatformUnsupported = "windows"
)

// アウトボックスに保存されるイベントの種別。
const (
	EventTypeCardPackPurchased = "card_pack_purchased"
	EventTypeFactionAcquired   = "faction_acquired"
	EventTypePremiumUpdated    = "premium_updated"
)

// StoreUnreachableMessage は、ストアに接続できないときに偽のストアが返す通信エラーのメッセージ。
const StoreUnreachableMessage = "TST-store-unreachable"

var (
	// ErrStoreUnreachable は、ストアに接続できないときに偽のストアが返す通信エラー。
	ErrStoreUnreachable = errors.New(StoreUnreachableMessage)
	// StoreExpiresAt は、有効なレシートに対して偽のストアが返す有効期限。
	StoreExpiresAt = time.Date(2030, 1, 15, 8, 0, 0, 0, time.UTC)
)

// Env は、実 PostgreSQL のリポジトリと偽の Apple・Google のストアに結線した購入サービス。
type Env struct {
	Service *purchase.Service
	Apple   *StoreVerifier
	Google  *StoreVerifier
}

// NewEnv は、全テーブルを空にした DB に結線した購入サービスを返す。
func NewEnv(t *testing.T, pg *postgrestest.Postgres) Env {
	t.Helper()
	pg.Truncate(t)
	apple := NewStoreVerifier()
	google := NewStoreVerifier()
	return Env{Service: NewService(pg.Pool, apple, google), Apple: apple, Google: google}
}

// NewPurchaseEnv は、NewEnv の環境に SeedPurchaseCatalog の商品を投入して返す。
func NewPurchaseEnv(t *testing.T, pg *postgrestest.Postgres) Env {
	t.Helper()
	env := NewEnv(t, pg)
	SeedPurchaseCatalog(t, pg)
	return env
}

// SeedPurchaseCatalog は、販売中の faction_set・card_pack 2 種・cosmetic・subscription の商品と、販売停止中の card_pack 商品を投入する。
func SeedPurchaseCatalog(t *testing.T, pg *postgrestest.Postgres) {
	t.Helper()
	pg.InsertFactionSetProduct(t, ActiveSeed(FactionSetProductID), FactionSetFaction, FactionSetCardPackID)
	pg.InsertCardPackProduct(t, ActiveSeed(CardPackProductID), CardPackID)
	pg.InsertCardPackProduct(t, ActiveSeed(OtherCardPackProductID), OtherCardPackID)
	pg.InsertCosmeticProduct(t, ActiveSeed(CosmeticProductID), "playmat", 1)
	pg.InsertCardPackProduct(t, postgrestest.ProductSeed{ProductID: InactiveProductID, Name: "TST-inactive", Price: 100, IsActive: false}, "TST-CARD-PACK-ID-9")
	pg.InsertSubscriptionProduct(t, ActiveSeed(SubscriptionProductID), 1)
}

// ActiveSeed は、販売中の商品 productID の投入内容を返す。
func ActiveSeed(productID string) postgrestest.ProductSeed {
	return postgrestest.ProductSeed{ProductID: productID, Name: "TST-" + productID, Price: 100, IsActive: true}
}

// AcceptReceiptFor は、PurchaseToken のレシートを商品 productID の有効なものと判定させる設定を返す。
func AcceptReceiptFor(productID string) func(*StoreVerifier) {
	return func(verifier *StoreVerifier) {
		verifier.AcceptToken(PurchaseToken, productID, StoreExpiresAt)
	}
}

// RejectReceipt は、PurchaseToken のレシートを無効と判定させる。
func RejectReceipt(verifier *StoreVerifier) {
	verifier.RejectToken(PurchaseToken)
}

// FailReceipt は、PurchaseToken の検証でストアに接続できないようにする。
func FailReceipt(verifier *StoreVerifier) {
	verifier.FailToken(PurchaseToken, ErrStoreUnreachable)
}

// FailedOperation は、失敗する購入または登録の1通り。Name は失敗の内容を「…して失敗したあと」の形で表す。
type FailedOperation struct {
	Name           string
	ProductID      string
	Platform       string
	ArrangeReceipt func(*StoreVerifier)
}

// SingleEvent は、イベントがちょうど1件であることを確かめて、その1件を返す。
func SingleEvent(t *testing.T, events []OutboxEvent) OutboxEvent {
	t.Helper()
	require.Len(t, events, 1)
	return events[0]
}
