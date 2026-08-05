package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allEnvKeys は FromEnv が読む全 env キー。各テストは毎回これらを明示値（または ""）
// で上書きし、シェル環境からの漏れで Given が非決定になるのを防ぐ。
// testPublicKeyPEM は config が値をそのまま保持することの確認にだけ使うダミー。
// 鍵としての妥当性は検証しないため、PEM の体裁だけ揃えている。
const testPublicKeyPEM = "-----BEGIN PUBLIC KEY-----\ndummy-not-a-real-key\n-----END PUBLIC KEY-----\n"

var allEnvKeys = []string{
	"PORT",
	"DATABASE_CONN",
	"GOOGLE_CLOUD_PROJECT",
	"CARD_PACK_PURCHASED_TOPIC",
	"FACTION_ACQUIRED_TOPIC",
	"PREMIUM_UPDATED_TOPIC",
	"IAP_VERIFIER",
	"LOG_MODE",
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
	"INTERNAL_AUTH_PUBLIC_KEY",
	"DATABASE_IAM_AUTH_ENABLED",
	"CLOUDSQL_CONNECTION_NAME",
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

// validLocalEnv は IAP_VERIFIER=stub での最小構成（必須 env を全て明示）。
// CLAUDE.md「デフォルト値へのフォールバックを行わない」方針により、
// 全必須 env を明示的に供給する。各ケースはこれを baseline に override を重ねる。
var validLocalEnv = map[string]string{
	"PORT":                      "9006",
	"DATABASE_CONN":             "host=localhost port=5432 dbname=test sslmode=disable",
	"GOOGLE_CLOUD_PROJECT":      "test-project",
	"CARD_PACK_PURCHASED_TOPIC": "card-pack-purchased",
	"FACTION_ACQUIRED_TOPIC":    "faction-acquired",
	"PREMIUM_UPDATED_TOPIC":     "premium-updated",
	"IAP_VERIFIER":              "stub",
	"LOG_MODE":                  "local",
	"OUTBOX_POLL_INTERVAL":      "1s",
	"OUTBOX_BATCH_SIZE":         "100",
	"OUTBOX_FAILURE_THRESHOLD":  "5",
	"OUTBOX_VISIBILITY_TIMEOUT": "30s",
	"INTERNAL_AUTH_PUBLIC_KEY":  testPublicKeyPEM,
	"DATABASE_IAM_AUTH_ENABLED": "false",
}

func TestFromEnv(t *testing.T) {
	t.Run("環境変数からのConfig構築", func(t *testing.T) {
		t.Run("IAP設定が未指定のlocal modeのとき、起動に成功しAppleKeyIDは空になる", func(t *testing.T) {
			setEnv(t, validLocalEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, IAPVerifierStub, cfg.IAPVerifier)
			assert.Equal(t, LogModeLocal, cfg.LogMode)
			assert.Empty(t, cfg.AppleKeyID)
		})

		t.Run("必須envが揃うとき、全フィールドがConfigに伝搬する", func(t *testing.T) {
			setEnv(t, validLocalEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 9006, cfg.Port)
			assert.Equal(t, "host=localhost port=5432 dbname=test sslmode=disable", cfg.DatabaseConn)
			assert.Equal(t, "test-project", cfg.GoogleCloudProject)
			assert.Equal(t, "card-pack-purchased", cfg.CardPackPurchasedTopic)
			assert.Equal(t, "faction-acquired", cfg.FactionAcquiredTopic)
			assert.Equal(t, "premium-updated", cfg.PremiumUpdatedTopic)
			assert.Equal(t, testPublicKeyPEM, cfg.InternalAuthPublicKey)
		})

		t.Run("DATABASE_IAM_AUTH_ENABLEDがfalseのとき、CLOUDSQL_CONNECTION_NAMEが未設定でも成功する", func(t *testing.T) {
			setEnv(t, validLocalEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.False(t, cfg.DatabaseIAMAuthEnabled)
			assert.Empty(t, cfg.CloudSQLConnectionName)
		})

		t.Run("DATABASE_IAM_AUTH_ENABLEDがtrueかつCLOUDSQL_CONNECTION_NAMEが指定されるとき、両方の値がConfigに反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{
				"DATABASE_IAM_AUTH_ENABLED": "true",
				"CLOUDSQL_CONNECTION_NAME":  "overload-party-dev:asia-northeast1:overload-party-db",
			}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.True(t, cfg.DatabaseIAMAuthEnabled)
			assert.Equal(t, "overload-party-dev:asia-northeast1:overload-party-db", cfg.CloudSQLConnectionName)
		})

		t.Run("OUTBOX_POLL_INTERVAL等のoutbox envが指定されるとき、値がConfigに反映される", func(t *testing.T) {
			setEnv(t, validLocalEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1*time.Second, cfg.OutboxPollInterval)
			assert.Equal(t, 100, cfg.OutboxBatchSize)
			assert.Equal(t, 5, cfg.OutboxFailureThreshold)
			assert.Equal(t, 30*time.Second, cfg.OutboxVisibilityTimeout)
		})

		t.Run("OUTBOX_BATCH_SIZEが有効最小の1のとき、バッチサイズに1が設定される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": "1"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1, cfg.OutboxBatchSize)
		})

		t.Run("OUTBOX_FAILURE_THRESHOLDが有効最小の1のとき、失敗閾値に1が設定される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "1"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1, cfg.OutboxFailureThreshold)
		})

		t.Run("OUTBOX_VISIBILITY_TIMEOUTが下限ちょうどの1msのとき、可視性タイムアウトに1msが設定される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "1ms"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, time.Millisecond, cfg.OutboxVisibilityTimeout)
		})

		t.Run("OUTBOX_POLL_INTERVALが正の最小値 (1ns)のとき、ポーリング間隔に1nsが設定される", func(t *testing.T) {
			setEnv(t, mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "1ns"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, time.Nanosecond, cfg.OutboxPollInterval)
		})

		t.Run("local modeでAPPLE_KEY_ID等のIAP envが指定されるとき、値がConfigに反映される", func(t *testing.T) {
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

		t.Run("PORTと各topic envがbaselineから上書きされるとき、上書き後の値がConfigに反映される", func(t *testing.T) {
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
				name:    "PORTが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"PORT": ""}),
				wantErr: "PORT is required",
			},
			{
				name:    "PORTが数値でないとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"PORT": "not-a-number"}),
				wantErr: "PORT",
			},
			{
				name:    "DATABASE_CONNが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"DATABASE_CONN": ""}),
				wantErr: "DATABASE_CONN is required",
			},
			{
				name:    "GOOGLE_CLOUD_PROJECTが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"GOOGLE_CLOUD_PROJECT": ""}),
				wantErr: "GOOGLE_CLOUD_PROJECT is required",
			},
			{
				name:    "CARD_PACK_PURCHASED_TOPICが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"CARD_PACK_PURCHASED_TOPIC": ""}),
				wantErr: "CARD_PACK_PURCHASED_TOPIC is required",
			},
			{
				name:    "FACTION_ACQUIRED_TOPICが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"FACTION_ACQUIRED_TOPIC": ""}),
				wantErr: "FACTION_ACQUIRED_TOPIC is required",
			},
			{
				name:    "PREMIUM_UPDATED_TOPICが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"PREMIUM_UPDATED_TOPIC": ""}),
				wantErr: "PREMIUM_UPDATED_TOPIC is required",
			},
			{
				name:    "IAP_VERIFIERが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"IAP_VERIFIER": ""}),
				wantErr: "IAP_VERIFIER must be",
			},
			{
				name:    "IAP_VERIFIERが未定義値 (invalid)のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"IAP_VERIFIER": "invalid"}),
				wantErr: "IAP_VERIFIER must be",
			},
			{
				name:    "LOG_MODEが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"LOG_MODE": ""}),
				wantErr: "LOG_MODE must be",
			},
			{
				name:    "LOG_MODEが未定義値 (invalid)のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"LOG_MODE": "invalid"}),
				wantErr: "LOG_MODE must be",
			},
			{
				name: "IAP_VERIFIERがstoreかつAPPLE_ENVIRONMENTが未設定のとき、エラーになる",
				envs: mergeEnv(validLocalEnv, map[string]string{
					"IAP_VERIFIER":      "store",
					"APPLE_ENVIRONMENT": "",
				}),
				wantErr: "APPLE_ENVIRONMENT must be",
			},
			{
				name: "IAP_VERIFIERがstoreかつAPPLE_ENVIRONMENTが未定義値 (staging)のとき、エラーになる",
				envs: mergeEnv(validLocalEnv, map[string]string{
					"IAP_VERIFIER":      "store",
					"APPLE_ENVIRONMENT": "staging",
				}),
				wantErr: "APPLE_ENVIRONMENT must be",
			},
			{
				name:    "OUTBOX_POLL_INTERVALが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": ""}),
				wantErr: "OUTBOX_POLL_INTERVAL is required",
			},
			{
				name:    "OUTBOX_POLL_INTERVALがdurationとしてパースできない (abc)のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "abc"}),
				wantErr: "OUTBOX_POLL_INTERVAL",
			},
			{
				name:    "OUTBOX_POLL_INTERVALが0sのとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "0s"}),
				wantErr: "OUTBOX_POLL_INTERVAL must be positive",
			},
			{
				name:    "OUTBOX_BATCH_SIZEが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": ""}),
				wantErr: "OUTBOX_BATCH_SIZE is required",
			},
			{
				name:    "OUTBOX_BATCH_SIZEが数値でない (abc)のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": "abc"}),
				wantErr: "OUTBOX_BATCH_SIZE",
			},
			{
				name:    "OUTBOX_BATCH_SIZEが0のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_BATCH_SIZE": "0"}),
				wantErr: "OUTBOX_BATCH_SIZE must be positive",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLDが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": ""}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD is required",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLDが0のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "0"}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD must be positive",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLDが数値でない (abc)のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "abc"}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUTが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": ""}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT is required",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUTがdurationとしてパースできない (abc)のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "abc"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUTが0sのとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "0s"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUTが1ms未満 (500us)のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "500us"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms",
			},
			{
				name:    "INTERNAL_AUTH_PUBLIC_KEYが未設定のとき、エラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"INTERNAL_AUTH_PUBLIC_KEY": ""}),
				wantErr: "INTERNAL_AUTH_PUBLIC_KEY is required",
			},
			{
				name:    "DATABASE_IAM_AUTH_ENABLEDが未設定のとき、変数名を含むエラーになる",
				envs:    mergeEnv(validLocalEnv, map[string]string{"DATABASE_IAM_AUTH_ENABLED": ""}),
				wantErr: "DATABASE_IAM_AUTH_ENABLED must be",
			},
			{
				name:    `DATABASE_IAM_AUTH_ENABLEDが "true"/"false" 以外の "yes" のとき、変数名を含むエラーになる`,
				envs:    mergeEnv(validLocalEnv, map[string]string{"DATABASE_IAM_AUTH_ENABLED": "yes"}),
				wantErr: "DATABASE_IAM_AUTH_ENABLED must be",
			},
			{
				name: "DATABASE_IAM_AUTH_ENABLEDがtrueかつCLOUDSQL_CONNECTION_NAMEが未設定のとき、CLOUDSQL_CONNECTION_NAMEを含むエラーになる",
				envs: mergeEnv(validLocalEnv, map[string]string{
					"DATABASE_IAM_AUTH_ENABLED": "true",
					"CLOUDSQL_CONNECTION_NAME":  "",
				}),
				wantErr: "CLOUDSQL_CONNECTION_NAME is required",
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
