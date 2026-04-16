package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
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

// outbox 行は aggregate repo 経由でのみ書き込まれる (package 外に書き込み API を
// 公開しない設計)。ProcessBatch のテストでは faction_purchase 経由で事前に outbox 行を
// 植えた上で worker 側の claim/publish/mark フローを検証する。
func seedOutboxViaFactionPurchase(t *testing.T, playerID, faction, token string, payload []byte) uuid.UUID {
	t.Helper()
	id := uuid.New()
	factionRepo := postgres.NewFactionPurchaseRepository(sharedPg.Pool)
	purchase := &apishop.OneTimePurchase{PlayerID: playerID, ProductID: "faction_" + faction, PurchasedAt: time.Now().UTC()}
	_, err := factionRepo.CreatePurchase(context.Background(), purchase, faction, apishop.PlatformIOS, token,
		port.OutboxEvent{EventID: id, Topic: "faction-selected", Payload: payload})
	require.NoError(t, err)
	return id
}

// countOutboxRows は発行状態別の行数を返す。
func countOutboxRows(t *testing.T) (total, unpublished int) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.outbox_events`).Scan(&total))
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM shop.outbox_events WHERE published_at IS NULL`).Scan(&unpublished))
	return total, unpublished
}

func TestOutboxRepository_ProcessBatch_AllSucceed(t *testing.T) {
	sharedPg.Truncate(t)
	ctx := context.Background()

	seedOutboxViaFactionPurchase(t, "11111111-0000-0000-0000-000000000001", "Tenki", "tok-A", []byte(`{"kind":"A"}`))
	seedOutboxViaFactionPurchase(t, "11111111-0000-0000-0000-000000000002", "SHE", "tok-B", []byte(`{"kind":"B"}`))

	repo := postgres.NewOutboxRepository(sharedPg.Pool)

	var delivered []string
	succeeded, failed, err := repo.ProcessBatch(ctx, 10, func(_ context.Context, ev postgres.ClaimedOutboxEvent) error {
		delivered = append(delivered, string(ev.Payload))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, succeeded)
	assert.Equal(t, 0, failed)
	// JSONB は DB 側で空白等が正規化されるため、意味的比較で固定する。
	require.Len(t, delivered, 2)
	assert.ElementsMatch(t, []string{`{"kind":"A"}`, `{"kind":"B"}`}, normalizeJSON(t, delivered))

	total, unpublished := countOutboxRows(t)
	assert.Equal(t, 2, total)
	assert.Equal(t, 0, unpublished, "全行が published_at 更新済みになる")
}

func TestOutboxRepository_ProcessBatch_PublishFailureRecorded(t *testing.T) {
	sharedPg.Truncate(t)
	ctx := context.Background()

	seedOutboxViaFactionPurchase(t, "22222222-0000-0000-0000-000000000001", "Tenki", "tok-fail", []byte(`{"k":"v"}`))

	repo := postgres.NewOutboxRepository(sharedPg.Pool)

	boom := errors.New("pubsub unavailable")
	succeeded, failed, err := repo.ProcessBatch(ctx, 10, func(_ context.Context, _ postgres.ClaimedOutboxEvent) error {
		return boom
	})
	require.NoError(t, err, "publish 単発失敗は ProcessBatch の戻りエラーにならない")
	assert.Equal(t, 0, succeeded)
	assert.Equal(t, 1, failed)

	total, unpublished := countOutboxRows(t)
	assert.Equal(t, 1, total)
	assert.Equal(t, 1, unpublished, "失敗行は未 publish のまま")

	var fc int
	var lastErr *string
	require.NoError(t, sharedPg.Pool.QueryRow(ctx,
		`SELECT failure_count, last_error FROM shop.outbox_events`).Scan(&fc, &lastErr))
	assert.Equal(t, 1, fc)
	require.NotNil(t, lastErr)
	assert.Contains(t, *lastErr, "pubsub unavailable")
}

// claim は「未 publish 行のみ」を対象にする。
func TestOutboxRepository_ProcessBatch_SkipsPublished(t *testing.T) {
	sharedPg.Truncate(t)
	ctx := context.Background()

	seedOutboxViaFactionPurchase(t, "33333333-0000-0000-0000-000000000001", "Tenki", "tok-old", []byte(`{"k":"old"}`))
	seedOutboxViaFactionPurchase(t, "33333333-0000-0000-0000-000000000002", "SHE", "tok-new", []byte(`{"k":"new"}`))

	// 1 行だけ手動で published_at を埋める (別 worker で配信済み相当)。
	_, err := sharedPg.Pool.Exec(ctx,
		`UPDATE shop.outbox_events SET published_at = now() WHERE payload = '{"k":"old"}'::jsonb`)
	require.NoError(t, err)

	repo := postgres.NewOutboxRepository(sharedPg.Pool)

	var seen []string
	succeeded, failed, err := repo.ProcessBatch(ctx, 10, func(_ context.Context, ev postgres.ClaimedOutboxEvent) error {
		seen = append(seen, string(ev.Payload))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 0, failed)
	assert.Equal(t, []string{`{"k":"new"}`}, normalizeJSON(t, seen), "published_at 済み行は claim されない")
}

// 混在バッチ (成功 + 失敗) でも各行独立に状態が反映される。
func TestOutboxRepository_ProcessBatch_MixedResults(t *testing.T) {
	sharedPg.Truncate(t)
	ctx := context.Background()

	okID := seedOutboxViaFactionPurchase(t, "44444444-0000-0000-0000-000000000001", "Tenki", "tok-ok", []byte(`{"k":"ok"}`))
	ngID := seedOutboxViaFactionPurchase(t, "44444444-0000-0000-0000-000000000002", "SHE", "tok-ng", []byte(`{"k":"ng"}`))

	repo := postgres.NewOutboxRepository(sharedPg.Pool)

	succeeded, failed, err := repo.ProcessBatch(ctx, 10, func(_ context.Context, ev postgres.ClaimedOutboxEvent) error {
		if ev.EventID == ngID {
			return errors.New("nope")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, failed)

	assertPublished := func(id uuid.UUID, want bool) {
		var publishedAt *string
		var fc int
		require.NoError(t, sharedPg.Pool.QueryRow(ctx,
			`SELECT published_at::text, failure_count FROM shop.outbox_events WHERE event_id = $1`, id,
		).Scan(&publishedAt, &fc))
		if want {
			assert.NotNil(t, publishedAt, "成功行は published_at が埋まる")
			assert.Equal(t, 0, fc)
		} else {
			assert.Nil(t, publishedAt, "失敗行は published_at が NULL のまま")
			assert.Equal(t, 1, fc)
		}
	}
	assertPublished(okID, true)
	assertPublished(ngID, false)
}

// limit は claim 行数を制限する。次回 tick で残りが拾われる。
func TestOutboxRepository_ProcessBatch_RespectsLimit(t *testing.T) {
	sharedPg.Truncate(t)
	ctx := context.Background()

	const n = 5
	for i := 0; i < n; i++ {
		seedOutboxViaFactionPurchase(t,
			fmt.Sprintf("55555555-0000-0000-0000-%012d", i),
			"Tenki",
			fmt.Sprintf("tok-%d", i),
			[]byte(`{"k":"v"}`))
	}
	// 同一ファクションは 2 行目以降 CHECK 違反になるため、2 行目以降は faction を変える。
	// seedOutboxViaFactionPurchase が player ごとに独立なので n 件が全て別 player で書ける。

	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	succeeded, _, err := repo.ProcessBatch(ctx, 2, func(_ context.Context, _ postgres.ClaimedOutboxEvent) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, succeeded)

	_, unpublished := countOutboxRows(t)
	assert.Equal(t, n-2, unpublished)
}

// 空テーブルでは何も起きずエラーも出ない。
func TestOutboxRepository_ProcessBatch_EmptyTable(t *testing.T) {
	sharedPg.Truncate(t)
	ctx := context.Background()
	repo := postgres.NewOutboxRepository(sharedPg.Pool)

	publishCalled := false
	succeeded, failed, err := repo.ProcessBatch(ctx, 10, func(_ context.Context, _ postgres.ClaimedOutboxEvent) error {
		publishCalled = true
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 0, succeeded)
	assert.Equal(t, 0, failed)
	assert.False(t, publishCalled, "行がないと publish 関数は呼ばれない")
}
