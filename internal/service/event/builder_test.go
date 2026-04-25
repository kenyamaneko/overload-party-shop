package event

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

func TestBuildFactionPurchased_Validation(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		playerID string
		faction  string
		wantErr  string
	}{
		{
			name:     "topic 空",
			topic:    "",
			playerID: "player-1",
			faction:  "Tenki",
			wantErr:  "topic is empty",
		},
		{
			name:     "playerID 空",
			topic:    apishop.TopicFactionPurchased,
			playerID: "",
			faction:  "Tenki",
			wantErr:  "playerID is empty",
		},
		{
			name:     "faction 空",
			topic:    apishop.TopicFactionPurchased,
			playerID: "player-1",
			faction:  "",
			wantErr:  "faction is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildFactionPurchased(tt.topic, tt.playerID, tt.faction)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// 正常系: 指定 topic を持ち、payload は apishop.FactionPurchasedEvent の shape に従う。
// payload 内 eventId は OutboxEvent.EventID と一致 (subscriber の冪等キー)。
func TestBuildFactionPurchased_Success(t *testing.T) {
	ev, err := BuildFactionPurchased(apishop.TopicFactionPurchased, "player-1", "Tenki")
	require.NoError(t, err)

	assert.Equal(t, apishop.TopicFactionPurchased, ev.Topic)
	assert.NotEqual(t, "", ev.EventID.String())

	var decoded apishop.FactionPurchasedEvent
	require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
	assert.Equal(t, apishop.EventTypeFactionPurchased, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload 内 eventId は outbox 行の PK と一致する")
	assert.Equal(t, "player-1", decoded.PlayerID)
	assert.Equal(t, "Tenki", decoded.Faction)
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
}

// 同一入力でも EventID は毎回異なる (subscriber dedupe の前提)。
func TestBuildFactionPurchased_EventIDUnique(t *testing.T) {
	ev1, err := BuildFactionPurchased(apishop.TopicFactionPurchased, "player-1", "Tenki")
	require.NoError(t, err)
	ev2, err := BuildFactionPurchased(apishop.TopicFactionPurchased, "player-1", "Tenki")
	require.NoError(t, err)
	assert.NotEqual(t, ev1.EventID, ev2.EventID, "同一入力でも event_id は毎回異なる")
}

func TestBuildPremiumUpdated_Validation(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		playerID string
		wantErr  string
	}{
		{
			name:     "topic 空",
			topic:    "",
			playerID: "player-1",
			wantErr:  "topic is empty",
		},
		{
			name:     "playerID 空",
			topic:    apishop.TopicPremiumUpdated,
			playerID: "",
			wantErr:  "playerID is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildPremiumUpdated(tt.topic, tt.playerID, true, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// 正常系: is_premium と expiresAt の組み合わせがそのまま payload に乗る。
// Source は shop 固定。
func TestBuildPremiumUpdated_Success(t *testing.T) {
	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC()

	tests := []struct {
		name      string
		isPremium bool
		expiresAt *time.Time
	}{
		{
			name:      "is_premium=true + expires_at 付き",
			isPremium: true,
			expiresAt: &expiresAt,
		},
		{
			name:      "is_premium=false かつ expires_at=nil",
			isPremium: false,
			expiresAt: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := BuildPremiumUpdated(apishop.TopicPremiumUpdated, "player-1", tt.isPremium, tt.expiresAt)
			require.NoError(t, err)

			assert.Equal(t, apishop.TopicPremiumUpdated, ev.Topic)

			var decoded apishop.PremiumUpdatedEvent
			require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
			assert.Equal(t, apishop.EventTypePremiumUpdated, decoded.EventType)
			assert.Equal(t, ev.EventID.String(), decoded.EventID)
			assert.Equal(t, "player-1", decoded.PlayerID)
			assert.Equal(t, tt.isPremium, decoded.IsPremium)
			assert.Equal(t, apishop.PremiumUpdatedSourceShop, decoded.Source)
			assert.Equal(t, tt.expiresAt == nil, decoded.PremiumExpiresAt == nil)
		})
	}
}
