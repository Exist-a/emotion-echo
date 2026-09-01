// Package fusion — Stage 34 · PR-13 RED
//
// FusionWorker 是后台 tick 调度器，每 5s 扫描"该融合但还没融合"的消息。
//
// 设计：
//   - 每轮 tick 调 FusedEmotionRepo.ListPending 拿候选 messageID 列表
//   - 对每个 candidate，拼 ModalitySnapshot（来自 text/face/voice 三个 repo）
//   - 调 LLMFuser，失败 fallback 到 WeightedLateFuser
//   - 调 FusedEmotionRepo.Upsert 写库（UNIQUE 幂等）
//
// 依赖反转（AGENTS.md §三.1）：
//   - FusedEmotionRepo / EmotionRepo / FaceEmotionRepo / VoiceEmotionRepo 都通过 interface 注入
//   - Fuser 接口（LLMFuser + WeightedLateFuser 都实现）
//   - Clock interface（避免 time.Sleep）—— 当前 PR 用 real clock，未来可注入 fake
//
// 测试用 fake repo + fake clock，不依赖真实 DB。
package fusion

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEmotionRepo 实现必要的 EmotionRepo 子集（仅 GetByMessageID）。
type fakeEmotionRepo struct {
	mu   sync.Mutex
	data map[int64]*model.EmotionAnalysis
}

func newFakeEmotionRepo() *fakeEmotionRepo {
	return &fakeEmotionRepo{data: map[int64]*model.EmotionAnalysis{}}
}

func (f *fakeEmotionRepo) GetByMessageID(_ context.Context, msgID int64) (*model.EmotionAnalysis, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.data[msgID]; ok {
		return e, nil
	}
	return nil, nil
}

func (f *fakeEmotionRepo) put(msgID int64, e *model.EmotionAnalysis) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[msgID] = e
}

// fakeFaceRepo 实现 FaceEmotionRepo 子集。
type fakeFaceRepo struct {
	mu   sync.Mutex
	data map[int64]*model.FaceEmotionResult
}

func newFakeFaceRepo() *fakeFaceRepo {
	return &fakeFaceRepo{data: map[int64]*model.FaceEmotionResult{}}
}

func (f *fakeFaceRepo) GetLatestByMessageID(_ context.Context, msgID int64) (*model.FaceEmotionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.data[msgID]; ok {
		return e, nil
	}
	return nil, nil
}
func (f *fakeFaceRepo) put(msgID int64, e *model.FaceEmotionResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[msgID] = e
}

// fakeVoiceRepo 实现 VoiceEmotionRepo 子集。
type fakeVoiceRepo struct {
	mu   sync.Mutex
	data map[int64]*model.VoiceEmotionResult
}

func newFakeVoiceRepo() *fakeVoiceRepo {
	return &fakeVoiceRepo{data: map[int64]*model.VoiceEmotionResult{}}
}

func (f *fakeVoiceRepo) GetLatestByMessageID(_ context.Context, msgID int64) (*model.VoiceEmotionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.data[msgID]; ok {
		return e, nil
	}
	return nil, nil
}
func (f *fakeVoiceRepo) put(msgID int64, e *model.VoiceEmotionResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[msgID] = e
}

// fakeFusedRepo 实现 FusedEmotionRepo + 记录 Upsert 调用。
type fakeFusedRepo struct {
	mu      sync.Mutex
	data    map[int64]*model.FusedEmotion
	upserts []int64 // 记录被 Upsert 的 messageID
}

func newFakeFusedRepo() *fakeFusedRepo {
	return &fakeFusedRepo{data: map[int64]*model.FusedEmotion{}}
}

func (f *fakeFusedRepo) Upsert(_ context.Context, e *model.FusedEmotion) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[e.MessageID] = e
	f.upserts = append(f.upserts, e.MessageID)
	return nil
}
func (f *fakeFusedRepo) GetByMessageID(_ context.Context, msgID int64) (*model.FusedEmotion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.data[msgID]; ok {
		return e, nil
	}
	return nil, nil
}
func (f *fakeFusedRepo) ListPending(_ context.Context, ttlSeconds int) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]int64, 0, len(f.data))
	for msgID := range f.data {
		ids = append(ids, msgID)
	}
	return ids, nil
}

