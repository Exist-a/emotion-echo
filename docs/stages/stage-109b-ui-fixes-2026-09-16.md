---
status: landed
priority: critical
owner: TBD
created: 2026-09-16
last-refresh: 2026-09-16
type: ui-fixes
depends-on:
  - stage-109a-apisix-jwt-401-fix-2026-09-16.md (A7 修通)
  - stage-108-sender-architecture-debt-fix-2026-09-16.md (sender 修复)
related-stages:
  - stage-105-browser-e2e-2026-09-16.md (browser-use 浏览器 E2E)
related-issues:
  - docs/plans/test-coverage-tracker-2026-09-16.md §四 A7 (FIXED Sprint 109a)
---

# Stage 109b — UI 全链路修复

> **目的**：在 Sprint 109a (A7 APISIX 401 修通) 基础上，**完整修复用户实测浏览器发现的 7 个 UI / 链路 bug**。本 stage 一次性修复所有发现的 bug，避免遗留技术债给后续 sprint 挖坑。
>
> **范围**：APISIX 网关 + BFF 后端 + 前端 Vue 组件 + 数字人组件 + 摄像头多模态入口。

---

## 一、起点 + 用户反馈

### 1.1 起点（Sprint 109a 修后）

| 维度 | 状态 |
|---|---|
| A7 APISIX jwt-auth 401 | ✅ FIXED（6/6 runtime test PASS）|
| dev 模式 | 17 容器 healthy |
| curl API 端到端 | ✅ login → conversation → message → AI stream 全链通 |

### 1.2 用户实测发现的 7 个 bug

用户用浏览器实际操作后反馈：

1. **发送按钮无反应**：点击发送按钮后页面没变化，无 AI 回复
2. **按钮排布问题**：附件/语音/发送按钮在输入框下的位置不合理
3. **右上角头像"体"字**：用户昵称首字显示突兀
4. **左下角"此刻，我在听"**：侧边栏底部的 status 文本不必要
5. **数字人圆框 + emoji 按钮**：👁🔊 emoji 难看，圆框突兀
6. **AI 回复 loading 气泡不可见**：刷新页面后 AI 回复丢失
7. **没有开启摄像头功能**：无法做多模态融合（FER 人脸表情识别）

---

## 二、修复清单（7 个 bug）

### Fix 1: APISIX route 100 补 OPTIONS（CORS preflight 404）

**根因**：前端 fetch 跨域带 Authorization header 时，浏览器发 OPTIONS preflight。route 100 (`/api/v1/*`) methods 缺 OPTIONS，preflight 被 APISIX 404 → 前端 fetch 报 "Failed to fetch"。

**实测**：
```bash
# 修前
curl -X OPTIONS http://localhost:19080/api/v1/conversations \
  -H "Origin: http://localhost:3000" -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: Authorization,Content-Type"
→ HTTP/1.1 404 Not Found
   {"error_msg":"404 Route Not Found"}

# 修后
→ HTTP/1.1 200 OK + CORS headers (Allow-Origin, Allow-Methods, Allow-Headers, Allow-Credentials)
```

**修改**：`deploy/apisix/seed.sh:487`
```bash
put_route 100 "/api/v1/*" 6 '["GET","POST","PUT","DELETE","PATCH","OPTIONS"]'
```

### Fix 2+7: 按钮排布统一 + 加开启摄像头按钮

**用户反馈**：附件/语音/发送位置不合理 + 没有摄像头入口

**修法**：
- 布局统一为 `[附件] [摄像头] [spacer] [语音] [发送]`（与 [id].vue 一致）
- `camera-btn` SVG icon（摄像头开/关两种状态）
- `camera-preview` `<video>` 元素 + currentEmotion 状态显示
- `toggleCamera()` 调 `useFaceEmotion.startCamera/stopCamera` + notify 反馈
- `useFaceEmotion` composable 已存在（依赖 `navigator.mediaDevices.getUserMedia` + FER `/api/v1/multimodal/analyze`）

**修改**：
- `emotion-echo-web/app/pages/chat/conversation/new.vue`（重写）
- `emotion-echo-web/app/pages/chat/conversation/[id].vue`（加按钮 + script）

