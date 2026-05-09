package presenter_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
	"github.com/kenyamaneko/overload-party-shop/internal/presenter"
	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

func TestToProductResponse(t *testing.T) {
	desc := "説明"
	img := "https://example.com/p.png"
	tests := []struct {
		name        string
		view        domain.ProductView
		isOwned     bool
		wantContent string
	}{
		{
			name: "faction_set は faction を content にエンコード",
			view: domain.FactionSetProduct{
				Product: domain.Product{
					ProductID:   "faction_tenki",
					Name:        "天気",
					Type:        domain.ProductTypeFactionSet,
					Price:       980,
					Description: &desc,
					ImageURL:    &img,
					IsActive:    true,
				},
				Faction: "Tenki",
			},
			isOwned:     true,
			wantContent: `{"faction":"Tenki"}`,
		},
		{
			name: "cosmetic は item_type / item_no を content にエンコード",
			view: domain.CosmeticProduct{
				Product: domain.Product{
					ProductID: "stamp_001",
					Name:      "スタンプ",
					Type:      domain.ProductTypeCosmetic,
					Price:     120,
					IsActive:  true,
				},
				ItemType: domain.ItemTypeStamp,
				ItemNo:   1,
			},
			isOwned:     false,
			wantContent: `{"item_type":"stamp","item_no":1}`,
		},
		{
			name: "subscription は空オブジェクトを content として返す",
			view: domain.SubscriptionProduct{
				Product: domain.Product{
					ProductID: "premium_monthly",
					Type:      domain.ProductTypeSubscription,
				},
			},
			wantContent: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := domain.ProductWithOwnership{ProductView: tt.view, IsOwned: tt.isOwned}

			got, err := presenter.ToProductResponse(in)
			require.NoError(t, err)

			common := tt.view.Common()
			assert.Equal(t, common.ProductID, got.ProductID)
			assert.Equal(t, common.Name, got.Name)
			assert.Equal(t, apishop.ProductType(common.Type), got.Type)
			assert.Equal(t, common.Price, got.Price)
			contentJSON, err := json.Marshal(got.Content)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantContent, string(contentJSON))
			assert.Equal(t, common.Description, got.Description)
			assert.Equal(t, common.ImageURL, got.ImageURL)
			assert.Equal(t, common.IsActive, got.IsActive)
			assert.Equal(t, tt.isOwned, got.IsOwned)
		})
	}
}

func TestToProductResponse_NilOptionalFieldsPropagate(t *testing.T) {
	in := domain.ProductWithOwnership{
		ProductView: domain.SubscriptionProduct{
			Product: domain.Product{
				ProductID:   "premium",
				Type:        domain.ProductTypeSubscription,
				Description: nil,
				ImageURL:    nil,
			},
		},
	}

	got, err := presenter.ToProductResponse(in)
	require.NoError(t, err)

	assert.Nil(t, got.Description, "下書き等で description=null は wire でも nil で透過する")
	assert.Nil(t, got.ImageURL)
	assert.False(t, got.IsOwned)
}

func TestToProductResponses(t *testing.T) {
	in := []domain.ProductWithOwnership{
		{
			ProductView: domain.FactionSetProduct{
				Product: domain.Product{ProductID: "p1", Type: domain.ProductTypeFactionSet},
				Faction: "SHE",
			},
			IsOwned: true,
		},
		{
			ProductView: domain.CosmeticProduct{
				Product:  domain.Product{ProductID: "p2", Type: domain.ProductTypeCosmetic},
				ItemType: domain.ItemTypeStamp,
				ItemNo:   1,
			},
			IsOwned: false,
		},
	}

	got, err := presenter.ToProductResponses(in)
	require.NoError(t, err)

	require.Len(t, got, 2)
	assert.Equal(t, "p1", got[0].ProductID)
	assert.True(t, got[0].IsOwned)
	assert.Equal(t, "p2", got[1].ProductID)
	assert.False(t, got[1].IsOwned)
}

func TestToProductResponses_EmptyInputReturnsEmptySlice(t *testing.T) {
	got, err := presenter.ToProductResponses(nil)
	require.NoError(t, err)
	assert.NotNil(t, got, "JSON エンコード時に null ではなく [] にするため空 slice を返す")
	assert.Empty(t, got)
}
