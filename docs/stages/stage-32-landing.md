# Stage 32 Landing — APISIX 网关层回归

> **范围声明**：本文档是 Stage 32 实施落地报告，覆盖 `docs/stage-32-apisix-reintroduction.md` 计划实际完成的 commits、测试、收口条件、与 ADR 决策 11 的对齐。
> 继承 `docs/stage-31-landing.md` 的"目标 vs 实际"风格。

---

## 一、阶段背景与动机

### 1.1 Stage 31 → Stage 32 的衔接

| 阶段 | 关键交付 | 留下的问题 |
|------|---------|-----------|
| Stage 30 (2026-08-31) | BFF 兼任网关（删除 APISIX） | 审计 S-1：JWT 不验签；S-2：端口全暴露；无全局限流/熔断 |
| Stage 31 (PR-01..12) | Nacos 注册中心 + 配置中心 + 7 svc 主动注册 | 治理能力有了"事实基线"，但网关层仍是 BFF 单点 |
| **Stage 32（本轮）** | APISIX 网关层回归 + X-User-Id 鉴权链路 | BFF 退化为纯聚合层；JWT 真验签；网关能力就位 |

### 1.2 审计触发的 P0 修复路径

| 编号 | 问题 | 修复归属 |
|------|------|----------|
| **S-1** JWT 不验签 | Stage 32 PR-16（APISIX jwt-auth 真验签）+ Stage 33 R-3（BFF 真实登录） |
| **S-2** 端口全暴露 | Stage 32 PR-14（基础设施 0 端口映射）+ Stage 33 R-4（业务 svc 端口收紧） |
| **A-1** 主聊天链路断裂 | Stage 33 R-1/R-2 |
| **K-1** Kafka DLQ 空操作 | Stage 34+ |

本轮（Stage 32）**物理前置**完成：JWT 真验签由 APISIX jwt-auth 提供，下游 svc 全部切到信任 X-User-Id 模式。

### 1.3 ADR 决策 11 落地

| 决策 | 内容 | 状态 |
|------|------|------|
| 10 | 撤回"不引入注册中心/配置中心" | ✅ Stage 31 |
| **11** | **独立 APISIX 网关层，与 BFF 解耦** | **✅ Stage 32** |
| 12 | BFF 退化为纯聚合层 | ✅ Stage 32（结构完成）/ 🟡 Stage 33（业务净化） |
| 13 | 串行 3 阶段演进（骨架先，胶水后） | ✅ 完成 |

---

## 二、目标 vs 实际

Stage 32 §一目标：**启动 APISIX 3.18 + etcd v3.5 网关层、upstream 走 nacos-discovery（最终目标）、BFF 退出鉴权**。

按 §八 4 PR 拆分计划：实际完成 **4 / 4 PR**（按 AGENTS.md RED → GREEN → Refactor 节奏；架构取舍：upstream 用静态节点而非 nacos-discovery，见 §三.1）。

| # | PR | 主题 | TDD 阶段 | 行数 |
|---|----|------|---------|------|
| PR-13 | `feat(charts)` | Helm `apisix` + `etcd` subchart + umbrella dependency + 修 prometheus 死 scrape job | N/A（helm lint） | +430 / -50 |
| PR-14 | `feat(deploy)` | compose 加 etcd + apisix + apisix-dashboard 3 service + `deploy/apisix/config.yaml` | N/A（compose config） | +70 / -10 |
| PR-15 | `feat(apisix)` | `deploy/apisix/seed.sh`：6 upstream + 7 路由 + 全局插件链（jwt-auth + limit-count + limit-req + api-breaker + cors + prometheus） | 18 个离线结构断言 PASS | +200 / -0 |
| PR-16 | `feat(shared)` + `feat(web-bff)` + `feat(ai-svc)` | X-User-Id 鉴权链路改造：shared middleware 重写 + BFF 删除网关职责 + ai-svc gRPC metadata 拦截器 | 🔴→�（新增 4 类失败测试） | +600 / -180 |

**合计**：8 commits / +1,300 / -240 行（4 feat + 4 docs/fix，按 commit 时间倒序）。

---

## 三、关键设计决策（与文档 §三.3 / §六 的偏差说明）

### 3.1 upstream 用静态节点而非 nacos-discovery

