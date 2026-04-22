package apishopfake_test

import (
	"context"
	"testing"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
	"github.com/kenyamaneko/overload-party-shop/packages/api-shop/apishopfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Expect → Publish → Wait の round-trip: shop 送信側の typed publish と
// consumer 側の typed 受信が矛盾なく繋がる基本契約。consumer サービス
// (account / card / gateway) が subscribe テストを書く際の最小使用例でもある。
func TestFactionPurchased_ExpectPublishWaitRoundTrip(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)
	ctx := context.Background()

	exp := apishopfake.ExpectFactionPurchased(sub)

	require.NoError(t, apishopfake.PublishFactionPurchased(ctx, pub, apishop.FactionPurchasedEvent{
		PlayerID: "player-1",
		Faction:  "Tenki",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "player-1", got.PlayerID)
	assert.Equal(t, "Tenki", got.Faction)
}

// PublishFactionPurchased は、caller が省略したフィールドに契約上のデフォルトを
// 埋める。テスト側は検証したい PlayerID / Faction のみ書けばよい、という
// ergonomic の契約。
func TestFactionPurchased_PublishFillsDefaults(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)
	ctx := context.Background()

	before := time.Now().UTC()
	exp := apishopfake.ExpectFactionPurchased(sub)

	require.NoError(t, apishopfake.PublishFactionPurchased(ctx, pub, apishop.FactionPurchasedEvent{
		PlayerID: "p", Faction: "SHE",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, apishop.EventTypeFactionPurchased, got.EventType, "EventType は契約で固定")
	assert.NotEmpty(t, got.EventID, "EventID は未指定なら自動生成される")
	assert.False(t, got.Timestamp.Before(before), "Timestamp は未指定なら現在時刻以降")
}

// Expect で subscribe していない状態 (publish が先) では Wait が message を
// 拾わず timeout error を返す。consumer 側テストが「subscribe を忘れた」
// ケースを fake 側で黙って成功させない契約を fix する。
func TestFactionPurchased_WaitTimesOutWhenPublishedBeforeExpect(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)
	ctx := context.Background()

	require.NoError(t, apishopfake.PublishFactionPurchased(ctx, pub, apishop.FactionPurchasedEvent{
		PlayerID: "p", Faction: "Tenki",
	}))

	exp := apishopfake.ExpectFactionPurchased(sub)
	_, err := exp.Wait(50 * time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// PremiumUpdated 側も round-trip + defaults + Source 補完が効くことを確認する。
// faction 側と独立した topic/type のため、1 方が壊れてもう 1 方が動くケースを
// 生まないようそれぞれでテストを持つ。
func TestPremiumUpdated_ExpectPublishWaitRoundTrip(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	sub := apishopfake.NewSubscriber(broker)
	ctx := context.Background()

	exp := apishopfake.ExpectPremiumUpdated(sub)

	expiry := time.Now().Add(30 * 24 * time.Hour).UTC()
	require.NoError(t, apishopfake.PublishPremiumUpdated(ctx, pub, apishop.PremiumUpdatedEvent{
		PlayerID:         "player-2",
		IsPremium:        true,
		PremiumExpiresAt: &expiry,
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "player-2", got.PlayerID)
	assert.True(t, got.IsPremium)
	require.NotNil(t, got.PremiumExpiresAt)
	assert.True(t, got.PremiumExpiresAt.Equal(expiry))
	assert.Equal(t, apishop.EventTypePremiumUpdated, got.EventType)
	assert.Equal(t, apishop.PremiumUpdatedSourceShop, got.Source, "Source は未指定なら shop 固定")
	assert.NotEmpty(t, got.EventID)
}

// Publisher.Published() は typed helper 経由の publish でも同じ記録 API を提供する。
// 送信側サービスが「TopicFactionPurchased に正しく発行されたか」を低レベル API
// からでも確認できることで、typed helper の追加で観測性が落ちないことを固定する。
func TestTyped_PublishedRecordsTopicAndPayload(t *testing.T) {
	broker := apishopfake.NewBroker()
	pub := apishopfake.NewPublisher(broker)
	ctx := context.Background()

	require.NoError(t, apishopfake.PublishFactionPurchased(ctx, pub, apishop.FactionPurchasedEvent{
		PlayerID: "p", Faction: "Sugar",
	}))
	require.NoError(t, apishopfake.PublishPremiumUpdated(ctx, pub, apishop.PremiumUpdatedEvent{
		PlayerID: "p", IsPremium: false,
	}))

	history := pub.Published()
	require.Len(t, history, 2)
	assert.Equal(t, apishop.TopicFactionPurchased, history[0].Topic)
	assert.Equal(t, apishop.TopicPremiumUpdated, history[1].Topic)
	// payload は json bytes。decode テストは round-trip 系で既に担保済みのため
	// ここでは topic 配線が意図通りであることまで固定する。
	assert.NotEmpty(t, history[0].Data)
	assert.NotEmpty(t, history[1].Data)
}
