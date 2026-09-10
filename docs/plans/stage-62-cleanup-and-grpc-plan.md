---
status: planned
priority: high
owner: TBD
created: 2026-09-10
related-stages:
  - stage-38-A-dev-apisix-path.md（OAuth 路径主动弃）
  - stage-33-landing.md §七（PR-19a username+password 落地）
  - stage-61-doc-drift-closure.md（本批前批收口）
related-decisions:
  - adr-2026-09-doc-drift-registry.md（决策 18 失真台账）
  - decisions.md 决策 4（跨服务调用 = gRPC）
  - decisions.md 决策 12（BFF = 纯聚合层）
related-plans:
  - todo-pile-2026-09-04.md §D（杂项批处理）
  - grpc-inter-service-migration.md §一
---

# Plan — Stage 62 · 合规优先最小集 + gRPC 内部调用收口

> 本计划 = **合规优先最小集**（5 项：登录限流测试 / 全景图同步 / BFF→3 svc gRPC 化 /
> `--profile ai` 启动段同步 / OAuth DDL 残留清理）+ Stage 60 后发现的
> "gRPC 改造差 4 条 HTTP" 集中收口。
>
> 按 AGENTS.md 第一性原则，全部 RED→GREEN→REFACTOR，每个 PR 配 TDD 测试 +
> commit msg 调研依据。

---

## 一、目标

把"最重要的几个 bug / 决策栈收口"作为 1 个独立 sprint 推进，按依赖顺序 5 个 PR：

```
PR-1  Bug: BFF 登录限流测试           (半天，安全相关，最高优先)
PR-2  Doc: 架构全景图同步决策 11/12    (15 分钟，文档失真)
PR-3  Arch: BFF→user/assessment/analytics gRPC 化  (1~1.5 天，架构收口)
PR-4  Doc: README + QUICKSTART "5 秒必读：AI profile"  (30 分钟，新人 onboarding)
PR-5  Cleanup: OAuth DDL 残留清理      (1 小时，数据库债务)
```

合计 **2.5~3 天** 可全部 landed。每项可独立 merge。

---

## 二、现状调研（写计划前必做——AGENTS.md §〇）

### 2.1 PR-1 · BFF `isLocked`/`recordFailure` 实现 + 缺测

**已读代码**：

| 文件 | 关键发现 |
|---|---|
| `emotion-echo-web-bff/internal/handler/auth_handler.go:53-58` | 限流常量：`loginLockWindow = 5 * time.Minute` / `loginMaxFailures = 5` |
| `auth_handler.go:62-71` `AuthHandler struct` | 内嵌 `loginFailures map[string]*loginAttempt` + `loginMu sync.RWMutex` |
| `auth_handler.go:93` 初始化 | `loginFailures: make(map[string]*loginAttempt)` |
| `auth_handler.go:127-153` login 端点 | 调用 `isLocked` → `recordFailure` → `clearFailures` |
| `auth_handler.go:269-302` 三个 helper | `isLocked` / `recordFailure` / `clearFailures` 实现齐全 |
| 注释 `auth_handler.go:66` | "in-memory 限流（单实例假设；多实例 BFF 留 Stage 34+ Redis 迁移）" |

**单测覆盖**：

```
$ grep -rn "loginMaxFailures\|loginLockWindow\|isLocked\|recordFailure" \
    emotion-echo-web-bff/internal/handler/*_test.go
# （零命中）
```

**结论**：
- 业务逻辑完整 + sync.RWMutex 保护 + 5min 窗口 + 5次阈值
- **零单测覆盖**——决策 18 §三 类型 5 自报告风险（作者踩过的"无测试 = 失效"教训）
- 多实例 BFF 下失效（但本项目单机 dev 场景 OK）

**修复方案**：

```go
// 新文件：emotion-echo-web-bff/internal/handler/auth_handler_test.go
// （如不存在；或追加 test 函数）

func TestLogin_LockAfter5Failures(t *testing.T) // 6次失败 → 第6次开始 423 Locked
func TestLogin_LockWindowExpires(t *testing.T)  // mock clock，5min 后解锁
func TestLogin_ClearOnSuccess(t *testing.T)     // 5次失败后正确登录 → clearFailures 调用
func TestLogin_LockReturns423(t *testing.T)     // 锁定中即使正确密码也 423
func TestLogin_ConcurrentFailures(t *testing.T) // 10 并发 goroutine 失败 → 仍只锁 1 次
func TestLogin_RaceCondition(t *testing.T)      // sync.RWMutex 安全性
```

依赖 mock：业务需要 `h.user.Login` mock（已有 fakeUserClient 模式参考 `auth_handler_test.go` 中已有）。

