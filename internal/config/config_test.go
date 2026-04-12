package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

func TestFromEnv_LocalMode_Minimal(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":    "postgres://localhost/test",
		"PUBSUB_PROJECT_ID": "test-project",
		"IAP_MODE":        "local",
	})

	cfg, err := FromEnv()
	require.NoError(t, err)
	assert.Equal(t, IAPModeLocal, cfg.IAPMode)
	assert.Equal(t, 9006, cfg.Port)
	assert.Equal(t, "dev", cfg.Env)
	assert.Equal(t, "faction-selected", cfg.FactionSelectedTopic)
	assert.Equal(t, "premium-updated", cfg.PremiumUpdatedTopic)
}

func TestFromEnv_LocalMode_WithAppleEnvVars(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":         "postgres://localhost/test",
		"PUBSUB_PROJECT_ID":   "test-project",
		"IAP_MODE":            "local",
		"APPLE_KEY_ID":        "KEY123",
		"APPLE_ISSUER_ID":     "ISS456",
		"APPLE_BUNDLE_ID":     "com.test.app",
		"APPLE_PRIVATE_KEY_PATH": "/tmp/test.p8",
		"GOOGLE_PACKAGE_NAME": "com.test.android",
	})

	cfg, err := FromEnv()
	require.NoError(t, err)
	assert.Equal(t, "KEY123", cfg.AppleKeyID)
	assert.Equal(t, "ISS456", cfg.AppleIssuerID)
	assert.Equal(t, "com.test.app", cfg.AppleBundleID)
	assert.Equal(t, "/tmp/test.p8", cfg.ApplePrivateKeyPath)
	assert.Equal(t, "com.test.android", cfg.GooglePackageName)
}

func TestFromEnv_MissingDatabaseURL(t *testing.T) {
	setEnvs(t, map[string]string{
		"PUBSUB_PROJECT_ID": "test-project",
		"IAP_MODE":          "local",
	})
	// DATABASE_URL は意図的に未設定。
	os.Unsetenv("DATABASE_URL")

	_, err := FromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL is required")
}

func TestFromEnv_MissingPubsubProjectID(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL": "postgres://localhost/test",
		"IAP_MODE":     "local",
	})
	os.Unsetenv("PUBSUB_PROJECT_ID")

	_, err := FromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PUBSUB_PROJECT_ID is required")
}

func TestFromEnv_InvalidIAPMode(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":    "postgres://localhost/test",
		"PUBSUB_PROJECT_ID": "test-project",
		"IAP_MODE":        "invalid",
	})

	_, err := FromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IAP_MODE must be")
}

func TestFromEnv_ProductionMode_MissingGCPProject(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":    "postgres://localhost/test",
		"PUBSUB_PROJECT_ID": "test-project",
		"IAP_MODE":        "production",
	})
	os.Unsetenv("GCP_PROJECT")

	_, err := FromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GCP_PROJECT is required when IAP_MODE=production")
}

func TestFromEnv_CustomPort(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":    "postgres://localhost/test",
		"PUBSUB_PROJECT_ID": "test-project",
		"IAP_MODE":        "local",
		"PORT":            "8080",
	})

	cfg, err := FromEnv()
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Port)
}

func TestFromEnv_InvalidPort(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":    "postgres://localhost/test",
		"PUBSUB_PROJECT_ID": "test-project",
		"IAP_MODE":        "local",
		"PORT":            "not-a-number",
	})

	_, err := FromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORT")
}

func TestFromEnv_DefaultIAPModeIsProduction(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":    "postgres://localhost/test",
		"PUBSUB_PROJECT_ID": "test-project",
	})
	// IAP_MODE 未設定 → デフォルトは production → GCP_PROJECT 必須
	os.Unsetenv("IAP_MODE")
	os.Unsetenv("GCP_PROJECT")

	_, err := FromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GCP_PROJECT is required when IAP_MODE=production")
}
