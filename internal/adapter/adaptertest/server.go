// Package adaptertest は外部サービスの代わりに立てる HTTP サーバのテストヘルパ。
package adaptertest

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// RecordedRequest はサーバが受け取った要求の記録。
type RecordedRequest struct {
	Method      string
	Path        string
	EscapedPath string
	Header      http.Header
}

// Server は受け取った要求を記録する HTTP サーバ。
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	requests []RecordedRequest
}

// NewServer は handler で応答し、受け取った要求を記録するサーバを立てる。テスト終了時に停止する。
func NewServer(t *testing.T, handler http.HandlerFunc) *Server {
	t.Helper()
	s := &Server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, RecordedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			EscapedPath: r.URL.EscapedPath(),
			Header:      r.Header.Clone(),
		})
		s.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// Requests はサーバが受け取った要求を受け取った順に返す。
func (s *Server) Requests() []RecordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]RecordedRequest(nil), s.requests...)
}

// RespondWith は指定したステータスコードと本文を返す handler を返す。
func RespondWith(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// NewUnreachableURL は接続を受け付けないサーバの URL を返す。
func NewUnreachableURL(t *testing.T) string {
	t.Helper()
	s := httptest.NewServer(http.NotFoundHandler())
	url := s.URL
	s.Close()
	return url
}

// NewServerStarter は、指定した応答を返すサーバを立ててその URL を返す関数を返す。
func NewServerStarter(status int, body string) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()
		return NewServer(t, RespondWith(status, body)).URL
	}
}
