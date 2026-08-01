package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	shopadapter "github.com/kenyamaneko/overload-party-shop/internal/adapter/apple"
	googleadapter "github.com/kenyamaneko/overload-party-shop/internal/adapter/google"
	localadapter "github.com/kenyamaneko/overload-party-shop/internal/adapter/local"
	shoppubsub "github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-shop/internal/config"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/worker"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	shopfirestore "github.com/kenyamaneko/overload-party-shop/internal/repository/firestore"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-shop/internal/router"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/outbox"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/purchase"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

func main() {
	if err := run(); err != nil {
		slog.Error("shop fatal", "error", err)
		os.Exit(1)
	}
}

// setupLogger は IAP_MODE に応じてグローバル slog ロガーを初期化する。
func setupLogger(mode config.IAPMode) error {
	switch mode {
	case config.IAPModeProduction:
		slog.SetDefault(slog.New(newCloudLoggingHandler()).With("service", "shop"))
	case config.IAPModeLocal:
		h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		slog.SetDefault(slog.New(h).With("service", "shop"))
	default:
		return fmt.Errorf("unexpected IAP_MODE: %s", mode)
	}
	return nil
}

// newCloudLoggingHandler は Cloud Logging 互換の JSON ハンドラを返す。
func newCloudLoggingHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				a.Key = "severity"
				if level, ok := a.Value.Any().(slog.Level); ok {
					switch {
					case level >= slog.LevelError:
						a.Value = slog.StringValue("ERROR")
					case level >= slog.LevelWarn:
						a.Value = slog.StringValue("WARNING")
					case level >= slog.LevelInfo:
						a.Value = slog.StringValue("INFO")
					default:
						a.Value = slog.StringValue("DEBUG")
					}
				}
			}
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	})
}

// verifiers は IAP_MODE=production のときだけ初期化される verifier をまとめる。
type verifiers struct {
	apple     port.ReceiptVerifier
	google    port.ReceiptVerifier
	googleSub port.GoogleSubVerifier
	appleJWS  port.AppleJWSVerifier
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	if err := setupLogger(cfg.IAPMode); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, closeDatabasePool, err := newDatabasePool(ctx, cfg)
	if err != nil {
		return fmt.Errorf("new database pool: %w", err)
	}
	defer closeDatabasePool()
	defer pool.Close()

	fsClient, err := firestore.NewClient(ctx, cfg.GoogleCloudProject)
	if err != nil {
		return fmt.Errorf("firestore new client: %w", err)
	}
	defer func() { _ = fsClient.Close() }()
	// game_config は現在 shop の runtime パスから参照していない。
	// クライアント到達性は起動時に検証するため、repo を生成だけしておく。
	_ = shopfirestore.NewGameConfigRepository(fsClient)

	pub, err := shoppubsub.New(ctx, cfg.GoogleCloudProject, cfg.CardPackPurchasedTopic, cfg.FactionAcquiredTopic, cfg.PremiumUpdatedTopic)
	if err != nil {
		return fmt.Errorf("shop publisher: %w", err)
	}
	defer func() {
		if cerr := pub.Close(); cerr != nil {
			slog.Error("publisher close failed", "error", cerr)
		}
	}()

	verifierSet, err := setupVerifiers(ctx, cfg)
	if err != nil {
		return err
	}

	outboxTicker, err := buildOutboxTicker(pool, pub, cfg)
	if err != nil {
		return err
	}

	handler, err := buildHTTPHandler(cfg, pool, verifierSet)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("listening",
		"addr", srv.Addr,
		"google_cloud_project", cfg.GoogleCloudProject,
		"card_pack_purchased_topic", cfg.CardPackPurchasedTopic,
		"faction_acquired_topic", cfg.FactionAcquiredTopic,
		"premium_topic", cfg.PremiumUpdatedTopic,
	)

	return runHTTPAndWorker(ctx, srv, outboxTicker)
}

