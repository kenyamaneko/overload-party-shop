// Package outbox は Transactional Outbox パターンの消費側ユースケースを提供する。
package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// Relay は outbox 行を claim → publish → mark/fail に振り分ける Polling Publisher。
type Relay struct {
	store             port.OutboxStore
	pub               port.RawEventPublisher
	batchSize         int
	failureThreshold  int
	visibilityTimeout time.Duration
}

// Config は Relay の駆動パラメータ。ゼロ値は禁止。
type Config struct {
	// BatchSize は 1 回の RunOnce で claim する最大行数。
	BatchSize int
	// FailureThreshold はこの回数以上の連続失敗で ERROR ログを出す閾値 (死蔵検知)。
	FailureThreshold int
	// VisibilityTimeout は claim された行が他 worker から隠蔽される期間。
	VisibilityTimeout time.Duration
}

func New(store port.OutboxStore, pub port.RawEventPublisher, cfg Config) (*Relay, error) {
	if store == nil {
		return nil, errors.New("outbox relay: store is nil")
	}
	if pub == nil {
		return nil, errors.New("outbox relay: publisher is nil")
	}
	if cfg.BatchSize <= 0 {
		return nil, errors.New("outbox relay: BatchSize must be positive")
	}
	if cfg.FailureThreshold <= 0 {
		return nil, errors.New("outbox relay: FailureThreshold must be positive")
	}
	if cfg.VisibilityTimeout <= 0 {
		return nil, errors.New("outbox relay: VisibilityTimeout must be positive")
	}
	return &Relay{
		store:             store,
		pub:               pub,
		batchSize:         cfg.BatchSize,
		failureThreshold:  cfg.FailureThreshold,
		visibilityTimeout: cfg.VisibilityTimeout,
	}, nil
}

// RunOnce は 1 バッチ分の claim + publish + 結果記録を実行する。
// claim 自体が失敗した場合のみエラーを返す。各行の publish 失敗は RecordFailure で記録する。
func (r *Relay) RunOnce(ctx context.Context) error {
	claimed, err := r.store.ClaimUnpublished(ctx, r.batchSize, r.visibilityTimeout, r.failureThreshold)
	if err != nil {
		return fmt.Errorf("claim unpublished: %w", err)
	}
	for _, ev := range claimed {
		r.processOne(ctx, ev)
	}
	return nil
}

// processOne は 1 行の publish と結果記録を行う (同バッチ内の他行を止めないため返り値なし)。
func (r *Relay) processOne(ctx context.Context, ev port.ClaimedOutboxEvent) {
	pubErr := r.pub.Publish(ctx, ev.EventType, ev.Payload)
	if pubErr == nil {
		if err := r.store.MarkPublished(ctx, ev.EventID); err != nil {
			slog.Error("mark published failed", "event_id", ev.EventID, "error", err)
		}
		return
	}
	attempts := ev.FailureCount + 1
	if attempts >= r.failureThreshold {
		slog.Error("outbox event stuck",
			"event_id", ev.EventID, "event_type", ev.EventType, "attempts", attempts, "error", pubErr)
	} else {
		slog.Warn("outbox publish failed",
			"event_id", ev.EventID, "event_type", ev.EventType, "attempts", attempts, "error", pubErr)
	}
	if err := r.store.RecordFailure(ctx, ev.EventID, pubErr.Error()); err != nil {
		slog.Error("record failure failed", "event_id", ev.EventID, "error", err)
	}
}
