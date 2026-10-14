package apishopfake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// 各 Topic は data/asyncapi.yaml の channels の address と同じ値。
const (
	// TopicFactionAcquired は FactionAcquiredEvent の topic 名。
	TopicFactionAcquired = "faction-acquired"
	// TopicCardPackPurchased は CardPackPurchasedEvent の topic 名。
	TopicCardPackPurchased = "card-pack-purchased"
	// TopicPremiumUpdated は PremiumUpdatedEvent の topic 名。
	TopicPremiumUpdated = "premium-updated"
)

// PublishFactionAcquired は TopicFactionAcquired へ FactionAcquiredEvent を 1 件発行する。
// EventID / Timestamp 未設定なら UUIDv4 / 現在時刻を補完、EventType は常に上書きする。
func PublishFactionAcquired(ctx context.Context, p *Publisher, event apishop.FactionAcquiredEvent) error {
	event = fillFactionAcquiredDefaults(event)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal FactionAcquiredEvent: %w", err)
	}
	return p.Publish(ctx, TopicFactionAcquired, data)
}

// PublishCardPackPurchased は TopicCardPackPurchased へ CardPackPurchasedEvent を 1 件発行する。
// EventID / Timestamp 未設定なら UUIDv4 / 現在時刻を補完、EventType は常に上書きする。
func PublishCardPackPurchased(ctx context.Context, p *Publisher, event apishop.CardPackPurchasedEvent) error {
	event = fillCardPackPurchasedDefaults(event)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal CardPackPurchasedEvent: %w", err)
	}
	return p.Publish(ctx, TopicCardPackPurchased, data)
}

// PublishPremiumUpdated は TopicPremiumUpdated へ PremiumUpdatedEvent を 1 件発行する。
// 他 Publish ヘルパと同じ補完に加え、Source 空なら PremiumUpdatedSourceShop を埋める。
func PublishPremiumUpdated(ctx context.Context, p *Publisher, event apishop.PremiumUpdatedEvent) error {
	event = fillPremiumDefaults(event)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal PremiumUpdatedEvent: %w", err)
	}
	return p.Publish(ctx, TopicPremiumUpdated, data)
}

// FactionAcquiredExpecter は TopicFactionAcquired に subscribe 済みの待受器。
// publish より前に subscribe を確定する必要があるため (Broker は過去メッセージを配信しない)、
// API は ExpectFactionAcquired → publish → Wait の順序を強制する形に分割している。
type FactionAcquiredExpecter struct {
	ch <-chan []byte
}

// ExpectFactionAcquired は即時 subscribe して Expecter を返す (publish より前に呼ぶこと)。
func ExpectFactionAcquired(s *Subscriber) *FactionAcquiredExpecter {
	return &FactionAcquiredExpecter{ch: s.Messages(TopicFactionAcquired)}
}

// Wait は subscribe 後に publish された最初の event を timeout 付きで取り出す。
func (e *FactionAcquiredExpecter) Wait(timeout time.Duration) (apishop.FactionAcquiredEvent, error) {
	return waitTypedFromChan[apishop.FactionAcquiredEvent](e.ch, TopicFactionAcquired, timeout)
}

// CardPackPurchasedExpecter は TopicCardPackPurchased 版の Expecter。
type CardPackPurchasedExpecter struct {
	ch <-chan []byte
}

// ExpectCardPackPurchased は TopicCardPackPurchased に即時 subscribe し Expecter を返す。
func ExpectCardPackPurchased(s *Subscriber) *CardPackPurchasedExpecter {
	return &CardPackPurchasedExpecter{ch: s.Messages(TopicCardPackPurchased)}
}

// Wait は publish された最初の CardPackPurchasedEvent を timeout 付きで取り出す。
func (e *CardPackPurchasedExpecter) Wait(timeout time.Duration) (apishop.CardPackPurchasedEvent, error) {
	return waitTypedFromChan[apishop.CardPackPurchasedEvent](e.ch, TopicCardPackPurchased, timeout)
}

// PremiumUpdatedExpecter は TopicPremiumUpdated 版の Expecter。
type PremiumUpdatedExpecter struct {
	ch <-chan []byte
}

// ExpectPremiumUpdated は TopicPremiumUpdated に即時 subscribe し Expecter を返す。
func ExpectPremiumUpdated(s *Subscriber) *PremiumUpdatedExpecter {
	return &PremiumUpdatedExpecter{ch: s.Messages(TopicPremiumUpdated)}
}

// Wait は publish された最初の PremiumUpdatedEvent を timeout 付きで取り出す。
func (e *PremiumUpdatedExpecter) Wait(timeout time.Duration) (apishop.PremiumUpdatedEvent, error) {
	return waitTypedFromChan[apishop.PremiumUpdatedEvent](e.ch, TopicPremiumUpdated, timeout)
}

// waitTypedFromChan は payload bytes を timeout 付きで受信し型 T にデコードする。
func waitTypedFromChan[T any](ch <-chan []byte, topic string, timeout time.Duration) (T, error) {
	var zero T
	select {
	case data := <-ch:
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return zero, fmt.Errorf("unmarshal %q payload: %w", topic, err)
		}
		return v, nil
	case <-time.After(timeout):
		return zero, fmt.Errorf("timeout waiting for %q after %s", topic, timeout)
	}
}

// fillEventIdentity は EventID / Timestamp が未設定なら補完する。
func fillEventIdentity(eventID *string, timestamp *time.Time) {
	if *eventID == "" {
		*eventID = newEventID()
	}
	if timestamp.IsZero() {
		*timestamp = time.Now().UTC()
	}
}

// fillFactionAcquiredDefaults は EventType を上書きし、EventID / Timestamp 未設定なら補完する。
func fillFactionAcquiredDefaults(event apishop.FactionAcquiredEvent) apishop.FactionAcquiredEvent {
	event.EventType = apishop.EventTypeFactionAcquired
	fillEventIdentity(&event.EventID, &event.Timestamp)
	return event
}

// fillCardPackPurchasedDefaults は fillFactionAcquiredDefaults の CardPackPurchasedEvent 版。
func fillCardPackPurchasedDefaults(event apishop.CardPackPurchasedEvent) apishop.CardPackPurchasedEvent {
	event.EventType = apishop.EventTypeCardPackPurchased
	fillEventIdentity(&event.EventID, &event.Timestamp)
	return event
}

// fillPremiumDefaults は fillFactionAcquiredDefaults の PremiumUpdatedEvent 版 (Source 未設定なら shop)。
func fillPremiumDefaults(event apishop.PremiumUpdatedEvent) apishop.PremiumUpdatedEvent {
	event.EventType = apishop.EventTypePremiumUpdated
	fillEventIdentity(&event.EventID, &event.Timestamp)
	if event.Source == "" {
		event.Source = apishop.PremiumUpdatedSourceShop
	}
	return event
}