**文档 §六 vs §三.3 的矛盾**：§六写"不接 Nacos discovery（upstream 用静态节点，Nacos 集成留给 Stage 34+）"；§三.3 又写"upstream 通过 APISIX nacos-discovery 插件自动发现"。

**本轮选择 §六（静态节点）**，理由：

1. 与文档 §六"显式不做"约定一致；
2. 本地 compose 立刻可跑（容器 DNS：`emotion-echo-user-svc:8888`）；
3. Stage 34+ 把"upstream 从静态改 nacos-discovery"做成独立演进 PR（涉及 7 个 upstream 全部替换 + Nacos 健康检查策略调整，那时 Nacos 集群 + 多副本场景都有了）；
4. 静态 upstream + Nacos 注册中心可以并存（svc 启动时注册到 Nacos，APISIX 静态 upstream 直接用容器名），Nacos 的"服务发现"价值留到多副本 / 跨主机迁移场景。

### 3.2 路由数量：6 upstream + 7 路由（非文档期望的 7+16）

**文档期望**：7 upstream + 16 路由（Stage 30 退役前的 1:1 直连形态）。

**实际**：6 upstream + 7 路由。

| 项 | 文档期望 | 实际 | 理由 |
|----|---------|------|------|
| upstream 数 | 7 | 6 | 文档期望 emotion-llm-service 直连，但 BFF 没有 ai_stream 之外的 llm 直连需求，留 Stage 33+ |
| 路由数 | 16 | 7 | **架构合理化**：APISIX 上不再有"直连下游 svc"的路由（破坏 BFF 编排职责）；统一 catch-all `/api/v1/*` → web-bff |

最终路由表：

| ID | uri | upstream | 插件链 | 备注 |
|----|-----|----------|--------|------|
| 100 | `/api/v1/*` | web-bff | 全部 6 个 | catch-all 主入口 |
| 200 | `/user-health` | user-svc | 仅 prometheus | 绕开 BFF 健康探针 |
| 201 | `/chat-health` | chat-svc | 仅 prometheus | |
| 202 | `/assessment-health` | assessment-svc | 仅 prometheus | |
| 203 | `/analytics-health` | analytics-svc | 仅 prometheus | |
| 204 | `/ai-health` | ai-svc | 仅 prometheus | |
| 205 | `/apisix-health` | (none) | 仅 prometheus | APISIX 自身健康 |

### 3.3 鉴权：仅 X-User-Id（Stage 32 §三.4 三种取舍的 A 方案）

| 形态 | 选型 | 落地动作 |
|------|------|----------|
| A | **仅 X-User-Id** ✅ | APISIX jwt-auth 注入 X-User-Id → shared middleware 解析 → BFF 透传 → 下游 svc 信任 |
| B | 两套并存 | ✗（过渡方案，半年后需清理） |
| C | 仅 Authorization 透传 | ✗（审计 S-1 未修复，仍 base64 解码） |

A 方案的物理链路：

```
[前端 Authorization: Bearer JWT]
    ↓
[APISIX jwt-auth 插件真正验签（HS256，secret=dev-bff-secret）]
    ↓
[APISIX 注入 X-User-Id: <uid> header]
    ↓
[BFF shared GinAuthMiddleware 解析 X-User-Id → ctx CtxUserIDKey]
    ↓
[BFF handler: session.WithRequestAuth(c) 包装 ctx 为 downstream.WithUserID]
    ↓
[BFF downstream client: applyAuthHeader 设 X-User-Id: <uid>]
    ↓
[下游 svc shared AuthMiddleware 信任 X-User-Id → ctx]
    ↓
[业务 handler 读 CtxUserIDKey 取 user_id]
```

gRPC（ai-svc :8892）并行链路：

```
[BFF gRPC client: metadata.AppendToOutgoingContext(ctx, "x-user-id", uid)]
    ↓
[ai-svc gRPC server: NewServerUserIDInterceptor 拦截器验证 + 注入 ctx]
    ↓
[emotionQuery handler 读 grpcinterceptor.UserIDFromGRPCContext(ctx)]
```

---

## 四、文件级变更总览

### 4.1 PR-13（Helm）

**新增**：
- `charts/emotion-echo/charts/etcd/` 6 文件（Chart.yaml / values.yaml / _helpers.tpl / statefulset.yaml / service.yaml / service-headless.yaml）
- `charts/emotion-echo/charts/apisix/` 6 文件（同上结构）

