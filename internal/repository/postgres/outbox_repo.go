package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// writeOutboxEvent はビジネス行と同一トランザクション内で outbox 行を INSERT する
// パッケージ内ヘルパ。dual-write 問題を避けるため、各 aggregate repo は
// Create/Update の tx に outbox INSERT を相乗りさせる。
func writeOutboxEvent(ctx context.Context, tx pgx.Tx, ev port.OutboxEvent) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO shop.outbox_events (event_id, topic, payload)
		 VALUES ($1, $2, $3)`,
		ev.EventID, ev.Topic, ev.Payload,
	); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

// ClaimedOutboxEvent は worker が publish を試行する 1 行分の情報。
// failure_count は閾値超過の alert 判定に使う。
type ClaimedOutboxEvent struct {
	EventID      uuid.UUID
	Topic        string
	Payload      []byte
	FailureCount int
}

// OutboxRepository は worker が outbox を消費するための repo。
// aggregate repo (faction/item/subscription) とは独立して構築する。
type OutboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

// ProcessBatch は未配信行を claim し、publish 関数を呼んだ結果を各行に反映する。
//
// 1 バッチ全体を単一トランザクションで処理する。SELECT ... FOR UPDATE SKIP LOCKED
// により複数 worker (複数 pod) が同じ行を拾わないことを担保し、publish 成否に応じて
// published_at / failure_count / last_error を同一 tx 内で更新してから COMMIT する。
//
// publish が失敗した行も last_attempted_at と failure_count は記録するため、
// tx 全体は publish エラーでは abort しない。publish 関数がエラーを返しても
// ProcessBatch 自体は nil を返し、行の更新だけは commit する。
// DB エラー (SELECT / UPDATE / COMMIT) だけが ProcessBatch の戻りエラーになる。
//
// 返り値は (publish 成功数, publish 失敗数, err)。
func (r *OutboxRepository) ProcessBatch(
	ctx context.Context,
	limit int,
	publish func(ctx context.Context, ev ClaimedOutboxEvent) error,
) (succeeded, failed int, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`SELECT event_id, topic, payload, failure_count
		   FROM shop.outbox_events
		  WHERE published_at IS NULL
		  ORDER BY created_at
		  LIMIT $1
		  FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("claim unpublished: %w", err)
	}

	var claimed []ClaimedOutboxEvent
	for rows.Next() {
		var ev ClaimedOutboxEvent
		if err := rows.Scan(&ev.EventID, &ev.Topic, &ev.Payload, &ev.FailureCount); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("scan claimed row: %w", err)
		}
		claimed = append(claimed, ev)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("iterate claimed rows: %w", err)
	}

	for _, ev := range claimed {
		pubErr := publish(ctx, ev)
		if pubErr == nil {
			if _, uerr := tx.Exec(ctx,
				`UPDATE shop.outbox_events
				    SET published_at = now(),
				        last_attempted_at = now(),
				        last_error = NULL
				  WHERE event_id = $1`,
				ev.EventID,
			); uerr != nil {
				return 0, 0, fmt.Errorf("mark published event_id=%s: %w", ev.EventID, uerr)
			}
			succeeded++
			continue
		}
		if _, uerr := tx.Exec(ctx,
			`UPDATE shop.outbox_events
			    SET failure_count = failure_count + 1,
			        last_attempted_at = now(),
			        last_error = $2
			  WHERE event_id = $1`,
			ev.EventID, pubErr.Error(),
		); uerr != nil {
			return 0, 0, fmt.Errorf("record failure event_id=%s: %w", ev.EventID, uerr)
		}
		failed++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit batch: %w", err)
	}
	return succeeded, failed, nil
}
