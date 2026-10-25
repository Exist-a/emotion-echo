// internal/logic/memory_extract_handler_graph_test.go
//
// CL-08c · Phase 2 · RED：抽取流程的情节/实体持久化契约（假 LLM + 内存替身，
// 不依赖网络/DB；PG 侧语义由 integration_test 的真库测试钉死）。
//
// Phase 2 增量（相对 CL-08a 的事实抽取）：
//   - 每次成功抽取，除 facts 外还要落 1 条 memory_episode（图重建的原料）
//     + N 条 memory_entity（按 fact.Subject 去重，摘要 = 该主体最新事实）；
//   - T-4 幂等：同一条消息重复消费 ⇒ episode/entity 仍各 1 行；
//   - T-5 单测面：episode/entity 写失败 ⇒ 返回 error（交消费框架重试/DLQ，
//     不吞错；运行时 A/B「LLM 挂发消息仍成功」待 dev 栈窗口取证）；
//   - 零回归：未注入 graphRepo（nil）时行为与 CL-08a 完全一致。

package logic

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/memory"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingGraphRepo 按需让 UpsertEpisode / UpsertEntity 失败（其余透传内存替身）。
type failingGraphRepo struct {
	repository.MemoryGraphRepo
	failEpisode bool
	failEntity  bool
}

func (f *failingGraphRepo) UpsertEpisode(ctx context.Context, ep *model.MemoryEpisode) (bool, error) {
	if f.failEpisode {
		return false, errors.New("pg down: episode")
	}
	return f.MemoryGraphRepo.UpsertEpisode(ctx, ep)
}

func (f *failingGraphRepo) UpsertEntity(ctx context.Context, ent *model.MemoryEntity) (bool, error) {
	if f.failEntity {
		return false, errors.New("pg down: entity")
	}
	return f.MemoryGraphRepo.UpsertEntity(ctx, ent)
}

func TestMemoryExtractHandler_EpisodeEntity_Persisted(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	graph := repository.NewInMemoryMemoryGraphRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph))

	require.NoError(t, h.Handle(context.Background(), msgEvent(100, 7, 1, "user", "我是后端工程师")))

	// episode：1 行，content=原始消息，valid_at=事件时间（秒级时间戳还原）
	ep, err := graph.GetEpisode(context.Background(), model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	require.NotNil(t, ep, "成功抽取必须落 1 条 episode（图重建原料）")
	assert.Equal(t, "我是后端工程师", ep.Content)
	assert.Equal(t, int64(1), ep.UserID)
	assert.Equal(t, int64(7), ep.ConversationID)
	assert.True(t, ep.ValidAt.Equal(time.Unix(1790000000, 0).UTC()), "valid_at 必须取事件时间")
	assert.NotEmpty(t, ep.Name, "episode.name 必须非空（短标签）")

	// entity：fact.Subject 去重后 1 行，摘要 = 该主体最新事实文本
	ent, err := graph.GetEntityByName(context.Background(), 1, "用户")
	require.NoError(t, err)
	require.NotNil(t, ent, "抽取出的主体必须登记实体")
	assert.Equal(t, "用户职业后端工程师", ent.Summary)
}

func TestMemoryExtractHandler_EpisodeEntity_MultiSubjectAndLatestSummary(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	graph := repository.NewInMemoryMemoryGraphRepo()

	old := factOf("居住地", "北京")
	newer := factOf("居住地", "上海")
	newer.ValidFrom = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	mother := factOf("健康状况", "住院")
	mother.Subject = "母亲"
	mother.FactText = "母亲健康状况住院"

	ex := &fakeExtractor{facts: []model.MemoryFact{old, newer, mother}}
	h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph))

	require.NoError(t, h.Handle(context.Background(), msgEvent(200, 7, 1, "user", "搬家了，母亲住院")))

	// 两个主体 ⇒ 两条实体；同主体两条事实 ⇒ 1 条实体、摘要取 ValidFrom 最新的
	user, err := graph.GetEntityByName(context.Background(), 1, "用户")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "用户居住地上海", user.Summary, "同主体多事实 ⇒ 摘要取最新一条")

	mom, err := graph.GetEntityByName(context.Background(), 1, "母亲")
	require.NoError(t, err)
	require.NotNil(t, mom, "不同主体必须各自登记")
	assert.Equal(t, "母亲健康状况住院", mom.Summary)

	ents, err := graph.ListEntitiesByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Len(t, ents, 2)
}

