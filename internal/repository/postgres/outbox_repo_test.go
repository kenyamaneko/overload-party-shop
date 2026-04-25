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

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
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

// validFactions は player_owned_factions CHECK 制約で許される値。
// 1 ケースあたり最大 4 seed まで (CHECK 制約の都合)。
var validFactions = []string{"Tenki", "SHE", "Sugar", "Tuners"}

// insertOutboxRow はユニークな player_id / token / faction で outbox 行を 1 本挿入する。
// 書き込み API は package 外に公開していないので faction_purchase 経由で作る。
// seed 追加の変種 (published 済み / in-flight 等) は apply 関数で後続の mutation を合成する。
func insertOutboxRow(t *testing.T, testIdx, seedIdx int, payload []byte) uuid.UUID {
	t.Helper()
	require.Less(t, seedIdx, len(validFactions), "1 ケースあたりの seed 数は faction CHECK 制約で 4 まで")
	id := uuid.New()
	playerID := fmt.Sprintf("%08d-%04d-%04d-0000-000000000000", testIdx+1, seedIdx, seedIdx)
	faction := validFactions[seedIdx]
	token := fmt.Sprintf("tok-%d-%d", testIdx, seedIdx)

	factionRepo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	purchase := &apishop.OneTimePurchase{PlayerID: playerID, ProductID: "faction_" + faction, PurchasedAt: time.Now().UTC()}
	_, err := factionRepo.CreatePurchase(context.Background(), purchase, faction, apishop.PlatformIOS, token,
		port.OutboxEvent{EventID: id, EventType: apishop.EventTypeFactionPurchased, Payload: payload})
	require.NoError(t, err)
	return id
}

// markPublished は seed 後に published_at を埋める（既配信行のシミュレーション）。
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

// insertExhausted は failure_count が閾値に到達した行を作る seed 関数。
func insertExhausted(threshold int) func(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
	return func(t *testing.T, testIdx, seedIdx int, payload string) uuid.UUID {
		id := insertOutboxRow(t, testIdx, seedIdx, []byte(payload))
		setFailureCount(t, id, threshold)
		return id
	}
}

// defaultVisibility はケース指定がない時の visibility timeout (30s)。
const defaultVisibility = 30 * time.Second

// defaultFailureThreshold はケース指定がない時の failure threshold。
// 既存テストは閾値超過行を扱わないため、十分大きい値を設定。
const defaultFailureThreshold = 100

func TestOutboxRepository_ClaimUnpublished(t *testing.T) {
	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	ctx := context.Background()

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
			name: "未配信行を payload そのままで返す",
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
			name: "published_at が埋まった行はスキップ",
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
			name: "visibility timeout 以内に試行された行はスキップ (in-flight 扱い)",
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
			name: "visibility timeout を超えた行は再 claim 対象 (worker クラッシュ後の再試行)",
			seeds: []seed{
				{payload: `{"k":"recovered"}`, insert: insertInFlight(60 * time.Second)},
			},
			limit:             10,
			visibilityTimeout: defaultVisibility,
			failureThreshold:  defaultFailureThreshold,
			wantPayloads:      []string{`{"k":"recovered"}`},
		},
		{
			name:              "未配信行がなければ空で返す",
			seeds:             nil,
			limit:             10,
			visibilityTimeout: defaultVisibility,
			failureThreshold:  defaultFailureThreshold,
			wantPayloads:      []string{},
		},
		{
			name: "failure_count が閾値に達した行はスキップ",
			seeds: []seed{
				{payload: `{"k":"exhausted"}`, insert: insertExhausted(failureThreshold)},
				{payload: `{"k":"healthy"}`, insert: insertUnpublished},
			},
			limit:             10,
			visibilityTimeout: defaultVisibility,
			failureThreshold:  failureThreshold,
			wantPayloads:      []string{`{"k":"healthy"}`},
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
}

// limit の効き目は「claim 件数 <= limit」「残り unpublished = 全体 - claim 数」で固定する。
// どの行が選ばれるかは同 ms 挿入時に不定なので、本体テーブルから分離して件数のみ検証する。
func TestOutboxRepository_ClaimUnpublished_RespectsLimit(t *testing.T) {
	sharedPg.Truncate(t)
	ctx := context.Background()
	repo := postgres.NewOutboxRepository(sharedPg.Pool)

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
}

// ClaimUnpublished は side-effect として last_attempted_at を更新する。これが
// 他 worker からの in-flight 扱いに必要な不変条件なので、独立テストとして固定。
func TestOutboxRepository_ClaimUnpublished_UpdatesLastAttemptedAt(t *testing.T) {
	sharedPg.Truncate(t)
	ctx := context.Background()
	repo := postgres.NewOutboxRepository(sharedPg.Pool)

	id := insertOutboxRow(t, 0, 0, []byte(`{"k":"v"}`))

	_, err := repo.ClaimUnpublished(ctx, 10, defaultVisibility, defaultFailureThreshold)
	require.NoError(t, err)

	var lastAttemptedNotNull bool
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT last_attempted_at IS NOT NULL FROM shop.outbox_events WHERE event_id = $1`,
		id).Scan(&lastAttemptedNotNull))
	assert.True(t, lastAttemptedNotNull, "claim 成功後は last_attempted_at が now() に更新される")
}

func TestOutboxRepository_MarkPublished(t *testing.T) {
	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	ctx := context.Background()

	tests := []struct {
		name  string
		seed  seed
	}{
		{
			name: "未配信行を配信済みにする",
			seed: seed{payload: `{"k":"v"}`, insert: insertUnpublished},
		},
		{
			// 同じ event を別 worker が重複処理しても落ちないこと (at-least-once 契約の一部)。
			name: "既配信行への再呼び出しは冪等",
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

	tests := []struct {
		name             string
		priorFailures    func(t *testing.T, id uuid.UUID) // 本体呼び出し前の状態作り
		errMsg           string
		wantFailureCount int
		wantLastError    string
	}{
		{
			name:             "初回失敗で failure_count=1、last_error を記録",
			priorFailures:    noPrior,
			errMsg:           "pubsub down",
			wantFailureCount: 1,
			wantLastError:    "pubsub down",
		},
		{
			name:             "連続失敗で failure_count が積み上がる (死蔵検知の素材)",
			priorFailures:    recordN(2, "prior"),
			errMsg:           "still down",
			wantFailureCount: 3,
			wantLastError:    "still down",
		},
		{
			name:             "last_error は直近エラーで上書きされる",
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
}
