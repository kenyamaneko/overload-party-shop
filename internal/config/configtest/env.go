// Package configtest は起動設定のテストで使う環境変数の組み立てを提供するテストヘルパ。
package configtest

import "testing"

var envKeys = []string{
	"PORT",
	"DATABASE_CONN",
	"DATABASE_IAM_AUTH_ENABLED",
	"CLOUDSQL_CONNECTION_NAME",
	"GOOGLE_CLOUD_PROJECT",
	"CARD_PACK_PURCHASED_TOPIC",
	"FACTION_ACQUIRED_TOPIC",
	"PREMIUM_UPDATED_TOPIC",
	"INTERNAL_AUTH_PUBLIC_KEY",
	"IAP_VERIFIER",
	"LOG_MODE",
	"APPLE_ENVIRONMENT",
	"PUBSUB_PUSH_SERVICE_ACCOUNT_EMAIL",
	"PUBSUB_PUSH_AUDIENCE",
	"OUTBOX_POLL_INTERVAL",
	"OUTBOX_BATCH_SIZE",
	"OUTBOX_FAILURE_THRESHOLD",
	"OUTBOX_VISIBILITY_TIMEOUT",
}

// StubEnvWith は IAP_VERIFIER が stub で必須の環境変数がすべて有効な値の環境変数に、overrides を上書きして返す。
func StubEnvWith(overrides map[string]string) map[string]string {
	env := map[string]string{
		"PORT":                      "8080",
		"DATABASE_CONN":             "host=tst-default-host port=5432 dbname=tst_default user=tst_default password=TST-DEFAULT-PASSWORD sslmode=disable",
		"DATABASE_IAM_AUTH_ENABLED": "false",
		"GOOGLE_CLOUD_PROJECT":      "tst-default-project",
		"CARD_PACK_PURCHASED_TOPIC": "tst-default-card-pack-topic",
		"FACTION_ACQUIRED_TOPIC":    "tst-default-faction-topic",
		"PREMIUM_UPDATED_TOPIC":     "tst-default-premium-topic",
		"INTERNAL_AUTH_PUBLIC_KEY":  "-----BEGIN PUBLIC KEY-----\nTST-DEFAULT-PUBLIC-KEY\n-----END PUBLIC KEY-----\n",
		"IAP_VERIFIER":              "stub",
		"LOG_MODE":                  "local",
		"OUTBOX_POLL_INTERVAL":      "2s",
		"OUTBOX_BATCH_SIZE":         "50",
		"OUTBOX_FAILURE_THRESHOLD":  "3",
		"OUTBOX_VISIBILITY_TIMEOUT": "10s",
	}
	return merge(env, overrides)
}

// StoreEnvWith は IAP_VERIFIER が store でストア連携用の環境変数がすべて有効な値の環境変数に、overrides を上書きして返す。
func StoreEnvWith(overrides map[string]string) map[string]string {
	env := StubEnvWith(map[string]string{
		"IAP_VERIFIER":                      "store",
		"APPLE_ENVIRONMENT":                 "Sandbox",
		"PUBSUB_PUSH_SERVICE_ACCOUNT_EMAIL": "tst-push@tst-default-project.iam.gserviceaccount.com",
		"PUBSUB_PUSH_AUDIENCE":              "https://tst-shop.example.com/webhook/google",
	})
	return merge(env, overrides)
}

// SetEnv は起動設定が読むすべての環境変数を空にしてから、env の値を設定する。テストの終了時に元へ戻る。
func SetEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range envKeys {
		t.Setenv(key, "")
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
}

func merge(base, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overrides))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}
