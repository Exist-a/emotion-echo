---
status: landed
stage: 71
date: 2026-09-11
target: BFF 路由三方对齐（todo-pile §C8）—— 前端/BFF/APISIX 三方路径 source of truth + 契约测试
related:
  - docs/plans/todo-pile-2026-09-04.md §C8
  - emotion-echo-web-bff/main_test.go:48-107 wantRoutes + wantRoutesWithEmotionQ
  - emotion-echo-web/app/lib/apiRoutes.ts API_ROUTES
  - emotion-echo-web/app/lib/apiRoutes.test.ts vitest
  - docs/architecture/decisions.md 决策 4 §八 backlog（chat-svc Pin/Update 业务未触发）
---

# Stage 71 — BFF 路由三方对齐（C8 一次性永久关闭，2026-09-11）

> **状态**：🟢 **scripts/check_routes_alignment.sh 30/30 PASS · BFF v0.1.11 容器化 · e2e 验证 501 透传清晰**
> **触发**：todo-pile-2026-09-04.md §C8 "前端/BFF/APISIX 三方路径 source of truth + 契约测试，1.5~2 天"
> **关联**：Stage 64 PR-1 BFF 路由清单测试（main_test.go wantRoutes 27 条）+ Sprint 1 PR-2 前端 API_ROUTES 路径单点真理

---

## 一、目标

C8 todo-pile 描述：**前端、BFF、APISIX 三方各自硬编码路径，缺统一 source of truth + 契约测试**。任何一方改动忘了同步另两方，会出现 404/业务失败（不可见 bug）。

**最终架构**：
- **BFF = 路径 source of truth**（最底层，实现真实）
- **前端 API_ROUTES = 调用契约**（含 knownOrphans）
- **APISIX = 反向代理**（`/api/v1/*` 通配，无需路径对齐）
- **跨语言对齐脚本** 永久守护

---

## 二、现状（侦察发现）

| 部分 | 状态 |
|---|---|
| BFF `main_test.go` wantRoutes 切片 | ✅ Sprint 1 PR-1 已锁（27 条主路径 + 3 条 EmotionQ 条件分支）|
| 前端 `apiRoutes.ts` API_ROUTES + knownOrphans | ✅ Sprint 1 PR-2 已锁（26 条主 + 5 条 orphans）|
| 前端 `apiRoutes.test.ts` 扫所有 .vue/.ts 路径 | ✅ Sprint 1 PR-2 已锁（31 条断言）|
| **跨语言对齐脚本** | ❌ 本批新建（缺失闭环）|
| **真实路径漂移**（C8 实测发现）| ❌ 3 处未对齐 |

### 2.1 实测发现 3 处未对齐

`scripts/check_routes_alignment.sh` 首次运行结果（修复前）：

```
[FAIL] 以下前端路径在 BFF 中找不到对应:
  POST /api/v1/conversations/:id/pin
  PUT /api/v1/conversations/:id
  PUT /api/v1/user/profile
```

**根因**：
1. **`PUT /user/profile`** — 前端 store `updateProfile` 调用 PUT，但 BFF `user_handler.go:63` 只注册 `PATCH /users/me` → **生产 404**
2. **`PUT /conversations/:id`** — 前端 store `updateConversationTitle` 调用 PUT，但 BFF `chat_handler.go` 只注册 POST/GET/DELETE → **生产 404**
3. **`POST /api/v1/conversations/:id/pin`** — chat-svc PinConversation **未实现**（决策 4 ADR §八 backlog），BFF/前端都没路由

---

## 三、修复（commit `9eceb8d`）

### 3.1 新建跨语言对齐脚本

[`scripts/check_routes_alignment.sh`](scripts/check_routes_alignment.sh)：

- **BFF 解析**：`grep -oE '\{Method: "[A-Z]+", Path: "[^"]+"\}' emotion-echo-web-bff/main_test.go` + sed
- **前端解析**：awk 跳过 `knownOrphans:` 段，提取 `method: 'X', path: '/...'` 行（TypeScript 单引号字符串）
- **规范化**：前端 path 加回 `/api/v1` 前缀；method 大写统一
- **匹配规则**：BFF 通配参数（`:action / :kind / :id / :messageId / :conversationId / :resultId`）匹配任意具体值
- **断言**：前端主路径 ⊆ BFF；FAIL > 0 时退出码 1

### 3.2 修复 3 处未对齐

| # | 路径 | 修法 | 文件 |
|---|---|---|---|
| 1 | `PUT /user/profile` | 改 `PATCH /users/me`（前端 store `updateProfile` + `useApi.ts` 加 `patch()` helper）| [apiRoutes.ts](emotion-echo-web/app/lib/apiRoutes.ts) + [stores/user.ts](emotion-echo-web/app/stores/user.ts) + [useApi.ts](emotion-echo-web/app/composables/useApi.ts) |
| 2 | `PUT /conversations/:id` | BFF 加 `r.PATCH("/api/v1/conversations/:id", h.updateConversation)` 暂存 handler 返 501 Not Implemented；前端 `updateConversationTitle` 改纯本地更新 + TODO 注释（等 chat-svc 落地）| [chat_handler.go](emotion-echo-web-bff/internal/handler/chat_handler.go) + [stores/conversation.ts](emotion-echo-web/app/stores/conversation.ts) |
| 3 | `POST /conversations/:id/pin` | API_ROUTES 改 `pinConversationOrphan`（业务未触发保留调用入口但走 knownOrphans 路径）| [apiRoutes.ts](emotion-echo-web/app/lib/apiRoutes.ts) + [stores/conversation.ts](emotion-echo-web/app/stores/conversation.ts) |

