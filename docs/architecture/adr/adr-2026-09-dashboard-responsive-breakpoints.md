---
adr: 2026-09-dashboard-responsive-breakpoints
title: 仪表盘网格响应式断点 = ref 追踪窗口宽度（不得直接读 window.innerWidth）
status: accepted
date: 2026-09-21
owners: [frontend]
references: [E2E-15/stages/e2e-15-reports-dashboard/iab-compliance-report.md]
supersedes: null
---

# ADR-2026-09 · 仪表盘网格响应式断点 = ref 追踪窗口宽度

## 上下文

`app/components/report/chartsCard.vue` 负责仪表盘图表的响应式网格布局，断点约定：

| 视口宽度 | 列数 |
|---------|------|
| < 992px | 1 |
| 992–1600px | 2 |
| ≥ 1600px | 3 |

**E2E-15 合规补完轮的 IAB 实测发现缺陷**（详见 [iab-compliance-report.md §4 缺陷 A](../../e2e-roadmap/stages/e2e-15-reports-dashboard/iab-compliance-report.md)）：

原实现：
```js
const currentColumns = computed(() => {
  const width = window.innerWidth   // ← 非 Vue 响应式依赖
  if (width < 992) return 1
  if (width < 1600) return 2
  return 3
})
const handleResize = () => {
  // currentColumns是计算属性，会自动更新   ← 错误假设
}
```

**Vue 的 `computed` 只追踪响应式依赖**；`window.innerWidth` 是普通全局属性 ⇒ 首次渲染后不再重算。
实测（IAB）：以 1280px 加载 weeklyReport 后 `setViewportSize(800)` → 网格仍为 `190.5px 190.5px`（2 列，应 1 列），图表被挤压在 ~190px 容器内。手动 `dispatchEvent(new Event('resize'))` 亦不重算。

**为什么既有测试没抓到**：`chartsCard.test.ts` 原有 2 条列数用例（`renders 1 column on narrow viewports` / `renders 3 columns on wide viewports`）均在 **mount 之前** `setInnerWidth` ⇒ 只覆盖首次渲染，未覆盖 resize 重算。

## 决策

**响应式布局尺寸必须经 ref/响应式状态参与 computed 计算，不得在 computed 内直接读取非响应式全局值**：

```js
const windowWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1600)

const currentColumns = computed(() => {
  const width = windowWidth.value
  if (width < 992) return 1
  if (width < 1600) return 2
  return 3
})

const handleResize = () => {
  if (typeof window !== 'undefined') windowWidth.value = window.innerWidth
}

onMounted(() => window.addEventListener('resize', handleResize))
onUnmounted(() => window.removeEventListener('resize', handleResize))
```

**禁止**：
1. 在 `computed` 内直接读 `window.innerWidth` / `document.documentElement.clientWidth` 等非响应式值
2. 写空 `handleResize` 并注释"computed 会自动更新"（这是错误假设，见上方实测）
3. SSR 场景中不带 `typeof window !== 'undefined'` 守卫地读窗口尺寸

## 后果

### 正面
- resize 后列数正确重算（IAB 复验：800px → `405px` 单列；1800px → `313.328px ×3`）
- 回归钉 `chartsCard.test.ts` +1 用例（`recomputes columns after window resize`）钉住该行为
- 保护所有使用 `<chartsCard>` 的页面（4 个 dashboard）

### 风险
- 其他组件若存在同类"computed 读非响应式全局值"写法，本 ADR 覆盖不到（本 ADR 只管 chartsCard）。**建议**：新增响应式布局组件时按本模式自查
- resize 监听需在 `onUnmounted` 移除（原实现已有，保留）

### 配套行动
- `chartsCard.vue` 按本 ADR 重构（TDD：RED → GREEN）
- `e2e/dashboard-reports.spec.ts` #14 加列数取值断言（原仅截图）
- E2E-15 report §11 + iab-compliance-report §4 记录

## 关联

- E2E-15 测试点 #14「跨视口响应式（<992 / 992-1600 / ≥1600 列数）」
- ADR `adr-2026-09-dashboard-empty-state.md`（同组件的空态渲染模式，D-27）
- 同类"弱断言放过"教训：E2E-F-91 / E2E-F-97

## 变更记录

- 2026-09-21：决策落地（E2E-15 合规补完轮，IAB 实测发现 → TDD 修复 → ADR 追认）
