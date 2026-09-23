// Package handler — d14_emotion_context_test.go
//
// D-14（E2E-16 plan §B.10）：融合结果（face / voice emotion）注入 system prompt。
//
// 现状：buildSystemPrompt 只拼 baseSystemPrompt + （可选）personalityGuide。
// 前端 /ai/stream 请求体里的 emotion 字段（已有 aiStreamReq.Emotion）从未被消费。
//
// 修法：
//  1. aiStreamReq 加 FaceEmotion / FaceConfidence / VoiceEmotion 字段（前端从 useFaceEmotion
//     / useVoiceRecorder 取出最近一次结果随请求带上）
//  2. buildSystemPrompt 接 emotionContext 参数（有 face 或 voice 时拼"当前情绪上下文"段）
//  3. 三句护栏：不点破来源、不贴标签、不过火（与 E2E-F-95 人格画像同款约束）
//
// 契约钉（5 项）：
//   1. aiStreamReq 结构体含 FaceEmotion 字段
//   2. aiStreamReq 结构体含 VoiceEmotion 字段
//   3. buildSystemPrompt 函数签名含 emotion context 参数（或独立 emotionBuilder）
//   4. emotionContext 段含"不点破来源"等护栏（不直说"你正在读我的表情"）
//   5. emotionContext 段在无情绪上下文时不应污染 baseSystemPrompt
package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCtxNoPersonality 返回 personality 为 nil 的 context，触发 buildSystemPrompt 走"无画像"路径。
func testCtxNoPersonality() context.Context {
	return context.Background()
}

func TestAIStreamReq_HasFaceEmotionField(t *testing.T) {
	// 通过 Unmarshal 校验字段存在且能 round-trip
	jsonIn := `{"message":"hi","emotion":"neutral","faceEmotion":"happy","faceConfidence":0.82,"voiceEmotion":"sad","conversationId":"1"}`
	var req aiStreamReq
	require.NoError(t, json.Unmarshal([]byte(jsonIn), &req), "aiStreamReq 必须能反序列化 face/voice emotion 字段")
	assert.Equal(t, "happy", req.FaceEmotion)
	assert.InDelta(t, 0.82, req.FaceConfidence, 0.001)
	assert.Equal(t, "sad", req.VoiceEmotion)
}

func TestBuildSystemPrompt_EmotionContext_Appended_WhenFaceOrVoicePresent(t *testing.T) {
	h := &AIStreamHandler{}
	// 模拟带情绪上下文
	withCtx := h.buildSystemPromptWithEmotion(testCtxNoPersonality(), "happy", 0.82, "neutral", 0)
	require.Contains(t, withCtx, "愉快", "face emotion 必须出现在 emotion context 段（happy→愉快）")
	require.Contains(t, withCtx, "平静", "voice emotion 必须出现在 emotion context 段（neutral→平静）")
	require.Contains(t, withCtx, "情绪上下文", "必须有情绪上下文段标识")
	require.NotEqual(t, baseSystemPrompt, withCtx, "有情绪上下文时 prompt 必须不等于 base")
}

func TestBuildSystemPrompt_EmotionContext_Absent_ReturnsBase(t *testing.T) {
	h := &AIStreamHandler{}
	// 无情绪上下文（前端未带 face/voice emotion）→ 必须返回基础 prompt，不污染
	noCtx := h.buildSystemPromptWithEmotion(testCtxNoPersonality(), "", 0, "", 0)
	assert.Equal(t, baseSystemPrompt, noCtx,
		"无情绪上下文时必须返回与原 baseSystemPrompt 逐字相同（不编造、不退化）—— E2E-F-95 教训：画像缺时不假数据")
}

func TestBuildSystemPrompt_EmotionContext_HasGuardrails(t *testing.T) {
	h := &AIStreamHandler{}
	prompt := h.buildSystemPromptWithEmotion(testCtxNoPersonality(), "happy", 0.82, "neutral", 0)
	// 三句护栏关键词：
	//   - 不点破来源：不出现"摄像头""识别""分析""检测""设备""传感器"
	//   - 不贴标签：不当面称呼用户为"你很[emotion]"
	//   - 不过火：情绪描述不超过 1 句话
	bannedSources := []string{"摄像头", "识别", "分析", "检测", "设备", "传感器", "面部识别"}
	for _, w := range bannedSources {
		assert.NotContains(t, prompt, w,
			"emotion context 段不应点破来源（避免出现 %q）", w)
	}
	// 不贴标签：用户情绪描述应是中性观察而非贴脸标签
	assert.False(t, strings.Contains(prompt, "你很happy") || strings.Contains(prompt, "你很happy的"),
		"emotion context 不应给用户贴标签（避免「你很happy」式直接称呼）")
}
