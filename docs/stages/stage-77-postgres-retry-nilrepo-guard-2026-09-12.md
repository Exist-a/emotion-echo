# Stage 77 — Postgres nil repo 无自愈修复（启动重试退避 + gRPC Unavailable 守卫）

> 日期：2026-09-12
> 来源：Stage 76 收口报告 §四 open 首项 → 用户指示"先做最推荐这个"
> 性质：TDD 修复批次（RED `c0ea53e` → GREEN `7632eee`），2 commit

## 一、问题（stage-76 §二.3 实测）

Stage 76 验收期间，Docker DNS 瞬时抖动使 5 个业务 svc 恰好在重启窗口 `openPostgres`
失败（`lookup emotion-echo-postgres: no such host`）。按"dev 不阻断"策略，svc 带着
**nil repo** 继续启动且容器 healthy——之后每个触 repo 的 RPC 都在 logic 层 panic：

- user-svc Login：`authlogic.go:62` nil pointer dereference（BFF 兜底 502，
  用户视角 = "登录永远失败但没有任何告警"）

根因不是代码 bug，而是两个既有设计的叠加缺口：
1. 启动连接失败**一次即放弃**（无重试）——秒级 DNS 抖动被固化为整轮生命周期降级；
2. 降级后 gRPC 端点**无 nil-repo 守卫**——svcCtx != nil 但 repo == nil 直接穿透到 logic 层 panic。

## 二、修复内容

### shared（新增 `pkg/dbconnect`）

- `ConnectWithRetry[T]`：泛型有限次退避重试，成功即返回；每次失败后 sleep 一个
  backoff（最后一次除外）；`attempts < 1` 归一化为 1；sleep 函数注入（测试收集器）
- `DefaultAttempts=10 / DefaultBackoff=500ms`——≈5s 覆盖窗口，盖过 Docker 网络
  重建/瞬时 DNS 抖动，又不拖垮"DB 真挂了"的启动
- 4 个单测锁定契约（首试成功不 sleep / 退避重试 / 耗尽返最后错误 / 归一化）

### 5 svc main.go：openPostgres 接入 ConnectWithRetry

- user/chat/analytics/assessment/ai 全部接线；"dev 不阻断"语义保留，降级日志带
  `after N attempts`；chat/ai 因 openPostgres 返回双值引入 `pgConn`/`aiPgConn`
  聚合 struct 适配泛型

### 5 svc gRPC server：nil-repo → codes.Unavailable

| svc | 守卫 | 说明 |
|---|---|---|
| user | `ensureRepo()` 统一守卫 7 RPC | svcCtx nil 与 UserRepo nil 分开报错 |
| chat | `ConversationRepo == nil`（8 处，含 StreamMessages） | OutboxRepo 是合法可选 nil（"nil = 不写 outbox"），**不**纳入守卫 |
| analytics | `EventRepo == nil`（9 处） | EventRepo 是 DB 成功时必然注入的 canonical repo，无误伤 |
| assessment | `SurveyRepo == nil`（5 处） | |
| ai | `emotionQueryServer.repo == nil`（3 处触点） | 与既有 `fusedEmotionRepo nil → Unimplemented` 先例同风格，降级用 Unavailable |

## 三、验收

- RED（`c0ea53e`）：6 个测试文件全红——user/assessment/ai **panic**，
  chat/analytics 返 Internal 而非 Unavailable
- GREEN（`7632eee`）：6 模块 `go vet` 0 err + `go test ./...` 全绿
- dev 栈 e2e：`build_dev_images.sh` 8/8 镜像重建 → 5 svc 滚动重启
  → 5 svc 日志 `[postgres] connected` → BFF 重启后经 APISIX 登录 **200** →
  `smoke_data_layer.py` **11/11 PASS** + §契约 7 `smoke_nacos_registry.sh` **6/6**
- 正常路径回归：重试包在连接成功时零开销（首次成功不 sleep，单测锁定）

## 四、本批未做（open）

| 项 | 说明 |
|---|---|
| Kafka P3 | outbox relay dead 告警接 alertmanager（kafka-reliability-gaps.md §3.6，小） |
| Kafka §1.4 可选 | consumer 进程级指标（小） |
| 业务功能排期 | ai-response-structured / intent-classification-6-types / file-upload-message-extension |
| Nacos 深水区（可选） | SDK 升级 / Subscribe 动态感知（本次滚动重启再次实证 boot 期解析需重启 BFF 生效——与 stage-75 §二结论一致）/ llm-service 注册 |
| DB 纳入 fail-fast required 依赖（可选强化） | 本批选了"重试 + Unavailable"路线；若未来 dev 要求 DB 强制，走 PR-5 `IsRequired("postgres")` 模式即可 |

## 五、调研依据

- 已读：emotion-echo-{user,chat,analytics,assessment,ai}-svc/main.go（openPostgres 段）、
  5 svc internal/grpcserver/{user_server,chat_server,metric_server,agent_server,server}.go
  （守卫与 repo 触点）、5 svc internal/svc/servicecontext.go（repo 字段与可选性）、
  emotion-echo-user-svc/internal/{logic/authlogic.go,repository/user_repository.go}、
  emotion-echo-shared/pkg/dbconnect（新）、deploy/apisix/test_seed_nacos.sh（Stage 76 上下文）
- 已查：stage-76 §二.3/§四、stage-39 §2.2（PR-5 fail-fast 先例）、
  chat ServiceContext "OutboxRepo nil = 不写 outbox" 注释（决定不纳入守卫）
- 命令证据：RED 6 文件全红（panic ×3 + Internal ×2 + build 红 ×1）→ GREEN 全绿；
  6 模块 go vet 0 err；dev 栈 8/8 镜像 + 5 svc 重启 + 登录 200 + smoke 11/11 + §契约 7 6/6
- 关联：stage-76（问题发现）、stage-39 PR-5（fail-fast 先例）、AGENTS.md §3.1（依赖反转）

---

> 最后更新：2026-09-12 by Stage 77 实施 session
> 关联：stage-76 §二.3、shared/pkg/dbconnect、5 svc grpcserver 守卫
