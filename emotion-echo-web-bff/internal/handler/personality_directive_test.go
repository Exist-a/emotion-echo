// Package handler — personality_directive_test.go
//
// 人格提示词语义化（E2E-F-95）：把注入内容从「5 个形容词」改成「行为规格」，
// 并用双通道分档解决覆盖率缺陷（绝对档 80.6% 落中区 ⇒ 34% 用户拿不到任何指令）。
//
// 契约要点：
//  1. 绝对档（≥23 高 / ≤13 低）→ 明确指令
//  2. 无绝对极端时走 ipsative（个体内相对）→「五项中相对最高/最低」
//  3. 极差 <4（真正平坦，如全选 3）→ 返回空，调用方回落基础人设（不编造）
//  4. 指令必须互斥：高外向的文案不得与低外向的文案同时出现
//  5. 三句护栏常在：不点破来源 / 不贴标签 / 不过火
package handler

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dims 便捷构造：five dims
func dimsOf(openness, conscientiousness, extraversion, agreeableness, neuroticism float64) map[string]float64 {
	return map[string]float64{
		"openness":          openness,
		"conscientiousness": conscientiousness,
		"extraversion":      extraversion,
		"agreeableness":     agreeableness,
		"neuroticism":       neuroticism,
	}
}

// =====================================================
// #1 绝对档
// =====================================================

// hasDirectiveFor 判断指南里是否存在 label 的**指令行**（而非头部数字摘要里的标签）。
// 指令行形如：`- <label>明显偏高：…` / `- <label>明显偏低：…` / `- <label>（在五项中相对最高）：…`
func hasDirectiveFor(guide, label string) bool {
	for _, line := range strings.Split(guide, "\n") {
		if strings.HasPrefix(line, "- "+label) {
			return true
		}
	}
	return false
}

func TestBuildPersonalityGuide_AbsoluteHigh_EmitsHighDirective(t *testing.T) {
	t.Parallel()
	// 外向性 30（高）；其余中档
	g := buildPersonalityGuide(dimsOf(18, 18, 30, 18, 18))
	require.NotEmpty(t, g)
	assert.Contains(t, g, "外向性明显偏高")
	assert.Contains(t, g, "更主动", "高外向应给出「更主动」这类行为指令")
	// 中档维度不得出**指令行**（头部数字摘要仍会列出全部维度，那是给模型的量级感）
	for _, mid := range []string{"开放性", "尽责性", "宜人性", "神经质"} {
		assert.False(t, hasDirectiveFor(g, mid), "中档维度 %s 不应产出指令行", mid)
	}
}

func TestBuildPersonalityGuide_AbsoluteLow_EmitsLowDirective(t *testing.T) {
	t.Parallel()
	// 外向性 6（低）；其余中档
	g := buildPersonalityGuide(dimsOf(18, 18, 6, 18, 18))
	require.NotEmpty(t, g)
	assert.Contains(t, g, "外向性明显偏低")
	assert.Contains(t, g, "不要连珠炮", "低外向应给出「不要连珠炮追问」这类行为指令")
	for _, mid := range []string{"开放性", "尽责性", "宜人性", "神经质"} {
		assert.False(t, hasDirectiveFor(g, mid), "中档维度 %s 不应产出指令行", mid)
	}
}

func TestBuildPersonalityGuide_MidBand_ProducesNoAbsoluteLine(t *testing.T) {
	t.Parallel()
	// 全部中档但极差 0 → 无绝对行；见 #5 亦断言整体为空
	g := buildPersonalityGuide(dimsOf(18, 18, 18, 18, 18))
	assert.Equal(t, "", g, "全中档且无极差 ⇒ 无可用信息，必须回落基础人设而非编造")
}

// =====================================================
// #2 互斥性
// =====================================================

func TestBuildPersonalityGuide_HighAndLowTextsAreMutuallyExclusive(t *testing.T) {
	t.Parallel()
	high := buildPersonalityGuide(dimsOf(18, 18, 30, 18, 18))
	low := buildPersonalityGuide(dimsOf(18, 18, 6, 18, 18))

	assert.NotContains(t, high, "不要连珠炮", "高外向不得出现低外向的禁令")
	assert.NotContains(t, low, "更主动", "低外向不得出现高外向的要求")

	// 神经质一对同样检查
	nHigh := buildPersonalityGuide(dimsOf(18, 18, 18, 18, 30))
	nLow := buildPersonalityGuide(dimsOf(18, 18, 18, 18, 6))
	assert.Contains(t, nHigh, "先稳住情绪")
	assert.NotContains(t, nLow, "先稳住情绪")
	assert.Contains(t, nLow, "不必反复安抚")
}

// =====================================================
// #4/#5 ipsative 相对档
// =====================================================

