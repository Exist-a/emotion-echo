---
status: landed
stage: 66
date: 2026-09-11
target: 剩余 bug 类 4 项集中收口（决策 20 sign-off + dashboard 空根因 + chat-svc 表依赖 ADR + 错误码统一映射）
related:
  - docs/architecture/decisions.md 决策 20 / 决策 22
  - docs/architecture/adr/adr-2026-09-env-profile-strategy.md
  - docs/architecture/adr/adr-2026-09-chat-svc-table-deps.md
  - docs/stages/stage-65-dashboard-empty-root-cause.md
  - docs/architecture/adr/adr-2026-09-decision-4-closure.md §八
---

# Stage 66 — 剩余 bug 类 4 项集中收口（2026-09-11）

> **状态**：🟢 **4 PR 全 landed · 5 svc 容器化到 v0.1.4 · 决策 18 #32 + #23 关闭**
> **触发**：上一会话对剩余 bug 类整理（todo-pile + 决策 4 ADR §八 backlog + 决策 18 失真台账）
> **4 项**：
>
> 1. **B1** 决策 20 owner sign-off（ADR-20 + decisions.md + README）
> 2. **B3** Stage 36 dashboard `chartData.length===0` 根因诊断（纯文档）
> 3. **B2** chat-svc 表依赖清单 ADR（防 PR-GRPC 错装 stage 38 重演）
> 4. **B4** 错误码统一映射（5 svc 接入 grpcerr + 19 处 status.Errorf 散写归零 + 补 BFF 409）

---

## 一、4 项落地清单

### 1.1 B1 ·决策 20 owner sign-off（commit `585082e`）

**变更**：
- `decisions.md` 决策 20 区块头：🟡 Proposed / 待决策 → ✅ Accepted（2026-09-11 owner sign-off）
- `decisions.md` 决策 20 末尾追加 Owner Sign-off 区块（不动历史更正块）
- `adr-2026-09-env-profile-strategy.md` 头 4 行：Proposed / ⏸ 未开始 → Accepted / ✅ 完成
- `README.md` 顶部 Status 表追加决策 20 行 + Stage 61~63 / Stage 64 行（同步 git log）

**决策 18 #23 关闭**：决策 20 失真台账闭环。

### 1.2 B3 · Stage 65 dashboard 空根因诊断（commit `b357f31`）

**纯文档**：`docs/stages/stage-65-dashboard-empty-root-cause.md` 报告。

**根因（实测 docker + BFF curl）**：

| 数据层 | 状态 | dashboard 表现 |
|---|---|---|
| `emotion_echo_chat.conversations / messages` | **0 行**（demo 用户无聊天数据）| ❌ |
| `emotion_echo_chat.msg_summary_v`（视图）| **0 行**（依赖 conversations + messages）| ❌ |
| `emotion_echo_ai.emotion_analysis` | 4 行（KAFKA_ENABLED=true 时期残留）| 部分 |
| `emotion_echo_ai.daily_emotion_v`（视图）| 4 行 | 部分（按 user_id + date 过滤后 0）|
| `emotion_echo_analytics.user_behavior_events` | 12 行 | 部分 |

**结论**：
- daily/weekly/monthly/annual `chartData.length===0` **根因 = demo 用户无真实聊天流量**，`conversations`/`messages` 表 0 行
- **与 KAFKA_ENABLED 无关**——daily report 走 `msg_summary_v` 聚合 chat-svc 表，不走 Kafka 路径
- Stage 36-FU 报告把 4 dashboard 全归因 Kafka off 是过度推断（count 类字段与 distribution 类字段混合原因）

**修复路径（下次 sprint）**：
- 选项 A：`scripts/seed_demo_chat.sql` + db-migrate 接入（30 分钟）
- 选项 B：demo 登录后自动 bot reply 1 次对话（半天）
- 合计 1 天

