# Stage 33 · P0 修复 + BFF 退化为纯聚合层

> **配套文档**：[`stage-32-landing.md`](./stage-32-landing.md)（Stage 32 APISIX 网关层）、
> [`stage-33-p0-fix-bff-purify.md`](./stage-33-p0-fix-bff-purify.md)（Stage 33 规划）、
> [`architecture-audit-2026-08-31.md`](./architecture-audit-2026-08-31.md)（4 个 P0 问题清单）。
>
> **落地日期**：2026-09-01 · **目标基线**：`docs/stage-31-landing` 分支 HEAD

---

## 一、阶段背景与动机

### 1.1 Stage 31 + Stage 32 → Stage 33 的衔接

| 阶段 | 落地内容 | 留下的 P0 |
|------|----------|-----------|
| Stage 31（Nacos 12 PR） | 7 svc 注册 + 配置中心 + Helm/compose nacos | 治理能力有事实基线，但网关层与下游鉴权仍有缺口 |
| Stage 32（APISIX 4 PR） | 网关层回归 + jwt-auth 真验签 + X-User-Id 透传 | S-1 P0（JWT 不验签）物理层修了，BFF 侧仍签发 mock JWT |

Stage 33 任务：修复 A-1（聊天链路不可用）、S-1 BFF 侧（mock login）、S-2（端口全暴露），完成决策 12 "BFF 纯聚合层"。

### 1.2 P0 问题清单（修复结果）

| P0 | 问题 | 修复 PR | 状态 |
|----|------|---------|------|
| **A-1** | 消息不落库 + SSE 协议不匹配 | PR-17 + PR-18 | ✅ 修复 |
| **S-1** | JWT 不验签（BFF 侧 mock JWT） | PR-19a + PR-19b | ✅ 修复 |
| **S-2** | 端口全暴露 + 基础设施零防护 | PR-20 | ✅ 修复 |

### 1.3 ADR 决策 11/12/13 落地

- 决策 11（Nacos 撤回 + 重新引入）已在 Stage 31 落地
- 决策 12（BFF 纯聚合层）通过 PR-19b + PR-21 完全落地
- 决策 13（client_msg_id 幂等）通过 PR-18 落地

---

## 二、目标 vs 实际（7 个 PR 落地清单）

| # | PR | 主题 | TDD 阶段 | commit | 行数 |
|---|----|------|----------|--------|------|
| 1 | PR-17 | SSE 协议对齐：前端 useAIStreamHandler 改 OpenAI 兼容解析 | 🔴 8 RED → 🟢 GREEN → ♻️ REFACTOR | `27a3d2f` | +284/-100 |
| 2 | PR-18 | 聊天写库前移：写库前移到 stream 调用前 + client_msg_id UNIQUE 约束 | 🔴 4 RED → 🟢 GREEN → ♻️ FRONTEND | `e12e8f3` | +312/-26 |
| 3 | PR-19a | user-svc login/register + shared/pkg/password 包抽离 | 🔴 18 RED（8 password + 10 authlogic）→ 🟢 GREEN | `efeb0cf` | +637/-3 |
| 4 | PR-19b | BFF 重写 auth_handler + 5次锁定 + 验证码限流 + APISIX 白名单 | 🔴 13 RED → 🟢 GREEN + frontend + APISIX | `51e5956` | +514/-110 |
| 5 | PR-20 | 端口收紧（11 业务 svc + 4 中间件 + 1 AI 子 svc） | — | `01ab327` | +56/-35 |
| 6 | PR-21 | BFF 收口（mock 清理 + 注释改写 + 决策 12 文档化） | — | `d6104ef` | +31/-24 |
| 7 | PR-22 | `scripts/smoke_stage33.sh` 端到端冒烟 | — | `a520af9` | +273/0 |

**合计**：7 commits / +2,107 / -298 行（按 `git diff --shortstat 7cbcca6..HEAD` 实测 = 37 files / +2,101 / -292）。

---

## 三、关键设计决策

### 3.1 PR-19 拆分为 19a + 19b（与原文档偏差）

`stage-33-p0-fix-bff-purify.md` 原计划 1 个 PR-19 涵盖 user-svc login + BFF 重写。**实施前调研发现 user-svc 完全没有 login/register 端点**（架构审计未覆盖的盲点），将 PR-19 拆分为：

- **PR-19a**：user-svc 加 login/register 端点 + shared/pkg/password 抽离（bcrypt 包）+ 11 个 authlogic 测试。**纯 Go 改动，可独立 review**。
- **PR-19b**：BFF 重写 auth_handler.go（注入 UserClient + 限流）+ 前端去 sha256 + APISIX seed.sh 加 5 条 auth 白名单。**跨 3 个子模块**（bff/web/apisix），必须整体验收。

