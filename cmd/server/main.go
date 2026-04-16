package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/kenyamaneko/overload-party-shop/internal/config"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	shopadapter "github.com/kenyamaneko/overload-party-shop/internal/adapter/apple"
	googleadapter "github.com/kenyamaneko/overload-party-shop/internal/adapter/google"
	shoppubsub "github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/worker"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	shopfirestore "github.com/kenyamaneko/overload-party-shop/internal/repository/firestore"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-shop/internal/router"
	"github.com/kenyamaneko/overload-party-shop/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("shop: %v", err)
	}
}

// nilCardLister は CardLister interface を空結果で満たす no-op 実装。
// shop はカード付与を行わないが、interface 充足のため保持。
type nilCardLister struct{}

func (nilCardLister) ListAllCards(_ context.Context) ([]*apishop.CardView, error) {
	return nil, nil
}

// verifiers は IAP_MODE=production のときだけ初期化される 3 種の verifier をまとめる。
// local モードでは全て nil のまま返り、shop_service 側で ErrUnsupportedPlatform を返す。
type verifiers struct {
	apple     port.ReceiptVerifier
	google    port.ReceiptVerifier
	googleSub port.GoogleSubVerifier
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseConn)
	if err != nil {
		return fmt.Errorf("pgxpool new: %w", err)
	}
	defer pool.Close()

	fsClient, err := firestore.NewClient(ctx, cfg.GoogleCloudProject)
	if err != nil {
		return fmt.Errorf("firestore new client: %w", err)
	}
	defer func() { _ = fsClient.Close() }()
	// game_config は現在 shop の runtime パスから参照していない。
	// クライアント到達性は起動時に検証するため、repo を生成だけしておく。
	_ = shopfirestore.NewGameConfigRepository(fsClient)

	pub, err := shoppubsub.New(ctx, cfg.GoogleCloudProject, cfg.FactionSelectedTopic, cfg.PremiumUpdatedTopic)
	if err != nil {
		return fmt.Errorf("shop publisher: %w", err)
	}
	defer func() {
		if cerr := pub.Close(); cerr != nil {
			log.Printf("shop: publisher close failed: %v", cerr)
		}
	}()

	eventBuilder, err := shoppubsub.NewEventBuilder(cfg.FactionSelectedTopic, cfg.PremiumUpdatedTopic)
	if err != nil {
		return fmt.Errorf("shop event builder: %w", err)
	}

	vfs, err := setupVerifiers(ctx, cfg)
	if err != nil {
		return err
	}

	outboxTicker, err := buildOutboxTicker(pool, pub, cfg)
	if err != nil {
		return err
	}

	handler := buildHTTPHandler(cfg, pool, eventBuilder, vfs)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("shop: listening on %s (gcp project=%s faction-topic=%s premium-topic=%s)",
		srv.Addr, cfg.GoogleCloudProject, cfg.FactionSelectedTopic, cfg.PremiumUpdatedTopic)

	return runHTTPAndWorker(ctx, srv, outboxTicker)
}

// setupVerifiers は IAP_MODE に応じて Apple/Google verifier を初期化する。
// local モードでは全 verifier を nil のまま返し、webhook ルートも未登録となる。
func setupVerifiers(ctx context.Context, cfg *config.Config) (verifiers, error) {
	if cfg.IAPMode != config.IAPModeProduction {
		log.Printf("shop: IAP_MODE=local — skipping Apple/Google verifier init and webhook route registration")
		return verifiers{}, nil
	}
	av, err := shopadapter.NewVerifierFromPEM(
		cfg.AppleKeyID, cfg.AppleIssuerID, cfg.AppleBundleID,
		cfg.ApplePrivateKeyPEM, cfg.AppleEnvironment,
	)
	if err != nil {
		return verifiers{}, fmt.Errorf("apple verifier: %w", err)
	}
	gv, err := googleadapter.NewVerifier(ctx, cfg.GooglePackageName)
	if err != nil {
		return verifiers{}, fmt.Errorf("google verifier: %w", err)
	}
	gsv, err := googleadapter.NewSubVerifier(ctx, cfg.GooglePackageName)
	if err != nil {
		return verifiers{}, fmt.Errorf("google sub verifier: %w", err)
	}
	return verifiers{apple: av, google: gv, googleSub: gsv}, nil
}

// buildHTTPHandler は repo / service / handler の配線を一箇所にまとめる。
// run() の肥大を避けるための分割であり、起動順には依存していない。
func buildHTTPHandler(cfg *config.Config, pool *pgxpool.Pool, eventBuilder port.OutboxEventBuilder, vfs verifiers) http.Handler {
	productRepo := postgres.NewProductRepository(pool)
	factionPurchaseRepo := postgres.NewFactionPurchaseRepository(pool)
	itemPurchaseRepo := postgres.NewItemPurchaseRepository(pool)
	purchaseLookup := postgres.NewPurchaseLookupRepository(pool)
	subRepo := postgres.NewSubscriptionRepository(pool)

	shopSvc := service.NewShopService(
		productRepo, factionPurchaseRepo, itemPurchaseRepo, purchaseLookup,
		subRepo, nilCardLister{},
		vfs.apple, vfs.google,
		eventBuilder,
	)
	subSvc := service.NewSubscriptionService(subRepo, eventBuilder, vfs.googleSub)

	shopH := rest.NewShopHandler(shopSvc)
	var webhookH *rest.WebhookHandler
	if cfg.IAPMode == config.IAPModeProduction {
		webhookH = rest.NewWebhookHandler(subSvc)
	}
	return router.New(shopH, webhookH)
}

// buildOutboxTicker は outbox 消費フローを構成する 2 コンポーネント (service の
// use case + handler/worker の ticker) を組み立てる。依存方向は worker → service → port。
func buildOutboxTicker(pool *pgxpool.Pool, pub port.RawEventPublisher, cfg *config.Config) (*worker.OutboxTicker, error) {
	outboxRepo := postgres.NewOutboxRepository(pool)
	publisher, err := service.NewOutboxPublisher(outboxRepo, pub, service.OutboxPublisherConfig{
		BatchSize:         cfg.OutboxBatchSize,
		FailureThreshold:  cfg.OutboxFailureThreshold,
		VisibilityTimeout: cfg.OutboxVisibilityTimeout,
	})
	if err != nil {
		return nil, err
	}
	return worker.NewOutboxTicker(publisher, cfg.OutboxPollInterval)
}

// runHTTPAndWorker は HTTP server と outbox ticker を並行起動し、
// どちらかの失敗・シグナル到来で両方を graceful に停止する。
// errgroup を使うのは「どちらが先に終わっても相方を畳む」ためで、shop 独自の
// 停止順序 (http 先・worker 後) が必要になった場合はここを変える。
func runHTTPAndWorker(ctx context.Context, srv *http.Server, outboxTicker *worker.OutboxTicker) error {
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		return outboxTicker.Run(gCtx)
	})

	g.Go(func() error {
		<-gCtx.Done()
		log.Printf("shop: shutdown requested")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}
