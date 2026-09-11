---
status: landed
stage: 64
date: 2026-09-11
target: bug 类未做事项集中收口（chat-svc 中间件分层 + #32 根因 + 杂项 + 文档失真关闭）
related:
  - docs/architecture/adr/adr-2026-09-decision-4-closure.md §八
  - docs/architecture/adr/adr-2026-09-doc-drift-registry.md #32
  - docs/plans/todo-pile-2026-09-04.md §G / §D
  - docs/plans/grpc-inter-service-migration.md §一.3
---

# Stage 64 — bug 类未做事项集中收口（2026-09-11）

> **状态**：🟢 **5 PR 全 landed · 决策 18 #32 关闭 · chat-svc v0.1.3 容器化**
> **关联**：Stage 63 Final gRPC 化收口之后，针对"零碎未做 bug"的集中清理。

---

## 一、目标

把 todo-pile §G + 决策 18 #32 + Sprint 后暴露的零碎 bug 集中收口，按"由简到难"5 个 PR 一次落地。

---

## 二、PR 全清单与 commit

| PR | 范围 | commit | 状态 |
|---|---|---|---|
| **PR-1** | 杂项清理（删 ;D 残影 + chat-svc main.go WithOutboxRepo 重复注入）| `6628124` | 🟢 landed |
| **PR-2** | chat-svc 中间件分层（/health + /metrics 免鉴权 + 业务群 auth.Use 分组）| `229133c` (RED) + `602e39a` (GREEN) | 🟢 landed |
| **PR-3** | ListConversations 端到端 + #32 根因排查 + chat-svc v0.1.3 rebuild | `16661a3` (RED) + `3e07571` (GREEN) | 🟢 landed |
| **PR-4** | 前端 quickLogin 注释清理 | `7d8c1d6` | 🟢 landed |
| **PR-5** | docs 收口（决策 4 ADR §八 + 决策 18 #32 关闭追踪）| 本批 | 🟢 landed |

**总耗时**：3.5 小时（按计划），实际 5 PR 串行。

---

## 三、PR-1 · 杂项清理（commit `6628124`）

### 3.1 修复内容

1. **chat-svc/main.go** 删 `svcCtx.WithOutboxRepo(outboxRepo)` 第二次注入（173-175 行重复 setter 调用，与 155-157 行同义）
2. **.gitignore** 加 `*;D` 规则（Windows mv/rename 失败时残留的"Duplicate name"影子目录防御）
3. 删 `emotion-echo-shared;D` / `scripts/grpc_smoke;D` 两个本地残影目录

### 3.2 调研依据

- `chat-svc/main.go:155-157` 与 `173-175` 两块逻辑等价（同一 outboxRepo 实例）
- `;` 在 Windows 是非法字符，Git Bash mv 偶发 rename 失败会留 `<name>;D` 残影

---

## 四、PR-2 · chat-svc 中间件分层（commits `229133c` + `602e39a`）

### 4.1 问题（根因级修复）

[chat-svc/main.go:214](emotion-echo-chat-svc/main.go) 原代码：

```go
r.Use(sharedmw.GinAuthMiddleware())  // 全局挂
r.GET("/health", handler.HealthHandler(svcCtx))
r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))
```

**后果**：K8s liveness probe 与 Prometheus scrape 必须无 X-User-Id header 调用 /health 与 /metrics——**全局中间件强制 401 阻断**。

### 4.2 修复

仿 `user-svc/main.go:147-155` 范式：业务端点改用 `auth := r.Group("/api/v1"); auth.Use(sharedmw.GinAuthMiddleware())`，/health 与 /metrics 在 `r.Use(auth)` 之前注册。

### 4.3 测试

[chat_handler_test.go](emotion-echo-chat-svc/internal/handler/chat_handler_test.go) 新增 3 用例：
- `TestChatHandler_Health_NoUserID_Returns200`
- `TestChatHandler_Metrics_NoUserID_Returns200`
- `TestChatHandler_BusinessRoute_NoUserID_Returns401`

`newPR2Router` 复刻 PR-2 目标形态作为契约守卫，任何后续重构把 auth 全挂或 /health 移进 auth 群会失败。

---

## 五、PR-3 · ListConversations 端到端 + #32 根因（commits `16661a3` + `3e07571`）

### 5.1 RED 测试

[chat_handler_test.go](emotion-echo-chat-svc/internal/handler/chat_handler_test.go) 新增 5 用例：

| 测试 | 场景 |
|---|---|
| `TestListConversationsHandler_Happy_ReturnsUserScopedList` | seed uid=7/8 各会话，X-User-Id:7 → 200 + 仅 uid=7 的 2 条（用户隔离）|
| `TestListConversationsHandler_Pagination_HasMore` | seed 5 条 + limit=2 → hasMore=true + 2 条；limit=10 → hasMore=false + 5 条 |
| `TestListConversationsHandler_EmptyUser_ReturnsEmptyList` | uid=9999 无会话 → 200 + 空 list + hasMore=false（不能返 500 也不能 nil）|
| `TestListConversationsHandler_InvalidLimit_DefaultsTo20` | ?limit=abc → 默认 20 而非 0/400 |
| `TestListConversationsHandler_NoUserID_Returns401` | 业务群内必须鉴权兜底 |

