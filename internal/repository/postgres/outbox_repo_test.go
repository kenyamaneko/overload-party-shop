//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// normalizeJSON は JSONB round-trip で付与される空白等の差異を吸収する。
// compact 形式に揃えることで「書き込んだ JSON が取り出せた」ことだけを固定する。
func normalizeJSON(t *testing.T, raws []string) []string {
	t.Helper()
	out := make([]string, 0, len(raws))
	for _, s := range raws {
		var v interface{}
		require.NoError(t, json.Unmarshal([]byte(s), &v))
		b, err := json.Marshal(v)
		require.NoError(t, err)
		out = append(out, string(b))
	}
	return out
}

// insertOutboxRow はユニークな player_id / token / card_pack_id で outbox 行を **1 本** 挿入する。
// 書き込み API は package 外に公開していないので card_pack_purchase 経由で作る (本 repo は 1 outbox 行のみ書く)。
// faction_purchase は 2 outbox 行を書くので seed 件数の意味付けが変わるため使わない。
// seed 追加の変種 (published 済み / in-flight 等) は apply 関数で後続の mutation を合成する。
func insertOutboxRow(t *testing.T, testIdx, seedIdx int, payload []byte) uuid.UUID {
	t.Helper()
	id := uuid.New()
	playerID := fmt.Sprintf("%08d-%04d-%04d-0000-000000000000", testIdx+1, seedIdx, seedIdx)
	cardPackID := fmt.Sprintf("pack-%d-%d", testIdx, seedIdx)
	token := fmt.Sprintf("tok-%d-%d", testIdx, seedIdx)

	cardPackRepo := postgres.NewCardPackPurchaseRepository(sharedPg.Pool)
	purchase := &domain.OneTimePurchase{PlayerID: playerID, ProductID: cardPackID, PurchasedAt: time.Now().UTC()}
	ev := port.OutboxEvent{EventID: id, EventType: apishop.EventTypeCardPackPurchased, Payload: payload}
	_, err := cardPackRepo.CreatePurchase(context.Background(), purchase, cardPackID, domain.PlatformIOS, token, ev)
	require.NoError(t, err)
	return id
}

// markPublishedDirectly は seed 後に published_at を埋める（既配信行のシミュレーション）。
func markPublishedDirectly(t *testing.T, id uuid.UUID) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`UPDATE shop.outbox_events SET published_at = now() WHERE event_id = $1`, id)
	require.NoError(t, err)
}

// backdateLastAttempted は last_attempted_at を過去方向に移動させ、別 worker が既に
// 試行中 (visibility timeout 内) もしくは試行後 (timeout 超過) の状態を作る。
func backdateLastAttempted(t *testing.T, id uuid.UUID, ago time.Duration) {
	t.Helper()
	interval := fmt.Sprintf("%d milliseconds", ago.Milliseconds())
	_, err := sharedPg.Pool.Exec(context.Background(),
		`UPDATE shop.outbox_events SET last_attempted_at = now() - ($2::text)::interval WHERE event_id = $1`,
		id, interval)
	require.NoError(t, err)
}

// seed は 1 行の挿入と、必要に応じた状態変更までを含む自己完結した Given 断片。
// runner は insert 関数を呼ぶだけで、ケースごとの if 分岐を持たない。
type seed struct {
	payload string
	insert  func(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID
}

// insertUnpublished は基本の未配信行を挿入する。
func insertUnpublished(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
	return insertOutboxRow(t, testIdx, seedIdx, []byte(payload))
}

// insertAlreadyPublished は seed 時点で既に配信済みの行 (published_at あり)。
func insertAlreadyPublished(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
	id := insertOutboxRow(t, testIdx, seedIdx, []byte(payload))
	markPublishedDirectly(t, id)
	return id
}

// insertInFlight は別 worker が試行中 (visibility timeout 以内) の行を作る。
func insertInFlight(ago time.Duration) func(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
	return func(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
		id := insertOutboxRow(t, testIdx, seedIdx, []byte(payload))
		backdateLastAttempted(t, id, ago)
		return id
	}
}

// setFailureCount は seed 後に failure_count を直接指定値に更新する。
func setFailureCount(t *testing.T, id uuid.UUID, count int) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`UPDATE shop.outbox_events SET failure_count = $2 WHERE event_id = $1`, id, count)
	require.NoError(t, err)
}

// insertWithFailureCount は failure_count を指定値に設定した行を作る seed 関数。
func insertWithFailureCount(count int) func(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
	return func(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
		id := insertOutboxRow(t, testIdx, seedIdx, []byte(payload))
		setFailureCount(t, id, count)
		return id
	}
}

// insertExhausted は failure_count が閾値に到達した行を作る seed 関数。
func insertExhausted(threshold int) func(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
	return insertWithFailureCount(threshold)
}

