---
status: accepted
date: 2026-09-30
deciders: [用户（D-29 拍板）, 当前协作 Agent]
supersedes: []
related: [adr-2026-09-nacos-reintroduction.md]
---

# ADR-2026-09 健康检查契约（liveness / readiness 分离）

## 一、背景与问题

E2E-23（健康检查与服务发现）计划期实测发现，本项目的健康探针**全线在"说假话"**：

| 现象 | 证据 |
|------|------|
| 4 个服务的 `/health` 把 `Status` **硬编码**为字面量 `"ok"`，而同一响应里的 `dbOk` 会如实变 false | `user-svc/internal/logic/healthlogic.go:41`、`analytics-svc:40`、`assessment-svc:40`、`ai-svc:40` |
| BFF 探出 `degraded` 却**恒返 HTTP 200**，而 compose healthcheck 只看状态码 | `web-bff/internal/handler/health_handler.go:109` |
| **全仓无任何 `/health` 检查 Redis 或 Nacos 注册状态** | 0 命中 |
| 编排层：11 个常驻服务零 healthcheck（含**全站入口 APISIX 自己**） | `docker-compose.infra.yml` 原仅 6 处 |

由此产生账本 **E2E-F-137** 记录的那个悖论：

> "BFF /health 200 与网关 503 **可同时成立**"

—— 进程活着 + 下游都通 = 自查健康，但它**自己在 Nacos 里没有实例**，
APISIX 解析不到节点 ⇒ 全站 502。**探针看不到"我是否可被发现"。**

## 二、决策

### 2.1 liveness 与 readiness 分离（D-29，用户 2026-09-29 拍板）

| 端点 | 语义 | HTTP 码 | 消费方 |
|------|------|---------|--------|
| `/health` | **liveness** —— 进程活着即可服务 | **恒 200** | `apisix-seed` 自带探活、smoke 脚本、既有引用 |
| `/health/ready` | **readiness** —— 依赖可用才接流量 | 依赖不通返 **503** | compose `healthcheck`（6 个 Go 服务） |

**否决方案**："直接让 `/health` 返 503"。

理由：`apisix-seed` 对 6 个服务有 `depends_on: condition: service_healthy`
（`docker-compose.apps.yml:722-735`）。若 `/health` 在依赖降级时返 503，
则**下游一降级 ⇒ seed 永不运行 ⇒ 网关连路由都没有** —— 把"某下游降级"
放大成"整站无路由"。

### 2.2 响应体的 `status` 字段必须说真话

`/health` 的 HTTP 码保兼容，但**字段不得撒谎**：任一必需依赖不通即
`status: "degraded"`。二者组合（码保兼容 + 字段说真话）是本决策的核心。

### 2.3 依赖覆盖范围

| 依赖 | 覆盖情况 |
|------|---------|
| DB（Postgres） | 4 个服务原有 `DbOK` 计算保留，并接进 `status` |
| Redis | BFF 已覆盖（`deps.redis`）—— 需先把 client 从 `buildAuthLockStore()` 内部提出来共用 |
| Nacos 注册状态 | BFF 已覆盖（`deps.nacos`）—— `NacosRuntime.registered atomic.Bool` |
| Kafka | chat-svc 仅判 `EventPublisher != nil`（**不真连**，已在代码注释与本 ADR 如实记录） |

**探针未注入时不报降级**（零值语义）：老部署无 Redis、无 Nacos 时不得被误判死。

### 2.4 gRPC health 的状态翻转

5 个 Go 服务在**优雅停机时**翻 `NOT_SERVING`，顺序为
**先翻状态（上游摘流量）→ 再 `GracefulStop`（等在途 RPC）**。
此前的实现是 `healthSrv` 作为构造函数**局部变量**，停机分支拿不到它。

> 注：本 ADR 同时澄清 `adr-2026-09-nacos-reintroduction.md` §四原写的
> 「grpc health 5s/次、连续 3 次失败摘除」—— **没有**这套周期主动探测；
> 实际机制就是上述"停机翻转"，其余摘除逻辑属上游（APISIX/Nacos）职责。

## 三、影响面

- 6 个 Go 服务各新增一路由 + 一套依赖检查；`shared/pkg/middleware` 白名单同步
  （`isHealthProbePath` 集中定义，gin 与 net/http 两版共用）
- `docker-compose.apps.yml` 的 11 处 healthcheck 改指 `/health/ready`
- `deploy/apisix/seed.sh` 自带探针改指 `/health/ready`（原探 liveness 恒 200，
  **结构上不可能发现降级**）
- **顺序敏感（实施期实测踩到）**：改了 compose 的 healthcheck 但**未重建镜像**
  ⇒ 旧二进制无该端点 ⇒ 401 ⇒ 容器永远 unhealthy，表现为"全栈假死"

## 四、后果

**正面**：
- 依赖真断时 readiness 真的会红（运行时验证：停 Redis → `degraded` + 503；
  停 Postgres → 503；恢复后回 200）
- 堵住 F-137 悖论的机制层 —— 未注册 Nacos 时 BFF 自报 degraded

**负面 / 代价**：
- 多一个端点要维护，且**新旧镜像不匹配会假死**（见上）
- `status` 字段的消费者需自行解析（HTTP 码不再单独承载语义）

**未闭合（如实记录）**：
- APISIX upstream 的**主动健康检查**（`checks` 段）未做，归 E2E-25（账本 F-154）。
  故实例摘除后 APISIX 摘除**有滞后**（实测停 user-svc 75s 后其路由仍 401 而非 503）
- `Resume()` 未接线（停机是单向终态，语义上用不到；如需"暂停后恢复"再补）
- **进程外告警**（Prometheus 自身死亡无人发告警）归运维轮，见账本 F-152

## 五、关联

- 决策登记：`docs/e2e-roadmap/decisions.md` D-29 / D-31 / D-32
- 执行记录：`docs/e2e-roadmap/stages/e2e-23-health-discovery/report.md`
- 账本：E2E-F-137（悖论）/ F-151（db-migrate）/ F-154（APISIX 主动探测）/ F-155（热更）/ F-156（F-107 失真）