func TestBuildPersonalityGuide_Ipsative_FallbackWhenNoAbsoluteExtreme(t *testing.T) {
	t.Parallel()
	// 80.6% 的现实情形：全部落在中档区间内，但有明显极差（22 vs 14 = 8 ≥ 4）
	g := buildPersonalityGuide(dimsOf(22, 18, 14, 18, 18))
	require.NotEmpty(t, g, "极差 ≥4 但无绝对极端时，必须启用 ipsative 回退（否则近 1/3 用户拿不到任何指令）")
	assert.Contains(t, g, "开放性")
	assert.Contains(t, g, "外向性")
	assert.Contains(t, g, "相对最高", "ipsative 措辞必须标明是「相对」，不能被读成绝对值")
	assert.Contains(t, g, "相对最低")
}

func TestBuildPersonalityGuide_Ipsative_NotTriggeredWhenSpreadTiny(t *testing.T) {
	t.Parallel()
	// 极差 3 < 4 → 视为真正平坦
	g := buildPersonalityGuide(dimsOf(19, 18, 17, 18, 18))
	assert.Equal(t, "", g, "极差 <4 视为平坦画像，返回空而不是硬凑")
}

func TestBuildPersonalityGuide_Ipsative_SkippedWhenAbsoluteExtremeExists(t *testing.T) {
	t.Parallel()
	// 有绝对极端（外向 28）时，不应再叠加 ipsative 行（避免同一维度两条指令）
	g := buildPersonalityGuide(dimsOf(18, 18, 28, 18, 8))
	assert.Contains(t, g, "外向性明显偏高")
	assert.Contains(t, g, "神经质明显偏低")
	assert.NotContains(t, g, "相对最高", "已有绝对极端时不再追加相对档")
}

// =====================================================
// #6 缺维度
// =====================================================

func TestBuildPersonalityGuide_PartialDims_WorksWithoutFabricating(t *testing.T) {
	t.Parallel()
	g := buildPersonalityGuide(map[string]float64{"extraversion": 30})
	require.NotEmpty(t, g)
	assert.Contains(t, g, "外向性明显偏高")
	// 未提供的维度不得出现（不补 0 —— 补 0 会被读成「极低」= 编造）
	for _, absent := range []string{"开放性", "尽责性", "宜人性", "神经质"} {
		assert.NotContains(t, g, absent, "缺失维度 %s 不得凭空出现在指令里", absent)
	}
}

func TestBuildPersonalityGuide_EmptyDims_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", buildPersonalityGuide(nil))
	assert.Equal(t, "", buildPersonalityGuide(map[string]float64{}))
}

// =====================================================
// #7 护栏
// =====================================================

func TestBuildPersonalityGuide_GuardrailsAlwaysPresent(t *testing.T) {
	t.Parallel()
	g := buildPersonalityGuide(dimsOf(18, 18, 30, 18, 18))
	assert.Contains(t, g, "不要点破", "护栏：不得点破画像来源")
	assert.Contains(t, g, "标签", "护栏：不得给用户贴标签")
	assert.Contains(t, g, "不过火", "护栏：调整强度要有界")
	// 基底身份不得被画像覆盖
	assert.Contains(t, g, "陪伴者", "必须声明身份/基调不变")
}

// =====================================================
// #8 覆盖率（本迭代的核心量化断言）
// =====================================================

// TestBuildPersonalityGuide_Coverage 用随机答卷做统计：修前覆盖率 66%（34% 全中档），
// 加 ipsative 回退后应 ≥95%。这是 E2E-F-95 缺口 2 的回归钉。
func TestBuildPersonalityGuide_Coverage(t *testing.T) {
	t.Parallel()
	// 确定性伪随机（不依赖 math/rand 的版本差异，便于复现）
	seed := uint64(20260921)
	next := func() uint64 { // xorshift64
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return seed
	}

	const trials = 10000
	hit := 0
	dims := []string{"openness", "conscientiousness", "extraversion", "agreeableness", "neuroticism"}
	for i := 0; i < trials; i++ {
		m := make(map[string]float64, 5)
		for _, d := range dims {
			sum := 0
			for q := 0; q < 6; q++ { // 每题 Likert 1..5
				sum += 1 + int(next()%5)
			}
			m[d] = float64(sum)
		}
		if buildPersonalityGuide(m) != "" {
			hit++
		}
	}
	rate := float64(hit) / float64(trials)
	t.Logf("覆盖率 = %.1f%%（%d/%d）", rate*100, hit, trials)
	assert.GreaterOrEqual(t, rate, 0.95,
		"加 ipsative 回退后覆盖率应 ≥95%%（修前约 66%%：34%% 的用户五个维度全落中档拿不到任何指令）")
}

// =====================================================
// 头部摘要
// =====================================================

