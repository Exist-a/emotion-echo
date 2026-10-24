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
	"sort"
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

	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
)

// MemoryExtractHandler 处理记忆抽取相关事件与自述条。
type MemoryExtractHandler struct {
	repo      repository.MemoryFactRepo
	extractor memory.Extractor
	// summaryStore 可选：写侧摘要（nil = 不写，单测/未配 Redis 时）。
	summaryStore  memory.SummaryStore
	summaryBudget int
	// graphRepo 可选（CL-08c Phase 2）：情节/实体持久化（nil = CL-08a 行为，零回归）。
	graphRepo repository.MemoryGraphRepo
}

// HandlerOption 可选依赖。
type HandlerOption func(*MemoryExtractHandler)

// WithSummaryStore 注入摘要写侧（best-effort）。
func WithSummaryStore(store memory.SummaryStore, budgetTokens int) HandlerOption {
	return func(h *MemoryExtractHandler) {
		h.summaryStore = store
		if budgetTokens > 0 {
			h.summaryBudget = budgetTokens
		}
	}
}

// WithGraphRepo 注入情节/实体仓储（CL-08c Phase 2）。
// 写失败向上传播（交消费框架重试/DLQ）；nil = 不落 episode/entity。
func WithGraphRepo(repo repository.MemoryGraphRepo) HandlerOption {
	return func(h *MemoryExtractHandler) { h.graphRepo = repo }
}

func NewMemoryExtractHandler(repo repository.MemoryFactRepo, ex memory.Extractor, opts ...HandlerOption) *MemoryExtractHandler {
	h := &MemoryExtractHandler{repo: repo, extractor: ex, summaryBudget: memory.DefaultSummaryBudgetTokens}
	for _, opt := range opts {
		opt(h)
	}
	return h
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
	started := time.Now()
	first, err := h.repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind:     src.Kind,
		SourceID:       src.ID,
		UserID:         src.UserID,
		ConversationID: src.ConversationID,
	})
	if err != nil {
		sharedmetrics.MemoryExtractTotal.WithLabelValues(sharedmetrics.MemoryResultError).Inc()
		return fmt.Errorf("memory: mark extract run: %w", err)
	}
	if !first {
		sharedmetrics.MemoryExtractTotal.WithLabelValues(sharedmetrics.MemoryResultSkipped).Inc()
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
		sharedmetrics.MemoryExtractTotal.WithLabelValues(sharedmetrics.MemoryResultError).Inc()
		sharedmetrics.MemoryExtractDurationSeconds.WithLabelValues(sharedmetrics.MemoryResultError).
			Observe(time.Since(started).Seconds())
		return fmt.Errorf("memory: extract: %w", err)
	}

	existing, err := h.repo.ListActiveByUser(ctx, src.UserID, 0)
	if err != nil {
		h.finishFailed(ctx, src, err)
		sharedmetrics.MemoryExtractTotal.WithLabelValues(sharedmetrics.MemoryResultError).Inc()
		sharedmetrics.MemoryExtractDurationSeconds.WithLabelValues(sharedmetrics.MemoryResultError).
			Observe(time.Since(started).Seconds())
		return fmt.Errorf("memory: list active facts: %w", err)
	}
	plan := repository.PlanFactUpsert(existing, facts)
	written, err := h.repo.ApplyFacts(ctx, plan)
	if err != nil {
		h.finishFailed(ctx, src, err)
		sharedmetrics.MemoryExtractTotal.WithLabelValues(sharedmetrics.MemoryResultError).Inc()
		sharedmetrics.MemoryExtractDurationSeconds.WithLabelValues(sharedmetrics.MemoryResultError).
			Observe(time.Since(started).Seconds())
		return fmt.Errorf("memory: apply facts: %w", err)
	}

	// CL-08c Phase 2：情节/实体持久化。必须在 FinishExtractRun(done) **之前**——
	// 一旦标 done，该来源永久跳过，episode 就再也补不上了；放前面则失败 ⇒ failed ⇒ 重投重抽，
	// facts 侧 ApplyFacts 幂等、episode/entity 侧唯一索引幂等 ⇒ 重试收敛、不重复。
	if err := h.persistGraphArtifacts(ctx, src, facts); err != nil {
		h.finishFailed(ctx, src, err)
		sharedmetrics.MemoryExtractTotal.WithLabelValues(sharedmetrics.MemoryResultError).Inc()
		sharedmetrics.MemoryExtractDurationSeconds.WithLabelValues(sharedmetrics.MemoryResultError).
			Observe(time.Since(started).Seconds())
		return fmt.Errorf("memory: persist episode/entities: %w", err)
	}

	if err := h.repo.FinishExtractRun(ctx, src.Kind, src.ID, model.MemoryExtractStatusDone, written, ""); err != nil {
		// 收尾回填失败时**不能**把运行行留在 pending：
		// pending 会被 TryMarkExtractRun 的幂等判据当成「已处理」而永久跳过（该来源再也不抽取）。
		// 这里 best-effort 落 failed（failed 允许重投重抽），再向上返回错误让消费框架重试。
		// 事实已提交且 ApplyFacts 幂等 ⇒ 重抽不会产生重复行，最坏只多一次 LLM 调用。
		h.finishFailed(ctx, src, err)
		sharedmetrics.MemoryExtractTotal.WithLabelValues(sharedmetrics.MemoryResultError).Inc()
		sharedmetrics.MemoryExtractDurationSeconds.WithLabelValues(sharedmetrics.MemoryResultError).
			Observe(time.Since(started).Seconds())
		return fmt.Errorf("memory: finish extract run: %w", err)
	}

	sharedmetrics.MemoryExtractTotal.WithLabelValues(sharedmetrics.MemoryResultOK).Inc()
	sharedmetrics.MemoryExtractDurationSeconds.WithLabelValues(sharedmetrics.MemoryResultOK).
		Observe(time.Since(started).Seconds())
	if written > 0 {
		sharedmetrics.MemoryFactsWrittenTotal.Add(float64(written))
	}

	slog.InfoContext(ctx, "memory: facts upserted",
		"source_kind", src.Kind,
		"source_id", src.ID,
		"user_id", src.UserID,
		"extracted", len(facts),
		"written", written,
		"superseded", len(plan.Supersede))

	// A3：重建用户级摘要（best-effort —— 失败不影响已提交的事实）。
	h.refreshSummary(ctx, src.UserID)
	return nil
}