### 5.2 #32 根因排查（commit `3e07571`）

**docker 端到端实测**（chat-svc v0.1.3 rebuild 后）：

| 路径 | 结果 |
|---|---|
| `/health` 无 X-User-Id | 200 ✅ |
| `/metrics` 无 X-User-Id | 200 + prometheus format ✅ |
| `/api/v1/conversations` 无 X-User-Id | 401 ✅ |
| `/api/v1/conversations?limit=5` + `X-User-Id: 1` | **200 + `{"list":[],"hasMore":false}`** ✅ |

**#32 500 不复现**。

**结论**：500 现象源自更早版本镜像（v0.1.0 前后）的中间件/handler 时代 bug，已在多次重构（PR-2 中间件分层 + Sprint D chat-svc gRPC 4 RPC 实现 + PR-3 ListConversations 端到端测试）中无意修复。决策 18 #32 关闭。

### 5.3 镜像升级

[docker-compose.apps.yml:133](deploy/docker-compose.apps.yml) image tag `v0.1.2` → `v0.1.3`，chat-svc 容器按 PR-2 分层契约生效。

---

## 六、PR-4 · 前端 quickLogin 一致性（commit `7d8c1d6`）

[emotion-echo-web/app/pages/login/index.vue:178-180](emotion-echo-web/app/pages/login/index.vue) 注释由长篇历史叙述（Stage 38-A + demo@emotion-echo.com + 后端无 quick-login 端点）改为简短现状描述。

**现状说明**：
- 后端从未实现 `/auth/quick-login` 端点
- 前端 `quickLogin()` 函数调 `userStore.login` 标准端点（POST /api/v1/auth/login）
- seed 默认账号 echo / echo123 由 `deploy/db/03-seed-default-users.sql` 提供
- 决策 18 todo-pile §C6 关闭

---

## 七、PR-5 · docs 收口（本次 commit）

- `adr-2026-09-decision-4-closure.md` §八：#32 行加删除线 + 关闭追踪（PR-3 commit）
- `adr-2026-09-doc-drift-registry.md` #32 行：新增关闭追踪块（PR-3 `3e07571` 端到端实测结论）

---

## 八、本批未做（仍 open）

| 项 | 来源 | 工作量 |
|---|---|---|
| chat-svc PinConversation gRPC | 决策 4 ADR §八 | 1 天（业务未触发）|
| chat-svc StreamMessages gRPC | 决策 4 ADR §八 | 1 天（无流式业务）|
| 错误码统一映射（4 svc 各自 mapLogicError / mapAuthError）| 决策 4 ADR §八 | 1 天 |
| gRPC mTLS（dev insecure → prod mTLS）| 决策 4 ADR §八 | 1 周 |
| BFF 路由清单三方对齐（C8 todo-pile）| todo-pile §G | 1.5~2 天 |
| Helm chart 与 compose dev 全面对齐（D6 todo-pile）| todo-pile §D6 | 1 天 |
| chat-svc 表依赖清单 ADR（D5 todo-pile）| todo-pile §D5 | 半天 |

---

## 九、调研依据汇总

| PR | 已读文件 / 已查文档 / 已跑验证 |
|---|---|
| PR-1 | `chat-svc/main.go:155-175` + `.gitignore` + Windows mv 行为 |
| PR-2 | `user-svc/main.go:147-155`（noAuth group + r.Use 范式）+ `chat_handler_test.go` 现有 4 用例 |
| PR-3 | `chat-svc/internal/handler/chat_handler.go:62 ListConversationsHandler` + `logic/listconversationslogic.go:38-79` + `repository/conversation_repository.go:320 ListConversations` + `model/conversation.go` + `types/types.go:74-84` + `grpcserver/chat_server.go:155 ListConversations` + `shared/pkg/middleware/gin_auth.go:24-27`（白名单机制）+ `sprint-d-chat-grpc-implementation.md` + `sprint-e-user-grpc-auth.md` + 决策 18 #32 |
| PR-4 | `emotion-echo-web/app/pages/login/index.vue:175-194` + `deploy/db/03-seed-default-users.sql:18` |
| PR-5 | `adr-2026-09-decision-4-closure.md §八` + `adr-2026-09-doc-drift-registry.md #32` |

---

## 十、变更文件清单

```
emotion-echo-chat-svc/main.go                                       (PR-1 + PR-2)
emotion-echo-chat-svc/internal/handler/chat_handler_test.go         (PR-2 + PR-3)
deploy/docker-compose.apps.yml                                       (PR-3)
emotion-echo-web/app/pages/login/index.vue                           (PR-4)
docs/architecture/adr/adr-2026-09-decision-4-closure.md              (PR-5)
docs/architecture/adr/adr-2026-09-doc-drift-registry.md              (PR-5)
.gitignore                                                           (PR-1)
```

共 **7 个文件**，**5 个 commit**，**3.5 小时净工作时段**。

---

> 最后更新：2026-09-11 by Stage 64 实施 session