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

// Publish は未登録 topic を先に弾く。outbox 行の topic 設定ミスを Pub/Sub SDK に
// 届く前に検出し、worker 側で failure_count を積ませるのが狙い。
// ゼロ値 Publisher (byTopic が nil の map) に対して呼び出しても、未登録 topic 判定は
// 通常の map ルックアップで ok=false を返すだけで到達可能。
func TestPublish_UnknownTopic(t *testing.T) {
	p := &Publisher{}
	err := p.Publish(context.Background(), "unknown-topic", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown topic")
}
