// Package memory — summary.go
//
// CL-08a · A3：用户级记忆摘要（注入侧读的那份纯文本）。
//
// 纯函数、无 DB/Redis：排序、格式、预算截断都在这里定死，
// 写侧（ai-svc）与测试共用；读侧（BFF）只负责"取不到就降级为空"。
package memory

import (
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"emotion-echo-ai-svc/internal/model"
)

// DefaultSummaryBudgetTokens 记忆段默认预算（与执行计划 §0.1 的 ≤600 一致）。
const DefaultSummaryBudgetTokens = 600

// BuildSummary 把当前有效事实拼成注入用的纯文本摘要。
//
// 规则：
//   - 只接受 active 且未软删的行（调用方通常已过滤，这里再防御一次）；
//   - 按 valid_from DESC（最近优先）；
//   - 每行 `- YYYY-MM <事实文本>`；
//   - 超出 token 预算时**从最旧的行开始丢**（最近的记忆更重要）；
//   - fact_text 里的换行压成空格（摘要本身按行解析，避免破坏结构）。
//
// token 为粗估（中文 1 rune ≈ 1 token，非中文 4 rune ≈ 1 token）；
// 精确 tokenizer 不在本阶段范围，预算只用于"别把 prompt 撑爆"的上界保护。
func BuildSummary(facts []model.MemoryFact, budgetTokens int) string {
	if budgetTokens <= 0 {
		budgetTokens = DefaultSummaryBudgetTokens
	}

	active := make([]model.MemoryFact, 0, len(facts))
	for _, f := range facts {
		if f.DeletedAt != nil {
			continue
		}
		if f.Status != "" && f.Status != model.MemoryFactStatusActive {
			continue
		}
		if strings.TrimSpace(factLineText(f)) == "" {
			continue
		}
		active = append(active, f)
	}
	sort.SliceStable(active, func(i, j int) bool {
		if !active[i].ValidFrom.Equal(active[j].ValidFrom) {
			return active[i].ValidFrom.After(active[j].ValidFrom)
		}
		return active[i].ID > active[j].ID
	})

	var lines []string
	used := 0
	for _, f := range active {
		line := "- " + validFromLabel(f.ValidFrom) + " " + sanitizeLine(factLineText(f))
		cost := estimateTokens(line) + 1 // +1 = 换行
		if used+cost > budgetTokens {
			break
		}
		lines = append(lines, line)
		used += cost
	}
	return strings.Join(lines, "\n")
}

// factLineText 事实的可读文本；FactText 为空时用三元组兜底。
func factLineText(f model.MemoryFact) string {
	if s := strings.TrimSpace(f.FactText); s != "" {
		return s
	}
	return strings.TrimSpace(f.Subject + f.Predicate + f.Object)
}

// validFromLabel 取 YYYY-MM（注入只关心"大约什么时候"，精确到秒没有意义）。
func validFromLabel(t time.Time) string {
	if t.IsZero() {
		return "未知时间"
	}
	return t.UTC().Format("2006-01")
}

// sanitizeLine 压掉换行与连续空白。
func sanitizeLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// estimateTokens 粗估 token 数（CJK 逐字计 1；其余按 4 字符计 1）。
func estimateTokens(s string) int {
	cjk, other := 0, 0
	for _, r := range s {
		if r < utf8.RuneSelf {
			other++
			continue
		}
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			cjk++
			continue
		}
		other++
	}
	t := cjk + (other+3)/4
	if t == 0 && s != "" {
		t = 1
	}
	return t
}
