package pubsub_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-shop/internal/adapter/pubsub"
)

func TestNew(t *testing.T) {
	t.Run("[shopイベントのパブリッシュ]パブリッシャー生成時の入力検証", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("パブリッシャーを生成すると", func(t *testing.T) {
				t.Run("プロジェクトIDが空のとき、エラーの内容にプロジェクトID未指定が含まれる", func(t *testing.T) {
					_, err := pubsub.New(context.Background(), "", "test-card-pack-topic", "test-faction-topic", "test-premium-topic")

					assert.ErrorContains(t, err, "projectID is empty")
				})

				emptyTopicTests := []struct {
					name                   string
					cardPackPurchasedTopic string
					factionAcquiredTopic   string
					premiumUpdatedTopic    string
				}{
					{
						name:                   "event_typeがcard_pack_purchasedのトピック名が空のとき、エラーの内容にトピック名未指定が含まれる",
						cardPackPurchasedTopic: "",
						factionAcquiredTopic:   "test-faction-topic",
						premiumUpdatedTopic:    "test-premium-topic",
					},
					{
						name:                   "event_typeがfaction_acquiredのトピック名が空のとき、エラーの内容にトピック名未指定が含まれる",
						cardPackPurchasedTopic: "test-card-pack-topic",
						factionAcquiredTopic:   "",
						premiumUpdatedTopic:    "test-premium-topic",
					},
					{
						name:                   "event_typeがpremium_updatedのトピック名が空のとき、エラーの内容にトピック名未指定が含まれる",
						cardPackPurchasedTopic: "test-card-pack-topic",
						factionAcquiredTopic:   "test-faction-topic",
						premiumUpdatedTopic:    "",
					},
				}
				for _, tc := range emptyTopicTests {
					t.Run(tc.name, func(t *testing.T) {
						_, err := pubsub.New(context.Background(), "test-project", tc.cardPackPurchasedTopic, tc.factionAcquiredTopic, tc.premiumUpdatedTopic)

						assert.ErrorContains(t, err, "all topic names are required")
					})
				}

				noPublisherTests := []struct {
					name                   string
					projectID              string
					cardPackPurchasedTopic string
					factionAcquiredTopic   string
					premiumUpdatedTopic    string
				}{
					{
						name:                   "プロジェクトIDが空のとき、パブリッシャーは得られない",
						projectID:              "",
						cardPackPurchasedTopic: "test-card-pack-topic",
						factionAcquiredTopic:   "test-faction-topic",
						premiumUpdatedTopic:    "test-premium-topic",
					},
					{
						name:                   "event_typeがcard_pack_purchasedのトピック名が空のとき、パブリッシャーは得られない",
						projectID:              "test-project",
						cardPackPurchasedTopic: "",
						factionAcquiredTopic:   "test-faction-topic",
						premiumUpdatedTopic:    "test-premium-topic",
					},
					{
						name:                   "event_typeがfaction_acquiredのトピック名が空のとき、パブリッシャーは得られない",
						projectID:              "test-project",
						cardPackPurchasedTopic: "test-card-pack-topic",
						factionAcquiredTopic:   "",
						premiumUpdatedTopic:    "test-premium-topic",
					},
					{
						name:                   "event_typeがpremium_updatedのトピック名が空のとき、パブリッシャーは得られない",
						projectID:              "test-project",
						cardPackPurchasedTopic: "test-card-pack-topic",
						factionAcquiredTopic:   "test-faction-topic",
						premiumUpdatedTopic:    "",
					},
				}
				for _, tc := range noPublisherTests {
					t.Run(tc.name, func(t *testing.T) {
						got, _ := pubsub.New(context.Background(), tc.projectID, tc.cardPackPurchasedTopic, tc.factionAcquiredTopic, tc.premiumUpdatedTopic)

						assert.Nil(t, got)
					})
				}
			})
		})
	})
}
