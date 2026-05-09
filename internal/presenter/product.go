package presenter

import (
	"fmt"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// ToProductResponse は domain.ProductWithOwnership を wire の ProductResponse に詰め替える。
// type 固有属性 (FactionSetProduct.Faction / CosmeticProduct.{ItemType,ItemNo}) は
// wire 側 Content (map) に詰め直して外部 API 形状を維持する。
func ToProductResponse(it domain.ProductWithOwnership) (apishop.ProductResponse, error) {
	common := it.ProductView.Common()
	content, err := buildProductContent(it.ProductView)
	if err != nil {
		return apishop.ProductResponse{}, fmt.Errorf("build content for %s: %w", common.ProductID, err)
	}
	return apishop.ProductResponse{
		ProductID:   common.ProductID,
		Name:        common.Name,
		Type:        apishop.ProductType(common.Type),
		Price:       common.Price,
		Content:     content,
		Description: common.Description,
		ImageURL:    common.ImageURL,
		IsActive:    common.IsActive,
		IsOwned:     it.IsOwned,
	}, nil
}

// ToProductResponses は ProductWithOwnership slice を wire slice に詰め替える。
func ToProductResponses(items []domain.ProductWithOwnership) ([]apishop.ProductResponse, error) {
	out := make([]apishop.ProductResponse, 0, len(items))
	for _, it := range items {
		resp, err := ToProductResponse(it)
		if err != nil {
			return nil, err
		}
		out = append(out, resp)
	}
	return out, nil
}

// buildProductContent は per-type 商品ビューから wire の content フィールド (map) を構築する。
// faction_set => {"faction": "<faction>"}, cosmetic => {"item_type":"...","item_no":...},
// subscription => {} (型固有属性なし)。
func buildProductContent(pv domain.ProductView) (map[string]interface{}, error) {
	switch p := pv.(type) {
	case domain.FactionSetProduct:
		return map[string]interface{}{"faction": p.Faction}, nil
	case domain.CosmeticProduct:
		return map[string]interface{}{"item_type": p.ItemType, "item_no": p.ItemNo}, nil
	case domain.SubscriptionProduct:
		return map[string]interface{}{}, nil
	default:
		return nil, fmt.Errorf("unknown product view type %T", pv)
	}
}
