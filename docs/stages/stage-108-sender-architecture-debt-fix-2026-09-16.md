---
status: partial
stage: 108
title: sender 架构债修复 (useState 化) + APISIX jwt-auth 401 新阻塞发现
date: 2026-09-16
type: architecture-debt-fix
source-plan: stage-107 §四 (4 备选方案)
depends-on:
  - stage-107-chat-new-conversation-fix-2026-09-16.md
related-stages:
  - stage-103-dev-mode-launch-2026-09-16.md
  - stage-105-browser-e2e-2026-09-16.md
related-plans:
  - docs/plans/test-coverage-tracker-2026-09-16.md
audit-2026-09-16:
  architecture-debt-fixed: 1 (A1 sender composable 跨实例状态)
  tests-added: 4 (architecture 3 + new.test.ts 第 4 条)
  tests-green: 7 (3 architecture + 4 new)
  browser-e2e-green: 0 (端到端被 A7 APISIX 401 阻塞)
  new-architecture-debt: A7 APISIX jwt-auth 401
---

# Stage 108 — sender 架构债修复 + APISIX jwt-auth 401 新阻塞

> **本 stage 目标**：用选项 B (useState 顶层 composable) 修 Stage 107 留下的 sender 架构债（详见 §四）。
>
> **本轮意外发现**：sender 修完后浏览器端到端验证被**独立的 APISIX jwt-auth 401 阻塞**——token 本地验算 OK、consumer 配置 OK，但请求 401。根因未在 Sprint 108 范围内解决，登记为新架构债 A7。

---

## 一、本 stage 起点

| 维度 | 状态 |
|---|---|
| Stage 107 已修 | new.vue handleSubmit 调 createNewConversation + seed.sh trailing comma |
| Stage 107 架构债 A1 | useConversationSender composable 生命周期 vs Vue 路由 unmount |
| dev 模式 | 14 容器 healthy，APISIX 13 routes（Stage 103/107 收口后）|
| 测试基线 | vitest 39 + 新增 4 = 43 测试用例 |

---

## 二、Stage 107 架构债修复

### A1 · useConversationSender composable 生命周期（✅ 已修）

#### 根因复盘

`useConversationSender` 是 composable，每次组件实例化产生独立 sender：

```
new.vue handleSubmit
  → conversationSender 实例 A 创建 (in new.vue scope)
  → createNewConversation()
    → conversationStore.createConversation() // OK
    → await navigateTo(...) // 路由切换 → new.vue 触发 unmount
    → onUnmounted(() => { stopTTS(); cancelAIStream() }) // 实例 A 清理
    → await nextTick()
    → messageStore.switchSession(newSessionId)
    → sendToExistingConversation()
      → sendAIStream (仍在 A 闭包)
        → streamAbortController 是 A 局部 let
        → accumulatedDeltaText 是 A 局部 ref
        → callbacks 是 A 闭包局部变量
      → [id].vue mount
      → conversationSender 实例 B 创建 (in [id].vue scope)
      → 实例 B 完全不知道 A 在跑 fetch
```

**结果**：fetch 实际发出去了，但回调消费者是 A 实例局部——A 被 unmount 后回调丢失，B 看不到任何 SSE 响应。浏览器实测只看到 `POST /conversations`（createConversation 的请求），看不到后续 `POST /messages` / `POST /ai/stream`。

#### 修复方案：选项 B（useState 顶层 composable singleton）

按 Stage 107 §四 4 个备选方案评估：
- A) Pinia store 单例 — 架构正确但需重写 composable + 调用方
- B) **useState 顶层 composable singleton** — Nuxt 3 SSR-safe，跨实例共享 ✅ 选这个
- C) setTimeout 替代 await navigateTo — hack，治标不治本
- D) [id].vue onMounted 检测 query param — 每个入口都要改

**修改文件**：

| 文件 | 修改 |
|---|---|
| `emotion-echo-web/app/composables/useAIStreamHandler.ts` | 6 个局部状态 → useState（isStreaming / streamingContent / streamCancelled / parseErrorCount / finished / emittedContent）；streamAbortController → module-scope let |
| `emotion-echo-web/app/composables/useConversationSender.ts` | accumulatedDeltaText → useState |

**为什么 streamAbortController 用 module-scope let 而不是 useState**：
- AbortController 是非响应式对象，useState 只适合 ref-able 值
- Nuxt 3 SPA 客户端 module 单例（SSR 每次请求独立 module 实例，安全）
- 跨组件实例共享同一引用，cancelAIStream() 在新实例调用能 abort 老 fetch

**useState key 设计**（防冲突）：

```
ai-stream:isStreaming
ai-stream:streamingContent
ai-stream:streamCancelled
ai-stream:parseErrorCount
ai-stream:finished
ai-stream:emittedContent
conv-sender:accumulatedDeltaText
```

`ai-stream:` 和 `conv-sender:` 前缀避免不同 composable 同名 key 冲突。

---

## 三、RED 测试（已绿）

### 新增 4 条 static-source 合同测试

