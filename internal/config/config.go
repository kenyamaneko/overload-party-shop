package config

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// IAPMode は IAP verifier 設定を必須とするかを制御する。
type IAPMode string

const (
	IAPModeProduction IAPMode = "production"
	IAPModeLocal      IAPMode = "local"
)

// Config は shop サービスの起動設定を保持する。
type Config struct {
	Port int

	// DatabaseConn は libpq キーワード形式の接続文字列。Cloud SQL Auth Proxy + IAM 認証前提でパスワードを含まない。
	DatabaseConn string

	GoogleCloudProject string

	CardPackPurchasedTopic string
	FactionAcquiredTopic   string
	PremiumUpdatedTopic    string

	IAPMode IAPMode

	// InternalAuthSecret は内部サービス間 JWT (HS256) 検証の共有秘密鍵。
	InternalAuthSecret string

	// Apple IAP
	AppleKeyID         string
	AppleIssuerID      string
	AppleBundleID      string
	ApplePrivateKeyPEM []byte
	AppleEnvironment   string

	// Apple IAP（local mode 専用 — ファイルベース秘密鍵）
	ApplePrivateKeyPath string

	// Google Play
	GooglePackageName string

	// Outbox worker のチューニング値。
	OutboxPollInterval      time.Duration
	OutboxBatchSize         int
	OutboxFailureThreshold  int
	OutboxVisibilityTimeout time.Duration
}

// FromEnv は環境変数から Config を構築する。全 env は必須で未設定は fail する。
func FromEnv() (*Config, error) {
	cfg := &Config{
		DatabaseConn:           os.Getenv("DATABASE_CONN"),
		GoogleCloudProject:     os.Getenv("GOOGLE_CLOUD_PROJECT"),
		CardPackPurchasedTopic: os.Getenv("CARD_PACK_PURCHASED_TOPIC"),
		FactionAcquiredTopic:   os.Getenv("FACTION_ACQUIRED_TOPIC"),
		PremiumUpdatedTopic:    os.Getenv("PREMIUM_UPDATED_TOPIC"),
		IAPMode:                IAPMode(os.Getenv("IAP_MODE")),
		InternalAuthSecret:     os.Getenv("INTERNAL_AUTH_SECRET"),
		AppleEnvironment:       os.Getenv("APPLE_ENVIRONMENT"),
	}

	rawPort := os.Getenv("PORT")
	if rawPort == "" {
		return nil, fmt.Errorf("config: PORT is required")
	}
	n, err := strconv.Atoi(rawPort)
	if err != nil {
		return nil, fmt.Errorf("config: PORT %q: %w", rawPort, err)
	}
	cfg.Port = n

	if cfg.DatabaseConn == "" {
		return nil, fmt.Errorf("config: DATABASE_CONN is required")
	}
	if cfg.GoogleCloudProject == "" {
		return nil, fmt.Errorf("config: GOOGLE_CLOUD_PROJECT is required")
	}
	if cfg.CardPackPurchasedTopic == "" {
		return nil, fmt.Errorf("config: CARD_PACK_PURCHASED_TOPIC is required")
	}
	if cfg.FactionAcquiredTopic == "" {
		return nil, fmt.Errorf("config: FACTION_ACQUIRED_TOPIC is required")
	}
	if cfg.PremiumUpdatedTopic == "" {
		return nil, fmt.Errorf("config: PREMIUM_UPDATED_TOPIC is required")
	}
	if cfg.InternalAuthSecret == "" {
		return nil, fmt.Errorf("config: INTERNAL_AUTH_SECRET is required")
	}

	if err := loadOutboxConfig(cfg); err != nil {
		return nil, err
	}

	switch cfg.IAPMode {
	case IAPModeProduction:
		if err := loadProductionIAP(cfg); err != nil {
			return nil, err
		}
	case IAPModeLocal:
		loadLocalIAP(cfg)
	default:
		return nil, fmt.Errorf("config: IAP_MODE must be %q or %q, got %q", IAPModeProduction, IAPModeLocal, cfg.IAPMode)
	}
	return cfg, nil
}

