// Package apishopserverfake は shop の HTTP 契約を実装する httptest.Server ラッパー。
// 各 endpoint は Fn field (func callback) で応答を制御し、Fn=nil なら既定値を返す。
package apishopserverfake

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

const (
	purchaseAcceptedMessage     = "purchase accepted"
	subscriptionAcceptedMessage = "subscription accepted"
)

var defaultSubscriptionExpiresAt = time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)

// Server は shop HTTP 契約を実装する httptest.Server wrapper。
type Server struct {
	mu  sync.Mutex
	srv *httptest.Server

	// GetProductsFn は GET /api/v1/shop/products の応答を決定する (nil は 200 + 空 Products)。
	GetProductsFn func() (int, any)

	// PurchaseFn は POST /api/v1/shop/purchase の応答を決定する (nil は 200 + PurchaseResponse)。
	PurchaseFn func(req apishop.PurchaseRequest) (int, any)

	// SubscribeFn は POST /api/v1/shop/subscribe の応答を決定する (nil は 200 + SubscribeResponse)。
	SubscribeFn func(req apishop.PurchaseRequest) (int, any)
}

// NewServer は起動済み Server を返す。テスト終了時に Close() すること。
func NewServer() *Server {
	s := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/shop/products", s.handleGetProducts)
	mux.HandleFunc("POST /api/v1/shop/purchase", s.handlePurchase)
	mux.HandleFunc("POST /api/v1/shop/subscribe", s.handleSubscribe)
	s.srv = httptest.NewServer(mux)
	return s
}

// URL は httptest.Server のベース URL を返す。
func (s *Server) URL() string { return s.srv.URL }

// Close は内部 httptest.Server を閉じる。
func (s *Server) Close() { s.srv.Close() }

func (s *Server) handleGetProducts(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fn := s.GetProductsFn
	s.mu.Unlock()

	if fn == nil {
		writeJSON(w, http.StatusOK, apishop.ProductListResponse{Products: []apishop.ProductResponse{}})
		return
	}
	status, body := fn()
	writeJSON(w, status, body)
}

func (s *Server) handlePurchase(w http.ResponseWriter, r *http.Request) {
	var req apishop.PurchaseRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.mu.Lock()
	fn := s.PurchaseFn
	s.mu.Unlock()

	if fn == nil {
		writeJSON(w, http.StatusOK, apishop.PurchaseResponse{Message: purchaseAcceptedMessage, ProductID: req.ProductID})
		return
	}
	status, body := fn(req)
	writeJSON(w, status, body)
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	var req apishop.PurchaseRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.mu.Lock()
	fn := s.SubscribeFn
	s.mu.Unlock()

	if fn == nil {
		writeJSON(w, http.StatusOK, apishop.SubscribeResponse{Message: subscriptionAcceptedMessage, ExpiresAt: defaultSubscriptionExpiresAt})
		return
	}
	status, body := fn(req)
	writeJSON(w, status, body)
}

// writeJSON は status と body を書く (body=nil なら status のみ)。
func writeJSON(w http.ResponseWriter, status int, body any) {
	if body == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
