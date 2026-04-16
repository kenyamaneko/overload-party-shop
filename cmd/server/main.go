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

	"github.com/kenyamaneko/overload-party-shop/internal/config"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	shopadapter "github.com/kenyamaneko/overload-party-shop/internal/adapter/apple"
	googleadapter "github.com/kenyamaneko/overload-party-shop/internal/adapter/google"
	shoppubsub "github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
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
// Pub/Sub リファクタ以降 shop はカード付与を行わないが、interface 充足のため保持。
type nilCardLister struct{}

func (nilCardLister) ListAllCards(_ context.Context) ([]*apishop.CardView, error) {
	return nil, nil
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

	productRepo := postgres.NewProductRepository(pool)
	factionPurchaseRepo := postgres.NewFactionPurchaseRepository(pool)
	itemPurchaseRepo := postgres.NewItemPurchaseRepository(pool)
	purchaseLookup := postgres.NewPurchaseLookupRepository(pool)
	subRepo := postgres.NewSubscriptionRepository(pool)
	// game_config は現在 shop の runtime パスから参照していない。
	// クライアント到達性は起動時に検証するため、repo を生成だけしておく。
	_ = shopfirestore.NewGameConfigRepository(fsClient)

	// Pub/Sub publisher（faction-selected + premium-updated）。
	pub, err := shoppubsub.New(ctx, cfg.GoogleCloudProject, cfg.FactionSelectedTopic, cfg.PremiumUpdatedTopic)
	if err != nil {
		return fmt.Errorf("shop publisher: %w", err)
	}
	defer func() {
		if cerr := pub.Close(); cerr != nil {
			log.Printf("shop: publisher close failed: %v", cerr)
		}
	}()

	var (
		appleVerifier     port.ReceiptVerifier
		googleVerifier    port.ReceiptVerifier
		googleSubVerifier port.GoogleSubVerifier
	)

	if cfg.IAPMode == config.IAPModeProduction {
		av, err := shopadapter.NewVerifierFromPEM(
			cfg.AppleKeyID, cfg.AppleIssuerID, cfg.AppleBundleID,
			cfg.ApplePrivateKeyPEM, cfg.AppleEnvironment,
		)
		if err != nil {
			return fmt.Errorf("apple verifier: %w", err)
		}
		appleVerifier = av

		gv, err := googleadapter.NewVerifier(ctx, cfg.GooglePackageName)
		if err != nil {
			return fmt.Errorf("google verifier: %w", err)
		}
		googleVerifier = gv

		gsv, err := googleadapter.NewSubVerifier(ctx, cfg.GooglePackageName)
		if err != nil {
			return fmt.Errorf("google sub verifier: %w", err)
		}
		googleSubVerifier = gsv
	} else {
		log.Printf("shop: IAP_MODE=local — skipping Apple/Google verifier init and webhook route registration")
	}

	shopSvc := service.NewShopService(
		productRepo, factionPurchaseRepo, itemPurchaseRepo, purchaseLookup,
		subRepo, nilCardLister{},
		appleVerifier, googleVerifier,
		pub, pub,
	)
	subSvc := service.NewSubscriptionService(subRepo, pub, googleSubVerifier)

	shopH := rest.NewShopHandler(shopSvc)

	var webhookH *rest.WebhookHandler
	if cfg.IAPMode == config.IAPModeProduction {
		webhookH = rest.NewWebhookHandler(subSvc)
	}

	r := router.New(shopH, webhookH)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("shop: listening on %s (gcp project=%s faction-topic=%s premium-topic=%s)",
			srv.Addr, cfg.GoogleCloudProject, cfg.FactionSelectedTopic, cfg.PremiumUpdatedTopic)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Printf("shop: shutdown requested")
	case err := <-errCh:
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}
