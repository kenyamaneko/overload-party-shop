// Package adaptertest は外部サービスの代わりに立てる HTTP サーバのテストヘルパ。
package adaptertest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// NewServer は handler で応答するサーバを立てる。テスト終了時に停止する。
func NewServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
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
