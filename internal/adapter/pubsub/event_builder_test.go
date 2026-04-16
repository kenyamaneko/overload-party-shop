package pubsub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pubsubevents "github.com/kenyamaneko/overload-party-common/packages/pubsub-events"
)

func TestNewEventBuilder_Validation(t *testing.T) {
	tests := []struct {
		name                 string
		factionSelectedTopic string
		premiumUpdatedTopic  string
		wantErr              bool
	}{
		{
			name:                 "両方空ならエラー",
			factionSelectedTopic: "",
			premiumUpdatedTopic:  "",
			wantErr:              true,
		},
		{
			name:                 "faction のみ空ならエラー",
			factionSelectedTopic: "",
			premiumUpdatedTopic:  "premium-updated",
			wantErr:              true,
		},
		{
			name:                 "premium のみ空ならエラー",
			factionSelectedTopic: "faction-selected",
			premiumUpdatedTopic:  "",
			wantErr:              true,
		},
		{
			name:                 "両方埋まっていれば成功",
			factionSelectedTopic: "faction-selected",
			premiumUpdatedTopic:  "premium-updated",
			wantErr:              false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewEventBuilder(tt.factionSelectedTopic, tt.premiumUpdatedTopic)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, b)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, b)
			}
		})
	}
}

// BuildFactionSelected は playerID/faction を検証した上で、
// 設定された faction topic と pubsubevents.FactionSelectedEvent の shape に合う
// JSON payload を持つ OutboxEvent を返す。event_id は UUID で payload 内 eventId と一致する。
func TestBuildFactionSelected(t *testing.T) {
	b, err := NewEventBuilder("faction-selected", "premium-updated")
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		ev, err := b.BuildFactionSelected("player-1", "Tenki")
		require.NoError(t, err)

		assert.Equal(t, "faction-selected", ev.Topic)
		assert.NotEqual(t, "", ev.EventID.String())

		var decoded pubsubevents.FactionSelectedEvent
		require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
		assert.Equal(t, pubsubevents.EventTypeFactionSelected, decoded.EventType)
		assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload 内 eventId は outbox 行の PK と一致する")
		assert.Equal(t, "player-1", decoded.PlayerID)
		assert.Equal(t, "Tenki", decoded.Faction)
		assert.Equal(t, pubsubevents.FactionSourceShopPurchase, decoded.Source)
		assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	})

	t.Run("playerID が空ならエラー", func(t *testing.T) {
		_, err := b.BuildFactionSelected("", "Tenki")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "playerID is empty")
	})

	t.Run("faction が空ならエラー", func(t *testing.T) {
		_, err := b.BuildFactionSelected("player-1", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "faction is empty")
	})
}

func TestBuildPremiumUpdated(t *testing.T) {
	b, err := NewEventBuilder("faction-selected", "premium-updated")
	require.NoError(t, err)

	t.Run("is_premium=true + expires_at 付き", func(t *testing.T) {
		expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC()
		ev, err := b.BuildPremiumUpdated("player-1", true, &expiresAt)
		require.NoError(t, err)

		assert.Equal(t, "premium-updated", ev.Topic)

		var decoded pubsubevents.PremiumUpdatedEvent
		require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
		assert.Equal(t, pubsubevents.EventTypePremiumUpdated, decoded.EventType)
		assert.Equal(t, ev.EventID.String(), decoded.EventID)
		assert.Equal(t, "player-1", decoded.PlayerID)
		assert.True(t, decoded.IsPremium)
		require.NotNil(t, decoded.PremiumExpiresAt)
		assert.WithinDuration(t, expiresAt, *decoded.PremiumExpiresAt, time.Second)
	})

	t.Run("is_premium=false では expires_at が nil になる", func(t *testing.T) {
		ev, err := b.BuildPremiumUpdated("player-2", false, nil)
		require.NoError(t, err)

		var decoded pubsubevents.PremiumUpdatedEvent
		require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
		assert.False(t, decoded.IsPremium)
		assert.Nil(t, decoded.PremiumExpiresAt)
	})

	t.Run("playerID が空ならエラー", func(t *testing.T) {
		_, err := b.BuildPremiumUpdated("", true, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "playerID is empty")
	})
}

// event_id は毎回異なる UUID。subscriber 側の dedupe が正しく機能する前提 (同じ
// payload でも id が違えば新規イベントとして扱える) の素材になる。
func TestBuildFactionSelected_EventIDUnique(t *testing.T) {
	b, err := NewEventBuilder("faction-selected", "premium-updated")
	require.NoError(t, err)

	ev1, err := b.BuildFactionSelected("player-1", "Tenki")
	require.NoError(t, err)
	ev2, err := b.BuildFactionSelected("player-1", "Tenki")
	require.NoError(t, err)
	assert.NotEqual(t, ev1.EventID, ev2.EventID, "同一入力でも event_id は毎回異なる")
}
