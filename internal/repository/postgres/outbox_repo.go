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

// writeOutboxEvent はビジネス行と同一 tx 内で outbox 行を INSERT する。
func writeOutboxEvent(ctx context.Context, tx pgx.Tx, event port.OutboxEvent) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO shop.outbox_events (event_id, event_type, payload)
		 VALUES ($1, $2, $3)`,
		event.EventID, event.EventType, event.Payload,
	); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

// OutboxRepository は outbox を消費するための data access 層。
type OutboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

// ClaimUnpublished は未配信かつ直近 visibilityTimeout 以内に試行されていない行を最大 limit 件取得する。
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
		var event port.ClaimedOutboxEvent
		if err := rows.Scan(&event.EventID, &event.EventType, &event.Payload, &event.FailureCount); err != nil {
			return nil, fmt.Errorf("scan claimed row: %w", err)
		}
		claimed = append(claimed, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed rows: %w", err)
	}
	return claimed, nil
}

// MarkPublished は publish 成功した行に published_at を立て、last_error を解除する (冪等)。
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