### 1.3 B2 ·决策 22 chat-svc 表依赖清单 + 字段变更契约（commit `8b66d2a`）

**新 ADR**：`docs/architecture/adr/adr-2026-09-chat-svc-table-deps.md`

**范围**：
- chat-svc 3 张表（`conversations` / `messages` / `outbox_events`）字段 + 索引 + 跨域引用方清单
- 跨域依赖矩阵（chat-svc 表 → analytics-svc `msg_summary_v` / ai-svc Kafka consumer / 前端 dashboard）
- 跨域写入约束（chat-svc **零跨 svc 写入**；event_id 生成是跨 svc 契约）
- 字段变更 9 步 checklist（migration → model → types → proto → 跨域评估 → 集成测试 → regression 守护 → 文档同步）
- 6 类禁止事项（跨 schema 外键 / JSONB 字段塞关键数据 / 删除 outbox 行 / 改 UNIQUE 约束 / 跨 svc 直接写库）

**注册**：`decisions.md` 决策 22 = ✅ Accepted。

### 1.4 B4 ·错误码统一映射（commits `edc016e` + `9aff13a` + `f57ba9f`）

#### 1.4.1 shared 包新建设计

`emotion-echo-shared/pkg/grpcerr/`：
- 6 类通用 sentinel errors（`ErrInvalidArgument` / `ErrUnauthenticated` / `ErrPermissionDenied` / `ErrNotFound` / `ErrUnavailable` / `ErrAlreadyExists`）
- `Map(err) (codes.Code, string)` 通用 helper：sentinel `errors.Is` 优先 → 通用 sentinel → 字符串前缀 → `context.DeadlineExceeded` → 默认 Internal
- `MapError(businessErr, code)` 注册业务 sentinel（`sync.RWMutex` 保护并发）
- `MapToError(err, op)` 便利方法（直接返 `status.Error`）
- `Wrap(err, op)` 包装（自动检测已 wrap 状态避免双重 wrap）
- 字符串前缀扩展：`xtts / sensevoice / fer / llm service` 兜底 ai-svc 模型不可用错误
- `ResetForTest()` 测试辅助

**测试**（10 用例 RED → GREEN）：
- Nil → OK
- 6 类 sentinel → 对应 gRPC code
- 5 类字符串前缀 → 对应 gRPC code
- 业务 sentinel 优先于字符串前缀
- `errors.Is` 优先于字符串前缀
- 包装 gRPC status error 直接返回原 code
- `context.DeadlineExceeded` → `DeadlineExceeded`
- 默认 fallback Internal
- Wrap / 业务 sentinel Wrap

#### 1.4.2 5 svc gRPC server 接入

| svc | 文件 | 改动 |
|---|---|---|
| user-svc | `internal/grpcserver/user_server.go` | init() 注册 4 类 auth sentinel + `mapAuthError` → `grpcerr.MapToError` |
| chat-svc | `internal/grpcserver/chat_server.go` | init() 注册 `repository.ErrNotFound` + `mapLogicError` → `grpcerr.MapToError` + 删字符串前缀模糊匹配 |
| ai-svc | `internal/grpcserver/server.go` | init() 注册 3 类 model unavailable sentinel + `mapAIError` → `grpcerr.MapToError` + 删混合匹配 |
| analytics-svc | `internal/grpcserver/metric_server.go` | init() 注册 `repository.ErrNotFound` + 5 处 `status.Errorf(codes.Internal, "<op>: %v", err)` → `grpcerr.MapToError` |
| assessment-svc | `internal/grpcserver/agent_server.go` | init() 注册 `repository.ErrNotFound` + 5 处 `status.Errorf(codes.Internal, "<op>: %v", err)` → `grpcerr.MapToError` |

**实测 grep 验证**：
- 修复前：`grep -rn "status.Errorf(codes.Internal" emotion-echo-*/internal/grpcserver/` = **19 处**
- 修复后：**0 处** ✅

