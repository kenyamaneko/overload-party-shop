package pubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

// New の入力検証は gpubsub.NewClient 呼び出し前に return するため、
// emulator なしで単体テスト可能。実 publish 経路は integration test でカバーする。
func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name                  string
		projectID             string
		factionPurchasedTopic string
		premiumUpdatedTopic   string
		wantSubs              string
	}{
		{
			name:                  "projectID が空",
			projectID:             "",
			factionPurchasedTopic: domain.TopicFactionPurchased,
			premiumUpdatedTopic:   domain.TopicPremiumUpdated,
			wantSubs:              "projectID is empty",
		},
		{
			name:                  "faction-purchased topic 名が空",
			projectID:             "test-project",
			factionPurchasedTopic: "",
			premiumUpdatedTopic:   domain.TopicPremiumUpdated,
			wantSubs:              "both topic names are required",
		},
		{
			name:                  "premium-updated topic 名が空",
			projectID:             "test-project",
			factionPurchasedTopic: domain.TopicFactionPurchased,
			premiumUpdatedTopic:   "",
			wantSubs:              "both topic names are required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := New(context.Background(), tt.projectID, tt.factionPurchasedTopic, tt.premiumUpdatedTopic)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSubs)
			assert.Nil(t, p)
		})
	}
}

// Publish は未登録 eventType を先に弾く。outbox 行の eventType 設定ミスを
// Pub/Sub SDK に届く前に検出し、worker 側で failure_count を積ませるのが狙い。
// ゼロ値 Publisher (byEventType が nil の map) に対して呼び出しても、未登録判定は
// 通常の map ルックアップで ok=false を返すだけで到達可能。
func TestPublish_UnknownEventType(t *testing.T) {
	p := &Publisher{}
	err := p.Publish(context.Background(), "unknown-event-type", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}
