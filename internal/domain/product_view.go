package domain

import "fmt"

// ProductView は per-type 商品 (FactionSetProduct / CardPackProduct / CosmeticProduct / SubscriptionProduct)
// を束ねる sealed interface。usecase は ProductView を型 switch して type 固有属性へアクセスする。
type ProductView interface {
	// Common は商品の type 横断共通属性を返す。
	Common() Product
	isProductView()
}

func (p FactionSetProduct) Common() Product { return p.Product }
func (FactionSetProduct) isProductView()    {}

func (p CardPackProduct) Common() Product { return p.Product }
func (CardPackProduct) isProductView()    {}

func (p CosmeticProduct) Common() Product { return p.Product }
func (CosmeticProduct) isProductView()    {}

func (p SubscriptionProduct) Common() Product { return p.Product }
func (SubscriptionProduct) isProductView()    {}

// NewFactionSetProduct は card_pack 参照と faction を持つ FactionSetProduct を構築する。
// faction_set 商品は購入時に card_pack を配布する (card 側で GrantPack) と同時に
// faction 所有権を付与する (account 側で player_factions INSERT) ので両方の情報が必要。
func NewFactionSetProduct(common Product, cardPackID, faction string) FactionSetProduct {
	return FactionSetProduct{Product: common, CardPackID: cardPackID, Faction: faction}
}

// NewCardPackProduct は card_pack 参照を持つ CardPackProduct を構築する。
func NewCardPackProduct(common Product, cardPackID string) CardPackProduct {
	return CardPackProduct{Product: common, CardPackID: cardPackID}
}

// NewCosmeticProduct は有効な (item_type, item_no) を持つ CosmeticProduct を構築する。
func NewCosmeticProduct(common Product, itemType string, itemNo int64) CosmeticProduct {
	return CosmeticProduct{Product: common, ItemType: itemType, ItemNo: itemNo}
}

// NewSubscriptionProduct は課金周期を持つ SubscriptionProduct を構築する。
func NewSubscriptionProduct(common Product, periodMonths int64) SubscriptionProduct {
	return SubscriptionProduct{Product: common, PeriodMonths: periodMonths}
}

// NewProductView は type discriminator と optional な type 固有入力から ProductView を組み立てる dispatcher。
// repository が副表 LEFT JOIN で取得した nullable 列をそのまま渡せるよう optional pointer で受ける。
// type と入力値の整合は本関数が担保する (例: type=faction_set だが faction=nil は error)。
func NewProductView(common Product, cardPackID *string, faction *string, itemType *string, itemNo *int64, periodMonths *int64) (ProductView, error) {
	switch common.Type {
	case ProductTypeFactionSet:
		if cardPackID == nil {
			return nil, fmt.Errorf("product %s: type=%s but card_pack_id missing", common.ProductID, common.Type)
		}
		if faction == nil {
			return nil, fmt.Errorf("product %s: type=%s but faction missing", common.ProductID, common.Type)
		}
		return NewFactionSetProduct(common, *cardPackID, *faction), nil
	case ProductTypeCardPack:
		if cardPackID == nil {
			return nil, fmt.Errorf("product %s: type=%s but card_pack_id missing", common.ProductID, common.Type)
		}
		return NewCardPackProduct(common, *cardPackID), nil
	case ProductTypeCosmetic:
		if itemType == nil || itemNo == nil {
			return nil, fmt.Errorf("product %s: type=%s but item_type/item_no missing", common.ProductID, common.Type)
		}
		return NewCosmeticProduct(common, *itemType, *itemNo), nil
	case ProductTypeSubscription:
		if periodMonths == nil {
			return nil, fmt.Errorf("product %s: type=%s but period_months missing", common.ProductID, common.Type)
		}
		return NewSubscriptionProduct(common, *periodMonths), nil
	default:
		return nil, fmt.Errorf("product %s: unknown type %q", common.ProductID, common.Type)
	}
}
