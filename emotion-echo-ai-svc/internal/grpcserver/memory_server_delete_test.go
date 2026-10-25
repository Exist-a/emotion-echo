// CL-08d：DeleteMemory 编排的单测（InMemory repo + fake 图依赖）。
package grpcserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	emotionmemory "github.com/emotion-echo/shared/pkg/emotionmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGraphDeps struct {
	removeErr   error
	dropErr     error
	removed     []string
	droppedFor  []int64
	rebuildErr  error
	rebuildFor  []int64
	rebuildStat string
}

func (f *fakeGraphDeps) DeleteUserData(ctx context.Context, userID int64) error {
	if f.dropErr != nil {
		return f.dropErr
	}
	f.droppedFor = append(f.droppedFor, userID)
	return nil
}

func (f *fakeGraphDeps) RemoveEpisode(ctx context.Context, userID int64, uuid string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, uuid)
	return nil
}

func newDeleteTestServer(t *testing.T) (*memoryServer, *repository.InMemoryMemoryFactRepo, *fakeGraphDeps, *[]int64) {
	t.Helper()
	repo := repository.NewInMemoryMemoryFactRepo()
	graph := &fakeGraphDeps{}
	refreshed := &[]int64{}
	s := newMemoryServer(repo, func(ctx context.Context, note *model.MemoryNote) error { return nil })
	s.SetGraphDeps(&memoryGraphDeps{
		graph: graph,
		refresh: func(ctx context.Context, userID int64) error {
			*refreshed = append(*refreshed, userID)
			return nil
		},
		rebuild: func(ctx context.Context, userID int64) (int, int, error) {
			if graph.rebuildErr != nil {
				return 0, 0, graph.rebuildErr
			}
			graph.rebuildFor = append(graph.rebuildFor, userID)
			return 3, 0, nil
		},
	})
	return s, repo, graph, refreshed
}

func TestDeleteMemory_Fact_RemovesPGAndGraph(t *testing.T) {
	s, repo, graph, refreshed := newDeleteTestServer(t)
	ctx := authCtx(1)

	// 造一条消息来源的事实 + 情节（uuid 已知）
	mustApply(t, repo, repository.PlanFactUpsert(nil, []model.MemoryFact{{
		UserID: 1, ConversationID: 7, SourceKind: "message", SourceID: 100,
		Subject: "用户", Predicate: "居住地", Object: "杭州", FactText: "用户居住地杭州",
		ValidFrom: validFromFixture(),
	}}))
	require.NoError(t, repo.UpsertEpisodeForTest(ctx, &model.MemoryEpisode{
		UserID: 1, ConversationID: 7, SourceKind: "message", SourceID: 100,
		Name: "n", Content: "c", ValidAt: validFromFixture(), GraphUUID: "ep-uuid-1",
	}))

	resp, err := s.DeleteMemory(ctx, &emotionmemory.DeleteMemoryRequest{UserId: 1, Scope: "fact", Id: 1})
	require.NoError(t, err)
	require.True(t, resp.Ok)

	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 0, "PG 侧必须已软删")
	assert.Equal(t, []string{"ep-uuid-1"}, graph.removed, "图侧必须按 uuid 摘除")
	assert.Equal(t, []int64{1}, *refreshed, "摘要必须刷新")
}

func TestDeleteMemory_Fact_UUIDMissing_FallsBackToRebuild(t *testing.T) {
	s, repo, graph, _ := newDeleteTestServer(t)
	ctx := authCtx(1)
	mustApply(t, repo, repository.PlanFactUpsert(nil, []model.MemoryFact{{
		UserID: 1, ConversationID: 7, SourceKind: "message", SourceID: 101,
		Subject: "用户", Predicate: "职业", Object: "木工", FactText: "用户职业木工",
		ValidFrom: validFromFixture(),
	}}))
	// 情节存在但无 uuid（旧行）

	resp, err := s.DeleteMemory(ctx, &emotionmemory.DeleteMemoryRequest{UserId: 1, Scope: "fact", Id: 1})
	require.NoError(t, err)
	require.True(t, resp.Ok)
	assert.Equal(t, []int64{1}, graph.rebuildFor, "uuid 缺失必须回落全图重建")
	assert.Empty(t, graph.removed)
}

