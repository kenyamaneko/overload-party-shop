package apishopfake

import (
	"context"
	"testing"
	"time"
)

// Stream は特定 topic に subscribe 済みの observable stream。consumer 側サービス
// (account / card / gateway 等) がテストで「Broker に publish した payload を
// subscriber の Start() ループ全体を通して受け取れたか」を同期的に観測するため
// の test double。
//
// consumer 側が port.MessageStream のような interface を定義している場合、
// handler 型を type alias として定義しておけば Stream を直接その interface を
// 満たす値として扱える (consumer 側に数十行の adapter を書かせずに済む)。
//
// subscribe は NewStream の時点で eager に行う (Consume まで遅延させると、
// goroutine で Start() を起動してから Consume ループに到達する前に publish が
// 走るレースが起きうるため)。channel は Broker 側で buffer 済みなので、
// Consume 前に publish しても消えない。
//
// handler の戻り値 (ack/nack 相当) は handled channel に転送され、テストは
// ExpectHandled で同期観測する。nack 後の再配信はせず、in-memory の at-most-once
// 配信として振る舞う (retry 挙動を確かめたいテストは別 stream 実装を書く)。
type Stream struct {
	ch      <-chan []byte
	topic   string
	handled chan error
}

// NewStream は Subscriber から指定 topic のメッセージを読む Stream を返す。
// subscribe は eager に行うため、Start() を goroutine で呼ぶ前後で publish しても
// メッセージが失われない。
func NewStream(sub *Subscriber, topic string) *Stream {
	return &Stream{
		ch: sub.Messages(topic),
		// topic は ExpectHandled が timeout 時に診断ログを出すためだけに保持。
		topic:   topic,
		handled: make(chan error, handledBufferSize),
	}
}

// handledBufferSize は handled channel の buffer サイズ。buffer を非ゼロに
// することで、Consume ループが handler 結果を送出するときに ExpectHandled
// の回収を待たずブロックせず進める。
//
// 1 Stream が保持する「未回収 handler 結果」の最大数でもあるため、ExpectHandled
// を呼ばずに連続 publish すると超過してブロックする。この上限を意図的に
// 越えたいシナリオ (一度に大量 publish した後に後でまとめて検証、等) は、
// 本パッケージの想定用法外として別の fake を用意する想定。
const handledBufferSize = 16

// Consume は ctx がキャンセルされるまで subscribe 済み topic のメッセージを
// handler に渡し続ける。handler の戻り値 (ack/nack 相当の error) は handled
// channel に流し、ExpectHandled が同期的に取り出せる。
//
// 戻り値: ctx キャンセル時は nil (consumer ランナー側で「ctx キャンセルは
// 正常終了」として扱えるようにする契約)。channel クローズ時も nil。
func (s *Stream) Consume(ctx context.Context, handler func(ctx context.Context, data []byte) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case data, ok := <-s.ch:
			if !ok {
				return nil
			}
			s.handled <- handler(ctx, data)
		}
	}
}

// ExpectHandled は 1 メッセージ分の handler 戻り値を timeout 付きで取り出す。
// Publish → ExpectHandled → 副作用アサートの流れで非同期配信を同期的に観測するための
// 核となる API。timeout 超過時は t.Fatal で即失敗し、テストが silent に通り
// 抜けないようにする。
func (s *Stream) ExpectHandled(t *testing.T, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-s.handled:
		return err
	case <-time.After(timeout):
		t.Fatalf("apishopfake.Stream[%s]: timeout waiting for handler completion (%s)", s.topic, timeout)
		return nil
	}
}