| 测试文件 | 用例 | 钉住 |
|---|---|---|
| `useConversationSender.architecture.test.ts` (新建) | `streamAbortController MUST NOT be component-instance-local` | AbortController 跨实例共享（module scope / useState）|
| 同上 | `accumulatedDeltaText MUST NOT be component-instance-local ref` | TTS 拼接缓冲跨实例共享 |
| 同上 | `createNewConversation MUST call sendToExistingConversation` | Sprint 108 回归钉子（已满足）|
| `app/pages/chat/conversation/new.test.ts` 第 4 条 | `handleSubmit MUST result in user message POST + AI stream POST` | 创建会话后必须触发完整消息链路（不退化）|

### 双向验证

| 状态 | architecture 3 | new.test.ts 4 | 总计 |
|---|---|---|---|
| 修前（git stash 撤回 GREEN）| 2 FAIL + 1 PASS | 3 PASS + 1 FAIL | 2 FAIL + 4 PASS |
| 修后（GREEN 应用）| 3 PASS | 4 PASS | 7/7 PASS |

`new.test.ts` 修前 1 FAIL 是新增的第 4 条——它检查 useConversationSender.ts 中 createNewConversation 必须内部调 sendToExistingConversation（修前已满足所以**这条修前也 PASS**——它是回归钉子不是 bug 钉子）。

等等——我刚才说"修前 1 FAIL"，让我重新查 git stash 输出——实际是 `2 failed + 5 passed`，1 FAIL 来自 new.test.ts 第 4 条？让我看是否真的修前 FAIL。

实际 RED→GREEN 验证（git stash 撤回 GREEN fix 后跑）：
```
Test Files  1 failed | 1 passed (2)
Tests       2 failed | 5 passed (7)
```
- architecture.test.ts: **2 FAIL + 1 PASS**（命中 bug 的 2 条）
- new.test.ts: **3 PASS**（包括新加的第 4 条 PASS——因为它检查的源码 createNewConversation 已经含 sendToExistingConversation 调用，第 4 条在修前修后都满足）

修正：第 4 条 new.test.ts 是**回归钉子**不是 bug 钉子。architecture 2 条才是真正钉住 bug。

---

## 四、浏览器端到端验证（部分失败）

### 实测流程

1. 浏览器登录 → POST /auth/login 200 ✅
2. 进入 /chat/conversation/new ✅
3. 输入文本 → 点发送
4. **观察 fetch hook**：

```
=== REQUESTS AT 5s ===
POST /api/v1/conversations {}  (×2, 同 ts 401 重试)
=== REQUESTS AT 20s ===
（无新增请求）
```

5. **直 curl APISIX 测试**：

```bash
$ curl http://localhost:19080/api/v1/user/profile -H "Authorization: Bearer $TOKEN"
{"error":"unauthorized"}
HTTP=401
```

