// Package handler — f122_emotion_history_fallback_test.go
//
// E2E-F-122（D-14 emotionSource 高级模式）RED：
//
// 最小模式（PR #64 已落地）只消费前端 payload 的 face/voice emotion ——
// 摄像头关闭 / 权限被拒 / 3 秒窗口过期时 prompt 无情绪段，AI 感知为零。
//
// 高级模式：ServeHTTP 在 face/voice 均为空时回落 emotionSource
// （DB 会话情绪历史），把"最近情绪模式"拼进 system prompt。
// 前端 payload 优先（当前会话的实时信号 > 历史统计）。
//
// 契约钉（4 项）：
//   1. AIStreamDeps 含 Emotion 字段（镜像 Personality）
//   2. face/voice 全空 + emotionSource 有数据 → prompt 含历史情绪段
//   3. face/voice 任一非空 → 不查询 emotionSource（前端优先，不浪费一次 gRPC）
//   4. emotionSource nil → 与最小模式行为逐字一致（不污染）
package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEmotionSource 记录调用并返回预置结果。
type fakeEmotionSource struct {
	pattern     string
	err         error
	calls       int
	lastConvID  int64
}

func (f *fakeEmotionSource) RecentEmotionPattern(_ context.Context, conversationID int64) (string, error) {
	f.calls++
	f.lastConvID = conversationID
	return f.pattern, f.err
}

func TestAIStreamDeps_HasEmotionField(t *testing.T) {
	// 契约 1：Emotion 字段存在且类型为 emotionSource（编译期即断言）
	deps := AIStreamDeps{Emotion: &fakeEmotionSource{pattern: "sad"}}
	var src emotionSource = deps.Emotion
	require.NotNil(t, src)
}

func TestBuildSystemPromptWithEmotion_HistoryFallback_WhenPayloadEmpty(t *testing.T) {
	// 契约 2：face/voice 全空 → 回落历史情绪
	fake := &fakeEmotionSource{pattern: "sad"}
	h := &AIStreamHandler{emotion: fake}

	prompt := h.buildSystemPromptWithEmotion(
		context.Background(), 284,
		"", 0, "", 0, // 前端 payload 全空（摄像头关闭场景）
	)
	assert.Greater(t, fake.calls, 0, "payload 全空时必须查询 emotionSource")
	assert.Equal(t, int64(284), fake.lastConvID, "conversationID 必须透传给 emotionSource")
	assert.Contains(t, prompt, "低落", "历史情绪必须进入 prompt（sad→低落，D-14 中文映射约定）")
	assert.Contains(t, prompt, baseSystemPrompt, "基础人设必须保留")
	// 护栏复用：不得点破数据来源（不出现"摄像头/识别/分析"等词）
	for _, banned := range []string{"摄像头", "识别", "分析", "检测"} {
		assert.NotContains(t, prompt, banned, "历史情绪段必须遵守 D-14 不点破来源护栏")
	}
}

func TestBuildSystemPromptWithEmotion_PayloadWins_NoHistoryQuery(t *testing.T) {
	// 契约 3：face 非空 → 不查 emotionSource（前端实时信号优先）
	fake := &fakeEmotionSource{pattern: "sad"}
	h := &AIStreamHandler{emotion: fake}

	prompt := h.buildSystemPromptWithEmotion(
		context.Background(), 284,
		"happy", 0.9, "", 0,
	)
	assert.Equal(t, 0, fake.calls, "前端 payload 非空时不得查 emotionSource（省一次 gRPC）")
	assert.Contains(t, prompt, "愉快", "实时信号必须保留（happy→愉快）")
	assert.NotContains(t, prompt, "对方最近", "历史情绪段不得出现（实时信号优先）")
}

func TestBuildSystemPromptWithEmotion_NilEmotionSource_IdenticalToMinimal(t *testing.T) {
	// 契约 4：emotionSource nil → 与最小模式（D-14 PR #64）行为逐字一致
	h := &AIStreamHandler{} // emotion == nil

	withNil := h.buildSystemPromptWithEmotion(context.Background(), 284, "", 0, "", 0)
	// 无 payload 无 source → 只有基础人设 + 人格判断，无情绪段
	assert.Equal(t, h.buildSystemPrompt(context.Background()), withNil,
		"nil emotionSource 且 payload 空时不得凭空生成情绪段（与最小模式逐字一致）")
}

func TestBuildSystemPromptWithEmotion_HistoryError_SilentlyDegrades(t *testing.T) {
	// 上游错误 → 回落基础 prompt，不冒泡（增强不是依赖）
	fake := &fakeEmotionSource{err: errors.New("grpc unavailable")}
	h := &AIStreamHandler{emotion: fake}

	prompt := h.buildSystemPromptWithEmotion(context.Background(), 284, "", 0, "", 0)
	assert.Equal(t, h.buildSystemPrompt(context.Background()), prompt,
		"emotionSource 报错时必须静默降级为基础 prompt")
}

func TestBuildEmotionHistoryContext_SentenceAndLabel(t *testing.T) {
	// 历史段句式：与实时段（"此刻神情/语气"）区分 —— 历史不能冒充"此刻"
	seg := buildEmotionHistoryContext("sad")
	require.NotEmpty(t, seg)
	assert.Contains(t, seg, "低落", "英文情绪标签必须映射为中文（复用 emotionChineseLabel）")
	assert.False(t, strings.Contains(seg, "此刻"), "历史模式不是此刻信号，句式不得用「此刻」（不贴标签护栏的诚实性延伸）")
	assert.Equal(t, "", buildEmotionHistoryContext(""), "空情绪 → 空段（不拼残句）")
}
