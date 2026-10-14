package apishopclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopclient"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopserverfake"
)

func newStubServer(t *testing.T, status int, responseBody string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func newClient(t *testing.T, baseURL string, opts ...apishopclient.Option) *apishopclient.Client {
	t.Helper()
	client, err := apishopclient.New(baseURL, opts...)
	require.NoError(t, err)
	return client
}

func newFakeServer(t *testing.T) *apishopserverfake.Server {
	t.Helper()
	server := apishopserverfake.NewServer()
	t.Cleanup(server.Close)
	return server
}

func respondWithStatusOnly(status int) func(apishop.PurchaseRequest) (int, any) {
	return func(apishop.PurchaseRequest) (int, any) {
		return status, nil
	}
}

func newPurchaseRequest(platform string) apishop.PurchaseRequest {
	return apishop.PurchaseRequest{
		ProductID:     "TST-0001",
		Platform:      apishop.Platform(platform),
		PurchaseToken: "TST-TOKEN-0001",
	}
}

func TestStatusCodeToError(t *testing.T) {
	t.Run("[商品APIクライアント]ステータスコードからのエラー判別", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("商品購入がエラーのステータスを受けたとき", func(t *testing.T) {
				cases := []struct {
					name   string
					status int
					want   error
				}{
					{name: "400を受けたとき、不正なリクエストを示すエラーになる", status: 400, want: apishopclient.ErrBadRequest},
					{name: "401を受けたとき、認証の失敗を示すエラーになる", status: 401, want: apishopclient.ErrUnauthorized},
					{name: "402を受けたとき、購入の検証の失敗を示すエラーになる", status: 402, want: apishopclient.ErrPaymentRequired},
					{name: "404を受けたとき、商品が見つからないことを示すエラーになる", status: 404, want: apishopclient.ErrNotFound},
					{name: "409を受けたとき、商品を既に所有していることを示すエラーになる", status: 409, want: apishopclient.ErrConflict},
					{name: "500を受けたとき、サーバー内部の障害を示すエラーになる", status: 500, want: apishopclient.ErrInternalServer},
					{name: "503を受けたとき、サーバー内部の障害を示すエラーになる", status: 503, want: apishopclient.ErrInternalServer},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						server := newFakeServer(t)
						server.PurchaseFn = respondWithStatusOnly(tc.status)
						client := newClient(t, server.URL())

						_, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

						assert.ErrorIs(t, err, tc.want)
					})
				}
			})

			t.Run("商品購入が定義済みでないステータスを受けたとき", func(t *testing.T) {
				detailCases := []struct {
					name       string
					status     int
					wantDetail string
				}{
					{name: "499を受けたとき、エラーの内容にステータスコード499が含まれる", status: 499, wantDetail: "unexpected status 499"},
					{name: "403を受けたとき、エラーの内容にステータスコード403が含まれる", status: 403, wantDetail: "unexpected status 403"},
				}
				unmatchedCases := []struct {
					name   string
					status int
				}{
					{name: "499を受けたとき、エラーは400・401・402・404・409・500・503を受けたときに返るどのエラーにも一致しない", status: 499},
					{name: "403を受けたとき、エラーは400・401・402・404・409・500・503を受けたときに返るどのエラーにも一致しない", status: 403},
				}
				definedErrors := []error{
					apishopclient.ErrBadRequest,
					apishopclient.ErrUnauthorized,
					apishopclient.ErrPaymentRequired,
					apishopclient.ErrNotFound,
					apishopclient.ErrConflict,
					apishopclient.ErrInternalServer,
				}
				for _, tc := range detailCases {
					t.Run(tc.name, func(t *testing.T) {
						server := newFakeServer(t)
						server.PurchaseFn = respondWithStatusOnly(tc.status)
						client := newClient(t, server.URL())

						_, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

						assert.ErrorContains(t, err, tc.wantDetail)
					})
				}
				for _, tc := range unmatchedCases {
					t.Run(tc.name, func(t *testing.T) {
						server := newFakeServer(t)
						server.PurchaseFn = respondWithStatusOnly(tc.status)
						client := newClient(t, server.URL())

						_, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

						for _, defined := range definedErrors {
							assert.NotErrorIs(t, err, defined)
						}
					})
				}
			})
		})
	})
}

func TestListPlayerProducts(t *testing.T) {
	t.Run("[商品APIクライアント]商品一覧の取得", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("サーバーが200で商品TST-0001を含む一覧を返したとき、返された一覧に商品TST-0001が含まれる", func(t *testing.T) {
				server := newFakeServer(t)
				server.GetProductsFn = func() (int, any) {
					return http.StatusOK, apishop.ProductListResponse{Products: []apishop.ProductResponse{
						{ProductID: "TST-0001", Name: "TST-NAME-0001", Type: "subscription", Content: map[string]any{}},
					}}
				}
				client := newClient(t, server.URL())

				list, err := client.ListPlayerProducts(context.Background())

				require.NoError(t, err)
				productIDs := []string{}
				for _, product := range list.Products {
					productIDs = append(productIDs, product.ProductID)
				}
				assert.Contains(t, productIDs, "TST-0001")
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("401を受けたとき、認証の失敗を示すエラーになる", func(t *testing.T) {
				server := newFakeServer(t)
				server.GetProductsFn = func() (int, any) {
					return http.StatusUnauthorized, nil
				}
				client := newClient(t, server.URL())

				_, err := client.ListPlayerProducts(context.Background())

				assert.ErrorIs(t, err, apishopclient.ErrUnauthorized)
			})
		})
	})
}

