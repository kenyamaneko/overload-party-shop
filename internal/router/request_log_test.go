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
				tests := []struct {
					name        string
					method      string
					path        string
					body        []byte
					notifierErr error
					wantStatus  int
					wantLevel   slog.Level
				}{
					{name: "不正な本文のApple通知をPOSTすると、ログのレベルはWarnになる", method: http.MethodPost, path: routertest.AppleWebhookPath, body: []byte("not-json"), wantStatus: http.StatusBadRequest, wantLevel: slog.LevelWarn},
					{name: "X-Internal-Authヘッダーが無い商品一覧をGETすると、ログのレベルはWarnになる", method: http.MethodGet, path: routertest.ProductsPath, wantStatus: http.StatusUnauthorized, wantLevel: slog.LevelWarn},
					{name: "未登録のパスをGETすると、ログのレベルはWarnになる", method: http.MethodGet, path: "/TST-UNREGISTERED-PATH", wantStatus: http.StatusNotFound, wantLevel: slog.LevelWarn},
					{
						name:        "通知処理が一時的な失敗を返すApple通知をPOSTすると、ログのレベルはErrorになる",
						method:      http.MethodPost,
						path:        routertest.AppleWebhookPath,
						body:        appleBody,
						notifierErr: errors.New("TST-TEMPORARY-FAILURE"),
						wantStatus:  http.StatusInternalServerError,
						wantLevel:   slog.LevelError,
					},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						logs := logtest.Capture(t)
						engine := routertest.BuildRouter(t, routertest.WithApple(&routertest.AppleNotifier{Err: tt.notifierErr}))

						response := routertest.Send(engine, tt.method, tt.path, tt.body, nil)

						require.Equal(t, tt.wantStatus, response.Status)
						records := selectRequestLogs(logs)
						require.Len(t, records, 1)
						assert.Equal(t, tt.wantLevel, records[0].Level)
					})
				}
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