### 3.3 BFF wantRoutes 同步

[main_test.go:62-67](emotion-echo-web-bff/main_test.go#L62) chat_handler.go 5 条 → 6 条（加 PATCH）：

```go
// ----- chat_handler.go (6 条，Stage 71 PR-C8 加 PATCH /:id) -----
{Method: "GET", Path: "/api/v1/conversations"},
{Method: "POST", Path: "/api/v1/conversations"},
{Method: "PATCH", Path: "/api/v1/conversations/:id"},  // 新增
{Method: "POST", Path: "/api/v1/conversations/:id/messages"},
{Method: "GET", Path: "/api/v1/conversations/:id/messages"},
{Method: "DELETE", Path: "/api/v1/conversations/:id"},
```

---

## 四、端到端实测

### 4.1 BFF 单测

```
$ cd emotion-echo-web-bff && go test -run "TestRegisterRoutes" -v
=== RUN   TestRegisterRoutes_MainContract
--- PASS (0.00s)
=== RUN   TestRegisterRoutes_WithEmotionQ
--- PASS (0.00s)
=== RUN   TestRegisterRoutes_NoUnknownPathPrefix
--- PASS (0.00s)
PASS
ok  	emotion-echo-web-bff  0.886s
```

### 4.2 跨语言对齐脚本

```
$ bash scripts/check_routes_alignment.sh
[check] BFF wantRoutes 条数: 35
[check] 前端 API_ROUTES 主路径: 31

[check] 路径对齐校验（前端主路径 ⊆ BFF 通配骨架）...
pass=30 fail=0
[PASS] 所有 30 个前端主路径在 BFF 中能找到对应（含通配匹配）

[check] 反向校验：BFF 业务路径 vs 前端调用
[WARN] 9 条 BFF 路径前端未直接调用：
  - GET /api/v1/emotion/{conversation,message,fused} (3 条)
  - GET /api/v1/mental-health/assessment
  - GET /api/v1/surveys/results{,/:resultId} (2 条)
  - GET /api/v1/users/{me,:id} (2 条)
  - POST /api/v1/tts/synthesize

  说明：这些是 BFF 内部 handler（如 /users/me 由 BFF 中间件链透传 + emotion
  路径给聊天前端用），前端 store 没直接调 → WARN 不计入 FAIL（设计预期）

PASS=1 FAIL=0
```

### 4.3 Docker 端到端

```
$ docker exec emotion-echo-postgres ... # 业务正常
$ curl -X PATCH -H "Content-Type: application/json" -H "X-User-Id: 1" \
  -d '{"title":"updated"}' \
  "http://localhost:8894/api/v1/conversations/14"
{"error":"chat-svc UpdateConversation RPC not implemented (decision 4 ADR §八 backlog)","id":14}
HTTP 501
```

✅ **PATCH 端到端走通**：契约对齐 + 业务透明告知（chat-svc 缺 UpdateConversation RPC，决策 4 ADR §八 backlog）。

---

## 五、调研依据

| 项 | 文件 / 命令 |
|---|---|
| BFF 路径 source of truth | [emotion-echo-web-bff/main_test.go:48-107](emotion-echo-web-bff/main_test.go#L48) |
| 前端 API_ROUTES | [emotion-echo-web/app/lib/apiRoutes.ts](emotion-echo-web/app/lib/apiRoutes.ts) |
| 对齐脚本 | [scripts/check_routes_alignment.sh](scripts/check_routes_alignment.sh) |
| BFF PATCH handler | [emotion-echo-web-bff/internal/handler/chat_handler.go:36-67](emotion-echo-web-bff/internal/handler/chat_handler.go#L36) |
| 前端 store alignment | [emotion-echo-web/app/stores/user.ts](emotion-echo-web/app/stores/user.ts) + [stores/conversation.ts](emotion-echo-web/app/stores/conversation.ts) |
| C8 todo-pile | [docs/plans/todo-pile-2026-09-04.md §C8](docs/plans/todo-pile-2026-09-04.md) |
| 决策 4 ADR §八 backlog | [docs/architecture/adr/adr-2026-09-decision-4-closure.md §八](docs/architecture/adr/adr-2026-09-decision-4-closure.md) |

---

## 六、本批未做（仍 open — chat-svc 业务未触发）

| 项 | 来源 | 工作量 |
|---|---|---|
| chat-svc PinConversation RPC（proto + server + logic + repo）| 决策 4 ADR §八 | 1 天 |
| chat-svc UpdateConversation RPC（proto + server + logic + repo）| 决策 4 ADR §八 | 1 天（前端 store `updateConversationTitle` 等落地后改回真实 API 调用）|
| Kafka P2：consumer lag 监控 + Protobuf 迁移 | kafka-reliability-gaps.md §1.4/§1.5 | 3~4 天 |
| Helm chart 与 compose dev 全面对齐 | todo-pile §D6 | 1 天 |
| gRPC mTLS（prod必做）| 决策 4 ADR §八 | 1 周 |

---

## 七、commit 时间线

```
9eceb8d fix(bff,web): Stage 71 C8 三方路径对齐
efe5b8a docs(stage): Stage 70 收口报告
c67f428 fix(svc): Stage 70 Kafka Sprint B P1
```

---

> 最后更新：2026-09-11 by Stage 71 实施 session
> 关联：todo-pile §C8 + Sprint 1 PR-1 (BFF 路由清单) + Sprint 1 PR-2 (前端 API_ROUTES) + 决策 4 ADR §八 backlog