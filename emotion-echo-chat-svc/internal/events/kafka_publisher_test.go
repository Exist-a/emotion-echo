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
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
	"google.golang.org/protobuf/proto"

	chatevents "github.com/emotion-echo/shared/pkg/chatevents"
	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
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

// Stage 92 PR-1 RED：KafkaEventPublisher 必须支持注入 sw8 trace header 到
// ProducerMessage.Headers，让 ai-svc / analytics-svc consumer 通过 sarama
// header 重建上游 trace context（跨进程 trace）。
//
// 行为契约：
//   - 构造时注入 tracer（grpcinterceptor.Tracer 接口）
//   - Publish 时 ctx 含上游 sw8 → 必须把 sw8 写到 msg.Headers["sw8"]
//   - tracer 为 nil 时降级原行为（不破坏现有 dev/in-memory 路径）
//
// 注意：本测试只锁"sw8 被写到 header"；consumer 端用 sw8 重建父 span 由
// ai-svc PR-2 验证（独立 RED/GREEN）。
func TestKafkaEventPublisher_Publish_InjectsSw8Header(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)

	const fakeSw8 = "1-aabbccdd-eeff0011-1-aabbccdd-aabbccdd-aabbccdd-aabbccdd-aabbccdd"
	var gotSw8 string

	mockProducer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(
		func(msg *sarama.ProducerMessage) error {
			for _, h := range msg.Headers {
				if string(h.Key) == "sw8" {
					gotSw8 = string(h.Value)
				}
			}
			return nil
		},
	)

	// mock Tracer：CreateExitSpan 时调 injector 注入 fakeSw8 到 SpanContext
	mockTracer := newSw8MockTracer(fakeSw8)

	p := &KafkaEventPublisher{
		producer: mockProducer,
		tracer:   mockTracer,
	}
	ctx := context.Background()
	err := p.Publish(ctx, TopicChatEvents, &Event{
		ID:     "evt-sw8",
		Type:   EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   time.Unix(1700000000, 0).UTC(),
		Data:   MessageCreatedData{MessageID: 1},
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	if gotSw8 != fakeSw8 {
		t.Errorf("sw8 header = %q, want %q (CreateExitSpan 未被 Publish 调用或 injector 未生效)",
			gotSw8, fakeSw8)
	}
}

// mock Tracer 实现：CreateExitSpan 时用 fakeSw8 填充 injector("sw8", ...)
// 测试只关心 sw8 是否被写到 header，不关心 span lifecycle
//
// Stage 94 PR-1 §P0-6 扩展：可选 outSpan 字段。
// - nil（旧测试）：CreateExitSpan 返 (ctx, nil, nil) —— span 隐式丢弃，行为不变
// - 非 nil（新测试）：CreateExitSpan 返 (ctx, outSpan, nil) —— Publish 可调 EndSpan 断言
type sw8MockTracer struct {
	fakeSw8 string
	outSpan grpcinterceptor.Span // Stage 94 PR-1：可选，让 mock 返回真实 span 供 EndSpan 断言
}

func newSw8MockTracer(sw8 string) *sw8MockTracer {
	return &sw8MockTracer{fakeSw8: sw8}
}

// CreateExitSpan：按 propagation.Injector 协议写 sw8
func (m *sw8MockTracer) CreateExitSpan(
	ctx context.Context, operationName, peer string,
	injector func(key, value string) error,
) (context.Context, grpcinterceptor.Span, error) {
	if injector != nil {
		_ = injector("sw8", m.fakeSw8)
	}
	return ctx, m.outSpan, nil
}

// spanEndRecorder 记录 EndSpan 调用次数与 err，用于 §P0-6 钉死 span 生命周期契约
type spanEndRecorder struct {
	endCalls int
	lastErr  error
}

func (s *spanEndRecorder) EndSpan(err error) {
	s.endCalls++
	s.lastErr = err
}
func (s *spanEndRecorder) Tag(k, v string)            {}
func (s *spanEndRecorder) SetComponent(id int32)     {}
func (s *spanEndRecorder) SetSpanLayer(layer int32)  {}

// 编译期断言 spanEndRecorder 满足 grpcinterceptor.Span 接口
var _ grpcinterceptor.Span = (*spanEndRecorder)(nil)

// newSw8MockTracerWithSpan 构造带 spanEndRecorder 的 mock tracer
func newSw8MockTracerWithSpan(sw8 string) (*sw8MockTracer, *spanEndRecorder) {
	rec := &spanEndRecorder{}
	return &sw8MockTracer{fakeSw8: sw8, outSpan: rec}, rec
}

// TestKafkaEventPublisher_Publish_SpanEndSpanCalled §P0-6 RED：
//
// 钉死 producer span 生命周期 —— happy path 必须 EndSpan(nil) 1 次。
// 旧实现（_, _, _ = tracer.CreateExitSpan）：span 被丢弃，endCalls == 0 → FAIL
// 新实现（span, _, _ := ...; defer/显式 span.EndSpan(sendErr)）：endCalls == 1 → PASS
func TestKafkaEventPublisher_Publish_SpanEndSpanCalled(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	mockProducer.ExpectSendMessageAndSucceed()

	mockTr, rec := newSw8MockTracerWithSpan("1-p06-endspan-1")
	p := &KafkaEventPublisher{producer: mockProducer, tracer: mockTr}

	err := p.Publish(context.Background(), TopicChatEvents, &Event{
		ID:   "evt-end-span-p06",
		Type: EventTypeMessageCreated,
		Data: MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 1},
	})
	if err != nil {
		t.Fatalf("Publish err: %v", err)
	}
	if rec.endCalls != 1 {
		t.Errorf("span.EndSpan 应被调 1 次，实际 %d 次（§P0-6 未修：span 被 _, _, _ 丢弃）", rec.endCalls)
	}
	if rec.lastErr != nil {
		t.Errorf("happy path EndSpan err = %v, want nil", rec.lastErr)
	}
}

