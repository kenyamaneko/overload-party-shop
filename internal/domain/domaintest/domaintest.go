// Package domaintest は domain を使うテストが共有する部品を提供する。
package domaintest

import "github.com/kenyamaneko/overload-party-shop/internal/domain"

// NewPointer は値へのポインタを返す。
func NewPointer[T any](value T) *T {
	return &value
}

// NewCommonProduct は指定した商品種別のダミーの商品共通情報を返す。
func NewCommonProduct(productType string) domain.Product {
	return domain.Product{
		ProductID:   "TST-0001",
		Name:        "TST-NAME-0001",
		Type:        productType,
		Price:       980,
		Description: NewPointer("TST-DESC-0001"),
		ImageURL:    NewPointer("https://example.com/TST-0001.png"),
		IsActive:    true,
	}
}
