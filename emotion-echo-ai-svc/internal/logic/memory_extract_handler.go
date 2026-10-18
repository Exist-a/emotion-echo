// Package logic — memory_extract_handler.go
//
// CL-08a · A2：事实抽取处理器。
//
// 两个触发源（用户裁定 D2 / D6）：
//   - 正常会话：独立 consumer group "ai-svc-memory" 消费
//       · message.created（role=user）⇒ 逐条抽取
//       · conversation.closed           ⇒ 确定性合并/失效判定（不调 LLM）
//   - 「我的记忆」自述条：gRPC CreateNote 写入后异步调用 HandleNote（同一套抽取器）
//
// 失败语义：返回 error 交给调用方（消费框架重试/DLQ；自述条回填 failed 供重试）。
// 抽取是**异步增强**，绝不进入发消息关键路径（CL-D-33）。
package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"emotion-echo-ai-svc/internal/events"
	"emotion-echo-ai-svc/internal/memory"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
)

// MemoryExtractHandler 处理记忆抽取相关事件与自述条。
type MemoryExtractHandler struct {
	repo      repository.MemoryFactRepo
	extractor memory.Extractor
}

func NewMemoryExtractHandler(repo repository.MemoryFactRepo, ex memory.Extractor) *MemoryExtractHandler {
	return &MemoryExtractHandler{repo: repo, extractor: ex}
}

// extractSource 一个抽取来源的统一描述。
type extractSource struct {
	Kind           string
	ID             int64
	UserID         int64
	ConversationID int64
	Content        string
	OccurredAt     time.Time
}

// Handle 分发 Kafka 事件；未关心的事件类型直接忽略。
func (h *MemoryExtractHandler) Handle(ctx context.Context, evt *events.Event) error {
	if evt == nil {
		return nil
	}
	switch evt.Type {
	case events.EventTypeMessageCreated:
		return h.handleMessageCreated(ctx, evt)
	case events.EventTypeConversationClosed:
		return h.handleConversationClosed(ctx, evt)
	default:
		return nil
	}
}

// HandleNote 抽取一条「我的记忆」自述（由 gRPC CreateNote 异步调用）。
func (h *MemoryExtractHandler) HandleNote(ctx context.Context, note *model.MemoryNote) error {
	if note == nil {
		return nil
	}
	err := h.extractSource(ctx, extractSource{
		Kind:   model.MemorySourceKindNote,
		ID:     note.ID,
		UserID: note.UserID,
		// 自述条不属于任何会话（D6）⇒ conversation_id = 0
		Content:    note.Content,
		OccurredAt: note.CreatedAt,
	})
	status := model.MemoryExtractStatusDone
	if err != nil {
		status = model.MemoryExtractStatusFailed
	}
	if uerr := h.repo.UpdateNoteStatus(ctx, note.ID, status); uerr != nil {
		slog.WarnContext(ctx, "memory: update note status", "note_id", note.ID, "err", uerr)
	}
	return err
}

// handleMessageCreated 对一条 user 消息做事实抽取与落库。
func (h *MemoryExtractHandler) handleMessageCreated(ctx context.Context, evt *events.Event) error {
	var data events.MessageCreatedData
	if err := decodeEventData(evt, &data); err != nil {
		return err
	}
	if data.Role != "user" || strings.TrimSpace(data.Content) == "" {
		return nil
	}
	return h.extractSource(ctx, extractSource{
		Kind:           model.MemorySourceKindMessage,
		ID:             data.MessageID,
		UserID:         data.UserID,
		ConversationID: data.ConversationID,
		Content:        data.Content,
		OccurredAt:     occurredAt(data.CreatedAt),
	})
}

