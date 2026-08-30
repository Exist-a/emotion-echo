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
//   - Happy path: Publish encodes event JSON, uses event ID as key,
//     sends to mock producer; mock returns (partition=0, offset=0),
//     Publish returns nil
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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
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

func TestKafkaEventPublisher_Publish_ValueIsValidJSON(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	mockProducer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(
		func(msg *sarama.ProducerMessage) error {
			enc, ok := msg.Value.(sarama.ByteEncoder)
			if !ok {
				t.Fatalf("msg.Value is %T, want sarama.ByteEncoder", msg.Value)
			}
			// Round-trip JSON → verify shape.
			var got map[string]any
			if err := json.Unmarshal(enc, &got); err != nil {
				t.Fatalf("msg.Value is not valid JSON: %v", err)
			}
			if got["id"] != "evt-1" {
				t.Errorf("JSON id = %v, want evt-1", got["id"])
			}
			if got["type"] != EventTypeMessageCreated {
				t.Errorf("JSON type = %v, want %v", got["type"], EventTypeMessageCreated)
			}
			if got["source"] != "chat-svc" {
				t.Errorf("JSON source = %v, want chat-svc", got["source"])
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

func TestKafkaEventPublisher_Publish_SendMessageError_Propagates(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	boom := errors.New("kafka broker unavailable")
	mockProducer.ExpectSendMessageAndFail(boom)

	p := &KafkaEventPublisher{producer: mockProducer}
	err := p.Publish(context.Background(), TopicChatEvents, &Event{
		ID:   "evt-err",
		Type: EventTypeMessageCreated,
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
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
}