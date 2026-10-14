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
//
// CL-04 Phase 4：本文件只保留**生成算法**；适配表 / 护栏文案 / 分档阈值等**文案与契约常量**
// 已集中到 `prompt_templates.go`（plan §3.4「system 文案集中一处」）。
package handler

import (
	"fmt"
	"strings"
)

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