**理由**：user-svc 是其他 svc 共享的依赖，必须先就位 BFF 才能调；19a 单独 merge 后 19b 可在干净的 user-svc API 上做对接。

### 3.2 前端去 sha256（与原文档偏差）

原文档 `pages/login/index.vue:146` 把密码先 sha256 后传给 BFF，user-svc bcrypt(sha256(明文))。改为：

- 前端直接传明文密码 → BFF → user-svc bcrypt(明文) 入库

**理由**：`bcrypt(sha256(明文))` 等价于 `bcrypt(明文)` 但削弱了 bcrypt 安全性（攻击者只需对 sha256 字典攻击）。dev compose 默认不走 TLS（明文仅本地开发）；prod 必须启用 `nginx:alpine profile: tls`。

### 3.3 限流策略（in-memory 单实例假设）

PR-19b 在 BFF 内实现：
- **登录失败锁定**：连续 5 次错误密码 → 锁定 5 分钟（`map[username]*loginAttempt` + `sync.RWMutex`）
- **验证码限流**：同 username 60s 内只能发一次（`map[username]*verificationEntry`）
- **多实例假设**：文档化"单实例部署假设"，多实例 BFF 留 Stage 34+ Redis 迁移（O-3 P2）

---

## 四、文件级变更总览

### 4.1 PR-17（SSE 协议）

```
Emotion-Echo-Web/app/composables/useAIStreamHandler.ts          | 重写 OpenAI 兼容 SSE 解析
Emotion-Echo-Web/app/composables/useAIStreamHandler.test.ts     | 新增 8 个测试
Emotion-Echo-Web/app/composables/useConversationSender.ts       | 适配新回调（删 onStart/onTruncated）
```

### 4.2 PR-18（写库前移）

```
emotion-echo-chat-svc/migrations/002_add_client_msg_id.sql      | 新增 partial UNIQUE INDEX
emotion-echo-chat-svc/internal/model/conversation.go            | Message.ClientMsgID *string
emotion-echo-chat-svc/internal/types/types.go                  | SendMessageReq.ClientMsgID
emotion-echo-chat-svc/internal/repository/conversation_repository.go | +GetMessageByClientMsgID interface + 两实现
emotion-echo-chat-svc/internal/logic/sendmessagelogic.go        | 幂等查重 + ContentType 透传
emotion-echo-chat-svc/internal/logic/sendmessagelogic_test.go   | +3 测试（Duplicate/Different/Empty/DifferentUser）
emotion-echo-chat-svc/internal/logic/deleteconversationlogic_test.go | failingDeleteRepo 适配新接口
emotion-echo-web-bff/internal/downstream/chat.go                 | SendMessageReq.ClientMsgID 透传
Emotion-Echo-Web/app/types/api.ts                               | SendMessageParams + AIStreamParams 加 clientMsgId
Emotion-Echo-Web/app/stores/message.ts                          | sendMessage 接收 clientMsgId 参数
Emotion-Echo-Web/app/composables/useConversationSender.ts       | 流程改造：先落库再 stream
```

### 4.3 PR-19a（user-svc login）

```
emotion-echo-shared/go.mod                                       | golang.org/x/crypto 从 indirect 提升到 direct
emotion-echo-shared/pkg/password/password.go                   | 新建（Hash/Verify）
emotion-echo-shared/pkg/password/password_test.go              | 新建 8 个测试
emotion-echo-user-svc/internal/types/types.go                   | LoginReq/RegisterReq/AuthErrorResp
emotion-echo-user-svc/internal/repository/user_repository.go   | +UsernameExists interface + 两实现
emotion-echo-user-svc/internal/logic/authlogic.go               | 新建（AuthLogic.Login/Register）
emotion-echo-user-svc/internal/logic/authlogic_test.go          | 新建 10 个测试
emotion-echo-user-svc/internal/handler/auth_handler.go         | 新建（LoginHandler/RegisterHandler）
emotion-echo-user-svc/main.go                                    | 注册无 auth 中间件的 /login /register 路由
```

### 4.4 PR-19b（BFF 重写）

```
emotion-echo-web-bff/internal/downstream/user.go                | +Login/Register 方法
emotion-echo-web-bff/internal/handler/auth_handler.go           | 完全重写（注入 UserClient + 限流）
emotion-echo-web-bff/internal/handler/auth_handler_test.go      | 重写（13 个测试）
emotion-echo-web-bff/internal/handler/user_handler_test.go       | fakeUserClient 适配新接口
emotion-echo-web-bff/main.go                                     | NewAuthHandler 注入 UserClient
Emotion-Echo-Web/app/pages/login/index.vue                       | 3 处去 sha256
deploy/apisix/seed.sh                                            | +5 条 auth 白名单路由（无 jwt-auth）
```

