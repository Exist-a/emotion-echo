// Package downstream — emotion_history_test.go
//
// E2E-F-122（D-14 emotionSource 高级模式）RED：
//
// 背景：D-14 最小模式只用前端 payload（faceEmotion.getRecentEmotion() 3 秒窗口）——
// 摄像头关闭 / 权限被拒时 emotion 上下文为空，AI 完全感知不到用户情绪；
// 且只能感知"当前会话最近 3 秒"，无历史视角。
//
// 修法：EmotionHistorySource 镜像 personalitySource 模式 —— 从
// EmotionQueryService.ByConversation 拉会话情绪历史，产出"最近情绪模式"供
// buildSystemPrompt 回落注入。nil / 无数据 / 上游错误 → 返回空串（回落是增强不是依赖）。
//
// 判定规则（写死防漂移）：
//   - 过滤 neutral（sync-fallback 占位行 primary_emotion="neutral" confidence=0 无信息量）
//   - 按 created_at_ms 降序（不依赖上游排序，与 personalitySource 同款纪律）
//   - 非 neutral 行 >= 3 且众数占比 >= 50% → 返回众数（"历史情绪模式"，gap ②）
//   - 其余（1~2 行，或无主导众数）→ 返回最新一行（gap ① 兜底）
//   - 全 neutral / 空列表 → ""（不注入）
package downstream

import (
	"context"
	"testing"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEmotionQueryForHistory 实现 EmotionQueryClient，返回预置历史。
type fakeEmotionQueryForHistory struct {
	rows []*emotionquery.Emotion
	err  error
	// lastConvID 记录被调用的 conversationID（断言透传）
	lastConvID int64
	lastLimit  int
}

func (f *fakeEmotionQueryForHistory) ByMessage(context.Context, int64) (*emotionquery.Emotion, error) {
	return nil, nil
}

func (f *fakeEmotionQueryForHistory) ByConversation(_ context.Context, conversationID int64, limit int) ([]*emotionquery.Emotion, int32, error) {
	f.lastConvID = conversationID
	f.lastLimit = limit
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.rows, int32(len(f.rows)), nil
}

func (f *fakeEmotionQueryForHistory) ByFusedMessage(context.Context, int64) (*emotionquery.FusedEmotion, error) {
	return nil, nil
}

// row 构造历史行（乱序传入，验证排序不依赖上游）。
func row(emotion string, confidence float64, createdAtMs int64) *emotionquery.Emotion {
	return &emotionquery.Emotion{
		Id:             1,
		PrimaryEmotion: emotion,
		Confidence:     confidence,
		CreatedAtMs:    createdAtMs,
	}
}

func TestEmotionHistorySource_NilSafe(t *testing.T) {
	var nilSrc *EmotionHistorySource
	em, err := nilSrc.RecentEmotionPattern(context.Background(), 284)
	require.NoError(t, err, "nil receiver 不应报错（与 personalitySource 同款 nil-safe）")
	assert.Empty(t, em)

	src := NewEmotionHistorySource(nil)
	em, err = src.RecentEmotionPattern(context.Background(), 284)
	require.NoError(t, err)
	assert.Empty(t, em, "nil query client → 空串回落")
}

func TestEmotionHistorySource_AllNeutral_ReturnsEmpty(t *testing.T) {
	fake := &fakeEmotionQueryForHistory{rows: []*emotionquery.Emotion{
		row("neutral", 0, 3000),
		row("neutral", 0, 2000),
	}}
	src := NewEmotionHistorySource(fake)
	em, err := src.RecentEmotionPattern(context.Background(), 7)
	require.NoError(t, err)
	assert.Empty(t, em, "sync-fallback neutral 占位行无信息量，不得注入")
	assert.Equal(t, int64(7), fake.lastConvID, "conversationID 必须透传")
}

func TestEmotionHistorySource_LatestNonNeutral_WinsForShortHistory(t *testing.T) {
	// 1~2 行：取最新非 neutral（乱序输入验证内部排序）
	fake := &fakeEmotionQueryForHistory{rows: []*emotionquery.Emotion{
		row("sad", 0.7, 1000),
		row("happy", 0.9, 5000), // 最新
	}}
	src := NewEmotionHistorySource(fake)
	em, err := src.RecentEmotionPattern(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "happy", em, "短历史取 created_at_ms 最大的非 neutral 行")
}

func TestEmotionHistorySource_ModeWinsWhenDominant(t *testing.T) {
	// 4 行非 neutral，sad ×3 (75% >= 50%)，happy ×1 → 众数 sad
	fake := &fakeEmotionQueryForHistory{rows: []*emotionquery.Emotion{
		row("sad", 0.6, 1000),
		row("happy", 0.9, 2000),
		row("sad", 0.7, 3000),
		row("sad", 0.8, 4000),
	}}
	src := NewEmotionHistorySource(fake)
	em, err := src.RecentEmotionPattern(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "sad", em, ">=3 行且众数占比>=50% → 返回众数（历史情绪模式）")
}

func TestEmotionHistorySource_NoDominantMode_FallsBackToLatest(t *testing.T) {
	// 4 行，两两持平（无 >=50% 众数）→ 回落最新行
	fake := &fakeEmotionQueryForHistory{rows: []*emotionquery.Emotion{
		row("sad", 0.6, 1000),
		row("happy", 0.9, 2000),
		row("sad", 0.7, 3000),
		row("happy", 0.8, 4000), // 最新
	}}
	src := NewEmotionHistorySource(fake)
	em, err := src.RecentEmotionPattern(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "happy", em, "无主导众数 → 回落最新非 neutral 行")
}

func TestEmotionHistorySource_NeutralRowsFilteredBeforeMode(t *testing.T) {
	// 2 非 neutral(sad) + 3 neutral 占位：neutral 过滤后只剩 2 行 → 短历史路径取最新 sad
	fake := &fakeEmotionQueryForHistory{rows: []*emotionquery.Emotion{
		row("sad", 0.7, 1000),
		row("neutral", 0, 2000),
		row("neutral", 0, 3000),
		row("sad", 0.8, 4000),
		row("neutral", 0, 5000), // 最新但被过滤
	}}
	src := NewEmotionHistorySource(fake)
	em, err := src.RecentEmotionPattern(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "sad", em, "neutral 占位行必须先过滤再参与统计")
}

func TestEmotionHistorySource_ErrorPassthrough_AsEmpty(t *testing.T) {
	fake := &fakeEmotionQueryForHistory{err: context.DeadlineExceeded}
	src := NewEmotionHistorySource(fake)
	em, err := src.RecentEmotionPattern(context.Background(), 7)
	require.NoError(t, err, "上游错误不向调用方冒泡（回落是增强不是依赖，与 personalitySource 一致）")
	assert.Empty(t, em)
}
