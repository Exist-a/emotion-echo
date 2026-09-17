---
status: backlog
priority: high
owner: TBD
created: 2026-09-17
last-refresh: 2026-09-17
type: known-issues-backlog
---

# Runtime Known Issues (2026-09-17 增量登记)

> **本文件**：Sprint 110 调查过程中**逐一记录**每个新发现的环境/集成 bug，独立成项，独立排期。
> **来源**：用户 2026-09-17 反馈"项目是一坨屎，全是问题"，要求每个排查到的问题**单独记录**。

---

## R-01 · APISIX 上游 nodes 空 → 503

**现象**：POST `/api/v1/auth/login` 返回 `HTTP/1.1 503 Service Temporarily Unavailable`，但路由存在（`/apisix/admin/routes` 14 条）。

**根因**：
- 上游 ID 6 (web-bff) 配置 `discovery_type: nacos`，期望从 nacos 拉实例
- 但 `nacos/v1/ns/instance/list?serviceName=emotion-echo-web-bff&...` 返回 `hosts: []`
- **emotion-echo-web-bff 没注册到 nacos**
- 上游 nodes 空 → APISIX 无法路由 → 503

**调试路径**：
```bash
# 1. 查路由
curl -s http://localhost:9180/apisix/admin/routes -H 'X-API-KEY: WhZEPlrGviCSXlKFfALZlQWinluoGAbj'
# 看到 /api/v1/auth/login → upstream_id: 6, nodes: []

# 2. 查 upstream 6
curl -s http://localhost:9180/apisix/admin/upstreams/6 -H 'X-API-KEY: ...'
# 看到 discovery_type: nacos

# 3. 查 nacos 是否有 bff 实例
curl -s 'http://localhost:8848/nacos/v1/ns/instance/list?serviceName=emotion-echo-web-bff&namespaceId=emotion-echo-dev&groupName=DEFAULT_GROUP'
# 看到 hosts: []
```

**修复路径（待定）**：
- 重启 emotion-echo-web-bff，看是否自动注册 nacos
- 或 emotion-echo-web-bff 启动时 nacos 注册失败（需查 BFF 启动日志）
- 或 BFF 根本没启动 nacos client

**影响**：所有经 APISIX → BFF 的 API 全部 503，包括 login/chat/messages/ai-stream。

**优先级**：🔴 high（阻塞所有前端 E2E 测试）

---

## R-02 · APISIX X-RateLimit-Limit: 60 (限流过严)

**现象**：Response header `X-RateLimit-Limit: 60`，`X-RateLimit-Remaining: 57`（3 次后），`X-RateLimit-Reset: 49.13s`。

**根因**：APISIX 默认 60 req/min 限流。Sprint 110 期间反复调 API + Playwright 跑测试，超过 60 次/分钟触发。

**修复路径（待定）**：
- 调整 `deploy/apisix/seed.sh` route 100 catch-all 限流配置（`+ X-RateLimit-Limit` 调到 600）
- 或 Playwright 测试间 sleep 60s 等限流重置

**优先级**：🟡 medium（影响 E2E 频繁重跑）

---

## R-03 · Nuxt dev 按需 chunk 编译返回 503

**现象**：Playwright `await page.goto('/chat/conversation/new')` 后 `waitForURL` 超时（30s）。浏览器 console 报错 `Failed to load resource: 503 Service Temporarily Unavailable`。Nuxt dev 容器 `Listening on http://[::]:3000` 正常但首次加载某些 chunk 编译时返回 503。

**根因**：Nuxt 3 dev mode 是按需编译（vite SSR + client chunks），新页面跳转会触发服务端编译新 chunk，期间 APISIX 转发到 Nuxt dev 收到 503。

**修复路径（待定）**：
- 跳过 dev mode，用 production build（`node .output/server/index.mjs`）做测试
- 或 pre-warm 访问 `/login` + `/chat/conversation/new` 后再跑测试
- 或 Playwright webServer 设 `reuseExistingServer: true` + 手动 warm

**优先级**：🟡 medium

---

## R-04 · emotion-echo-web 容器跑 production build（源码修改不生效）

**现象**：源码修改后 `docker restart emotion-echo-web` 不生效，必须 `docker build` 重建镜像。

