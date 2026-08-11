package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-shop/internal/usecase/subscription"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeRouterVerifier は router 単体テスト用の internalauth.Verifier 最小 fake。
// webhook / health の検証では auth middleware の経路を通らないため Verify は呼ばれない。
type fakeRouterVerifier struct{}

func (fakeRouterVerifier) Verify(string) (string, error) { return "", nil }

func testVerifier() internalauth.Verifier {
	return fakeRouterVerifier{}
}

const (
	testPushServiceAccountEmail = "push-sa@example.com"
	testPushAudience            = "TST-PUBSUB-PUSH-AUDIENCE"
	testValidPushAuthHeader     = "Bearer valid-id-token"
)

// fakePushValidator は router 単体テスト用の PubSubPushTokenValidator 最小 fake。
// 常に testPushServiceAccountEmail を返して push 認証を通過させる。
type fakePushValidator struct{}

func (fakePushValidator) Validate(context.Context, string, string) (string, error) {
	return testPushServiceAccountEmail, nil
}

func testPushValidator() PubSubPushTokenValidator {
	return fakePushValidator{}
}

// stubPushValidator は push 認証の判定結果テスト用の PubSubPushTokenValidator stub。
type stubPushValidator struct {
	validateFn func(ctx context.Context, idToken, audience string) (string, error)
}

func (s *stubPushValidator) Validate(ctx context.Context, idToken, audience string) (string, error) {
	return s.validateFn(ctx, idToken, audience)
}

// stubShopService は固定値を返す shop usecase スタブ。
type stubShopService struct{}

// GetProducts は商品なしを返す。
func (stubShopService) GetProducts(context.Context, string) ([]domain.ProductWithOwnership, error) {
	return []domain.ProductWithOwnership{}, nil
}

// Purchase は常に成功する。
func (stubShopService) Purchase(context.Context, string, string, string, string) error {
	return nil
}

// Subscribe は期限なしで成功する。
func (stubShopService) Subscribe(context.Context, string, string, string, string) (*time.Time, error) {
	return nil, nil
}

// fakeAppleNotifier は webhook が handler を経て notifier まで到達したかを呼び出し回数で観測する。
type fakeAppleNotifier struct {
	calls int
}

// HandleNotification は Apple webhook の到達を記録する。
func (f *fakeAppleNotifier) HandleNotification(_ context.Context, _ string) error {
	f.calls++
	return nil
}

// fakeGoogleNotifier は webhook が handler を経て notifier まで到達したかを呼び出し回数で観測する。
type fakeGoogleNotifier struct {
	calls int
}

// HandleNotification は Google webhook の到達を記録する。
func (f *fakeGoogleNotifier) HandleNotification(_ context.Context, _ subscription.GoogleRTDNMessage) error {
	f.calls++
	return nil
}

const (
	appleWebhookPath  = "/webhook/apple"
	googleWebhookPath = "/webhook/google"
	appleValidBody    = `{"signedPayload":"fake-jws"}`
	googleValidBody   = `{"message":{"data":"fake-base64"}}`
)