// persistGraphArtifacts 落情节与实体（CL-08c Phase 2）。
//
//   - episode：content = 来源原文（「从 PG 重建图」的原料，ADR 决策 8）；
//     valid_at = 事件发生时间；name = 原文短标签。空内容跳过（无可存档）。
//   - entity：facts 按 Subject 去重，一个主体一行；summary 取该主体 ValidFrom
//     最新的 fact_text（"我妈"/"母亲"的合并由 LLM 消解负责，这里只保证不重复落行）。
//
// 任一写失败向上传播（⇒ failed ⇒ 重投重抽；两侧唯一索引保证重试收敛）。
func (h *MemoryExtractHandler) persistGraphArtifacts(ctx context.Context, src extractSource, facts []model.MemoryFact) error {
	if h.graphRepo == nil {
		return nil
	}
	if content := strings.TrimSpace(src.Content); content != "" {
		if _, err := h.graphRepo.UpsertEpisode(ctx, &model.MemoryEpisode{
			UserID:         src.UserID,
			ConversationID: src.ConversationID,
			SourceKind:     src.Kind,
			SourceID:       src.ID,
			Name:           episodeShortLabel(src.Kind, src.ID, content),
			Content:        content,
			ValidAt:        src.OccurredAt,
		}); err != nil {
			return err
		}
	}
	for _, ent := range entitiesFromFacts(src.UserID, facts) {
		if _, err := h.graphRepo.UpsertEntity(ctx, ent); err != nil {
			return err
		}
	}
	return nil
}

