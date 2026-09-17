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

