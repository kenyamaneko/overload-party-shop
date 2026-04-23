// Package apishopserverfake は shop サービスの HTTP 契約を実装する
// httptest.Server ラッパー。consumer (gateway 等) が shopclient を使う
// handler テストで、実 shop サービスを起動せずに REST 呼び出しを検証するための
// テストダブルを提供する。
//
// 各 endpoint は Fn field (func callback) で status + response body を制御する。
// Fn が nil の endpoint は既定値を返す (happy-path を仮定した最低限の応答)。
//
// JSON request / response shape は shopclient が送受する形式に合わせる。
// shopclient が private に保持している Request / Response 型は本パッケージでも
// 独立に定義しなおし、テスト側が typed でデータを組み立てられるようにする
// (api-shop 本体の公開型を増やさないための pragmatic な選択)。
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
// 本パッケージのテスト側で typed にリクエストボディを扱うために独立定義している。
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
// ExpiresAt は *time.Time で shopclient 側と同じ RFC 3339 JSON 文字列としてシリアライズされる。
type SubscribeResponse struct {
	Message   string     `json:"message"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Server は shop HTTP 契約を実装する httptest.Server wrapper。テスト側は NewServer
// で起動し、Fn field で endpoint 毎の応答を設定し、shopclient.New(server.URL()) を
// 通じて検証を行う。
type Server struct {
	mu  sync.Mutex
	srv *httptest.Server

	// SelectFactionFn は POST /internal/v1/players/{playerID}/select-faction の応答を決定する。
	// nil の場合は既定値 200 + 空の SelectFactionResponse を返す。
	SelectFactionFn func(playerID string, req SelectFactionRequest) (int, any)

	// GetProductsFn は GET /internal/v1/players/{playerID}/products の応答を決定する。
	// nil の場合は既定値 200 + 空 Products を返す。
	GetProductsFn func(playerID string) (int, any)

	// PurchaseFn は POST /internal/v1/players/{playerID}/purchase の応答を決定する。
	// nil の場合は既定値 204 No Content を返す。
	PurchaseFn func(playerID string, req apishop.PurchaseRequest) (int, any)

	// SubscribeFn は POST /internal/v1/players/{playerID}/subscribe の応答を決定する。
	// nil の場合は既定値 200 + 空の SubscribeResponse を返す。
	SubscribeFn func(playerID string, req apishop.PurchaseRequest) (int, any)
}

// NewServer は起動済み Server を返す。URL() で base URL を取得し
// shopclient.New(server.URL()) に渡して利用する。テスト終了時に Close() すること。
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
	// body が壊れていても Fn にはそのまま空構造体を渡す。bad request を擬似する
	// テストは Fn 内で status=400 を返すことで表現可能。
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

// writeJSON は status code を書き、body が非 nil なら Content-Type: application/json
// で JSON encode して送る。body が nil の場合は body 無しでレスポンスを終わる
// (shopclient は 4xx/5xx のエラー body を一部しか読まないため、body=nil でも応答は成立する)。
func writeJSON(w http.ResponseWriter, status int, body any) {
	if body == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
