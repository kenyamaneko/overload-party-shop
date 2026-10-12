package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/config"
	"github.com/kenyamaneko/overload-party-shop/internal/config/configtest"
)

const stubEnvGiven = "IAP_VERIFIERがstubで、他の必須環境変数がすべて有効な値で設定されているとき"

func TestFromEnv(t *testing.T) {
	t.Run("[起動設定]必須の環境変数", func(t *testing.T) {
		t.Run(stubEnvGiven, func(t *testing.T) {
			t.Run("異常系", func(t *testing.T) {
				t.Run("必須の環境変数が未設定のとき", func(t *testing.T) {
					configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
						{
							Name:         "PORTが未設定のとき、PORTが必須であることを示すエラーになる",
							Env:          map[string]string{"PORT": ""},
							WantContains: []string{"PORT", "is required"},
						},
						{
							Name:         "DATABASE_CONNが未設定のとき、DATABASE_CONNが必須であることを示すエラーになる",
							Env:          map[string]string{"DATABASE_CONN": ""},
							WantContains: []string{"DATABASE_CONN", "is required"},
						},
						{
							Name:         "GOOGLE_CLOUD_PROJECTが未設定のとき、GOOGLE_CLOUD_PROJECTが必須であることを示すエラーになる",
							Env:          map[string]string{"GOOGLE_CLOUD_PROJECT": ""},
							WantContains: []string{"GOOGLE_CLOUD_PROJECT", "is required"},
						},
						{
							Name:         "CARD_PACK_PURCHASED_TOPICが未設定のとき、CARD_PACK_PURCHASED_TOPICが必須であることを示すエラーになる",
							Env:          map[string]string{"CARD_PACK_PURCHASED_TOPIC": ""},
							WantContains: []string{"CARD_PACK_PURCHASED_TOPIC", "is required"},
						},
						{
							Name:         "FACTION_ACQUIRED_TOPICが未設定のとき、FACTION_ACQUIRED_TOPICが必須であることを示すエラーになる",
							Env:          map[string]string{"FACTION_ACQUIRED_TOPIC": ""},
							WantContains: []string{"FACTION_ACQUIRED_TOPIC", "is required"},
						},
						{
							Name:         "PREMIUM_UPDATED_TOPICが未設定のとき、PREMIUM_UPDATED_TOPICが必須であることを示すエラーになる",
							Env:          map[string]string{"PREMIUM_UPDATED_TOPIC": ""},
							WantContains: []string{"PREMIUM_UPDATED_TOPIC", "is required"},
						},
						{
							Name:         "INTERNAL_AUTH_PUBLIC_KEYが未設定のとき、INTERNAL_AUTH_PUBLIC_KEYが必須であることを示すエラーになる",
							Env:          map[string]string{"INTERNAL_AUTH_PUBLIC_KEY": ""},
							WantContains: []string{"INTERNAL_AUTH_PUBLIC_KEY", "is required"},
						},
					})
				})

				t.Run("PORTがabcのとき、PORTと入力値abcを含むエラーになる", func(t *testing.T) {
					configtest.AssertConfigError(t,
						configtest.StubEnvWith(map[string]string{"PORT": "abc"}),
						[]string{"PORT", "abc"},
					)
				})

				t.Run("PORTが数値でも範囲の外のとき", func(t *testing.T) {
					configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
						{
							Name:         "PORTが0のとき、PORTと入力値0と許容範囲を含むエラーになる",
							Env:          map[string]string{"PORT": "0"},
							WantContains: []string{"PORT", "0", "1", "65535"},
						},
						{
							Name:         "PORTが65536のとき、PORTと入力値65536と許容範囲を含むエラーになる",
							Env:          map[string]string{"PORT": "65536"},
							WantContains: []string{"PORT", "65536", "1", "65535"},
						},
					})
				})
			})
		})
	})

	t.Run("[起動設定]環境変数の値の読み取り", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run(stubEnvGiven, func(t *testing.T) {
				const dummyConn = "host=tst-db-host port=5432 dbname=tst_db user=tst_user password=TST-PASSWORD sslmode=disable"
				const dummyPublicKey = "-----BEGIN PUBLIC KEY-----\nTST-DUMMY-LINE-1\nTST-DUMMY-LINE-2\n-----END PUBLIC KEY-----\n"
				configtest.RunReadCases(t, configtest.StubEnvWith, []configtest.ReadCase{
					{
						Name: "PORTが9006のとき、待ち受けポートが9006になる",
						Env:  map[string]string{"PORT": "9006"},
						Read: func(cfg *config.Config) any { return cfg.Port },
						Want: 9006,
					},
					{
						Name: "PORTが1のとき、待ち受けポートが1になる",
						Env:  map[string]string{"PORT": "1"},
						Read: func(cfg *config.Config) any { return cfg.Port },
						Want: 1,
					},
					{
						Name: "PORTが65535のとき、待ち受けポートが65535になる",
						Env:  map[string]string{"PORT": "65535"},
						Read: func(cfg *config.Config) any { return cfg.Port },
						Want: 65535,
					},
					{
						Name: "DATABASE_CONNがダミーの接続文字列のとき、データベース接続文字列が同じ文字列になる",
						Env:  map[string]string{"DATABASE_CONN": dummyConn},
						Read: func(cfg *config.Config) any { return cfg.DatabaseConn },
						Want: dummyConn,
					},
					{
						Name: "GOOGLE_CLOUD_PROJECTがtest-projectのとき、GoogleCloudプロジェクトがtest-projectになる",
						Env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "test-project"},
						Read: func(cfg *config.Config) any { return cfg.GoogleCloudProject },
						Want: "test-project",
					},
					{
						Name: "CARD_PACK_PURCHASED_TOPICがtest-card-pack-purchasedのとき、カードパック購入通知のトピックがtest-card-pack-purchasedになる",
						Env:  map[string]string{"CARD_PACK_PURCHASED_TOPIC": "test-card-pack-purchased"},
						Read: func(cfg *config.Config) any { return cfg.CardPackPurchasedTopic },
						Want: "test-card-pack-purchased",
					},
					{
						Name: "FACTION_ACQUIRED_TOPICがtest-faction-acquiredのとき、陣営獲得通知のトピックがtest-faction-acquiredになる",
						Env:  map[string]string{"FACTION_ACQUIRED_TOPIC": "test-faction-acquired"},
						Read: func(cfg *config.Config) any { return cfg.FactionAcquiredTopic },
						Want: "test-faction-acquired",
					},
					{
						Name: "PREMIUM_UPDATED_TOPICがtest-premium-updatedのとき、プレミアム更新通知のトピックがtest-premium-updatedになる",
						Env:  map[string]string{"PREMIUM_UPDATED_TOPIC": "test-premium-updated"},
						Read: func(cfg *config.Config) any { return cfg.PremiumUpdatedTopic },
						Want: "test-premium-updated",
					},
					{
						Name: "INTERNAL_AUTH_PUBLIC_KEYが改行を含むダミーのPEM文字列のとき、内部認証の公開鍵が改行を含めて同じ文字列になる",
						Env:  map[string]string{"INTERNAL_AUTH_PUBLIC_KEY": dummyPublicKey},
						Read: func(cfg *config.Config) any { return cfg.InternalAuthPublicKey },
						Want: dummyPublicKey,
					},
				})
			})
		})
	})

	t.Run("[起動設定]データベースのIAM認証", func(t *testing.T) {
		t.Run(stubEnvGiven, func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("DATABASE_IAM_AUTH_ENABLEDがfalseのとき、IAM認証は無効になる", func(t *testing.T) {
					cfg, err := configtest.LoadConfig(t, configtest.StubEnvWith(map[string]string{
						"DATABASE_IAM_AUTH_ENABLED": "false",
					}))
					require.NoError(t, err)
					assert.False(t, cfg.DatabaseIAMAuthEnabled)
				})

				t.Run("DATABASE_IAM_AUTH_ENABLEDがfalseでCLOUDSQL_CONNECTION_NAMEが設定されているとき、CloudSQL接続名は空になる", func(t *testing.T) {
					cfg, err := configtest.LoadConfig(t, configtest.StubEnvWith(map[string]string{
						"DATABASE_IAM_AUTH_ENABLED": "false",
						"CLOUDSQL_CONNECTION_NAME":  "tst-project:tst-region:tst-instance",
					}))
					require.NoError(t, err)
					assert.Empty(t, cfg.CloudSQLConnectionName)
				})

				t.Run("DATABASE_IAM_AUTH_ENABLEDがtrueでCLOUDSQL_CONNECTION_NAMEが設定されているとき", func(t *testing.T) {
					env := configtest.StubEnvWith(map[string]string{
						"DATABASE_IAM_AUTH_ENABLED": "true",
						"CLOUDSQL_CONNECTION_NAME":  "tst-project:tst-region:tst-instance",
					})

					t.Run("IAM認証は有効になる", func(t *testing.T) {
						cfg, err := configtest.LoadConfig(t, env)
						require.NoError(t, err)
						assert.True(t, cfg.DatabaseIAMAuthEnabled)
					})

					t.Run("CloudSQL接続名がCLOUDSQL_CONNECTION_NAMEの値になる", func(t *testing.T) {
						cfg, err := configtest.LoadConfig(t, env)
						require.NoError(t, err)
						assert.Equal(t, "tst-project:tst-region:tst-instance", cfg.CloudSQLConnectionName)
					})
				})
			})

			t.Run("異常系", func(t *testing.T) {
				t.Run("DATABASE_IAM_AUTH_ENABLEDがtrueでもfalseでもないとき", func(t *testing.T) {
					configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
						{
							Name:         "DATABASE_IAM_AUTH_ENABLEDが未設定のとき、DATABASE_IAM_AUTH_ENABLEDの許容値を示すエラーになる",
							Env:          map[string]string{"DATABASE_IAM_AUTH_ENABLED": ""},
							WantContains: []string{"DATABASE_IAM_AUTH_ENABLED", "true", "false"},
						},
						{
							Name:         "DATABASE_IAM_AUTH_ENABLEDがyesのとき、DATABASE_IAM_AUTH_ENABLEDの許容値と入力値yesを含むエラーになる",
							Env:          map[string]string{"DATABASE_IAM_AUTH_ENABLED": "yes"},
							WantContains: []string{"DATABASE_IAM_AUTH_ENABLED", "true", "false", "yes"},
						},
					})
				})

				t.Run("DATABASE_IAM_AUTH_ENABLEDがtrueでCLOUDSQL_CONNECTION_NAMEが未設定のとき、CLOUDSQL_CONNECTION_NAMEが必須であることを示すエラーになる", func(t *testing.T) {
					configtest.AssertConfigError(t,
						configtest.StubEnvWith(map[string]string{
							"DATABASE_IAM_AUTH_ENABLED": "true",
							"CLOUDSQL_CONNECTION_NAME":  "",
						}),
						[]string{"CLOUDSQL_CONNECTION_NAME", "is required"},
					)
				})
			})
		})
	})

	t.Run("[起動設定]ログ出力方式", func(t *testing.T) {
		t.Run(stubEnvGiven, func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				configtest.RunReadCases(t, configtest.StubEnvWith, []configtest.ReadCase{
					{
						Name: "LOG_MODEがlocalのとき、ログ出力方式がlocalになる",
						Env:  map[string]string{"LOG_MODE": "local"},
						Read: func(cfg *config.Config) any { return cfg.LogMode },
						Want: config.LogMode("local"),
					},
					{
						Name: "LOG_MODEがproductionのとき、ログ出力方式がproductionになる",
						Env:  map[string]string{"LOG_MODE": "production"},
						Read: func(cfg *config.Config) any { return cfg.LogMode },
						Want: config.LogMode("production"),
					},
				})
			})

			t.Run("異常系", func(t *testing.T) {
				configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
					{
						Name:         "LOG_MODEが未設定のとき、LOG_MODEの許容値を示すエラーになる",
						Env:          map[string]string{"LOG_MODE": ""},
						WantContains: []string{"LOG_MODE", "production", "local"},
					},
					{
						Name:         "LOG_MODEがinvalidのとき、LOG_MODEの許容値と入力値invalidを含むエラーになる",
						Env:          map[string]string{"LOG_MODE": "invalid"},
						WantContains: []string{"LOG_MODE", "production", "local", "invalid"},
					},
				})
			})
		})
	})

	t.Run("[起動設定]レシート検証方式", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("IAP_VERIFIERがstubで、他の必須環境変数がすべて有効な値で、ストア連携用の環境変数が一つも設定されていないとき、レシート検証方式がstubになる", func(t *testing.T) {
				cfg, err := configtest.LoadConfig(t, configtest.StubEnvWith(map[string]string{
					"IAP_VERIFIER":                      "stub",
					"APPLE_ENVIRONMENT":                 "",
					"PUBSUB_PUSH_SERVICE_ACCOUNT_EMAIL": "",
					"PUBSUB_PUSH_AUDIENCE":              "",
				}))
				require.NoError(t, err)
				assert.Equal(t, config.IAPVerifier("stub"), cfg.IAPVerifier)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("他の必須環境変数がすべて有効な値で設定されているとき", func(t *testing.T) {
				t.Run("IAP_VERIFIERが不正なとき", func(t *testing.T) {
					configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
						{
							Name:         "IAP_VERIFIERが未設定のとき、IAP_VERIFIERの許容値を示すエラーになる",
							Env:          map[string]string{"IAP_VERIFIER": ""},
							WantContains: []string{"IAP_VERIFIER", "store", "stub"},
						},
						{
							Name:         "IAP_VERIFIERがinvalidのとき、IAP_VERIFIERの許容値と入力値invalidを含むエラーになる",
							Env:          map[string]string{"IAP_VERIFIER": "invalid"},
							WantContains: []string{"IAP_VERIFIER", "store", "stub", "invalid"},
						},
					})
				})

				t.Run("IAP_VERIFIERがstoreのとき", func(t *testing.T) {
					t.Run("PubSubプッシュ用の環境変数の片方だけが未設定のとき", func(t *testing.T) {
						configtest.RunErrorCases(t, configtest.StoreEnvWith, []configtest.ErrorCase{
							{
								Name:         "PUBSUB_PUSH_SERVICE_ACCOUNT_EMAILが未設定のとき、PUBSUB_PUSH_SERVICE_ACCOUNT_EMAILが必須であることを示すエラーになる",
								Env:          map[string]string{"PUBSUB_PUSH_SERVICE_ACCOUNT_EMAIL": ""},
								WantContains: []string{"PUBSUB_PUSH_SERVICE_ACCOUNT_EMAIL", "is required"},
							},
							{
								Name:         "PUBSUB_PUSH_AUDIENCEが未設定のとき、PUBSUB_PUSH_AUDIENCEが必須であることを示すエラーになる",
								Env:          map[string]string{"PUBSUB_PUSH_AUDIENCE": ""},
								WantContains: []string{"PUBSUB_PUSH_AUDIENCE", "is required"},
							},
						})
					})

					t.Run("PubSubプッシュ用の環境変数が両方設定されているとき", func(t *testing.T) {
						configtest.RunErrorCases(t, configtest.StoreEnvWith, []configtest.ErrorCase{
							{
								Name:         "APPLE_ENVIRONMENTが未設定のとき、APPLE_ENVIRONMENTの許容値を示すエラーになる",
								Env:          map[string]string{"APPLE_ENVIRONMENT": ""},
								WantContains: []string{"APPLE_ENVIRONMENT", "Production", "Sandbox"},
							},
							{
								Name:         "APPLE_ENVIRONMENTがstagingのとき、APPLE_ENVIRONMENTの許容値と入力値stagingを含むエラーになる",
								Env:          map[string]string{"APPLE_ENVIRONMENT": "staging"},
								WantContains: []string{"APPLE_ENVIRONMENT", "Production", "Sandbox", "staging"},
							},
						})
					})
				})
			})
		})
	})

	t.Run("[起動設定]outbox送信の設定値", func(t *testing.T) {
		t.Run(stubEnvGiven, func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("OUTBOX_POLL_INTERVAL", func(t *testing.T) {
					configtest.RunReadCases(t, configtest.StubEnvWith, []configtest.ReadCase{
						{
							Name: "OUTBOX_POLL_INTERVALが1sのとき、outboxのポーリング間隔が1秒になる",
							Env:  map[string]string{"OUTBOX_POLL_INTERVAL": "1s"},
							Read: func(cfg *config.Config) any { return cfg.OutboxPollInterval },
							Want: time.Second,
						},
						{
							Name: "OUTBOX_POLL_INTERVALが1nsのとき、outboxのポーリング間隔が1ナノ秒になる",
							Env:  map[string]string{"OUTBOX_POLL_INTERVAL": "1ns"},
							Read: func(cfg *config.Config) any { return cfg.OutboxPollInterval },
							Want: time.Nanosecond,
						},
					})
				})

				t.Run("OUTBOX_BATCH_SIZE", func(t *testing.T) {
					configtest.RunReadCases(t, configtest.StubEnvWith, []configtest.ReadCase{
						{
							Name: "OUTBOX_BATCH_SIZEが100のとき、outboxのバッチサイズが100になる",
							Env:  map[string]string{"OUTBOX_BATCH_SIZE": "100"},
							Read: func(cfg *config.Config) any { return cfg.OutboxBatchSize },
							Want: 100,
						},
						{
							Name: "OUTBOX_BATCH_SIZEが1のとき、outboxのバッチサイズが1になる",
							Env:  map[string]string{"OUTBOX_BATCH_SIZE": "1"},
							Read: func(cfg *config.Config) any { return cfg.OutboxBatchSize },
							Want: 1,
						},
					})
				})

				t.Run("OUTBOX_FAILURE_THRESHOLD", func(t *testing.T) {
					configtest.RunReadCases(t, configtest.StubEnvWith, []configtest.ReadCase{
						{
							Name: "OUTBOX_FAILURE_THRESHOLDが5のとき、outboxの失敗閾値が5になる",
							Env:  map[string]string{"OUTBOX_FAILURE_THRESHOLD": "5"},
							Read: func(cfg *config.Config) any { return cfg.OutboxFailureThreshold },
							Want: 5,
						},
						{
							Name: "OUTBOX_FAILURE_THRESHOLDが1のとき、outboxの失敗閾値が1になる",
							Env:  map[string]string{"OUTBOX_FAILURE_THRESHOLD": "1"},
							Read: func(cfg *config.Config) any { return cfg.OutboxFailureThreshold },
							Want: 1,
						},
					})
				})

				t.Run("OUTBOX_VISIBILITY_TIMEOUT", func(t *testing.T) {
					configtest.RunReadCases(t, configtest.StubEnvWith, []configtest.ReadCase{
						{
							Name: "OUTBOX_VISIBILITY_TIMEOUTが30sのとき、outboxの可視性タイムアウトが30秒になる",
							Env:  map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "30s"},
							Read: func(cfg *config.Config) any { return cfg.OutboxVisibilityTimeout },
							Want: 30 * time.Second,
						},
						{
							Name: "OUTBOX_VISIBILITY_TIMEOUTが1msのとき、outboxの可視性タイムアウトが1ミリ秒になる",
							Env:  map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "1ms"},
							Read: func(cfg *config.Config) any { return cfg.OutboxVisibilityTimeout },
							Want: time.Millisecond,
						},
					})
				})
			})

			t.Run("異常系", func(t *testing.T) {
				t.Run("OUTBOX_POLL_INTERVAL", func(t *testing.T) {
					configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
						{
							Name:         "OUTBOX_POLL_INTERVALが未設定のとき、OUTBOX_POLL_INTERVALが必須であることを示すエラーになる",
							Env:          map[string]string{"OUTBOX_POLL_INTERVAL": ""},
							WantContains: []string{"OUTBOX_POLL_INTERVAL", "is required"},
						},
						{
							Name:         "OUTBOX_POLL_INTERVALがabcのとき、OUTBOX_POLL_INTERVALと入力値abcを含むエラーになる",
							Env:          map[string]string{"OUTBOX_POLL_INTERVAL": "abc"},
							WantContains: []string{"OUTBOX_POLL_INTERVAL", "abc"},
						},
						{
							Name:         "OUTBOX_POLL_INTERVALが0sのとき、OUTBOX_POLL_INTERVALが正の値であることを求めるエラーになる",
							Env:          map[string]string{"OUTBOX_POLL_INTERVAL": "0s"},
							WantContains: []string{"OUTBOX_POLL_INTERVAL", "must be positive"},
						},
					})
				})

				t.Run("OUTBOX_BATCH_SIZE", func(t *testing.T) {
					configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
						{
							Name:         "OUTBOX_BATCH_SIZEが未設定のとき、OUTBOX_BATCH_SIZEが必須であることを示すエラーになる",
							Env:          map[string]string{"OUTBOX_BATCH_SIZE": ""},
							WantContains: []string{"OUTBOX_BATCH_SIZE", "is required"},
						},
						{
							Name:         "OUTBOX_BATCH_SIZEがabcのとき、OUTBOX_BATCH_SIZEと入力値abcを含むエラーになる",
							Env:          map[string]string{"OUTBOX_BATCH_SIZE": "abc"},
							WantContains: []string{"OUTBOX_BATCH_SIZE", "abc"},
						},
						{
							Name:         "OUTBOX_BATCH_SIZEが0のとき、OUTBOX_BATCH_SIZEが正の値であることを求めるエラーになる",
							Env:          map[string]string{"OUTBOX_BATCH_SIZE": "0"},
							WantContains: []string{"OUTBOX_BATCH_SIZE", "must be positive"},
						},
					})
				})

				t.Run("OUTBOX_FAILURE_THRESHOLD", func(t *testing.T) {
					configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
						{
							Name:         "OUTBOX_FAILURE_THRESHOLDが未設定のとき、OUTBOX_FAILURE_THRESHOLDが必須であることを示すエラーになる",
							Env:          map[string]string{"OUTBOX_FAILURE_THRESHOLD": ""},
							WantContains: []string{"OUTBOX_FAILURE_THRESHOLD", "is required"},
						},
						{
							Name:         "OUTBOX_FAILURE_THRESHOLDがabcのとき、OUTBOX_FAILURE_THRESHOLDと入力値abcを含むエラーになる",
							Env:          map[string]string{"OUTBOX_FAILURE_THRESHOLD": "abc"},
							WantContains: []string{"OUTBOX_FAILURE_THRESHOLD", "abc"},
						},
						{
							Name:         "OUTBOX_FAILURE_THRESHOLDが0のとき、OUTBOX_FAILURE_THRESHOLDが正の値であることを求めるエラーになる",
							Env:          map[string]string{"OUTBOX_FAILURE_THRESHOLD": "0"},
							WantContains: []string{"OUTBOX_FAILURE_THRESHOLD", "must be positive"},
						},
					})
				})

				t.Run("OUTBOX_VISIBILITY_TIMEOUT", func(t *testing.T) {
					configtest.RunErrorCases(t, configtest.StubEnvWith, []configtest.ErrorCase{
						{
							Name:         "OUTBOX_VISIBILITY_TIMEOUTが未設定のとき、OUTBOX_VISIBILITY_TIMEOUTが必須であることを示すエラーになる",
							Env:          map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": ""},
							WantContains: []string{"OUTBOX_VISIBILITY_TIMEOUT", "is required"},
						},
						{
							Name:         "OUTBOX_VISIBILITY_TIMEOUTがabcのとき、OUTBOX_VISIBILITY_TIMEOUTと入力値abcを含むエラーになる",
							Env:          map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "abc"},
							WantContains: []string{"OUTBOX_VISIBILITY_TIMEOUT", "abc"},
						},
						{
							Name:         "OUTBOX_VISIBILITY_TIMEOUTが0sのとき、OUTBOX_VISIBILITY_TIMEOUTが1ミリ秒以上であることを求めるエラーになる",
							Env:          map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "0s"},
							WantContains: []string{"OUTBOX_VISIBILITY_TIMEOUT", "must be >= 1ms"},
						},
						{
							Name:         "OUTBOX_VISIBILITY_TIMEOUTが999usのとき、OUTBOX_VISIBILITY_TIMEOUTが1ミリ秒以上であることを求めるエラーになる",
							Env:          map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "999us"},
							WantContains: []string{"OUTBOX_VISIBILITY_TIMEOUT", "must be >= 1ms"},
						},
					})
				})
			})
		})
	})
}
