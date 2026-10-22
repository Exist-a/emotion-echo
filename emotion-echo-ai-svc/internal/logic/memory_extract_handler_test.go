// internal/logic/memory_extract_handler_test.go
//
// CL-08a · A2：抽取处理器行为契约（假 LLM，不依赖网络/DB）。

package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/events"
	"emotion-echo-ai-svc/internal/memory"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeExtractor struct {
	facts []model.MemoryFact
	err   error
	calls int
}

func (f *fakeExtractor) Extract(ctx context.Context, in memory.ExtractInput) ([]model.MemoryFact, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make([]model.MemoryFact, 0, len(f.facts))
	for _, m := range f.facts {
		m.UserID = in.UserID
		m.ConversationID = in.ConversationID
		m.SourceKind = in.SourceKind
		m.SourceID = in.SourceID
		out = append(out, m)
	}
	return out, nil
}

func factOf(predicate, object string) model.MemoryFact {
	return model.MemoryFact{
		Subject:   "用户",
		Predicate: predicate,
		Object:    object,
		FactText:  "用户" + predicate + object,
		ValidFrom: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC),
		Status:    model.MemoryFactStatusActive,
	}
}

func msgEvent(messageID, conversationID, userID int64, role, content string) *events.Event {
	return &events.Event{
		ID:   "evt",
		Type: events.EventTypeMessageCreated,
		Data: events.MessageCreatedData{
			MessageID: messageID, ConversationID: conversationID, UserID: userID,
			Role: role, Content: content, CreatedAt: 1790000000,
		},
	}
}

func closedEvent(conversationID, userID int64) *events.Event {
	return &events.Event{
		ID:   "evt-closed",
		Type: events.EventTypeConversationClosed,
		Data: events.ConversationClosedData{ConversationID: conversationID, UserID: userID},
	}
}

func TestMemoryExtractHandler_MessageCreated_HappyPath(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex)

	require.NoError(t, h.Handle(context.Background(), msgEvent(100, 7, 1, "user", "我是后端工程师")))

	active, err := repo.ListActiveByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "后端工程师", active[0].Object)
	assert.Equal(t, model.MemorySourceKindMessage, active[0].SourceKind)
	assert.Equal(t, int64(100), active[0].SourceID)

	run, err := repo.GetExtractRun(context.Background(), model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, model.MemoryExtractStatusDone, run.Status)
	assert.Equal(t, 1, run.FactsCount)
}

func TestMemoryExtractHandler_MessageCreated_Idempotent(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex)
	evt := msgEvent(100, 7, 1, "user", "我是后端工程师")

	require.NoError(t, h.Handle(context.Background(), evt))
	require.NoError(t, h.Handle(context.Background(), evt))

	assert.Equal(t, 1, ex.calls, "同一条消息重复消费不得重复调 LLM")
	active, err := repo.ListActiveByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

func TestMemoryExtractHandler_SkipsNonUserRole(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex)

	require.NoError(t, h.Handle(context.Background(), msgEvent(100, 7, 1, "assistant", "我是助手")))
	assert.Equal(t, 0, ex.calls)
}