**根因**：Dockerfile CMD `["node", ".output/server/index.mjs"]` 跑的是 `npm run build` 预构建产物，HMR 对生产 build 无效。

**修复路径（待定）**：
- dev 模式验证用 dev 容器（CMD 改为 `npm run dev`）
- production 验证用 build 容器（每次源码修改都要 `docker build`）

**优先级**：🟡 medium（开发体验差）

---

## R-05 · docker Desktop build EOF 错误（强制 no-cache rebuild 失败）

**现象**：`docker build --no-cache` 报 `failed to receive status: rpc error: code = Unavailable desc = error reading from server: EOF`。

**根因**：Docker Desktop Windows + WSL2 集成下，no-cache 大型 build 上下文传输不稳定。

**修复路径（待定）**：
- 用 cache build（不加 `--no-cache`）
- 或分多次 build

**优先级**：🟢 low（build 时偶发）

---

## R-06 · IAB session 反复断连（webview not ready）

**现象**：`mcp__node_repl__js` 调用 browser tab 时报 `browser guest not attached (webview not ready)`。

**根因**：ZCode IAB (In-App Browser) webview 在多次调用后状态丢失，需重新 init 或开新 tab。

**修复路径（待定）**：
- 每次 browser call 前先 `tabs.list()` 验证
- 用 Playwright chromium 代替 IAB（Playwright 更稳定）

**优先级**：🟢 low（测试工具稳定性）

---

## R-07 · R-08 ...（后续排查中持续追加）

---

## 排查日志

- 2026-09-17 08:17 - 用户反馈"项目是一坨屎"，要求每个问题单独记录
- R-01 / R-02 排查路径：APISIX 503 → admin routes 查 → upstream 6 discovery_type nacos → nacos instance list hosts=[] → BFF 未注册

## R-09 · A8 真修复后 SSE 流回 "请求失败: JWT token invalid"

**现象**：A8 真修复后 IAB Playwright 浏览器端到端验收：`POST /api/v1/ai/stream` 触发成功，`.dialog-ai` 元素渲染了，但 AI 回复气泡显示 "请求失败: JWT token invalid"（来自 BFF ai_stream_handler error 分支）。

**根因候选**：
- Playwright 通过 `addCookies` + `evaluate` 点击 demo btn 登录路径，token 可能没正确写入 cookie
- 或 ai-stream 请求时 token 已过期（Sprint 108 useApi 401 重试逻辑没生效）
- 或 Playwright chromium headless 对 cookie 处理与真实浏览器不一致

**调试路径**：
```bash
# IAB 实测 2026-09-17 08:30：
# 1. POST /api/v1/ai/stream 触发成功（fetch hook 抓到）
# 2. .dialog-ai present: true（UI 渲染）
# 3. 但 BFF 返回 JWT token invalid，AI 回复是错误信息而非 mock 文案
```

**修复路径（待定）**：
- 排查 ai_stream_handler token 校验逻辑（emotion-echo-web-bff/internal/handler/ai_stream_handler.go:172-344）
- 或 useAIStreamHandler.ts:95 token 读取（cookie vs userStore）

**影响**：A8 修复成功（`.dialog-ai` 渲染），但 AI 回复内容不正确。

**优先级**：🟡 medium（A8 主路径已通，AI 内容正确性次要）

---

## R-10 · message.ts sendMessage happy path 漏返 data: message（A8 真根因 — 已修）

**现象**：`messageStore.sendMessage` 在 line 132 返回 `{ isOk: true, msg: '发送成功' }` —— **漏了 `data: message` 字段**。
**后果**：`useConversationSender.ts:106` `if (!persistResult.isOk || !persistResult.data)` 命中 → return `{ isOk: false, msg: '消息保存失败' }` → sendAIStream 永远不被调用 → A8。

**修复**（Sprint 110 commit）：
```diff
- return { isOk: true, msg: '发送成功' }
+ return { isOk: true, msg: '发送成功', data: message }
```

**验证**：Playwright chat-flow.spec.ts 2/2 PASS（happy-path-1 + happy-path-2），IAB 浏览器端到端 `.dialog-ai` 渲染。

**优先级**：✅ FIXED


---

## R-11 · R-09 真根因 — useAIStreamHandler Authorization 头为空 (Sprint 111 已修)