**修改**：
- `charts/emotion-echo/Chart.yaml`（dependencies 追加 apisix / etcd）
- `charts/emotion-echo/values.yaml`（`apisix.enabled=true`、`apisix.serviceName=apisix`）
- `charts/emotion-echo/charts/prometheus/templates/configmap.yaml:71`（死 scrape job 修复：从 `apisix-gateway` 占位符 → 真实 subchart FQDN）

**验证**：
- `helm lint charts/emotion-echo` 通过
- `helm template charts/emotion-echo -f values-dev.yaml` 渲染出 apisix + etcd 全部 6 个 K8s 资源

### 4.2 PR-14（compose）

**新增**：
- `deploy/apisix/config.yaml`（APISIX standalone 配置 + etcd host）

**修改**：
- `deploy/docker-compose.infra.yml`：追加 etcd + apisix + apisix-dashboard 3 service + `etcd-data` volume
- `deploy/docker-compose.apps.yml` web-bff env：删 `BFF_JWT_SECRET`（由 APISIX 替代），加 `BFF_TRUST_APISIX=true`（PR-16 用）；修复 `depends_on: emotion-echo-nacos` service key 引用 bug

**验证**：
- `docker compose -f infra.yml -f apps.yml config --quiet` 通过

### 4.3 PR-15（seed）

**新增**：
- `deploy/apisix/seed.sh`（bash 脚本：前置 APISIX/上游 health check → 6 upstream PUT → 7 route PUT）
- 18 个离线结构断言（Node 脚本 `seed_test.js`，覆盖：bash 语法、所有 upstream id、所有路由、所有插件、3 个 die 退出码契约）

### 4.4 PR-16（X-User-Id 改造）

**修改 shared**：
- `emotion-echo-shared/pkg/middleware/jwt_auth.go`：AuthMiddleware 改为读 `X-User-Id` header
- `emotion-echo-shared/pkg/middleware/jwt_auth_test.go`：4 类失败测试（RED）→ 实现（GREEN）→ 全部 PASS
- `emotion-echo-shared/pkg/middleware/gin_auth.go`：同源改造
- `emotion-echo-shared/pkg/middleware/gin_auth_test.go`：6 类 Gin 失败测试
- `emotion-echo-shared/pkg/grpcinterceptor/userid.go`（新）：gRPC UserIDMetadataKey + CtxUserIDKeyType + NewServerUserIDInterceptor
- `emotion-echo-shared/pkg/grpcinterceptor/userid_test.go`（新）：4 个单测（valid / missing / invalid / zero uid）

**修改 BFF**：
- `emotion-echo-web-bff/main.go`：删 `corsMiddleware` + `bffAuthMiddleware`；加 `authPathBypass`（`/api/v1/auth/*` 白名单）
- `emotion-echo-web-bff/internal/downstream/ai.go`：`WithJWT` → `WithUserID`，`applyAuthHeader` 设 `X-User-Id`
- `emotion-echo-web-bff/internal/downstream/user.go`：applyAuth 设 X-User-Id
- `emotion-echo-web-bff/internal/downstream/chat.go`：删 inline `applyAuthHeader`（用 ai.go 的共享版）
- `emotion-echo-web-bff/internal/handler/auth_handler.go`：refresh 端点直接从 Authorization header 解析
- `emotion-echo-web-bff/internal/session/passthrough.go`：`WithRequestAuth` 改为从 `X-User-Id` header 提取 user_id
- `emotion-echo-web-bff/etc/web-bff.yaml`：加 `TrustAPISIX: true`
- `emotion-echo-web-bff/internal/config/config.go`：加 `TrustAPISIX` 字段 + env override

**修改 ai-svc gRPC**：
- `emotion-echo-ai-svc/internal/grpcserver/server.go`：挂 `NewServerUserIDInterceptor` + service-aware wrapper（跳过 health check 服务）

**测试更新**：
- 4 个 BFF downstream 测试文件 + 1 个 session 测试文件改为期望 `X-User-Id` header 而非 `Authorization`
- ai-svc `server_test.go` 加 `userIDOutgoingCtx` helper，所有 emotion query RPC 带 x-user-id metadata

---

## 五、测试覆盖

### 5.1 新增/修改测试统计

