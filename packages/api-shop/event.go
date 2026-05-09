package apishop

import "time"

const (
	EventTypeCardPackPurchased = "card_pack_purchased"
	EventTypeFactionAcquired   = "faction_acquired"
	EventTypePremiumUpdated    = "premium_updated"
)

const (
	PremiumUpdatedSourceShop = "shop"
)

// CardPackPurchasedEvent はプレイヤーが card_pack を含む商品を購入した際に shop が発行するイベント payload。
// subscriber: card / gateway。subscriber は EventID で冪等性を担保する (at-least-once)。
type CardPackPurchasedEvent struct {
	EventType  string    `json:"event_type"`
	EventID    string    `json:"event_id"`
	Timestamp  time.Time `json:"timestamp"`
	PlayerID   string    `json:"player_id"`
	CardPackID string    `json:"card_pack_id"`
}

// FactionAcquiredEvent はプレイヤーが faction を獲得した業務事実を表す shop 発行イベント payload。
// subscriber: account / gateway。subscriber は EventID で冪等性を担保する (at-least-once)。
type FactionAcquiredEvent struct {
	EventType string    `json:"event_type"`
	EventID   string    `json:"event_id"`
	Timestamp time.Time `json:"timestamp"`
	PlayerID  string    `json:"player_id"`
	Faction   string    `json:"faction"`
}

// PremiumUpdatedEvent は subscription 状態遷移により is_premium が変化した際に shop が発行するイベント payload。
// subscriber: account / gateway。subscriber は EventID で冪等性を担保する (at-least-once)。
type PremiumUpdatedEvent struct {
	EventType        string     `json:"event_type"`
	EventID          string     `json:"event_id"`
	Timestamp        time.Time  `json:"timestamp"`
	PlayerID         string     `json:"player_id"`
	IsPremium        bool       `json:"is_premium"`
	PremiumExpiresAt *time.Time `json:"premium_expires_at,omitempty"`
	Source           string     `json:"source"`
}
