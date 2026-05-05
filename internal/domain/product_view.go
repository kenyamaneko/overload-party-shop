package domain

import "fmt"

// ProductView は per-type 商品 (FactionSetProduct / CosmeticProduct / SubscriptionProduct) を束ねる sealed interface。
// usecase は ProductView を型 switch して type 固有属性へアクセスする。
type ProductView interface {
	// Common は商品の type 横断共通属性を返す。
	Common() Product
	isProductView()
}

func (p FactionSetProduct) Common() Product { return p.Product }
func (FactionSetProduct) isProductView()    {}

func (p CosmeticProduct) Common() Product { return p.Product }
func (CosmeticProduct) isProductView()    {}

func (p SubscriptionProduct) Common() Product { return p.Product }
func (SubscriptionProduct) isProductView()    {}

// NewFactionSetProduct は有効な faction を持つ FactionSetProduct を構築する。
func NewFactionSetProduct(common Product, faction string) FactionSetProduct {
	return FactionSetProduct{Product: common, Faction: faction}
}

// NewCosmeticProduct は有効な (item_type, item_no) を持つ CosmeticProduct を構築する。
func NewCosmeticProduct(common Product, itemType string, itemNo int64) CosmeticProduct {
	return CosmeticProduct{Product: common, ItemType: itemType, ItemNo: itemNo}
}

// NewSubscriptionProduct は SubscriptionProduct を構築する (type 固有属性なし)。
func NewSubscriptionProduct(common Product) SubscriptionProduct {
	return SubscriptionProduct{Product: common}
}

// NewProductView は type discriminator と optional な type 固有入力から ProductView を組み立てる dispatcher。
// repository が副表 LEFT JOIN で取得した nullable 列をそのまま渡せるよう optional pointer で受ける。
// type と入力値の整合は本関数が担保する (例: type=faction_set だが faction=nil は error)。
func NewProductView(common Product, faction *string, itemType *string, itemNo *int64) (ProductView, error) {
	switch common.Type {
	case ProductTypeFactionSet:
		if faction == nil {
			return nil, fmt.Errorf("product %s: type=%s but faction missing", common.ProductID, common.Type)
		}
		return NewFactionSetProduct(common, *faction), nil
	case ProductTypeCosmetic:
		if itemType == nil || itemNo == nil {
			return nil, fmt.Errorf("product %s: type=%s but item_type/item_no missing", common.ProductID, common.Type)
		}
		return NewCosmeticProduct(common, *itemType, *itemNo), nil
	case ProductTypeSubscription:
		return NewSubscriptionProduct(common), nil
	default:
		return nil, fmt.Errorf("product %s: unknown type %q", common.ProductID, common.Type)
	}
}
