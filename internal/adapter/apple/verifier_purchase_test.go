package apple_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/adaptertest"
	"github.com/kenyamaneko/overload-party-shop/internal/adapter/apple"
	"github.com/kenyamaneko/overload-party-shop/internal/adapter/apple/appletest"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

const (
	purchaseResponseBody      = `{"signedTransactionInfo":"signed-txn"}`
	purchaseTransactionInfo   = `{"bundleId":"com.test.shop","productId":"prod-1"}`
	otherBundleTransaction    = `{"bundleId":"com.other.app","productId":"prod-1"}`
	maxTokenLifetimeInSeconds = 60 * 60
)

type decodedToken struct {
	header       map[string]any
	claims       map[string]any
	registered   jwt.RegisteredClaims
	signingInput string
	signature    string
}

func decodeTokenSegment(t *testing.T, segment string, into any) {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(segment)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, into))
}

func decodeToken(t *testing.T, token string) decodedToken {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	decoded := decodedToken{
		signingInput: parts[0] + "." + parts[1],
		signature:    parts[2],
	}
	decodeTokenSegment(t, parts[0], &decoded.header)
	decodeTokenSegment(t, parts[1], &decoded.claims)
	decodeTokenSegment(t, parts[1], &decoded.registered)
	return decoded
}

func requestAuthorization(t *testing.T, server *adaptertest.Server) string {
	t.Helper()
	requests := server.Requests()
	require.Len(t, requests, 1)
	return requests[0].Header.Get("Authorization")
}

func verifyPurchaseWithAuthentication(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))
	key := appletest.GenerateECDSAKey(t, elliptic.P256())
	jwsVerifier := &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo}
	verifier, err := appletest.NewVerifierFromPEM(appletest.EncodePKCS8PEM(t, key), "Sandbox", jwsVerifier, apple.WithEndpoint(server.URL))
	require.NoError(t, err)

	_, err = verifier.VerifyPurchase(context.Background(), "purchase-token")

	require.NoError(t, err)
	return strings.TrimPrefix(requestAuthorization(t, server), "Bearer "), key
}

