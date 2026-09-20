// Package handler — personality_directive.go
//
// 人格提示词语义化（账本 E2E-F-95）。
//
// 设计原则：给模型的是「**如何与这个人相处**」的说明书，不是「AI 的性格设定」。
// "被理解感"来自 AI 的联系方式贴合用户的体验方式；让 AI 扮演性格是表演，且容易崩
// （变成话痨/客服/说教）。因此基底人设不变，画像只调整"怎么说话"。
//
// 指令写的是**可观察的输出特征**（更主动/不要连珠炮/少用「也许」），
// 而不是抽象形容词 —— 后者模型只能自行脑补，行为不可控也不可测。
package handler

import (
	"fmt"
	"strings"
)

// personalityStyleGuide 一个维度的「联系方式适配」文案。
// WhenHigh/WhenLow 描述的是「该维度偏高的用户需要 AI 怎么说话」，
// 而不是「AI 应该具有什么性格」。
type personalityStyleGuide struct {
	Key      string
	Label    string
	WhenHigh string
	WhenLow  string
}

// personalityStyleGuides 五维度适配表。顺序即输出顺序（确定性）。
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
const personalityGuideHeader = "与这位用户相处的方式（其人格五因素量表得分：%s；每维 6-30，18 为中性。" +
	"请据此调整**怎么和他说话**，不要点破来源）："

// personalityGuideGuardrail 护栏：防止画像把 AI 变成另一个人 / 冒犯用户。
const personalityGuideGuardrail = "\n护栏：以上只调整「怎么和他说话」，你的身份与基调不变（温柔共情的陪伴者）。" +
	"不要点破来源（不说「根据你的测评／你的人格」）；不要给他贴标签（不说「你是个内向的人」，" +
	"用感受去贴合而不是用标签去定义他）；调整要有感但不过火（不要因为他外向就变成话痨，" +
	"也不要因为他敏感就只敢说空话）。"

// buildPersonalityGuide 由五维分数产出「联系方式适配」指令文本。
//
// 双通道分档（解决覆盖率缺陷：仅用绝对档时 80.6% 的维度落中区、
// 34% 的用户五个维度全中档 ⇒ 拿不到任何指令）：
//
//	① 绝对档：≥23 明显偏高 / ≤13 明显偏低
//	② ipsative 个体内相对档：无绝对极端时，取最高/最低维度（极差 ≥4）标为「相对最高/最低」
//	③ 两者皆无（真正平坦）⇒ 返回 ""，调用方回落基础人设（**不编造**画像）
//
// 返回 "" 的语义 = "无可用信息"，不是错误。
func buildPersonalityGuide(dims map[string]float64) string {
	if len(dims) == 0 {
		return ""
	}

	var (
		lines      []string
		summary    []string
		best       personalityStyleGuide
		bestScore  float64
		worst      personalityStyleGuide
		worstScore float64
		seen       int
	)

	for _, g := range personalityStyleGuides {
		score, ok := dims[g.Key]
		if !ok {
			continue // 缺失维度跳过 —— 补 0 会被读成「极低」，那是编造画像
		}
		seen++
		summary = append(summary, fmt.Sprintf("%s %.0f/30", g.Label, score))

		switch {
		case score >= personalityHighThreshold:
			lines = append(lines, fmt.Sprintf("- %s明显偏高：%s", g.Label, g.WhenHigh))
		case score <= personalityLowThreshold:
			lines = append(lines, fmt.Sprintf("- %s明显偏低：%s", g.Label, g.WhenLow))
		default:
			// 记录候选，供 ipsative 回退用
			if seen == 1 || score > bestScore {
				best, bestScore = g, score
			}
			if seen == 1 || score < worstScore {
				worst, worstScore = g, score
			}
		}
	}

	// ② ipsative 回退：仅在没有绝对极端、且个体内极差足够大时启用。
	// 措辞必须写明「在五项中相对」，否则会被模型当成绝对值。
	if len(lines) == 0 && seen >= 2 && worst.Key != "" && best.Key != "" && worst.Key != best.Key {
		if bestScore-worstScore >= personalitySpreadThreshold {
			lines = append(lines,
				fmt.Sprintf("- %s（在五项中相对最高）：%s", best.Label, best.WhenHigh),
				fmt.Sprintf("- %s（在五项中相对最低）：%s", worst.Label, worst.WhenLow),
			)
		}
	}

	if len(lines) == 0 {
		return "" // 平坦画像：确无信息可用，回落基础人设
	}

	return fmt.Sprintf(personalityGuideHeader, strings.Join(summary, "、")) +
		"\n" + strings.Join(lines, "\n") +
		personalityGuideGuardrail
}
