// Package googletest は google パッケージのテストで共有する検証器の組み立てヘルパ。
package googletest

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/google"
)

// NewVerifier は endpoint を接続先にし、パッケージ名 com.test.shop で GooglePlay の購入検証器を組み立てる。
func NewVerifier(t *testing.T, endpoint string) *google.Verifier {
	t.Helper()
	v, err := google.NewVerifier(context.Background(), "com.test.shop", option.WithEndpoint(endpoint), option.WithHTTPClient(http.DefaultClient))
	require.NoError(t, err)
	return v
}

// NewSubVerifier は endpoint を接続先にし、パッケージ名 com.test.shop で GooglePlay のサブスクリプション有効期限取得器を組み立てる。
func NewSubVerifier(t *testing.T, endpoint string) *google.SubVerifier {
	t.Helper()
	v, err := google.NewSubVerifier(context.Background(), "com.test.shop", option.WithEndpoint(endpoint), option.WithHTTPClient(http.DefaultClient))
	require.NoError(t, err)
	return v
}
