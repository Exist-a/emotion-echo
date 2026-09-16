---
status: planned
priority: high
owner: TBD
created: 2026-09-16
last-refresh: 2026-09-16
type: end-to-end-validation
depends-on:
  - stage-108-sender-architecture-debt-fix-2026-09-16.md (sender 架构债 ✅ FIXED)
  - sprint-109a-apisix-jwt-401-2026-09-16.md (A7 必须先修)
related-stages:
  - stage-103-dev-mode-launch-2026-09-16.md (dev 模式基线)
  - stage-105-browser-e2e-2026-09-16.md (browser-use 浏览器 E2E)
related-issues:
  - docs/plans/test-coverage-tracker-2026-09-16.md §一 E2E-2 (🔴 → 🟢 目标)
blocking:
  - sprint-109c-data-contract-smoke-2026-09-16.md (数据契约 smoke 前提)
  - sprint-110 E2E-2 Playwright spec 写入
---

# Sprint 109b — 端到端 E2E-2 chat 跑通验证

> **目的**：在 Sprint 109a (A7 APISIX 401) 修通 + Stage 108 (sender 架构债) 已修的基础上，**端到端验证浏览器实测能看到 AI 回复**。
>
> **DoD 标准**：浏览器开 localhost:3000 → 登录 → /chat/conversation/new → 输入消息 → 点击发送 → **看到 AI 流式回复渲染在 dialog 中**，且 chat-svc / ai-svc 日志可见对应记录。

---