// episodeShortLabel 情节短标签：原文前 24 个字符；不足以 kind-id 兜底。
func episodeShortLabel(kind string, id int64, content string) string {
	runes := []rune(content)
	if len(runes) > 24 {
		runes = runes[:24]
	}
	if label := strings.TrimSpace(string(runes)); label != "" {
		return label
	}
	return fmt.Sprintf("%s-%d", kind, id)
}

// entitiesFromFacts 把本批 facts 折成实体行（每主体一行，摘要 = 最新事实）。
func entitiesFromFacts(userID int64, facts []model.MemoryFact) []*model.MemoryEntity {
	latest := make(map[string]model.MemoryFact, len(facts))
	for _, f := range facts {
		name := strings.TrimSpace(f.Subject)
		if name == "" {
			continue
		}
		if cur, ok := latest[name]; !ok || f.ValidFrom.After(cur.ValidFrom) {
			latest[name] = f
		}
	}
	// map 迭代无序 ⇒ 排序保证实体写入顺序确定（同批重放结果一致）
	names := make([]string, 0, len(latest))
	for n := range latest {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*model.MemoryEntity, 0, len(names))
	for _, n := range names {
		out = append(out, &model.MemoryEntity{UserID: userID, Name: n, Summary: latest[n].FactText})
	}
	return out
}

// refreshSummary 重建 Redis 用户级摘要。任何失败只记日志，不向上返回
// （记忆注入是增强不是依赖；下一次抽取会重试重建）。
func (h *MemoryExtractHandler) refreshSummary(ctx context.Context, userID int64) {
	if h.summaryStore == nil {
		return
	}
	facts, err := h.repo.ListActiveByUser(ctx, userID, 200)
	if err != nil {
		sharedmetrics.MemorySummaryBuildTotal.WithLabelValues(sharedmetrics.MemoryResultError).Inc()
		slog.WarnContext(ctx, "memory: load facts for summary", "user_id", userID, "err", err)
		return
	}
	summary := memory.BuildSummary(facts, h.summaryBudget)
	if err := h.summaryStore.Set(ctx, userID, summary); err != nil {
		sharedmetrics.MemorySummaryBuildTotal.WithLabelValues(sharedmetrics.MemoryResultError).Inc()
		slog.WarnContext(ctx, "memory: write summary", "user_id", userID, "err", err)
		return
	}
	sharedmetrics.MemorySummaryBuildTotal.WithLabelValues(sharedmetrics.MemoryResultOK).Inc()
	slog.DebugContext(ctx, "memory: summary refreshed", "user_id", userID, "facts", len(facts), "chars", len(summary))
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
// occurredAt 把事件里的 createdAt 还原成时间。
//
// ⚠️ 单位不统一是**跨服务契约的历史事实**（2026-10-21 收口轮实测出来的）：
// chat-svc 发的是 **UnixMilli**（`sendmessagelogic.go:116`、`proto_marshal.go:31`），
// 而这里历史上按**秒**解析 ⇒ 事实 valid_from 落到 58773 年，
// 双时间轴排序全错，摘要直接把「58773-03 用户是一名木工」写进 prompt。
//
// 判据：秒级时间戳在 2286 年才达到 1e10，毫秒级现在就是 1.7e12 ⇒
// 用 1e11 做分界（≈ 5138 年的秒数），两种单位都能正确还原，且对未来几个世纪安全。
func occurredAt(createdAt int64) time.Time {
	if createdAt <= 0 {
		return time.Now().UTC()
	}
	if createdAt >= epochMillisThreshold {
		return time.UnixMilli(createdAt).UTC()
	}
	return time.Unix(createdAt, 0).UTC()
}

// epochMillisThreshold 秒 / 毫秒时间戳的分界（1e11 秒 ≈ 5138 年；毫秒级现在已是 1e12 量级）。
const epochMillisThreshold = int64(100_000_000_000)