// loadProductionIAP は Secret Manager から IAP 認証情報を取得する。
func loadProductionIAP(cfg *Config) error {
	switch cfg.AppleEnvironment {
	case "Production", "Sandbox":
	default:
		return fmt.Errorf("config: APPLE_ENVIRONMENT must be %q or %q when IAP_MODE=production, got %q", "Production", "Sandbox", cfg.AppleEnvironment)
	}

	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("config: secret manager client: %w", err)
	}
	defer func() { _ = client.Close() }()

	cfg.AppleKeyID, err = accessSecret(ctx, client, cfg.GoogleCloudProject, "shop-apple-key-id")
	if err != nil {
		return err
	}
	cfg.AppleIssuerID, err = accessSecret(ctx, client, cfg.GoogleCloudProject, "shop-apple-issuer-id")
	if err != nil {
		return err
	}
	cfg.AppleBundleID, err = accessSecret(ctx, client, cfg.GoogleCloudProject, "shop-apple-bundle-id")
	if err != nil {
		return err
	}
	cfg.GooglePackageName, err = accessSecret(ctx, client, cfg.GoogleCloudProject, "shop-google-package-name")
	if err != nil {
		return err
	}

	pemStr, err := accessSecret(ctx, client, cfg.GoogleCloudProject, "shop-apple-private-key")
	if err != nil {
		return err
	}
	cfg.ApplePrivateKeyPEM = []byte(pemStr)

	return nil
}

// loadOutboxConfig は outbox worker のチューニング値を env から読む。
func loadOutboxConfig(cfg *Config) error {
	raw := os.Getenv("OUTBOX_POLL_INTERVAL")
	if raw == "" {
		return fmt.Errorf("config: OUTBOX_POLL_INTERVAL is required")
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("config: OUTBOX_POLL_INTERVAL %q: %w", raw, err)
	}
	if d <= 0 {
		return fmt.Errorf("config: OUTBOX_POLL_INTERVAL must be positive, got %q", raw)
	}
	cfg.OutboxPollInterval = d

	rawBatch := os.Getenv("OUTBOX_BATCH_SIZE")
	if rawBatch == "" {
		return fmt.Errorf("config: OUTBOX_BATCH_SIZE is required")
	}
	n, err := strconv.Atoi(rawBatch)
	if err != nil {
		return fmt.Errorf("config: OUTBOX_BATCH_SIZE %q: %w", rawBatch, err)
	}
	if n <= 0 {
		return fmt.Errorf("config: OUTBOX_BATCH_SIZE must be positive, got %q", rawBatch)
	}
	cfg.OutboxBatchSize = n

	rawThreshold := os.Getenv("OUTBOX_FAILURE_THRESHOLD")
	if rawThreshold == "" {
		return fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD is required")
	}
	t, err := strconv.Atoi(rawThreshold)
	if err != nil {
		return fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD %q: %w", rawThreshold, err)
	}
	if t <= 0 {
		return fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD must be positive, got %q", rawThreshold)
	}
	cfg.OutboxFailureThreshold = t

	rawVis := os.Getenv("OUTBOX_VISIBILITY_TIMEOUT")
	if rawVis == "" {
		return fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT is required")
	}
	v, err := time.ParseDuration(rawVis)
	if err != nil {
		return fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT %q: %w", rawVis, err)
	}
	if v < time.Millisecond {
		return fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms, got %q", rawVis)
	}
	cfg.OutboxVisibilityTimeout = v
	return nil
}

// loadLocalIAP はローカル開発用に環境変数から IAP 認証情報を読み込む (欠落許容)。
func loadLocalIAP(cfg *Config) {
	cfg.AppleKeyID = os.Getenv("APPLE_KEY_ID")
	cfg.AppleIssuerID = os.Getenv("APPLE_ISSUER_ID")
	cfg.AppleBundleID = os.Getenv("APPLE_BUNDLE_ID")
	cfg.ApplePrivateKeyPath = os.Getenv("APPLE_PRIVATE_KEY_PATH")
	cfg.GooglePackageName = os.Getenv("GOOGLE_PACKAGE_NAME")
}

func accessSecret(ctx context.Context, client *secretmanager.Client, project, secretID string) (string, error) {
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, secretID)
	result, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		return "", fmt.Errorf("config: access secret %s: %w", secretID, err)
	}
	return string(result.Payload.Data), nil
}