### 2.2 PR-2 · decisions.md 架构全景图失真复核

**已读代码**（`docs/architecture/decisions.md §二 🏗 当前架构全景` line 403-454）：

| 行号 | 措辞 | 评估 |
|---|---|---|
| 410 | `← 唯一业务入口（决策 11）` | ✅ 合规——决策 11 明确 APISIX 是唯一业务入口 |
| 416 | `← 聚合层（决策 9 / 12；APISIX upstream）` | ✅ 合规——决策 12 明确 BFF 是纯聚合层 |
| 418 | `(dev :8894 监听宿主 — 仅调试例外)` | ✅ 合规——决策 12 dev 例外段 |
| 462 | `web-bff` 行含 "dev 监听宿主 :8894（仅调试例外，apps.yml:602-604）" | ✅ 合规 |

**结论**：全景图**无实际失真**——上次会话决策 18 #24 登记时基于"作者推断"，实测全景图本身就是合规的。

**修复方案**：

唯一的"修复"是把全景图的"决策 9 vs 11/12"关系说明显式化——加一行注释说明"全景图已对齐决策 12 关系说明段"，让后来者不要再走"作者推断为唯一入口"的失真路径。

预计 15 分钟，不开 PR，作为本批 doc cleanup 一并 commit。

### 2.3 PR-3 · BFF → user/assessment/analytics-svc HTTP→gRPC

**已读代码**：

| 维度 | user-svc | assessment-svc | analytics-svc |
|---|---|---|---|
| **BFF 调用端点** | 3 个 | 5 个 | 9 个 |
| **proto 文件** | ❌ 无 | ❌ 无 | ❌ 无 |
| **gRPC 客户端** | ❌ 无（`internal/downstream/user.go` HTTP）| ❌ 无 | ❌ 无 |
| **feature flag** | — | — | — |

**BFF 调用清单**（实测）：

```
emotion-echo-web-bff/internal/downstream/user.go
  GET    /api/v1/users/me
  PATCH  /api/v1/users/me
  GET    /api/v1/users/:id

emotion-echo-web-bff/internal/downstream/assessment.go
  GET    /api/v1/surveys
  GET    /api/v1/surveys/:id
  POST   /api/v1/surveys/:id/submit
  GET    /api/v1/surveys/results
  GET    /api/v1/surveys/results/:resultId

emotion-echo-web-bff/internal/downstream/analytics.go
  GET    /api/v1/reports/daily
  GET    /api/v1/reports/trend
  GET    /api/v1/user-behavior/day-night
  GET    /api/v1/user-behavior/depth
  GET    /api/v1/user-behavior/frequency
  GET    /api/v1/mental-health/assessment
  GET    /api/v1/mental-health/history
  POST   /api/v1/mental-health/trigger
  GET    /api/v1/mental-health/trend
```

合计 **17 个 RPC** + 3 个 proto + 3 个 server实现 + 3 个 client实现。

**基础设施**：

- ✅ proto 生成：`proto/gen.sh` + `shared/pkg/emotionchat` / `emotionquery` / `emotionllm` 已就位
- ✅ 拦截器：`shared/pkg/grpcinterceptor`（auth / client / retry / server / stream / tracing / userid / logging / recovery）全部有单测
- ✅ BFF gRPC 客户端范本：`internal/downstream/chat_grpc.go`（PR-GRPC-4）+ `emotion_query.go`
- ✅ chat-svc gRPC server 范本：`internal/grpcserver/{server.go,chat_server.go}`
- ✅ feature flag 模式：chat-svc 用 `CHAT_TRANSPORT=grpc\|http`（默认 grpc）

**修复方案**（参考 Stage 58 PR-GRPC-1~6 范式）：

```bash
# PR-3.1：proto + stub 生成（半天）
proto/user.proto       # 3 个 RPC
proto/assessment.proto # 5 个 RPC
proto/analytics.proto  # 9 个 RPC
proto/gen.sh           # 生成到 shared/pkg/{user,assessment,analytics}

# PR-3.2：3 个 svc 加 gRPC server（半天，参照 chat-svc grpcserver/）
emotion-echo-user-svc/internal/grpcserver/{server.go,user_server.go,user_server_test.go}
emotion-echo-assessment-svc/internal/grpcserver/{...}
emotion-echo-analytics-svc/internal/grpcserver/{...}
# main.go 双轨启动：HTTP :8888 + gRPC :8887（避 APISIX 上游端口冲突）
# 各 svc etc/{svc}-api.yaml 加 GRPCServer.Port 字段

# PR-3.3：BFF 改 client + feature flag（半天）
web-bff/internal/downstream/{user_grpc,assessment_grpc,analytics_grpc}.go
# ChatClientOptions 加 {Transport + GRPCConn}
# 走 chat-svc PR-GRPC-4 同款：interface 不变 + 新增 gRPC 实现 + flag 切换
```

