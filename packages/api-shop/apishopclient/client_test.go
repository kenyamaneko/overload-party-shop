package apishopclient_test

import (
	"context"
	"io"
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

type receivedRequest struct {
	Header http.Header
	Body   []byte
}

func newRecordingServer(t *testing.T, status int, responseBody string) (string, <-chan receivedRequest) {
	t.Helper()
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- receivedRequest{Header: r.Header.Clone(), Body: body}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)
	return server.URL, received
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

const (
	emptyProductListBody = `{"products":[]}`
	purchaseResultBody   = `{"message":"TST-MESSAGE-0001","product_id":"TST-0001"}`
	subscribeResultBody  = `{"message":"TST-MESSAGE-0001","expires_at":"2026-05-01T12:00:00Z"}`
)

func TestStatusCodeToError(t *testing.T) {
	t.Run("[商品APIクライアント]ステータスコードからのエラー判別", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("商品購入がエラーのステータスを受けたとき", func(t *testing.T) {
				cases := []struct {
					name   string
					status int
					want   error
				}{
					{name: "400を受けたとき、ErrBadRequestになる", status: 400, want: apishopclient.ErrBadRequest},
					{name: "401を受けたとき、ErrUnauthorizedになる", status: 401, want: apishopclient.ErrUnauthorized},
					{name: "402を受けたとき、ErrPaymentRequiredになる", status: 402, want: apishopclient.ErrPaymentRequired},
					{name: "404を受けたとき、ErrNotFoundになる", status: 404, want: apishopclient.ErrNotFound},
					{name: "409を受けたとき、ErrConflictになる", status: 409, want: apishopclient.ErrConflict},
					{name: "500を受けたとき、ErrInternalServerになる", status: 500, want: apishopclient.ErrInternalServer},
					{name: "503を受けたとき、ErrInternalServerになる", status: 503, want: apishopclient.ErrInternalServer},
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
				cases := []struct {
					name       string
					status     int
					wantDetail string
				}{
					{name: "499を受けたとき、想定外ステータスのエラーになる", status: 499, wantDetail: "unexpected status 499"},
					{name: "403を受けたとき、想定外ステータスのエラーになる", status: 403, wantDetail: "unexpected status 403"},
				}
				definedErrors := []error{
					apishopclient.ErrBadRequest,
					apishopclient.ErrUnauthorized,
					apishopclient.ErrPaymentRequired,
					apishopclient.ErrNotFound,
					apishopclient.ErrConflict,
					apishopclient.ErrInternalServer,
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						server := newFakeServer(t)
						server.PurchaseFn = respondWithStatusOnly(tc.status)
						client := newClient(t, server.URL())

						_, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

						assert.ErrorContains(t, err, tc.wantDetail)
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

			t.Run("商品一覧を取得したとき、サーバーに届くリクエストにX-Player-Idヘッダは付かない", func(t *testing.T) {
				baseURL, received := newRecordingServer(t, http.StatusOK, emptyProductListBody)
				client := newClient(t, baseURL)

				_, err := client.ListPlayerProducts(context.Background())

				require.NoError(t, err)
				assert.Empty(t, (<-received).Header.Values("X-Player-Id"))
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("401を受けたとき、ErrUnauthorizedになる", func(t *testing.T) {
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
			t.Run("サーバーが200でmessageとproduct_idのTST-0001を返したとき、返された購入結果のproduct_idはTST-0001になる", func(t *testing.T) {
				server := newFakeServer(t)
				server.PurchaseFn = func(apishop.PurchaseRequest) (int, any) {
					return http.StatusOK, apishop.PurchaseResponse{Message: "TST-MESSAGE-0001", ProductID: "TST-0001"}
				}
				client := newClient(t, server.URL())

				result, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

				require.NoError(t, err)
				assert.Equal(t, "TST-0001", result.ProductID)
			})

			t.Run("商品を購入したとき、サーバーに届くリクエストにX-Player-Idヘッダは付かない", func(t *testing.T) {
				baseURL, received := newRecordingServer(t, http.StatusOK, purchaseResultBody)
				client := newClient(t, baseURL)

				_, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

				require.NoError(t, err)
				assert.Empty(t, (<-received).Header.Values("X-Player-Id"))
			})

			t.Run("商品IDがTST-0001・プラットフォームがios・購入トークンがTST-TOKEN-0001で購入したとき、サーバーに届くリクエスト本文は指定した購入内容と同じになる", func(t *testing.T) {
				baseURL, received := newRecordingServer(t, http.StatusOK, purchaseResultBody)
				client := newClient(t, baseURL)

				_, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

				require.NoError(t, err)
				assert.JSONEq(t,
					`{"product_id":"TST-0001","platform":"ios","purchase_token":"TST-TOKEN-0001"}`,
					string((<-received).Body))
			})
		})
	})
}

func TestSubscribeProduct(t *testing.T) {
	t.Run("[商品APIクライアント]サブスクリプションの購入", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("サーバーが200でmessageとexpires_atの2026-05-01T12:00:00Zを返したとき、返された結果のexpires_atは2026-05-01T12:00:00Zになる", func(t *testing.T) {
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

			t.Run("サブスクリプションを購入したとき、サーバーに届くリクエストにX-Player-Idヘッダは付かない", func(t *testing.T) {
				baseURL, received := newRecordingServer(t, http.StatusOK, subscribeResultBody)
				client := newClient(t, baseURL)

				_, err := client.SubscribeProduct(context.Background(), newPurchaseRequest("android"))

				require.NoError(t, err)
				assert.Empty(t, (<-received).Header.Values("X-Player-Id"))
			})

			t.Run("商品IDがTST-0001・プラットフォームがandroid・購入トークンがTST-TOKEN-0001で購入したとき、サーバーに届くリクエスト本文は指定した購入内容と同じになる", func(t *testing.T) {
				baseURL, received := newRecordingServer(t, http.StatusOK, subscribeResultBody)
				client := newClient(t, baseURL)

				_, err := client.SubscribeProduct(context.Background(), newPurchaseRequest("android"))

				require.NoError(t, err)
				assert.JSONEq(t,
					`{"product_id":"TST-0001","platform":"android","purchase_token":"TST-TOKEN-0001"}`,
					string((<-received).Body))
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("402を受けたとき、ErrPaymentRequiredになる", func(t *testing.T) {
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
				baseURL, _ := newRecordingServer(t, http.StatusOK, `{"status":"ok"}`)
				client := newClient(t, baseURL)

				health, err := client.GetHealth(context.Background())

				require.NoError(t, err)
				assert.Equal(t, "ok", health.Status)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("500を受けたとき、ErrInternalServerになる", func(t *testing.T) {
				baseURL, _ := newRecordingServer(t, http.StatusInternalServerError, "")
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
				baseURL, _ := newRecordingServer(t, http.StatusOK, "")
				client := newClient(t, baseURL)

				err := client.AppleWebhook(context.Background(), apishop.AppleNotificationPayload{SignedPayload: "TST-JWS-0001"})

				assert.NoError(t, err)
			})

			t.Run("signedPayloadがTST-JWS-0001のAppleの通知を送ったとき、サーバーに届くリクエスト本文のsignedPayloadはTST-JWS-0001になる", func(t *testing.T) {
				baseURL, received := newRecordingServer(t, http.StatusOK, "")
				client := newClient(t, baseURL)

				err := client.AppleWebhook(context.Background(), apishop.AppleNotificationPayload{SignedPayload: "TST-JWS-0001"})

				require.NoError(t, err)
				assert.JSONEq(t, `{"signedPayload":"TST-JWS-0001"}`, string((<-received).Body))
			})

			t.Run("Googleの通知を送り、サーバーが200を返したとき、エラーにならない", func(t *testing.T) {
				baseURL, _ := newRecordingServer(t, http.StatusOK, "")
				client := newClient(t, baseURL)

				err := client.GoogleWebhook(context.Background(), apishop.GoogleRTDNMessage{
					Message: apishop.GoogleRTDNMessageBody{Data: "TST-DATA-0001"},
				})

				assert.NoError(t, err)
			})

			t.Run("message.dataがTST-DATA-0001のGoogleの通知を送ったとき、サーバーに届くリクエスト本文のmessage.dataはTST-DATA-0001になる", func(t *testing.T) {
				baseURL, received := newRecordingServer(t, http.StatusOK, "")
				client := newClient(t, baseURL)

				err := client.GoogleWebhook(context.Background(), apishop.GoogleRTDNMessage{
					Message: apishop.GoogleRTDNMessageBody{Data: "TST-DATA-0001"},
				})

				require.NoError(t, err)
				assert.JSONEq(t, `{"message":{"data":"TST-DATA-0001"}}`, string((<-received).Body))
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("Appleの通知にサーバーが500を返したとき、ErrInternalServerになる", func(t *testing.T) {
				baseURL, _ := newRecordingServer(t, http.StatusInternalServerError, "")
				client := newClient(t, baseURL)

				err := client.AppleWebhook(context.Background(), apishop.AppleNotificationPayload{SignedPayload: "TST-JWS-0001"})

				assert.ErrorIs(t, err, apishopclient.ErrInternalServer)
			})

			t.Run("Googleの通知にサーバーが500を返したとき、ErrInternalServerになる", func(t *testing.T) {
				baseURL, _ := newRecordingServer(t, http.StatusInternalServerError, "")
				client := newClient(t, baseURL)

				err := client.GoogleWebhook(context.Background(), apishop.GoogleRTDNMessage{
					Message: apishop.GoogleRTDNMessageBody{Data: "TST-DATA-0001"},
				})

				assert.ErrorIs(t, err, apishopclient.ErrInternalServer)
			})
		})
	})
}

type viaMarkingDoer struct {
	next *http.Client
}

func (d viaMarkingDoer) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-Test-Via", "TST-VIA-0001")
	return d.next.Do(req)
}

func setInternalAuthHeader(_ context.Context, req *http.Request) error {
	req.Header.Set("X-Internal-Auth", "TST-TOKEN-0001")
	return nil
}

func TestClientRequestOptions(t *testing.T) {
	t.Run("[商品APIクライアント]リクエスト送信の設定", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("送信前にX-Internal-AuthヘッダへTST-TOKEN-0001を付ける設定をしたとき", func(t *testing.T) {
				t.Run("商品一覧を取得すると、サーバーに届くリクエストのX-Internal-AuthヘッダはTST-TOKEN-0001になる", func(t *testing.T) {
					baseURL, received := newRecordingServer(t, http.StatusOK, emptyProductListBody)
					client := newClient(t, baseURL, apishopclient.WithRequestEditorFn(setInternalAuthHeader))

					_, err := client.ListPlayerProducts(context.Background())

					require.NoError(t, err)
					assert.Equal(t, "TST-TOKEN-0001", (<-received).Header.Get("X-Internal-Auth"))
				})

				t.Run("商品を購入すると、サーバーに届くリクエストのX-Internal-AuthヘッダはTST-TOKEN-0001になる", func(t *testing.T) {
					baseURL, received := newRecordingServer(t, http.StatusOK, purchaseResultBody)
					client := newClient(t, baseURL, apishopclient.WithRequestEditorFn(setInternalAuthHeader))

					_, err := client.PurchaseProduct(context.Background(), newPurchaseRequest("ios"))

					require.NoError(t, err)
					assert.Equal(t, "TST-TOKEN-0001", (<-received).Header.Get("X-Internal-Auth"))
				})
			})

			t.Run("HTTPクライアントを差し替える設定をして商品一覧を取得したとき、リクエストは差し替えたHTTPクライアントを通してサーバーに届く", func(t *testing.T) {
				baseURL, received := newRecordingServer(t, http.StatusOK, emptyProductListBody)
				client := newClient(t, baseURL, apishopclient.WithHTTPClient(viaMarkingDoer{next: http.DefaultClient}))

				_, err := client.ListPlayerProducts(context.Background())

				require.NoError(t, err)
				assert.Equal(t, "TST-VIA-0001", (<-received).Header.Get("X-Test-Via"))
			})
		})
	})
}