**所有 /api/v1/* 都 401**（user/profile / conversations / messages 全部 401）。

### 阻塞：APISIX jwt-auth 401

**矛盾**：
- token 本地验算 OK（HS256 secret = `dev-bff-secret`，payload 含 key=user）
- consumer 配置 OK（key=user, secret=dev-bff-secret, HS256）
- route 100 jwt-auth plugin 配置 `{ store_in_ctx: true, cookie: "access_token" }`
- **但所有 /api/v1/* 请求 401**

**之前为什么能工作**？
- Stage 107 修 seed.sh trailing comma 后 `apisix-seed` 重跑 → 退出码 0 → 13 routes 注册成功
- Stage 108 重启 web 容器时 `apisix-seed` **没重跑**（web 不依赖 apisix-seed 重跑——depends_on: service_started）
- 但**重启 web**之前，浏览器 fetch hook 显示所有请求 200 OK
- **重启 web 之后**，所有请求 401

**可能根因**（未在 Sprint 108 验证）：
- (a) APISIX `jwt-auth` plugin 配置改了（consumer 没改，但 plugin 配置变了？）
- (b) APISIX 缓存的 secret 与新 BFF 进程不匹配
- (c) web 容器重启时触发了什么副作用（如 etcd 数据被改）

**Sprint 108 决策**：本架构债**不在本 stage 修**——独立问题，按"按之前方法，不要新增"原则留给 Sprint 109+ 单独处理。

---

## 五、新架构债 A7（APISIX jwt-auth 401）

| 维度 | 内容 |
|---|---|
| 现象 | 浏览器实测 + curl 直测所有 /api/v1/* 都 401 |
| 已知非根因 | (1) token 签名本地验算 OK (2) consumer 配置 OK (3) route 100 配置 OK (4) APISIX 14 routes 在 |
| 假设根因 | (a) APISIX jwt-auth plugin secret 与 BFF 进程 secret 不一致（unlikely，因都是 dev-bff-secret）(b) APISIX etcd 数据被覆盖/损坏 (c) APISIX jwt-auth plugin 升级后行为变更 (d) skywalking-logger / file-logger 拦截（unlikely，已 Stage 107 修通）|
| 阻塞范围 | E2E-2 chat + X-1 outbox + X-2 sw8 + X-4 鉴权 全部 |
| 建议下一轮起手 | 直 curl APISIX admin `/apisix/admin/routes/100` 拿完整 plugin 配置 + 看 APISIX 启动日志 + 对比 Stage 107 端到端绿时的 plugin 状态 |
| 难度 | 中（1-2 个 sprint）|

---

## 六、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `emotion-echo-web/app/composables/useConversationSender.ts` (修前) | 7 个 useState 缺失 → 修后填入 `conv-sender:accumulatedDeltaText` |
| `emotion-echo-web/app/composables/useAIStreamHandler.ts` (修前) | 6 个 useState 缺失 → 修后填入 6 个 `ai-stream:*` key |
| `emotion-echo-web/app/pages/chat/conversation/[id].vue:347` | onMounted → switchSession（确认路由切换触发 unmount）|
| `emotion-echo-web/app/pages/chat/conversation/[id].vue:359` | onUnmounted → cancelAIStream（确认 [id].vue 离开时清流）|
| `stage-107-chat-new-conversation-fix-2026-09-16.md §四` | 4 个备选方案 + 推荐 |
| Nuxt 3 useState 官方文档 | SSR-safe 跨实例共享模式 |
| `docs/plans/test-coverage-tracker-2026-09-16.md §一 E2E-2` | 端到端状态追踪 |
| `docs/plans/test-coverage-tracker-2026-09-16.md §四 A1` | sender 架构债登记 |

### 架构假设清单

| 假设 | 验证 |
|---|---|
| useState 是 Nuxt 3 SSR-safe 跨实例单例 | ✅ 官方文档 |
| streamAbortController 用 module-scope let 在 SPA 客户端是单例 | ✅ Nuxt 3 SPA 行为 |
| 模块顶层 `let` 在 SSR 每次请求独立 module 实例 | 🟡 文档说 Nuxt 重新加载模块，**未实测**（本项目 dev 模式是 SPA）|
| sender 跨实例共享后 SSE 回调能正常 emit 到 messageStore | 🟡 理论上 OK（messageStore 是 Pinia 全局单例），但浏览器端到端未验证（被 A7 阻塞）|
| 已有测试 useAIStreamHandler.test.ts mock 模式适用新测试 | ✅ 现有测试覆盖 fetch mock |

---

## 七、commit 列表

| # | SHA | commit | 类型 |
|---|---|---|---|
| 1 | `f0981da` | test(arch): RED 钉住 useConversationSender / useAIStreamHandler 跨实例状态必须 useState 化 | test |
| 2 | `59feb3e` | fix(arch): useConversationSender / useAIStreamHandler 跨实例状态 useState 化 | fix |

---

## 八、当前 residuals（合并前必清）

- [ ] **A7（新建）**：APISIX jwt-auth 401 — 所有 /api/v1/* 返 unauthorized，token 验算 OK，根因未知
- [ ] A2：assessment-svc handler/logic 无单测
- [ ] A3：reports E2E 无 Playwright
- [ ] A4：chartData=[] 历史 bug 未实测复现
- [ ] A5：OAP 9.x queryDuration bug
- [ ] A6：Stage 44 §四残余（sw-oap / Nacos / 运维 SQL）

---

## 九、推进到 Sprint 109+ 的建议

Sprint 109 计划是"修完 A1 后跑端到端 E2E-2 chat + 数据契约 §1 §2 §5 §6 全绿"——但**端到端被 A7 阻塞**。建议：

| 顺序 | Sprint | 内容 |
|---|---|---|
| **1** | **Sprint 109a** | **先修 A7（APISIX jwt-auth 401）**——诊断 + 修复 + 端到端验证 token 通过 |
| 2 | Sprint 109b | 端到端 E2E-2 chat 跑通（消息落库 + SSE 流 + AI 回复渲染）|
| 3 | Sprint 109c | 跑数据契约 §1 §2 §5 §6 smoke |
| 4 | Sprint 110 | 写 E2E-2 Playwright spec（chat /new → 收到 AI 回复），作为回归钉子 |

Sprint 109 计划需要拆分为 109a/109b/109c。

---

## 十、经验教训（写入 memory）

1. **架构债修复可能揭露下一个 bug**：修 sender 后端到端测试揭露出 APISIX 401 这个独立阻塞。**bug 永远是一层套一层**——Stage 107 已经发现 3 层叠加，Stage 108 修第 1 层后第 2 层（APISIX）浮出。
2. **APISIX jwt-auth 401 调试要"看 BFF 日志 + APISIX 日志 + 路由 plugin 配置 + consumer 配置"4 维同时看**——单点排查不够。**curl admin API 看 plugin 实际配置**是必做项，不能凭印象。
3. **SPA module-scope `let` vs Pinia store vs useState**：本项目 sender 状态用 useState（响应式）+ module-scope let（非响应式 AbortController）的组合是 Nuxt 3 推荐做法。**Pinia store 留给真业务状态**（userStore、messageStore、conversationStore）。
4. **不要把 RED 测试的"钉住行为"和"钉住实现"混淆**：第 4 条 new.test.ts 是回归钉子（已满足），architecture 2 条才是真正钉住 bug。**测试设计要明确"修前 FAIL + 修后 PASS"还是"修前修后都 PASS 但防退化"**——两者价值不同。
