//go:build integration

package config_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"maps"
	"os"
	"testing"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kenyamaneko/overload-party-shop/internal/config"
	"github.com/kenyamaneko/overload-party-shop/internal/config/configtest"
)

const testProjectEnvKey = "SHOP_TEST_GOOGLE_CLOUD_PROJECT"

const (
	secretAppleKeyID      = "shop-apple-key-id"
	secretAppleIssuerID   = "shop-apple-issuer-id"
	secretAppleBundleID   = "shop-apple-bundle-id"
	secretGooglePackage   = "shop-google-package-name"
	secretApplePrivateKey = "shop-apple-private-key"
)

var allSecretNames = []string{
	secretAppleKeyID,
	secretAppleIssuerID,
	secretAppleBundleID,
	secretGooglePackage,
	secretApplePrivateKey,
}

func newApplePrivateKeyPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func newSecretManagerClient(t *testing.T) *secretmanager.Client {
	t.Helper()
	client, err := secretmanager.NewClient(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func deleteSecret(t *testing.T, client *secretmanager.Client, project, name string) {
	t.Helper()
	err := client.DeleteSecret(context.Background(), &secretmanagerpb.DeleteSecretRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s", project, name),
	})
	require.Contains(t, []codes.Code{codes.OK, codes.NotFound}, status.Code(err))
}

func addSecrets(t *testing.T, client *secretmanager.Client, project string, values map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	for name, value := range values {
		secret, err := client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
			Parent:   "projects/" + project,
			SecretId: name,
			Secret: &secretmanagerpb.Secret{
				Replication: &secretmanagerpb.Replication{
					Replication: &secretmanagerpb.Replication_Automatic_{
						Automatic: &secretmanagerpb.Replication_Automatic{},
					},
				},
			},
		})
		require.NoError(t, err)
		_, err = client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
			Parent:  secret.GetName(),
			Payload: &secretmanagerpb.SecretPayload{Data: value},
		})
		require.NoError(t, err)
		t.Cleanup(func() { deleteSecret(t, client, project, name) })
	}
}

func registerSecrets(t *testing.T, client *secretmanager.Client, project string, values map[string][]byte) {
	t.Helper()
	for _, name := range allSecretNames {
		deleteSecret(t, client, project, name)
	}
	addSecrets(t, client, project, values)
}

func withoutSecret(values map[string][]byte, name string) map[string][]byte {
	remaining := maps.Clone(values)
	delete(remaining, name)
	return remaining
}