### 4.5 PR-20（端口收紧）

```
deploy/docker-compose.apps.yml                                  | 9 个 svc 改 expose（保留 web:3000 + web-bff:8894 + ai-svc grpc:8892）
deploy/docker-compose.infra.yml                                 | postgres/redis/kafka/skywalking/nacos gRPC/apisix admin 全改 expose
```

### 4.6 PR-21（BFF 收口）

```
emotion-echo-shared/pkg/middleware/jwt_auth.go                  | 注释改为决策 12 明确信任边界
emotion-echo-shared/pkg/grpcinterceptor/auth.go                 | 删 "Future (TODO)" 前缀，改为 Roadmap 注释
emotion-echo-shared/pkg/grpcinterceptor/server.go               | ServerAuth 从 TODO 移除（已实现）
emotion-echo-web-bff/etc/web-bff.yaml                            | JWTSecret 注释改为"必须与 APISIX jwt-auth secret 同源"
emotion-echo-web-bff/internal/config/config.go                  | Auth struct 注释同步
emotion-echo-web-bff/internal/handler/auth_handler.go           | 顶部注释从 Stage 30 mock 改为 Stage 33 PR-19b/21 真实登录
emotion-echo-web-bff/main.go                                     | authPathBypass 注释说明白名单仅剩 5 条 auth 端点
```

### 4.7 PR-22（冒烟脚本）

```
scripts/smoke_stage33.sh                                         | 新建（9 步端到端冒烟，dry-run 模式可独立验证脚本本身）
```

---

## 五、测试覆盖统计

### 5.1 新增/修改测试

| 文件 | 测试数 | 说明 |
|------|--------|------|
| `shared/pkg/password/password_test.go`（PR-19a） | 8 | Hash/Verify 边界 + 不同盐 + 互操作 |
| `user-svc/internal/logic/authlogic_test.go`（PR-19a） | 10 | Login（5）+ Register（5）覆盖 happy path + 边界 |
| `chat-svc/internal/logic/sendmessagelogic_test.go`（PR-18） | +3 | Duplicate/Different/Empty client_msg_id |
| `bff/internal/handler/auth_handler_test.go`（PR-19b） | 13 | 5次锁定 + 锁定窗口内 + refresh mock + 多种 status code |
| `web/composables/useAIStreamHandler.test.ts`（PR-17） | 8 | OpenAI 格式解析 + Abort + HTTP 错误 + 跨字节 chunk |

### 5.2 全量回归

| 模块 | 测试结果 |
|------|----------|
| emotion-echo-shared | ✅ 7 包全绿 |
| emotion-echo-user-svc | ✅ 7 包全绿 |
| emotion-echo-chat-svc | ✅ 9 包全绿 |
| emotion-echo-assessment-svc | ✅ 全绿 |
| emotion-echo-analytics-svc | ✅ 全绿 |
| emotion-echo-ai-svc | ✅ 全绿 |
| emotion-echo-web-bff | ✅ 9 包全绿 |
| Emotion-Echo-Web（前端） | ✅ 20 文件 / 232 测试全绿 |

### 5.3 端到端冒烟

`scripts/smoke_stage33.sh`（PR-22）9 步端到端冒烟，dry-run 模式已验证脚本本身；完整运行需要 docker compose up + APISIX seed + jq + 可达 postgres。

---

## 六、收口条件核对

| 收口项 | 状态 | 说明 |
|--------|------|------|
| A-1 SSE 协议对齐 | ✅ | PR-17 + 8 个前端测试 |
| A-1 消息落库 | ✅ | PR-18 + chat-svc partial UNIQUE INDEX |
| S-1 BFF 真实登录 | ✅ | PR-19a + PR-19b + 23 个新测试 |
| S-2 端口收紧 | ✅ | PR-20（仅 6 端口对外） |
| 决策 12 BFF 纯聚合层 | ✅ | PR-19b + PR-21 |
| 决策 13 client_msg_id 幂等 | ✅ | PR-18 partial UNIQUE INDEX |
| 端到端冒烟脚本 | ✅ | PR-22 `smoke_stage33.sh` |
| AGENTS.md TDD 节奏 | ✅ | 所有 Go/前端新代码遵循 🔴 RED → 🟢 GREEN → ♻️ Refactor |
| 不引入 K8s / Kafka DLQ / 数据库迁移工具 | ✅ | 严格按计划 |

---

## 七、与 Stage 32 / Stage 34+ 的衔接

### 7.1 Stage 32 已就位（未改动）

- APISIX 网关层 + X-User-Id 透传 + 6 上游 + 7 路由
- 决策 11（Nacos 撤回 + 重新引入）
- 物理层 S-1（APISIX jwt-auth 真验签）已修复

