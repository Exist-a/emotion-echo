---
status: complete
priority: high
owner: TBD
created: 2026-09-17
last-refresh: 2026-09-17
type: stage-report
depends-on:
  - sprint-110-a8-a9-a10-fix-2026-09-17.md (Sprint 110 起点)
related-stages:
  - stage-110-a8-a9-a10-fix-2026-09-17.md (Sprint 110 收口)
  - stage-109b-end-to-end-chat-2026-09-16.md (端到端基线)
related-issues:
  - known-issues-backlog-runtime-bugs-2026-09-17.md (R-09 / R-11 / R-12 / R-13 / R-14)
blocking:
  - Sprint 112 (chat-svc title 异步生成修复 R-13)
---

# Stage 111 — R-09 JWT 真修复 + A11 sidebar 标题 fallback

> **状态**：R-09 + A11 完全修复；dev mode IAB 浏览器端到端验收通过；A9/A10 上轮已修，本轮确认。
>
> **本 stage 实际产出**：
> - ✅ R-09 真根因 (HttpOnly cookie + computed ref) 修复 → AI 回复正常
> - ✅ A11 sidebar 会话标题为空 bug 修复 → 显示"对话 #125"等可识别 label
> - ✅ RED→GREEN 单测 11 个 (R-09 architecture 7 + A11 fallback 4)
> - ✅ Dev mode 流程确立 (避免反复 rebuild)

---

## 一、起点（已知问题）

Sprint 110 web rebuild 后浏览器实测仍有问题：

### R-09 — 发送消息无 AI 回复（被 Sprint 110 误判）
- **现象**：用户消息气泡渲染正常，但 AI 回复气泡显示 "请求失败: JWT token invalid"
- **Sprint 110 误判**：以为是 web 镜像没 rebuild
- **Sprint 111 真相**：见 §二

### A11 — sidebar 会话列表全空白（新增发现）
- **现象**：sidebar 21 个会话 item 全部显示空标题，aria-label "对「」更多操作"
- 用户无法识别任何历史会话

---

## 二、R-09 真根因诊断

### 链路追踪
1. 浏览器 fetch 拦截器抓到：`POST http://localhost:19080/api/v1/ai/stream` headers `Authorization: ""`
2. APISIX jwt-auth plugin：`[warn] JWT token invalid: invalid jwt string` → 401
3. BFF 日志：0 条 ai-stream 处理记录（BFF 没收到）
4. curl 模拟：`curl -H "Authorization: Bearer ..."` → 200 OK + 正常 SSE 流
5. **结论**：后端链路完全通，bug 在前端拼 Authorization header

### 三层根因
1. **HttpOnly cookie 浏览器读不到**：`Set-Cookie: access_token=...; HttpOnly` → `document.cookie = ""`，`useCookie('access_token').value` 也读不到
2. **直接 `useCookie('access_token').value` 永远空**：useAIStreamHandler.ts:95 直接用 → Authorization 永远空
3. **Pinia computed ref 陷阱**：`user.ts:30` `const getAccessToken = computed(() => accessToken.value)`，调用 `store.getAccessToken` 返回 ref 对象不是字符串。必须 `.value`。

### 修复方案（双重）
新建 `app/lib/clientAccessToken.ts` helper：
- CSR 优先 `useUserStore()` → `.value` 取字符串
- Fallback `useCookie('access_token').value`
- SSR 直接 cookie

3 个调用点改用 helper：
- `useAIStreamHandler.ts:97`
- `useAIStream.ts:47`
- `useTTSPlayer.ts:178`

---

## 三、A11 真根因

### 链路
1. `curl http://localhost:19080/api/v1/conversations?limit=3` 返回 `"title": "", "lastMessage": null`
2. 前端 `pages/chat/conversation/index.vue:150` `label: c.title` 直接用，无 fallback
3. 21 个会话全部空标题

### chat-svc 异步生成 title 时机问题
- chat-svc 在 SSE 流完后调 AI 生成 title（应在 Sprint 109b 修复时引入）
- 但 list API 读不到（写库前 list 返回空，或写库后 cache 没 invalidate）
- 这是 **R-13**（建议 Sprint 112 后端修复）

### 修复（A11 前端 fallback）
3 级 fallback chain：
```ts
label: c.title?.trim()
  || (c.lastMessage ? String(c.lastMessage).slice(0, 30) : '')
  || `对话 #${c.id}`,
