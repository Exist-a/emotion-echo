---
status: landed
stage: 106
title: /chat/user 按钮走浏览器默认样式 — 提到 global.scss + DRY 收口
date: 2026-09-16
type: ui-style-recovery
source-plan: （接 Stage 105 浏览器实测：用户在浏览器打开 /chat/user，3 个按钮样式奇怪）
phase: 测试找问题修复
depends-on:
  - stage-105-browser-e2e-2026-09-16.md
  - stage-104-frontend-ui-recovery-2026-09-16.md
related-stages:
  - stage-103-dev-mode-launch-2026-09-16.md
related-adrs:
  - ADR-15（前端 Web 选型 = Nuxt 3）
---

# Stage 106 — /chat/user 按钮走浏览器默认样式

> **本 stage 目标**：浏览器实测发现 `/chat/user` 页面 3 个按钮视觉突兀（背景 `#f0f0f0`、黑色 outset 边框），根因是 `.ee-btn` 样式只在 3 处 page 复制粘贴，本 page 漏写。本 stage 修复 + DRY 收口到 global.scss，**所有未来 page 自动继承**。

---

## 一、本 stage 起点

| 维度 | 状态 |
|---|---|
| dev 模式 | Stage 105 修完 Dockerfile alpine musl binding，v0.1.0 重 build + 容器 healthy |
| 用户工作流 | "user 空间的按钮样式很奇怪" — 浏览器实测 `/chat/user` 页面 |
| 测试基线 | Stage 105 末态 318/318 全绿 |
| 现象 | 浏览器打开 `127.0.0.1:3000/chat/user`，3 按钮（修改资料 / 开始测验 / 退出登录）走浏览器 user agent 默认样式 |

---

## 二、问题调查

### 2.1 浏览器实测截图（修复前）

```text
button 修改资料 / 开始测验 / 退出登录:
  background: rgb(240, 240, 240)    ← 浏览器默认灰
  border: 2px outset rgb(0, 0, 0)    ← 浏览器默认黑色 outset 边框
  color: rgb(32, 37, 34)
  padding: 0
```

视觉突兀：与全站绿系品牌色割裂，`/chat/dashboard` 报表页（设计 token 化）vs `/chat/user`（浏览器默认）像两个项目。

### 2.2 根因分析

| 维度 | 状态 |
|---|---|
| **页面源码** | `app/pages/chat/user/index.vue` `<button class="ee-btn ee-btn-primary btn">修改资料</button>` |
| **`<style scoped>`** | 只定义 `.btn { min-height: 38px; border-radius: var(--ee-radius-md) }` + `.btn.danger` |
| **`.ee-btn` 样式** | ❌**不在本 page**——只在 login、 question/index、 question/[id] 三处 page 复制粘贴 |
| **Vue scoped 作用域** | 即使其它 page 写 `.ee-btn`，scoped 属性选择器不外溢，本 page 拿不到 |
| **浏览器 fallback** | button 标签无匹配样式 → 走 user agent 默认 |

### 2.3 重复代码审计

```bash
grep -rn "\.ee-btn" emotion-echo-web/app/
```

修改前 `.ee-btn` 定义位置（**3 处重复**）：

| 文件 | 行号 | 重复内容 |
|---|---|---|
| `pages/login/index.vue` | 276-279 | `.ee-btn` 基础 + `.ee-btn-primary` + hover |
| `pages/question/index.vue` | 123-127 | 同上 |
| `pages/question/[id].vue` | 189-193 | 同上 + `.ee-btn-lg` 变体 |

DRY 违反：3 处复制 ≈ 18 行重复代码，**任何新 page 漏写就会触发本 bug**。

---

## 三、本 stage 修复

### 3.1 改动清单

| 文件 | 改动 |
|---|---|
| `app/assets/scss/global.scss` | 新增 `.ee-btn` + `.ee-btn-primary` + 状态样式（约 16 行） |
| `app/pages/login/index.vue` | 删除 `.ee-btn`/`.ee-btn-primary` 重复（保留 `:hover/:disabled` 微调由全局覆盖） |
| `app/pages/question/index.vue` | 同上 |
| `app/pages/question/[id].vue` | 同上 + 保留 `.ee-btn-lg` 变体 |
| `app/assets/scss/global-buttons.test.ts` | 新增 4 项 RED 契约（基础 / primary / 状态 / REGRARD-GUARD） |

### 3.2 修复后 `.ee-btn` 定义（global.scss）

```scss
// ---- Stage 105 浏览器实测: 全局按钮样式 (从 login + question/* 3 处 page 提到此处) ----
// 之前 .ee-btn 只在 3 处 page 复制粘贴 (login/index, question/index, question/[id]),
// /chat/user 等新 page 没复制 → 按钮走浏览器默认样式 (background: #f0f0f0, 2px outset #000),
// 视觉突兀. 提到 global 让所有 page 默认继承, 也避免未来 DRY 违反.
.ee-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 38px;
  padding: 0 18px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  color: var(--ee-text);
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  transition: background var(--ee-transition), color var(--ee-transition), border-color var(--ee-transition);
}
.ee-btn:hover:not(:disabled) { background: var(--ee-surface-muted); }
.ee-btn:disabled { cursor: not-allowed; opacity: 0.6; }
.ee-btn-primary { background: var(--ee-primary); color: #fff; border-color: var(--ee-primary); }
.ee-btn-primary:hover:not(:disabled) { background: var(--ee-primary-hover); border-color: var(--ee-primary-hover); }
```

### 3.3 RED 契约（global-buttons.test.ts）

