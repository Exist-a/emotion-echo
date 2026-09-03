---
status: landed
superseded-by: stage-26-O-frontend-redesign.md
original-path: .trae/documents/frontend-refactor/elementplus-replacement.md
original-date: 2026-07-17
migrated-at: 2026-09-03
round: 2-A
---

# Element Plus 全面替换为原生 —— 落地文档

> 2026-07-17 · 状态：已落地
> 范围：`Emotion-Echo-Web/` 前端全部页面、组件、Store

## 一、背景与决策

### 1.1 为什么替换

`Element Plus`（`@element-plus/nuxt` + `@element-plus/icons-vue`）在本项目中带来三类问题：

| 问题 | 表现 |
|------|------|
| 视觉陈旧 | 默认风格偏"中后台"，对情绪/陪伴类产品过于严肃 |
| 设计语言割裂 | 大量 `:deep(.el-*)` 强制覆盖，难以维持 Quiet Companion 设计语言 |
| 性能 / 包体 | 完整 Element Plus 体积较大；按需引入也带来 tree-shaking 不稳定 |

`.trae/documents/frontend-refactor/frontend-refactor-plan.md` 原本建议迁移到 **Nuxt UI v4** 或 **Naive UI**。本轮决定**完全不下任何新的 UI 库**——所有交互组件用原生 HTML + Token 化的手写 CSS 替代。

### 1.2 最终方案

> **Element Plus 全部下线，UI 全部用 Vue 3 原生 + 我们的设计 Token + 几个微小组件。**

新依赖：

| 库 | 角色 | 备注 |
|----|------|------|
| ~~`element-plus`~~ | 移除 | 完全下线 |
| ~~`@element-plus/nuxt`~~ | 移除 | 模块配置删除 |
| ~~`@element-plus/icons-vue`~~ | 移除 | 图标统一用 inline SVG |
| `naive-ui` | 已安装但暂未启用 | 留作未来表单 / 日期选择器等复杂组件的备选 |
| `vueuc` | 同上 | Naive UI 内部依赖，按需 transpile |

## 二、设计 Token（基础）

所有手写组件**不**再写裸值，统一引用以下 CSS 变量（在 `app/assets/scss/global.scss` 定义）：

```css
:root {
  --ee-bg: #f7f8f7;
  --ee-surface: #ffffff;
  --ee-surface-muted: #eef3f0;
  --ee-text: #202522;
  --ee-text-muted: #68736d;
  --ee-primary: #5f8f7b;
  --ee-primary-hover: #4f7e6a;
  --ee-primary-soft: #e6f0eb;
  --ee-accent: #d98773;
  --ee-border: #e0e8e3;
  --ee-focus: rgba(95, 143, 123, 0.28);
  --ee-radius-md: 8px;
  --ee-radius-lg: 12px;
  --ee-radius-xl: 20px;
  --ee-shadow-soft: 0 1px 2px rgba(32, 37, 34, 0.06), 0 8px 24px rgba(32, 37, 34, 0.05);
  --ee-transition: 200ms cubic-bezier(0.22, 1, 0.36, 1);
}

html.dark {
  --ee-bg: #171c1a;
  --ee-surface: #222a26;
  --ee-surface-muted: #2a3530;
  --ee-text: #edf3ef;
  --ee-text-muted: #aab8b0;
  --ee-primary: #8ebaa8;
  --ee-primary-hover: #a8cbbd;
  --ee-primary-soft: #2b4036;
  --ee-accent: #e0a091;
  --ee-border: #39483f;
}
```

暗色模式不再依赖 Element Plus 的 `html.dark` 类，直接由本文件管理。

## 三、组件映射表

