// Package handler — prompt_templates.go
//
// CL-04 · Phase 4（A 轨，2026-10-14）：**system 段文案与契约常量的唯一存放点**（plan §3.4 / 验收 D）。
//
// 为什么集中：这些文案原先散在 `ai_stream_handler.go`（基础人设、情绪段句式）与
// `personality_directive.go`（适配表、护栏、阈值）里，改一句要跨文件找；
// 更麻烦的是"同一份契约三处实现"（前端展示 / BFF 提示词 / assessment 计分）——
// 见 `personality_contract_test.go` 的三方一致性守卫。
//
// 载体口径（roadmap §4「已定参数」）：**Go 常量集中**，不引外置模板/配置中心；
// 将来需要热更时，本文件是唯一切换点。
//
// 本文件只放**文案与契约常量**，不放逻辑：装配逻辑在 `prompt_assembly.go` / `prompt_history.go`，
// 人格指令的生成算法在 `personality_directive.go`。
package handler

// personalityTemplatesFile 本文件名 —— 供守卫测试按路径读自身源码（如解析文案里的分数区间）。
const personalityTemplatesFile = "prompt_templates.go"

// ---------------------------------------------------------------------------
// 基础人设
// ---------------------------------------------------------------------------

// baseSystemPrompt 基础人设（无画像 / 无情绪 / 无历史时的完整 system）。
//
// 措辞纪律见 ADR-2026-09-personality-prompt-semantics：这是**陪伴者身份**，不是"AI 性格设定"；
// "2-3 句话"是与用户约定的回复长度（也是 Stage 113 回复变长的对照基线）。
const baseSystemPrompt = "你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应（2-3 句话），表达理解、不评判、鼓励继续说。"

// ---------------------------------------------------------------------------
// 人格「相处方式适配」段（E2E-F-95）
// ---------------------------------------------------------------------------

// personalityStyleGuide 一个维度的「联系方式适配」文案。
// WhenHigh/WhenLow 描述的是「该维度偏高的用户需要 AI 怎么说话」，而不是「AI 应该具有什么性格」。
type personalityStyleGuide struct {
	Key      string
	Label    string
	WhenHigh string
	WhenLow  string
}

// personalityStyleGuides 五维度适配表。**顺序即输出顺序**（确定性），也是三方契约的比对基准。
//
// 文案依据 Big Five 的语言学/临床相关性常识：外向性↔互动节奏与话量、
// 宜人性↔关系和谐敏感度、尽责性↔秩序与标准、开放性↔抽象与具象偏好、
// 神经质↔情绪唤起度与对确定性的需求。
var personalityStyleGuides = []personalityStyleGuide{
	{
		Key:      "openness",
		Label:    "开放性",
		WhenHigh: "可以用一点隐喻或换个更抽象的角度，允许留白；不必事事给可操作步骤。",
		WhenLow:  "直白说事，少用比喻和抽象概念，落在他能做的事上。",
	},
	{
		Key:      "conscientiousness",
		Label:    "尽责性",
		WhenHigh: "回应可以有条理（先…再…），并认可他付出的努力与自我要求。",
		WhenLow:  "语气放松、不施压，不要给「你应该按步骤做」的任务感或清单。",
	},
	{
		Key:      "extraversion",
		Label:    "外向性",
		WhenHigh: "可以更主动、节奏轻快一些，多给回应与互动感，也可以主动开个话题。",
		WhenLow:  "给足空间，不要连珠炮式追问、不要催他多说；简短的回应是被允许的。",
	},
	{
		Key:      "agreeableness",
		Label:    "宜人性",
		WhenHigh: "先认可再表达，语气温和；避免直接下判断或反驳。",
		WhenLow:  "可以直接给判断和实话，少堆叠客套与附和（铺垫多了他会觉得绕或假）。",
	},
	{
		Key:      "neuroticism",
		Label:    "神经质",
		WhenHigh: "先稳住情绪再谈内容；用确定、安定的措辞，少用「也许／可能／大概」；不追问、不引入新的担忧点，也别轻描淡写他的难受。",
		WhenLow:  "不必反复安抚，可以直接谈内容，他受得住直接的话。",
	},
}

// 分档阈值（维度分 6-30，每维 6 题 × Likert 1-5，18 为中性）
const (
	personalityHighThreshold = 23 // ≥ 视为明显偏高
	personalityLowThreshold  = 13 // ≤ 视为明显偏低
	// personalitySpreadThreshold ipsative 回退的最小极差：低于此视为"平坦画像"，
	// 此时不应硬凑指令（例如全选 3 ⇒ 五维全 18，确实无信息可用）。
	personalitySpreadThreshold = 4
)

// personalityGuideHeader 适配层头部：说明这是「相处说明书」而非 AI 性格设定，
// 并带上五维数字摘要（给模型量级感，而不只是档位）。
// ⚠️ 其中「每维 6-30」是**契约声明**：守卫会把它与前端满分 30 比对（personality_contract_test.go）。
const personalityGuideHeader = "与这位用户相处的方式（其人格五因素量表得分：%s；每维 6-30，18 为中性。" +
	"请据此调整**怎么和他说话**，不要点破来源）："

// personalityGuideGuardrail 护栏：防止画像把 AI 变成另一个人 / 冒犯用户。
const personalityGuideGuardrail = "\n护栏：以上只调整「怎么和他说话」，你的身份与基调不变（温柔共情的陪伴者）。" +
	"不要点破来源（不说「根据你的测评／你的人格」）；不要给他贴标签（不说「你是个内向的人」，" +
	"用感受去贴合而不是用标签去定义他）；调整要有感但不过火（不要因为他外向就变成话痨，" +
	"也不要因为他敏感就只敢说空话）。"

// ---------------------------------------------------------------------------
// 情绪上下文段（D-14 / E2E-F-122）
// ---------------------------------------------------------------------------

// 情绪段句式模板。**实时段**（face/voice）与**历史段**（会话统计）刻意用不同措辞 ——
// 历史统计不能冒充"此刻信号"（诚实性，延伸自 D-14 不点破来源护栏）。
const (
	emotionSegmentPrefix = "情绪上下文："
	// 实时段：face / voice 各自一句，逗号连接
	emotionFacePhrase   = "对方此刻神情看起来"
	emotionVoicePhrase  = "对方语气听起来"
	emotionNowSuffix    = "。请让回应贴合对方此刻的状态。"
	emotionPhraseJoiner = "，"
	// 历史段
	emotionHistoryPhrase = "对方最近的状态多为"
	emotionHistorySuffix = "。请让回应贴合对方最近的状态。"
)

// emotionChineseLabels 英文情绪标签 → 中文（避免 prompt 里出现英文枚举值）。
// 未收录的情绪**原样返回**（不会泄漏系统术语，也不编造中文词）。
var emotionChineseLabels = map[string]string{
	"happy":    "愉快",
	"sad":      "低落",
	"angry":    "烦躁",
	"anxious":  "不安",
	"neutral":  "平静",
	"surprise": "惊讶",
	"fear":     "紧张",
	"disgust":  "不适",
}

// emotionChineseLabel 查表；未收录原样返回。
func emotionChineseLabel(emotion string) string {
	if label, ok := emotionChineseLabels[emotion]; ok {
		return label
	}
	return emotion
}
