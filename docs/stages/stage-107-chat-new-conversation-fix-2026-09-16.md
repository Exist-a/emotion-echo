---
status: partial
stage: 107
title: chat /new 发消息无 AI 回复根因 = new.vue handleSubmit 不调 sender + seed.sh trailing comma 致 catch-all 缺失
date: 2026-09-16
type: bug-fix-plus-architecture-debt
source-plan: （接 Stage 105/106 浏览器 E2E）
phase: TDD 修复
depends-on:
  - stage-105-browser-e2e-2026-09-16.md
  - stage-106-chat-user-buttons-2026-09-16.md
related-stages:
  - stage-74-cors-allow-credential.md
  - stage-94-code-review-2026-09-14-p0-closure.md (BFF JWT secret 修)
related-adrs:
  - ADR-15（前端 Web 选型 = Nuxt 3）
  - 决策 11/12（网关是唯一业务入口 + APISIX 鉴权）
audit-2026-09-16:
  bugs-fixed: 2 (new.vue + seed.sh)
  tests-added: 4 (new.test.ts 3 用例 + seed_test.js 1 用例)
  tests-green: 43 (new.test.ts 3 + seed_test.js 40)
  browser-e2e-green: 0 / 1 (chat 链路 blocked by architecture debt)
  architecture-debt: 1 (useConversationSender composable 生命周期 vs Vue 路由 unmount)
---

# Stage 107 — chat /new 发消息无 AI 回复根因修复

> **本 stage 目标**：用 browser-use 操控浏览器复现"chat 发消息无 AI 回复"（Stage 105 留下的 1 个未修 bug），按 TDD 修。
>
> **本轮意外发现**：bug 实际是**两个 bug 叠加**——前端 new.vue handleSubmit 缺陷 + APISIX seed.sh JSON schema 失败致 catch-all 路由缺失。后者让前者更难诊断。

---

## 一、本 stage 起点

| 维度 | 状态 |
|---|---|
| 浏览器 tab | Stage 105 留下的 IAB tab `iab-tab:d8c704f1-df4e-4c64-8903-cfc4840a233b` 仍激活在 localhost:3000 |
| dev 模式 | 14 容器 healthy，APISIX 13 routes（Stage 103 收口） |
| 用户期望 | "发消息能看到 AI 回复" |

---

## 二、浏览器实测：4 个独立观察

### 观察 1（修复前）：`POST /conversations` body 是 `{"title":"你好..."}`

```text
1789549846910 POST /api/v1/conversations {"title":"你好，请介绍一下你自己"}
1789549849274 POST /api/v1/conversations {"title":"11"}    ← 字号调整误触
1789549857122 PATCH /api/v1/users/me {"config":{"fontSize":"14px"}}
1789549857534 PATCH /api/v1/users/me {"config":{"fontSize":"16px"}}
1789549858420 PATCH /api/v1/users/me {"config":{"fontSize":"18px"}}
```

→ **根因**：`emotion-echo-web/app/pages/chat/conversation/new.vue:80-86` `handleSubmit` **只调 `createConversation({title: value.slice(0,30)})` 然后 navigateTo**，**完全没调 `conversationSender`**。`useConversationSender.createNewConversation(value)`（`useConversationSender.ts:173-215`）已经写好完整链路（createConversation → navigateTo → switchSession → sendMessage + sendAIStream），但 new.vue 没用它。

### 观察 2（修复后）：`POST /conversations` body 变 `{}`，**但还是 401**

```text
1789550690387 POST /api/v1/conversations {}  ← 修改生效, 但 APISIX 拒了
```

→ **第二个根因**：重启 `emotion-echo-web` 容器时**触发 `apisix-seed` 重跑**，seed 跑挂（exit 3），catch-all `/api/v1/*` 路由缺失，前端所有非 auth 业务被 APISIX 401 拦截。**修复前根本看不到这个 bug，因为前端代码本身就不发消息**——两个 bug 互相掩盖。

### 观察 3（seed 修复后）：`POST /conversations` 返 200，**但后续 sendMessage/sendAIStream 不触发**