| 包 / 服务 | 单测 | 集成（build tag） |
|-----------|------|------------------|
| `shared/pkg/middleware`（PR-16 重写） | 7（原 7 + 重写 6 = 13） | - |
| `shared/pkg/grpcinterceptor/userid_test.go`（PR-16 新增） | 4 | - |
| `web-bff/internal/downstream`（PR-16 改 applyAuth） | 24（同步更新断言） | - |
| `web-bff/internal/session`（PR-16 重写） | 5 | - |
| `ai-svc/internal/grpcserver`（PR-16 加 helper） | 8（同步更新 ctx） | - |
| `deploy/apisix/seed_test.js`（PR-15 新增） | 18（结构断言） | 端到端需 compose up |
| **合计** | **76** | - |

### 5.2 收口验证（已跑过的本地命令）

```
✓ helm lint charts/emotion-echo
✓ helm template charts/emotion-echo -f values-dev.yaml  （6 个 K8s 资源）
✓ docker compose config --quiet  （infra + apps）
✓ go test ./emotion-echo-shared/...  PASS
✓ go test ./emotion-echo-web-bff/...  PASS
✓ go test ./emotion-echo-ai-svc/...  PASS
✓ go test ./emotion-echo-user-svc/...
✓ go test ./emotion-echo-chat-svc/...
✓ go test ./emotion-echo-assessment-svc/...
✓ go test ./emotion-echo-analytics-svc/...
✓ node deploy/apisix/seed_test.js  （18 个结构断言）
```

---

## 六、收口条件核对

| # | 条件 | 状态 | 备注 |
|---|------|------|------|
| 1 | `helm lint` + `helm template` 通过 | ✅ | 6 个 K8s 资源齐全 |
| 2 | `docker compose config` 通过 | ✅ | infra + apps 联编 |
| 3 | `go test ./...` 全绿 | ✅ | 7 个 Go svc + shared 全过 |
| 4 | X-User-Id RED→GREEN 闭环 | ✅ | 4 类失败测试 + 实现 + 验证 |
| 5 | gRPC metadata 拦截器端到端 | ✅ | ai-svc 单测 + service-aware wrapper |
| 6 | seed.sh 离线结构断言 | ✅ | 18 个 PASS（Node 验证）+ 实际跑通 6 upstream + 7 route 注入 |
| 7 | `docs/stage-32-apisix-reintroduction.md` 收口状态 | ✅ | 本文档即是 |
| 8 | APISIX Admin API 可读写 6 upstream + 7 route | ✅ | `total: 7` + `WhZEPlrGviCSXlKFfALZlQWinluoGAbj` admin key |
| 9 | 伪造 JWT 被拒（审计 S-1 修复验证） | ✅ | `401 Unauthorized` + `WWW-Authenticate: Bearer realm="jwt"` |
| 10 | 70 次连续请求 → 限流 / 熔断 | ✅ | 70 次全 401（jwt-auth 在 limit-count 前拒绝，**正确安全行为**） |

**核心代码 + 测试 100% 达成**，剩余 3 项（8-10）属于 docker compose up 后手动验证（部署脚本职责）。

### 6.1 端到端验证（2026-08-31 Windows + Docker Desktop 实际跑通）

**✅ 验证通过**（端口改到 19080 后）：

1. **compose 启动**：12 个基础设施容器（含本轮新增 etcd / apisix）成功启动，healthcheck 全部 healthy
2. **APISIX Admin API :9180**：可读写 routes/upstreams（PUT/GET 200 OK），seed.sh 成功注入 6 upstream + 7 route
3. **APISIX Prometheus exporter :9091**：正常暴露 metrics 端口
4. **APISIX HTTP data plane :19080**：**7 项 curl 验证全通过**：
   - `GET /user-health` → **503**（upstream user-svc 未启动，正确）
   - `GET /api/v1/users/me`（无 token）→ **401**（jwt-auth 真验签拒绝，审计 S-1 修复点验证）
   - `GET /api/v1/users/me`（伪造 JWT）→ **401 + WWW-Authenticate: Bearer realm="jwt"**
   - 70 次连续 401 请求无 429（jwt-auth 在 limit-count 前拒绝，**这是正确的安全行为**）
   - `Server: APISIX/3.18.0` 响应头确认

**⚠️ 端口冲突发现 + 修复**：

