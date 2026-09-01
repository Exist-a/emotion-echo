// Package fusion — Stage 34 · PR-14 GREEN
//
// FusionWorker 是后台 tick 调度器，把 LLMFuser / WeightedLateFuser / 三个 repo
// 串成一个闭环。
//
// 设计：
//   - 每 TickInterval 跑一次 tick()
//   - tick() 是纯函数（除 ctx 外无副作用），便于测试
//   - Run() 在 goroutine 里跑循环，监听 ctx.Done() 优雅退出
//   - 依赖反转：所有 repo / fuser / clock 都通过 interface 注入
//
// 当前依赖（接口子集，非完整 repo interface）：
//   - EmotionTextGetter    —— text 情绪查
//   - FaceModalityGetter   —— face 最新结果查
//   - VoiceModalityGetter  —— voice 最新结果查
//   - FusedUpserter        —— fused 写
//   - Fuser                —— fusion 算法
package fusion

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"emotion-echo-ai-svc/internal/logging"
	"emotion-echo-ai-svc/internal/model"
)

// EmotionTextGetter 是 EmotionRepo 的最小子集（Worker 只需要 GetByMessageID）。
type EmotionTextGetter interface {
	GetByMessageID(ctx context.Context, messageID int64) (*model.EmotionAnalysis, error)
}

// FaceModalityGetter 是 FaceEmotionRepo 的最小子集。
type FaceModalityGetter interface {
	GetLatestByMessageID(ctx context.Context, messageID int64) (*model.FaceEmotionResult, error)
}

// VoiceModalityGetter 是 VoiceEmotionRepo 的最小子集。
type VoiceModalityGetter interface {
	GetLatestByMessageID(ctx context.Context, messageID int64) (*model.VoiceEmotionResult, error)
}

// FusedUpserter 是 FusedEmotionRepo 的最小子集。
type FusedUpserter interface {
	Upsert(ctx context.Context, f *model.FusedEmotion) error
}

// FusionWorkerDeps Worker 的依赖。
type FusionWorkerDeps struct {
	EmotionRepo     EmotionTextGetter
	FaceEmotionRepo FaceModalityGetter
	VoiceEmotionRepo VoiceModalityGetter
	FusedEmotionRepo FusedUpserter
	LLMFuser        Fuser
	LateFuser       Fuser
	TickInterval    time.Duration
	// PendingLister 可选：决定 tick() 找哪些 candidate
	PendingLister interface {
		ListPending(ctx context.Context, ttlSeconds int) ([]int64, error)
	}
	// RateLimit 可选：Stage 35 PR-3 同 msgID 限流器。nil 表示不限流。
	RateLimit *MsgIDLRU
}

// FusionWorker 调度器。
type FusionWorker struct {
	deps   FusionWorkerDeps
	ticked atomic.Int64
}

// NewFusionWorker 构造器。
func NewFusionWorker(deps FusionWorkerDeps) *FusionWorker {
	return &FusionWorker{deps: deps}
}

// Tick 是核心逻辑（可独立测试）。
//
// 步骤：
//  1. 从 deps.PendingLister 拿候选 messageID（没有则跳过）
//  2. 对每个 candidate，拼 ModalitySnapshot
//  3. text 为空 → skip（避免无主情绪的假融合）
//  4. 调 LLMFuser，失败 → LateFuser
//  5. FusedEmotionRepo.Upsert 写库
//
// 返回值：首个 candidate 的 error（如有）。后续 candidate 的错误被吞掉（避免一坏全坏）。
func (w *FusionWorker) Tick(ctx context.Context) error {
	w.ticked.Add(1)

	if w.deps.PendingLister == nil {
		logging.Printf("[fusion] tick: PendingLister nil, skipping")
		return nil
	}
	ttl := 300 // 5 分钟 TTL
	candidates, err := w.deps.PendingLister.ListPending(ctx, ttl)
	if err != nil {
		logging.Errorf(err, "[fusion] ListPending err")
		return err
	}
	logging.Printf("[fusion] tick: candidates=%d (msgIDs=%v)", len(candidates), candidates)

	for _, msgID := range candidates {
		if err := w.processOne(ctx, msgID); err != nil {
			// 单条失败影响整体（继续下一条）
			continue
		}
	}
	return nil
}

