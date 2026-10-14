// Package handler — prompt_params.go
//
// CL-04 · Phase 3（A 轨，2026-10-14）：**生成参数单一来源**（T-7）。
//
// 缺陷 3（plan §0.2）：handler 从不给上游请求赋 Temperature / MaxTokens
// ⇒ 上游用零值 ⇒ **输出长度无上限**（对"2-3 句话"的人设与调用成本都不受控）。
//
// 载体口径（roadmap §4 CL-04「已定参数」）：**Go 常量集中**，不引外置模板/配置中心；
// 将来需要热更或按环境差异化时，再把这两项下沉到 `config.LLM`（本文件就是唯一切换点）。
package handler

// llmGeneration BFF 下发给上游的生成参数（**单一来源**，两条上游路径共用）。
//
// 取值 = 用户 2026-10-14 裁定（plan §2 T-16 [M] 已落定）：
//   - Temperature 0.7：偏稳定，情绪陪伴场景不易跑偏；
//   - MaxTokens 1024：比原计划建议的 512 更长（用户选择），**与"2-3 句话"基础人设有张力**
//     ⇒ plan §6 R1 要求 Phase 3 落地后在 IAB 实测里核对回复是否真的变啰嗦。
var llmGeneration = struct {
	Temperature float64
	MaxTokens   int32
}{
	Temperature: 0.7,
	MaxTokens:   1024,
}

// 标题生成专用参数（T-18）：标题要稳定、要短 ⇒ 低温度 + 很小 max_tokens。
// 与对话参数分开命名，避免被误认为可以拿对话参数去生成标题。
const (
	titleTemperature = 0.3
	titleMaxTokens   = 32
)

// llmWithIntent（CL-04 Phase 5）：是否让 llm-service 先分类意图、再按意图注入回复风格。
//
// 该分支（`intent.inject_style`）自 Stage 82 就存在，但 BFF 从不设置 pb 的 `with_intent`
// ⇒ **从未在生产触发**（缺陷 5 死路径）。roadmap §4 的处置是「**接回**」，故这里置 true。
//
// ⚠️ 成本（Phase 5 收口须实测 TTFT）：llm-service 侧规则式分类拿不准时（实测样例 8 条中 6 条）
// 会用 LLM 消歧（≤5s，**在首字之前**）。若 TTFT 明显劣化，可选优化 = 在 llm-service 加
// text→intent 短 TTL 缓存（同一句话刚被 `ClassifyIntent` 分过，可命中），或改为把已分类结果
// 透传下来（需改 proto，属跨轨 spine 文件，须先握手）。
const llmWithIntent = true
