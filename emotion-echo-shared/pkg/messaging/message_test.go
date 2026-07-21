package messaging

import (
	"errors"
	"testing"
)

// TestMessagingErrors 定义错误可被 errors.Is 识别
func TestMessagingErrors_ErrorsIs(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"EmptyTopic", ErrEmptyTopic, ErrEmptyTopic},
		{"EmptyValue", ErrEmptyValue, ErrEmptyValue},
		{"ProducerClosed", ErrProducerClosed, ErrProducerClosed},
		{"PublishCancelled", ErrPublishCancelled, ErrPublishCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.want) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", tt.err, tt.want)
			}
		})
	}
}

// TestMessagingErrors_NonEmptyMessages 内容非空
func TestMessagingErrors_NonEmptyMessages(t *testing.T) {
	for _, err := range []error{ErrEmptyTopic, ErrEmptyValue, ErrProducerClosed, ErrPublishCancelled} {
		if err.Error() == "" {
			t.Fatalf("err message should be non-empty: %v", err)
		}
	}
}

// TestMessage_Struct 字段赋值正确性
func TestMessage_Struct(t *testing.T) {
	m := Message{
		Topic: "chat-events",
		Key:   []byte("k1"),
		Value: []byte(`{"foo":"bar"}`),
		Headers: map[string]string{
			"trace-id": "abc",
		},
	}
	if m.Topic != "chat-events" {
		t.Fatalf("topic mismatch")
	}
	if string(m.Key) != "k1" {
		t.Fatalf("key mismatch")
	}
	if string(m.Value) != `{"foo":"bar"}` {
		t.Fatalf("value mismatch")
	}
	if m.Headers["trace-id"] != "abc" {
		t.Fatalf("header mismatch")
	}
}

// TestMessage_ZeroValue 全零值合法
func TestMessage_ZeroValue(t *testing.T) {
	var m Message
	if m.Topic != "" {
		t.Fatalf("zero topic")
	}
	if m.Value != nil {
		t.Fatalf("zero value")
	}
	if m.Key != nil {
		t.Fatalf("zero key")
	}
	if m.Headers != nil {
		t.Fatalf("zero headers")
	}
}

// TestMessagingErrorTypes_StaticTypes 不同错误类型不应等价
func TestMessagingErrorTypes_StaticTypes(t *testing.T) {
	if errors.Is(ErrEmptyTopic, ErrEmptyValue) {
		t.Fatalf("empty topic should not match empty value")
	}
	if errors.Is(ErrEmptyTopic, ErrProducerClosed) {
		t.Fatalf("empty topic should not match producer closed")
	}
}