// processOne 拼装 snapshot + 调 fuser + upsert。
func (w *FusionWorker) processOne(ctx context.Context, messageID int64) error {
	// Stage 35 PR-3：LRU 限流。Touch 命中 → skip（不调 fuser），未命中 → 正常处理。
	if w.deps.RateLimit != nil && w.deps.RateLimit.Touch(messageID) {
		logging.Printf("[fusion] msgID=%d skipped (LRU hit)", messageID)
		return nil
	}

	// 1. 查三路
	text, _ := w.deps.EmotionRepo.GetByMessageID(ctx, messageID)
	face, _ := w.deps.FaceEmotionRepo.GetLatestByMessageID(ctx, messageID)
	voice, _ := w.deps.VoiceEmotionRepo.GetLatestByMessageID(ctx, messageID)

	// 2. text 是必备模态（用户主诉）；没有就 skip
	if text == nil {
		logging.Printf("[fusion] msgID=%d skipped (no text emotion)", messageID)
		return nil
	}

	// 3. 拼 snapshot
	snap := ModalitySnapshot{
		Text: emotionToModality(text),
		Face: faceEmotionToModality(face),
		Voice: voiceEmotionToModality(voice),
	}

	// 4. 调 LLM（主路径，如果可用）
	var fused *model.FusedEmotion
	var err error
	if w.deps.LLMFuser != nil {
		fused, err = w.deps.LLMFuser.Fuse(ctx, snap)
	}
	if err != nil || fused == nil {
		// fallback 到 late（无论 LLM 失败还是未配置）
		if err != nil {
			logging.Printf("[fusion] msgID=%d LLM miss (err=%v), fallback to late", messageID, err)
		}
		fused, err = w.deps.LateFuser.Fuse(ctx, snap)
		if err != nil || fused == nil {
			return errors.New("both LLM and late fusion failed")
		}
	}

	// 5. 补 user_id / conversation_id（来自 text）
	if fused.UserID == 0 {
		fused.UserID = text.UserID
	}
	if fused.ConversationID == 0 {
		fused.ConversationID = text.ConversationID
	}
	fused.MessageID = messageID

	logging.Printf("[fusion] msgID=%d fused: emotion=%s sentiment=%.2f method=%s modalities=%v",
		messageID, fused.PrimaryEmotion, fused.SentimentScore, fused.FusionMethod, fused.AvailableModalities)

	// 6. Upsert
	if err := w.deps.FusedEmotionRepo.Upsert(ctx, fused); err != nil {
		return err
	}

	// 7. Stage 35 PR-3：Upsert 成功后登记 LRU（防止下一 tick 重复融合）
	if w.deps.RateLimit != nil {
		w.deps.RateLimit.Add(messageID)
	}
	return nil
}

// emotionToModality 把 text EmotionAnalysis 转为 ModalityScore。
func emotionToModality(e *model.EmotionAnalysis) *ModalityScore {
	if e == nil {
		return nil
	}
	return &ModalityScore{
		Emotion:    e.PrimaryEmotion,
		Confidence: e.Confidence,
		Sentiment:  e.SentimentScore,
		Source:     e.Model,
	}
}

// faceEmotionToModality 把 FaceEmotionResult 转为 ModalityScore（无 Sentiment）。
func faceEmotionToModality(f *model.FaceEmotionResult) *ModalityScore {
	if f == nil {
		return nil
	}
	return &ModalityScore{
		Emotion:    f.PrimaryEmotion,
		Confidence: f.Confidence,
		Source:     f.Model,
	}
}

// voiceEmotionToModality 把 VoiceEmotionResult 转为 ModalityScore（无 Sentiment）。
func voiceEmotionToModality(v *model.VoiceEmotionResult) *ModalityScore {
	if v == nil {
		return nil
	}
	return &ModalityScore{
		Emotion:    v.PrimaryEmotion,
		Confidence: v.Confidence,
		Source:     v.Model,
	}
}

// Run 启动循环。监听 ctx.Done() 优雅退出。
func (w *FusionWorker) Run(ctx context.Context) error {
	interval := w.deps.TickInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logging.Printf("[fusion] worker Run loop entered, tick=%v", interval)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			logging.Printf("[fusion] tick fired (counter=%d)", w.ticked.Load()+1)
			func() {
				defer func() {
					if r := recover(); r != nil {
						logging.Printf("[fusion] PANIC recovered: %v", r)
					}
				}()
				_ = w.Tick(ctx)
			}()
		}
	}
}
