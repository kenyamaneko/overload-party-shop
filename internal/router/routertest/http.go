package routertest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// Response はルーターが返した応答のステータスと本文。
type Response struct {
	Status int
	Body   []byte
}

// Send は engine に要求を送り、応答を返す。
func Send(engine *gin.Engine, method, path string, body []byte, headers map[string]string) Response {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return Response{Status: recorder.Code, Body: recorder.Body.Bytes()}
}

// ErrorMessage は応答本文の error の値を返す。
func (r Response) ErrorMessage(t *testing.T) string {
	t.Helper()
	var decoded struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(r.Body, &decoded), "応答本文: %s", r.Body)
	return decoded.Error
}

// Products は商品一覧の応答本文の products を返す。
func (r Response) Products(t *testing.T) []apishop.ProductResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, r.Status, "応答本文: %s", r.Body)
	var decoded apishop.ProductListResponse
	require.NoError(t, json.Unmarshal(r.Body, &decoded), "応答本文: %s", r.Body)
	return decoded.Products
}

// Product は商品一覧の応答本文から商品 ID が productID の商品を返す。
func (r Response) Product(t *testing.T, productID string) apishop.ProductResponse {
	t.Helper()
	for _, product := range r.Products(t) {
		if product.ProductID == productID {
			return product
		}
	}
	require.FailNow(t, "商品一覧に商品がない", "商品ID: %s 応答本文: %s", productID, r.Body)
	return apishop.ProductResponse{}
}

// PurchaseBody は商品の購入の応答本文を返す。
func (r Response) PurchaseBody(t *testing.T) apishop.PurchaseResponse {
	t.Helper()
	var decoded apishop.PurchaseResponse
	require.NoError(t, json.Unmarshal(r.Body, &decoded), "応答本文: %s", r.Body)
	return decoded
}

// SubscribeBody はサブスクリプションの購入の応答本文を返す。
func (r Response) SubscribeBody(t *testing.T) apishop.SubscribeResponse {
	t.Helper()
	var decoded apishop.SubscribeResponse
	require.NoError(t, json.Unmarshal(r.Body, &decoded), "応答本文: %s", r.Body)
	return decoded
}

// MarshalJSONBody は値を JSON の本文にする。
func MarshalJSONBody(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

// ExtractProductIDs は商品の一覧から商品 ID を一覧の順に取り出す。
func ExtractProductIDs(products []apishop.ProductResponse) []string {
	ids := make([]string, 0, len(products))
	for _, product := range products {
		ids = append(ids, product.ProductID)
	}
	return ids
}
