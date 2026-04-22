// Package apishopfake は api-shop の送信側サービス (shop) が consumer 側
// サービスに提供するテスト用 fake。ADR-024 のサービス間結合テスト戦略に
// 基づき、送信側パッケージがテストダブルを同梱することで、consumer 側の
// 手書き fake が契約と乖離するのを防ぐ。
//
// 基本構成: Broker + Publisher + Subscriber + typed helpers。
//   - Broker: topic 名をキーとした in-memory pub/sub 基盤 (低レベル API)
//   - Publisher: 送信側サービス (shop) の自テストが発行アサーションに使う fake
//   - Subscriber: 受信側サービス (account / card 等) のテストで消費検証に使う fake
//   - typed helpers: TopicFactionPurchased / TopicPremiumUpdated に対応する
//     型付き publish/receive API (typed.go)
//
// Broker / Publisher / Subscriber は topic 名と payload ([]byte) を引数で
// 取る低レベル API で、新しい topic が増えた時に本体を変更せず使い続けられる。
// 一方で consumer 側のテスト書き心地を優先する場合は typed helpers を使うと
// json encode / decode の定型コードを書かずに済む。
package apishopfake

import "sync"

// Broker は apishopfake 内の publisher / subscriber を仲介する topic ベースの
// in-memory pub/sub 実装。テスト毎に NewBroker で新規生成する想定で、状態は
// プロセス内揮発的で永続化・再送・配信保証は行わない。
type Broker struct {
	mu      sync.Mutex
	byTopic map[string][]chan []byte
}

// NewBroker は空の in-memory broker を返す。
func NewBroker() *Broker {
	return &Broker{byTopic: make(map[string][]chan []byte)}
}

// Publish は topic に subscribe している全 channel に data を配信する。
// channel 満杯時はブロックする — テストがメッセージをドレインし忘れた場合に
// ハングが観測できるようにする意図的な選択 (silent drop だとテスト漏れが
// 隠れるため)。buffer は Subscribe 側で確保する (デフォルト 100)。
func (b *Broker) Publish(topic string, data []byte) {
	b.mu.Lock()
	subs := append([]chan []byte(nil), b.byTopic[topic]...)
	b.mu.Unlock()
	for _, ch := range subs {
		ch <- data
	}
}

// Subscribe は topic に対する新しい受信 channel を返す。buffer サイズは 100。
// 複数回呼べば複数 subscriber 扱いとなり、Publish 時に fan-out される。
func (b *Broker) Subscribe(topic string) <-chan []byte {
	ch := make(chan []byte, defaultSubscribeBuffer)
	b.mu.Lock()
	b.byTopic[topic] = append(b.byTopic[topic], ch)
	b.mu.Unlock()
	return ch
}

// defaultSubscribeBuffer は Subscribe が返す channel の buffer サイズ。
// 通常のテストシナリオ (1 ケースあたり数十メッセージ) で詰まらないサイズとして 100 を選択。
const defaultSubscribeBuffer = 100