func TestVerifier_VerifyPurchase(t *testing.T) {
	t.Run("[Apple購入検証]単発購入のレシート検証", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("AppStoreServerAPIがステータスコード200で署名付き取引情報を返し、署名検証を通り、取引情報のbundleIdが検証器に渡したバンドルIDと同じとき", func(t *testing.T) {
				verifyWithTransaction := func(t *testing.T, transactionInfo string) (*port.VerifyResult, error) {
					t.Helper()
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))
					verifier := appletest.NewVerifier(t, server.URL, &appletest.FakeJWSVerifier{Payload: transactionInfo})
					return verifier.VerifyPurchase(context.Background(), "purchase-token")
				}

				t.Run("検証結果が有効になる", func(t *testing.T) {
					got, err := verifyWithTransaction(t, purchaseTransactionInfo)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.True(t, got.IsValid)
				})

				t.Run("取引情報がproductId=prod-1のとき、商品IDはprod-1になる", func(t *testing.T) {
					got, err := verifyWithTransaction(t, purchaseTransactionInfo)

					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, "prod-1", got.ProductID)
				})
			})

			t.Run("購入トークンがpurchase-tokenのとき、AppStoreServerAPIの/inApps/v2/transactions/purchase-tokenにGETで要求が届く", func(t *testing.T) {
				server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))
				verifier := appletest.NewVerifier(t, server.URL, &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo})

				_, err := verifier.VerifyPurchase(context.Background(), "purchase-token")

				require.NoError(t, err)
				requests := server.Requests()
				require.Len(t, requests, 1)
				assert.Equal(t, "GET", requests[0].Method)
				assert.Equal(t, "/inApps/v2/transactions/purchase-token", requests[0].EscapedPath)
			})

			t.Run("購入トークンがtoken/with?specialのとき、AppStoreServerAPIの/inApps/v2/transactions/token%2Fwith%3FspecialにGETで要求が届く", func(t *testing.T) {
				server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))
				verifier := appletest.NewVerifier(t, server.URL, &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo})

				_, err := verifier.VerifyPurchase(context.Background(), "token/with?special")

				require.NoError(t, err)
				requests := server.Requests()
				require.Len(t, requests, 1)
				assert.Equal(t, "GET", requests[0].Method)
				assert.Equal(t, "/inApps/v2/transactions/token%2Fwith%3Fspecial", requests[0].EscapedPath)
			})

			t.Run("応答のsignedTransactionInfoがsigned-txnのとき、署名検証にはsigned-txnが渡される", func(t *testing.T) {
				server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))
				jwsVerifier := &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo}
				verifier := appletest.NewVerifier(t, server.URL, jwsVerifier)

				_, err := verifier.VerifyPurchase(context.Background(), "purchase-token")

				require.NoError(t, err)
				assert.Equal(t, []string{"signed-txn"}, jwsVerifier.Received)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("単発購入を検証すると", func(t *testing.T) {
				errJWSVerification := errors.New("jws verification failed")
				verifyWith := func(t *testing.T, endpoint string, jwsVerifier port.AppleJWSVerifier) (*port.VerifyResult, error) {
					t.Helper()
					return appletest.NewVerifier(t, endpoint, jwsVerifier).VerifyPurchase(context.Background(), "purchase-token")
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

						_, err := verifyWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo})

						assert.ErrorContains(t, err, tc.want)
					})
				}

				t.Run("AppStoreServerAPIがステータスコード404と応答本文not-found-bodyを返すとき、エラーの内容に応答本文not-found-bodyが含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(404, "not-found-body"))

					_, err := verifyWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo})

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
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo},
					},
					{
						name:        "AppStoreServerAPIがステータスコード404を返すとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(404, "not-found-body"),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo},
					},
					{
						name:        "AppStoreServerAPIに接続できないとき、検証結果は返らない",
						startServer: adaptertest.NewUnreachableURL,
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo},
					},
					{
						name:        "応答本文がJSONでないとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(http.StatusOK, "not-json"),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo},
					},
					{
						name:        "署名検証がエラーを返すとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(http.StatusOK, purchaseResponseBody),
						jwsVerifier: &appletest.FakeJWSVerifier{Err: errJWSVerification},
					},
					{
						name:        "署名検証を通った取引情報がJSONでないとき、検証結果は返らない",
						startServer: adaptertest.NewServerStarter(http.StatusOK, purchaseResponseBody),
						jwsVerifier: &appletest.FakeJWSVerifier{Payload: "not-json"},
					},
				}
				for _, tc := range noResultTests {
					t.Run(tc.name, func(t *testing.T) {
						got, _ := verifyWith(t, tc.startServer(t), tc.jwsVerifier)

						assert.Nil(t, got)
					})
				}

				t.Run("署名検証を通った取引情報のbundleIdがcom.other.appのとき、検証結果が無効になる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))

					got, _ := verifyWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: otherBundleTransaction})

					require.NotNil(t, got)
					assert.False(t, got.IsValid)
				})

				t.Run("署名検証を通った取引情報のbundleIdがcom.other.appのとき、エラーは返らない", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))

					_, err := verifyWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: otherBundleTransaction})

					assert.NoError(t, err)
				})

				t.Run("AppStoreServerAPIに接続できないとき、エラーの内容に要求失敗が含まれる", func(t *testing.T) {
					_, err := verifyWith(t, adaptertest.NewUnreachableURL(t), &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo})

					assert.ErrorContains(t, err, "request to Apple")
				})

				t.Run("応答本文がJSONでないとき、エラーの内容に応答デコード失敗が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, "not-json"))

					_, err := verifyWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo})

					assert.ErrorContains(t, err, "decode Apple response")
				})

				t.Run("署名検証がエラーを返すとき、署名検証が返したエラーが原因として取り出せる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))

					_, err := verifyWith(t, server.URL, &appletest.FakeJWSVerifier{Err: errJWSVerification})

					assert.ErrorIs(t, err, errJWSVerification)
				})

				t.Run("署名検証を通った取引情報がJSONでないとき、エラーの内容に取引情報の解析失敗が含まれる", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))

					_, err := verifyWith(t, server.URL, &appletest.FakeJWSVerifier{Payload: "not-json"})

					assert.ErrorContains(t, err, "unmarshal transaction info")
				})
			})
		})
	})

	t.Run("[Apple購入検証]AppStoreServerAPIへの要求に付ける認証トークン", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("単発購入を検証すると", func(t *testing.T) {
				t.Run("AppStoreServerAPIへの要求のAuthorizationヘッダは、Bearerに続けてトークンが付く", func(t *testing.T) {
					server := adaptertest.NewServer(t, adaptertest.RespondWith(http.StatusOK, purchaseResponseBody))
					verifier := appletest.NewVerifier(t, server.URL, &appletest.FakeJWSVerifier{Payload: purchaseTransactionInfo})

					_, err := verifier.VerifyPurchase(context.Background(), "purchase-token")

					require.NoError(t, err)
					assert.Regexp(t, `^Bearer [A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`, requestAuthorization(t, server))
				})

				t.Run("AppStoreServerAPIへの要求のトークンのヘッダのalgは、ES256になる", func(t *testing.T) {
					token, _ := verifyPurchaseWithAuthentication(t)

					assert.Equal(t, "ES256", decodeToken(t, token).header["alg"])
				})

				t.Run("検証器の生成にキーIDを渡したとき、要求のトークンのヘッダのkidは、渡したキーIDになる", func(t *testing.T) {
					token, _ := verifyPurchaseWithAuthentication(t)

					assert.Equal(t, "test-key-id", decodeToken(t, token).header["kid"])
				})

				t.Run("検証器の生成に発行者IDを渡したとき、要求のトークンのissは、渡した発行者IDになる", func(t *testing.T) {
					token, _ := verifyPurchaseWithAuthentication(t)

					assert.Equal(t, "test-issuer-id", decodeToken(t, token).claims["iss"])
				})

				t.Run("検証器の生成にバンドルIDを渡したとき、要求のトークンのbidは、渡したバンドルIDになる", func(t *testing.T) {
					token, _ := verifyPurchaseWithAuthentication(t)

					assert.Equal(t, "com.test.shop", decodeToken(t, token).claims["bid"])
				})

				t.Run("AppStoreServerAPIへの要求のトークンのaudは、appstoreconnect-v1になる", func(t *testing.T) {
					token, _ := verifyPurchaseWithAuthentication(t)

					assert.Equal(t, jwt.ClaimStrings{"appstoreconnect-v1"}, decodeToken(t, token).registered.Audience)
				})

				t.Run("AppStoreServerAPIへの要求のトークンのexpは、iatの60分後以内になる", func(t *testing.T) {
					token, _ := verifyPurchaseWithAuthentication(t)

					registered := decodeToken(t, token).registered
					require.NotNil(t, registered.IssuedAt)
					require.NotNil(t, registered.ExpiresAt)
					assert.LessOrEqual(t, registered.ExpiresAt.Unix(), registered.IssuedAt.Unix()+maxTokenLifetimeInSeconds)
				})

				t.Run("検証器の生成に秘密鍵を渡したとき、要求のトークンの署名は、渡した秘密鍵に対応する公開鍵で検証できる", func(t *testing.T) {
					token, key := verifyPurchaseWithAuthentication(t)

					decoded := decodeToken(t, token)
					err := jwt.SigningMethodES256.Verify(decoded.signingInput, decoded.signature, &key.PublicKey)
					assert.NoError(t, err)
				})
			})
		})
	})
}
