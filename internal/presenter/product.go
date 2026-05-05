package presenter

import (
	"encoding/json"
	"fmt"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// ToProductResponse は domain.ProductWithOwnership を wire の ProductResponse に詰め替える。
// type 固有属性 (FactionSetProduct.Faction / CosmeticProduct.{ItemType,ItemNo}) は
// wire 側 Content (json.RawMessage) に再 marshal して外部 API 形状を維持する。
func ToProductResponse(it domain.ProductWithOwnership) (apishop.ProductResponse, error) {
	common := it.ProductView.Common()
	content, err := encodeProductContent(it.ProductView)
	if err != nil {
		return apishop.ProductResponse{}, fmt.Errorf("encode content for %s: %w", common.ProductID, err)
	}
	return apishop.ProductResponse{
		ProductID:   common.ProductID,
		Name:        common.Name,
		Type:        common.Type,
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

// encodeProductContent は per-type 商品ビューを wire 互換の content JSON に直列化する。
// faction_set => {"faction": "<faction>"}, cosmetic => {"item_type":"...","item_no":...},
// subscription => {} (型固有属性なし)。
func encodeProductContent(pv domain.ProductView) (json.RawMessage, error) {
	switch p := pv.(type) {
	case domain.FactionSetProduct:
		return json.Marshal(struct {
			Faction string `json:"faction"`
		}{Faction: p.Faction})
	case domain.CosmeticProduct:
		return json.Marshal(struct {
			ItemType string `json:"item_type"`
			ItemNo   int64  `json:"item_no"`
		}{ItemType: p.ItemType, ItemNo: p.ItemNo})
	case domain.SubscriptionProduct:
		return json.RawMessage("{}"), nil
	default:
		return nil, fmt.Errorf("unknown product view type %T", pv)
	}
}