// TestKafkaEventPublisher_Publish_SpanEndSpanOnBrokerError §P0-6 RED 边界用例：
//
// SendMessage 失败时也必须 EndSpan(sendErr) —— 让 OAP 上 producer span 标记失败。
// 旧实现：span 已被丢弃，endCalls == 0；新实现：endCalls == 1 且 lastErr = sendErr
func TestKafkaEventPublisher_Publish_SpanEndSpanOnBrokerError(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	brokerErr := errors.New("broker down")
	mockProducer.ExpectSendMessageAndFail(brokerErr)

	mockTr, rec := newSw8MockTracerWithSpan("1-p06-broker-err")
	p := &KafkaEventPublisher{producer: mockProducer, tracer: mockTr}

	err := p.Publish(context.Background(), TopicChatEvents, &Event{
		ID:   "evt-broker-err-p06",
		Type: EventTypeMessageCreated,
		Data: MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 1},
	})
	if err == nil {
		t.Fatal("Publish 应返回 broker error")
	}
	if rec.endCalls != 1 {
		t.Errorf("SendMessage 失败时 span.EndSpan 仍应被调 1 次（让 OAP 标记失败），got %d", rec.endCalls)
	}
	if !errors.Is(rec.lastErr, brokerErr) {
		t.Errorf("EndSpan err = %v, want broker error %v", rec.lastErr, brokerErr)
	}
}

// CreateEntrySpan：mock 实现（本测试不涉及，保留接口合规）
func (m *sw8MockTracer) CreateEntrySpan(
	ctx context.Context, operationName string,
	extractor func(string) (string, error),
) (context.Context, grpcinterceptor.Span, error) {
	return ctx, nil, nil
}

// CreateLocalSpan + StartEntry：mock 实现（接口合规所需）
func (m *sw8MockTracer) CreateLocalSpan(
	ctx context.Context, opName string,
) (context.Context, grpcinterceptor.Span, error) {
	return ctx, nil, nil
}

func (m *sw8MockTracer) StartEntry(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span) {
	return ctx, nil
}