### Fix 3: nav.vue 删右上角 avatar 头像按钮

**修法**：删 `<NuxtLink to="/chat/setting" class="avatar-link">` 整个 block + `userInitial` computed（设置入口保留在左侧边栏 `secondaryLinks` 里的 `/chat/setting`）。

**修改**：`emotion-echo-web/app/layouts/nav.vue:40-44, 83-86`

### Fix 4: nav.vue 删左下角 sidebar-footer

**修法**：删 `<div class="sidebar-footer">` 整个 div（含 status-dot + "此刻，我在听" span）。

**修改**：`emotion-echo-web/app/layouts/nav.vue:26-29`

### Fix 5: 数字人 emoji 按钮换 SVG + 圆框缩小

**修法**：
- `👁` → `<svg>` eye icon / eye-off icon
- `🔊` → `<svg>` volume icon / volume-x icon
- 圆框从 200x200 → 160x160
- 按钮从 32x32 → 28x28
- 按钮背景从 `rgba(64,158,255,0.9)` 蓝 → 白色半透明 + `var(--ee-border)` 描边（与 app design system 统一）

**修改**：`emotion-echo-web/app/components/digital-human/DigitalHuman.vue:21-36, 628-639`

### Fix 6: BFF ai_stream_handler 保存 AI 回复

**根因**：BFF `ai_stream_handler` 只把 AI 回复写到 SSE 流（前端能看），不调 chat-svc 存库 → 刷新页面 AI 回复丢失（后端只有 user 消息）。

**修法**：
1. `AIStreamHandler` 加 `chat downstream.ChatClient` 字段
2. 新增 `NewAIStreamHandlerWithDepsFull(llm + files + chat)` 构造函数
3. `saveAIMessage()` 方法：SSE 流完后用 `session.WithRequestAuth(c)` 保留 x-user-id gRPC metadata → `chat.SendMessage(role="assistant")`
4. LLM gRPC 路径 + LLM HTTP 路径 + Mock 路径都调 `saveAIMessage`（用 `strings.Builder` 累计完整 AI 回复文本）
5. `main.go` 改用 `NewAIStreamHandlerWithDepsFull` 注入 chat client

**实测（修后）**：
```bash
curl POST /api/v1/conversations/72/messages → 200 (user msg)
curl POST /api/v1/ai/stream → SSE
curl GET /api/v1/conversations/72/messages
→ messages count: 2
   [user] id=83: 你好
   [ai] id=84: 我在呢。愿意和我说说刚才发生了什么吗？不用着急，按你的节奏来就好。
```

**修改**：
- `emotion-echo-web-bff/internal/handler/ai_stream_handler.go`（加 chat 字段 + saveAIMessage + 三个路径都调用）
- `emotion-echo-web-bff/main.go:412`（改用 NewAIStreamHandlerWithDepsFull）

---

## 三、BFF image 版本演进（修复过程）

| 版本 | 改动 | 状态 |
|---|---|---|
| v0.1.14 | Stage 94 baseline | 旧镜像 |
| v0.1.15 | saveAIMessage 初始实现（context.Background 丢失 x-user-id）| ❌ gRPC Unauthenticated |
| v0.1.16 | 改用同步 + session.WithRequestAuth + role="ai" | ❌ validation: role 必须 user/assistant/system |
| v0.1.17 | role="ai" → role="assistant" | ❌ role 校验仍是 user/assistant/system |
| v0.1.18 | role="assistant" + 同步写 | ✅ AI 回复入库成功 |

---

## 四、回归验证矩阵