func TestMemoryExtractHandler_ExtractError_Retryable(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{err: errors.New("llm down")}
	h := NewMemoryExtractHandler(repo, ex)
	evt := msgEvent(100, 7, 1, "user", "我是后端工程师")

	err := h.Handle(context.Background(), evt)
	require.Error(t, err, "抽取失败必须返回 error（交给消费重试/DLQ）")

	run, gerr := repo.GetExtractRun(context.Background(), model.MemorySourceKindMessage, 100)
	require.NoError(t, gerr)
	require.NotNil(t, run)
	assert.Equal(t, model.MemoryExtractStatusFailed, run.Status)

	// 恢复后重投：failed 允许重试
	ex.err = nil
	ex.facts = []model.MemoryFact{factOf("职业", "后端工程师")}
	require.NoError(t, h.Handle(context.Background(), evt))
	assert.Equal(t, 2, ex.calls)
	active, err := repo.ListActiveByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

func TestMemoryExtractHandler_SupersedesPreviousFact(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("居住地", "北京")}}
	h := NewMemoryExtractHandler(repo, ex)
	require.NoError(t, h.Handle(context.Background(), msgEvent(100, 7, 1, "user", "我住在北京")))

	next := factOf("居住地", "上海")
	next.ValidFrom = next.ValidFrom.Add(14 * 24 * time.Hour)
	ex.facts = []model.MemoryFact{next}
	require.NoError(t, h.Handle(context.Background(), msgEvent(200, 7, 1, "user", "上个月搬到上海")))

	active, err := repo.ListActiveByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "上海", active[0].Object)

	all, err := repo.ListAllByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Len(t, all, 2, "旧事实不删除（T-15 可追溯）")
}

func TestMemoryExtractHandler_ConversationClosed_Consolidates(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	// 人为制造同一键两条 active（并发/重放的异常状态）
	a := factOf("居住地", "北京")
	a.UserID = 1
	a.ConversationID = 7
	b := factOf("居住地", "上海")
	b.UserID = 1
	b.ConversationID = 7
	b.ValidFrom = b.ValidFrom.Add(time.Hour)
	_, err := repo.ApplyFacts(ctx, repository.PlanResult{Insert: []model.MemoryFact{a, b}})
	require.NoError(t, err)

	h := NewMemoryExtractHandler(repo, &fakeExtractor{})
	require.NoError(t, h.Handle(ctx, closedEvent(7, 1)))

	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 1, "会话收尾后同一键只保留最新一条")
	assert.Equal(t, "上海", active[0].Object)
}

func TestMemoryExtractHandler_HandleNote(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("经历", "独自去过西藏")}}
	h := NewMemoryExtractHandler(repo, ex)
	ctx := context.Background()

	note := &model.MemoryNote{UserID: 3, Content: "前年我一个人去了西藏"}
	require.NoError(t, repo.CreateNote(ctx, note))
	require.NotZero(t, note.ID)

	require.NoError(t, h.HandleNote(ctx, note))

	active, err := repo.ListActiveByUser(ctx, 3, 0)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, model.MemorySourceKindNote, active[0].SourceKind)
	assert.Equal(t, note.ID, active[0].SourceID)
	assert.Equal(t, int64(0), active[0].ConversationID, "自述条不属于任何会话")

	stored, err := repo.GetNote(ctx, note.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, model.MemoryExtractStatusDone, stored.ExtractStatus)
}

func TestMemoryExtractHandler_RefreshesSummary(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	store := memory.NewInMemorySummaryStore()
	h := NewMemoryExtractHandler(repo, ex, WithSummaryStore(store, 600))

	require.NoError(t, h.Handle(context.Background(), msgEvent(100, 7, 1, "user", "我是后端工程师")))

	got, ok := store.Get(1)
	require.True(t, ok, "抽取成功后必须重建用户级摘要")
	assert.Contains(t, got, "后端工程师")
}

func TestMemoryExtractHandler_SummaryFailureDoesNotFailExtraction(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	store := memory.NewInMemorySummaryStore()
	store.Err = errors.New("redis down")
	h := NewMemoryExtractHandler(repo, ex, WithSummaryStore(store, 600))

	require.NoError(t, h.Handle(context.Background(), msgEvent(100, 7, 1, "user", "我是后端工程师")),
		"摘要写失败不得让抽取失败（增强不是依赖）")

	active, err := repo.ListActiveByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 1, "事实仍须落库")
}

