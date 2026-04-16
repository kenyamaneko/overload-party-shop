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
	Port int

	// DatabaseConn は libpq キーワード形式の接続文字列（URL 形式ではない）。
	// 本番は Cloud SQL Auth Proxy + IAM 認証で接続するためパスワードを含まない。
	// したがって機密情報ではなく ConfigMap で注入して良い。
	DatabaseConn string

	// GoogleCloudProject はこのサービスが利用する全 GCP リソース
	// （Pub/Sub topic / Firestore / Secret Manager）の project ID。
	GoogleCloudProject string

	FactionSelectedTopic string
	PremiumUpdatedTopic  string

	// IAPMode は IAP verifier 設定を必須とするかを制御する。
	// IAP_MODE=local にすると IAP 設定なしで起動でき、webhook ルートは登録されない。
	IAPMode IAPMode

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

	// Outbox worker 設定。shop.outbox_events を消費する常駐 worker のチューニング値。
	// ハードコードではなく env で持つのは、負荷試験やインシデント時にデプロイなしで
	// 試行錯誤できるようにするため。
	OutboxPollInterval      time.Duration // 例: 1s
	OutboxBatchSize         int           // 1 tick で claim する最大行数
	OutboxFailureThreshold  int           // この回数以上の連続失敗で ERROR ログ (死蔵検知)
	OutboxVisibilityTimeout time.Duration // claim 後この期間は他 worker が同じ行を拾わない。publish 中の worker がクラッシュしたらこの時間経過後に他 worker が再試行する。
}

// FromEnv は環境変数から Config を構築する。
// 全 env は必須。未設定や未定義値は起動時に fail する（デフォルトへのフォールバック禁止）。
func FromEnv() (*Config, error) {
	cfg := &Config{
		DatabaseConn:         os.Getenv("DATABASE_CONN"),
		GoogleCloudProject:   os.Getenv("GOOGLE_CLOUD_PROJECT"),
		FactionSelectedTopic: os.Getenv("FACTION_SELECTED_TOPIC"),
		PremiumUpdatedTopic:  os.Getenv("PREMIUM_UPDATED_TOPIC"),
		IAPMode:              IAPMode(os.Getenv("IAP_MODE")),
		AppleEnvironment:     os.Getenv("APPLE_ENVIRONMENT"),
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
	if cfg.FactionSelectedTopic == "" {
		return nil, fmt.Errorf("config: FACTION_SELECTED_TOPIC is required")
	}
	if cfg.PremiumUpdatedTopic == "" {
		return nil, fmt.Errorf("config: PREMIUM_UPDATED_TOPIC is required")
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
// APPLE_ENVIRONMENT が不正・シークレット到達不可の場合は fail-fast する。
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

// loadOutboxConfig は outbox worker のチューニング値を env から読む。
// 全値必須で、パース不能・非正値は fail-fast する (CLAUDE.md「デフォルトへのフォール
// バック禁止」方針)。
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
	if v <= 0 {
		return fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT must be positive, got %q", rawVis)
	}
	cfg.OutboxVisibilityTimeout = v
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
