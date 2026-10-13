package apple_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/adaptertest"
	"github.com/kenyamaneko/overload-party-shop/internal/adapter/apple/appletest"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const (
	subscriptionResponseBody       = `{"data":[{"lastTransactions":[{"signedTransactionInfo":"signed-txn"}]}]}`
	emptyDataResponseBody          = `{"data":[]}`
	emptyLastTransactionsBody      = `{"data":[{"lastTransactions":[]}]}`
	subscriptionTransactionInfo    = `{"bundleId":"com.test.shop","productId":"prod-sub","expiresDate":1800000000000}`
	otherBundleSubscriptionInfo    = `{"bundleId":"com.other.app","productId":"prod-sub","expiresDate":1800000000000}`
	subscriptionWithoutExpiresDate = `{"bundleId":"com.test.shop","productId":"prod-sub"}`
	subscriptionWithZeroExpires    = `{"bundleId":"com.test.shop","productId":"prod-sub","expiresDate":0}`
)

func verifySubscriptionWith(t *testing.T, endpoint string, jwsVerifier port.AppleJWSVerifier) (*port.SubscriptionInfo, error) {
	t.Helper()
	return appletest.NewVerifier(t, endpoint, jwsVerifier).VerifySubscription(context.Background(), "sub-token")
}

func TestVerifier_VerifySubscription(t *testing.T) {
	t.Run("[Apple購入検証]サブスクリプションのレシート検証", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("AppStoreServerAPIがステータスコード200で署名付き取引情報を返し、署名検証を通り、取引情報のbundleIdが検証器に渡したバンドルIDと同じとき", func(t *testing.T) {
				verifyWithTransaction := func(t *testing.T) *port.SubscriptionInfo {
					t.Helper()
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))
					got, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo})
					require.NoError(t, err)
					require.NotNil(t, got)
					return got
				}

				t.Run("検証結果が有効になる", func(t *testing.T) {
					got := verifyWithTransaction(t)

					assert.True(t, got.IsValid)
				})

				t.Run("取引情報がproductId=prod-subのとき、商品IDはprod-subになる", func(t *testing.T) {
					got := verifyWithTransaction(t)

					assert.Equal(t, "prod-sub", got.ProductID)
				})

				t.Run("取引情報がexpiresDate=1800000000000のとき、有効期限は2027-01-15T08:00:00Zになる", func(t *testing.T) {
					got := verifyWithTransaction(t)

					assert.Equal(t, "2027-01-15T08:00:00Z", got.ExpiresAt.UTC().Format(time.RFC3339))
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("サブスクリプションを検証すると", func(t *testing.T) {
				errJWSVerification := errors.New("jws verification failed")

				invalidResultTests := []struct {
					name            string
					body            string
					transactionInfo string
				}{
					{name: "AppStoreServerAPIが返すdataが空の配列であるとき、検証結果が無効になる", body: emptyDataResponseBody, transactionInfo: subscriptionTransactionInfo},
					{name: "AppStoreServerAPIが返すdataの先頭のlastTransactionsが空の配列であるとき、検証結果が無効になる", body: emptyLastTransactionsBody, transactionInfo: subscriptionTransactionInfo},
					{name: "署名検証を通った取引情報のbundleIdがcom.other.appであるとき、検証結果が無効になる", body: subscriptionResponseBody, transactionInfo: otherBundleSubscriptionInfo},
				}
				for _, tc := range invalidResultTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, tc.body))

						got, _ := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: tc.transactionInfo})

						require.NotNil(t, got)
						assert.False(t, got.IsValid)
					})
				}

				noErrorTests := []struct {
					name            string
					body            string
					transactionInfo string
				}{
					{name: "AppStoreServerAPIが返すdataが空の配列であるとき、エラーは返らない", body: emptyDataResponseBody, transactionInfo: subscriptionTransactionInfo},
					{name: "AppStoreServerAPIが返すdataの先頭のlastTransactionsが空の配列であるとき、エラーは返らない", body: emptyLastTransactionsBody, transactionInfo: subscriptionTransactionInfo},
					{name: "署名検証を通った取引情報のbundleIdがcom.other.appであるとき、エラーは返らない", body: subscriptionResponseBody, transactionInfo: otherBundleSubscriptionInfo},
				}
				for _, tc := range noErrorTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, tc.body))

						_, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: tc.transactionInfo})

						assert.NoError(t, err)
					})
				}

				statusCodeTests := []struct {
					name       string
					statusCode int
					body       string
					want       string
				}{
					{name: "AppStoreServerAPIがステータスコード201を返すとき、エラーの内容にステータスコードが含まれる", statusCode: 201, body: "created-body", want: "201"},
					{name: "AppStoreServerAPIがステータスコード404を返すとき、エラーの内容にステータスコードが含まれる", statusCode: 404, body: "not-found-body", want: "404"},
				}
				for _, tc := range statusCodeTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(tc.statusCode, tc.body))

						_, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo})

						assert.ErrorContains(t, err, tc.want)
					})
				}

				t.Run("AppStoreServerAPIがステータスコード404と応答本文not-found-bodyを返すとき、エラーの内容に応答本文not-found-bodyが含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(404, "not-found-body"))

					_, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo})

					assert.ErrorContains(t, err, "not-found-body")
				})

				noResultTests := []struct {
					name        string
					startServer func(t *testing.T) string
					jwsVerifier port.AppleJWSVerifier
				}{
					{
						name:        "AppStoreServerAPIがステータスコード201を返すとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(201, "created-body"),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo},
					},
					{
						name:        "AppStoreServerAPIがステータスコード404を返すとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(404, "not-found-body"),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo},
					},
					{
						name:        "AppStoreServerAPIに接続できないとき、検証結果は返らない",
						startServer: adaptertest.NewUnreachableURL,
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo},
					},
					{
						name:        "応答本文がJSONでないとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(http.StatusOK, "not-json"),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo},
					},
					{
						name:        "取引情報の署名検証がエラーを返すとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(http.StatusOK, subscriptionResponseBody),
						jwsVerifier: &appletest.FakeJWSVerifier{Err: errJWSVerification},
					},
					{
						name:        "署名検証を通った取引情報がJSONでないとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(http.StatusOK, subscriptionResponseBody),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: "not-json"},
					},
					{
						name:        "署名検証を通った取引情報のexpiresDateが無いとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(http.StatusOK, subscriptionResponseBody),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: subscriptionWithoutExpiresDate},
					},
					{
						name:        "署名検証を通った取引情報のexpiresDateが0であるとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(http.StatusOK, subscriptionResponseBody),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: subscriptionWithZeroExpires},
					},
				}
				for _, tc := range noResultTests {
					t.Run(tc.name, func(t *testing.T) {
						got, _ := verifySubscriptionWith(t, tc.startServer(t), tc.jwsVerifier)

						assert.Nil(t, got)
					})
				}

				t.Run("AppStoreServerAPIに接続できないとき、エラーの内容に要求失敗が含まれる", func(t *testing.T) {
					_, err := verifySubscriptionWith(t, adaptertest.NewUnreachableURL(t), &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo})

					assert.ErrorContains(t, err, "request to Apple")
				})

				t.Run("応答本文がJSONでないとき、エラーの内容に応答デコード失敗が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, "not-json"))

					_, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: subscriptionTransactionInfo})

					assert.ErrorContains(t, err, "decode Apple subscription response")
				})

				t.Run("取引情報の署名検証がエラーを返すとき、エラーの内容に取引情報の読み取り失敗が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

					_, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Err: errJWSVerification})

					assert.ErrorContains(t, err, "decode transaction info")
				})

				t.Run("取引情報の署名検証がエラーを返すとき、署名検証が返したエラーが原因として取り出せる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

					_, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Err: errJWSVerification})

					assert.ErrorIs(t, err, errJWSVerification)
				})

				t.Run("署名検証を通った取引情報がJSONでないとき、エラーの内容に取引情報の解析失敗が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

					_, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: "not-json"})

					assert.ErrorContains(t, err, "unmarshal transaction info")
				})

				missingExpiresDateTests := []struct {
					name            string
					transactionInfo string
				}{
					{name: "署名検証を通った取引情報のexpiresDateが無いとき、エラーの内容に有効期限の欠落が含まれる", transactionInfo: subscriptionWithoutExpiresDate},
					{name: "署名検証を通った取引情報のexpiresDateが0であるとき、エラーの内容に有効期限の欠落が含まれる", transactionInfo: subscriptionWithZeroExpires},
				}
				for _, tc := range missingExpiresDateTests {
					t.Run(tc.name, func(t *testing.T) {
						server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, subscriptionResponseBody))

						_, err := verifySubscriptionWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: tc.transactionInfo})

						assert.ErrorContains(t, err, "expiresDate")
					})
				}
			})
		})
	})
}
