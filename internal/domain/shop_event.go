package domain

import "time"

// CardPackPurchasedEvent はプレイヤーが card_pack を含む商品を購入した業務事実。
// subscriber: card。
type CardPackPurchasedEvent struct {
	EventID    string
	Timestamp  time.Time
	PlayerID   string
	CardPackID string
}

// FactionAcquiredEvent はプレイヤーが faction を獲得した業務事実。
// subscriber: account。
type FactionAcquiredEvent struct {
	EventID   string
	Timestamp time.Time
	PlayerID  string
	Faction   string
}

// PremiumUpdatedEvent は subscription 状態遷移により is_premium が変化した業務事実。
// subscriber: account。Source は発行サービス名 (現状 shop のみ)。
type PremiumUpdatedEvent struct {
	EventID          string
	Timestamp        time.Time
	PlayerID         string
	IsPremium        bool
	PremiumExpiresAt *time.Time
	Source           string
}