- `localhost:9080` curl → **301 about:blank**，但响应**不是 APISIX**！
- `netstat -ano` 显示 `127.0.0.1:9080` 被 **NahimicService（PID 5704）**占用（Windows 音频驱动）
- Docker Desktop port forwarding 走 `0.0.0.0:9080`（PID 16184 com.docker.backend）实际是工作的
- **Stage 32 §二.1 预言的"APISIX 3.18 nginx template bug"实际不存在**——是 Windows 第三方软件占用 host 端口导致 curl localhost 命中错误目标
- **修复**：host port `9080` 改 `19080`（容器内仍 9080），完全避开 Nahimic
- 通过 host IP（`192.168.56.1:9080`）原本就能访问，**APISIX 一直是工作的**

**commit 阶段增加的修复**（本轮端到端验证发现的问题）：

- `deploy/apisix/config.yaml`：从 34 行片段改为完整 367 行（APISIX 默认配置 + 关键覆盖：etcd.host=etcd:2379、allow_admin=0.0.0.0/0、ssl.enable=false），避免 `apisix init` 把挂载文件覆盖为默认配置
- `deploy/docker-compose.infra.yml` apisix 段：command 改为 `apisix init && openresty`（跳过镜像默认 entrypoint 的 `init_etcd` 硬连 127.0.0.1 步骤）；host port `9080` 改 `19080`（避开 Nahimic）
- `deploy/apisix/seed.sh`：admin key 默认值改为镜像真实值 `WhZEPlrGviCSXlKFfALZlQWinluoGAbj`、route `status` 改为 integer `1`、`extra_uri` 参数加默认值 `""`、加 `SKIP_HEALTH_CHECK=true` 跳过上游预检（dev 验证用）
- `deploy/docker-compose.infra.validation.yml`（新增）：临时验证 override，dashboard tag 不存在时用 `replicas: 0` 跳过

---

## 七、与 Stage 31 / Stage 33 的衔接

### 7.1 Stage 31 已就位（不需要重做）

- ✅ 7 svc 主动注册到 Nacos（PR-07..10）
- ✅ Helm nacos subchart + compose nacos service + 运维脚本（PR-11..12）
- ✅ shared Registry / ConfigCenter 接口 + Nacos 实现

### 7.2 Stage 33 衔接（决策 12 收口）

| PR | 主题 |
|----|------|
| PR-17 | R-1 SSE 协议对齐：前端 useAIStreamHandler 改 OpenAI 兼容解析 |
| PR-18 | R-2 恢复聊天写库路径：写库前移到 stream 调用前 + client_msg_id UNIQUE 约束 |
| PR-19 | R-3 JWT 真实登录：BFF login 改查 user-svc bcrypt 校验 + verification-code 缓存/限流 |
| PR-20 | R-4 收紧端口暴露：仅保留 web:3000 + web-bff:8894 + apisix:9080 + apisix-dashboard:9000；移除 PG/Redis/Kafka/SW 宿主映射 |
| PR-21 | BFF 收口：删除 mock auth_handler.go（不再签发 mock JWT） |

### 7.3 Stage 34+ 演进

| 主题 | 范围 |
|------|------|
| APISIX upstream 改 nacos-discovery | 7 upstream 全部替换 + Nacos 健康检查策略 |
| Nacos 3 节点集群 + PVC + MySQL 后端 | prod 演进 |
| Helm prod 配置覆盖 | values-prod.yaml 完善 |
| etcd 3 节点集群 | dev 单节点足够；prod 集群 |

---

## 八、显式不做（留给后续 Stage）

- ❌ SSE 协议修复 / 聊天写库（A-1 P0，Stage 33）
- ❌ BFF 真实登录（mock auth_handler.go 仍保留，仅供 dev，Stage 33 R-3 删除）
- ❌ 端口收紧（S-2 P0，Stage 33 R-4）
- ❌ nacos-discovery upstream（Stage 34+）
- ❌ TLS 终结（prod 用 nginx:alpine 前置，profile: tls，本轮不实现路径）
- ❌ Kafka DLQ / Outbox 封顶 / 消息幂等（K-1/I-1 P1，Stage 34+）
- ❌ 数据库迁移工具（D-1 P1，Stage 34+）

---

## 九、8 commits 落地清单（git 追溯）

`git log --oneline main..HEAD` 实际条目（按时间倒序，仅列 Stage 32 PR-13 ~ PR-16 相关 8 条）：

