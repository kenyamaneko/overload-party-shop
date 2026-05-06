package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// allEnvKeys は FromEnv が読む全 env キー。各テストは毎回これらを明示値（または ""）
// で上書きし、シェル環境からの漏れで Given が非決定になるのを防ぐ。
var allEnvKeys = []string{
	"PORT",
	"DATABASE_CONN",
	"GOOGLE_CLOUD_PROJECT",
	"CARD_PACK_PURCHASED_TOPIC",
	"FACTION_ACQUIRED_TOPIC",
	"PREMIUM_UPDATED_TOPIC",
	"IAP_MODE",
	"APPLE_ENVIRONMENT",
	"APPLE_KEY_ID",
	"APPLE_ISSUER_ID",
	"APPLE_BUNDLE_ID",
	"APPLE_PRIVATE_KEY_PATH",
	"GOOGLE_PACKAGE_NAME",
	"OUTBOX_POLL_INTERVAL",
	"OUTBOX_BATCH_SIZE",
	"OUTBOX_FAILURE_THRESHOLD",
	"OUTBOX_VISIBILITY_TIMEOUT",
}

// setEnv は allEnvKeys を一括で上書きする。envs に無いキーは "" (未設定相当) として
// t.Setenv で適用する — os.Getenv は "" と unset を区別しないため、空文字指定で
// required チェックの missing 経路を発火できる。t.Setenv はテスト終了時に復元する。
func setEnv(t *testing.T, envs map[string]string) {
	t.Helper()
	for _, k := range allEnvKeys {
		t.Setenv(k, envs[k])
	}
}

func mergeEnv(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// validLocalEnv は IAP_MODE=local での最小構成（必須 env を全て明示）。
// CLAUDE.md「デフォルト値へのフォールバックを行わない」方針により、
// 全必須 env を明示的に供給する。各ケースはこれを baseline に override を重ねる。
var validLocalEnv = map[string]string{
	"PORT":                      "9006",
	"DATABASE_CONN":             "host=localhost port=5432 dbname=test sslmode=disable",
	"GOOGLE_CLOUD_PROJECT":      "test-project",
	"CARD_PACK_PURCHASED_TOPIC": domain.TopicCardPackPurchased,
	"FACTION_ACQUIRED_TOPIC":    domain.TopicFactionAcquired,
	"PREMIUM_UPDATED_TOPIC":     domain.TopicPremiumUpdated,
	"IAP_MODE":                  "local",
	"OUTBOX_POLL_INTERVAL":      "1s",
	"OUTBOX_BATCH_SIZE":         "100",
	"OUTBOX_FAILURE_THRESHOLD":  "5",
	"OUTBOX_VISIBILITY_TIMEOUT": "30s",
}

func TestFromEnv_Success(t *testing.T) {
	tests := []struct {
		name   string
		envs   map[string]string
		assert func(t *testing.T, cfg *Config)
	}{
		{
			name: "local mode は IAP 設定なしでも起動する",
			envs: validLocalEnv,
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, IAPModeLocal, cfg.IAPMode)
				assert.Empty(t, cfg.AppleKeyID, "IAP 設定欠落でも起動する")
			},
		},
		{
			name: "必須 env が Config に伝搬する",
			envs: validLocalEnv,
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 9006, cfg.Port)
				assert.Equal(t, "host=localhost port=5432 dbname=test sslmode=disable", cfg.DatabaseConn)
				assert.Equal(t, "test-project", cfg.GoogleCloudProject)
				assert.Equal(t, domain.TopicCardPackPurchased, cfg.CardPackPurchasedTopic)
				assert.Equal(t, domain.TopicFactionAcquired, cfg.FactionAcquiredTopic)
				assert.Equal(t, domain.TopicPremiumUpdated, cfg.PremiumUpdatedTopic)
			},
		},
		{
			name: "outbox 設定が env から Config に伝搬する",
			envs: validLocalEnv,
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 1*time.Second, cfg.OutboxPollInterval)
				assert.Equal(t, 100, cfg.OutboxBatchSize)
				assert.Equal(t, 5, cfg.OutboxFailureThreshold)
				assert.Equal(t, 30*time.Second, cfg.OutboxVisibilityTimeout)
			},
		},
		{
			name: "IAP 設定が env から Config に伝搬する（local mode）",
			envs: mergeEnv(validLocalEnv, map[string]string{
				"APPLE_KEY_ID":           "KEY123",
				"APPLE_ISSUER_ID":        "ISS456",
				"APPLE_BUNDLE_ID":        "com.test.app",
				"APPLE_PRIVATE_KEY_PATH": "/tmp/test.p8",
				"GOOGLE_PACKAGE_NAME":    "com.test.android",
			}),
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "KEY123", cfg.AppleKeyID)
				assert.Equal(t, "ISS456", cfg.AppleIssuerID)
				assert.Equal(t, "com.test.app", cfg.AppleBundleID)
				assert.Equal(t, "/tmp/test.p8", cfg.ApplePrivateKeyPath)
				assert.Equal(t, "com.test.android", cfg.GooglePackageName)
			},
		},
		{
			name: "PORT / topic 名が env から上書きされる",
			envs: mergeEnv(validLocalEnv, map[string]string{
				"PORT":                      "8080",
				"CARD_PACK_PURCHASED_TOPIC": "card-pack-purchased-ci",
				"FACTION_ACQUIRED_TOPIC":    "faction-acquired-ci",
				"PREMIUM_UPDATED_TOPIC":     "premium-updated-ci",
			}),
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 8080, cfg.Port)
				assert.Equal(t, "card-pack-purchased-ci", cfg.CardPackPurchasedTopic)
				assert.Equal(t, "faction-acquired-ci", cfg.FactionAcquiredTopic)
				assert.Equal(t, "premium-updated-ci", cfg.PremiumUpdatedTopic)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.envs)
			cfg, err := FromEnv()
			require.NoError(t, err)
			tt.assert(t, cfg)
		})
	}
}

