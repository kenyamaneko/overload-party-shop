package apishop

import "time"

// premium 状態変化イベントの topic 名。
const TopicPremiumUpdated = "premium-updated"

// PremiumUpdatedEvent の EventType 値。
const EventTypePremiumUpdated = "premium_updated"

// PremiumUpdatedSourceShop is the canonical Source value emitted by shop.
// Source は shop 単独で "shop" 固定だが、将来 admin-override 等の別 publisher が
// 追加された場合に備えて schema に残す (ADR-022 で common から移動した際に維持)。
const PremiumUpdatedSourceShop = "shop"

// PremiumUpdatedEvent is published by shop when a subscription state change
// results in a new is_premium value for a player. Subscribers:
//
//   - account: UPDATEs account.players.is_premium + account.players.premium_expires_at.
//   - gateway: pushes a WS `premium_update_complete` message.
//
// Idempotency: subscribers MUST dedupe on EventID via their processed_events
// table. See ADR-022 for the decomposition rationale.
type PremiumUpdatedEvent struct {
	EventType        string     `json:"event_type"`
	EventID          string     `json:"event_id"`
	Timestamp        time.Time  `json:"timestamp"`
	PlayerID         string     `json:"player_id"`
	IsPremium        bool       `json:"is_premium"`
	PremiumExpiresAt *time.Time `json:"premium_expires_at,omitempty"`
	Source           string     `json:"source"`
}
