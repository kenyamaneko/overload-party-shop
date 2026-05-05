package presenter_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
)

func TestToProductResponse(t *testing.T) {
	desc := "説明"
	img := "https://example.com/p.png"
	in := domain.ProductWithOwnership{
		Product: domain.Product{
			ProductID:   "faction_tenki",
			Name:        "天気",
			Type:        domain.ProductTypeFactionSet,
			Price:       980,
			Content:     json.RawMessage(`{"faction":"tenki"}`),
			Description: &desc,
			ImageURL:    &img,
			IsActive:    true,
		},
		IsOwned: true,
	}

	got := presenter.ToProductResponse(in)

	assert.Equal(t, "faction_tenki", got.ProductID)
	assert.Equal(t, "天気", got.Name)
	assert.Equal(t, domain.ProductTypeFactionSet, got.Type)
	assert.Equal(t, int64(980), got.Price)
	assert.JSONEq(t, `{"faction":"tenki"}`, string(got.Content))
	assert.Equal(t, &desc, got.Description)
	assert.Equal(t, &img, got.ImageURL)
	assert.True(t, got.IsActive)
	assert.True(t, got.IsOwned)
}

func TestToProductResponse_NilOptionalFieldsPropagate(t *testing.T) {
	in := domain.ProductWithOwnership{
		Product: domain.Product{
			ProductID:   "p1",
			Description: nil,
			ImageURL:    nil,
		},
	}

	got := presenter.ToProductResponse(in)

	assert.Nil(t, got.Description, "下書き等で description=null は wire でも nil で透過する")
	assert.Nil(t, got.ImageURL)
	assert.False(t, got.IsOwned)
}

func TestToProductResponses(t *testing.T) {
	in := []domain.ProductWithOwnership{
		{Product: domain.Product{ProductID: "p1"}, IsOwned: true},
		{Product: domain.Product{ProductID: "p2"}, IsOwned: false},
	}

	got := presenter.ToProductResponses(in)

	require.Len(t, got, 2)
	assert.Equal(t, "p1", got[0].ProductID)
	assert.True(t, got[0].IsOwned)
	assert.Equal(t, "p2", got[1].ProductID)
	assert.False(t, got[1].IsOwned)
}

func TestToProductResponses_EmptyInputReturnsEmptySlice(t *testing.T) {
	got := presenter.ToProductResponses(nil)
	assert.NotNil(t, got, "JSON エンコード時に null ではなく [] にするため空 slice を返す")
	assert.Empty(t, got)
}
