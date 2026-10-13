package google_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/adaptertest"
	"github.com/kenyamaneko/overload-party-shop/internal/adapter/google/googletest"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const (
	purchasedResponseBody     = `{"purchaseState":0}`
	subscriptionResponseBody  = `{"expiryTimeMillis":"1800000000000"}`
	zeroExpiryResponseBody    = `{"expiryTimeMillis":"0"}`
	missingExpiryResponseBody = `{}`
)

func verifyPurchase(t *testing.T, server *httptest.Server, token string) (*port.VerifyResult, error) {
	t.Helper()
	return googletest.NewVerifier(t, server.URL).VerifyPurchase(context.Background(), token)
}

func verifySubscription(t *testing.T, server *httptest.Server, token string) (*port.SubscriptionInfo, error) {
	t.Helper()
	return googletest.NewVerifier(t, server.URL).VerifySubscription(context.Background(), token)
}

func getSubscriptionExpiry(t *testing.T, server *httptest.Server, token string) (time.Time, error) {
	t.Helper()
	return googletest.NewSubVerifier(t, server.URL).GetSubscriptionExpiry(context.Background(), token)
}

func TestVerifier_VerifyPurchase(t *testing.T) {
	t.Run("[GooglePlay購入検証]購入トークンの商品IDとトークンへの分割", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("単発購入を検証すると", func(t *testing.T) {
				noResultTests := []struct {
					name  string
					token string
				}{
					{name: "トークンが区切り文字の:が無い文字列no-separatorのとき、検証結果は返らない", token: "no-separator"},
					{name: "トークンが空文字のとき、検証結果は返らない", token: ""},
					{name: "トークンが:opaque-token（商品ID部分が空）のとき、検証結果は返らない", token: ":opaque-token"},
					{name: "トークンがprod-1:（トークン部分が空）のとき、検証結果は返らない", token: "prod-1:"},
				}
				for _, tc := range noResultTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchasedResponseBody))

						got, _ := verifyPurchase(t, server, tc.token)

						assert.Nil(t, got)
					})
				}

				invalidFormatTests := []struct {
					name  string
					token string
				}{
					{name: "トークンが区切り文字の:が無い文字列no-separatorのとき、エラーの内容にトークン形式不正が含まれる", token: "no-separator"},
					{name: "トークンが空文字のとき、エラーの内容にトークン形式不正が含まれる", token: ""},
					{name: "トークンが:opaque-token（商品ID部分が空）のとき、エラーの内容にトークン形式不正が含まれる", token: ":opaque-token"},
					{name: "トークンがprod-1:（トークン部分が空）のとき、エラーの内容にトークン形式不正が含まれる", token: "prod-1:"},
				}
				for _, tc := range invalidFormatTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchasedResponseBody))

						_, err := verifyPurchase(t, server, tc.token)

						assert.ErrorContains(t, err, "invalid Google purchase token format")
					})
				}
			})
		})
	})

	t.Run("[GooglePlay購入検証]単発購入のレシート検証", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("GooglePlayAPIがpurchaseState=0を返すとき", func(t *testing.T) {
				t.Run("検証結果が有効になる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchasedResponseBody))

					got, err := verifyPurchase(t, server, "prod-1:opaque-token")

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.True(t, got.IsValid)
				})

				t.Run("トークンがprod-1:opaque-tokenのとき、商品IDはprod-1になる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchasedResponseBody))

					got, err := verifyPurchase(t, server, "prod-1:opaque-token")

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, "prod-1", got.ProductID)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("単発購入を検証すると", func(t *testing.T) {
				invalidResultTests := []struct {
					name string
					body string
				}{
					{name: "GooglePlayAPIがpurchaseState=1(キャンセル)を返すとき、検証結果が無効になる", body: `{"purchaseState":1}`},
					{name: "GooglePlayAPIがpurchaseState=2(保留)を返すとき、検証結果が無効になる", body: `{"purchaseState":2}`},
				}
				for _, tc := range invalidResultTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, tc.body))

						got, _ := verifyPurchase(t, server, "prod-1:opaque-token")

						require.NotNil(t, got)
						assert.False(t, got.IsValid)
					})
				}

				t.Run("GooglePlayAPIがステータスコード404を返すとき、検証結果は返らない", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(404, "{}"))

					got, _ := verifyPurchase(t, server, "prod-1:opaque-token")

					assert.Nil(t, got)
				})

				noErrorTests := []struct {
					name string
					body string
				}{
					{name: "GooglePlayAPIがpurchaseState=1(キャンセル)を返すとき、エラーは返らない", body: `{"purchaseState":1}`},
					{name: "GooglePlayAPIがpurchaseState=2(保留)を返すとき、エラーは返らない", body: `{"purchaseState":2}`},
				}
				for _, tc := range noErrorTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, tc.body))

						_, err := verifyPurchase(t, server, "prod-1:opaque-token")

						assert.NoError(t, err)
					})
				}

				t.Run("GooglePlayAPIがステータスコード404を返すとき、エラーの内容にステータスコード404が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(404, "{}"))

					_, err := verifyPurchase(t, server, "prod-1:opaque-token")

					assert.ErrorContains(t, err, "404")
				})
			})
		})
	})
}

