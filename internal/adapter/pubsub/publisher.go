// Package pubsub は shop サービスの Pub/Sub publisher を提供する。
//
// shop は 2 種のサービス横断イベントを発行する:
//
//   - `faction_purchased` — faction_set 購入 commit 時。account / card / gateway が
//     それぞれ subscribe して自スキーマを更新する。
//   - `premium_updated` — subscription 状態変更で is_premium が変わったとき。
//     account / gateway が subscribe する。
//
// Publisher は worker (outbox) から eventType + payload 指定で呼ばれる低レベル
// 送信層。物理 topic 名への解決は Publisher 内部の eventType→topic 表で完結し、
// 上位層 (service / outbox / worker) は topic という Pub/Sub 固有概念を一切
// 知らない。イベント struct の構築は service/event の build 関数が担う。
package pubsub

import (
	"context"
	"errors"
	"fmt"

	gpubsub "cloud.google.com/go/pubsub/v2"

	apishop "github.com/kenyamaneko/overload-party-shop/packages/api-shop"
)

// Publisher は shop が publish する全 topic をまとめる。起動時に一度だけ構築し、
// Close で in-flight メッセージを flush して gRPC client を解放する。
// port.RawEventPublisher を実装する。
type Publisher struct {
	client      *gpubsub.Client
	byEventType map[string]*gpubsub.Publisher
}

// New は指定 project + 物理 topic 名から Publisher を構築する。
// topic 名は GCP リソース固有 (configmap / env で外から差し替え可能) なため
// 構築時に受け取り、内部で apishop.EventType* → 該当 topic の対応表を組む。
// 以降 Publish 呼び出し側は eventType しか知らない。両 topic は Terraform
// (modules/pubsub) で事前作成されている前提。
func New(ctx context.Context, projectID, factionPurchasedTopic, premiumUpdatedTopic string) (*Publisher, error) {
	if projectID == "" {
		return nil, errors.New("pubsub: projectID is empty")
	}
	if factionPurchasedTopic == "" || premiumUpdatedTopic == "" {
		return nil, errors.New("pubsub: both topic names are required")
	}
	client, err := gpubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new pubsub client: %w", err)
	}
	return &Publisher{
		client: client,
		byEventType: map[string]*gpubsub.Publisher{
			apishop.EventTypeFactionPurchased: client.Publisher(factionPurchasedTopic),
			apishop.EventTypePremiumUpdated:   client.Publisher(premiumUpdatedTopic),
		},
	}, nil
}

// Close は in-flight メッセージを flush し Pub/Sub client を閉じる。
func (p *Publisher) Close() error {
	for _, pub := range p.byEventType {
		pub.Stop()
	}
	return p.client.Close()
}

// Publish は eventType に対応する Pub/Sub topic に payload をそのまま送る。
// worker が outbox 行から eventType + payload を読み出して呼ぶ想定。
// 未登録 eventType はエラーを返す (outbox 行の eventType 設定ミスを alert 経路に
// 載せるため)。
func (p *Publisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	pub, ok := p.byEventType[eventType]
	if !ok {
		return fmt.Errorf("pubsub: unknown event type %q", eventType)
	}
	result := pub.Publish(ctx, &gpubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish event_type=%s: %w", eventType, err)
	}
	return nil
}
