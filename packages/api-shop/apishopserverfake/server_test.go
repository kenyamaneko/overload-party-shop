package apishopserverfake_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopserverfake"
)

const (
	productsPath  = "/api/v1/shop/products"
	purchasePath  = "/api/v1/shop/purchase"
	subscribePath = "/api/v1/shop/subscribe"

	purchaseRequestBody = `{"product_id":"TST-0001","platform":"ios","purchase_token":"TST-TOKEN-0001"}`
)

type response struct {
	Status int
	Body   []byte
}

func (r response) DecodeFields(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(r.Body, &fields))
	return fields
}

func newServer(t *testing.T) *apishopserverfake.Server {
	t.Helper()
	server := apishopserverfake.NewServer()
	t.Cleanup(server.Close)
	return server
}

func send(t *testing.T, method, url, body string) response {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")

	httpResponse, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(httpResponse.Body)
	require.NoError(t, err)
	return response{Status: httpResponse.StatusCode, Body: responseBody}
}

func TestServerResponses(t *testing.T) {
	t.Run("[商品APIサーバーフェイク]応答の制御", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("応答を設定していないとき", func(t *testing.T) {
				t.Run("/api/v1/shop/productsにGETすると、応答のステータスは200になる", func(t *testing.T) {
					server := newServer(t)

					got := send(t, http.MethodGet, server.URL()+productsPath, "")

					assert.Equal(t, http.StatusOK, got.Status)
				})

				t.Run("/api/v1/shop/productsにGETすると、応答の本文のproductsは空の配列になる", func(t *testing.T) {
					server := newServer(t)

					got := send(t, http.MethodGet, server.URL()+productsPath, "")

					assert.JSONEq(t, `[]`, string(got.DecodeFields(t)["products"]))
				})

				t.Run("/api/v1/shop/purchaseにPOSTすると、応答のステータスは200になる", func(t *testing.T) {
					server := newServer(t)

					got := send(t, http.MethodPost, server.URL()+purchasePath, purchaseRequestBody)

					assert.Equal(t, http.StatusOK, got.Status)
				})

				t.Run("/api/v1/shop/purchaseにPOSTすると、応答の本文にmessageとproduct_idが含まれる", func(t *testing.T) {
					server := newServer(t)

					got := send(t, http.MethodPost, server.URL()+purchasePath, purchaseRequestBody)

					fields := got.DecodeFields(t)
					assert.Contains(t, fields, "message")
					assert.Contains(t, fields, "product_id")
				})

				t.Run("/api/v1/shop/subscribeにPOSTすると、応答のステータスは200になる", func(t *testing.T) {
					server := newServer(t)

					got := send(t, http.MethodPost, server.URL()+subscribePath, purchaseRequestBody)

					assert.Equal(t, http.StatusOK, got.Status)
				})

				t.Run("/api/v1/shop/subscribeにPOSTすると、応答の本文のexpires_atはnullでない日時になる", func(t *testing.T) {
					server := newServer(t)

					got := send(t, http.MethodPost, server.URL()+subscribePath, purchaseRequestBody)

					var body struct {
						ExpiresAt *time.Time `json:"expires_at"`
					}
					require.NoError(t, json.Unmarshal(got.Body, &body))
					assert.NotNil(t, body.ExpiresAt)
				})
			})

			t.Run("応答を設定したとき", func(t *testing.T) {
				t.Run("purchaseの本文がproduct_idのTST-0001・platformのios・purchase_tokenのTST-TOKEN-0001のとき、応答の指定がその購入内容を受け取る", func(t *testing.T) {
					server := newServer(t)
					received := make(chan apishop.PurchaseRequest, 1)
					server.PurchaseFn = func(req apishop.PurchaseRequest) (int, any) {
						received <- req
						return http.StatusOK, apishop.PurchaseResponse{Message: "TST-MESSAGE-0001", ProductID: req.ProductID}
					}

					send(t, http.MethodPost, server.URL()+purchasePath, purchaseRequestBody)

					assert.Equal(t, apishop.PurchaseRequest{
						ProductID:     "TST-0001",
						Platform:      "ios",
						PurchaseToken: "TST-TOKEN-0001",
					}, <-received)
				})

				t.Run("purchaseの応答の指定が409を返すと、応答のステータスは409になる", func(t *testing.T) {
					server := newServer(t)
					server.PurchaseFn = func(apishop.PurchaseRequest) (int, any) {
						return http.StatusConflict, nil
					}

					got := send(t, http.MethodPost, server.URL()+purchasePath, purchaseRequestBody)

					assert.Equal(t, http.StatusConflict, got.Status)
				})

				t.Run("subscribeの応答の指定がmessageとexpires_atの2026-05-01T12:00:00Zを返すと、応答の本文のexpires_atは2026-05-01T12:00:00Zになる", func(t *testing.T) {
					server := newServer(t)
					server.SubscribeFn = func(apishop.PurchaseRequest) (int, any) {
						return http.StatusOK, apishop.SubscribeResponse{
							Message:   "TST-MESSAGE-0001",
							ExpiresAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
						}
					}

					got := send(t, http.MethodPost, server.URL()+subscribePath, purchaseRequestBody)

					assert.JSONEq(t, `"2026-05-01T12:00:00Z"`, string(got.DecodeFields(t)["expires_at"]))
				})

				t.Run("応答の指定が本文なしでステータス500を返すと、応答の本文は空になる", func(t *testing.T) {
					server := newServer(t)
					server.PurchaseFn = func(apishop.PurchaseRequest) (int, any) {
						return http.StatusInternalServerError, nil
					}

					got := send(t, http.MethodPost, server.URL()+purchasePath, purchaseRequestBody)

					assert.Empty(t, got.Body)
				})
			})
		})
	})
}
