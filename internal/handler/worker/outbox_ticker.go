// Package worker は周期起動される delivery 層のエントリポイントを提供する。
// HTTP handler が inbound 要求を受けて service を呼ぶのと対称に、worker は
// ticker 起点で service の RunOnce 系メソッドを呼ぶだけの薄い層に留める。
// orchestration / ビジネスロジックは service 層に閉じ込める。
package worker

import (
	"context"
	"errors"
	"log"
	"time"
)

// outboxRunner は OutboxTicker が依存する service の狭い契約。
// service.OutboxPublisher.RunOnce が満たす。interface にしておくことで
// handler/worker 側は service の内部実装を知らずに ticker 制御だけに集中できる。
type outboxRunner interface {
	RunOnce(ctx context.Context) error
}

// OutboxTicker は一定間隔で outboxRunner.RunOnce を呼ぶ常駐 worker。
// ctx キャンセルで終了し、tick ごとのエラーは ERROR ログ化して継続する
// (DB 一時障害で常駐プロセスを落とすと復旧機会を失うため)。
type OutboxTicker struct {
	runner   outboxRunner
	interval time.Duration
}

// NewOutboxTicker は OutboxTicker を構築する。
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

	log.Printf("shop: outbox ticker started (interval=%s)", t.interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("shop: outbox ticker stopping")
			return nil
		case <-ticker.C:
			if err := t.runner.RunOnce(ctx); err != nil {
				log.Printf("ERROR: outbox tick failed: %v", err)
			}
		}
	}
}