**现象**：Sprint 110 web 镜像 rebuild 后浏览器实测：POST /api/v1/ai/stream 触发，APISIX 返回 401 "JWT token invalid"，前端 fetch 拦截器抓到 headers 是 `{"Authorization":""}` 空字符串。BFF 日志 0 条 ai-stream 调用（BFF 未收到请求）。

**真根因**：chat-svc 等业务 Set-Cookie 是 HttpOnly，浏览器 JS 读不到 document.cookie → useCookie('access_token').value 永远空。useAIStreamHandler.ts:95 直接用 `useCookie('access_token').value || ''` 拼 Authorization，永远空。但 useApi.ts:116-128 已经用 fallback 链路 `userStore.getAccessToken → cookie`，能拿到。

**更深入的坑**：`user.ts:30` `const getAccessToken = computed(() => accessToken.value)` — **computed 是 ref 对象，不是字符串**！直接用 `store.getAccessToken` 拿到的是 ref，必须 `.value`。R-09 第一次修复 (PR 用 store.getAccessToken) 还是空字符串因为这个。

**修复**（Sprint 111）：

新建 `app/lib/clientAccessToken.ts`:
```ts
export function getClientAccessToken(): string {
  if (!import.meta.client) return useCookie('access_token').value || ''
  try {
    const userStore = useUserStore()
    // 注意: getAccessToken 是 computed ref 对象, 必须 .value
    const raw = userStore?.getAccessToken
    const storeToken = typeof raw === 'string' ? raw : (raw as any)?.value || ''
    if (storeToken) return storeToken
  } catch {}
  return useCookie('access_token').value || ''
}
```

`useAIStreamHandler.ts` / `useAIStream.ts` / `useTTSPlayer.ts` 全部改用 helper。

**验证**（dev mode 2026-09-17 09:38）：
- 浏览器 fetch 拦截器: `Authorization: Bearer eyJhbG...`
- APISIX 200 OK
- BFF /chat-svc / ai-svc 全链路通
- AI 回复气泡显示正常（"嗯，我能理解你的心情..."）

**优先级**：✅ FIXED

---

## R-12 · A11 sidebar 会话标题为空 bug (Sprint 111 已修)

**现象**：浏览器实测 sidebar 21 个会话全部显示空标题，aria-label "对「」更多操作"（「」里是空）。用户找不到历史对话。

**真根因**：后端 chat-svc /conversations 接口返回的 conversation title 经常为空（chat-svc 在 SSE 流完后异步用 AI 生成 title，写库前 list API 读不到）+ lastMessage 也可能是 null。前端 `pages/chat/conversation/index.vue:150` `label: c.title` 直接用，没 fallback。

**curl 验证**：
```bash
curl http://localhost:19080/api/v1/conversations?limit=3
# {"id":"125", "title":"", "lastMessage":null, ...}
```

**修复**（Sprint 111）：3 级 fallback chain：
```ts
label: c.title?.trim()
  || (c.lastMessage ? String(c.lastMessage).slice(0, 30) : '')
  || `对话 #${c.id}`,