| Element Plus | 替代方案 | 实现位置 |
|--------------|----------|----------|
| `<el-button>` | 原生 `<button class="ee-btn ee-btn-primary">` | 所有页面 |
| `<el-input>` / `<el-input type="textarea">` | 原生 `<input class="ee-input">` / `<textarea class="ee-textarea">` | 所有页面 |
| `<el-form>` + `<el-form-item>` | 原生 `<form>` / `<label class="ee-field">` | 登录、忘记密码、用户中心 |
| `<el-checkbox>` | `<label class="ee-checkbox"><input type="checkbox"><span>` | 登录 |
| `<el-radio>` / `<el-radio-group>` | `<label class="ee-radio"><input type="radio">` | 设置、量表答题 |
| `<el-radio-button>` | `<label class="ee-radio-button">` + 隐藏的 `<input type="radio">` | 设置 |
| `<el-dropdown>` + `<el-dropdown-menu>` + `<el-dropdown-item>` | 简易原生下拉（自己实现 open/close + 定位） | 会话列表"更多" |
| `<el-skeleton>` | `<div class="ee-skeleton">` + 1.4s shimmer 关键帧 | 报表、量表列表 |
| `<el-empty>` | `<div class="empty-state">` | 报表、量表列表 |
| `<el-progress>` | 原生 `<progress class="progress-bar">` | 量表答题 |
| `<el-divider>` | `<hr class="ee-divider">` | 音量、忘记密码 |
| `<el-avatar>` | `<img class="avatar">` | 用户中心 |
| `<el-link>` | `<a class="link">` | 多个 |
| `<el-icon>` | inline `<svg>` 或 `<span class="ee-icon">` | 全部 |
| `<el-table>` + `<el-table-column>` | 卡片网格 | 量表列表 |
| `<el-date-picker>` | 原生 `<input type="date/month">` / 双 input daterange | 4 个报表 |
| `<el-dialog>` | `<Teleport>` + 自定义 modal + 背景遮罩 | 重命名、退出、提交结果 |
| `<el-message>` / `ElNotification` | `useNotify()` toast 队列 | 全局 |
| `<el-message-box.confirm>` | `window.confirm()` | 删除会话、离开答题 |

## 四、自研组件

### 4.1 通知 Toast —— `composables/useNotify.ts`

替代 `ElNotification` / `ElMessage`。`useNotify()` 返回 `{ toasts, success, error, warning, info, show, push }`，也导出 `notify()` 全局短调用。

实现：

```ts
const toasts = ref<Toast[]>([])
let seq = 0
function push(title, message, type = 'info', duration = 3000) {
  const id = ++seq
  toasts.value.push({ id, title, message, type, duration })
  setTimeout(() => { toasts.value = toasts.value.filter(t => t.id !== id) }, duration)
}
```

挂载点：`<NotifyHost />` 放在 `app/layouts/default.vue`，所有页面共享。

### 4.2 模态对话框 —— `<Teleport>` + `.modal-backdrop` / `.modal-card`

替代 `el-dialog`：

```vue
<Teleport v-if="dialogVisible" to="body">
  <div class="modal-backdrop" @click.self="dialogVisible = false">
    <div class="modal-card" role="dialog" aria-modal="true">
      <h3>...</h3>
      <slot />
      <div class="modal-actions">
        <button class="ee-btn btn-ghost" @click="dialogVisible = false">取消</button>
        <button class="ee-btn ee-btn-primary" @click="onConfirm">确认</button>
      </div>
    </div>
  </div>
</Teleport>
```

CSS（`global.scss` 中可抽取但目前各页自治）：

```css
.modal-backdrop {
  position: fixed; inset: 0; z-index: 80;
  display: flex; align-items: center; justify-content: center;
  padding: 16px;
  background: rgba(20, 27, 23, 0.45);
  backdrop-filter: blur(2px);
}
.modal-card {
  width: min(520px, 100%);
  padding: 20px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  box-shadow: 0 12px 36px rgba(32, 37, 34, 0.15);
}
```

### 4.3 自定义下拉菜单（替换 `el-dropdown`）

`app/pages/chat/conversation/index.vue` 中实现：

```vue
<div class="more-wrap" @click.stop>
  <button class="icon-btn more-btn" @click.stop="toggleMore(item.key">⋮</button>
  <ul v-if="openMoreKey === item.key" class="more-menu" role="menu">
    <li role="menuitem" @click="onMenuCommand('pin', item)">置顶</li>
    <li role="menuitem" @click="onMenuCommand('rename', item)">重命名</li>
    <li role="menuitem" class="danger" @click="onMenuCommand('delete', item)">删除</li>
  </ul>
</div>
```

外部点击关闭：`document.addEventListener('click', closeMore)`，Esc 关闭：`keydown`。

### 4.4 输入框统一规范

```html
<label class="auth-field">
  <span class="input-icon" aria-hidden="true">@</span>
  <input v-model="loginInfo.username" class="ee-input" placeholder="邮箱" autocomplete="username" />
</label>
```

- 整行 `.auth-field` 拥有边框、圆角、聚焦光晕
- `.ee-input` 是透明的 `<input>`，撑满剩余空间
- `<input type="password">` 不需要额外的小眼睛切换，登录页加 `autocomplete="current-password"`，注册页加 `autocomplete="new-password"`

