package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/port"
)

func resolveTokenTablePrefix(platform string) (string, error) {
	switch platform {
	case domain.PlatformIOS:
		return "apple", nil
	case domain.PlatformAndroid:
		return "google", nil
	default:
		return "", fmt.Errorf("%w: platform %q", port.ErrUnsupportedPlatform, platform)
	}
}

func resolvePurchaseTokenTable(platform string) (string, error) {
	prefix, err := resolveTokenTablePrefix(platform)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("shop.%s_purchase_tokens", prefix), nil
}

// insertOneTimePurchaseAndToken は購入とその購入トークンを保存する。
// 同じ購入トークンが保存済みのときは何も保存せず、保存したかどうかを返す。
func insertOneTimePurchaseAndToken(ctx context.Context, tx pgx.Tx, purchase *domain.OneTimePurchase, platform, purchaseToken string) (isInserted bool, err error) {
	table, err := resolvePurchaseTokenTable(platform)
	if err != nil {
		return false, err
	}

	var purchaseID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO shop.one_time_purchases (player_id, product_id, purchased_at)
		 VALUES ($1,$2,$3) RETURNING purchase_id`,
		purchase.PlayerID, purchase.ProductID, purchase.PurchasedAt,
	).Scan(&purchaseID); err != nil {
		return false, fmt.Errorf("insert purchase: %w", err)
	}

	tag, err := tx.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (token, purchase_id) VALUES ($1, $2) ON CONFLICT (token) DO NOTHING`, table),
		purchaseToken, purchaseID,
	)
	if err != nil {
		return false, fmt.Errorf("insert purchase token: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