func TestPurchaseProduct(t *testing.T) {
	t.Run("[商品APIクライアント]商品の購入", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("サーバーが200でproduct_idのTST-0001を返したとき、返された購入結果のproduct_idはTST-0001になる", func(t *testing.T) {
				server := newFakeServer(t)
				server.PurchaseFn = func(apishop.PurchaseRequest) (int, any) {
					return http.StatusOK, apishop.PurchaseResponse{Message: "TST-MESSAGE-0001", ProductID: "TST-0001"}
				}
				client := newClient(t, server.URL())

				result, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

				require.NoError(t, err)
				assert.Equal(t, "TST-0001", result.ProductID)
			})
		})
	})
}

func TestSubscribeProduct(t *testing.T) {
	t.Run("[商品APIクライアント]サブスクリプションの購入", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("サーバーが200でexpires_atの2026-05-01T12:00:00Zを返したとき、返された結果のexpires_atは2026-05-01T12:00:00Zになる", func(t *testing.T) {
				server := newFakeServer(t)
				server.SubscribeFn = func(apishop.PurchaseRequest) (int, any) {
					return http.StatusOK, apishop.SubscribeResponse{
						Message:   "TST-MESSAGE-0001",
						ExpiresAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
					}
				}
				client := newClient(t, server.URL())

				result, err := client.SubscribeProduct(context.Background(), newPurchaseRequest("android"))

				require.NoError(t, err)
				assert.Equal(t, "2026-05-01T12:00:00Z", result.ExpiresAt.UTC().Format(time.RFC3339))
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("402を受けたとき、購入の検証の失敗を示すエラーになる", func(t *testing.T) {
				server := newFakeServer(t)
				server.SubscribeFn = respondWithStatusOnly(http.StatusPaymentRequired)
				client := newClient(t, server.URL())

				_, err := client.SubscribeProduct(context.Background(), newPurchaseRequest("android"))

				assert.ErrorIs(t, err, apishopclient.ErrPaymentRequired)
			})
		})
	})
}

func TestGetHealth(t *testing.T) {
	t.Run("[商品APIクライアント]死活監視", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("サーバーが200でstatusのokを返したとき、返された結果のstatusはokになる", func(t *testing.T) {
				baseURL := newStubServer(t, http.StatusOK, `{"status":"ok"}`)
				client := newClient(t, baseURL)

				health, err := client.GetHealth(context.Background())

				require.NoError(t, err)
				assert.Equal(t, "ok", health.Status)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("500を受けたとき、サーバー内部の障害を示すエラーになる", func(t *testing.T) {
				baseURL := newStubServer(t, http.StatusInternalServerError, "")
				client := newClient(t, baseURL)

				_, err := client.GetHealth(context.Background())

				assert.ErrorIs(t, err, apishopclient.ErrInternalServer)
			})
		})
	})
}

func TestForwardStoreNotification(t *testing.T) {
	t.Run("[商品APIクライアント]ストア通知の転送", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("Appleの通知を送り、サーバーが200を返したとき、エラーにならない", func(t *testing.T) {
				baseURL := newStubServer(t, http.StatusOK, "")
				client := newClient(t, baseURL)

				err := client.AppleWebhook(context.Background(), apishop.AppleNotificationPayload{SignedPayload: "TST-JWS-0001"})

				assert.NoError(t, err)
			})

			t.Run("Googleの通知を送り、サーバーが200を返したとき、エラーにならない", func(t *testing.T) {
				baseURL := newStubServer(t, http.StatusOK, "")
				client := newClient(t, baseURL)

				err := client.GoogleWebhook(context.Background(), apishop.GoogleRTDNMessage{
					Message: apishop.GoogleRTDNMessageBody{Data: "TST-DATA-0001"},
				})

				assert.NoError(t, err)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("Appleの通知にサーバーが500を返したとき、サーバー内部の障害を示すエラーになる", func(t *testing.T) {
				baseURL := newStubServer(t, http.StatusInternalServerError, "")
				client := newClient(t, baseURL)

				err := client.AppleWebhook(context.Background(), apishop.AppleNotificationPayload{SignedPayload: "TST-JWS-0001"})

				assert.ErrorIs(t, err, apishopclient.ErrInternalServer)
			})

			t.Run("Googleの通知にサーバーが500を返したとき、サーバー内部の障害を示すエラーになる", func(t *testing.T) {
				baseURL := newStubServer(t, http.StatusInternalServerError, "")
				client := newClient(t, baseURL)

				err := client.GoogleWebhook(context.Background(), apishop.GoogleRTDNMessage{
					Message: apishop.GoogleRTDNMessageBody{Data: "TST-DATA-0001"},
				})

				assert.ErrorIs(t, err, apishopclient.ErrInternalServer)
			})
		})
	})
}