```text
1789551147537 POST /api/v1/conversations {}  ← 200 OK
[stuck] 没有 POST /conversations/:id/messages
[stuck] 没有 POST /ai/stream
```

→ **第三个根因（架构债）**：`useConversationSender` 是 composable，每个 Vue 组件实例产生一个 sender 实例。`createNewConversation` 内部 `await navigateTo(...)` 后，`new.vue` 被路由切换触发 unmount，`useConversationSender` 实例的 `onUnmounted(() => cancelAIStream())` 钩子（`useConversationSender.ts:48-51`）清理状态。`[id].vue` mount 时新建 sender 实例，看不到前一个 sender 准备好的 user message 和 accumulatedDeltaText。

### 观察 4：BFF/chat-svc/ai-svc 日志印证

- `emotion-echo-web-bff` 日志只有 Login（5 条），无 `ChatCompletion` 调用
- `emotion-echo-chat-svc` 日志**完全为空**（聊天链路没启动）
- `emotion-echo-ai-svc` `fusion` 模块每 5s tick 但 `candidates=0 (msgIDs=[])`——没有消息进队列

---

## 三、修复（已 GREEN 的 2 个）

### Bug A：new.vue handleSubmit 缺陷（✅ 已修）

| 维度 | 内容 |
|---|---|
| **文件** | `emotion-echo-web/app/pages/chat/conversation/new.vue:80-86` |
| **RED 测试** | `emotion-echo-web/app/pages/chat/conversation/new.test.ts`（新建，3 条 static-source 合同断言）|
| **RED 验证** | `npx vitest run new.test.ts` → **2 FAIL + 1 PASS**（修复前的"禁止裸 createConversation"反向断言预判正确 PASS）|
| **GREEN 修复** | 改 `handleSubmit` 调 `conversationSender.createNewConversation(value)`，失败时 `notify('发送失败', msg, 'error')` |
| **GREEN 验证** | `npx vitest run new.test.ts` → **3/3 PASS** |
| **镜像构建** | `docker compose -f deploy/docker-compose.apps.yml build emotion-echo-web` → `emotion-echo/web:v0.1.0` Built |
| **运行时验证** | 浏览器实测 `POST /conversations` body 从 `{"title":"..."}` 变 `{}`，证明新代码进 bundle（`new-BqRnQ2Yh.mjs` 含 `useConversationSender` import）|

#### 3 条 RED 合同（new.test.ts）