#### 1.4.3 BFF MapGRPCError 补 409（Sprint G 漏的边界）

`emotion-echo-web-bff/internal/downstream/error.go`：
- 新增 `case codes.AlreadyExists → http.StatusConflict`
- 顶部注释同步
- 新增 `internal/downstream/error_test.go` 13 用例 RED → GREEN（含 wrapped status + 7 类 gRPC code）

**B4 端到端实测**：
| 路径 | 修复前 | 修复后 |
|---|---|---|
| `/api/v1/users/me` | 200 | 200 ✅ |
| `/api/v1/auth/login` 错密码 | 401 | 401 ✅ |
| `/api/v1/conversations/9999/messages` | 404 | 404 ✅ |
| `/api/v1/surveys/9999` | 404 | 404 ✅ |
| `/api/v1/auth/register` 重名 | **502** ❌ | **409** ✅ |

---

## 二、镜像版本（容器化）

| svc | 旧 tag | 新 tag |
|---|---|---|
| user-svc | v0.1.3 | **v0.1.4** |
| chat-svc | v0.1.3 | **v0.1.4** |
| ai-svc | v0.1.1 | **v0.1.2** |
| analytics-svc | v0.1.1 | **v0.1.2** |
| assessment-svc | v0.1.1 | **v0.1.2** |
| web-bff | v0.1.9 | **v0.1.10** |

`docker compose ... up -d` 全部 6 svc 已落地 + healthy。

---

## 三、Commit 时间线

```
f57ba9f feat(bff): B4 BFF MapGRPCError 补 codes.AlreadyExists → HTTP 409
9aff13a feat(svc): B4 5 svc gRPC server 接入 grpcerr 统一映射
edc016e feat(shared): B4 GREEN 错误码统一映射 shared 包实现
8b66d2a docs(adr): B2 决策 22 chat-svc 表依赖清单 + 字段变更契约
b357f31 docs(stage): B3 Stage 65 dashboard 空根因诊断（纯报告）
585082e docs(adr): B1 决策 20 owner sign-off（Accepted）
```

---

## 四、本批未做（仍 open）

| 项 | 来源 | 备注 |
|---|---|---|
| Stage 36 dashboard 空修复（选项 A seed + 选项 B 自动 bot）| todo-pile §C7 | 1 天，下次 sprint |
| chat-svc PinConversation / StreamMessages gRPC | 决策 4 ADR §八 | 1 天，业务未触发 |
| gRPC mTLS（dev insecure → prod mTLS）| 决策 4 ADR §八 | 1 周 |
| BFF 路由三方对齐（C8 todo-pile）| todo-pile §C8 | 1.5~2 天 |
| Helm chart 与 compose dev 全面对齐 | todo-pile §D6 | 1 天 |
| Kafka DevEventPublisher（KAFKA_ENABLED=false 路径）| kafka-reliability-gaps.md §1.1 | 1~2 天（AGENTS.md §2.4 契约 6 红）|

---

## 五、调研依据

| PR | 已读 / 已查 |
|---|---|
| B1 | decisions.md 决策 20 区块 + ADR-20 + README.md Status + 决策 18 #23 |
| B3 | `deploy/docker-compose.infra.yml` / `docker exec postgres \dv` + 4 表 count + analytics-svc report_repository.go:175-205 SQL + BFF /api/v1/reports/daily 端到端 |
| B2 | `deploy/db/02-create-tables-in-schemas.sql:62-86` + `chat-svc/migrations/001/002` + `chat-svc/internal/model/conversation.go` + `chat-svc/internal/events/events.go` + `analytics-svc/internal/repository/report_repository.go:175-205` + todo-pile §D5 |
| B4 | 4 svc gRPC server 文件（chat/user/ai/analytics/assessment）+ BFF error.go + 19 处 status.Errorf grep + 端到端 5 路径 curl 实测 |

---

> 最后更新：2026-09-11 by Stage 66 实施 session