// T-4：同一条消息重复消费 ⇒ episode/entity 各 1 行（at-least-once 投递）。
func TestMemoryExtractHandler_EpisodeEntity_Idempotent(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	graph := repository.NewInMemoryMemoryGraphRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph))

	evt := msgEvent(300, 7, 1, "user", "我是后端工程师")
	require.NoError(t, h.Handle(context.Background(), evt))
	require.NoError(t, h.Handle(context.Background(), evt))
	require.NoError(t, h.Handle(context.Background(), evt))

	eps, err := graph.ListEpisodesByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Len(t, eps, 1, "重复消费不得产生重复 episode")

	ents, err := graph.ListEntitiesByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Len(t, ents, 1, "重复消费不得产生重复 entity")

	assert.Equal(t, 1, ex.calls, "幂等门（TryMarkExtractRun）必须挡住重复 LLM 调用")
}

// T-5 单测面：episode/entity 写失败 ⇒ 返回 error 交给消费框架重试，且运行行回填 failed
//（failed 允许重投重抽 ⇒ 自愈）；绝不吞错。
func TestMemoryExtractHandler_GraphWriteFailure_PropagatesAndFailed(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	base := repository.NewInMemoryMemoryGraphRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}

	t.Run("episode write fails", func(t *testing.T) {
		graph := &failingGraphRepo{MemoryGraphRepo: base, failEpisode: true}
		h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph))
		err := h.Handle(context.Background(), msgEvent(400, 7, 1, "user", "内容"))
		require.Error(t, err, "episode 写失败必须向上传播（重试/DLQ）")

		run, rerr := repo.GetExtractRun(context.Background(), model.MemorySourceKindMessage, 400)
		require.NoError(t, rerr)
		require.NotNil(t, run)
		assert.Equal(t, model.MemoryExtractStatusFailed, run.Status, "失败必须回填 failed（允许重投重抽）")
	})

	t.Run("entity write fails", func(t *testing.T) {
		graph := &failingGraphRepo{MemoryGraphRepo: base, failEntity: true}
		h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph))
		err := h.Handle(context.Background(), msgEvent(401, 7, 1, "user", "内容"))
		require.Error(t, err, "entity 写失败必须向上传播（重试/DLQ）")
	})

	t.Run("retry after failure converges", func(t *testing.T) {
		graph := &failingGraphRepo{MemoryGraphRepo: base, failEpisode: true}
		h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph))
		require.Error(t, h.Handle(context.Background(), msgEvent(402, 7, 1, "user", "内容")))

		// 故障恢复：第二次消费走 failed 重试路径，全链成功
		graph.failEpisode = false
		require.NoError(t, h.Handle(context.Background(), msgEvent(402, 7, 1, "user", "内容")))
		ep, err := base.GetEpisode(context.Background(), model.MemorySourceKindMessage, 402)
		require.NoError(t, err)
		require.NotNil(t, ep, "恢复后重试必须补齐 episode")
	})
}

// 零回归：未注入 graphRepo（CL-08a 旧装配路径）⇒ 行为不变，不 panic。
func TestMemoryExtractHandler_NilGraphRepo_ZeroRegression(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex)

	require.NoError(t, h.Handle(context.Background(), msgEvent(500, 7, 1, "user", "我是后端工程师")))
	active, err := repo.ListActiveByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 1)
}

// 自述条（note 来源）也要落 episode（source_kind=note，conversation_id=0）。
func TestMemoryExtractHandler_NoteAlsoPersistsEpisode(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	graph := repository.NewInMemoryMemoryGraphRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "设计师")}}
	h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph))

	require.NoError(t, h.HandleNote(context.Background(), &model.MemoryNote{
		ID: 42, UserID: 1, Content: "我是设计师", CreatedAt: time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC),
	}))

	ep, err := graph.GetEpisode(context.Background(), model.MemorySourceKindNote, 42)
	require.NoError(t, err)
	require.NotNil(t, ep, "note 来源同样要落 episode")
	assert.Equal(t, "我是设计师", ep.Content)
	assert.Equal(t, int64(0), ep.ConversationID, "自述条不属于任何会话（D6）")
}