// defaultVisibility はケース指定がない時の visibility timeout (30s)。
const defaultVisibility = 30 * time.Second

// defaultFailureThreshold はケース指定がない時の failure threshold。
// 既存テストは閾値超過行を扱わないため、十分大きい値を設定。
const defaultFailureThreshold = 100

func TestOutboxRepository_ClaimUnpublished(t *testing.T) {
	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("未配信イベントの取得", func(t *testing.T) {
		const failureThreshold = 3

		tests := []struct {
			name              string
			seeds             []seed
			limit             int
			visibilityTimeout time.Duration
			failureThreshold  int
			wantPayloads      []string // order-insensitive. 空スライスは「何も claim されない」を表す。
		}{
			{
				name: "未配信行があるとき、payload そのままで返す",
				seeds: []seed{
					{payload: `{"k":"a"}`, insert: insertUnpublished},
					{payload: `{"k":"b"}`, insert: insertUnpublished},
				},
				limit:             10,
				visibilityTimeout: defaultVisibility,
				failureThreshold:  defaultFailureThreshold,
				wantPayloads:      []string{`{"k":"a"}`, `{"k":"b"}`},
			},
			{
				name: "published_at が埋まった行があるとき、スキップする",
				seeds: []seed{
					{payload: `{"k":"old"}`, insert: insertAlreadyPublished},
					{payload: `{"k":"new"}`, insert: insertUnpublished},
				},
				limit:             10,
				visibilityTimeout: defaultVisibility,
				failureThreshold:  defaultFailureThreshold,
				wantPayloads:      []string{`{"k":"new"}`},
			},
			{
				name: "visibility timeout 以内に試行された行があるとき、in-flight としてスキップする",
				seeds: []seed{
					{payload: `{"k":"in-flight"}`, insert: insertInFlight(5 * time.Second)},
					{payload: `{"k":"available"}`, insert: insertUnpublished},
				},
				limit:             10,
				visibilityTimeout: defaultVisibility,
				failureThreshold:  defaultFailureThreshold,
				wantPayloads:      []string{`{"k":"available"}`},
			},
			{
				name: "visibility timeout を超えた行があるとき、再 claim 対象にする (worker クラッシュ後の再試行)",
				seeds: []seed{
					{payload: `{"k":"recovered"}`, insert: insertInFlight(60 * time.Second)},
				},
				limit:             10,
				visibilityTimeout: defaultVisibility,
				failureThreshold:  defaultFailureThreshold,
				wantPayloads:      []string{`{"k":"recovered"}`},
			},
			{
				name:              "未配信行が無いとき、空で返す",
				seeds:             nil,
				limit:             10,
				visibilityTimeout: defaultVisibility,
				failureThreshold:  defaultFailureThreshold,
				wantPayloads:      []string{},
			},
			{
				name: "failure_count が閾値に達した行があるとき、スキップする",
				seeds: []seed{
					{payload: `{"k":"exhausted"}`, insert: insertExhausted(failureThreshold)},
					{payload: `{"k":"healthy"}`, insert: insertUnpublished},
				},
				limit:             10,
				visibilityTimeout: defaultVisibility,
				failureThreshold:  failureThreshold,
				wantPayloads:      []string{`{"k":"healthy"}`},
			},
			{
				name: "failure_count が閾値未満の行があるとき、claim される",
				seeds: []seed{
					{payload: `{"k":"below-threshold"}`, insert: insertWithFailureCount(failureThreshold - 1)},
				},
				limit:             10,
				visibilityTimeout: defaultVisibility,
				failureThreshold:  failureThreshold,
				wantPayloads:      []string{`{"k":"below-threshold"}`},
			},
		}

		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				for j, s := range tt.seeds {
					s.insert(t, i, j, s.payload)
				}

				claimed, err := repo.ClaimUnpublished(ctx, tt.limit, tt.visibilityTimeout, tt.failureThreshold)
				require.NoError(t, err)

				got := make([]string, len(claimed))
				for k, ev := range claimed {
					got[k] = string(ev.Payload)
				}
				assert.ElementsMatch(t, tt.wantPayloads, normalizeJSON(t, got))
			})
		}

		t.Run("上限を超えて取得せず、取得されなかった行は未試行のまま残る", func(t *testing.T) {
			// どの行が選ばれるかは同 ms 挿入時に不定なので、件数のみで固定する。
			sharedPg.Truncate(t)

			const totalSeeded = 3
			const limit = 2
			for j := range totalSeeded {
				insertUnpublished(t, 0, j, fmt.Sprintf(`{"k":"%d"}`, j))
			}

			claimed, err := repo.ClaimUnpublished(ctx, limit, defaultVisibility, defaultFailureThreshold)
			require.NoError(t, err)
			assert.Len(t, claimed, limit, "limit で claim 件数が制限される")

			var remaining int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM shop.outbox_events
				  WHERE published_at IS NULL AND last_attempted_at IS NULL`,
			).Scan(&remaining))
			assert.Equal(t, totalSeeded-limit, remaining, "claim されなかった行は未試行のまま残る")
		})

		t.Run("取得成功後、last_attempted_at が now() に更新される", func(t *testing.T) {
			sharedPg.Truncate(t)

			id := insertOutboxRow(t, 0, 0, []byte(`{"k":"v"}`))

			_, err := repo.ClaimUnpublished(ctx, 10, defaultVisibility, defaultFailureThreshold)
			require.NoError(t, err)

			var lastAttemptedNotNull bool
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT last_attempted_at IS NOT NULL FROM shop.outbox_events WHERE event_id = $1`,
				id).Scan(&lastAttemptedNotNull))
			assert.True(t, lastAttemptedNotNull, "claim 成功後は last_attempted_at が now() に更新される")
		})
	})
}

