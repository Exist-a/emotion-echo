// Package handler — prompt_assembly_test.go
//
// CL-04 · Phase 1（A 轨，2026-10-14）：抽出**统一组装层**且**不改变行为**。
//
// 现状（开工前）：system prompt 在 `buildSystemPromptWithEmotion` 里拼，且在 ServeHTTP 里
// 有**两处等价内联调用点**（gRPC 路径 :414-422、HTTP 直连路径 :477-485）——缺陷 11。
//
// 本文件钉三件事：
//   T-1 零回归：各信号组合下，组装层输出**逐字等于旧实现的 golden 基线**
//       （golden 由旧实现在改造前一次性产出，作为 oracle；见下方 golden* 常量）
//   T-2 段顺序固定：base 恒在首，其余段按 canonical 顺序出现（护栏类段恒在用户可控段之后）
//   T-8 单一组装点：ServeHTTP 不再自行构造 system 消息（防缺陷 11 复发）
package handler

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// golden 基线：**由改造前的旧实现产出**（`TestZZGenerateGoldens` 一次性生成，随后该临时
// 文件删除）。这些字面量是 T-1 的 oracle —— 不是从新实现反推出来的。
// ---------------------------------------------------------------------------

const (
	goldenBase = "你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应（2-3 句话），表达理解、不评判、鼓励继续说。"

	goldenPersonalitySegment = "与这位用户相处的方式（其人格五因素量表得分：开放性 26/30、尽责性 18/30、外向性 24/30、宜人性 19/30、神经质 16/30；每维 6-30，18 为中性。请据此调整**怎么和他说话**，不要点破来源）：\n- 开放性明显偏高：可以用一点隐喻或换个更抽象的角度，允许留白；不必事事给可操作步骤。\n- 外向性明显偏高：可以更主动、节奏轻快一些，多给回应与互动感，也可以主动开个话题。\n护栏：以上只调整「怎么和他说话」，你的身份与基调不变（温柔共情的陪伴者）。不要点破来源（不说「根据你的测评／你的人格」）；不要给他贴标签（不说「你是个内向的人」，用感受去贴合而不是用标签去定义他）；调整要有感但不过火（不要因为他外向就变成话痨，也不要因为他敏感就只敢说空话）。"

	goldenEmotionNowHappyNeutral = "情绪上下文：对方此刻神情看起来愉快，对方语气听起来平静。请让回应贴合对方此刻的状态。"
	goldenEmotionNowAnxious       = "情绪上下文：对方此刻神情看起来不安。请让回应贴合对方此刻的状态。"
	goldenEmotionHistorySad       = "情绪上下文：对方最近的状态多为低落。请让回应贴合对方最近的状态。"
)

// goldenDimsMap 与 goldenPersonalitySegment 对应的五维输入。
func goldenDimsMap() map[string]float64 {
	return map[string]float64{
		"openness": 26, "conscientiousness": 18, "extraversion": 24,
		"agreeableness": 19, "neuroticism": 16,
	}
}

// ---------------------------------------------------------------------------
// T-1 零回归：逐字等于 golden 基线
// ---------------------------------------------------------------------------

func TestPromptAssembly_ZeroRegression_GoldenBytes(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name     string
		h        *AIStreamHandler
		conv     int64
		face     string
		faceConf float64
		voice    string
		expected string
	}{
		{
			name: "无画像/无历史/无情绪 → 逐字等于旧输出的基础人设",
			h:    &AIStreamHandler{}, expected: goldenBase,
		},
		{
			name:     "仅画像 → 基础人设 + 人格段",
			h:        &AIStreamHandler{personality: &fakePersonalitySource{dims: goldenDimsMap()}},
			expected: goldenBase + "\n\n" + goldenPersonalitySegment,
		},
		{
			name:     "仅前端情绪 payload（face+voice）→ 基础人设 + 此刻情绪段",
			h:        &AIStreamHandler{},
			face:     "happy", faceConf: 0.82, voice: "neutral",
			expected: goldenBase + "\n\n" + goldenEmotionNowHappyNeutral,
		},
		{
			name:     "仅会话情绪历史（payload 全空 + 有 conversationID）→ 基础人设 + 历史情绪段",
			h:        &AIStreamHandler{emotion: &fakeEmotionSource{pattern: "sad"}},
			conv:     284,
			expected: goldenBase + "\n\n" + goldenEmotionHistorySad,
		},
		{
			name:     "画像 + 前端情绪 → 基础人设 + 人格段 + 情绪段（段序固定）",
			h:        &AIStreamHandler{personality: &fakePersonalitySource{dims: goldenDimsMap()}},
			face:     "anxious", faceConf: 0.7,
			expected: goldenBase + "\n\n" + goldenPersonalitySegment + "\n\n" + goldenEmotionNowAnxious,
		},
		{
			name:     "画像来源报错 → 静默回落基础人设（不污染）",
			h:        &AIStreamHandler{personality: &fakePersonalitySource{err: context.DeadlineExceeded}},
			expected: goldenBase,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.h.assembleSystemPrompt(assembleInput{
				ctx:            ctx,
				conversationID: c.conv,
				faceEmotion:    c.face,
				faceConfidence: c.faceConf,
				voiceEmotion:   c.voice,
			})
			assert.Equal(t, c.expected, got, "组装层输出必须与旧实现逐字一致（CL-04 验收 A）")
		})
	}
}