// fakeFuser 总是返回预设结果（用于测试 Worker 调度逻辑，不测试 fuser 本身）。
type fakeFuser struct {
	result *model.FusedEmotion
	err    error
	called int
	mu     sync.Mutex
}

func (f *fakeFuser) Fuse(_ context.Context, _ ModalitySnapshot) (*model.FusedEmotion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called++
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// TestWorker_Tick_FusesOneMessage_NoModalityButText 单 text 模态也能融合。
func TestWorker_Tick_FusesOneMessage_NoModalityButText(t *testing.T) {
	t.Parallel()
	textRepo := newFakeEmotionRepo()
	faceRepo := newFakeFaceRepo()
	voiceRepo := newFakeVoiceRepo()
	fusedRepo := newFakeFusedRepo()

	textRepo.put(100, &model.EmotionAnalysis{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "sad", SentimentScore: -0.5, Confidence: 0.9, Model: "text-v1",
	})
	fusedRepo.data[100] = &model.FusedEmotion{MessageID: 100} // 触发 ListPending 返回 100

	llm := &fakeFuser{result: &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "sad", FusionMethod: "llm",
	}}
	late := &fakeFuser{result: &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "sad", FusionMethod: "late_fusion_weighted",
	}}

	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo:    textRepo,
		FaceEmotionRepo: faceRepo,
		VoiceEmotionRepo: voiceRepo,
		FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo,
		LLMFuser: llm,
		LateFuser: late,
		TickInterval: 5 * time.Second,
	})

	err := w.Tick(context.Background())
	require.NoError(t, err)

	// LLM 成功 → late 不被调
	assert.Equal(t, 1, llm.called)
	assert.Equal(t, 0, late.called, "late should not be called when LLM succeeds")
	require.Len(t, fusedRepo.upserts, 1)
	assert.Equal(t, int64(100), fusedRepo.upserts[0])

	// 写入的 fused 行来自 LLM
	got, _ := fusedRepo.GetByMessageID(context.Background(), 100)
	require.NotNil(t, got)
	assert.Equal(t, "llm", got.FusionMethod)
}

// TestWorker_Tick_AllThreeModalities 完整三路。
func TestWorker_Tick_AllThreeModalities(t *testing.T) {
	t.Parallel()
	textRepo := newFakeEmotionRepo()
	faceRepo := newFakeFaceRepo()
	voiceRepo := newFakeVoiceRepo()
	fusedRepo := newFakeFusedRepo()

	textRepo.put(200, &model.EmotionAnalysis{
		MessageID: 200, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "happy", SentimentScore: 0.5, Confidence: 0.9, Model: "text-v1",
	})
	faceRepo.put(200, &model.FaceEmotionResult{
		MessageID: 200, PrimaryEmotion: "neutral", Confidence: 0.7, Model: "fer",
	})
	voiceRepo.put(200, &model.VoiceEmotionResult{
		MessageID: 200, PrimaryEmotion: "happy", Confidence: 0.8, Model: "sv",
	})
	fusedRepo.data[200] = &model.FusedEmotion{MessageID: 200}

	llm := &fakeFuser{result: &model.FusedEmotion{
		MessageID: 200, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "happy", FusionMethod: "llm", Reasoning: "三路一致",
	}}
	late := &fakeFuser{result: &model.FusedEmotion{
		MessageID: 200, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "happy", FusionMethod: "late_fusion_weighted",
	}}

	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo: textRepo, FaceEmotionRepo: faceRepo, VoiceEmotionRepo: voiceRepo,
		FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo, LLMFuser: llm, LateFuser: late,
		TickInterval: 5 * time.Second,
	})

