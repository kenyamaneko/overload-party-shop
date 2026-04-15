package config

import (
	"context"
	"fmt"
	"os"
	"strconv"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// IAPMode は起動時に Apple/Google IAP verifier 設定を必須とするかを制御する。
// production は完全な IAP 設定がないと起動を拒否し、local は IAP 設定なしでの
// 起動を許容する。
type IAPMode string

const (
	// IAPModeProduction は本番環境で IAP 設定を必須とするモード。
	IAPModeProduction IAPMode = "production"
	// IAPModeLocal はローカル開発で IAP 設定なしでも起動可能なモード。
	IAPModeLocal IAPMode = "local"
)

// Config は shop サービスの起動設定を保持する。
type Config struct {
	Port        int
	Env         string
	DatabaseURL string

	// faction-selected + premium-updated topic をホストする Google Cloud project。
	// 必須 — 設定がない場合イベントが静かに失われるため fail-fast する。
	PubsubProjectID string
	// topic 名はクロスプロジェクトテスト用に変更可能。本番はデフォルト値を使用。
	FactionSelectedTopic string
	PremiumUpdatedTopic  string

	// strict / permissive verifier 要件を切り替える。必須設定 — 未指定で fail する。
	// IAP_MODE=local にすると IAP 設定なしで起動でき、webhook ルートは登録されない。
	IAPMode IAPMode

	// Secret Manager 参照用 Google Cloud project ID。IAPMode == production のとき必須。
	GoogleCloudProject string

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

	// FirestoreProjectID は game_config の読み取り先プロジェクト ID。
	// ローカル/CI では FIRESTORE_EMULATOR_HOST を別途設定することでエミュレーターに接続。
	FirestoreProjectID string
}

// FromEnv は環境変数から Config を構築する。
func FromEnv() (*Config, error) {
	cfg := &Config{
		Port:                 9006,
		Env:                  getEnv("ENV", "dev"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		PubsubProjectID:      os.Getenv("PUBSUB_PROJECT_ID"),
		FactionSelectedTopic: getEnv("FACTION_SELECTED_TOPIC", "faction-selected"),
		PremiumUpdatedTopic:  getEnv("PREMIUM_UPDATED_TOPIC", "premium-updated"),
		IAPMode:              IAPMode(os.Getenv("IAP_MODE")),
		GoogleCloudProject:   os.Getenv("GOOGLE_CLOUD_PROJECT"),
		AppleEnvironment:     getEnv("APPLE_ENVIRONMENT", "Sandbox"),
		FirestoreProjectID:   os.Getenv("FIRESTORE_PROJECT_ID"),
	}

	if raw := os.Getenv("PORT"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("config: PORT %q: %w", raw, err)
		}
		cfg.Port = n
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.PubsubProjectID == "" {
		return nil, fmt.Errorf("config: PUBSUB_PROJECT_ID is required (shop publishes faction-selected / premium-updated events)")
	}
	if cfg.FirestoreProjectID == "" {
		return nil, fmt.Errorf("config: FIRESTORE_PROJECT_ID is required (game_config)")
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
// GOOGLE_CLOUD_PROJECT 未設定やシークレット到達不可の場合は fail-fast する。
func loadProductionIAP(cfg *Config) error {
	if cfg.GoogleCloudProject == "" {
		return fmt.Errorf("config: GOOGLE_CLOUD_PROJECT is required when IAP_MODE=production")
	}

	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("config: secret manager client: %w", err)
	}
	defer client.Close()

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

// loadLocalIAP はローカル開発用に環境変数から IAP 認証情報を読み込む。
// 値が欠落していても許容する（IAPMode == local では verifier 初期化をスキップする）。
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