func TestVerifier_VerifySubscription(t *testing.T) {
	t.Run("[GooglePlay購入検証]サブスクリプションのレシート検証", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("GooglePlayAPIが0でないexpiryTimeMillisを含むステータスコード200を返すとき", func(t *testing.T) {
				t.Run("検証結果が有効になる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

					got, err := verifySubscription(t, server, "sub-1:opaque-token")

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.True(t, got.IsValid)
				})

				t.Run("トークンがsub-1:opaque-tokenのとき、商品IDはsub-1になる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

					got, err := verifySubscription(t, server, "sub-1:opaque-token")

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, "sub-1", got.ProductID)
				})

				t.Run("応答のexpiryTimeMillisが1800000000000のとき、有効期限は2027-01-15T08:00:00Zになる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

					got, err := verifySubscription(t, server, "sub-1:opaque-token")

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, "2027-01-15T08:00:00Z", got.ExpiresAt.UTC().Format(time.RFC3339))
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("サブスクリプションを検証すると", func(t *testing.T) {
				noResultTests := []struct {
					name   string
					status int
					body   string
					token  string
				}{
					{name: "GooglePlayAPIがステータスコード500を返すとき、検証結果は返らない", status: 500, body: "{}", token: "sub-1:opaque-token"},
					{name: "トークンに区切り文字の:が無いとき、検証結果は返らない", status: http.StatusOK, body: subscriptionResponseBody, token: "no-separator"},
					{name: "GooglePlayAPIが返す応答のexpiryTimeMillisが0であるとき、検証結果は返らない", status: http.StatusOK, body: zeroExpiryResponseBody, token: "sub-1:opaque-token"},
					{name: "GooglePlayAPIが返す応答にexpiryTimeMillisが無いとき、検証結果は返らない", status: http.StatusOK, body: missingExpiryResponseBody, token: "sub-1:opaque-token"},
				}
				for _, tc := range noResultTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(tc.status, tc.body))

						got, _ := verifySubscription(t, server, tc.token)

						assert.Nil(t, got)
					})
				}

				t.Run("GooglePlayAPIがステータスコード500を返すとき、エラーの内容にステータスコード500が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(500, "{}"))

					_, err := verifySubscription(t, server, "sub-1:opaque-token")

					assert.ErrorContains(t, err, "500")
				})

				t.Run("トークンに区切り文字の:が無いとき、エラーの内容にトークン形式不正が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

					_, err := verifySubscription(t, server, "no-separator")

					assert.ErrorContains(t, err, "invalid Google purchase token format")
				})

				missingExpiryTests := []struct {
					name string
					body string
				}{
					{name: "GooglePlayAPIが返す応答のexpiryTimeMillisが0であるとき、エラーの内容に有効期限の欠落が含まれる", body: zeroExpiryResponseBody},
					{name: "GooglePlayAPIが返す応答のexpiryTimeMillisが無いとき、エラーの内容に有効期限の欠落が含まれる", body: missingExpiryResponseBody},
				}
				for _, tc := range missingExpiryTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, tc.body))

						_, err := verifySubscription(t, server, "sub-1:opaque-token")

						assert.ErrorContains(t, err, "expiryTimeMillis")
					})
				}
			})
		})
	})
}

func TestSubVerifier_GetSubscriptionExpiry(t *testing.T) {
	t.Run("[GooglePlay購入検証]サブスクリプション有効期限の取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("GooglePlayAPIがexpiryTimeMillis=1800000000000を返すとき、有効期限は2027-01-15T08:00:00Zになる", func(t *testing.T) {
				server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

				got, err := getSubscriptionExpiry(t, server, "sub-1:opaque-token")

				require.NoError(t, err)
				assert.Equal(t, "2027-01-15T08:00:00Z", got.UTC().Format(time.RFC3339))
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("有効期限を取得すると", func(t *testing.T) {
				unsetTimeTests := []struct {
					name   string
					status int
					body   string
					token  string
				}{
					{name: "GooglePlayAPIがステータスコード500を返すとき、有効期限は未設定の時刻になる", status: 500, body: "{}", token: "sub-1:opaque-token"},
					{name: "トークンに区切り文字の:が無いとき、有効期限は未設定の時刻になる", status: http.StatusOK, body: subscriptionResponseBody, token: "no-separator"},
					{name: "GooglePlayAPIが返す応答のexpiryTimeMillisが0であるとき、有効期限は未設定の時刻になる", status: http.StatusOK, body: zeroExpiryResponseBody, token: "sub-1:opaque-token"},
					{name: "GooglePlayAPIが返す応答にexpiryTimeMillisが無いとき、有効期限は未設定の時刻になる", status: http.StatusOK, body: missingExpiryResponseBody, token: "sub-1:opaque-token"},
				}
				for _, tc := range unsetTimeTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(tc.status, tc.body))

						got, _ := getSubscriptionExpiry(t, server, tc.token)

						assert.Equal(t, time.Time{}, got)
					})
				}

				t.Run("GooglePlayAPIがステータスコード500を返すとき、エラーの内容にステータスコード500が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(500, "{}"))

					_, err := getSubscriptionExpiry(t, server, "sub-1:opaque-token")

					assert.ErrorContains(t, err, "500")
				})

				t.Run("トークンに区切り文字の:が無いとき、エラーの内容にトークン形式不正が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

					_, err := getSubscriptionExpiry(t, server, "no-separator")

					assert.ErrorContains(t, err, "invalid Google purchase token format")
				})

				missingExpiryTests := []struct {
					name string
					body string
				}{
					{name: "GooglePlayAPIが返す応答のexpiryTimeMillisが0であるとき、エラーの内容に有効期限の欠落が含まれる", body: zeroExpiryResponseBody},
					{name: "GooglePlayAPIが返す応答のexpiryTimeMillisが無いとき、エラーの内容に有効期限の欠落が含まれる", body: missingExpiryResponseBody},
				}
				for _, tc := range missingExpiryTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, tc.body))

						_, err := getSubscriptionExpiry(t, server, "sub-1:opaque-token")

						assert.ErrorContains(t, err, "expiryTimeMillis")
					})
				}
			})
		})
	})
}