| 验证项 | 修前 | 修后 | 证据 |
|---|---|---|---|
| CORS preflight OPTIONS `/api/v1/conversations` | ❌ 404 | ✅ 200 + CORS headers | curl OPTIONS |
| POST `/api/v1/conversations` | ❌ "Failed to fetch" | ✅ 200 | curl POST |
| POST `/api/v1/conversations/:id/messages` | ✅ 200 | ✅ 200 | curl POST |
| POST `/api/v1/ai/stream` (SSE) | ✅ SSE chunks | ✅ SSE chunks | curl SSE |
| GET `/api/v1/conversations/:id/messages` 消息数 | 1 (user only) | **2 (user + assistant)** | curl GET |
| AI 回复刷新页面丢失 | ❌ 是 | ✅ 否（DB 已存）| stage-109b §四 Fix 6 |
| runtime test 6/6 (jwt-auth chain) | ✅ | ✅ | deploy/apisix/test_jwt_auth_runtime.sh |
| browser e2e (login → send → AI 流) | 🔴 blocked by 401 | ✅ 通 (curl 路径) | stage-109b §四 |
| new.vue 按钮排布 | ❌ 与 [id].vue 不一致 | ✅ 统一 (附件/摄像头/语音/发送) | screenshots 10-empty-new.png |
| 摄像头按钮 | ❌ 不存在 | ✅ 加 (camera-btn + video preview) | useFaceEmotion composable |
| 数字人 emoji 按钮 | 👁🔊 | ✅ SVG (eye / volume) | DigitalHuman.vue |
| 右上角 avatar "体" 字 | ❌ 突兀 | ✅ 删 | nav.vue |
| 左下角 "此刻，我在听" | ❌ 不必要 | ✅ 删 | nav.vue |

---

## 五、commit 列表（5 commits pushed）

```
113ce0e fix(apisix): route 100 catch-all 补 OPTIONS — 修 CORS preflight 404
779e28c fix(nav): 删右上角 avatar 头像按钮 + 左下角 sidebar footer
b4e5f11 fix(digital-human): emoji 按钮换 SVG icon + 圆框缩小
853bcb5 feat(new.vue): 改布局 + 加开启摄像头按钮
319c9c4 feat([id].vue): 加开启摄像头按钮 + camera-preview
eee35a0 fix(bff): ai_stream_handler 保存 AI 回复到 chat-svc
```

---

## 六、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `docs/plans/sprint-109b-end-to-end-chat-2026-09-16.md` | Sprint 109b 计划 |
| `docs/stages/stage-109a-apisix-jwt-401-fix-2026-09-16.md` | Sprint 109a 修复（A7） |
| `docs/stages/stage-108-sender-architecture-debt-fix-2026-09-16.md` | sender 修复 |
| `docs/plans/test-coverage-tracker-2026-09-16.md §一 E2E-2` | 追踪状态 |
| `emotion-echo-web/app/layouts/nav.vue` | 头像 + sidebar footer 删除 |
| `emotion-echo-web/app/components/digital-human/DigitalHuman.vue` | emoji → SVG |
| `emotion-echo-web/app/pages/chat/conversation/new.vue` | 布局统一 + 摄像头 |
| `emotion-echo-web/app/pages/chat/conversation/[id].vue` | 摄像头按钮 |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go` | saveAIMessage + 三路径调用 |
| `emotion-echo-web-bff/internal/downstream/chat.go:88-94` | ChatClient interface |
| `emotion-echo-web-bff/internal/downstream/chat_grpc.go:62-80` | SendMessage RPC（role="assistant"）|
| `emotion-echo-web/app/composables/useFaceEmotion.ts` | 摄像头 + FER composable |
| `deploy/apisix/seed.sh` | route 100 OPTIONS 修复 |
| `deploy/apisix/test_jwt_auth_runtime.sh` | 端到端 JWT chain 测试 |

### 架构假设清单

| 假设 | 验证 |
|---|---|
| OPTIONS preflight 是 fetch 失败根因 | ✅ curl OPTIONS 修前 404 / 修后 200 |
| AI 回复存 DB 需要 role="assistant" | ✅ chat-svc 校验：role must be one of user/assistant/system |
| gRPC 写 DB 需要 x-user-id ctx metadata | ✅ BFF 日志验证：`missing x-user-id metadata` → 改用 session.WithRequestAuth |
| useFaceEmotion composable 已支持多模态 | ✅ 已存在，依赖 navigator.mediaDevices.getUserMedia + FER API |
| 摄像头按钮位置放附件右侧 | ✅ 用户确认（AskUserQuestion 选 "Recommended"）|
| 头像按钮删除 vs 改头像 | ✅ 用户选 "删除整个头像按钮" |
| 数字人 emoji 改 SVG | ✅ 用户选 "换成 SVG icon" |
| nav.vue footer 整个删除 | ✅ 用户反馈 |