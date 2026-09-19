# ADR · 2026-09 · 前端 UI 组件策略：不引入 UI 框架，统一原生 + 设计 Token

> **本文档是追认+收口记录**：`docs/legacy-plans/landed/elementplus-to-native.md`
> （2026-07-17）早已声明"Element Plus 全部下线，UI 全部用 Vue 原生 + 设计 Token"，
> 但该计划**未完全落地**——2026-09-19（E2E-11）实测发现 `chat/user/index.vue`
> 仍在使用未解析的 `<el-dialog>` / `<el-upload>`，且 `package.json` 仍依赖
> `@element-plus/icons-vue`。本 ADR 把这条既成事实的架构约定正式登记，
> 并明确残留项的处置口径。
>
> **决策状态**：✅ **Accepted**（2026-07-17 实施；2026-09-19 追认补档 + 残留收口）
> **登记位置**：[decisions.md](../decisions.md) 决策 25

---

## 一、背景

原计划（`.trae/documents/frontend-refactor/elementplus-replacement.md`，2026-07-17）
决定：**完全不下任何新 UI 库**，交互组件一律用原生 HTML + Token 化 CSS 替代。理由三条：

| 问题 | 表现 |
|------|------|
| 视觉陈旧 | Element Plus 默认风格偏"中后台"，与情绪陪伴类产品调性不符 |
| 设计语言割裂 | 大量 `:deep(.el-*)` 强制覆盖，难以维持 Quiet Companion 设计语言 |
| 性能/包体 | 完整 Element Plus 体积大，按需引入也带来 tree-shaking 不稳定 |

该计划被标记为 `status: landed`，但 **2026-09-19 E2E-11 实测证明它没有完全落地**：

```
[Vue warn]: Failed to resolve component: el-dialog
[Vue warn]: Failed to resolve component: el-upload
```

未解析的组件产生两个**静默**后果（都是浏览器实测才发现的）：

1. `<el-dialog>` 被当作普通自定义元素**内联渲染** ⇒ 所谓"弹框"恒可见，无遮罩、无模态语义；
2. `<template #footer>` 命名插槽对非组件宿主被**静默丢弃** ⇒ 「保存资料 / 确认退出 / 取消 / 留下」
   四个按钮在 DOM 中**根本不存在**（实测 count 全为 0）——用户无法保存资料、无法从该页退出登录。

## 二、决策

**维持并正式确认**：不引入任何 UI 组件框架（包括 Element Plus、Naive UI 的业务组件），
前端交互组件统一走：

| 场景 | 采用方案 | 参考实现 |
|------|---------|---------|
| 模态弹框 | `<Teleport to="body">` + `v-if` 遮罩 + `role="dialog"` `aria-modal="true"` | `app/components/SecurityQuestionDialog.vue` |
| 文件选择 | 原生 `<input type="file">`（隐藏，由按钮触发，`accept` 限定类型） | `app/pages/chat/user/index.vue` |
| 通知/toast | 项目自有 `useNotify` + `NotifyHost.vue`（需挂在**每个** layout 上） | `app/layouts/{default,nav}.vue` |
| 图标 | 内联 SVG（`stroke="currentColor"`），不用图标字体 | `app/pages/chat/conversation/new.vue` |
| 按钮/输入 | `.ee-btn` / `.ee-field` / `.ee-input` + CSS Token | `app/assets/scss/global.scss` |

**残留项处置口径**（本次收口）：

| 残留 | 处置 |
|------|------|
| `chat/user/index.vue` 的 `<el-dialog>` / `<el-upload>` | ✅ 已在本 ADR 同批改为原生（E2E-11） |
| `package.json` 的 `@element-plus/icons-vue` 依赖 | ⏸️ **保留**——`app/components/face/FaceCamera.vue` 仍 `import { CircleClose }`。按本 ADR，"图标用内联 SVG"是目标态，该文件属待迁移项，已登记账本 |
| `nuxt.config.ts` 的 `transpile: ['@element-plus/icons-vue']` | ⏸️ 随上一项一并保留（依赖移除后才能删） |

**新增约束**（防止再次静默漂移）：

- 页面中**不得出现** `<el-*>` 标签。已加静态契约测试
  `app/pages/chat/user/e2e-11-my-space-contract.architecture.test.ts` 钉住（断言模板段无 `el-dialog`/`el-upload`）；
- 新增 layout 必须挂 `<NotifyHost />`，否则该 layout 下所有 `notify()` 静默
  （契约钉：`app/layouts/nav.test.ts`）。

## 三、后果

**正面**：

- 消除"未解析组件 → 插槽静默丢弃 → 操作按钮消失"这一类**只在浏览器里才暴露**的缺陷；
- 弹框/文件上传/图标实现方式统一，新增页面无歧义；
- 不引入 UI 框架运行时体积。

**代价 / 遗留**：

- 弹框需要自己维护 `Teleport` + 遮罩 + 焦点/滚动锁（当前实现未做 focus trap，属可访问性欠账）；
- `@element-plus/icons-vue` 依赖仍在（见上表），仓库尚未达到"零 Element Plus 依赖"；
- 静态契约测试是**文本级**断言（扫源码），能防"又写了 `<el-dialog>`"，
  但不能防"原生弹框写错"。行为正确性仍需 Playwright 覆盖（`e2e/my-space.spec.ts`）。

## 四、与既有决策的关系

- 本 ADR **追认** `docs/legacy-plans/landed/elementplus-to-native.md` 的既有决策，
  不改变其方向，只补上"未完全落地"的事实与残留处置；
- 与 **决策 24（Nuxt SSR）** 无冲突：SSR 下原生组件同样可服务端渲染，
  弹框因 `v-if` 初始为 false 不参与首屏；
- 与 **E2E-04（前端工程化门槛）** 衔接：a11y 基线属该阶段，
  本 ADR 遗留的 focus trap 欠账应在那里收口。
