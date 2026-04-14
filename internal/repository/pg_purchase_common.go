package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

func purchaseTokenTableForPlatform(platform string) (string, error) {
	switch platform {
	case apishop.PlatformIOS:
		return "shop.apple_purchase_tokens", nil
	case apishop.PlatformAndroid:
		return "shop.google_purchase_tokens", nil
	default:
		return "", fmt.Errorf("%w: purchase platform %q", port.ErrUnsupportedPlatform, platform)
	}
}

// insertOneTimePurchaseAndToken は shop.one_time_purchases + 対応する token テーブル
// への挿入を同一 tx 内で実行する。faction_purchase / item_purchase repo の共通処理。
//   - 既存 token があれば created=false、purchase.PurchaseID に既存 id を埋めて返す
//     (呼び出し側は owned_faction / player_item 挿入をスキップする責務を持つ)
//   - 新規なら INSERT 2 本を実行し created=true を返す
func insertOneTimePurchaseAndToken(ctx context.Context, tx pgx.Tx, purchase *apishop.OneTimePurchase, platform, purchaseToken string) (created bool, err error) {
	table, err := purchaseTokenTableForPlatform(platform)
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
