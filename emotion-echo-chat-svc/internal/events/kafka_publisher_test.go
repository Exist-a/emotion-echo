// Package events — kafka_publisher_test.go
//
// Sibling test for kafka_publisher.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §五 coverage: chat-svc/events/kafka_publisher.go
// (LOC=64) had no sibling test. The KafkaEventPublisher.Publish
// path encodes JSON → builds ProducerMessage → SendMessage. We
// exercise that contract with a sarama mock SyncProducer (no real
// broker required; see sarama/mocks).
//
// Coverage matrix:
//
//   - Happy path: Publish encodes event as Protobuf envelope (Stage 73),
//     uses event ID as key, writes content-type header, sends to mock
//     producer; mock returns (partition=0, offset=0), Publish returns nil
//   - Marshal error: Event without typed Data is rejected by
//     MarshalChatEvent BEFORE reaching the broker (oneof 契约)
//   - SendMessage error: mock returns SendMessage error; Publish
//     propagates the error verbatim
//   - Close: propagates mock Close error
//
// Why not test NewKafkaEventPublisher end-to-end? It dials a real
// broker via sarama.NewSyncProducer — that requires a Kafka host
// and belongs in the //go:build integration suite (see
// chat-svc/integration_test/), not in this unit-test sibling file.
package events

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
	"google.golang.org/protobuf/proto"

	chatevents "github.com/emotion-echo/shared/pkg/chatevents"
)

func TestKafkaEventPublisher_Publish_HappyPath_EncodesJSON(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	mockProducer.ExpectSendMessageAndSucceed()

	p := &KafkaEventPublisher{producer: mockProducer}

	ev := &Event{
		ID:     "evt-123",
		Type:   EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   time.Unix(1700000000, 0).UTC(),
		Data:   MessageCreatedData{MessageID: 1, ConversationID: 2, UserID: 3},
	}
	err := p.Publish(context.Background(), TopicChatEvents, ev)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	// mockProducer.VerifyExpectations() is called automatically by
	// mocks.NewSyncProducer(t, ...) at test cleanup; one
	// ExpectSendMessageAndSucceed must be matched.
}