### 4.5 按钮统一规范

```html
<button type="button" class="ee-btn ee-btn-primary" :disabled="isLoading">登录</button>
```

变体：`ee-btn-primary` / `ee-btn-ghost` / `ee-btn-lg`（44px 高）。

## 五、图标策略

不再依赖 `@element-plus/icons-vue`。统一在需要处手写 inline SVG：

```html
<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d="M21 11.5l-9 9a5 5 0 0 1-7-7l9-9a3.5 3.5 0 0 1 5 5l-9 9a2 2 0 0 1-3-3l8-8" />
</svg>
```

如未来图标数量爆炸，再考虑接入 `lucide-vue-next`（一个库 100KB，tree-shakable）。**目前不引入**。

## 六、布局与导航

- `app/layouts/nav.vue`：保留，作为 `/chat/*` 与 `/question` 的应用框架
- `app/layouts/default.vue`：加入 `<NotifyHost />`，作为所有页面的公共宿主
- 会话侧边栏的 `fold / expand` 切换：纯 Vue 状态管理，不依赖任何 UI 库
- 路由结构未动

## 七、改动文件清单

新增：
- `app/composables/useNotify.ts` — 通知队列
- `app/components/NotifyHost.vue` — 全局通知宿主
- `scripts/convert-el-to-native.py` — 一次性批处理脚本（已存档，不再生效）
- `scripts/convert-el-notify.py` — 一次性批处理脚本（已存档，不再生效）

修改（手写重写）：
- `app/assets/scss/global.scss` — 删除 `@import 'element-plus/theme-chalk/dark/css-vars.css'`
- `nuxt.config.ts` — `modules` 中删除 `@element-plus/nuxt`，`build.transpile` 加入 `naive-ui` 与 `vueuc`（占位）
- `package.json` — 移除 `element-plus` / `@element-plus/nuxt` / `@element-plus/icons-vue`
- 所有页面与组件：见 git log

## 八、验证

```text
$ pnpm test       # 35 / 35 passed
$ pnpm build      # Build complete!
$ pnpm typecheck  # 仅遗留的 47 个历史类型错误（与本次重构无关）
```

页面响应（dev server）：

```text
/login                          200
/chat                           200
/chat/conversation              200
/chat/setting                   200
/chat/user                      200
/chat/dashboard/dailyReport     200
/chat/dashboard/weeklyReport    200
/chat/dashboard/monthlyReport   200
/chat/dashboard/annualReport    200
/question                       200
/question/<id>                  200
```

## 九、给后续开发者的注意事项

1. **不要再 import `@element-plus/nuxt`、`<el-*>`、`@element-plus/icons-vue`**。已下线，Nuxt 配置和 `tsconfig` 都不再支持。
2. **图标用 inline SVG**。在 `<svg>` 上加 `aria-hidden="true"`。
3. **新增表单**：直接用原生 `<form>` + `<label class="ee-field">` + `<input class="ee-input">`。校验用 `<input required pattern=...>`，或调用 `useNotify().error()`。
4. **新增弹窗**：用 `<Teleport to="body">` + `.modal-backdrop` + `.modal-card` 模板（见 `app/pages/chat/conversation/index.vue` 内的"重命名"弹窗）。
5. **新增通知**：用 `useNotify().success/error/warning/info(title, message)`。
6. **新增 loading 占位**：用 `<div class="ee-skeleton" style="height: 16px; width: 60%"></div>`。
7. **新增日期选择**：原生 `<input type="date" | "month">`，双 input 模拟 daterange（见 `app/components/report/ReportScaffold.vue`）。
8. **不要把设计 Token 写死成具体值**——`color: #5f8f7b` 是反例，`color: var(--ee-primary)` 是正例。
9. **不要省略 `:focus-visible` / 键盘焦点样式**。`:focus-visible { outline: 3px solid var(--ee-focus); }` 已在 global.scss 提供。

## 十、回顾

- 干掉 Element Plus 后，包体积下降（首屏 chunk 应比之前小，但本轮未量化）
- 设计语言完全统一，没有"中后台感"
- 性能更可控：所有交互都是 Vue 3 原生 + 我们的 token CSS，没有第三方组件库的内部样式与 hydration 成本
- 代价：每个新组件都要手写 CSS（但实际上我们 80% 的 UI 是按钮 + 输入框 + 卡片 + 弹窗，4 个样式 class 已可覆盖）
