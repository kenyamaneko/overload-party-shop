package apishopfake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// 本ファイルは shop が publish する topic (ADR-022 で定義) に対する type-safe
// wrapper を提供する。低レベル API (Broker / Publisher / Subscriber) の上に薄く
// 被せ、consumer 側テストが json encode / decode / event_type / event_id /
// timestamp の定型を書かずに済むようにする。
//
// 新しい topic が api-shop に追加された時は本ファイルに wrapper を追加する
// (publish ヘルパ + Expecter の 2 つセット)。

// PublishFactionPurchased は shop publisher の role を演じて
// TopicFactionPurchased へ FactionPurchasedEvent を 1 件発行する。
// EventID / Timestamp が未設定なら UUIDv4 / 現在時刻を自動付与し、EventType は
// 常に EventTypeFactionPurchased に固定する — テスト側で手書きする必要があるのは
// PlayerID / Faction など検証対象のフィールドのみ。
func PublishFactionPurchased(ctx context.Context, p *Publisher, ev apishop.FactionPurchasedEvent) error {
	ev = fillFactionDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal FactionPurchasedEvent: %w", err)
	}
	return p.Publish(ctx, apishop.TopicFactionPurchased, data)
}

// PublishPremiumUpdated は shop publisher の role を演じて TopicPremiumUpdated へ
// PremiumUpdatedEvent を 1 件発行する。デフォルト補完は PublishFactionPurchased
// と同様で、加えて Source が空なら PremiumUpdatedSourceShop を埋める。
func PublishPremiumUpdated(ctx context.Context, p *Publisher, ev apishop.PremiumUpdatedEvent) error {
	ev = fillPremiumDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal PremiumUpdatedEvent: %w", err)
	}
	return p.Publish(ctx, apishop.TopicPremiumUpdated, data)
}

// FactionPurchasedExpecter は TopicFactionPurchased に subscribe 済みの待受器。
// ExpectFactionPurchased で subscribe を確定 → publish → Wait で型付き payload
// を受け取る順序を API レベルで強制することで、Broker が新規 subscriber に過去
// メッセージを配信しない制約 (実 Pub/Sub の subscription 新規作成挙動に揃える
// 意図) を破らない構造にしている。
type FactionPurchasedExpecter struct {
	ch <-chan []byte
}

// ExpectFactionPurchased は TopicFactionPurchased に即時 subscribe し、
// Wait 可能な Expecter を返す。publish より前に呼び出す必要がある。
func ExpectFactionPurchased(s *Subscriber) *FactionPurchasedExpecter {
	return &FactionPurchasedExpecter{ch: s.Messages(apishop.TopicFactionPurchased)}
}

// Wait は Expecter が subscribe 開始した以降に publish された最初の
// FactionPurchasedEvent を timeout 付きで取り出す。timeout 超過や
// payload デコード失敗は error で返し、zero 値 + error の契約とする。
func (e *FactionPurchasedExpecter) Wait(timeout time.Duration) (apishop.FactionPurchasedEvent, error) {
	return waitTypedFromChan[apishop.FactionPurchasedEvent](e.ch, apishop.TopicFactionPurchased, timeout)
}

// PremiumUpdatedExpecter は TopicPremiumUpdated 版の Expecter。
type PremiumUpdatedExpecter struct {
	ch <-chan []byte
}

// ExpectPremiumUpdated は TopicPremiumUpdated に即時 subscribe し Expecter を返す。
func ExpectPremiumUpdated(s *Subscriber) *PremiumUpdatedExpecter {
	return &PremiumUpdatedExpecter{ch: s.Messages(apishop.TopicPremiumUpdated)}
}

// Wait は publish された最初の PremiumUpdatedEvent を timeout 付きで取り出す。
func (e *PremiumUpdatedExpecter) Wait(timeout time.Duration) (apishop.PremiumUpdatedEvent, error) {
	return waitTypedFromChan[apishop.PremiumUpdatedEvent](e.ch, apishop.TopicPremiumUpdated, timeout)
}

// waitTypedFromChan は payload bytes を timeout 付きで受信し型 T にデコードする。
// 受信・timeout・json.Unmarshal の 3 責務を Expecter 実装に共通化するための helper。
func waitTypedFromChan[T any](ch <-chan []byte, topic string, timeout time.Duration) (T, error) {
	var zero T
	select {
	case data, ok := <-ch:
		if !ok {
			return zero, fmt.Errorf("channel closed for topic %q before receiving message", topic)
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return zero, fmt.Errorf("unmarshal %q payload: %w", topic, err)
		}
		return v, nil
	case <-time.After(timeout):
		return zero, fmt.Errorf("timeout waiting for %q after %s", topic, timeout)
	}
}

// fillFactionDefaults は FactionPurchasedEvent の定型フィールドを補完する。
// EventType は契約固定のため既存値に関わらず上書きし、EventID / Timestamp は
// caller が事前に意図的にセットした値があればそれを尊重する。
func fillFactionDefaults(ev apishop.FactionPurchasedEvent) apishop.FactionPurchasedEvent {
	ev.EventType = apishop.EventTypeFactionPurchased
	if ev.EventID == "" {
		ev.EventID = newEventID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	return ev
}

// fillPremiumDefaults は PremiumUpdatedEvent 版の fillFactionDefaults。
// Source は shop 単独 publish のため PremiumUpdatedSourceShop を既定値とする。
func fillPremiumDefaults(ev apishop.PremiumUpdatedEvent) apishop.PremiumUpdatedEvent {
	ev.EventType = apishop.EventTypePremiumUpdated
	if ev.EventID == "" {
		ev.EventID = newEventID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	if ev.Source == "" {
		ev.Source = apishop.PremiumUpdatedSourceShop
	}
	return ev
}
