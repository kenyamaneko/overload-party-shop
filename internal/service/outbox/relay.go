// Package outbox は Transactional Outbox パターンの消費側ユースケースを提供する。
// shop のビジネスドメインからは独立しており、repo (data access) + publisher (adapter)
// を orchestrate する。呼び出し契機 (ticker / cron) は持たず、handler/worker 側が制御する。
package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// Relay は outbox 行を claim → publish → mark/fail に振り分ける message relay
// (Transactional Outbox パターンの "Polling Publisher" 役)。呼び出し契機
// (ticker / cron / 1 回だけ) は持たず、handler/worker 側で周期駆動する
// (依存方向: handler/worker → service → port)。
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
	// worker クラッシュ時はこの時間経過で自動的に再試行対象に戻る。
	// 典型的な publish 時間 (100ms〜1s) より十分長く (例 30s) 設定する。
	VisibilityTimeout time.Duration
}

// New は Relay を構築する。依存・ゼロ値バリデーションは起動時に行う。
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
// claim 自体が失敗した場合のみエラーを返す (DB 到達不能などの致命的状況)。
// 各行の publish 失敗は RecordFailure で記録し、RunOnce 自体はエラーにしない
// (visibility timeout 経過後に自動再試行される契約)。
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

// processOne は 1 行の publish と結果記録を行う。エラーを伝播しないのは
// 同バッチ内の他行処理を止めないため。記録自体が失敗した場合は ERROR ログだけ出して継続。
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