func TestMemoryExtractHandler_MetricsEmitted(t *testing.T) {
	t.Parallel()
	before, err := sharedmetrics.RegistryGatherCounter("emotion_echo_memory_extract_total", map[string]string{"result": sharedmetrics.MemoryResultOK})
	require.NoError(t, err)

	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex)
	require.NoError(t, h.Handle(context.Background(), msgEvent(100, 7, 1, "user", "我是后端工程师")))

	after, err := sharedmetrics.RegistryGatherCounter("emotion_echo_memory_extract_total", map[string]string{"result": sharedmetrics.MemoryResultOK})
	require.NoError(t, err)
	assert.Greater(t, after, before, "T-18：抽取成功必须留下指标（否则面板永远为空）")
}

func TestMemoryExtractHandler_HandleNote_FailureMarksNote(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{err: errors.New("llm down")}
	h := NewMemoryExtractHandler(repo, ex)
	ctx := context.Background()

	note := &model.MemoryNote{UserID: 3, Content: "x"}
	require.NoError(t, repo.CreateNote(ctx, note))
	require.Error(t, h.HandleNote(ctx, note))

	stored, err := repo.GetNote(ctx, note.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, model.MemoryExtractStatusFailed, stored.ExtractStatus)
}

// flakyFinishRepo：落 done 时回填失败（模拟审计表/DB 抖动），其余透传内存实现。
type flakyFinishRepo struct {
	*repository.InMemoryMemoryFactRepo
	failDone bool
}

func (f *flakyFinishRepo) FinishExtractRun(ctx context.Context, sourceKind string, sourceID int64, status string, factsCount int, errMsg string) error {
	if f.failDone && status == model.MemoryExtractStatusDone {
		return errors.New("audit table unavailable")
	}
	return f.InMemoryMemoryFactRepo.FinishExtractRun(ctx, sourceKind, sourceID, status, factsCount, errMsg)
}

// CL-08a 残留修复：收尾回填 done 失败时，运行行若留在 pending，
// 重投会被 TryMarkExtractRun 的幂等判据永久跳过 ⇒ 该来源的事实再也不会更新。
func TestMemoryExtractHandler_FinishDoneFailure_LeavesRetryableState(t *testing.T) {
	t.Parallel()
	base := repository.NewInMemoryMemoryFactRepo()
	repo := &flakyFinishRepo{InMemoryMemoryFactRepo: base, failDone: true}
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex)
	ctx := context.Background()
	evt := msgEvent(100, 7, 1, "user", "我是后端工程师")

	require.Error(t, h.Handle(ctx, evt), "收尾回填失败必须向上返回错误（触发消费框架重投）")

	run, err := base.GetExtractRun(ctx, model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, model.MemoryExtractStatusFailed, run.Status,
		"不得把运行行留在 pending（pending 会被幂等判据当成「已处理」，该来源永久不再抽取）")

	// 回填恢复后重投：必须真的再抽一次（而不是被 first=false 跳过）
	repo.failDone = false
	require.NoError(t, h.Handle(ctx, evt))
	assert.Equal(t, 2, ex.calls, "恢复后重投必须重新抽取")
	run, err = base.GetExtractRun(ctx, model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	assert.Equal(t, model.MemoryExtractStatusDone, run.Status)
}

// 2026-10-21 收口轮实测：chat-svc 发的事件 createdAt 是**毫秒**
// （`sendmessagelogic.go:116` / `proto_marshal.go:31` 均为 `UnixMilli()`），
// 而 ai-svc 的 occurredAt 按**秒**解析 ⇒ 事实 valid_from 落到 58773 年，
// 双时间轴排序全错、摘要里直接写出「58773-03 用户是一名木工」喂给模型。
func TestOccurredAt_AcceptsMillisecondsFromEvent(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 10, 21, 6, 30, 0, 0, time.UTC)

	gotMs := occurredAt(want.UnixMilli())
	assert.True(t, gotMs.Equal(want), "毫秒时间戳必须被正确解析，实得 %s（want %s）", gotMs, want)

	gotS := occurredAt(want.Unix())
	assert.True(t, gotS.Equal(want), "秒级时间戳仍需兼容，实得 %s", gotS)
}
