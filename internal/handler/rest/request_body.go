package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/gin-gonic/gin"
)

var (
	// errEmptyRequestBody はリクエスト本文が空 (長さ 0 または空白だけ) の場合に返される。
	errEmptyRequestBody = errors.New("request body is empty")
	// errInvalidRequestBody はリクエスト本文が JSON として読めない場合に返される。
	errInvalidRequestBody = errors.New("request body is not valid JSON")
)

// decodeRequestBody はリクエスト本文を JSON として dst に読み込む。
// 読み込めない原因は errEmptyRequestBody と errInvalidRequestBody で区別する。
func decodeRequestBody(c *gin.Context, dst any) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return errEmptyRequestBody
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(dst); err != nil {
		slog.Warn("request body is not valid JSON", "path", c.Request.URL.Path, "error", err)
		return errInvalidRequestBody
	}
	return nil
}
