// Package repository は PostgreSQL に対するデータアクセスを実装する。
//
// トランザクションポリシー:
// 全 repository メソッドは context に載せられたトランザクションがあれば参加する
// （TxManager.RunInTx が設定）。単一ステートメントのメソッドは connFrom(ctx, pool)
// でトランザクションまたはコネクションプールを透過的に使い分ける。
// アトミック性が必要な複数ステートメントメソッドは既存トランザクションの有無を
// チェックし、なければ独自にトランザクションを開始する。
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.TxRunner = (*TxManager)(nil)

// dbtx は pgxpool.Pool と pgx.Tx の共通サブセット。
type dbtx interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

func txFromContext(ctx context.Context) dbtx {
	if tx, ok := ctx.Value(txKey{}).(dbtx); ok {
		return tx
	}
	return nil
}

func connFrom(ctx context.Context, pool *pgxpool.Pool) dbtx {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	return pool
}

// TxManager は pgxpool.Pool を使用して port.TxRunner を実装する。
type TxManager struct {
	pool *pgxpool.Pool
}

// NewTxManager は pgxpool.Pool を受け取り TxManager を構築する。
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// RunInTx はトランザクション内で fn を実行し、成功時に commit、失敗時に rollback する。
func (m *TxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := context.WithValue(ctx, txKey{}, tx)
	if err := fn(txCtx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