// postWebhook は JSON body 付き POST を router に投げ、レスポンスを返す。
func postWebhook(r http.Handler, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// postGoogleWebhook は JSON body 付き POST を /webhook/google に投げる。authHeader が空でなければ
// Authorization ヘッダーとして設定する。
func postGoogleWebhook(r http.Handler, body, authHeader string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, googleWebhookPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestNew(t *testing.T) {
	t.Run("ルーターのwebhook配線", func(t *testing.T) {
		t.Run("/healthはwebhookの登録状態に関わらず200を返す", func(t *testing.T) {
			r := New(rest.NewShopHandler(nil), nil, nil, testVerifier(), testPushValidator(), testPushServiceAccountEmail, testPushAudience)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
			assert.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("apple webhookのみ登録したとき、appleはnotifierに到達しgoogleは404のままになる", func(t *testing.T) {
			n := &fakeAppleNotifier{}
			r := New(rest.NewShopHandler(nil), rest.NewAppleWebhookHandler(n), nil, testVerifier(), testPushValidator(), testPushServiceAccountEmail, testPushAudience)

			w := postWebhook(r, appleWebhookPath, appleValidBody)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, 1, n.calls)

			sibling := postWebhook(r, googleWebhookPath, googleValidBody)
			assert.Equal(t, http.StatusNotFound, sibling.Code)
		})

		t.Run("google webhookのみ登録したとき、googleはnotifierに到達しappleは404のままになる", func(t *testing.T) {
			n := &fakeGoogleNotifier{}
			r := New(rest.NewShopHandler(nil), nil, rest.NewGoogleWebhookHandler(n), testVerifier(), testPushValidator(), testPushServiceAccountEmail, testPushAudience)

			w := postGoogleWebhook(r, googleValidBody, testValidPushAuthHeader)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, 1, n.calls)

			sibling := postWebhook(r, appleWebhookPath, appleValidBody)
			assert.Equal(t, http.StatusNotFound, sibling.Code)
		})

		t.Run("webhook handlerがnilのとき、ルートが未登録のまま404になる", func(t *testing.T) {
			tests := []struct {
				name string
				path string
				body string
			}{
				{
					name: "apple",
					path: appleWebhookPath,
					body: appleValidBody,
				},
				{
					name: "google",
					path: googleWebhookPath,
					body: googleValidBody,
				},
			}
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					r := New(rest.NewShopHandler(nil), nil, nil, testVerifier(), testPushValidator(), testPushServiceAccountEmail, testPushAudience)
					w := postWebhook(r, tc.path, tc.body)
					assert.Equal(t, http.StatusNotFound, w.Code)
				})
			}
		})
	})

	t.Run("/api/v1/shopの内部認証配線", func(t *testing.T) {
		t.Run("auth headerが欠落しているとき、401になりhandlerに到達しない", func(t *testing.T) {
			// VerifyFn 未設定: header 欠落時は middleware が verifier に到達しないことの検出を兼ねる
			r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{}, testPushValidator(), testPushServiceAccountEmail, testPushAudience)

			cases := []struct {
				name   string
				method string
				path   string
			}{
				{
					name:   "GET /products",
					method: http.MethodGet,
					path:   "/api/v1/shop/products",
				},
				{
					name:   "POST /purchase",
					method: http.MethodPost,
					path:   "/api/v1/shop/purchase",
				},
				{
					name:   "POST /subscribe",
					method: http.MethodPost,
					path:   "/api/v1/shop/subscribe",
				},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					w := httptest.NewRecorder()
					r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
					assert.Equal(t, http.StatusUnauthorized, w.Code)
				})
			}
		})

		t.Run("verifierがエラーを返すとき、401になりhandlerに到達しない", func(t *testing.T) {
			r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{
				VerifyFn: func(string) (string, error) { return "", errors.New("invalid token") },
			}, testPushValidator(), testPushServiceAccountEmail, testPushAudience)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("有効なトークンのとき、handlerの応答まで到達する", func(t *testing.T) {
			r := New(rest.NewShopHandler(stubShopService{}), nil, nil, &internalauth.MockVerifier{
				VerifyFn: func(string) (string, error) { return "TST-PLAYER-1", nil },
			}, testPushValidator(), testPushServiceAccountEmail, testPushAudience)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	})

	t.Run("/webhook/googleのPub/Sub push認証配線", func(t *testing.T) {
		t.Run("Authorizationヘッダーが無いとき、401になりnotifierに到達しない", func(t *testing.T) {
			n := &fakeGoogleNotifier{}
			r := New(rest.NewShopHandler(nil), nil, rest.NewGoogleWebhookHandler(n), testVerifier(), testPushValidator(), testPushServiceAccountEmail, testPushAudience)

			w := postGoogleWebhook(r, googleValidBody, "")

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Equal(t, 0, n.calls)
		})

		t.Run(`Authorizationヘッダーが"Bearer "で始まらないとき、401になりnotifierに到達しない`, func(t *testing.T) {
			n := &fakeGoogleNotifier{}
			r := New(rest.NewShopHandler(nil), nil, rest.NewGoogleWebhookHandler(n), testVerifier(), testPushValidator(), testPushServiceAccountEmail, testPushAudience)

			w := postGoogleWebhook(r, googleValidBody, "Token xyz")

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Equal(t, 0, n.calls)
		})

		t.Run("OIDCトークンの検証が失敗するとき、401になりnotifierに到達しない", func(t *testing.T) {
			n := &fakeGoogleNotifier{}
			validator := &stubPushValidator{
				validateFn: func(context.Context, string, string) (string, error) {
					return "", errors.New("signature invalid")
				},
			}
			r := New(rest.NewShopHandler(nil), nil, rest.NewGoogleWebhookHandler(n), testVerifier(), validator, testPushServiceAccountEmail, testPushAudience)

			w := postGoogleWebhook(r, googleValidBody, testValidPushAuthHeader)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Equal(t, 0, n.calls)
		})

		t.Run("OIDCトークンの検証は成功するが、署名者のメールアドレスが想定するpush用サービスアカウントと一致しないとき、401になりnotifierに到達しない", func(t *testing.T) {
			n := &fakeGoogleNotifier{}
			validator := &stubPushValidator{
				validateFn: func(context.Context, string, string) (string, error) {
					return "other-sa@example.com", nil
				},
			}
			r := New(rest.NewShopHandler(nil), nil, rest.NewGoogleWebhookHandler(n), testVerifier(), validator, testPushServiceAccountEmail, testPushAudience)

			w := postGoogleWebhook(r, googleValidBody, testValidPushAuthHeader)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Equal(t, 0, n.calls)
		})

		t.Run("OIDCトークンの検証が成功し、署名者のメールアドレスが想定するpush用サービスアカウントと一致するとき、notifierに到達する", func(t *testing.T) {
			n := &fakeGoogleNotifier{}
			validator := &stubPushValidator{
				validateFn: func(context.Context, string, string) (string, error) {
					return testPushServiceAccountEmail, nil
				},
			}
			r := New(rest.NewShopHandler(nil), nil, rest.NewGoogleWebhookHandler(n), testVerifier(), validator, testPushServiceAccountEmail, testPushAudience)

			w := postGoogleWebhook(r, googleValidBody, testValidPushAuthHeader)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, 1, n.calls)
		})
	})
}