require.NoError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, llm.called, "should call LLM first")
	assert.Equal(t, 0, late.called, "should NOT fall back when LLM succeeds")
	require.Len(t, fusedRepo.upserts, 1)
}

// TestWorker_Tick_LRUHitSkipsProcessing Stage 35 PR-3：LRU 命中 → 直接 skip，不调 fuser / 不 upsert。
func TestWorker_Tick_LRUHitSkipsProcessing(t *testing.T) {
	t.Parallel()
	textRepo := newFakeEmotionRepo()
	faceRepo := newFakeFaceRepo()
	voiceRepo := newFakeVoiceRepo()
	fusedRepo := newFakeFusedRepo()

	textRepo.put(700, &model.EmotionAnalysis{MessageID: 700, UserID: 7, ConversationID: 50, PrimaryEmotion: "happy"})
	fusedRepo.data[700] = &model.FusedEmotion{MessageID: 700}

	llm := &fakeFuser{result: &model.FusedEmotion{MessageID: 700, PrimaryEmotion: "happy", FusionMethod: "llm"}}
	late := &fakeFuser{result: &model.FusedEmotion{MessageID: 700, PrimaryEmotion: "happy", FusionMethod: "late_fusion_weighted"}}

	lru := NewMsgIDLRU(100, time.Minute)
	lru.Add(700) // 预登记，模拟"上一 tick 已融合"

	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo: textRepo, FaceEmotionRepo: faceRepo, VoiceEmotionRepo: voiceRepo,
		FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo, LLMFuser: llm, LateFuser: late,
		TickInterval: 5 * time.Second, RateLimit: lru,
	})

	require.NoError(t, w.Tick(context.Background()))
	assert.Equal(t, 0, llm.called, "LRU hit must skip LLM")
	assert.Equal(t, 0, late.called, "LRU hit must skip Late")
	assert.Empty(t, fusedRepo.upserts, "LRU hit must skip Upsert")
}

// TestWorker_Tick_LRUMissProcesses Stage 35 PR-3：LRU 未命中 → 正常处理 + Upsert 后 Add。
func TestWorker_Tick_LRUMissProcesses(t *testing.T) {
	t.Parallel()
	textRepo := newFakeEmotionRepo()
	faceRepo := newFakeFaceRepo()
	voiceRepo := newFakeVoiceRepo()
	fusedRepo := newFakeFusedRepo()

	textRepo.put(800, &model.EmotionAnalysis{MessageID: 800, UserID: 7, ConversationID: 50, PrimaryEmotion: "happy"})
	fusedRepo.data[800] = &model.FusedEmotion{MessageID: 800}

	llm := &fakeFuser{result: &model.FusedEmotion{MessageID: 800, PrimaryEmotion: "happy", FusionMethod: "llm"}}
	late := &fakeFuser{result: &model.FusedEmotion{MessageID: 800, PrimaryEmotion: "happy", FusionMethod: "late_fusion_weighted"}}

	lru := NewMsgIDLRU(100, time.Minute) // 空 LRU

	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo: textRepo, FaceEmotionRepo: faceRepo, VoiceEmotionRepo: voiceRepo,
		FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo, LLMFuser: llm, LateFuser: late,
		TickInterval: 5 * time.Second, RateLimit: lru,
	})

	require.NoError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, llm.called)
	require.Len(t, fusedRepo.upserts, 1)
	// Upsert 完后 LRU 应该有 800
	assert.True(t, lru.Touch(800), "msgID=800 should be in LRU after Upsert")
}