```

---

## 四、TDD 实施

### RED 测试 (vitest 静态源码)
| 文件 | 测试数 | 状态 |
|---|---|---|
| `emotion-echo-web/app/composables/r09-ai-stream-auth-header.architecture.test.ts` | 7 | RED → GREEN |
| `emotion-echo-web/app/pages/chat/conversation/a11-sidebar-fallback.architecture.test.ts` | 4 | RED → GREEN |

总计 **11/11 PASS**。

### GREEN 修复
| 文件 | 改动 |
|---|---|
| `app/lib/clientAccessToken.ts` | 新建 — CSR userStore(.value) → cookie fallback, SSR cookie |
| `app/composables/useAIStreamHandler.ts` | 改用 `getClientAccessToken()` |
| `app/composables/useAIStream.ts` | 改用 `getClientAccessToken()` |
| `app/composables/useTTSPlayer.ts` | 改用 `getClientAccessToken()` |
| `app/pages/chat/conversation/index.vue` | `conversationItems` computed 加 3 级 fallback |

---

## 五、dev mode IAB 浏览器验收（端到端）

### 流程
1. `pnpm dev` 本地启动 web (3000 端口) — **不再 rebuild image**
2. IAB 浏览器 hard reload login → 演示账号登录
3. `/chat/conversation/new` → 输入"测试完整链路" → 点发送
4. 5s 内看到 AI 回复气泡 + sidebar 顶部高亮新会话显示 lastMessage

### 验收结果（dev mode 2026-09-17 09:38）
- ✅ POST `/api/v1/ai/stream` headers `Authorization: Bearer eyJhbG...`
- ✅ APISIX 200 OK
- ✅ BFF 日志显示 ai-stream 处理 + chat-svc grpc 调用
- ✅ 浏览器 UI: 用户消息 + AI 回复气泡
- ✅ AI 回复内容: "谢谢你告诉我这些。能再多说说你的感受吗？我在认真听。"
- ✅ sidebar 21 个会话显示 "对话 #125" 等，新会话显示 AI 回复作为 label

截图：`docs/stages/stage-111-r09-ai-stream-jwt-2026-09-17/screenshots/01-final-r09-fixed-a11-fallback.png`

---

## 六、A9/A10 验证

A9 (sidebar fold-btn 露边) + A10 (voice-btn 颜色尺寸) 已在 Sprint 110 commit，本轮浏览器实测确认修复有效：
- A9: 折叠按钮"‹" + 标题"最近的对话" + "+"按钮间距整齐
- A10: 底部 voice-btn 浅绿色圆形 32×32

---

## 七、commit 历史

| # | Commit | 文件 |
|---|---|---|
| 1 | `test(arch): R-09 RED 钉死 useAIStreamHandler/useAIStream/useTTSPlayer 必须用 getClientAccessToken` | `app/composables/r09-ai-stream-auth-header.architecture.test.ts` |
| 2 | `fix(R-09): 新建 clientAccessToken helper (userStore.value → cookie fallback)` | `app/lib/clientAccessToken.ts` |
| 3 | `fix(R-09): useAIStreamHandler/useAIStream/useTTSPlayer 改用 getClientAccessToken` | 3 composable files |
| 4 | `test(arch): A11 RED 钉死 conversationItems 必须有 title→lastMessage→对话#id fallback` | `app/pages/chat/conversation/a11-sidebar-fallback.architecture.test.ts` |
| 5 | `fix(A11): conversationItems computed 加 3 级 fallback chain` | `app/pages/chat/conversation/index.vue` |
| 6 | `docs(stage-111): R-09 + A11 修复 + dev mode 流程确立` | `docs/stages/stage-111-r09-ai-stream-jwt-2026-09-17.md` + screenshots/ |
| 7 | `docs(backlog): R-11/R-12/R-13/R-14 登记` | `docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md` |

---

## 八、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `emotion-echo-web/app/lib/clientAccessToken.ts` | R-09 helper 核心 |
| `emotion-echo-web/app/composables/useAIStreamHandler.ts:95-110` | 原 useCookie 直接读 → 改 helper |
| `emotion-echo-web/app/composables/useAIStream.ts:47` | 同根因修复 |
| `emotion-echo-web/app/composables/useTTSPlayer.ts:178` | 同根因修复 |
| `emotion-echo-web/app/stores/user.ts:30` | `getAccessToken = computed(...)` ref 陷阱 |
| `emotion-echo-web/app/composables/useApi.ts:116-128` | 已有 fallback 链路参考 |
| `emotion-echo-web/app/pages/chat/conversation/index.vue:147-155` | A11 fallback 注入位置 |
| `emotion-echo-bff/internal/handler/ai_stream_handler.go` | BFF SSE 处理（curl 验证通） |
| `emotion-echo-web/Dockerfile` | 生产 build 模式确认 |
| `docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md` | R-09 / R-11 / R-12 / R-13 / R-14 登记 |

### 架构假设清单

| 假设 | 验证 |
|---|---|
| HttpOnly cookie 浏览器 JS 读不到 → useCookie 也读不到 | ✅ curl Set-Cookie + happy-dom 验证 document.cookie 空 |
| useApi 走 userStore → cookie fallback 能拿到 token | ✅ useApi.ts:118-123 实测 POST /messages 200 |
| Pinia computed 直接访问返回 ref 不是字符串 | ✅ user.ts:30 + 浏览器实测 store.getAccessToken 是 ref 对象 |
| dev mode HMR 实时生效 vs 生产 build 需 rebuild | ✅ pnpm dev HMR 工作正常, 用户多次强调先用 dev |
| chat-svc 异步生成 title 时机导致 list API 读不到 | ✅ curl /conversations 返回 title="", R-13 后端修复待 Sprint 112 |
| isInCreateFlow useState 不会破坏 SSR | 🟡 Sprint 110 已验，本轮未触及 |

---

## 九、工作量 vs 实际

- 计划：~30min 浏览器定位 + 1h 修复 + 30min 测试
- 实际：~2h (含多次 rebuild 浪费时间 → 改 dev mode 节省后续时间)
- 教训：**AGENTS.md §2.5 应明确写"前端调试优先 dev mode, prod 验证才 rebuild"** → 已记入 R-14

---

## 十、下一步 (Sprint 112 建议)

| Item | 优先级 | 来源 |
|---|---|---|
| chat-svc title 异步生成修复（R-13 后端） | 🟢 low | R-13 |
| E2E-3 assessment Playwright + A2 单测补齐 | medium | Sprint 110 |
| E2E-4 reports Playwright + chartData 复测 | medium | Sprint 110 |
| AGENTS.md §2.5 补充 "dev mode 优先" 规则 | 🟡 medium | R-14 |
