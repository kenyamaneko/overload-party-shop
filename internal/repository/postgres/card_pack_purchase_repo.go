package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

var _ port.CardPackPurchaseRepo = (*CardPackPurchaseRepository)(nil)

// CardPackPurchaseRepository は card_pack 商品 (faction を伴わない pure pack 購入) aggregate を扱う。
type CardPackPurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewCardPackPurchaseRepository(pool *pgxpool.Pool) *CardPackPurchaseRepository {
	return &CardPackPurchaseRepository{pool: pool}
}

func (r *CardPackPurchaseRepository) CreatePurchase(ctx context.Context, purchase *domain.OneTimePurchase, cardPackID, platform, purchaseToken string, eventOnCreate port.OutboxEvent) (created bool, err error) {
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
		`INSERT INTO shop.player_owned_card_packs (player_id, card_pack_id)
		 VALUES ($1, $2)`,
		purchase.PlayerID, cardPackID,
	); err != nil {
		return false, fmt.Errorf("insert owned card pack: %w", err)
	}

	if err := writeOutboxEvent(ctx, tx, eventOnCreate); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}
	return true, nil
}

func (r *CardPackPurchaseRepository) HasPlayerCardPack(ctx context.Context, playerID, cardPackID string) (bool, error) {
	var isOwned bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM shop.player_owned_card_packs
			WHERE player_id = $1 AND card_pack_id = $2
		)`,
		playerID, cardPackID,
	).Scan(&isOwned); err != nil {
		return false, fmt.Errorf("query owned card pack: %w", err)
	}
	return isOwned, nil
}
