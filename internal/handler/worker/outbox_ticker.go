// Package worker は周期起動される delivery 層のエントリポイントを提供する。
package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// outboxRunner は OutboxTicker が依存する usecase の狭い contract。
type outboxRunner interface {
	RunOnce(ctx context.Context) error
}

// OutboxTicker は一定間隔で outboxRunner.RunOnce を呼ぶ常駐 worker。
// tick ごとのエラーは ERROR ログ化して継続する (常駐プロセスを落とすと復旧機会を失うため)。
type OutboxTicker struct {
	runner   outboxRunner
	interval time.Duration
}

func NewOutboxTicker(runner outboxRunner, interval time.Duration) (*OutboxTicker, error) {
	if runner == nil {
		return nil, errors.New("outbox ticker: runner is nil")
	}
	if interval <= 0 {
		return nil, errors.New("outbox ticker: interval must be positive")
	}
	return &OutboxTicker{runner: runner, interval: interval}, nil
}

// Run は ctx が cancel されるまで tick ごとに runner.RunOnce を呼ぶ。
func (t *OutboxTicker) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	slog.Info("outbox ticker started", "interval", t.interval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox ticker stopping")
			return nil
		case <-ticker.C:
			if err := t.runner.RunOnce(ctx); err != nil {
				slog.Error("outbox tick failed", "error", err)
			}
		}
	}
}