// handleConversationClosed 会话收尾：确定性合并（同一键只保留最新一条 active）。
func (h *MemoryExtractHandler) handleConversationClosed(ctx context.Context, evt *events.Event) error {
	var data events.ConversationClosedData
	if err := decodeEventData(evt, &data); err != nil {
		return err
	}
	if data.ConversationID <= 0 {
		return nil
	}

	facts, err := h.repo.ListActiveByConversation(ctx, data.ConversationID)
	if err != nil {
		return fmt.Errorf("memory: list conversation facts: %w", err)
	}
	plan := repository.PlanConversationConsolidation(facts)
	if len(plan.Supersede) == 0 {
		return nil
	}
	if _, err := h.repo.ApplyFacts(ctx, plan); err != nil {
		return fmt.Errorf("memory: consolidate conversation facts: %w", err)
	}
	slog.InfoContext(ctx, "memory: conversation consolidated",
		"conversation_id", data.ConversationID,
		"superseded", len(plan.Supersede))
	return nil
}

// extractSource 幂等抽取 + 落库（message / note 共用）。
func (h *MemoryExtractHandler) extractSource(ctx context.Context, src extractSource) error {
	first, err := h.repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind:     src.Kind,
		SourceID:       src.ID,
		UserID:         src.UserID,
		ConversationID: src.ConversationID,
	})
	if err != nil {
		return fmt.Errorf("memory: mark extract run: %w", err)
	}
	if !first {
		slog.DebugContext(ctx, "memory: source already extracted",
			"source_kind", src.Kind, "source_id", src.ID)
		return nil
	}

	facts, err := h.extractor.Extract(ctx, memory.ExtractInput{
		UserID:         src.UserID,
		ConversationID: src.ConversationID,
		MessageID:      src.ID, // 兼容提取器输入；note 来源时即 note id
		SourceKind:     src.Kind,
		SourceID:       src.ID,
		Content:        src.Content,
		OccurredAt:     src.OccurredAt,
	})
	if err != nil {
		h.finishFailed(ctx, src, err)
		return fmt.Errorf("memory: extract: %w", err)
	}

	existing, err := h.repo.ListActiveByUser(ctx, src.UserID, 0)
	if err != nil {
		h.finishFailed(ctx, src, err)
		return fmt.Errorf("memory: list active facts: %w", err)
	}
	plan := repository.PlanFactUpsert(existing, facts)
	written, err := h.repo.ApplyFacts(ctx, plan)
	if err != nil {
		h.finishFailed(ctx, src, err)
		return fmt.Errorf("memory: apply facts: %w", err)
	}
	if err := h.repo.FinishExtractRun(ctx, src.Kind, src.ID, model.MemoryExtractStatusDone, written, ""); err != nil {
		return fmt.Errorf("memory: finish extract run: %w", err)
	}

	slog.InfoContext(ctx, "memory: facts upserted",
		"source_kind", src.Kind,
		"source_id", src.ID,
		"user_id", src.UserID,
		"extracted", len(facts),
		"written", written,
		"superseded", len(plan.Supersede))
	return nil
}

// finishFailed 回填失败状态（best-effort：回填失败不掩盖原始错误）。
func (h *MemoryExtractHandler) finishFailed(ctx context.Context, src extractSource, cause error) {
	if err := h.repo.FinishExtractRun(ctx, src.Kind, src.ID, model.MemoryExtractStatusFailed, 0, cause.Error()); err != nil {
		slog.WarnContext(ctx, "memory: finish failed run", "source_kind", src.Kind, "source_id", src.ID, "err", err)
	}
}

// decodeEventData 把 evt.Data（any）重新编码后解码到目标结构（与 consumehandler 同款）。
func decodeEventData(evt *events.Event, dst any) error {
	raw, err := json.Marshal(evt.Data)
	if err != nil {
		return fmt.Errorf("memory: marshal event data: %w", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("memory: unmarshal event data: %w", err)
	}
	return nil
}

// occurredAt 事件时间（秒级时间戳）；0/负值回落 now（避免 1970 污染双时间轴）。
func occurredAt(createdAt int64) time.Time {
	if createdAt <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(createdAt, 0).UTC()
}
