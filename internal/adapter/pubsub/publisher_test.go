package pubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// New の入力検証は gpubsub.NewClient 呼び出し前に return するため、
// emulator なしで単体テスト可能。実 publish 経路は integration test でカバーする。
func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name                 string
		projectID            string
		factionSelectedTopic string
		premiumUpdatedTopic  string
		wantSubs             string
	}{
		{
			name:                 "projectID が空",
			projectID:            "",
			factionSelectedTopic: "faction-selected",
			premiumUpdatedTopic:  "premium-updated",
			wantSubs:             "projectID is empty",
		},
		{
			name:                 "faction-selected topic 名が空",
			projectID:            "test-project",
			factionSelectedTopic: "",
			premiumUpdatedTopic:  "premium-updated",
			wantSubs:             "both topic names are required",
		},
		{
			name:                 "premium-updated topic 名が空",
			projectID:            "test-project",
			factionSelectedTopic: "faction-selected",
			premiumUpdatedTopic:  "",
			wantSubs:             "both topic names are required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := New(context.Background(), tt.projectID, tt.factionSelectedTopic, tt.premiumUpdatedTopic)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSubs)
			assert.Nil(t, p)
		})
	}
}

// Publish* の入力検証は SDK 到達前に return するので、zero value の Publisher
// に対して呼び出しても検証経路だけは実行可能。
func TestPublishFactionSelected_Validation(t *testing.T) {
	tests := []struct {
		name     string
		playerID string
		faction  string
		wantSubs string
	}{
		{
			name:     "playerID が空",
			playerID: "",
			faction:  "Tenki",
			wantSubs: "playerID is empty",
		},
		{
			name:     "faction が空",
			playerID: "player-1",
			faction:  "",
			wantSubs: "faction is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Publisher{}
			err := p.PublishFactionSelected(context.Background(), tt.playerID, tt.faction)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSubs)
		})
	}
}

func TestPublishPremiumUpdated_Validation(t *testing.T) {
	p := &Publisher{}
	err := p.PublishPremiumUpdated(context.Background(), "", true, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "playerID is empty")
}
