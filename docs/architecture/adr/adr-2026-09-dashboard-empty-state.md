---
adr: 2026-09-dashboard-empty-state
title: 仪表盘空态渲染模式 = v-else-if 与图表卡片互斥（不再允许裸 v-if 占位）
status: accepted
date: 2026-09-21
owners: [frontend]
references: [E2E-F-14, E2E-F-91, E2E-15-stages/e2e-15-reports-dashboard/plan.md]
supersedes: null
---

# ADR-2026-09 · 仪表盘空态渲染模式

## 上下文

E2E-F-14（2026-09-19 治理轮）发现 `chat/user/index.vue`（我的空间）3 个图表卡片在 `chartData.length === 0` 时仍渲染 `<div class="ee-empty">暂无数据</div>`，与 `<BaseChart>` 同屏出现 —— "有数据也显示暂无数据"的视觉错觉。修复方式：将 `ee-empty` 从无 v-if 改为 `v-else-if="chartData.length === 0"`，与 `v-if="chartData.length > 0"` 的图表卡片互斥。

E2E-15（2026-09-21）调研发现 **dashboard 子页面 4 个**（`dailyReport.vue` / `weeklyReport.vue` / `monthlyReport.vue` / `annualReport.vue`）line 24-25 同样模式残留：`<chartCard v-if="chartData.length > 0">` + 紧接裸 `<div class="ee-empty">暂无数据</div>`，无 v-if/v-else-if。同型 bug 复发原因 = 没有强制契约钉防止模式退化。

## 决策

**所有使用 `<chartsCard>` + `<div class="ee-empty">` 占位的 Vue 组件必须遵循「v-if / v-else-if 互斥」模式**：

```vue
<chartCard v-if="chartData.length > 0" :data="chartData" />
<div v-else-if="chartData.length === 0" class="ee-empty">暂无数据</div>
```

**禁止**以下反模式（任一出现即视为 bug）：

1. 裸 `<div class="ee-empty">暂无数据</div>`（无 v-if/v-else-if）—— 与图表卡片同屏渲染
2. `v-if="..."`（单独条件而非 v-else-if）—— 仍可能与图表卡片同屏（条件互斥不保证）
3. 用 `v-show` 替代 `v-else-if` —— 占用 DOM 空间，影响布局

## 后果

### 正面
- `app/pages/chat/dashboard/{daily,weekly,monthly,annual}Report.vue` 4 文件同型修复落地（E2E-15 阶段 1.1，PR #46）
- 静态契约钉 `app/pages/chat/dashboard/e2e-15-dashboard-reports-contract.architecture.test.ts` 4 文件 × 3 断言 = 12 用例，防回归
- 12/12 PASS（vitest）

### 风险
- 新增 dashboard / report 页面时若忘记该模式，**静态契约钉无法自动覆盖**（按文件名硬编码）—— 后续阶段应**扩展契约钉为动态扫描所有 dashboard 文件**
- 跨页契约一致性依赖人工 review + CI 跑契约钉

### 配套行动
- E2E-15 plan.md §4.1 + §5 测试点 #9 明确钉此模式
- 后续阶段（E2E-16 多模态 / E2E-17 数字人）涉及仪表盘卡片时须遵循本 ADR

## 关联

- E2E-F-14：原 bug 触发点（chat/user/index.vue 已修）
- E2E-F-91：E2E-13 弱断言放过同源问题的反例教训
- E2E-15 stages/e2e-15-reports-dashboard/plan.md §6 阶段 1.1

## 变更记录

- 2026-09-21：决策落地（E2E-15 阶段 1.1，PR #46，commit 4756028 + 本 ADR amend）