### 7.2 Stage 33 衔接（已落地）

- 修复 A-1 / S-1 BFF 侧 / S-2 三个 P0
- BFF 退化为纯聚合层（决策 12）
- 完整 9 步端到端冒烟脚本

### 7.3 Stage 34+ 演进（待启动，按文档出现频率排序）

1. **APISIX upstream 切 nacos-discovery**（Stage 32 文档 §三.3，Stage 33 维持静态 upstream）
2. **Kafka DLQ / Outbox 封顶 / 端到端幂等**（K-1 / I-1 P1；PR-18 仅做客户端幂等，服务端 DLQ 留 Stage 34+）
3. **数据库迁移工具**（D-1 P1；PR-18 002_add_client_msg_id.sql 仍手工 SQL）
4. **CI/CD 落地**（E-1 P1）
5. **Nacos 3 节点集群 + PVC + MySQL 后端**（prod HA）
6. **etcd 3 节点集群**（prod HA）
7. **多实例 BFF 限流迁移 Redis**（O-3 P2；当前 PR-19b in-memory 单实例假设）
8. **ADR 同步机制固化**（E-2 P1）
9. **前端核心链路测试补全**（T-1 P1；useAIStreamHandler 已补，useConversationSender 已新建测试）

---

## 八、显式不做（留给后续 Stage）

- ❌ 数据库迁移工具引入（仅 PR-18 加 1 个手工 SQL；Stage 34+ 用 golang-migrate）
- ❌ Kafka DLQ / Outbox 封顶（Stage 34+）
- ❌ 多实例 BFF 限流共享（Stage 34+ Redis）
- ❌ CI/CD（Stage 34+）
- ❌ 真实 refresh token 轮换（Stage 34+）
- ❌ 推送 origin / 合并 main / 处理 main 与 docs/stage-31-landing 之间的 133 commits 历史债

---

## 九、commits 落地清单（git 追溯）

```
a520af9 feat(stage-33-pr22): scripts/smoke_stage33.sh 端到端冒烟
d6104ef feat(stage-33-pr21): BFF 收口 + 决策 12 注释清理
01ab327 feat(stage-33-pr20): 端口收紧 + 6 端口对外
51e5956 feat(stage-33-pr19b): BFF 真实登录 + 限流 + APISIX 白名单
efeb0cf feat(stage-33-pr19a): user-svc login/register + shared/pkg/password
e12e8f3 feat(stage-33-pr18): 聊天写库前移 + client_msg_id UNIQUE 约束
27a3d2f feat(stage-33-pr17): SSE 协议对齐 + OpenAI 兼容解析
```

**合计**：7 commits / 37 files / +2,101 / -292 行（按 `git diff --shortstat 7cbcca6..HEAD` 实测）。

完整 main..HEAD 差异（含 Stage 31/32/33 收口）：**33 commits / 145 files / +12,771 / -1,253 行**。

---

## 十、收尾总结

**Stage 33 完成度**：**7/7 PR 全部落地，4 个 P0 修复 3 个（A-1 / S-1 BFF 侧 / S-2）**，决策 12（BFF 纯聚合层）完全收口。

**核心交付物**：

1. ✅ SSE OpenAI 兼容解析（前端 8 个新测试）
2. ✅ 聊天写库前移 + client_msg_id partial UNIQUE INDEX
3. ✅ user-svc 真实 login/register（bcrypt 包抽离）
4. ✅ BFF 重写 auth_handler + 5 次锁定 + 验证码限流 + APISIX 白名单
5. ✅ 端口收紧（仅 6 端口对外）
6. ✅ BFF 收口（决策 12 注释化）
7. ✅ 端到端冒烟脚本（9 步）

**测试**：Go 7 模块 + 前端 232 个测试全绿。

**架构演进意义**：

> Stage 33 是 Stage 31（Nacos）+ Stage 32（APISIX）+ Stage 33 三阶段的**功能性闭环**。
> 治理能力"服务发现 + 配置中心 + 网关 + 真实鉴权"完整闭环。
> 主聊天功能（A-1）从永远"streaming"恢复为完整可用的 OpenAI 兼容流式对话。
> 4 个 P0 修复 3 个，剩余 P0：仅 K-1/I-1（Kafka 可靠性与端到端幂等服务端部分）—— Stage 34+ 范围。

---

**下一步行动**：

1. 用户 review Stage 33 7 个 PR（已在 docs/stage-31-landing 上 squash merge）
2. 启动 Stage 34（候选：APISIX upstream 切 nacos-discovery / Kafka DLQ / 数据库迁移 / CI/CD）
3. 推送 origin（独立决策，处理 main 与 docs/stage-31-landing 之间的 133 commits 历史债）