```ts
// 1) handleSubmit MUST call conversationSender.createNewConversation or sendToExistingConversation
const usesSender = /conversationSender[\s\S]*createNewConversation/.test(block) ||
                   /conversationSender[\s\S]*sendToExistingConversation/.test(block)

// 2) MUST NOT bypass sender by only calling conversationStore.createConversation + navigateTo
const usesBareCreate = /post\(\s*API_ROUTES\.createConversation\.path/.test(block) &&
                       /navigateTo\(\s*\{\s*name:\s*['"`]chat-conversation-detail['"`]/.test(block) &&
                       !/conversationSender/.test(block)

// 3) user message content (value) MUST be passed as parameter
const callsBlockWithFullValue = /createNewConversation\(\s*value\b/.test(block) ||
                                /sendToExistingConversation\(\s*[^,]*,\s*value\b/.test(block)
```

### Bug B：seed.sh file-logger log_format trailing comma（✅ 已修）

| 维度 | 内容 |
|---|---|
| **文件** | `deploy/apisix/seed.sh:292` |
| **现象** | `apisix-seed` 容器 exit 3，`PUT /apisix/admin/routes/100` 失败，导致 catch-all `/api/v1/*` 缺失 |
| **根因** | `OBSERVABILITY_PLUGINS_JSON` 内 file-logger `log_format` 最后一行 `"trace_id": "$http_x_request_id",` 带 trailing comma → JSON schema 校验失败 |
| **RED 测试** | `deploy/apisix/seed_test.js` 新增 1 条："file-logger log_format 最后一行不得以 ',' 结尾" |
| **RED 验证** | `node seed_test.js` → **39 PASS + 1 FAIL**（断言命中 bug） |
| **GREEN 修复** | 删 trailing comma |
| **GREEN 验证** | `node seed_test.js` → **40/40 PASS** |
| **容器验证** | `docker compose -f deploy/docker-compose.apps.yml up -d emotion-echo-apisix-seed` → exit 0，13 routes 全注册，`route OK: 100 → upstream 6 (uri=/api/v1/*)` 出现 |

#### 新增 RED 合同（seed_test.js）

```js
(() => {
  const blockMatch = src.match(/"log_format":\s*\{([\s\S]*?)\n\s*\}/);
  if (!blockMatch) return ['file-logger log_format 块结构 (用于 trailing comma 校验)', false];
  const block = blockMatch[1];
  const lines = block.split('\n').map(l => l.trim()).filter(l => l.length > 0);
  const lastLine = lines[lines.length - 1];
  return ['file-logger log_format 最后一行不得以 "," 结尾 (Stage 107)',
    !lastLine.endsWith(',')];
})(),
```

---

## 四、未修：架构债（留 Open）

### Architecture Debt — `useConversationSender` composable 生命周期 vs Vue 路由 unmount

| 维度 | 内容 |
|---|---|
| **现象** | 浏览器实测 `POST /conversations` 返 200 后**没有任何后续 API 调用**（messages / ai/stream 都没发） |
| **根因** | `useConversationSender` 是 composable，每个 Vue 组件实例产生独立 sender。`new.vue` 调 `createNewConversation` → `await navigateTo(...)` → 路由切换触发 `new.vue` unmount → `useConversationSender.ts:48-51` `onUnmounted(() => { stopTTS(); cancelAIStream() })` **清理 accumulatedDeltaText + 取消 stream** → `[id].vue` mount 时新建 sender 实例，看不到前一个 sender 准备好的 user message |
| **架构方案（待决策）** | 选项 A：把 sender 状态抬升到 Pinia store（单例）<br>选项 B：把 sender 状态抬升到 `useState()` 顶层 composable（SSR-friendly 单例）<br>选项 C：改 `createNewConversation` 用 `setTimeout` 替代 `await navigateTo`，绕开 unmount 钩子（hack）<br>选项 D：`[id].vue` onMounted 检测"是否带 user_input query param 来的新会话"，自动触发 sendMessage + sendAIStream |
| **推荐** | 选项 A 或 B（架构正确） |
| **影响范围** | 任何走"新会话 + 自动 AI"路径的功能（chat /new、assessment 自动生成、语音消息）|
| **下一步** | 新建独立 session 评估 A vs B，本 session 不处理 |

### 顺手发现的小问题（不修，记入 residuals）

| # | 问题 | 备注 |
|---|---|---|
| 1 | 字号调整触发 `PATCH /users/me ×3` 而非 1 次 | `userConfig.fontSize` watch 防抖缺失，不阻塞主链路 |
| 2 | `POST /conversations` 同 ts 双记录（无 Auth + 有 Auth） | `useApi.ts` 401 重试逻辑看起来在同一 tick 触发，需确认是否真并发 |

---

## 五、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `emotion-echo-web/app/pages/chat/conversation/new.vue:80-86` | 修前 handleSubmit 实现 |
| `emotion-echo-web/app/composables/useConversationSender.ts:62-215` | `sendToExistingConversation` + `createNewConversation` 实现 |
| `emotion-echo-web/app/stores/conversation.ts:163-181` | `createConversation` 实现（body 决定） |
| `emotion-echo-web/app/composables/useApi.ts:115-128` | token 来源（userStore → cookie fallback） |
| `emotion-echo-web/app/stores/user.ts:123-156` | `setAccessToken` / `clearToken` 实现（确认 cookie 写入策略） |
| `emotion-echo-web-bff/internal/handler/chat_handler.go:42-53` | BFF `/conversations/:id/messages` 注册 |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:1-70` | BFF `/ai/stream` 实现 |
| `deploy/apisix/seed.sh:271-293` | file-logger log_format 块（trailing comma 位置） |
| `deploy/apisix/seed.sh:300-410` | CATCHALL_PLUGINS_JSON 含 serverless-post-function 注入 X-User-Id |
| `deploy/apisix/seed_test.js` | 静态结构断言（39 → 40 项） |
| `deploy/docker-compose.apps.yml` | web 容器配置（重建触发 seed 重跑）|
| `memory/emotion-echo-dev-mode-2026-09-16-session.md` | 上次 session 收口 + 1 已知未修 bug 起点 |

### 架构假设清单（写前对齐 §〇 硬规则）

| 假设 | 验证 |
|---|---|
| `createNewConversation` 是为 `new.vue` 量身定做的 composable | ✅ 读 `useConversationSender.ts:173-215` 确认 |
| `useConversationSender` composable 内部状态在 unmount 时被清理 | ✅ 读 line 48-51 `onUnmounted(() => { stopTTS(); cancelAIStream() })` 确认 |
| `seed.sh` 写完 commit 后立即可重跑 | ✅ `node seed_test.js` + 容器 exit 0 验证 |
| web 容器重建触发 apisix-seed 重跑 | ✅ `depends_on: emotion-echo-apisix-seed: condition: service_started` 启动时跑 |

---

## 六、commit 计划（待 push，本 stage 收口动作）

按 [multi-pr-commit-discipline] 建议拆为 2 个 commit：

| # | commit | 文件 | 类型 |
|---|---|---|---|
| 1 | `test(new.vue): RED 钉住 chat /new handleSubmit 必须调 conversationSender` | `emotion-echo-web/app/pages/chat/conversation/new.test.ts` | test |
| 2 | `fix(new.vue): handleSubmit 调 createNewConversation 触发完整发送链路` | `emotion-echo-web/app/pages/chat/conversation/new.vue` | fix |
| 3 | `test(seed.sh): RED 钉住 file-logger log_format 不允许 trailing comma` | `deploy/apisix/seed_test.js` | test |
| 4 | `fix(seed.sh): 删 file-logger log_format 末尾 trailing comma (PUT route 100 schema 失败致 catch-all 缺失)` | `deploy/apisix/seed.sh` | fix |

> **注意**：commit 1 + 2 必须同 PR（test 是 fix 的回归钉子）；commit 3 + 4 同理。两个 PR 可以按"是否互相依赖"判断合并顺序——本 stage 合并前**两个 bug 都修**才能让浏览器端到端看到 AI 回复（前提：解决 architecture debt）。

---

## 七、当前 residuals（合并前必清）

- [ ] **架构债**：useConversationSender composable 生命周期 vs Vue 路由 unmount（详见 §四）
- [ ] 顺手小问题 1：字号防抖
- [ ] 顺手小问题 2：useApi 401 重试同 ts 双记录

---

## 八、经验教训（写入 memory）

1. **TDD 静态合同 + 端到端浏览器实测**是 Emotion-Echo 的有效组合：static-source 合同测试能在 happy-dom mount 困难时钉住"行为契约"，浏览器实测能抓出"代码生效但运行时被外部依赖阻断"（本轮 seed.sh bug 就是这样被发现的）。
2. **APISIX seed 是隐性单点失败**：任何 web 容器重启都触发 seed 重跑，seed 挂掉 → 所有非 auth 业务 401。前端代码看起来"不工作"但根因在后端网关——**必须先 curl admin API 看路由表是否完整**，再去看应用层。
3. **多个 bug 叠加掩盖真因**：前端 bug + 网关 bug 叠加时，前端 bug 永远是第一个嫌疑。**先查 Network 看请求是否真的到达 BFF**（看 BFF 日志），再去看应用层。
4. **别跨越 TDD 循环边界**：本 session 修了 new.vue（一个 TDD 循环），顺手又修 seed.sh（另一个 TDD 循环）。按 §〇 精神应该拆 2 个独立 PR，但 seed.sh 修复是 new.vue 修复的"前置依赖"——本 stage 一个 PR 包含 2 个独立 TDD 循环是合理的（否则 chat 链路在 PR1 合并后仍不工作）。
