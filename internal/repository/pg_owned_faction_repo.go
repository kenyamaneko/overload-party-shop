package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

// PgOwnedFactionRepository は shop 購入で獲得した faction 所有のローカル read model。
// account.player_factions のミラーではなく、この shop インスタンスが発行した購入
// イベントのみを追跡する。GetProducts で faction_set 商品の IsOwned フラグに使用。
var _ port.OwnedFactionRepo = (*PgOwnedFactionRepository)(nil)

// PgOwnedFactionRepository は PostgreSQL で OwnedFactionRepo を実装する。
type PgOwnedFactionRepository struct {
	pool *pgxpool.Pool
}

// NewPgOwnedFactionRepository は pgxpool.Pool を受け取り PgOwnedFactionRepository を構築する。
func NewPgOwnedFactionRepository(pool *pgxpool.Pool) *PgOwnedFactionRepository {
	return &PgOwnedFactionRepository{pool: pool}
}

// Add はプレイヤーの faction 所有を shop.player_owned_factions に記録する。
func (r *PgOwnedFactionRepository) Add(ctx context.Context, playerID, faction string) error {
	_, err := connFrom(ctx, r.pool).Exec(ctx,
		`INSERT INTO shop.player_owned_factions (player_id, faction)
		 VALUES ($1, $2)
		 ON CONFLICT (player_id, faction) DO NOTHING`,
		playerID, faction,
	)
	if err != nil {
		return fmt.Errorf("insert shop.player_owned_factions: %w", err)
	}
	return nil
}

// List はプレイヤーが shop 購入で所有する faction 一覧を返す。
func (r *PgOwnedFactionRepository) List(ctx context.Context, playerID string) ([]string, error) {
	rows, err := connFrom(ctx, r.pool).Query(ctx,
		`SELECT faction FROM shop.player_owned_factions WHERE player_id = $1`,
		playerID)
	if err != nil {
		return nil, fmt.Errorf("query shop.player_owned_factions: %w", err)
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
