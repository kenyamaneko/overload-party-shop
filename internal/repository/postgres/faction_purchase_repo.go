package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.FactionPurchaseRepo = (*FactionPurchaseRepository)(nil)

// FactionPurchaseRepository は faction_set 購入 aggregate を扱う。
// 書き込みは shop.one_time_purchases + shop.{apple,google}_purchase_tokens +
// shop.player_owned_factions の 3 テーブルを同一 tx で更新する。
// shop.player_owned_factions は authoritative な account.player_factions の shop ローカル射影
// (cross-schema 読み込み不可のため GetProducts の IsOwned 判定用に独立保持)。
type FactionPurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewFactionPurchaseRepository(pool *pgxpool.Pool) *FactionPurchaseRepository {
	return &FactionPurchaseRepository{pool: pool}
}

func (r *FactionPurchaseRepository) CreatePurchase(ctx context.Context, purchase *apishop.OneTimePurchase, faction, platform, purchaseToken string, eventOnCreate port.OutboxEvent) (created bool, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err = insertOneTimePurchaseAndToken(ctx, tx, purchase, platform, purchaseToken)
	if err != nil {
		return false, err
	}
	if !created {
		return false, nil
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO shop.player_owned_factions (player_id, faction)
		 VALUES ($1, $2)`,
		purchase.PlayerID, faction,
	); err != nil {
		return false, fmt.Errorf("insert owned faction: %w", err)
	}

	if eventOnCreate.Topic != "" {
		if err := writeOutboxEvent(ctx, tx, eventOnCreate); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}
	return true, nil
}

func (r *FactionPurchaseRepository) ListOwnedFactions(ctx context.Context, playerID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT faction FROM shop.player_owned_factions WHERE player_id = $1`,
		playerID)
	if err != nil {
		return nil, fmt.Errorf("query owned factions: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, fmt.Errorf("scan faction: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate factions: %w", err)
	}
	return out, nil
}