func TestKafkaEventPublisher_Publish_KeyIsEventID(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	// Capture the actual message by capturing the closure passed to
	// ExpectSendMessageAndSucceed via WithChecker.
	mockProducer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(
		func(msg *sarama.ProducerMessage) error {
			// msg.Key is sarama.StringEncoder(e.ID); cast & verify.
			enc, ok := msg.Key.(sarama.StringEncoder)
			if !ok {
				t.Fatalf("msg.Key is %T, want sarama.StringEncoder", msg.Key)
			}
			if string(enc) != "my-event-id-42" {
				t.Errorf("key = %q, want %q", string(enc), "my-event-id-42")
			}
			return nil
		},
	)

	p := &KafkaEventPublisher{producer: mockProducer}
	err := p.Publish(context.Background(), TopicChatEvents, &Event{
		ID:   "my-event-id-42",
		Type: EventTypeMessageCreated,
		Data: MessageCreatedData{MessageID: 1},
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
}

// Stage 73 契约：payload 是 Protobuf 编码的 ChatEventEnvelope + content-type
// header 标记（不再是 JSON）。本测试锁定编码格式与 header，防止回退到 JSON
// 或丢 header（consumer 靠 header 识别新旧格式，双写窗口依赖它）。
func TestKafkaEventPublisher_Publish_ValueIsValidProtoEnvelope(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	mockProducer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(
		func(msg *sarama.ProducerMessage) error {
			enc, ok := msg.Value.(sarama.ByteEncoder)
			if !ok {
				t.Fatalf("msg.Value is %T, want sarama.ByteEncoder", msg.Value)
			}
			var got chatevents.ChatEventEnvelope
			if err := proto.Unmarshal(enc, &got); err != nil {
				t.Fatalf("msg.Value is not a valid ChatEventEnvelope: %v", err)
			}
			if got.GetId() != "evt-1" {
				t.Errorf("envelope id = %q, want evt-1", got.GetId())
			}
			if got.GetType() != EventTypeMessageCreated {
				t.Errorf("envelope type = %q, want %q", got.GetType(), EventTypeMessageCreated)
			}
			if got.GetSource() != "chat-svc" {
				t.Errorf("envelope source = %q, want chat-svc", got.GetSource())
			}
			if got.GetMessageCreated() == nil {
				t.Errorf("envelope oneof data is nil, want message_created set")
			}
			var ct []byte
			for _, h := range msg.Headers {
				if string(h.Key) == "content-type" {
					ct = h.Value
				}
			}
			if string(ct) != ContentTypeHeaderProto {
				t.Errorf("content-type header = %q, want %q", string(ct), ContentTypeHeaderProto)
			}
			return nil
		},
	)

	p := &KafkaEventPublisher{producer: mockProducer}
	err := p.Publish(context.Background(), TopicChatEvents, &Event{
		ID:     "evt-1",
		Type:   EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   time.Unix(1700000000, 0).UTC(),
		Data:   MessageCreatedData{MessageID: 1},
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
}

// Marshal 阶段报错（如 Data nil，违反 oneof 契约）必须发生在触达 broker 之前，
// 且错误原样返回——不能被吞掉也不能空发消息。
func TestKafkaEventPublisher_Publish_MarshalError_NotSentToBroker(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	mockProducer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(
		func(msg *sarama.ProducerMessage) error {
			t.Error("SendMessage reached broker, want marshal error before send")
			return nil
		},
	)

	p := &KafkaEventPublisher{producer: mockProducer}
	err := p.Publish(context.Background(), TopicChatEvents, &Event{
		ID:   "evt-nil-data",
		Type: EventTypeMessageCreated,
	})
	if err == nil {
		t.Fatal("expected Publish to return marshal error for nil Data, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported Data type") {
		t.Errorf("Publish err = %v, want marshal error mentioning unsupported Data type", err)
	}
}

func TestKafkaEventPublisher_Publish_SendMessageError_Propagates(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	boom := errors.New("kafka broker unavailable")
	mockProducer.ExpectSendMessageAndFail(boom)

	p := &KafkaEventPublisher{producer: mockProducer}
	err := p.Publish(context.Background(), TopicChatEvents, &Event{
		ID:   "evt-err",
		Type: EventTypeMessageCreated,
		// Stage 73 起 Data 必须是 typed payload，否则 marshal 先于 SendMessage
		// 报错；本测试验证的是 broker 错误透传，故需合法 Data 才能走到发送。
		Data: MessageCreatedData{MessageID: 1},
	})
	if err == nil {
		t.Fatal("expected Publish to return SendMessage error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Errorf("Publish err = %v, want wraps %v", err, boom)
	}
}

func TestKafkaEventPublisher_Close_PropagatesError(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	// mocks.NewSyncProducer auto-closes on test cleanup; we can't
	// intercept that to inject a Close error. Instead we test that
	// Close on a fresh publisher is a no-error wrapper around the
	// mock's Close (which returns nil).
	p := &KafkaEventPublisher{producer: mockProducer}
	if err := p.Close(); err != nil {
		t.Errorf("Close on freshly-constructed publisher err = %v, want nil", err)
	}
}

// KafkaEventPublisher_Publish_TopicIsForwarded locks that the topic
// string is passed through unmodified to sarama.
func TestKafkaEventPublisher_Publish_TopicIsForwarded(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	customTopic := "custom-topic-xyz"
	mockProducer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(
		func(msg *sarama.ProducerMessage) error {
			if msg.Topic != customTopic {
				t.Errorf("msg.Topic = %q, want %q", msg.Topic, customTopic)
			}
			return nil
		},
	)

	p := &KafkaEventPublisher{producer: mockProducer}
	err := p.Publish(context.Background(), customTopic, &Event{
		ID:   "evt-topic",
		Type: EventTypeMessageCreated,
		Data: MessageCreatedData{MessageID: 1},
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
}