// 空内容的 note：episode 跳过（无可存档），facts 照常 —— 不因空内容把整个抽取打成 failed。
func TestMemoryExtractHandler_EmptyContentSkipsEpisode(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryMemoryFactRepo()
	graph := repository.NewInMemoryMemoryGraphRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "工程师")}}
	h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph))

	require.NoError(t, h.HandleNote(context.Background(), &model.MemoryNote{
		ID: 43, UserID: 1, Content: "   ", CreatedAt: time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC),
	}))

	ep, err := graph.GetEpisode(context.Background(), model.MemorySourceKindNote, 43)
	require.NoError(t, err)
	assert.Nil(t, ep, "空内容不落 episode")

	active, err := repo.ListActiveByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 1, "facts 照常落库")
}

// ---------------------------------------------------------------------------
// CL-08c · Phase 4：图写入端接线 —— 抽取成功后异步投递到 llm-service 图摄取，
// 失败只记日志（PG 是系统记录，图可重建；R9）。
// ---------------------------------------------------------------------------

// fakeEpisodeGraphWriter 记录投递（memory 包接口的确定性替身）。
type fakeEpisodeGraphWriter struct {
	calls []memory.Episode
	err   error
	mu    sync.Mutex
}

func (f *fakeEpisodeGraphWriter) UpsertEpisode(ctx context.Context, ep memory.Episode) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, ep)
	return "graph-ep-uuid", f.err
}

func (f *fakeEpisodeGraphWriter) DeleteUserData(ctx context.Context, userID int64) error {
	return nil
}

func (f *fakeEpisodeGraphWriter) RemoveEpisode(ctx context.Context, userID int64, uuid string) error {
	return nil
}

func (f *fakeEpisodeGraphWriter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeEpisodeGraphWriter) first() memory.Episode {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[0]
}

func TestMemoryExtractHandler_EpisodeForwardedToGraphWriter(t *testing.T) {
	repo := repository.NewInMemoryMemoryFactRepo()
	graph := repository.NewInMemoryMemoryGraphRepo()
	gw := &fakeEpisodeGraphWriter{}
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex, WithGraphRepo(graph), WithGraphWriter(gw))

	require.NoError(t, h.Handle(context.Background(), msgEvent(600, 7, 1, "user", "我是后端工程师")))

	// 异步投递：轮询至多 2s
	deadline := time.Now().Add(2 * time.Second)
	for gw.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	require.Equal(t, 1, gw.count(), "抽取成功后必须投递一条 episode 给图写入端")
	ep := gw.first()
	assert.Equal(t, int64(1), ep.UserID)
	assert.Equal(t, "我是后端工程师", ep.Content)
	assert.Equal(t, model.MemorySourceKindMessage, ep.SourceKind)
	assert.EqualValues(t, 600, ep.SourceID)
	assert.EqualValues(t, 1790000000000, ep.ValidAtUnixMs, "valid_at 毫秒（事件时间轴）")
}

// 图写失败绝不影响抽取结果（Handle 必须仍为 nil、facts 照常落库）。
func TestMemoryExtractHandler_GraphWriterFailureDoesNotFailExtraction(t *testing.T) {
	repo := repository.NewInMemoryMemoryFactRepo()
	gw := &fakeEpisodeGraphWriter{err: errors.New("llm-service graph down")}
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex, WithGraphWriter(gw))

	require.NoError(t, h.Handle(context.Background(), msgEvent(601, 7, 1, "user", "内容")))
	active, err := repo.ListActiveByUser(context.Background(), 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 1, "图写失败不影响事实落库（best-effort）")
}

// nil（未装配）零回归。
func TestMemoryExtractHandler_NilGraphWriter_NoPanic(t *testing.T) {
	repo := repository.NewInMemoryMemoryFactRepo()
	ex := &fakeExtractor{facts: []model.MemoryFact{factOf("职业", "后端工程师")}}
	h := NewMemoryExtractHandler(repo, ex, WithGraphWriter(nil))
	require.NoError(t, h.Handle(context.Background(), msgEvent(602, 7, 1, "user", "内容")))
}
