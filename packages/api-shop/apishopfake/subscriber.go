package apishopfake

// Subscriber は受信側サービス (account / card / gateway 等) のテスト用 fake。
// Broker 経由で到着する payload bytes を topic 単位の channel で受け取る。
// typed helpers (typed.go) の WaitForFactionPurchased 等は本 Subscriber を
// 内部で使用する。
type Subscriber struct {
	broker *Broker
}

// NewSubscriber は指定 broker に紐づく Subscriber を返す。
func NewSubscriber(broker *Broker) *Subscriber {
	return &Subscriber{broker: broker}
}

// Messages は topic に発行された payload を読み取る channel を返す。複数回
// 呼ぶと独立した channel が返り、fan-out 検証や複数 consumer の振る舞いを
// 1 テスト内で表現できる。channel は Broker 側で buffer を確保しているため
// ドレインを怠ると Publisher 側がブロックする (テストハングで発覚する設計)。
func (s *Subscriber) Messages(topic string) <-chan []byte {
	return s.broker.Subscribe(topic)
}