**风险与缓解**：

| 风险 | 缓解 |
|---|---|
| 端口冲突（3 svc 各加 gRPC port）| user-svc :8887 / assessment-svc :8886 / analytics-svc :8885；compose expose + dev port mapping |
| proto 命名空间冲突 | 用 `package emotion_user.v1` / `emotion_assessment.v1` / `emotion_analytics.v1` |
| 拦截器与现有 HTTP 鉴权一致性 | 复用 `shared/pkg/grpcinterceptor.UserIDFromMetadata`（BFF 注入 `x-user-id`）|
| 端到端 smoke | 复用 Stage 58 PR-GRPC-6 模式，新增 `scripts/smoke_bff_3svc_grpc.sh` |

### 2.4 PR-4 · README/QUICKSTART `--profile ai` 启动段

**已读代码**：

```
README.md:108-121
  ### 方式一：Docker Compose（推荐）
  # Stage 30：APISIX 退役...（注释过时）
  cd deploy
  docker compose -f docker-compose.infra.yml up -d
  docker compose -f docker-compose.apps.yml up -d
  # 3. （可选）启动 AI profile
  docker compose --profile ai up -d --build  ← 措辞太简短
```

```
QUICKSTART.md:68-78
  ## ⚠️ 5 秒必读 · 前端是独立进程
  （已有前端说明，但**没有 AI profile** 5 秒必读）
```

**结论**：README/QUICKSTART 都缺 AI profile 启动段。新人 clone 后会发现 TTS 不可用 / 多模态无响应。

**修复方案**：

```markdown
## ⚠️ 5 秒必读 · AI 镜像是可选 profile（决策 17 / Stage 60）

`docker compose -f docker-compose.apps.yml up -d` 默认**只起 7 业务 + 8 infra 容器**——
TTS / 人脸识别 / 语音识别需显式启用 AI profile：

```bash
# 镜像已在本地仓库 / 阿里云 ACR（推荐）
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
    --profile ai up -d emotion-echo-xtts emotion-echo-fer emotion-echo-sensevoice

# 首次拉镜像 + build（耗时长 + 受国内镜像影响，见 stage-60 §五）
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
    --profile ai up -d --build emotion-echo-xtts emotion-echo-fer emotion-echo-sensevoice
```

**为什么**：`emotion-echo-models/{XTTS,FER-tflite,SV-fastbuild}/` 三容器总大小约 18GB，
dev 默认不起避免拖慢启动；prod 部署时按需启用。
```

预计 30 分钟，README + QUICKSTART 各加一段。

### 2.5 PR-5 · OAuth DDL 残留清理（用户确认 OAuth 已删）

**已读代码**（决策 18 #25 + 用户口头确认）：

| 位置 | 类型 | 内容 |
|---|---|---|
| `deploy/db/01-create-schemas.sql:48` | DDL | `CREATE TABLE IF NOT EXISTS user_oauth (id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, provider VARCHAR(32) NOT NULL, ...)` |
| `deploy/db/02-create-tables-in-schemas.sql:34` | DDL | `CREATE TABLE IF NOT EXISTS emotion_echo_user.user_oauth (...)` |
| `emotion-echo-web/.env.example:60-63` | 配置 | `# WECHAT_APP_ID=...` / `# WECHAT_REDIRECT_URI=...` / `# WECHAT_APP_ID=your_qq_id_here` / `# QQ_REDIRECT_URI=...`（已注释但保留）|
| `legacy/emotion-echo-gin/internal/handler/oauth_handler.go` | 代码 | legacy Gin 已归档，不动 |
| `legacy/emotion-echo-gin/docs/DESIGN.md` | 文档 | legacy 设计文档 |
| `docs/plans/wechat-qq-login-and-upload.md` | plan | `status: superseded by Stage 38-A`（合规） |

**关联决策依据**：

- `docs/plans/wechat-qq-login-and-upload.md:9-13`：
  > Stage 38-A 用户决策：改用 username + password 登录，去掉微信/QQ OAuth 路径。
  > 勘察时确认：roadmap §PR-B1 写"微信 OAuth 已完成" 与代码不符，代码上 OAuth 从未实施。
- `legacy/emotion-echo-gin/internal/handler/oauth_handler.go` 4f2b62d `flatten submodules` 时**带入**，Stage 33 PR-19a 切到 username+password 时**未清 DDL**

