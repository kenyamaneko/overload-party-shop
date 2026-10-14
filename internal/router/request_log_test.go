package router_test

import (
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"

	"github.com/kenyamaneko/overload-party-shop/internal/router/routertest"
	"github.com/kenyamaneko/overload-party-shop/internal/util/logtest"
)

func selectRequestLogs(logs *logtest.Logs) []logtest.Record {
	return logs.SelectByMessage("http request")
}

func sendHealthCheck(t *testing.T) []logtest.Record {
	t.Helper()
	logs := logtest.Capture(t)
	engine := routertest.BuildRouter(t)

	routertest.Send(engine, http.MethodGet, "/health", nil, nil)

	return selectRequestLogs(logs)
}

func TestRequestLog(t *testing.T) {
	appleBody := routertest.BuildAppleWebhookBody(t, "TST-SIGNED-PAYLOAD")

	t.Run("[HTTPリクエストログ]リクエストごとのログ", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("/healthへのGETが1回あったとき", func(t *testing.T) {
				t.Run("ログのレベルはInfoになる", func(t *testing.T) {
					records := sendHealthCheck(t)

					require.Len(t, records, 1)
					assert.Equal(t, slog.LevelInfo, records[0].Level)
				})

				t.Run("リクエストを表すログが1件だけ出る", func(t *testing.T) {
					records := sendHealthCheck(t)

					assert.Len(t, records, 1)
				})

				t.Run("ログのmethodがGETになる", func(t *testing.T) {
					records := sendHealthCheck(t)

					require.Len(t, records, 1)
					assert.Equal(t, "GET", records[0].String("method"))
				})

				t.Run("ログのpathが/healthになる", func(t *testing.T) {
					records := sendHealthCheck(t)

					require.Len(t, records, 1)
					assert.Equal(t, "/health", records[0].String("path"))
				})

				t.Run("ログのstatusが200になる", func(t *testing.T) {
					records := sendHealthCheck(t)

					require.Len(t, records, 1)
					assert.Equal(t, int64(200), records[0].Int("status"))
				})
			})

			t.Run("/health?probe=1へのGETのとき、ログのpathは/healthになる", func(t *testing.T) {
				logs := logtest.Capture(t)
				engine := routertest.BuildRouter(t)

				routertest.Send(engine, http.MethodGet, "/health?probe=1", nil, nil)

				records := selectRequestLogs(logs)
				require.Len(t, records, 1)
				assert.Equal(t, "/health", records[0].String("path"))
			})

			t.Run("リクエストの処理に20msかかるとき、ログのlatencyは20ms以上になる", func(t *testing.T) {
				logs := logtest.Capture(t)
				engine := routertest.BuildRouter(t, routertest.WithApple(&routertest.AppleNotifier{Delay: 20 * time.Millisecond}))

				routertest.Send(engine, http.MethodPost, routertest.AppleWebhookPath, appleBody, nil)

				records := selectRequestLogs(logs)
				require.Len(t, records, 1)
				assert.GreaterOrEqual(t, records[0].Duration("latency"), 20*time.Millisecond)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("失敗するリクエストのログのレベル", func(t *testing.T) {
				assertRequestLogLevel := func(t *testing.T, method, path string, body []byte, notifierErr error, wantStatus int, wantLevel slog.Level) {
					t.Helper()
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithApple(&routertest.AppleNotifier{Err: notifierErr}))

					response := routertest.Send(engine, method, path, body, nil)

					require.Equal(t, wantStatus, response.Status)
					records := selectRequestLogs(logs)
					require.Len(t, records, 1)
					assert.Equal(t, wantLevel, records[0].Level)
				}

				t.Run("不正な本文のApple通知をPOSTすると、ログのレベルはWarnになる", func(t *testing.T) {
					assertRequestLogLevel(t, http.MethodPost, routertest.AppleWebhookPath, []byte("not-json"), nil, http.StatusBadRequest, slog.LevelWarn)
				})

				t.Run("X-Internal-Authヘッダーが無い商品一覧をGETすると、ログのレベルはWarnになる", func(t *testing.T) {
					assertRequestLogLevel(t, http.MethodGet, routertest.ProductsPath, nil, nil, http.StatusUnauthorized, slog.LevelWarn)
				})

				t.Run("未登録のパスをGETすると、ログのレベルはWarnになる", func(t *testing.T) {
					assertRequestLogLevel(t, http.MethodGet, "/TST-UNREGISTERED-PATH", nil, nil, http.StatusNotFound, slog.LevelWarn)
				})

				t.Run("再送すれば結果が変わりうる失敗になるApple通知をPOSTすると、ログのレベルはErrorになる", func(t *testing.T) {
					assertRequestLogLevel(t, http.MethodPost, routertest.AppleWebhookPath, appleBody, errors.New("TST-TEMPORARY-FAILURE"), http.StatusInternalServerError, slog.LevelError)
				})
			})

			t.Run("リクエストの処理が途中で異常終了したとき", func(t *testing.T) {
				headers := map[string]string{internalauth.HeaderName: "TST-TOKEN-1"}

				t.Run("応答は500になる", func(t *testing.T) {
					engine := routertest.BuildRouter(t, routertest.WithService(routertest.NewPanickingShopService()))

					response := routertest.Send(engine, http.MethodGet, routertest.ProductsPath, nil, headers)

					assert.Equal(t, http.StatusInternalServerError, response.Status)
				})

				t.Run("Errorレベルのリクエストログが1件出る", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithService(routertest.NewPanickingShopService()))

					routertest.Send(engine, http.MethodGet, routertest.ProductsPath, nil, headers)

					assert.Len(t, logs.SelectByMessageAndLevel("http request", slog.LevelError), 1)
				})

				t.Run("Errorレベルのリクエストログのstatusが500になる", func(t *testing.T) {
					logs := logtest.Capture(t)
					engine := routertest.BuildRouter(t, routertest.WithService(routertest.NewPanickingShopService()))

					routertest.Send(engine, http.MethodGet, routertest.ProductsPath, nil, headers)

					records := logs.SelectByMessageAndLevel("http request", slog.LevelError)
					require.Len(t, records, 1)
					assert.Equal(t, int64(500), records[0].Int("status"))
				})
			})
		})
	})
}
