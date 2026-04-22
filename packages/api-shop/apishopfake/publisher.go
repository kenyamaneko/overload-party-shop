package apishopfake

import (
	"context"
	"sync"
)

// PublishedMessage は Publisher.Publish 1 回分の記録。送信側サービスの
// テストが「どの topic にどの payload を発行したか」をアサートするために使う。
type PublishedMessage struct {
	Topic string
	Data  []byte
}

// Publisher は送信側サービス (shop) のテスト用 fake。Broker 経由で全 subscriber に
// 配信しつつ、自身の Published() スライスに発行記録を残す。shop 自身の境界
// テストで「意図した topic と payload を発行したか」を検証する用途に加え、
// consumer 側テストから「shop がこういう publish をしたとき account はどう動くか」
// を再現する用途にも使う。
type Publisher struct {
	broker    *Broker
	mu        sync.Mutex
	published []PublishedMessage
}

// NewPublisher は指定 broker に紐づく Publisher を返す。1 つの broker を
// 複数 Publisher が共有しても良い (複数サービス役割を 1 テストで表現したい場合)。
func NewPublisher(broker *Broker) *Publisher {
	return &Publisher{broker: broker}
}

// Publish は data を topic に発行する。ctx は本 fake 内部では使わないが、
// 本番実装 (pubsub.Publisher) と同じ interface を満たすために受ける。
func (p *Publisher) Publish(_ context.Context, topic string, data []byte) error {
	// Data を caller 配下の buffer から独立させ、後から mutate されても記録が
	// 壊れないようにする。小さいコピーコストの代わりにテスト失敗時の診断性を優先。
	buf := append([]byte(nil), data...)
	p.mu.Lock()
	p.published = append(p.published, PublishedMessage{Topic: topic, Data: buf})
	p.mu.Unlock()
	p.broker.Publish(topic, buf)
	return nil
}

// Published は Publisher が発行したメッセージの snapshot を返す。戻り値は
// slice も各 Data も caller 専用の新規割付で、Publisher 内部とは独立する
// (caller の mutation が内部状態や他回呼び出し結果に影響しない)。
func (p *Publisher) Published() []PublishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]PublishedMessage, len(p.published))
	for i, m := range p.published {
		out[i] = PublishedMessage{
			Topic: m.Topic,
			Data:  append([]byte(nil), m.Data...),
		}
	}
	return out
}
