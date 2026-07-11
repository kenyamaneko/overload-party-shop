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
	t.Run("ProductResponse の組み立て", func(t *testing.T) {
		desc := "説明"
		img := "https://example.com/p.png"
		tests := []struct {
			name        string
			view        domain.ProductView
			isOwned     bool
			wantContent string
		}{
			{
				name: "faction_set のとき、faction が content にエンコードされる",
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
				name: "cosmetic のとき、item_type と item_no が content にエンコードされる",
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
				name: "subscription のとき、content が空オブジェクトになる",
				view: domain.SubscriptionProduct{
					Product: domain.Product{
						ProductID: "premium_monthly",
						Type:      domain.ProductTypeSubscription,
					},
				},
				wantContent: `{}`,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				in := domain.ProductWithOwnership{ProductView: tc.view, IsOwned: tc.isOwned}

				got, err := presenter.ToProductResponse(in)
				require.NoError(t, err)

				common := tc.view.Common()
				assert.Equal(t, common.ProductID, got.ProductID)
				assert.Equal(t, common.Name, got.Name)
				assert.Equal(t, apishop.ProductType(common.Type), got.Type)
				assert.Equal(t, common.Price, got.Price)
				contentJSON, err := json.Marshal(got.Content)
				require.NoError(t, err)
				assert.JSONEq(t, tc.wantContent, string(contentJSON))
				assert.Equal(t, common.Description, got.Description)
				assert.Equal(t, common.ImageURL, got.ImageURL)
				assert.Equal(t, common.IsActive, got.IsActive)
				assert.Equal(t, tc.isOwned, got.IsOwned)
			})
		}

		t.Run("description と imageURL が未設定のとき、wire でも nil で透過する", func(t *testing.T) {
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

			// 下書き等で description=null は wire でも nil で透過する
			assert.Nil(t, got.Description)
			assert.Nil(t, got.ImageURL)
			assert.False(t, got.IsOwned)
		})
	})
}

func TestToProductResponses(t *testing.T) {
	t.Run("ProductResponse スライスへの変換", func(t *testing.T) {
		t.Run("複数件のとき、入力順のまま各要素が変換される", func(t *testing.T) {
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
		})

		t.Run("nil が渡されたとき、nil ではなく空スライスが返る", func(t *testing.T) {
			got, err := presenter.ToProductResponses(nil)
			require.NoError(t, err)
			// JSON エンコード時に null ではなく [] にするため空 slice を返す
			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	})
}
