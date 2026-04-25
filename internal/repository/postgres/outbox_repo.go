package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.OutboxStore = (*OutboxRepository)(nil)

// writeOutboxEvent はビジネス行と同一トランザクション内で outbox 行を INSERT する
// パッケージ内ヘルパ。dual-write 問題を避けるため、各 aggregate repo は
// Create/Update の tx に outbox INSERT を相乗りさせる。
func writeOutboxEvent(ctx context.Context, tx pgx.Tx, ev port.OutboxEvent) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO shop.outbox_events (event_id, event_type, payload)
		 VALUES ($1, $2, $3)`,
		ev.EventID, ev.EventType, ev.Payload,
	); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

// OutboxRepository は outbox を消費するための薄い data access 層。
// claim → publish → mark の orchestration は呼び出し側 (service) が持つ。
// 二重配信を避けるために「visibility timeout」パターンを採用し、claim 時に
// last_attempted_at を更新、以降指定期間は他 worker から隠蔽する。worker が
// クラッシュしても timeout 経過で自動的に再試行対象に戻る。
type OutboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

// ClaimUnpublished は未配信で、かつ直近 visibilityTimeout 以内に試行されていない
// 行を最大 limit 件取得する。単一 SQL で「claim 対象の選定 + last_attempted_at 更新 +
// RETURNING」を完結させ、SKIP LOCKED により並行 worker と行の奪い合いをしない。
//
// 戻り行は last_attempted_at が now() に更新済み。以降 visibilityTimeout の間、
// 他 worker の ClaimUnpublished はこの行を除外する。
func (r *OutboxRepository) ClaimUnpublished(ctx context.Context, limit int, visibilityTimeout time.Duration, failureThreshold int) ([]port.ClaimedOutboxEvent, error) {
	// Postgres interval に渡せる text 表現。time.Duration.String() は "30s" 等だが
	// Postgres は "30 seconds" 形式しか受け付けないため、ms 単位で明示的に組み立てる。
	visibilityInterval := fmt.Sprintf("%d milliseconds", visibilityTimeout.Milliseconds())

	rows, err := r.pool.Query(ctx,
		`WITH claimed AS (
		    SELECT event_id
		      FROM shop.outbox_events
		     WHERE published_at IS NULL
		       AND failure_count < $3
		       AND (last_attempted_at IS NULL OR last_attempted_at < now() - $2::interval)
		     ORDER BY created_at
		     LIMIT $1
		     FOR UPDATE SKIP LOCKED
		)
		UPDATE shop.outbox_events o
		   SET last_attempted_at = now()
		  FROM claimed c
		 WHERE o.event_id = c.event_id
		 RETURNING o.event_id, o.event_type, o.payload, o.failure_count`,
		limit, visibilityInterval, failureThreshold,
	)
	if err != nil {
		return nil, fmt.Errorf("claim unpublished: %w", err)
	}
	defer rows.Close()

	var claimed []port.ClaimedOutboxEvent
	for rows.Next() {
		var ev port.ClaimedOutboxEvent
		if err := rows.Scan(&ev.EventID, &ev.EventType, &ev.Payload, &ev.FailureCount); err != nil {
			return nil, fmt.Errorf("scan claimed row: %w", err)
		}
		claimed = append(claimed, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed rows: %w", err)
	}
	return claimed, nil
}

// MarkPublished は publish 成功した行に published_at を立て、last_error を解除する。
// 冪等: 既に published_at が入っている行に対して呼んでも壊れない (値の上書きのみ)。
func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE shop.outbox_events
		    SET published_at = now(),
		        last_attempted_at = now(),
		        last_error = NULL
		  WHERE event_id = $1`,
		eventID,
	); err != nil {
		return fmt.Errorf("mark published event_id=%s: %w", eventID, err)
	}
	return nil
}

// RecordFailure は publish 失敗した行の failure_count を +1 し、last_error を記録する。
// last_attempted_at はここでも更新する (claim → 失敗 → 記録 の間でも publish 試行時点を反映)。
func (r *OutboxRepository) RecordFailure(ctx context.Context, eventID uuid.UUID, errMsg string) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE shop.outbox_events
		    SET failure_count = failure_count + 1,
		        last_attempted_at = now(),
		        last_error = $2
		  WHERE event_id = $1`,
		eventID, errMsg,
	); err != nil {
		return fmt.Errorf("record failure event_id=%s: %w", eventID, err)
	}
	return nil
}
