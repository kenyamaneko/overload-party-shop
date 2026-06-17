package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

func resolvePurchaseTokenTable(platform string) (string, error) {
	switch platform {
	case domain.PlatformIOS:
		return "shop.apple_purchase_tokens", nil
	case domain.PlatformAndroid:
		return "shop.google_purchase_tokens", nil
	default:
		return "", fmt.Errorf("%w: purchase platform %q", port.ErrUnsupportedPlatform, platform)
	}
}

// insertOneTimePurchaseAndToken は one_time_purchases と対応する token テーブルへの挿入を同一 tx で行う。
// 既存 token があれば created=false で existing purchase_id を埋めて返す。
func insertOneTimePurchaseAndToken(ctx context.Context, tx pgx.Tx, purchase *domain.OneTimePurchase, platform, purchaseToken string) (created bool, err error) {
	table, err := resolvePurchaseTokenTable(platform)
	if err != nil {
		return false, err
	}

	var existingPurchaseID int64
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT purchase_id FROM %s WHERE token = $1`, table),
		purchaseToken,
	).Scan(&existingPurchaseID)
	if err == nil {
		purchase.PurchaseID = existingPurchaseID
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check existing purchase token: %w", err)
	}

	if err := tx.QueryRow(ctx,
		`INSERT INTO shop.one_time_purchases (player_id, product_id, purchased_at)
		 VALUES ($1,$2,$3) RETURNING purchase_id`,
		purchase.PlayerID, purchase.ProductID, purchase.PurchasedAt,
	).Scan(&purchase.PurchaseID); err != nil {
		return false, fmt.Errorf("insert purchase: %w", err)
	}

	if _, err := tx.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (token, purchase_id) VALUES ($1, $2)`, table),
		purchaseToken, purchase.PurchaseID,
	); err != nil {
		return false, fmt.Errorf("insert purchase token: %w", err)
	}
	return true, nil
}
