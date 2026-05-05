package domain

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