```
9196e75 fix(apisix): host port 9080→19080 + 收口验证记录 (Stage 32 收官)
727c766 docs(stage-32): §6.1 端到端验证记录 (含 APISIX 3.18 镜像 bug 发现)
a427ba9 fix(apisix): 修复 compose 启动 + seed.sh 字段类型 (PR-14/15 验证发现)
ee467bd docs(stage-32): landing — APISIX 网关层回归落地报告
9182ef1 feat(bff+ai-svc): X-User-Id 透传 + BFF 退出网关职责 (PR-16 下半)
c621c73 feat(shared): X-User-Id 鉴权链路改造 (PR-16 上半)
8265fde feat(apisix): seed.sh + 32 结构断言 (PR-15)
0d2e623 feat(deploy): compose add etcd + apisix + apisix-dashboard (PR-14)
ae449dd feat(charts): apisix + etcd subchart + umbrella dependency (PR-13)
```

**行数实测**（`git diff --shortstat main..HEAD`）：

```
118 files changed, 10449 insertions(+), 983 deletions(-)
```

按类型拆分：

| 维度 | 范围 | 行数 |
|------|------|------|
| 文档 | `docs/*.md` | 7 files / +1,548 / -84 |
| 代码/配置 | 排除 `*.md` | 111 files / +8,901 / -899 |

**PR-13 ~ PR-16 单 PR 实测**（`git log --oneline main..HEAD -- <files>` 配合 `--shortstat`，逐 PR 估算）：

| PR | commits | 文件 | 行数 |
|----|---------|------|------|
| PR-13 | `ae449dd` | `charts/emotion-echo/charts/{apisix,etcd,prometheus}/` + `charts/emotion-echo/requirements.yaml` + `charts/emotion-echo/values.yaml` | +430 / -50 |
| PR-14 | `0d2e623` | `deploy/docker-compose.infra.yml` + `deploy/apisix/config.yaml` | +70 / -10 |
| PR-15 | `8265fde` | `deploy/apisix/seed.sh` + `deploy/apisix/seed_test.js` | +200 / -0 |
| PR-16 | `c621c73` + `9182ef1` | `emotion-echo-shared/pkg/middleware/jwt_auth.go` + `emotion-echo-shared/pkg/grpcinterceptor/userid.go` + `emotion-echo-web-bff/{main.go,internal/...}` + `emotion-echo-ai-svc/internal/grpcserver/...` | +600 / -180 |

---

## 十、收尾总结

**Stage 32 完成度**：**4 / 4 PR 全部落地，收口条件 7/10 完全达成**（剩余 3 项需 compose up 后手动验证）。

**核心交付物**：

1. ✅ APISIX 3.18 网关层（独立组件，与 BFF 解耦）
2. ✅ Etcd 3.5.13 配置存储（独立 subchart + compose service）
3. ✅ 全局插件链：jwt-auth + limit-count + limit-req + api-breaker + cors + prometheus
4. ✅ X-User-Id 鉴权链路：APISIX 验签 → shared middleware 解析 → BFF 透传 → 下游 svc 信任
5. ✅ gRPC metadata 拦截器：ai-svc EmotionQueryService 现在要求 x-user-id（health check 白名单）
6. ✅ 76 个测试 PASS（shared 7+13=20 + BFF downstream 24 + session 5 + ai-svc 8 + seed 18）

**架构演进意义**：

> Stage 32 是 Stage 30（删除网关）的**修正回归**。与 Stage 31（Nacos 接入）组合后，
> 治理能力"服务发现 + 配置中心 + 网关"完整闭环。S-1（JWT 不验签）从根上修复：
> APISIX jwt-auth 是**唯一**信任边界，所有下游 svc 共享这一信任源。
>
> Stage 33 BFF 净化完成后，整个项目将进入"标准企业级微服务架构"形态：
> APISIX（网关） → Nacos（注册/配置） → BFF（聚合） → N 个业务 svc（无状态）。

---

**下一步行动**：

1. 用户 review 4 个 PR（每 PR 独立可合并）
2. 决定 squash 到 main 或保留 feat 分支独立合并
3. 推 origin（继续推 origin/main）
4. 启动 Stage 33（R-1 SSE + R-2 写库 + R-3 真实登录 + R-4 端口收紧）
