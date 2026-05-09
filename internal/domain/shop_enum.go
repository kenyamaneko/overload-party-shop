package domain

// 商品種別の discriminator。同名の値は wire 層 (apishop) でも公開している (本ファイルとは独立した SSoT として、AsyncAPI/OpenAPI 由来)。
const (
	ProductTypeFactionSet   = "faction_set"
	ProductTypeCardPack     = "card_pack"
	ProductTypeCosmetic     = "cosmetic"
	ProductTypeSubscription = "subscription"
)

// クライアント実行プラットフォーム。
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
)

// 装飾アイテム (cosmetic) の種別。
const (
	ItemTypeStamp   = "stamp"
	ItemTypePlaymat = "playmat"
	ItemTypeSleeve  = "sleeve"
	ItemTypeIcon    = "icon"
)

// サブスクリプションの状態機械。domain 内部完結 (wire には流出しない)。
const (
	SubscriptionStatusActive      = "active"
	SubscriptionStatusExpired     = "expired"
	SubscriptionStatusCancelled   = "cancelled"
	SubscriptionStatusGracePeriod = "grace_period"
	SubscriptionStatusRevoked     = "revoked"
)