## 一、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `emotion-echo-web/app/pages/chat/conversation/new.vue` | handleSubmit 入口（Stage 107 修后调 createNewConversation）|
| `emotion-echo-web/app/composables/useConversationSender.ts` | createNewConversation + sendToExistingConversation（Stage 108 useState 化后）|
| `emotion-echo-web/app/composables/useAIStreamHandler.ts` | SSE 流解析（Stage 108 useState 化后）|
| `emotion-echo-web-bff/internal/handler/chat_handler.go:42-53` | BFF `/conversations/:id/messages` 注册 |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go` | BFF `/ai/stream` SSE 编排 |
| `emotion-echo-chat-svc/internal/handler/chat_handler.go` | chat-svc SendMessage RPC |
| `emotion-echo-chat-svc/internal/events/kafka_publisher.go` | outbox 事件发布 |
| `emotion-echo-ai-svc/internal/analyzer/` | AI 情绪分析 + 融合 |
| `emotion-llm-service/grpc_server.py` | llm-service ChatCompletion |
| `scripts/smoke_chat_svc.sh` + `scripts/smoke_ai_svc.sh` | 现有 smoke 脚本 |
| `stage-107-chat-new-conversation-fix-2026-09-16.md §二` | Stage 107 浏览器实测数据 |
| `stage-108-sender-architecture-debt-fix-2026-09-16.md §二` | Sprint 108 浏览器实测数据 |
| `docs/plans/test-coverage-tracker-2026-09-16.md §一 E2E-2` | 当前状态追踪 |

### 架构假设清单

| 假设 | 验证 |
|---|---|
| Sprint 109a 完成后所有 /api/v1/* 返 200 | 🟡 待 Sprint 109a |
| Stage 108 sender useState 化真生效（callbacks 跨实例共享）| ✅ vitest 7/7 PASS + 双向验证 |
| SSE 流式响应能被 Vue 响应式 `streamingContent` 正确 emit | 🟡 端到端验证（最后一步）|
| BFF ai_stream_handler 真发 SSE | 🟡 端到端验证 |
| chat-svc outbox 真写 DB | 🟡 端到端验证 |
| ai-svc 真消费 outbox + 调 llm-service | 🟡 端到端验证 |

---

## 二、TDD 实施步骤

### Step 1 · 手动端到端验证（Playwright spec 等 sprint-110）

**不写 Playwright**——本 sprint 用 browser-use 手动验证。Playwright 自动化是 sprint-110 的事（避免测了但永远 fail）。

**操作步骤**：

1. 打开浏览器 → http://localhost:3000/login
2. 点"用演示账号快速体验"（echo / echo123）
3. 验证进入 /chat/conversation/new
4. 在 textarea 输入 "你好，请用一句话介绍一下你自己"
5. 点击发送
6. **观察 fetch hook**（browser console 注入或 Playwright）：

```js
window.__eeReqs = []
const origFetch = window.fetch
window.fetch = function(input, init) {
  const url = typeof input === 'string' ? input : input.url
  window.__eeReqs.push({
    ts: Date.now(),
    method: (init && init.method) || 'GET',
    url,
    body: init?.body?.slice(0, 300)
  })
  return origFetch.apply(this, arguments)
}
```

7. **断言 fetch hook 看到**：
   - `POST /api/v1/conversations` （createNewConversation 调）
   - `POST /api/v1/conversations/:id/messages` （sendToExistingConversation 内 sendMessage）
   - `POST /api/v1/ai/stream` （sendAIStream 发 SSE）
   - `GET /api/v1/conversations/:id/messages` （可能 watch 触发列表 reload）

8. **断言 SSE 响应解析**：`onDelta` 回调多次被调用，UI 看到流式文本

9. **观察容器日志**：
   ```bash
   docker logs emotion-echo-chat-svc --since 1m | grep -E "SendMessage|outbox"
   docker logs emotion-echo-ai-svc --since 1m | grep -E "ChatCompletion|fusion"
   docker logs emotion-llm-service --since 1m | grep -E "ChatCompletion"
   ```

### Step 2 · 截图取证

截 4 张图：
- (a) 登录页（点 demo 按钮前）
- (b) /chat/conversation/new 初始
- (c) 用户消息发出后 streaming 状态
- (d) AI 回复完整渲染

存到 `docs/stages/stage-109b-end-to-end-chat-2026-09-16/screenshots/`

### Step 3 · 数据契约 smoke 入口验证（部分）

跑 `scripts/smoke_chat_svc.sh` 看现有 smoke 是否通过，作为辅助证据。

### Step 4 · 文档化

写 `docs/stages/stage-109b-end-to-end-chat-2026-09-16.md`：
- §一 起点 + §二 实施 + §三 实测截图 + §四 BFF/chat-svc/ai-svc 日志摘录 + §五 经验

---

## 三、DoD（Definition of Done）

- [ ] 浏览器实测 fetch hook 看到 4 类请求（POST /conversations + /messages + /ai/stream + GET /messages）
- [ ] SSE 流式响应被 `onDelta` 回调正确处理
- [ ] UI 看到 AI 回复文本（流式 + 最终）
- [ ] chat-svc 日志有 SendMessage / outbox 写入记录
- [ ] ai-svc 日志有 ChatCompletion / fusion 处理记录
- [ ] llm-service 日志有 ChatCompletion 调用记录
- [ ] 4 张截图存档
- [ ] docs/stages/stage-109b-end-to-end-chat-2026-09-16.md 落地
- [ ] test-coverage-tracker §一 E2E-2 状态从 🔴 → 🟢（browser-use 端到端列）
- [ ] §2.5 收口 + push

---

## 四、工作量估计

- Step 1 手动验证: 1-2 hour（含调试如果链路某段挂）
- Step 2 截图: 15 min
- Step 3 smoke: 30 min
- Step 4 文档: 30 min

**总计**: 2-3 hour

---

## 五、风险

| 风险 | 影响 | 应对 |
|---|---|---|
| sender useState 化后仍有未发现的边界问题 | UI 卡死或 SSE 不显示 | 回退到 Stage 107 状态定位 |
| BFF / ai-svc 配置问题 | 链路某段 500 | 看 BFF 日志 + ai-svc 日志 |
| SSE chunk 解析 bug | 流式中断 | useAIStreamHandler.test.ts 加新用例 |

---

## 六、commit 计划

| # | Commit | 文件 |
|---|---|---|
| 1 | `docs(stage-109b): E2E-2 chat 端到端验证 + 截图 + 日志摘录` | `docs/stages/stage-109b-end-to-end-chat-2026-09-16.md` + `screenshots/` |
| 2 | `docs(plans): test-coverage-tracker E2E-2 状态 🔴 → 🟢` | `docs/plans/test-coverage-tracker-2026-09-16.md` |

---

## 七、调研依据未做完（写前必补）

- [ ] 跑通后补全 chat-svc + ai-svc + llm-service 日志摘录
- [ ] 截图从浏览器 local 取，存到 docs/stages/stage-109b/screenshots/

---

## 八、依赖 Sprint 109a 的真实状态

Sprint 109b **完全阻塞于 Sprint 109a**——A7 不修，浏览器实测看不到 AI 回复（被 401 拦截）。**109a 是 109b 的前置硬依赖**。

如果 Sprint 109a 修了一个上午发现根因（最坏情况），109b 紧接着在下午跑即可。
