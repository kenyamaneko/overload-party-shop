package apishop

import "time"

// 本ファイルは data/asyncapi.yaml を SSoT として手書きで保守する。
// 仕様変更時は asyncapi.yaml を先に更新し、本ファイルを同期させる。
// breaking change は CI の `asyncapi diff` ジョブで検知される。

// EventType の取り得る値。AsyncAPI の各 message payload で `event_type` フィールドの const として定義されている。
const (
	EventTypeCardPackPurchased = "card_pack_purchased"
	EventTypeFactionAcquired   = "faction_acquired"
	EventTypePremiumUpdated    = "premium_updated"
)

// PremiumUpdated.source の取り得る値。AsyncAPI の enum 定義に対応する。
const (
	PremiumUpdatedSourceShop = "shop"
)

// CardPackPurchasedEvent はプレイヤーが card_pack を含む商品 (faction_set / card_pack 等) を購入した際に shop が発行するイベント payload。
// subscriber: card (GrantPack(card_pack_id) でカード配布) / gateway (WS 副次通知)。
// subscriber は EventID で冪等性を担保する (at-least-once)。
type CardPackPurchasedEvent struct {
	EventType  string    `json:"event_type"`
	EventID    string    `json:"event_id"`
	Timestamp  time.Time `json:"timestamp"`
	PlayerID   string    `json:"player_id"`
	CardPackID string    `json:"card_pack_id"`
}

// FactionAcquiredEvent はプレイヤーが faction を獲得した業務事実を表す shop 発行イベント payload。
// subscriber: account (player_factions に INSERT、authoritative 所有権) / gateway (WS 一次通知)。
// subscriber は EventID で冪等性を担保する (at-least-once)。
type FactionAcquiredEvent struct {
	EventType string    `json:"event_type"`
	EventID   string    `json:"event_id"`
	Timestamp time.Time `json:"timestamp"`
	PlayerID  string    `json:"player_id"`
	Faction   string    `json:"faction"`
}

// PremiumUpdatedEvent は subscription 状態遷移により is_premium が変化した際に shop が発行するイベント payload。
// subscriber: account (players.is_premium / premium_expires_at を更新) / gateway (WS 通知)。
// subscriber は EventID で冪等性を担保する (at-least-once)。
type PremiumUpdatedEvent struct {
	EventType        string     `json:"event_type"`
	EventID          string     `json:"event_id"`
	Timestamp        time.Time  `json:"timestamp"`
	PlayerID         string     `json:"player_id"`
	IsPremium        bool       `json:"is_premium"`
	PremiumExpiresAt *time.Time `json:"premium_expires_at,omitempty"`
	Source           string     `json:"source"`
}
