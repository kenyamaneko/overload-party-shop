package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	"INTERNAL_AUTH_SECRET",
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
	"CARD_PACK_PURCHASED_TOPIC": "card-pack-purchased",
	"FACTION_ACQUIRED_TOPIC":    "faction-acquired",
	"PREMIUM_UPDATED_TOPIC":     "premium-updated",
	"IAP_MODE":                  "local",
	"OUTBOX_POLL_INTERVAL":      "1s",
	"OUTBOX_BATCH_SIZE":         "100",
	"OUTBOX_FAILURE_THRESHOLD":  "5",
	"OUTBOX_VISIBILITY_TIMEOUT": "30s",
	"INTERNAL_AUTH_SECRET":      "test-internal-auth-secret-do-not-use-in-prod-xxxxx",
}

func TestFromEnv(t *testing.T) {
	t.Run("環境変数からの Config 構築", func(t *testing.T) {
		t.Run("IAP 設定が未指定の local mode のとき、起動に成功し AppleKeyID は空になる", func(t *testing.T) {
			setEnv(t, validLocalEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, IAPModeLocal, cfg.IAPMode)
			assert.Empty(t, cfg.AppleKeyID)
		})

		t.Run("必須 env が揃うとき、全フィールドが Config に伝搬する", func(t *testing.T) {
			setEnv(t, validLocalEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 9006, cfg.Port)
			assert.Equal(t, "host=localhost port=5432 dbname=test sslmode=disable", cfg.DatabaseConn)
			assert.Equal(t, "test-project", cfg.GoogleCloudProject)
			assert.Equal(t, "card-pack-purchased", cfg.CardPackPurchasedTopic)
			assert.Equal(t, "faction-acquired", cfg.FactionAcquiredTopic)
			assert.Equal(t, "premium-updated", cfg.PremiumUpdatedTopic)
			assert.Equal(t, "test-internal-auth-secret-do-not-use-in-prod-xxxxx", cfg.InternalAuthSecret)
		})

		t.Run("OUTBOX_POLL_INTERVAL 等の outbox env が指定されるとき、値が Config に反映される", func(t *testing.T) {
			setEnv(t, validLocalEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1*time.Second, cfg.OutboxPollInterval)
			assert.Equal(t, 100, cfg.OutboxBatchSize)
			assert.Equal(t, 5, cfg.OutboxFailureThreshold)
			assert.Equal(t, 30*time.Second, cfg.OutboxVisibilityTimeout)
		})

		t.Run("OUTBOX_BATCH_SIZE が有効最小の 1 のとき、Outbox バッチサイズに 1 が設定される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": "1"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1, cfg.OutboxBatchSize)
		})

		t.Run("OUTBOX_FAILURE_THRESHOLD が有効最小の 1 のとき、Outbox の失敗閾値に 1 が設定される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "1"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1, cfg.OutboxFailureThreshold)
		})

		t.Run("OUTBOX_VISIBILITY_TIMEOUT が下限ちょうどの 1ms のとき、可視性タイムアウトに 1ms が設定される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "1ms"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, time.Millisecond, cfg.OutboxVisibilityTimeout)
		})

		t.Run("OUTBOX_POLL_INTERVAL が正の最小値 (1ns) のとき、ポーリング間隔に 1ns が設定される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "1ns"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, time.Nanosecond, cfg.OutboxPollInterval)
		})

		t.Run("local mode で APPLE_KEY_ID 等の IAP env が指定されるとき、値が Config に反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{
				"APPLE_KEY_ID":           "KEY123",
				"APPLE_ISSUER_ID":        "ISS456",
				"APPLE_BUNDLE_ID":        "com.test.app",
				"APPLE_PRIVATE_KEY_PATH": "/tmp/test.p8",
				"GOOGLE_PACKAGE_NAME":    "com.test.android",
			}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, "KEY123", cfg.AppleKeyID)
			assert.Equal(t, "ISS456", cfg.AppleIssuerID)
			assert.Equal(t, "com.test.app", cfg.AppleBundleID)
			assert.Equal(t, "/tmp/test.p8", cfg.ApplePrivateKeyPath)
			assert.Equal(t, "com.test.android", cfg.GooglePackageName)
		})

		t.Run("PORT と各 topic env が baseline から上書きされるとき、上書き後の値が Config に反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{
				"PORT":                      "8080",
				"CARD_PACK_PURCHASED_TOPIC": "card-pack-purchased-ci",
				"FACTION_ACQUIRED_TOPIC":    "faction-acquired-ci",
				"PREMIUM_UPDATED_TOPIC":     "premium-updated-ci",
			}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 8080, cfg.Port)
			assert.Equal(t, "card-pack-purchased-ci", cfg.CardPackPurchasedTopic)
			assert.Equal(t, "faction-acquired-ci", cfg.FactionAcquiredTopic)
			assert.Equal(t, "premium-updated-ci", cfg.PremiumUpdatedTopic)
		})

		// 必須 env が未設定・未定義値のときはデフォルトにフォールバックせず即エラーにする (回帰防止)。
		invalidCases := []struct {
			name    string
			envs    map[string]string
			wantErr string
		}{
			{
				name:    "PORT が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"PORT": ""}),
				wantErr: "PORT is required",
			},
			{
				name:    "PORT が数値でないとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"PORT": "not-a-number"}),
				wantErr: "PORT",
			},
			{
				name:    "DATABASE_CONN が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"DATABASE_CONN": ""}),
				wantErr: "DATABASE_CONN is required",
			},
			{
				name:    "GOOGLE_CLOUD_PROJECT が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"GOOGLE_CLOUD_PROJECT": ""}),
				wantErr: "GOOGLE_CLOUD_PROJECT is required",
			},
			{
				name:    "CARD_PACK_PURCHASED_TOPIC が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"CARD_PACK_PURCHASED_TOPIC": ""}),
				wantErr: "CARD_PACK_PURCHASED_TOPIC is required",
			},
			{
				name:    "FACTION_ACQUIRED_TOPIC が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"FACTION_ACQUIRED_TOPIC": ""}),
				wantErr: "FACTION_ACQUIRED_TOPIC is required",
			},
			{
				name:    "PREMIUM_UPDATED_TOPIC が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"PREMIUM_UPDATED_TOPIC": ""}),
				wantErr: "PREMIUM_UPDATED_TOPIC is required",
			},
			{
				name:    "IAP_MODE が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"IAP_MODE": ""}),
				wantErr: "IAP_MODE must be",
			},
			{
				name:    "IAP_MODE が未定義値 (invalid) のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"IAP_MODE": "invalid"}),
				wantErr: "IAP_MODE must be",
			},
			{
				name: "IAP_MODE が production かつ APPLE_ENVIRONMENT が未設定のとき、エラーになる",
				envs: mergeEnv(validLocalEnv, map[string]string{
					"IAP_MODE":          "production",
					"APPLE_ENVIRONMENT": "",
				}),
				wantErr: "APPLE_ENVIRONMENT must be",
			},
			{
				name: "IAP_MODE が production かつ APPLE_ENVIRONMENT が未定義値 (staging) のとき、エラーになる",
				envs: mergeEnv(validLocalEnv, map[string]string{
					"IAP_MODE":          "production",
					"APPLE_ENVIRONMENT": "staging",
				}),
				wantErr: "APPLE_ENVIRONMENT must be",
			},
			{
				name:    "OUTBOX_POLL_INTERVAL が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": ""}),
				wantErr: "OUTBOX_POLL_INTERVAL is required",
			},
			{
				name:    "OUTBOX_POLL_INTERVAL が duration としてパースできない (abc) のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "abc"}),
				wantErr: "OUTBOX_POLL_INTERVAL",
			},
			{
				name:    "OUTBOX_POLL_INTERVAL が 0s のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "0s"}),
				wantErr: "OUTBOX_POLL_INTERVAL must be positive",
			},
			{
				name:    "OUTBOX_BATCH_SIZE が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": ""}),
				wantErr: "OUTBOX_BATCH_SIZE is required",
			},
			{
				name:    "OUTBOX_BATCH_SIZE が数値でない (abc) のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": "abc"}),
				wantErr: "OUTBOX_BATCH_SIZE",
			},
			{
				name:    "OUTBOX_BATCH_SIZE が 0 のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": "0"}),
				wantErr: "OUTBOX_BATCH_SIZE must be positive",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLD が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": ""}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD is required",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLD が 0 のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "0"}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD must be positive",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLD が数値でない (abc) のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "abc"}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUT が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": ""}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT is required",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUT が duration としてパースできない (abc) のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "abc"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUT が 0s のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "0s"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUT が 1ms 未満 (500us) のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "500us"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms",
			},
			{
				name:    "INTERNAL_AUTH_SECRET が未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"INTERNAL_AUTH_SECRET": ""}),
				wantErr: "INTERNAL_AUTH_SECRET is required",
			},
		}
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				setEnv(t, tc.envs)

				_, err := FromEnv()

				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			})
		}
	})
}