func TestOutboxRepository_MarkPublished(t *testing.T) {
	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("配信済みマーク", func(t *testing.T) {
		tests := []struct {
			name string
			seed seed
		}{
			{
				name: "未配信行のとき、published_at を立て last_error を解除する",
				seed: seed{payload: `{"k":"v"}`, insert: insertUnpublished},
			},
			{
				// 同じ event を別 worker が重複処理しても落ちないこと (at-least-once 契約の一部)。
				name: "既配信行に再呼び出ししても、冪等に配信済みのまま",
				seed: seed{payload: `{"k":"v"}`, insert: insertAlreadyPublished},
			},
		}

		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				id := tt.seed.insert(t, i, 0, tt.seed.payload)

				require.NoError(t, repo.MarkPublished(ctx, id))

				var publishedAtNotNull bool
				var lastError *string
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT published_at IS NOT NULL, last_error FROM shop.outbox_events WHERE event_id = $1`,
					id).Scan(&publishedAtNotNull, &lastError))
				assert.True(t, publishedAtNotNull, "published_at が立つ")
				assert.Nil(t, lastError, "last_error は解除される")
			})
		}
	})
}

func TestOutboxRepository_RecordFailure(t *testing.T) {
	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	ctx := context.Background()

	// recordN は RecordFailure を n 回呼んで行を「n 回連続失敗済み」状態にするヘルパ。
	// テストケース間で priorFailures の初期化を宣言的に揃える。
	recordN := func(n int, msgPrefix string) func(t *testing.T, id uuid.UUID) {
		return func(t *testing.T, id uuid.UUID) {
			for i := range n {
				require.NoError(t, repo.RecordFailure(context.Background(), id, fmt.Sprintf("%s-%d", msgPrefix, i)))
			}
		}
	}
	noPrior := func(t *testing.T, id uuid.UUID) {}

	t.Run("失敗の記録", func(t *testing.T) {
		tests := []struct {
			name             string
			priorFailures    func(t *testing.T, id uuid.UUID) // 本体呼び出し前の状態作り
			errMsg           string
			wantFailureCount int
			wantLastError    string
		}{
			{
				name:             "初回失敗のとき、failure_count=1 で last_error を記録する",
				priorFailures:    noPrior,
				errMsg:           "pubsub down",
				wantFailureCount: 1,
				wantLastError:    "pubsub down",
			},
			{
				name:             "連続失敗のとき、failure_count が積み上がる (死蔵検知の素材)",
				priorFailures:    recordN(2, "prior"),
				errMsg:           "still down",
				wantFailureCount: 3,
				wantLastError:    "still down",
			},
			{
				name:             "再失敗のとき、last_error が直近エラーで上書きされる",
				priorFailures:    recordN(1, "prior"),
				errMsg:           "newer error",
				wantFailureCount: 2,
				wantLastError:    "newer error",
			},
		}

		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				id := insertOutboxRow(t, i, 0, []byte(`{"k":"v"}`))
				tt.priorFailures(t, id)

				require.NoError(t, repo.RecordFailure(ctx, id, tt.errMsg))

				var fc int
				var lastError *string
				var publishedAtNotNull bool
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT failure_count, last_error, published_at IS NOT NULL FROM shop.outbox_events WHERE event_id = $1`,
					id).Scan(&fc, &lastError, &publishedAtNotNull))
				assert.Equal(t, tt.wantFailureCount, fc)
				require.NotNil(t, lastError)
				assert.Equal(t, tt.wantLastError, *lastError)
				assert.False(t, publishedAtNotNull, "失敗は published_at に影響しない")
			})
		}
	})
}
