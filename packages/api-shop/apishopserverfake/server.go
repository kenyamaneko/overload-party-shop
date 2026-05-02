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

// SelectFactionRequest は shopclient.SelectFactionRequest と同形のリクエスト型。
type SelectFactionRequest struct {
	Faction string `json:"faction"`
}

// SelectFactionResponse は shopclient.SelectFactionResponse と同形のレスポンス型。
type SelectFactionResponse struct {
	Message      string `json:"message"`
	Faction      string `json:"faction"`
	CardsGranted int    `json:"cards_granted"`
}

// ProductsResponse は GetProducts endpoint の JSON envelope。
type ProductsResponse struct {
	Products []apishop.ProductResponse `json:"products"`
}

// SubscribeResponse は shopclient.subscribeResponse と同形のレスポンス型。
type SubscribeResponse struct {
	Message   string     `json:"message"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Server は shop HTTP 契約を実装する httptest.Server wrapper。
type Server struct {
	mu  sync.Mutex
	srv *httptest.Server

	// SelectFactionFn は POST /internal/v1/players/{playerID}/select-faction の応答を決定する (nil は 200 + 空 body)。
	SelectFactionFn func(playerID string, req SelectFactionRequest) (int, any)

	// GetProductsFn は GET /internal/v1/players/{playerID}/products の応答を決定する (nil は 200 + 空 Products)。
	GetProductsFn func(playerID string) (int, any)

	// PurchaseFn は POST /internal/v1/players/{playerID}/purchase の応答を決定する (nil は 204)。
	PurchaseFn func(playerID string, req apishop.PurchaseRequest) (int, any)

	// SubscribeFn は POST /internal/v1/players/{playerID}/subscribe の応答を決定する (nil は 200 + 空 body)。
	SubscribeFn func(playerID string, req apishop.PurchaseRequest) (int, any)
}

// NewServer は起動済み Server を返す。テスト終了時に Close() すること。
func NewServer() *Server {
	s := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/players/{playerID}/select-faction", s.handleSelectFaction)
	mux.HandleFunc("GET /internal/v1/players/{playerID}/products", s.handleGetProducts)
	mux.HandleFunc("POST /internal/v1/players/{playerID}/purchase", s.handlePurchase)
	mux.HandleFunc("POST /internal/v1/players/{playerID}/subscribe", s.handleSubscribe)
	s.srv = httptest.NewServer(mux)
	return s
}

// URL は httptest.Server のベース URL を返す。
func (s *Server) URL() string { return s.srv.URL }

// Close は内部 httptest.Server を閉じる。
func (s *Server) Close() { s.srv.Close() }

func (s *Server) handleSelectFaction(w http.ResponseWriter, r *http.Request) {
	var req SelectFactionRequest
	// body 不正でも Fn には空構造体を渡し、bad request の擬似は Fn 側で表現する。
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.mu.Lock()
	fn := s.SelectFactionFn
	s.mu.Unlock()

	playerID := r.PathValue("playerID")
	if fn == nil {
		writeJSON(w, http.StatusOK, SelectFactionResponse{})
		return
	}
	status, body := fn(playerID, req)
	writeJSON(w, status, body)
}

func (s *Server) handleGetProducts(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fn := s.GetProductsFn
	s.mu.Unlock()

	playerID := r.PathValue("playerID")
	if fn == nil {
		writeJSON(w, http.StatusOK, ProductsResponse{Products: []apishop.ProductResponse{}})
		return
	}
	status, body := fn(playerID)
	writeJSON(w, status, body)
}

func (s *Server) handlePurchase(w http.ResponseWriter, r *http.Request) {
	var req apishop.PurchaseRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.mu.Lock()
	fn := s.PurchaseFn
	s.mu.Unlock()

	playerID := r.PathValue("playerID")
	if fn == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	status, body := fn(playerID, req)
	writeJSON(w, status, body)
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	var req apishop.PurchaseRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	s.mu.Lock()
	fn := s.SubscribeFn
	s.mu.Unlock()

	playerID := r.PathValue("playerID")
	if fn == nil {
		writeJSON(w, http.StatusOK, SubscribeResponse{})
		return
	}
	status, body := fn(playerID, req)
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
