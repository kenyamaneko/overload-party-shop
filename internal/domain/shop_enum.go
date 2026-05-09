package domain

// ProductType は商品種別の discriminator。本パッケージが SSoT。
const (
	ProductTypeFactionSet   = "faction_set"
	ProductTypeCardPack     = "card_pack"
	ProductTypeCosmetic     = "cosmetic"
	ProductTypeSubscription = "subscription"
)

// Platform はクライアント実行プラットフォーム。本パッケージが SSoT。
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
)

// ItemType は装飾アイテム (cosmetic) の種別。本パッケージが SSoT。
const (
	ItemTypeStamp   = "stamp"
	ItemTypePlaymat = "playmat"
	ItemTypeSleeve  = "sleeve"
	ItemTypeIcon    = "icon"
)

// SubscriptionStatus はサブスクリプションの状態機械。domain 内部完結 (wire には流出しない)。
const (
	SubscriptionStatusActive      = "active"
	SubscriptionStatusExpired     = "expired"
	SubscriptionStatusCancelled   = "cancelled"
	SubscriptionStatusGracePeriod = "grace_period"
	SubscriptionStatusRevoked     = "revoked"
)
