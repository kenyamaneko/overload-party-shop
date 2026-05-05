package presenter

import (
	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// ToProductResponse は domain.ProductWithOwnership を wire の ProductResponse に詰め替えます。
func ToProductResponse(it domain.ProductWithOwnership) apishop.ProductResponse {
	p := it.Product
	return apishop.ProductResponse{
		ProductID:   p.ProductID,
		Name:        p.Name,
		Type:        p.Type,
		Price:       p.Price,
		Content:     p.Content,
		Description: p.Description,
		ImageURL:    p.ImageURL,
		IsActive:    p.IsActive,
		IsOwned:     it.IsOwned,
	}
}

// ToProductResponses は domain.ProductWithOwnership slice を wire slice に詰め替えます。
func ToProductResponses(items []domain.ProductWithOwnership) []apishop.ProductResponse {
	out := make([]apishop.ProductResponse, 0, len(items))
	for _, it := range items {
		out = append(out, ToProductResponse(it))
	}
	return out
}