func TestBuildPersonalityGuide_HeaderCarriesScoresAndIntent(t *testing.T) {
	t.Parallel()
	g := buildPersonalityGuide(dimsOf(26, 18, 30, 26, 6))
	assert.Contains(t, g, "26/30", "头部应带五维数字摘要（给模型量级感）")
	assert.Contains(t, g, "18", "中档维度也应出现在摘要里")
	assert.True(t, strings.HasPrefix(g, "与这位用户相处的方式"),
		"头部必须说明这是「如何与他相处」的说明书，而不是 AI 的性格设定")
}

func TestBuildPersonalityGuide_DeterministicOrder(t *testing.T) {
	t.Parallel()
	d := dimsOf(30, 30, 30, 30, 30)
	first := buildPersonalityGuide(d)
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, buildPersonalityGuide(d), "同一输入必须产出同一文本（map 遍历顺序不得泄漏到输出）")
	}
	assert.Less(t, strings.Index(first, "开放性"), strings.Index(first, "神经质"),
		"指令顺序应固定为 开放→尽责→外向→宜人→神经质")
}

// =====================================================
// #9 接入 system prompt
// =====================================================

func TestAIStreamHandler_Personality_GuideReplacesAdjectives(t *testing.T) {
	t.Parallel()
	fake := &fakeLLMStreamer{deltas: []string{"hi"}}
	src := &fakePersonalitySource{dims: dimsOf(18, 18, 30, 18, 6)}
	router := newPersonalityRouter(fake, src)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ai/stream",
		strings.NewReader(`{"message":"最近有点累","conversationId":"1"}`))
	router.ServeHTTP(w, req)

	require.NotEmpty(t, fake.gotMsgs)
	sys := fake.gotMsgs[0].Content
	// 基底人设一字不改
	assert.Contains(t, sys, baseSystemPrompt, "基底人设必须是 system prompt 的前缀且不被改写")
	// 行为指令进入
	assert.Contains(t, sys, "外向性明显偏高")
	assert.Contains(t, sys, "神经质明显偏低")
	assert.Contains(t, sys, "不要点破")
	// 旧的「形容词罗列」措辞不应再是注入的主体
	assert.NotContains(t, sys, "请在回应风格上贴合该画像", "旧的无行为规格的措辞应被替换")
}

func TestAIStreamHandler_Personality_FlatProfile_FallsBackToBaseOnly(t *testing.T) {
	t.Parallel()
	fake := &fakeLLMStreamer{deltas: []string{"hi"}}
	src := &fakePersonalitySource{dims: dimsOf(18, 18, 18, 18, 18)}
	router := newPersonalityRouter(fake, src)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ai/stream",
		strings.NewReader(`{"message":"你好"}`))
	router.ServeHTTP(w, req)

	require.NotEmpty(t, fake.gotMsgs)
	assert.Equal(t, baseSystemPrompt, fake.gotMsgs[0].Content,
		"平坦画像（全 18）⇒ 无可用信息 ⇒ system prompt 必须与无画像时逐字相同")
}

// =====================================================
// 规模/护栏：避免把 prompt 撑爆
// =====================================================

func TestBuildPersonalityGuide_BoundedLength(t *testing.T) {
	t.Parallel()
	// 最坏情形：五个维度全部绝对极端
	g := buildPersonalityGuide(dimsOf(30, 30, 30, 30, 30))
	assert.Less(t, len([]rune(g)), 700,
		"完整指令应控制在 700 字以内（每对话都要随请求下发，避免无谓 token 开销）")
	assert.Contains(t, g, fmt.Sprintf("%.0f/30", 30.0))
}

// =====================================================
// 展示用：把两个对立画像的实际注入文本打出来，作为可复核的"生活文档"
// =====================================================

// TestBuildPersonalityGuide_ShowcaseOppositeProfiles 打印 A/B 两份对立画像的真实注入文本。
// 用途：人工复核"给模型看的到底是什么"，并作为验证脚本（scripts/verify_personality_prompt_diff.py）
// 所用画像的对照。跑法：go test ./internal/handler/ -run ShowcaseOppositeProfiles -v
func TestBuildPersonalityGuide_ShowcaseOppositeProfiles(t *testing.T) {
	a := dimsOf(18, 18, 30, 18, 6) // 高外向 · 低神经质
	b := dimsOf(18, 18, 6, 18, 30) // 低外向 · 高神经质
	t.Logf("【画像 A · 高外向 30 / 低神经质 6】注入文本：\n%s\n", buildPersonalityGuide(a))
	t.Logf("【画像 B · 低外向 6 / 高神经质 30】注入文本：\n%s\n", buildPersonalityGuide(b))
	t.Logf("【平坦画像（全 18）】注入文本：%q（空 ⇒ 回落基础人设）", buildPersonalityGuide(dimsOf(18, 18, 18, 18, 18)))
	require.NotEmpty(t, buildPersonalityGuide(a))
	require.NotEmpty(t, buildPersonalityGuide(b))
	require.Equal(t, "", buildPersonalityGuide(dimsOf(18, 18, 18, 18, 18)))
}