// T-1 补充：数组形态也必须逐字一致（system + user，Phase 1 不加历史）
func TestPromptAssembly_MessagesAreSystemThenUser(t *testing.T) {
	h := &AIStreamHandler{}
	msgs := h.assembleMessages(assembleInput{ctx: context.Background(), userContent: "我今天有点累"})
	require.Len(t, msgs, 2, "Phase 1 不引入历史 ⇒ 仍为两条")
	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, goldenBase, msgs[0].Content)
	assert.Equal(t, "user", msgs[1].Role)
	assert.Equal(t, "我今天有点累", msgs[1].Content)
}

// ---------------------------------------------------------------------------
// T-2 段顺序固定
// ---------------------------------------------------------------------------

func TestPromptAssembly_SegmentOrderIsFixed(t *testing.T) {
	// canonical 顺序：base 恒在首；其后按 人格 → 情绪 的固定次序出现。
	canonical := []string{segmentKindBase, segmentKindPersonality, segmentKindEmotion}

	cases := []struct {
		name     string
		h        *AIStreamHandler
		conv     int64
		face     string
		expectKV []string // 期望的 (kind) 序列
	}{
		{"无任何信号", &AIStreamHandler{}, 0, "", []string{segmentKindBase}},
		{"仅画像", &AIStreamHandler{personality: &fakePersonalitySource{dims: goldenDimsMap()}}, 0, "", []string{segmentKindBase, segmentKindPersonality}},
		{"仅情绪", &AIStreamHandler{}, 0, "happy", []string{segmentKindBase, segmentKindEmotion}},
		{"画像+情绪", &AIStreamHandler{personality: &fakePersonalitySource{dims: goldenDimsMap()}}, 0, "happy", []string{segmentKindBase, segmentKindPersonality, segmentKindEmotion}},
		{"画像平坦（无可用信息）→ 不产生人格段", &AIStreamHandler{personality: &fakePersonalitySource{dims: map[string]float64{"openness": 18, "neuroticism": 18}}}, 0, "", []string{segmentKindBase}},
		{"画像报错 → 不产生人格段", &AIStreamHandler{personality: &fakePersonalitySource{err: context.Canceled}}, 0, "", []string{segmentKindBase}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			segs := c.h.systemSegments(assembleInput{ctx: context.Background(), conversationID: c.conv, faceEmotion: c.face})
			gotKinds := make([]string, 0, len(segs))
			for _, s := range segs {
				gotKinds = append(gotKinds, s.Kind)
				assert.NotEmpty(t, s.Text, "段 %s 不应为空串（空段应被丢弃）", s.Kind)
			}
			assert.Equal(t, c.expectKV, gotKinds)

			// 不变量 1：base 恒在首
			require.NotEmpty(t, gotKinds)
			assert.Equal(t, segmentKindBase, gotKinds[0], "base 段必须恒为第一段")

			// 不变量 2：出现顺序必须是 canonical 的子序列（护栏类段恒在用户可控段之后）
			idx := 0
			for _, k := range canonical {
				if idx < len(gotKinds) && gotKinds[idx] == k {
					idx++
				}
			}
			assert.Equal(t, len(gotKinds), idx, "段序必须是 canonical 顺序的子序列，实际=%v", gotKinds)
		})
	}
}

// ---------------------------------------------------------------------------
// T-8 单一组装点（防缺陷 11 复发）
// ---------------------------------------------------------------------------

func TestPromptAssembly_ServeHTTPHasNoInlineSystemMessage(t *testing.T) {
	src, err := os.ReadFile("ai_stream_handler.go")
	require.NoError(t, err, "应能读到 ai_stream_handler.go（测试工作目录 = 包目录）")
	text := string(src)

	assert.NotContains(t, text, "buildSystemPromptWithEmotion",
		"旧的两处内联组装必须已收敛到统一组装层（缺陷 11）")
	assert.NotContains(t, text, `{Role: "system"`,
		"ServeHTTP 不得自行构造 system 消息——组装只允许发生在组装层内")
	assert.Contains(t, text, "assembleMessages(",
		"ServeHTTP 必须经由统一组装层产出 messages")
	assert.Equal(t, 2, strings.Count(text, "assembleMessages("),
		"两条上游路径（gRPC / HTTP 直连）必须共用同一组装函数（各调用一次）")
}