**结论**：

- `user_oauth` 表 **零引用**（已实测 grep `user_oauth\|provider.*openid` → 仅 DDL 命中）
- `user-svc/internal/model/user.go` 无 OAuth 字段（已实测 grep → 0 命中）
- 决策 18 §三 类型 5 自报告风险 = "OAuth 已删但 DDL/注释 残留"导致**新人 onboarding 时困惑**（model 无字段但表在）

**修复方案**（用户口头确认：删掉表）：

```sql
-- 新 migration：deploy/db/migration/NNN-drop-user-oauth.sql
DROP TABLE IF EXISTS emotion_echo_user.user_oauth;
-- (同时清理 01-create-schemas.sql:48 的重复声明)
```

```bash
# emotion-echo-web/.env.example:60-63 删 4 行 WECHAT/QQ 注释
# （如果未来要重新加 OAuth，留空行 + 简短 # TODO 即可，不留具体模板）
```

预计 1 小时：
- 写 migration 文件（5 分钟）
- 跑 smoke 验证 DB 健康（10 分钟）
- 改 .env.example（5 分钟）
- 加 ADR：`adr-2026-09-drop-oauth-ddl.md`（30 分钟）记录"为什么删 + 何时能恢复"
- 注册到 `decisions.md` 决策 21（10 分钟）

---

## 三、批次划分 + 提交顺序

| 顺序 | PR | 范围 | 工作量 | 阻塞关系 |
|---|---|---|---|---|
| 1 | **PR-1** | BFF 登录限流 6 测试 | 半天 | 无；最先做（最简单） |
| 2 | **PR-2** | decisions.md 全景图注释微调 | 15 分钟 | 无；可与 PR-5 合并 commit |
| 3 | **PR-4** | README/QUICKSTART "AI profile" 5 秒必读 | 30 分钟 | 无；可与 PR-2 合并 |
| 4 | **PR-5** | OAuth DDL 残留清理 + ADR | 1 小时 | 无；可与 PR-2/PR-4 合并 |
| 5 | **PR-3** | BFF→3 svc gRPC 化（4 子 PR）| 1~1.5 天 | 可与 1~4 并行；最后做 |

实际可并行执行（PR-1 / PR-2+4+5 / PR-3 三条线独立）。

---

## 四、TDD 节奏（每 PR 严格遵守 AGENTS.md §〇）

### PR-1 · 登录限流测试

```bash
# RED
git checkout -b fix/bff-login-throttle-tests
# 写 6 个测试函数（auth_handler_test.go）
go test ./internal/handler/ -run TestLogin_LockAfter5Failures
git commit -m "test(bff): RED 6 cases for login throttle isLocked/recordFailure/clearFailures"

# GREEN（代码已存在，RED 应已 PASS；如果 RED 失败说明有 bug — 修 GREEN 然后再跑 RED）
# 实际：本批是补测，所以代码逻辑已存在；RED → GREEN 自动通过
go test ./internal/handler/ -v
git commit -m "test(bff): GREEN login throttle tests + 锁定期 5min 验证"

# REFACTOR
# 加 mock 时钟（time.Now 注入）让测试快进 5min
git commit -m "refactor(bff): 注入 Clock 接口让 lock 过期测试不 sleep"
```

### PR-3 · gRPC 化（按 chat-svc PR-GRPC-1~6 范式）

```bash
# PR-3.1 proto + stub（半天）
git checkout -b feat/bff-grpc-3svc-proto
proto/user.proto proto/assessment.proto proto/analytics.proto
bash proto/gen.sh
go test ./emotion-echo-shared/pkg/{user,assessment,analytics}  # stub 测试
git commit -m "feat(proto): user/assessment/analytics .proto + stub 生成"

# PR-3.2 三 svc 加 gRPC server（半天）
... 3 svc main.go 加 GRPCServer { Port: 8887|8886|8885 }
... grpcserver/{server.go,user_server.go,...}
git commit -m "feat(svc): user/assessment/analytics gRPC server 骨架 + 拦截器"

# PR-3.3 BFF 改 gRPC client + feature flag（半天）
... web-bff/internal/downstream/user_grpc.go + assessment_grpc.go + analytics_grpc.go
... feature flag: USER_TRANSPORT=grpc|http
git commit -m "feat(bff): BFF → user/assessment/analytics gRPC client + feature flag"

# PR-3.4 smoke §契约 10/11/12 + OAP rpc tag（半天）
scripts/smoke_bff_3svc_grpc.sh
scripts/test_smoke_bff_3svc_grpc.sh
git commit -m "test(scripts): PR-3.4 smoke §契约 10/11/12 端到端验证"
```

