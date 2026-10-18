// Package repository — memory_fact_planner.go
//
// CL-08a · A1：事实落库计划（纯函数）。
//
// 抽成纯函数的理由：双时间轴失效（T-3）与事实更新（T-15）是本阶段最容易写错的语义，
// 但它们不需要 DB 就能完整覆盖边界。仓储层只负责"照计划执行"。
package repository

import (
	"strings"
	"time"

	"emotion-echo-ai-svc/internal/model"
)

// FactToSupersede 一条需要关区间的既有事实。
type FactToSupersede struct {
	ID      int64
	ValidTo time.Time
}

// PlanResult 一次抽取产物的落库计划。
type PlanResult struct {
	// Supersede 需要置 valid_to + status='superseded' 的既有行（不删除）
	Supersede []FactToSupersede
	// Insert 需要新增的事实（Status 已归一化为 active）
	Insert []model.MemoryFact
}

// PlanFactUpsert 计算一次抽取的落库计划（纯函数，无 DB、无时间副作用）。
//
// existing 应只包含**该用户**的事实（调用方负责按 user_id 过滤）；函数本身仍会
// 跳过非 active / 已软删的行（防御调用方传全量）。
//
// 规则（顺序即优先级）：
//  1. 空三元组（subject/predicate/object 任一为空）⇒ 丢弃（LLM 解析残缺宁缺毋滥）；
//  2. 同一批内同 (subject,predicate) ⇒ **后出现者胜出**（同一句话里"北京→上海"取后者）；
//  3. 与既有 active 行 (subject,predicate,object) 相同 ⇒ 不重复插入
//     （跨消息重复说同一件事不产生第二行 active）；
//  4. 与既有 active 行同 (subject,predicate) 但 object 不同 ⇒ 旧行关区间
//     （valid_to = 新行 valid_from；若乱序导致负区间则夹到旧行 valid_from）；
//  5. 其余 ⇒ 插入。
func PlanFactUpsert(existing []model.MemoryFact, incoming []model.MemoryFact) PlanResult {
	plan := PlanResult{}
	kept := normalizeIncoming(incoming)

	superseded := make(map[int64]struct{}, len(existing))
	for _, in := range kept {
		if hasSameActiveObject(existing, in) {
			continue
		}
		for _, ex := range existing {
			if !isActive(ex) {
				continue
			}
			if ex.Subject != in.Subject || ex.Predicate != in.Predicate || ex.Object == in.Object {
				continue
			}
			if _, done := superseded[ex.ID]; done {
				continue
			}
			superseded[ex.ID] = struct{}{}
			validTo := in.ValidFrom
			if validTo.Before(ex.ValidFrom) {
				// 乱序到达：不得写出 valid_to < valid_from 的负区间
				validTo = ex.ValidFrom
			}
			plan.Supersede = append(plan.Supersede, FactToSupersede{ID: ex.ID, ValidTo: validTo})
		}
		plan.Insert = append(plan.Insert, in)
	}
	return plan
}

// normalizeIncoming 过滤空三元组、归一化状态与时间、按 (subject,predicate) 去重（后者胜出）。
func normalizeIncoming(in []model.MemoryFact) []model.MemoryFact {
	type key struct{ subject, predicate string }
	idx := make(map[key]int, len(in))
	kept := make([]model.MemoryFact, 0, len(in))

	for _, f := range in {
		f.Subject = strings.TrimSpace(f.Subject)
		f.Predicate = strings.TrimSpace(f.Predicate)
		f.Object = strings.TrimSpace(f.Object)
		if f.Subject == "" || f.Predicate == "" || f.Object == "" {
			continue
		}
		if f.Status == "" {
			f.Status = model.MemoryFactStatusActive
		}
		if f.ValidFrom.IsZero() {
			f.ValidFrom = time.Now().UTC()
		}
		k := key{f.Subject, f.Predicate}
		if i, ok := idx[k]; ok {
			kept[i] = f
			continue
		}
		idx[k] = len(kept)
		kept = append(kept, f)
	}
	return kept
}

// hasSameActiveObject 判断既有 active 行里是否已有完全相同的三元组。
func hasSameActiveObject(existing []model.MemoryFact, in model.MemoryFact) bool {
	for _, ex := range existing {
		if !isActive(ex) {
			continue
		}
		if ex.Subject == in.Subject && ex.Predicate == in.Predicate && ex.Object == in.Object {
			return true
		}
	}
	return false
}

// isActive 一行是否"当前有效"（active 且未被软删）。
func isActive(f model.MemoryFact) bool {
	if f.DeletedAt != nil {
		return false
	}
	return f.Status == "" || f.Status == model.MemoryFactStatusActive
}
