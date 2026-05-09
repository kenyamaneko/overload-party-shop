package domain

import "time"

// 本ファイルは shop が発行する Pub/Sub イベントの domain 表現を定義する。
// wire 表現は packages/api-shop/event.go に存在し、両者は presenter 層で詰め替える。
// JSON 形状の一致は前提としない (ADR-034)。

// CardPackPurchasedEvent はプレイヤーが card_pack を含む商品を購入した業務事実。
// subscriber: card / gateway。
type CardPackPurchasedEvent struct {
	EventID    string
	Timestamp  time.Time
	PlayerID   string
	CardPackID string
}

// FactionAcquiredEvent はプレイヤーが faction を獲得した業務事実。
// subscriber: account / gateway。
type FactionAcquiredEvent struct {
	EventID   string
	Timestamp time.Time
	PlayerID  string
	Faction   string
}

// PremiumUpdatedEvent は subscription 状態遷移により is_premium が変化した業務事実。
// subscriber: account / gateway。Source は発行サービス名 (現状 shop のみ)。
type PremiumUpdatedEvent struct {
	EventID          string
	Timestamp        time.Time
	PlayerID         string
	IsPremium        bool
	PremiumExpiresAt *time.Time
	Source           string
}