func TestFromEnvStoreSecrets(t *testing.T) {
	t.Run("[起動設定]ストア認証情報の取得", func(t *testing.T) {
		t.Run("IAP_VERIFIERがstoreで、PubSubプッシュ用の環境変数が両方設定され、GOOGLE_CLOUD_PROJECTが実在するテスト用プロジェクトのとき", func(t *testing.T) {
			project := os.Getenv(testProjectEnvKey)
			require.NotEmpty(t, project, "%s にテスト用の Google Cloud プロジェクトを指定する必要がある", testProjectEnvKey)
			client := newSecretManagerClient(t)

			privateKeyPEM := newApplePrivateKeyPEM(t)
			secrets := map[string][]byte{
				secretAppleKeyID:      []byte("TST-KEY-ID"),
				secretAppleIssuerID:   []byte("TST-ISSUER-ID"),
				secretAppleBundleID:   []byte("TST-BUNDLE-ID"),
				secretGooglePackage:   []byte("TST-PACKAGE-NAME"),
				secretApplePrivateKey: privateKeyPEM,
			}
			buildEnv := func(overrides map[string]string) map[string]string {
				env := map[string]string{"GOOGLE_CLOUD_PROJECT": project}
				maps.Copy(env, overrides)
				return configtest.StoreEnvWith(env)
			}

			t.Run("正常系", func(t *testing.T) {
				t.Run("5つの秘密情報がすべて登録されているとき", func(t *testing.T) {
					registerSecrets(t, client, project, secrets)

					configtest.RunReadCases(t, buildEnv, []configtest.ReadCase{
						{
							Name: "秘密情報shop-apple-key-idの値が、AppleのキーIDになる",
							Read: func(cfg *config.Config) any { return cfg.AppleKeyID },
							Want: "TST-KEY-ID",
						},
						{
							Name: "秘密情報shop-apple-issuer-idの値が、Appleの発行者IDになる",
							Read: func(cfg *config.Config) any { return cfg.AppleIssuerID },
							Want: "TST-ISSUER-ID",
						},
						{
							Name: "秘密情報shop-apple-bundle-idの値が、AppleのバンドルIDになる",
							Read: func(cfg *config.Config) any { return cfg.AppleBundleID },
							Want: "TST-BUNDLE-ID",
						},
						{
							Name: "秘密情報shop-google-package-nameの値が、Googleのパッケージ名になる",
							Read: func(cfg *config.Config) any { return cfg.GooglePackageName },
							Want: "TST-PACKAGE-NAME",
						},
						{
							Name: "秘密情報shop-apple-private-keyの値が、Appleの秘密鍵になる",
							Read: func(cfg *config.Config) any { return cfg.ApplePrivateKeyPEM },
							Want: privateKeyPEM,
						},
						{
							Name: "APPLE_ENVIRONMENTがProductionのとき、Apple環境がProductionになる",
							Env:  map[string]string{"APPLE_ENVIRONMENT": "Production"},
							Read: func(cfg *config.Config) any { return cfg.AppleEnvironment },
							Want: "Production",
						},
						{
							Name: "APPLE_ENVIRONMENTがSandboxのとき、Apple環境がSandboxになる",
							Env:  map[string]string{"APPLE_ENVIRONMENT": "Sandbox"},
							Read: func(cfg *config.Config) any { return cfg.AppleEnvironment },
							Want: "Sandbox",
						},
					})
				})
			})

			t.Run("異常系", func(t *testing.T) {
				t.Run("5つのうち1つだけ登録されていないとき", func(t *testing.T) {
					cases := []struct {
						name    string
						missing string
					}{
						{
							name:    "秘密情報shop-apple-key-idのとき、shop-apple-key-idを含むエラーになる",
							missing: "shop-apple-key-id",
						},
						{
							name:    "秘密情報shop-apple-issuer-idのとき、shop-apple-issuer-idを含むエラーになる",
							missing: "shop-apple-issuer-id",
						},
						{
							name:    "秘密情報shop-apple-bundle-idのとき、shop-apple-bundle-idを含むエラーになる",
							missing: "shop-apple-bundle-id",
						},
						{
							name:    "秘密情報shop-google-package-nameのとき、shop-google-package-nameを含むエラーになる",
							missing: "shop-google-package-name",
						},
						{
							name:    "秘密情報shop-apple-private-keyのとき、shop-apple-private-keyを含むエラーになる",
							missing: "shop-apple-private-key",
						},
					}
					for _, tt := range cases {
						t.Run(tt.name, func(t *testing.T) {
							registerSecrets(t, client, project, withoutSecret(secrets, tt.missing))

							configtest.AssertConfigError(t, buildEnv(nil), []string{tt.missing})
						})
					}

					causeCases := []struct {
						name    string
						missing string
					}{
						{
							name:    "秘密情報shop-apple-key-idのとき、エラーの原因が未登録になる",
							missing: "shop-apple-key-id",
						},
						{
							name:    "秘密情報shop-apple-issuer-idのとき、エラーの原因が未登録になる",
							missing: "shop-apple-issuer-id",
						},
						{
							name:    "秘密情報shop-apple-bundle-idのとき、エラーの原因が未登録になる",
							missing: "shop-apple-bundle-id",
						},
						{
							name:    "秘密情報shop-google-package-nameのとき、エラーの原因が未登録になる",
							missing: "shop-google-package-name",
						},
						{
							name:    "秘密情報shop-apple-private-keyのとき、エラーの原因が未登録になる",
							missing: "shop-apple-private-key",
						},
					}
					for _, tt := range causeCases {
						t.Run(tt.name, func(t *testing.T) {
							registerSecrets(t, client, project, withoutSecret(secrets, tt.missing))

							_, err := configtest.LoadConfig(t, buildEnv(nil))
							require.Error(t, err)
							assert.Equal(t, codes.NotFound, status.Code(err))
						})
					}
				})

				t.Run("秘密情報shop-apple-key-idが登録されていないために読み込みが失敗した後、shop-apple-key-idを登録してから再度読み込むと、AppleのキーIDがTST-KEY-IDになる", func(t *testing.T) {
					registerSecrets(t, client, project, withoutSecret(secrets, secretAppleKeyID))
					configtest.SetEnv(t, buildEnv(nil))
					_, err := config.FromEnv()
					require.Error(t, err)

					addSecrets(t, client, project, map[string][]byte{secretAppleKeyID: secrets[secretAppleKeyID]})

					cfg, err := config.FromEnv()
					require.NoError(t, err)
					assert.Equal(t, "TST-KEY-ID", cfg.AppleKeyID)
				})
			})
		})
	})
}