func TestDeleteMemory_Note_Cascades(t *testing.T) {
	s, repo, graph, _ := newDeleteTestServer(t)
	ctx := authCtx(1)
	note := &model.MemoryNote{UserID: 1, Content: "我在学萨克斯"}
	require.NoError(t, repo.CreateNote(ctx, note))
	mustApply(t, repo, repository.PlanFactUpsert(nil, []model.MemoryFact{{
		UserID: 1, ConversationID: 0, SourceKind: "note", SourceID: note.ID,
		Subject: "用户", Predicate: "爱好", Object: "萨克斯", FactText: "用户爱好萨克斯",
		ValidFrom: validFromFixture(),
	}}))
	require.NoError(t, repo.UpsertEpisodeForTest(ctx, &model.MemoryEpisode{
		UserID: 1, SourceKind: "note", SourceID: note.ID,
		Name: "n", Content: "c", ValidAt: validFromFixture(), GraphUUID: "ep-note-1",
	}))

	resp, err := s.DeleteMemory(ctx, &emotionmemory.DeleteMemoryRequest{UserId: 1, Scope: "note", Id: note.ID})
	require.NoError(t, err)
	require.True(t, resp.Ok)

	notes, err := repo.ListNotes(ctx, 1, 0)
	require.NoError(t, err)
	assert.Len(t, notes, 0, "自述必须已删")
	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 0, "自述来源的事实必须级联软删")
	assert.Equal(t, []string{"ep-note-1"}, graph.removed)
}

func TestDeleteMemory_All_UsesFastPath(t *testing.T) {
	s, repo, graph, refreshed := newDeleteTestServer(t)
	ctx := authCtx(1)
	mustApply(t, repo, repository.PlanFactUpsert(nil, []model.MemoryFact{{
		UserID: 1, ConversationID: 7, SourceKind: "message", SourceID: 100,
		Subject: "用户", Predicate: "居住地", Object: "杭州", FactText: "用户居住地杭州",
		ValidFrom: validFromFixture(),
	}}))

	resp, err := s.DeleteMemory(ctx, &emotionmemory.DeleteMemoryRequest{UserId: 1, Scope: "all"})
	require.NoError(t, err)
	require.True(t, resp.Ok)
	assert.Equal(t, []int64{1}, graph.droppedFor, "删全部必须走物理分图快删")
	assert.Empty(t, graph.removed)
	assert.Equal(t, []int64{1}, *refreshed)
	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 0)
}

func TestDeleteMemory_GraphFailure_StillDeletes(t *testing.T) {
	s, repo, _, _ := newDeleteTestServer(t)
	ctx := authCtx(1)
	s.graphDeps.graph = &fakeGraphDeps{removeErr: errors.New("graph down")}
	mustApply(t, repo, repository.PlanFactUpsert(nil, []model.MemoryFact{{
		UserID: 1, ConversationID: 7, SourceKind: "message", SourceID: 100,
		Subject: "用户", Predicate: "居住地", Object: "杭州", FactText: "用户居住地杭州",
		ValidFrom: validFromFixture(),
	}}))
	require.NoError(t, repo.UpsertEpisodeForTest(ctx, &model.MemoryEpisode{
		UserID: 1, ConversationID: 7, SourceKind: "message", SourceID: 100,
		Name: "n", Content: "c", ValidAt: validFromFixture(), GraphUUID: "ep-uuid-9",
	}))

	resp, err := s.DeleteMemory(ctx, &emotionmemory.DeleteMemoryRequest{UserId: 1, Scope: "fact", Id: 1})
	require.NoError(t, err, "图失败不得让删除失败（PG 为事实源；缺口由重建工具兜底）")
	require.True(t, resp.Ok)
	assert.Contains(t, resp.Message, "rebuild pending", "必须如实标注图侧未完成")
	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 0, "PG 侧照常删掉")
}

func TestDeleteMemory_InvalidScope(t *testing.T) {
	s, _, _, _ := newDeleteTestServer(t)
	_, err := s.DeleteMemory(authCtx(1), &emotionmemory.DeleteMemoryRequest{UserId: 1, Scope: "bogus"})
	require.Error(t, err)
}

func mustApply(t *testing.T, repo repository.MemoryFactRepo, plan repository.PlanResult) {
	t.Helper()
	if _, err := repo.ApplyFacts(context.Background(), plan); err != nil {
		t.Fatalf("apply facts: %v", err)
	}
}

func validFromFixture() time.Time {
	return time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
}
