// Package pubsubtest は gcloud の Pub/Sub エミュレータを testcontainers で起動するテストヘルパ。
package pubsubtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	gpubsub "cloud.google.com/go/pubsub/v2"
	pubsubpb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ErrTimeout は WaitForMessage が待ち時間内にメッセージを受け取れなかったときに返る。
var ErrTimeout = errors.New("pubsubtest: timeout waiting for message")

const (
	emulatorImage          = "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators"
	emulatorPortNumber     = "8085"
	emulatorPort           = emulatorPortNumber + "/tcp"
	emulatorLogReady       = "Server started, listening on " + emulatorPortNumber
	emulatorStartupTimeout = 90 * time.Second
	idSuffixLength         = 8
	// 受信処理がチャンネルへの送信で止まらないよう、受信したメッセージを溜められるようにする。
	receivedMessageBufferSize = 64
)

// Emulator は起動済みの Pub/Sub エミュレータのコンテナを扱うハンドル。
type Emulator struct {
	container testcontainers.Container
	projectID string
	client    *gpubsub.Client
}

// StartEmulator は gcloud の Pub/Sub エミュレータのコンテナを起動し、PUBSUB_EMULATOR_HOST をプロセス全体に設定する。
func StartEmulator(ctx context.Context, projectID string) (*Emulator, error) {
	if projectID == "" {
		return nil, errors.New("pubsubtest: projectID is required")
	}

	req := testcontainers.ContainerRequest{
		Image:        emulatorImage,
		ExposedPorts: []string{emulatorPort},
		Cmd: []string{
			"gcloud", "beta", "emulators", "pubsub", "start",
			"--host-port=0.0.0.0:" + emulatorPortNumber,
			"--project=" + projectID,
		},
		WaitingFor: wait.ForLog(emulatorLogReady).WithStartupTimeout(emulatorStartupTimeout),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start pubsub emulator container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("container host: %w", err), container.Terminate(ctx))
	}
	mapped, err := container.MappedPort(ctx, emulatorPortNumber)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("mapped port: %w", err), container.Terminate(ctx))
	}
	endpoint := fmt.Sprintf("%s:%s", host, mapped.Port())

	// 本番コードの gpubsub.NewClient(projectID) を変えずにエミュレータへ向けるため、環境変数で接続先を渡す。
	if err := os.Setenv("PUBSUB_EMULATOR_HOST", endpoint); err != nil {
		return nil, fmt.Errorf("setenv PUBSUB_EMULATOR_HOST: %w", err)
	}

	client, err := newEmulatorClient(ctx, projectID, endpoint)
	if err != nil {
		return nil, errors.Join(err, container.Terminate(ctx))
	}

	return &Emulator{
		container: container,
		projectID: projectID,
		client:    client,
	}, nil
}

// newEmulatorClient はエミュレータ用の、認証なしの gRPC 接続を持つクライアントを構築する。
func newEmulatorClient(ctx context.Context, projectID, endpoint string) (*gpubsub.Client, error) {
	opts := []option.ClientOption{
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
	client, err := gpubsub.NewClient(ctx, projectID, opts...)
	if err != nil {
		return nil, fmt.Errorf("new pubsub client: %w", err)
	}
	return client, nil
}

// Close はクライアントとコンテナを停止する。
func (e *Emulator) Close(ctx context.Context) error {
	var errs []error
	if err := e.client.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close client: %w", err))
	}
	if err := e.container.Terminate(ctx); err != nil {
		errs = append(errs, fmt.Errorf("terminate container: %w", err))
	}
	if err := os.Unsetenv("PUBSUB_EMULATOR_HOST"); err != nil {
		errs = append(errs, fmt.Errorf("unsetenv PUBSUB_EMULATOR_HOST: %w", err))
	}
	return errors.Join(errs...)
}

// ProjectID はエミュレータの起動時に指定したプロジェクト ID を返す。
func (e *Emulator) ProjectID() string { return e.projectID }

// CreateTopic は接頭辞に UUID の接尾辞を付けた一意のトピックを作成し、そのトピック ID を返す。
func (e *Emulator) CreateTopic(t *testing.T, prefix string) string {
	t.Helper()
	topicID := fmt.Sprintf("%s-%s", prefix, uuid.NewString()[:idSuffixLength])
	e.CreateTopicWithID(t, topicID)
	return topicID
}

// CreateTopicWithID は指定した ID のトピックを作成する。
func (e *Emulator) CreateTopicWithID(t *testing.T, topicID string) {
	t.Helper()
	ctx := context.Background()
	_, err := e.client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{
		Name: fmt.Sprintf("projects/%s/topics/%s", e.projectID, topicID),
	})
	if err != nil {
		t.Fatalf("create topic %s: %v", topicID, err)
	}
}

// Subscribe は指定したトピックに、UUID の接尾辞を付けたサブスクリプションを作成し、受信を開始する。
func (e *Emulator) Subscribe(t *testing.T, topicID string) *Subscription {
	t.Helper()

	subscriptionID := fmt.Sprintf("%s-sub-%s", topicID, uuid.NewString()[:idSuffixLength])
	ctx := context.Background()
	_, err := e.client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:  fmt.Sprintf("projects/%s/subscriptions/%s", e.projectID, subscriptionID),
		Topic: fmt.Sprintf("projects/%s/topics/%s", e.projectID, topicID),
	})
	if err != nil {
		t.Fatalf("create subscription %s: %v", subscriptionID, err)
	}

	ch := make(chan *Message, receivedMessageBufferSize)
	receiveCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	subscriber := e.client.Subscriber(subscriptionID)
	go func() {
		defer close(done)
		_ = subscriber.Receive(receiveCtx, func(_ context.Context, m *gpubsub.Message) {
			m.Ack()
			msg := &Message{Data: m.Data}
			// 停止処理が送信待ちで止まらないよう、受信の終了も待つ。
			select {
			case ch <- msg:
			case <-receiveCtx.Done():
			}
		})
	}()

	s := &Subscription{
		ch:     ch,
		cancel: cancel,
		done:   done,
	}
	t.Cleanup(func() {
		s.stop()
		// コンテナごと破棄されるため、サブスクリプションの削除の失敗は無視する。
		_ = e.client.SubscriptionAdminClient.DeleteSubscription(context.Background(), &pubsubpb.DeleteSubscriptionRequest{
			Subscription: fmt.Sprintf("projects/%s/subscriptions/%s", e.projectID, subscriptionID),
		})
	})
	return s
}

// Message はサブスクリプションで受信したメッセージのテスト用の表現。
type Message struct {
	Data []byte
}

// Subscription は受信の停止とメッセージの受け取りに使うハンドル。
type Subscription struct {
	ch     chan *Message
	cancel context.CancelFunc
	done   chan struct{}

	stopOnce sync.Once
}

func (s *Subscription) stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		<-s.done
	})
}

// WaitForMessage は timeout 以内に 1 件届くまで待つ。届かなければ ErrTimeout を返す。
func (s *Subscription) WaitForMessage(ctx context.Context, timeout time.Duration) (*Message, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case msg := <-s.ch:
		return msg, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, ctx.Err()
	}
}
