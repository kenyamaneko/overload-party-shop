package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allEnvKeys は FromEnv が読む全 env キー。各テストは毎回これらを明示値（または ""）
// で上書きし、シェル環境からの漏れで Given が非決定になるのを防ぐ。
var allEnvKeys = []string{
	"ENV",
	"PORT",
	"DATABASE_URL",
	"PUBSUB_PROJECT_ID",
	"FACTION_SELECTED_TOPIC",
	"PREMIUM_UPDATED_TOPIC",
	"IAP_MODE",
	"GOOGLE_CLOUD_PROJECT",
	"APPLE_ENVIRONMENT",
	"FIRESTORE_PROJECT_ID",
	"APPLE_KEY_ID",
	"APPLE_ISSUER_ID",
	"APPLE_BUNDLE_ID",
	"APPLE_PRIVATE_KEY_PATH",
	"GOOGLE_PACKAGE_NAME",
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

// validLocalEnv は IAP_MODE=local での最小構成（必須 env のみ設定）。
// 各ケースはこれを baseline に override を重ねる。
var validLocalEnv = map[string]string{
	"DATABASE_URL":         "postgres://localhost/test",
	"PUBSUB_PROJECT_ID":    "test-project",
	"FIRESTORE_PROJECT_ID": "test-project",
	"IAP_MODE":             "local",
}

func TestFromEnv_Success(t *testing.T) {
	tests := []struct {
		name   string
		envs   map[string]string
		assert func(t *testing.T, cfg *Config)
	}{
		{
			name: "local mode は IAP 設定なしでも起動する（GOOGLE_CLOUD_PROJECT も不要）",
			envs: validLocalEnv,
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, IAPModeLocal, cfg.IAPMode)
				assert.Empty(t, cfg.GoogleCloudProject, "local mode では GOOGLE_CLOUD_PROJECT 不要")
				assert.Empty(t, cfg.AppleKeyID, "IAP 設定欠落でも起動する")
			},
		},
		{
			name: "デフォルト値が適用される（Port / Env / topic 名）",
			envs: validLocalEnv,
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 9006, cfg.Port)
				assert.Equal(t, "dev", cfg.Env)
				assert.Equal(t, "faction-selected", cfg.FactionSelectedTopic)
				assert.Equal(t, "premium-updated", cfg.PremiumUpdatedTopic)
			},
		},
		{
			name: "IAP 設定が env から Config に伝搬する",
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
			name: "PORT の env override",
			envs: mergeEnv(validLocalEnv, map[string]string{"PORT": "8080"}),
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 8080, cfg.Port)
			},
		},
		{
			name: "ENV の env override",
			envs: mergeEnv(validLocalEnv, map[string]string{"ENV": "staging"}),
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "staging", cfg.Env)
			},
		},
		{
			// topic 名はクロスプロジェクトテスト用に override 可能という設計意図を固定
			name: "topic 名の env override",
			envs: mergeEnv(validLocalEnv, map[string]string{
				"FACTION_SELECTED_TOPIC": "faction-selected-ci",
				"PREMIUM_UPDATED_TOPIC":  "premium-updated-ci",
			}),
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "faction-selected-ci", cfg.FactionSelectedTopic)
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

func TestFromEnv_Errors(t *testing.T) {
	tests := []struct {
		name    string
		envs    map[string]string
		wantErr string
	}{
		{
			name:    "DATABASE_URL が未設定で fail",
			envs:    mergeEnv(validLocalEnv, map[string]string{"DATABASE_URL": ""}),
			wantErr: "DATABASE_URL is required",
		},
		{
			name:    "PUBSUB_PROJECT_ID が未設定で fail",
			envs:    mergeEnv(validLocalEnv, map[string]string{"PUBSUB_PROJECT_ID": ""}),
			wantErr: "PUBSUB_PROJECT_ID is required",
		},
		{
			name:    "FIRESTORE_PROJECT_ID が未設定で fail",
			envs:    mergeEnv(validLocalEnv, map[string]string{"FIRESTORE_PROJECT_ID": ""}),
			wantErr: "FIRESTORE_PROJECT_ID is required",
		},
		{
			name:    "IAP_MODE が未設定で fail（デフォルトにフォールバックしない）",
			envs:    mergeEnv(validLocalEnv, map[string]string{"IAP_MODE": ""}),
			wantErr: "IAP_MODE must be",
		},
		{
			name:    "IAP_MODE が未定義値で fail",
			envs:    mergeEnv(validLocalEnv, map[string]string{"IAP_MODE": "invalid"}),
			wantErr: "IAP_MODE must be",
		},
		{
			name: "production mode で GOOGLE_CLOUD_PROJECT 未設定で fail",
			envs: mergeEnv(validLocalEnv, map[string]string{
				"IAP_MODE":             "production",
				"GOOGLE_CLOUD_PROJECT": "",
			}),
			wantErr: "GOOGLE_CLOUD_PROJECT is required when IAP_MODE=production",
		},
		{
			name:    "PORT が数値として解釈できない",
			envs:    mergeEnv(validLocalEnv, map[string]string{"PORT": "not-a-number"}),
			wantErr: "PORT",
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
