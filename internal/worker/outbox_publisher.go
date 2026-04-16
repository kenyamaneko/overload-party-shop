// Package worker は shop プロセスと同居する常駐バックグラウンド処理を提供する。
//
// outbox publisher は shop.outbox_events から未配信行を claim し、RawEventPublisher
// 経由で Pub/Sub に送出する。ビジネス行と outbox 行が同一 tx で commit されている
// ことを前提に at-least-once 配信を担保する (dual-write 問題の解決)。
package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres"
)

// outboxStore は worker が依存する outbox repo の狭い contract。
// テストではモック差し替えが楽になるよう interface で切っている。
type outboxStore interface {
	ProcessBatch(ctx context.Context, limit int, publish func(ctx context.Context, ev postgres.ClaimedOutboxEvent) error) (succeeded, failed int, err error)
}

// OutboxPublisher は一定間隔で outbox を消費する常駐 worker。
//
// 駆動方式: ticker ベースのポーリング (poll 間隔は Config.PollInterval)。
// 1 tick ごとに最大 BatchSize 件を FOR UPDATE SKIP LOCKED で claim し、
// 各行を RawEventPublisher で送出する。結果 (published_at 更新 / failure_count インクリメント)
// は同一 tx で commit される。
//
// 複数 pod で同時に走っても SKIP LOCKED により同じ行を奪い合わない。
type OutboxPublisher struct {
	store            outboxStore
	pub              port.RawEventPublisher
	pollInterval     time.Duration
	batchSize        int
	failureThreshold int
}

// Config は OutboxPublisher の駆動パラメータ。全フィールド必須 (zero 値は禁止)。
type Config struct {
	// PollInterval はバッチ取得の間隔。1s 前後を想定。
	PollInterval time.Duration
	// BatchSize は 1 tick で claim する最大行数。
	BatchSize int
	// FailureThreshold はこの回数を超えて失敗が継続した outbox 行を ERROR ログで
	// 通知する閾値 (死蔵検知)。常駐プロセスなので自動復旧手段はなく、監視が唯一の対応。
	FailureThreshold int
}

// New は OutboxPublisher を構築する。ゼロ値の Config 値は受け付けない。
func New(store *postgres.OutboxRepository, pub port.RawEventPublisher, cfg Config) (*OutboxPublisher, error) {
	if store == nil {
		return nil, errors.New("worker: outbox store is nil")
	}
	if pub == nil {
		return nil, errors.New("worker: publisher is nil")
	}
	if cfg.PollInterval <= 0 {
		return nil, errors.New("worker: PollInterval must be positive")
	}
	if cfg.BatchSize <= 0 {
		return nil, errors.New("worker: BatchSize must be positive")
	}
	if cfg.FailureThreshold <= 0 {
		return nil, errors.New("worker: FailureThreshold must be positive")
	}
	return &OutboxPublisher{
		store:            store,
		pub:              pub,
		pollInterval:     cfg.PollInterval,
		batchSize:        cfg.BatchSize,
		failureThreshold: cfg.FailureThreshold,
	}, nil
}

// Run は ctx が cancel されるまで tick ごとに runOnce を呼ぶ。
// runOnce のエラーは ERROR ログを出すだけで worker は継続する
// (DB 一時障害で落ちると復旧機会を失うため)。
func (w *OutboxPublisher) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	log.Printf("shop: outbox worker started (poll=%s batch=%d failure_threshold=%d)",
		w.pollInterval, w.batchSize, w.failureThreshold)

	for {
		select {
		case <-ctx.Done():
			log.Printf("shop: outbox worker stopping")
			return nil
		case <-ticker.C:
			if err := w.runOnce(ctx); err != nil {
				log.Printf("ERROR: outbox worker batch failed: %v", err)
			}
		}
	}
}

// runOnce は 1 バッチ分の claim + publish + mark を実行する。
// 戻りエラーは DB 系の致命的エラーのみ。publish 単発の失敗は outbox 行に
// failure_count を積むだけで、tick 間で自動再試行される。
func (w *OutboxPublisher) runOnce(ctx context.Context) error {
	_, _, err := w.store.ProcessBatch(ctx, w.batchSize, func(ctx context.Context, ev postgres.ClaimedOutboxEvent) error {
		if err := w.pub.Publish(ctx, ev.Topic, ev.Payload); err != nil {
			// 閾値を超えたら ERROR 相当で積極的に吐く。同じ event_id が
			// 何度も出るのは想定内 (監視側で event_id で集約する)。
			if ev.FailureCount+1 >= w.failureThreshold {
				log.Printf("ERROR: outbox event stuck (event_id=%s topic=%s attempts=%d): %v",
					ev.EventID, ev.Topic, ev.FailureCount+1, err)
			} else {
				log.Printf("outbox publish failed (event_id=%s topic=%s attempts=%d): %v",
					ev.EventID, ev.Topic, ev.FailureCount+1, err)
			}
			return err
		}
		return nil
	})
	return err
}
