// Package postgres は port.*Repo / port.OutboxStore の PostgreSQL 実装を提供する。
// ビジネス行と outbox 行を同一 tx で書き、pgx の positional Scan で domain 型へ
// 直接読み書きする。
package postgres