// setupVerifiers は IAP_MODE に応じて Apple/Google verifier を初期化する。
func setupVerifiers(ctx context.Context, cfg *config.Config) (verifiers, error) {
	if cfg.IAPMode != config.IAPModeProduction {
		slog.Info("using local IAP verifier; webhook routes not registered", "iap_mode", "local")
		localVerifier := localadapter.NewVerifier()
		return verifiers{apple: localVerifier, google: localVerifier}, nil
	}
	ajws := shopadapter.NewJWSVerifier()
	av, err := shopadapter.NewVerifierFromPEM(
		cfg.AppleKeyID, cfg.AppleIssuerID, cfg.AppleBundleID,
		cfg.ApplePrivateKeyPEM, cfg.AppleEnvironment, ajws,
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
	return verifiers{apple: av, google: gv, googleSub: gsv, appleJWS: ajws}, nil
}

// buildHTTPHandler は repo / usecase / handler の配線を組み立てる。
func buildHTTPHandler(cfg *config.Config, pool *pgxpool.Pool, verifierSet verifiers) (http.Handler, error) {
	productRepo := postgres.NewProductRepository(pool)
	factionPurchaseRepo := postgres.NewFactionPurchaseRepository(pool)
	cardPackPurchaseRepo := postgres.NewCardPackPurchaseRepository(pool)
	itemPurchaseRepo := postgres.NewItemPurchaseRepository(pool)
	purchaseLookup := postgres.NewPurchaseLookupRepository(pool)
	subscriptionRepo := postgres.NewSubscriptionRepository(pool)

	shopSvc := purchase.New(
		productRepo, factionPurchaseRepo, cardPackPurchaseRepo, itemPurchaseRepo, purchaseLookup,
		subscriptionRepo,
		verifierSet.apple, verifierSet.google,
	)
	shopHandler := rest.NewShopHandler(shopSvc)
	var (
		appleWebhookHandler  *rest.AppleWebhookHandler
		googleWebhookHandler *rest.GoogleWebhookHandler
	)
	if cfg.IAPMode == config.IAPModeProduction {
		appleWebhookHandler = rest.NewAppleWebhookHandler(
			subscription.NewAppleNotifier(subscriptionRepo, verifierSet.appleJWS),
		)
		googleWebhookHandler = rest.NewGoogleWebhookHandler(
			subscription.NewGoogleNotifier(subscriptionRepo, verifierSet.googleSub),
		)
	}
	internalAuthKey, err := internalauth.ParsePublicKeyPEM([]byte(cfg.InternalAuthPublicKey))
	if err != nil {
		return nil, fmt.Errorf("INTERNAL_AUTH_PUBLIC_KEY is invalid: %w", err)
	}
	authVerifier := internalauth.NewVerifier(
		internalauth.StaticPublicKeyResolver(internalAuthKey, internalauth.DefaultKeyID),
	)
	return router.New(shopHandler, appleWebhookHandler, googleWebhookHandler, authVerifier), nil
}

// buildOutboxTicker は outbox 消費フローの relay と worker ticker を組み立てる。
func buildOutboxTicker(pool *pgxpool.Pool, pub port.RawEventPublisher, cfg *config.Config) (*worker.OutboxTicker, error) {
	outboxRepo := postgres.NewOutboxRepository(pool)
	relay, err := outbox.New(outboxRepo, pub, outbox.Config{
		BatchSize:         cfg.OutboxBatchSize,
		FailureThreshold:  cfg.OutboxFailureThreshold,
		VisibilityTimeout: cfg.OutboxVisibilityTimeout,
	})
	if err != nil {
		return nil, err
	}
	return worker.NewOutboxTicker(relay, cfg.OutboxPollInterval)
}

// runHTTPAndWorker は HTTP server と outbox ticker を並行起動し、どちらかの失敗・signal で両方を graceful に停止する。
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
		slog.Info("shutdown requested")
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
