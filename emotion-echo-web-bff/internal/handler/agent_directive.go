// Package handler — agent_directive.go
//
// CL-06 · Phase 4：用户自定义角色的「角色设定」段生成（与 `personality_directive.go` 同结构：
// 本文件只放**生成算法**，文案与契约常量在 `prompt_templates.go`）。
//
// 设计口径（源计划 §3.3/§3.5）：用户文本进 system prompt 是**安全敏感面**，因此
//   · **受限槽位**：只渲染 name / persona / tone / boundaries 四个字段，用户**不能**写裸 system prompt；
//   · **空即不产生**：任一字段为空 ⇒ 不渲染该行（不留孤立标签）；四个字段全空 ⇒ 整段不产生
//     （等价于"无角色设定" ⇒ 输出回落现状，T-8）；
//   · **护栏后置**：本段之后必跟独立 `guard` 段（位置不变量由 `prompt_assembly.go` 保证）。
//
// 越狱句式黑名单 / 长度上限校验属 **Phase 5**（保存侧拦截）；本段只负责"渲染"。
package handler

import (
	"strings"

	"emotion-echo-web-bff/internal/downstream"
)

// buildAgentSegment 由角色档案产出「角色设定」段；无有效字段 ⇒ 返回 ""（该段不产生）。
//
// 返回 "" 的语义 = "无角色设定"，不是错误（与 `buildPersonalityGuide` 的平坦画像同款纪律）。
func buildAgentSegment(p *downstream.AgentProfile) string {
	if p == nil {
		return ""
	}
	lines := make([]string, 0, 4)
	if v := strings.TrimSpace(p.Name); v != "" {
		lines = append(lines, agentSegmentNameLabel+v)
	}
	if v := strings.TrimSpace(p.Persona); v != "" {
		lines = append(lines, agentSegmentPersonaLabel+v)
	}
	if v := strings.TrimSpace(p.Tone); v != "" {
		lines = append(lines, agentSegmentToneLabel+v)
	}
	if bs := nonEmptyStrings(p.Boundaries); len(bs) > 0 {
		lines = append(lines, agentSegmentBoundariesLabel+strings.Join(bs, agentBoundariesJoiner))
	}
	if len(lines) == 0 {
		return ""
	}
	return agentSegmentHeader + agentSegmentLineSeparator + strings.Join(lines, agentSegmentLineSeparator)
}

// nonEmptyStrings 去掉空串与纯空白项，并裁剪首尾空白（保持原始顺序）。
//
// 为什么要丢空项而不是保留：`不要：；` 这种残留会占 prompt 预算、且在人工核对时
// 看起来像"系统自己加了一条空约束"（本仓"不编造"纪律——见 E2E-F-95）。
func nonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if v := strings.TrimSpace(s); v != "" {
			out = append(out, v)
		}
	}
	return out
}