// TestWorker_Tick_LRUNilNoEffect Stage 35 PR-3：RateLimit=nil 时不限流（原行为不变）。
func TestWorker_Tick_LRUNilNoEffect(t *testing.T) {
	t.Parallel()
	textRepo := newFakeEmotionRepo()
	faceRepo := newFakeFaceRepo()
	voiceRepo := newFakeVoiceRepo()
	fusedRepo := newFakeFusedRepo()

	textRepo.put(900, &model.EmotionAnalysis{MessageID: 900, UserID: 7, ConversationID: 50, PrimaryEmotion: "happy"})
	fusedRepo.data[900] = &model.FusedEmotion{MessageID: 900}

	llm := &fakeFuser{result: &model.FusedEmotion{MessageID: 900, PrimaryEmotion: "happy", FusionMethod: "llm"}}
	late := &fakeFuser{result: &model.FusedEmotion{MessageID: 900, PrimaryEmotion: "happy", FusionMethod: "late_fusion_weighted"}}

	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo: textRepo, FaceEmotionRepo: faceRepo, VoiceEmotionRepo: voiceRepo,
		FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo, LLMFuser: llm, LateFuser: late,
		TickInterval: 5 * time.Second, RateLimit: nil, // 不限流
	})

	require.NoError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, llm.called)
	require.Len(t, fusedRepo.upserts, 1)
}

// TestWorker_Tick_LLMFailure_FallsBackToLate LLM 失败 → late 兜底。
func TestWorker_Tick_LLMFailure_FallsBackToLate(t *testing.T) {
	t.Parallel()
	textRepo := newFakeEmotionRepo()
	faceRepo := newFakeFaceRepo()
	voiceRepo := newFakeVoiceRepo()
	fusedRepo := newFakeFusedRepo()

	textRepo.put(300, &model.EmotionAnalysis{MessageID: 300, UserID: 7, ConversationID: 50, PrimaryEmotion: "happy"})
	fusedRepo.data[300] = &model.FusedEmotion{MessageID: 300}

	llm := &fakeFuser{err: errors.New("timeout")}
	late := &fakeFuser{result: &model.FusedEmotion{
		MessageID: 300, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "happy", FusionMethod: "late_fusion_weighted",
	}}

	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo: textRepo, FaceEmotionRepo: faceRepo, VoiceEmotionRepo: voiceRepo,
		FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo, LLMFuser: llm, LateFuser: late,
		TickInterval: 5 * time.Second,
	})
	require.NoError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, llm.called)
	assert.Equal(t, 1, late.called, "must fall back to late_fuser")
}

// TestWorker_Tick_NoTextEmotion_SkipUpsert text 没结果时跳过（避免假数据）。
func TestWorker_Tick_NoTextEmotion_SkipUpsert(t *testing.T) {
	t.Parallel()
	textRepo := newFakeEmotionRepo()
	faceRepo := newFakeFaceRepo()
	voiceRepo := newFakeVoiceRepo()
	fusedRepo := newFakeFusedRepo()

	// fusedRepo.data 有 messageID=400，但 textRepo 没有 400 的 emotion_analysis
	fusedRepo.data[400] = &model.FusedEmotion{MessageID: 400}
	faceRepo.put(400, &model.FaceEmotionResult{MessageID: 400, PrimaryEmotion: "happy"})

	llm := &fakeFuser{}
	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo: textRepo, FaceEmotionRepo: faceRepo, VoiceEmotionRepo: voiceRepo,
		FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo, LLMFuser: llm, LateFuser: &fakeFuser{},
		TickInterval: 5 * time.Second,
	})
	require.NoError(t, w.Tick(context.Background()))
	assert.Equal(t, 0, llm.called, "should not call fuser when no text modality")
	assert.Empty(t, fusedRepo.upserts)
}

// TestWorker_Run_StopsOnContextCancel Run 启动后能被 ctx cancel 优雅停止。
func TestWorker_Run_StopsOnContextCancel(t *testing.T) {
	t.Parallel()
	fusedRepo := newFakeFusedRepo()
	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo: newFakeEmotionRepo(), FaceEmotionRepo: newFakeFaceRepo(),
		VoiceEmotionRepo: newFakeVoiceRepo(), FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo,
		LLMFuser: &fakeFuser{}, LateFuser: &fakeFuser{},
		TickInterval: 10 * time.Millisecond, // 测试用快速 tick
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		// 优雅退出，nil 或 context.Canceled 都接受
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop within 2s after cancel")
	}
}
