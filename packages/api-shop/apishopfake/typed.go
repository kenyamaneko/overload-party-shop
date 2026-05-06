package apishopfake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// PublishFactionAcquired は TopicFactionAcquired へ FactionAcquiredEvent を 1 件発行する。
// EventID / Timestamp 未設定なら UUIDv4 / 現在時刻を補完、EventType は常に上書きする。
func PublishFactionAcquired(ctx context.Context, p *Publisher, ev apishop.FactionAcquiredEvent) error {
	ev = fillFactionAcquiredDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal FactionAcquiredEvent: %w", err)
	}
	return p.Publish(ctx, apishop.TopicFactionAcquired, data)
}

// PublishCardPackPurchased は TopicCardPackPurchased へ CardPackPurchasedEvent を 1 件発行する。
// EventID / Timestamp 未設定なら UUIDv4 / 現在時刻を補完、EventType は常に上書きする。
func PublishCardPackPurchased(ctx context.Context, p *Publisher, ev apishop.CardPackPurchasedEvent) error {
	ev = fillCardPackPurchasedDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal CardPackPurchasedEvent: %w", err)
	}
	return p.Publish(ctx, apishop.TopicCardPackPurchased, data)
}

// PublishPremiumUpdated は TopicPremiumUpdated へ PremiumUpdatedEvent を 1 件発行する。
// 他 Publish ヘルパと同じ補完に加え、Source 空なら PremiumUpdatedSourceShop を埋める。
func PublishPremiumUpdated(ctx context.Context, p *Publisher, ev apishop.PremiumUpdatedEvent) error {
	ev = fillPremiumDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal PremiumUpdatedEvent: %w", err)
	}
	return p.Publish(ctx, apishop.TopicPremiumUpdated, data)
}

// FactionAcquiredExpecter は TopicFactionAcquired に subscribe 済みの待受器。
// publish より前に subscribe を確定する必要があるため (Broker は過去メッセージを配信しない)、
// API は ExpectFactionAcquired → publish → Wait の順序を強制する形に分割している。
type FactionAcquiredExpecter struct {
	ch <-chan []byte
}

// ExpectFactionAcquired は即時 subscribe して Expecter を返す (publish より前に呼ぶこと)。
func ExpectFactionAcquired(s *Subscriber) *FactionAcquiredExpecter {
	return &FactionAcquiredExpecter{ch: s.Messages(apishop.TopicFactionAcquired)}
}

// Wait は subscribe 後に publish された最初の event を timeout 付きで取り出す。
func (e *FactionAcquiredExpecter) Wait(timeout time.Duration) (apishop.FactionAcquiredEvent, error) {
	return waitTypedFromChan[apishop.FactionAcquiredEvent](e.ch, apishop.TopicFactionAcquired, timeout)
}

// CardPackPurchasedExpecter は TopicCardPackPurchased 版の Expecter。
type CardPackPurchasedExpecter struct {
	ch <-chan []byte
}

// ExpectCardPackPurchased は TopicCardPackPurchased に即時 subscribe し Expecter を返す。
func ExpectCardPackPurchased(s *Subscriber) *CardPackPurchasedExpecter {
	return &CardPackPurchasedExpecter{ch: s.Messages(apishop.TopicCardPackPurchased)}
}

// Wait は publish された最初の CardPackPurchasedEvent を timeout 付きで取り出す。
func (e *CardPackPurchasedExpecter) Wait(timeout time.Duration) (apishop.CardPackPurchasedEvent, error) {
	return waitTypedFromChan[apishop.CardPackPurchasedEvent](e.ch, apishop.TopicCardPackPurchased, timeout)
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

// fillFactionAcquiredDefaults は EventType を上書きし、EventID / Timestamp 未設定なら補完する。
func fillFactionAcquiredDefaults(ev apishop.FactionAcquiredEvent) apishop.FactionAcquiredEvent {
	ev.EventType = apishop.EventTypeFactionAcquired
	if ev.EventID == "" {
		ev.EventID = newEventID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	return ev
}

// fillCardPackPurchasedDefaults は fillFactionAcquiredDefaults の CardPackPurchasedEvent 版。
func fillCardPackPurchasedDefaults(ev apishop.CardPackPurchasedEvent) apishop.CardPackPurchasedEvent {
	ev.EventType = apishop.EventTypeCardPackPurchased
	if ev.EventID == "" {
		ev.EventID = newEventID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	return ev
}

// fillPremiumDefaults は fillFactionAcquiredDefaults の PremiumUpdatedEvent 版 (Source 未設定なら shop)。
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