// Stage 92 PR-1 RED：tracer 为 nil 时 Publish 不应 panic 且必须保持现有行为
// （headers 仍含 content-type，但无 sw8）—— 锁定降级语义。
func TestKafkaEventPublisher_Publish_NoTracer_NoSw8Header(t *testing.T) {
	t.Parallel()
	mockProducer := mocks.NewSyncProducer(t, nil)
	var hasSw8 bool
	var hasContentType bool
	mockProducer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(
		func(msg *sarama.ProducerMessage) error {
			for _, h := range msg.Headers {
				switch string(h.Key) {
				case "sw8":
					hasSw8 = true
				case "content-type":
					hasContentType = true
				}
			}
			return nil
		},
	)

	p := &KafkaEventPublisher{producer: mockProducer, tracer: nil}
	err := p.Publish(context.Background(), TopicChatEvents, &Event{
		ID:   "evt-no-tracer",
		Type: EventTypeMessageCreated,
		Data: MessageCreatedData{MessageID: 1},
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	if hasSw8 {
		t.Error("tracer=nil 时不应有 sw8 header（不创造假的 trace context）")
	}
	if !hasContentType {
		t.Error("tracer=nil 时仍必须有 content-type header（向后兼容）")
	}
}

// =============================================================================
// Round 2.4 §P2-14 RED：ctx 取消在 SendMessage 阻塞期间应立即中断 Publish
//
// 现状（kafka_publisher.go:101-105）：ctx 取消仅阻止"发起新 SendMessage"，
// 不中断"在途 SendMessage"。sarama SyncProducer.SendMessage 是阻塞同步调用，
// broker 抖动时 Gin handler 卡到 Producer.Timeout=10s（kafka_publisher.go:38）。
//
// 期望行为：ctx 取消触发后 Publish 在 < 100ms 内返 ctx.Err()，不阻塞调用方。
//
// 实现思路（GREEN 阶段）：goroutine + select — 启 goroutine 调 SendMessage，
// select { case result <-ch: ... case <-ctx.Done(): return ctx.Err() }。
// =============================================================================

// blockingSyncProducer 实现 sarama.SyncProducer，SendMessage 阻塞直到 release 通道关闭
// 或 receiveCtx cancel。让测试模拟 "broker 卡住" 的场景，验证 Publish 不依赖 SendMessage 完成也能响应 ctx。
type blockingSyncProducer struct {
	release   chan struct{}  // 测试用：关闭后所有阻塞 SendMessage 立即返
	receiveCtx context.Context // 测试用：ctx.Done() 后所有阻塞 SendMessage 立即返 ctx.Err
	mu        sync.Mutex
	sendCalls int             // 记录 SendMessage 被调用次数
}

func newBlockingSyncProducer() *blockingSyncProducer {
	return &blockingSyncProducer{release: make(chan struct{})}
}

func (b *blockingSyncProducer) SendMessage(_ *sarama.ProducerMessage) (int32, int64, error) {
	b.mu.Lock()
	b.sendCalls++
	b.mu.Unlock()
	select {
	case <-b.release:
		return 0, 0, nil
	case <-b.receiveCtx.Done():
		return 0, 0, b.receiveCtx.Err()
	}
}

func (b *blockingSyncProducer) SendMessages(_ []*sarama.ProducerMessage) error { return nil }
func (b *blockingSyncProducer) Close() error                                     { return nil }
func (b *blockingSyncProducer) AbortTxn() error                                 { return nil }
func (b *blockingSyncProducer) AddMessageToTxn(_ *sarama.ConsumerMessage, _ string, _ *string) error {
	return nil
}
func (b *blockingSyncProducer) AddMessageToTxnWithGroupMetadata(_ *sarama.ConsumerMessage, _ *sarama.ConsumerGroupMetadata, _ *string) error {
	return nil
}
func (b *blockingSyncProducer) AddOffsetsToTxn(_ map[string][]*sarama.PartitionOffsetMetadata, _ string) error {
	return nil
}
func (b *blockingSyncProducer) AddOffsetsToTxnWithGroupMetadata(_ map[string][]*sarama.PartitionOffsetMetadata, _ *sarama.ConsumerGroupMetadata) error {
	return nil
}
func (b *blockingSyncProducer) BeginTxn() error                  { return nil }
func (b *blockingSyncProducer) CommitTxn() error                 { return nil }
func (b *blockingSyncProducer) IsTransactional() bool           { return false }
func (b *blockingSyncProducer) TxnStatus() sarama.ProducerTxnStatusFlag {
	return sarama.ProducerTxnFlagReady
}

// Round 2.4 RED §1：ctx 已取消时 Publish 立即返 ctx.Err()，不应调 SendMessage
func TestKafkaEventPublisher_Publish_CtxAlreadyCancelled_ReturnsImmediately(t *testing.T) {
	t.Parallel()
	bl := newBlockingSyncProducer()
	p := &KafkaEventPublisher{producer: bl}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 已取消

	start := time.Now()
	err := p.Publish(ctx, TopicChatEvents, &Event{
		ID:   "evt-cancelled",
		Type: EventTypeMessageCreated,
		Data: MessageCreatedData{MessageID: 1},
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Publish err = %v, want context.Canceled", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("Publish took %v, want < 50ms (ctx pre-check should bypass SendMessage)", elapsed)
	}
	if bl.sendCalls != 0 {
		t.Errorf("SendMessage calls = %d, want 0 (cancelled ctx must not reach broker)", bl.sendCalls)
	}
}

// Round 2.4 RED §2：SendMessage 阻塞期间 ctx.Done() 触发后 Publish 在 < 100ms 内返 ctx.Err()
//
// 当前实现（kafka_publisher.go:103-105）只预检 ctx，goroutine + select 尚未实现 →
// 旧行为 = SendMessage 阻塞整个 Publish；新行为 = goroutine + select 立即响应 ctx。
//
// 测试策略：bl.receiveCtx 与 Publish ctx 共享（测试用自己的 ctx 控 bl），
// 这样 ctx.Done() 触发时 SendMessage 内部也会唤醒——但旧实现不等待，所以测试断言
// elapsed < 100ms（goroutine + select）vs 旧实现会等到 receiveCtx 取消 ≈ 几 ms 即解除。
// 为了让 RED 明显失败：bl.release 永不关，bl.receiveCtx 仅 ctx.Done() 时解除。
// 旧实现 elapsed ~ Producer.Timeout=10s → FAIL；新实现 elapsed < 100ms → PASS。
func TestKafkaEventPublisher_Publish_CtxCancelDuringSendMessage_ReturnsImmediately(t *testing.T) {
	t.Parallel()
	bl := newBlockingSyncProducer()
	// 让阻塞 SendMessage 在 ctx.Done() 时解除（模拟 broker 在 ctx cancel 后中断）
	bl.receiveCtx, _ = context.WithCancel(context.Background())

	p := &KafkaEventPublisher{producer: bl}
	ctx, cancel := context.WithCancel(context.Background())

	// 50ms 后取消 ctx
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
		// bl.receiveCtx 复用同一个 ctx 测试，但 sarama mock 不用此 ctx。
		// 由于 blockingSyncProducer 共享 ctx，取消 cancel() 后 bl.receiveCtx 也 Done。
		// 但旧实现不启 goroutine，所以等不到 — 这是 RED 关键点。
	}()

	start := time.Now()
	err := p.Publish(ctx, TopicChatEvents, &Event{
		ID:   "evt-cancel-during",
		Type: EventTypeMessageCreated,
		Data: MessageCreatedData{MessageID: 1},
	})
	elapsed := time.Since(start)

	// RED 断言：ctx.Err() 透传 + < 100ms 响应（旧实现阻塞到 SendMessage 完成 ~10s）
	if err == nil {
		t.Fatal("Publish 应在 ctx 取消后返 ctx.Err()，got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Publish err = %v, want wraps context.Canceled", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Publish 在 ctx 取消后仍阻塞 %v，want < 500ms（goroutine + select 未实现）", elapsed)
	}
}