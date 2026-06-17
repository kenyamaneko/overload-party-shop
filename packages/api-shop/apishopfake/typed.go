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
func PublishFactionAcquired(ctx context.Context, p *Publisher, event apishop.FactionAcquiredEvent) error {
	event = fillFactionAcquiredDefaults(event)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal FactionAcquiredEvent: %w", err)
	}
	return p.Publish(ctx, "faction-acquired", data)
}

// PublishCardPackPurchased は TopicCardPackPurchased へ CardPackPurchasedEvent を 1 件発行する。
// EventID / Timestamp 未設定なら UUIDv4 / 現在時刻を補完、EventType は常に上書きする。
func PublishCardPackPurchased(ctx context.Context, p *Publisher, event apishop.CardPackPurchasedEvent) error {
	event = fillCardPackPurchasedDefaults(event)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal CardPackPurchasedEvent: %w", err)
	}
	return p.Publish(ctx, "card-pack-purchased", data)
}

// PublishPremiumUpdated は TopicPremiumUpdated へ PremiumUpdatedEvent を 1 件発行する。
// 他 Publish ヘルパと同じ補完に加え、Source 空なら PremiumUpdatedSourceShop を埋める。
func PublishPremiumUpdated(ctx context.Context, p *Publisher, event apishop.PremiumUpdatedEvent) error {
	event = fillPremiumDefaults(event)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal PremiumUpdatedEvent: %w", err)
	}
	return p.Publish(ctx, "premium-updated", data)
}

// FactionAcquiredExpecter は TopicFactionAcquired に subscribe 済みの待受器。
// publish より前に subscribe を確定する必要があるため (Broker は過去メッセージを配信しない)、
// API は ExpectFactionAcquired → publish → Wait の順序を強制する形に分割している。
type FactionAcquiredExpecter struct {
	ch <-chan []byte
}

// ExpectFactionAcquired は即時 subscribe して Expecter を返す (publish より前に呼ぶこと)。
func ExpectFactionAcquired(s *Subscriber) *FactionAcquiredExpecter {
	return &FactionAcquiredExpecter{ch: s.Messages("faction-acquired")}
}

// Wait は subscribe 後に publish された最初の event を timeout 付きで取り出す。
func (e *FactionAcquiredExpecter) Wait(timeout time.Duration) (apishop.FactionAcquiredEvent, error) {
	return waitTypedFromChan[apishop.FactionAcquiredEvent](e.ch, "faction-acquired", timeout)
}

// CardPackPurchasedExpecter は TopicCardPackPurchased 版の Expecter。
type CardPackPurchasedExpecter struct {
	ch <-chan []byte
}

// ExpectCardPackPurchased は TopicCardPackPurchased に即時 subscribe し Expecter を返す。
func ExpectCardPackPurchased(s *Subscriber) *CardPackPurchasedExpecter {
	return &CardPackPurchasedExpecter{ch: s.Messages("card-pack-purchased")}
}

// Wait は publish された最初の CardPackPurchasedEvent を timeout 付きで取り出す。
func (e *CardPackPurchasedExpecter) Wait(timeout time.Duration) (apishop.CardPackPurchasedEvent, error) {
	return waitTypedFromChan[apishop.CardPackPurchasedEvent](e.ch, "card-pack-purchased", timeout)
}

// PremiumUpdatedExpecter は TopicPremiumUpdated 版の Expecter。
type PremiumUpdatedExpecter struct {
	ch <-chan []byte
}

// ExpectPremiumUpdated は TopicPremiumUpdated に即時 subscribe し Expecter を返す。
func ExpectPremiumUpdated(s *Subscriber) *PremiumUpdatedExpecter {
	return &PremiumUpdatedExpecter{ch: s.Messages("premium-updated")}
}

// Wait は publish された最初の PremiumUpdatedEvent を timeout 付きで取り出す。
func (e *PremiumUpdatedExpecter) Wait(timeout time.Duration) (apishop.PremiumUpdatedEvent, error) {
	return waitTypedFromChan[apishop.PremiumUpdatedEvent](e.ch, "premium-updated", timeout)
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
func fillFactionAcquiredDefaults(event apishop.FactionAcquiredEvent) apishop.FactionAcquiredEvent {
	event.EventType = apishop.EventTypeFactionAcquired
	if event.EventID == "" {
		event.EventID = newEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	return event
}

// fillCardPackPurchasedDefaults は fillFactionAcquiredDefaults の CardPackPurchasedEvent 版。
func fillCardPackPurchasedDefaults(event apishop.CardPackPurchasedEvent) apishop.CardPackPurchasedEvent {
	event.EventType = apishop.EventTypeCardPackPurchased
	if event.EventID == "" {
		event.EventID = newEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	return event
}

// fillPremiumDefaults は fillFactionAcquiredDefaults の PremiumUpdatedEvent 版 (Source 未設定なら shop)。
func fillPremiumDefaults(event apishop.PremiumUpdatedEvent) apishop.PremiumUpdatedEvent {
	event.EventType = apishop.EventTypePremiumUpdated
	if event.EventID == "" {
		event.EventID = newEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Source == "" {
		event.Source = apishop.PremiumUpdatedSourceShop
	}
	return event
}
