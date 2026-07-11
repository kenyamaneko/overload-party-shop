package pubsub

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publisherTopics は test 内で New に渡す topic 名一式。
type publisherTopics struct {
	cardPackPurchased string
	factionAcquired   string
	premiumUpdated    string
}

// loadTopicsFromEnv は production と同じ env 経由で topic 名を解決する。
// 個別 case で空文字を強制したい時のみ struct を上書きする。
func loadTopicsFromEnv(t *testing.T) publisherTopics {
	t.Helper()
	t.Setenv("CARD_PACK_PURCHASED_TOPIC", "card-pack-purchased")
	t.Setenv("FACTION_ACQUIRED_TOPIC", "faction-acquired")
	t.Setenv("PREMIUM_UPDATED_TOPIC", "premium-updated")
	return publisherTopics{
		cardPackPurchased: os.Getenv("CARD_PACK_PURCHASED_TOPIC"),
		factionAcquired:   os.Getenv("FACTION_ACQUIRED_TOPIC"),
		premiumUpdated:    os.Getenv("PREMIUM_UPDATED_TOPIC"),
	}
}

func TestNew(t *testing.T) {
	t.Run("New の入力検証", func(t *testing.T) {
		// 入力検証は gpubsub.NewClient 呼び出し前に return する。
		tests := []struct {
			name      string
			projectID string
			override  func(topics *publisherTopics)
			wantSubs  string
		}{
			{
				name:      "projectID が空のとき、projectID is empty エラーになる",
				projectID: "",
				override:  func(*publisherTopics) {},
				wantSubs:  "projectID is empty",
			},
			{
				name:      "card-pack-purchased topic 名が空のとき、all topic names are required エラーになる",
				projectID: "test-project",
				override: func(topics *publisherTopics) {
					topics.cardPackPurchased = ""
				},
				wantSubs: "all topic names are required",
			},
			{
				name:      "faction-acquired topic 名が空のとき、all topic names are required エラーになる",
				projectID: "test-project",
				override: func(topics *publisherTopics) {
					topics.factionAcquired = ""
				},
				wantSubs: "all topic names are required",
			},
			{
				name:      "premium-updated topic 名が空のとき、all topic names are required エラーになる",
				projectID: "test-project",
				override: func(topics *publisherTopics) {
					topics.premiumUpdated = ""
				},
				wantSubs: "all topic names are required",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				topics := loadTopicsFromEnv(t)
				tt.override(&topics)
				p, err := New(context.Background(), tt.projectID, topics.cardPackPurchased, topics.factionAcquired, topics.premiumUpdated)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantSubs)
				assert.Nil(t, p)
			})
		}
	})
}

func TestPublish(t *testing.T) {
	t.Run("Publish", func(t *testing.T) {
		t.Run("未登録の event type のとき、unknown event type エラーになる", func(t *testing.T) {
			// 未登録 eventType は Pub/Sub SDK に届く前に弾く。
			p := &Publisher{}
			err := p.Publish(context.Background(), "unknown-event-type", []byte(`{}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown event type")
		})
	})
}