---

## 五、调研依据总览（每 PR commit msg 末尾格式）

| PR | 已读文件 / 已查文档 / 已跑验证 |
|---|---|
| PR-1 | `auth_handler.go:53-302`（已实测）+ 现有 `auth_handler_test.go` mock 范本 + `fakeUserClient` 模式（已在 `auth_handler_test.go` 中）|
| PR-2 | `decisions.md:403-454`（已实测）+ `decisions.md:127-149` 决策 9 末尾关系说明段（已合规）|
| PR-3 | `proto/chat.proto`（范本）+ `shared/pkg/emotionchat`（生成代码）+ `chat-svc/internal/grpcserver/chat_server.go`（server 范本）+ `web-bff/internal/downstream/chat_grpc.go`（client 范本）+ `stage-58-q3-followups.md §五`（24 commit 范式）|
| PR-4 | `stage-60-pr-tts-vendor-landing.md` §五（关闭命令）+ `todo-pile-2026-09-04.md §A1`（关闭命令）|
| PR-5 | `docs/plans/wechat-qq-login-and-upload.md:9-13`（superseded-by Stage 38-A）+ `legacy/emotion-echo-gin/internal/handler/oauth_handler.go`（历史）+ `emotion-echo-user-svc/internal/model/user.go`（无 OAuth 字段确认）|

---

## 六、不在 Stage 62 范围（与之前 sprint 候选一致）

- CI/CD pipeline（决策 3："单机多实例不启用" + 现状不阻塞）
- DB migration tool（Stage 33 deferred；本批**只** drop 单表，不引入工具）
- Kafka DLQ（Stage 33 deferred）
- Helm chart 与 compose dev 全面对齐（todo-pile §D6）
- Nacos Go SDK 升级（todo-pile §B2；等官方 v2.4 stable）
- 决策 20 owner sign-off（5 分钟，不开 PR；等你拍板）

---

## 七、累计统计 / 失真登记

| 维度 | 数值 |
|---|---|
| **新增单测** | PR-1 = 6 用例；PR-3 = 17+ 用例（含3 svc server 测试 + 3 BFF client bufconn 测试 + smoke §契约 10/11/12） |
| **修改文件** | PR-2 = 1；PR-3 = 12+（3 proto + 3 svc main.go + 3 grpcserver + 3 BFF client + smoke 脚本）；PR-4 = 2；PR-5 = 3 + 1 新 migration + 1 新 ADR |
| **决策失真登记** | doc-drift-registry 拟新增 #26（OAuth DDL 残留 type 2 陈旧结论）+ #27（README `--profile ai` 启动段缺失 type 4 探测方法错误）|
| **未做** | 本会话不 commit（agent 约定）；owner review 后再 5~7 条 commit |

---

## 八、风险登记

| 风险 | 触发可能性 | 缓解 |
|---|---|---|
| BFF 登录限流测试假绿（mock clock 配错）| 低 | 6 个测试覆盖 happy/edge/race，mock clock 通过 `Clock interface` 注入（AGENTS.md §3.2 强制）|
| gRPC 端口冲突 | 低 | 用 :8885/:8886/:8887 避开现有 :8888/:8889/:8893；compose expose + dev port mapping |
| proto stub 生成失败 | 中 | 复用 `proto/gen.sh`（chat-svc PR-GRPC-1 已验证流程）|
| drop table migration 误删 | 中 | migration 文件用 `IF EXISTS` 兜底；先写 `SELECT COUNT(*)` 确认零引用（已实测）；smoke 后跑 migration 验证 |
| `--profile ai` 镜像拉不到 | 低 | README 明确写"镜像已在本地 / ACR"两条路径；本地 fallback 是 Web Speech API（todo-pile §A1 方案 2）|

---

## 九、后续 sprint 建议（独立 session）

1. **owner review + commit Stage 62 全部 5 PR**（1 小时）
2. **owner 签字 ADR-20**（5 分钟）
3. **chat-svc 表依赖清单 ADR**（todo-pile §D5，半天）
4. **BFF 路由契约测试三方对齐**（todo-pile §C8，1.5~2 天）
5. **proto StreamMessages 业务实现**（等需求触发）
6. **Stage 36-FU dashboard §C7 根因**（半天）

---

> 最后更新：2026-09-10 by Stage 62 调查 session
> 用途：把"最重要的几个未做"作为单一 sprint 推进；每 PR 配 TDD + 调研依据
> 关联：stage-61 收口后的下一步；ADR-001 v2 + 决策 9 正式收口后的架构清理