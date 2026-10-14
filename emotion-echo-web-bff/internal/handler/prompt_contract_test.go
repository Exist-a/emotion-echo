// Package handler — prompt_contract_test.go
//
// CL-04 · Phase 6（T-13）：前后端字段契约对齐（plan §G6）。
//
// 本轮两条用户裁定（2026-10-14）：
//   ① **去掉基础人设的"简短"约束**（原"用中文简短回应（2-3 句话）"）——先看效果；
//   ② **`emotion` 字段"用起来"**。
//
// 关于 ② 的一手事实（落地前亲核，见 report §3.12）：
//   - 前端 `sendToExistingConversation(convId, content, emotion = 'neutral', ...)` 的 `emotion`
//     **在所有调用点都没传**（new.vue / [id].vue 只传 faceEmotion/voiceEmotion）
//     ⇒ 线上该字段**恒为默认值 `'neutral'`**，不是测量值；
//   - 前端 `app/utils/emotion.ts` 只有标签映射，**没有**文本情绪分类器；
//   - 承载它的 `pages/chat/conversation/[id].vue`、`composables/useConversationSender.ts`
//     在并行契约 §1.1 里属 **C 轨（CL-05）独占列** ⇒ A 轨不得改（需握手）。
//
// 因此本阶段落地 BFF 半场（零风险 + 向前兼容）：
//   - 把 `neutral`（= 前端默认值）视为"无信号"，**不**注入（否则每条都写"对方此刻平静"，是假陈述）；
//   - 非空且非 `neutral` 时，作为"此刻"情绪段注入（与 face/voice 同段位、同样遵守不点破来源护栏）；
//   - face/voice 优先：两者任一非空时不用文本信号（沿用"前端 payload 优先、不叠加多源"纪律）。
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ① 基础人设：不留"简短/2-3 句话"约束（用户 2026-10-14 裁定）
// ---------------------------------------------------------------------------

func TestBasePersona_NoBrevityConstraint(t *testing.T) {
	for _, banned := range []string{"2-3 句话", "简短", "3 句"} {
		assert.NotContains(t, baseSystemPrompt, banned,
			"用户 2026-10-14 裁定去掉基础人设的简短约束（与意图风格注入不再互相矛盾）")
	}
	// 保留项：语言 / 身份 / 三条行为基调（不得因去掉长度约束而被误删）
	assert.Contains(t, baseSystemPrompt, "中文", "必须保留语言约束")
	assert.Contains(t, baseSystemPrompt, "情绪疏导陪伴", "必须保留身份")
	for _, keep := range []string{"理解", "不评判", "鼓励"} {
		assert.Contains(t, baseSystemPrompt, keep, "必须保留行为基调：%s", keep)
	}
}

// ---------------------------------------------------------------------------
// ② 文本情绪字段（emotion）：非 neutral 才用；face/voice 优先
// ---------------------------------------------------------------------------

func TestTextEmotion_InjectedWhenNonNeutral(t *testing.T) {
	h := &AIStreamHandler{}
	got := h.assembleSystemPrompt(assembleInput{ctx: context.Background(), textEmotion: "anxious"})

	assert.Contains(t, got, "话语里透着", "非 neutral 的文本情绪必须作为「此刻」信号注入")
	assert.Contains(t, got, "不安", "anxious → 不安（复用中文映射）")
	assert.Contains(t, got, "情绪上下文", "必须与其它情绪段同一句式前缀")
	// 护栏：不得点破数据来源（与 D-14 同款）
	for _, banned := range []string{"识别", "分析", "检测", "模型", "分类"} {
		assert.NotContains(t, got, banned, "文本情绪段不得点破来源")
	}
}

func TestTextEmotion_NeutralOrDefaultIsNoSignal(t *testing.T) {
	h := &AIStreamHandler{}
	for _, v := range []string{"", "neutral"} {
		got := h.assembleSystemPrompt(assembleInput{ctx: context.Background(), textEmotion: v})
		assert.Equal(t, baseSystemPrompt, got,
			"`%s` 是前端默认值（非测量值）⇒ 必须视为无信号，不得凭空生成情绪段", v)
	}
}

func TestTextEmotion_FaceVoiceTakePrecedence(t *testing.T) {
	h := &AIStreamHandler{}
	got := h.assembleSystemPrompt(assembleInput{
		ctx:          context.Background(),
		faceEmotion:  "happy",
		textEmotion:  "sad",
		conversationID: 0,
	})
	assert.Contains(t, got, "神情看起来", "face 信号在位时必须用它")
	assert.NotContains(t, got, "话语里透着", "face/voice 非空时不得叠加文本信号（避免多源矛盾描述）")
}

// 与历史回落的关系：文本情绪（当前轮）优先于会话历史（统计）
func TestTextEmotion_BeatsHistoryFallback(t *testing.T) {
	fake := &fakeEmotionSource{pattern: "sad"}
	h := &AIStreamHandler{emotion: fake}

	got := h.assembleSystemPrompt(assembleInput{
		ctx: context.Background(), conversationID: 284, textEmotion: "anxious",
	})

	assert.Contains(t, got, "话语里透着", "当前轮的文本情绪优先于历史统计")
	assert.Equal(t, 0, fake.calls, "有当前轮信号时不得再查历史（省一次 gRPC，沿用 D-14 纪律）")
	require.NotContains(t, got, "最近的状态")
}
