// Package apishopclient は consumer から wire 詳細 (REST path / HTTP status code) を
// 隠蔽するため、生成 client を sentinel error 変換でラップする。
package apishopclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// Sentinel errors. wire 契約 (OpenAPI spec) で定義された status code の意味を sentinel として export する。
var (
	// ErrNotFound は status 404。
	ErrNotFound = errors.New("apishopclient: not found")

	// ErrUnauthorized は status 401。
	ErrUnauthorized = errors.New("apishopclient: unauthorized")

	// ErrForbidden は status 403。
	ErrForbidden = errors.New("apishopclient: forbidden")

	// ErrBadRequest は status 400。
	ErrBadRequest = errors.New("apishopclient: bad request")

	// ErrPaymentRequired は status 402。purchase / subscribe で課金処理が成立しなかった場合に返る。
	ErrPaymentRequired = errors.New("apishopclient: payment required")

	// ErrConflict は status 409。purchase が重複する等の状態競合で返る。
	ErrConflict = errors.New("apishopclient: conflict")

	// ErrInternalServer は status 5xx。
	ErrInternalServer = errors.New("apishopclient: internal server error")
)

// Client は overload-party-shop REST API の sentinel-error converted client。
type Client struct {
	api *apishop.ClientWithResponses
}

// Option は Client 構築時の設定。
type Option func(*config)

type config struct {
	httpClient apishop.HttpRequestDoer
	editors    []apishop.RequestEditorFn
}

// WithHTTPClient は基底 HTTP doer を差し替える。デフォルトは http.DefaultClient。
func WithHTTPClient(doer apishop.HttpRequestDoer) Option {
	return func(c *config) { c.httpClient = doer }
}

// WithRequestEditorFn は全リクエストに適用する RequestEditor を追加する。
func WithRequestEditorFn(fn apishop.RequestEditorFn) Option {
	return func(c *config) { c.editors = append(c.editors, fn) }
}

// New は baseURL に接続する Client を生成する。
func New(baseURL string, opts ...Option) (*Client, error) {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	apiOpts := make([]apishop.ClientOption, 0, 1+len(cfg.editors))
	if cfg.httpClient != nil {
		apiOpts = append(apiOpts, apishop.WithHTTPClient(cfg.httpClient))
	}
	for _, ed := range cfg.editors {
		apiOpts = append(apiOpts, apishop.WithRequestEditorFn(ed))
	}

	api, err := apishop.NewClientWithResponses(baseURL, apiOpts...)
	if err != nil {
		return nil, fmt.Errorf("apishopclient: new: %w", err)
	}
	return &Client{api: api}, nil
}

// GetHealth はサービス死活監視用エンドポイントを叩く。
func (c *Client) GetHealth(ctx context.Context) (*apishop.HealthResponse, error) {
	resp, err := c.api.GetHealthWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("apishopclient: GetHealth: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("GetHealth", resp.StatusCode())
}

// ListPlayerProducts は playerID の所持プロダクト一覧を返す。
// playerID は X-Player-Id header として送られる (gateway が認証後に付与する想定)。
func (c *Client) ListPlayerProducts(ctx context.Context, playerID string) (*apishop.ProductListResponse, error) {
	resp, err := c.api.ListPlayerProductsWithResponse(ctx, &apishop.ListPlayerProductsParams{XPlayerId: playerID})
	if err != nil {
		return nil, fmt.Errorf("apishopclient: ListPlayerProducts: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("ListPlayerProducts", resp.StatusCode())
}

// PurchaseProduct は単発購入を実行する。
func (c *Client) PurchaseProduct(ctx context.Context, playerID string, req apishop.PurchaseRequest) (*apishop.PurchaseResponse, error) {
	resp, err := c.api.PurchaseProductWithResponse(ctx,
		&apishop.PurchaseProductParams{XPlayerId: playerID},
		apishop.PurchaseProductJSONRequestBody(req))
	if err != nil {
		return nil, fmt.Errorf("apishopclient: PurchaseProduct: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("PurchaseProduct", resp.StatusCode())
}

// SubscribeProduct はサブスクリプション購入を実行する。
func (c *Client) SubscribeProduct(ctx context.Context, playerID string, req apishop.PurchaseRequest) (*apishop.SubscribeResponse, error) {
	resp, err := c.api.SubscribeProductWithResponse(ctx,
		&apishop.SubscribeProductParams{XPlayerId: playerID},
		apishop.SubscribeProductJSONRequestBody(req))
	if err != nil {
		return nil, fmt.Errorf("apishopclient: SubscribeProduct: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("SubscribeProduct", resp.StatusCode())
}

// AppleWebhook は Apple App Store からの webhook 通知を受け取る (server-to-server)。
func (c *Client) AppleWebhook(ctx context.Context, req apishop.AppleNotificationPayload) error {
	resp, err := c.api.AppleWebhookWithResponse(ctx, apishop.AppleWebhookJSONRequestBody(req))
	if err != nil {
		return fmt.Errorf("apishopclient: AppleWebhook: %w", err)
	}
	if resp.StatusCode() == http.StatusOK {
		return nil
	}
	return statusError("AppleWebhook", resp.StatusCode())
}

// GoogleWebhook は Google Play からの RTDN 通知を受け取る (server-to-server)。
func (c *Client) GoogleWebhook(ctx context.Context, req apishop.GoogleRTDNMessage) error {
	resp, err := c.api.GoogleWebhookWithResponse(ctx, apishop.GoogleWebhookJSONRequestBody(req))
	if err != nil {
		return fmt.Errorf("apishopclient: GoogleWebhook: %w", err)
	}
	if resp.StatusCode() == http.StatusOK {
		return nil
	}
	return statusError("GoogleWebhook", resp.StatusCode())
}

// statusError は HTTP status code を sentinel error (errors.Is 分岐可能) に変換する。
func statusError(op string, code int) error {
	var sentinel error
	switch {
	case code == http.StatusUnauthorized:
		sentinel = ErrUnauthorized
	case code == http.StatusPaymentRequired:
		sentinel = ErrPaymentRequired
	case code == http.StatusForbidden:
		sentinel = ErrForbidden
	case code == http.StatusNotFound:
		sentinel = ErrNotFound
	case code == http.StatusBadRequest:
		sentinel = ErrBadRequest
	case code == http.StatusConflict:
		sentinel = ErrConflict
	case code >= http.StatusInternalServerError:
		sentinel = ErrInternalServer
	default:
		return fmt.Errorf("apishopclient: %s: unexpected status %d", op, code)
	}
	return fmt.Errorf("apishopclient: %s: %w", op, sentinel)
}