```

**验证**：浏览器实测 21 个会话全部显示 "对话 #125" 等可识别标签，新建会话有 AI 回复时显示 lastMessage 前 30 字作为 label。

**优先级**：✅ FIXED

---

## R-13 · chat-svc title 异步生成时机问题 (后端，建议 Sprint 112)

**现象**：R-12 修复前端 fallback 后能显示 "对话 #125"，但用户期望的 "今天聊了xxx" / 自动 AI 摘要 仍然不可见。Sprint 109b 提到 chat-svc 在 SSE 完后调 AI 生成 title，但 list API 读不到。

**根因候选**：
- chat-svc generateTitle 异步任务排队但 list API 没等它
- 或 title 生成了但写库后没 invalidate cache
- 或 title 生成失败 fallback 一直是空

**修复路径（待定）**：
- 排查 chat-svc generateTitle 流程
- 列出当前已生成 vs 未生成的会话比例
- 决定是改 list API 还是前端 polling

**影响**：用户体验 — 新建会话要等几秒才有 AI 摘要标题；前端 fallback 是 workaround 不是根本修复。

**优先级**：🟢 low（R-12 已用 fallback 缓解，建议 Sprint 112 排期后端修复）

---

## R-14 · 重建 web 镜像 → 容器跑生产模式源码不生效 (dev 体验)

**现象**：每次改 web 前端代码后必须 `docker build --no-cache emotion-echo-web` (~1 min) + `docker compose up -d` 才生效。APISIX 还有 503 等不稳定因素叠加，单次修复常常要 rebuild 多次。

**根因**：`emotion-echo-web/Dockerfile` 跑的是 `node .output/server/index.mjs` 生产 build，源码改动需重新 build 镜像。

**修复路径**：
- Sprint 111 起约定: dev 模式验证用 `pnpm dev` 本地启动 (3000 端口), 修改实时生效 (HMR)
- prod 验证才 rebuild 镜像
- 详见 docs/AGENTS.md §2.5

**影响**：开发体验 — 用户多次提醒"先本地启动再 build"。

**优先级**：🟡 medium（流程优化，非阻塞）

---

## R-15 · 报表页面无法滚动（内容超出视口被裁切）— 待检查

**状态**：⏳ 待检查（用户 2026-09-17 Sprint 112 收口后报告，标记为待验证项，**本轮未修**）

**现象**：用户反馈"点击报表页面好像滑动不了"——进入日报/周报/月报/年报页后，当内容高于视口时无法向下滚动，超出部分不可见。

**初步定位（已读代码，未实测确认）**：
- `emotion-echo-web/app/layouts/nav.vue:144`：`.page-content { height: calc(100dvh - 88px); overflow: hidden; }`
  —— 固定高度 + `overflow: hidden`，内容超出直接被**裁掉**，且没有内部滚动容器。
- `emotion-echo-web/app/components/report/ReportScaffold.vue:116`：`.report-scaffold { display: grid; ... }`
  —— 无任何 `overflow` / 滚动设定；子项 `.report-charts { min-height: 240px }` 等可叠加超屏。
- 对比：对话页（`chat/conversation/*`）有独立高度链路（`.conversation-page` → `.chat-area` → 内部 `overflow-y: auto`，Stage 104/105 修复过），所以对话页不受影响。

**待检查清单**（下一轮执行）：
1. 实测确认受影响页面范围：报表 4 页 / 我的空间 / 设置 / 心理测验（预期：所有走"下载流"的 `nav` layout 页面都受影响；对话页不受影响）
2. 确认窗口尺寸依赖性：1366×768 等小高度视口下必现，大屏可能不明显（这解释了"好像"）
3. 修复方向 A（推荐）：`.page-content` 改 `overflow-y: auto`（把滚动交给页面层）；必须回归对话页高度链路（Stage 104/105 契约测试 `layouts/nav.test.ts` 会挡）
4. 修复方向 B：保留 `.page-content: hidden`，在报表页/ReportScaffold 内加 `overflow-y: auto; min-height: 0` 容器（改动局部，但要逐个页面加）
5. 写 Playwright/vitest 契约把"报表页可滚动"钉死

**优先级**：🟠 medium-high（用户可感知的 UI 缺陷，影响 4 个报表页 + 3 个辅助页；不阻塞业务链路）

**影响文件**（预计）：
- `emotion-echo-web/app/layouts/nav.vue`（方向 A）
- `emotion-echo-web/app/components/report/ReportScaffold.vue`（方向 B）
- `emotion-echo-web/app/layouts/nav.test.ts`（契约回归）

---

## 排查日志

- 2026-09-17 08:17 - 用户反馈"项目是一坨屎"，要求每个问题单独记录
- R-01 / R-02 排查路径：APISIX 503 → admin routes 查 → upstream 6 discovery_type nacos → nacos instance list hosts=[] → BFF 未注册
- 2026-09-17 09:38 - R-09 真根因锁定 (HttpOnly cookie + useCookie() 读不到 + computed ref 不取 .value)
- 2026-09-17 09:38 - R-12 锁定 (chat-svc title 异步生成 + 前端无 fallback)
- 2026-09-17 09:39 - 决定 R-14 流程优化 (dev 模式优先, rebuild 仅 prod 验证用)
- 2026-09-17 Sprint 112 收口后 - 用户报告报表页"滑动不了"→ 登记 R-15（待检查，已初步定位 `.page-content overflow:hidden` + ReportScaffold 无滚动容器；下一轮实测 + TDD 修复）