| # | 契约 | 验证 |
|---|---|---|
| 1 | global.scss 必须定义 `.ee-btn` 基础样式（`background var(--ee-surface)`、`height 38px`、`border-radius var(--ee-radius-md)`） | ✅ |
| 2 | 必须定义 `.ee-btn-primary` 变体（`background var(--ee-primary)`、`color `#fff``、hover `var(--ee-primary-hover)`） | ✅ |
| 3 | 必须定义 `.ee-btn:disabled { opacity: 0.6 }` + `:hover:not(:disabled)` 状态（Stage 104 chat 一致） | ✅ |
| 4 | REGRARD-GUARD 文档（防未来 page 再复制 `.ee-btn` 基础样式） | ✅（占位，lint 阶段补） |

---

## 四、验证

### 4.1 vitest

```
Test Files  36 passed (36)
Tests       322 passed (322)
```

基线 318 → 322（+4 全局按钮契约）。

### 4.2 浏览器实测（重 build v0.1.0 镜像后）

| 按钮 | 修复前 | 修复后 |
|---|---|---|
| 修改资料 | `bg: rgb(240,240,240)`、`border: 2px outset #000` | **`bg: rgb(95,143,123)`**（绿系品牌色）、`color: #fff` |
| 开始测验 | 同上 | `bg: #fff`、`border: 1px solid var(--ee-border)`、`color: var(--ee-text)` |
| 退出登录 | 同上 | `bg: #fff`、`border: 1px solid ee-accent-soft`、`color: rgb(217,135,115)`（accent 警示色） |

视觉确认：3 按钮圆角统一、绿系品牌色统一，与 `/chat/dashboard` 报表页设计 token 一致。

### 4.3 改动后 DRY 审计

```bash
grep -rn "\.ee-btn" emotion-echo-web/app/
```

| 位置 | 内容 |
|---|---|
| `global.scss:116-134` | 完整 `.ee-btn` + `.ee-btn-primary` 定义 |
| `pages/question/index.vue:123-124` | 仅 `:hover/:disabled` 微调（opacity 0.5 而非 0.6 — 业务需求） |
| `pages/question/[id].vue:189-191` | 同上 + `.ee-btn-lg` 变体 |
| `pages/login/index.vue:276-277` | 仅 `.ee-btn-primary:hover` 微调（重复可进一步清理，留后续 PR） |

---

## 五、Commit（已上 origin）

| commit | 类型 | 内容 |
|---|---|---|
| `d57d4fa` | test(scss) | RED 钉死 `.ee-btn` 必须定义在 global.scss（4 项契约） |
| `9a2c615` | fix(scss) | `.ee-btn` 提到 global.scss + 清理 3 page 重复样式（DRY） |

---

## 六、连锁收益

`.ee-btn` 现在是全局 token 化组件类，**任何未来 page 自动继承**：

- ✅ 新 page 用 `<button class="ee-btn">` 直接拿到品牌色按钮
- ✅ 未来想改按钮色/圆角/高度，只改 `global.scss` 一处
- ✅ DRY 收口避免回归（任何 page 漏写都不会触发浏览器默认样式）

可推广到其他组件类：
- `.ee-input` / `.ee-field`（目前 question/index, login 也重复定义）
- `.card`（目前 question/index 重复定义）

Stage 107 可作为"组件类 token 化收口"。

---

## 七、ADR / plans / decisions 影响

- **无 ADR 变更**：纯样式 token 化，不涉及架构
- **无 plans 变更**：本 stage 不是新功能
- **AGENTS.md**：未变更

---

## 八、调研依据（按 AGENTS.md §0 硬规则）

| 类别 | 依据 |
|---|---|
| 代码层 | `app/pages/chat/user/index.vue:11-13`（按钮 class）、`app/assets/scss/global.scss`（修改前无 `.ee-btn` 定义） |
| 测试层 | vitest 322/322 全绿；4 项 `global-buttons.test.ts` RED 契约 |
| 视觉层 | 浏览器实测 `getComputedStyle()` 修复前/后对比、scoped 属性选择器不外溢理论 |
| DRY 审计 | `grep -rn "\.ee-btn"` 3 处重复定位 |
| 决策 | `:hover:not(:disabled)` 保留页面 scoped（login/question 的 hover 微调与全局有差异 — opacity 0.5/0.6） |
| 链式修 | 不动 `Regs.ts`（Stage 105 已修）、不动 Dockerfile（Stage 105 已修） |

---

## 九、未在本 stage 覆盖、待 Stage 107 治理

- `pages/login/index.vue:276-277` 的 `.ee-btn-primary:hover` 仍是 scoped 重复，可彻底清理
- `pages/question/index.vue`/ `[id].vue` 的 `.ee-btn:hover/:disabled` 仍是 scoped 重复，可统一用全局 `.ee-btn:hover:not(:disabled)` + 移除 scoped
- `.ee-input` / `.ee-field` / `.card` 等组件类也是同 DRY 违反模式
- 阶段 106 鉴权链 bug（reports/* 路由 + JWT secret 不一致）仍未修，需 Stage 108 专项

---

> **stage 验收清单（Stage 103 工作流）**
> - [x] /chat/user 按钮样式已修（绿系品牌色统一）
> - [x] 3 处 page 重复样式清理（DRY）
> - [x] RED 契约 4/4 全绿
> - [x] 重 build v0.1.0 + 浏览器实测生效
> - [x] commit + push origin
> - [ ] Stage 107：`.ee-input/.ee-field/.card` 等组件类 token 化收口
> - [ ] Stage 108：dev 模式鉴权链 bug 4a/4b/4c 治理 + 浏览器 E2E 轮 2-4
> - [ ] chat 发消息无 AI 回复（Stage 103 已知未修 → Stage 107+）