// TestFromEnv_Errors は「必須 env が未設定・未定義値で起動を拒否する」仕様を固定する。
// CLAUDE.md「デフォルト値へのフォールバックを行わない」方針の回帰防止。
func TestFromEnv_Errors(t *testing.T) {
	tests := []struct {
		name    string
		envs    map[string]string
		wantErr string
	}{
		{
			name:    "PORT が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"PORT": ""}),
			wantErr: "PORT is required",
		},
		{
			name:    "PORT が数値でないならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"PORT": "not-a-number"}),
			wantErr: "PORT",
		},
		{
			name:    "DATABASE_CONN が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"DATABASE_CONN": ""}),
			wantErr: "DATABASE_CONN is required",
		},
		{
			name:    "GOOGLE_CLOUD_PROJECT が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"GOOGLE_CLOUD_PROJECT": ""}),
			wantErr: "GOOGLE_CLOUD_PROJECT is required",
		},
		{
			name:    "CARD_PACK_PURCHASED_TOPIC が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"CARD_PACK_PURCHASED_TOPIC": ""}),
			wantErr: "CARD_PACK_PURCHASED_TOPIC is required",
		},
		{
			name:    "FACTION_ACQUIRED_TOPIC が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"FACTION_ACQUIRED_TOPIC": ""}),
			wantErr: "FACTION_ACQUIRED_TOPIC is required",
		},
		{
			name:    "PREMIUM_UPDATED_TOPIC が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"PREMIUM_UPDATED_TOPIC": ""}),
			wantErr: "PREMIUM_UPDATED_TOPIC is required",
		},
		{
			name:    "IAP_MODE が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"IAP_MODE": ""}),
			wantErr: "IAP_MODE must be",
		},
		{
			name:    "IAP_MODE が未定義値ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"IAP_MODE": "invalid"}),
			wantErr: "IAP_MODE must be",
		},
		{
			name: "production mode で APPLE_ENVIRONMENT が未設定ならエラー",
			envs: mergeEnv(validLocalEnv, map[string]string{
				"IAP_MODE":          "production",
				"APPLE_ENVIRONMENT": "",
			}),
			wantErr: "APPLE_ENVIRONMENT must be",
		},
		{
			name: "production mode で APPLE_ENVIRONMENT が未定義値ならエラー",
			envs: mergeEnv(validLocalEnv, map[string]string{
				"IAP_MODE":          "production",
				"APPLE_ENVIRONMENT": "staging",
			}),
			wantErr: "APPLE_ENVIRONMENT must be",
		},
		{
			name:    "OUTBOX_POLL_INTERVAL が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": ""}),
			wantErr: "OUTBOX_POLL_INTERVAL is required",
		},
		{
			name:    "OUTBOX_POLL_INTERVAL が duration としてパースできないならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "abc"}),
			wantErr: "OUTBOX_POLL_INTERVAL",
		},
		{
			name:    "OUTBOX_POLL_INTERVAL が 0 以下ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "0s"}),
			wantErr: "OUTBOX_POLL_INTERVAL must be positive",
		},
		{
			name:    "OUTBOX_BATCH_SIZE が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": ""}),
			wantErr: "OUTBOX_BATCH_SIZE is required",
		},
		{
			name:    "OUTBOX_BATCH_SIZE が数値でないならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": "abc"}),
			wantErr: "OUTBOX_BATCH_SIZE",
		},
		{
			name:    "OUTBOX_BATCH_SIZE が 0 以下ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": "0"}),
			wantErr: "OUTBOX_BATCH_SIZE must be positive",
		},
		{
			name:    "OUTBOX_FAILURE_THRESHOLD が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": ""}),
			wantErr: "OUTBOX_FAILURE_THRESHOLD is required",
		},
		{
			name:    "OUTBOX_FAILURE_THRESHOLD が 0 以下ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "0"}),
			wantErr: "OUTBOX_FAILURE_THRESHOLD must be positive",
		},
		{
			name:    "OUTBOX_VISIBILITY_TIMEOUT が未設定ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": ""}),
			wantErr: "OUTBOX_VISIBILITY_TIMEOUT is required",
		},
		{
			name:    "OUTBOX_VISIBILITY_TIMEOUT が duration としてパースできないならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "abc"}),
			wantErr: "OUTBOX_VISIBILITY_TIMEOUT",
		},
		{
			name:    "OUTBOX_VISIBILITY_TIMEOUT が 0 以下ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "0s"}),
			wantErr: "OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms",
		},
		{
			name:    "OUTBOX_VISIBILITY_TIMEOUT が 1ms 未満ならエラー",
			envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "500us"}),
			wantErr: "OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.envs)
			_, err := FromEnv()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
