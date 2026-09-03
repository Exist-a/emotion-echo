---
status: planned
priority: high
owner: TBD
target-stage: Stage 39（候选）
created: 2026-09-04
depends-on:
  - docs/plans/nacos-enablement-dev.md
  - docs/stages/STAGE-28-LANDING.md
related-stages:
  - stage-28-observability.md (k8s 路径已全)
  - stage-32-apisix-reintroduction.md
  - stage-33-p0-fix-bff-purify.md
  - stage-35-system-feasibility.md (SkyWalking dial fail 现象记录)
related-adrs:
  - adr-2026-09-loki-aggregator-dev.md (dev Loki 选型)
  - adr-2026-09-nacos-reintroduction.md
---

# Plan — dev compose 可观测性三层补齐（与 k8s 路径对齐）

## 一、现状（与代码事实对齐）

### 1.1 k8s 路径：✅ 已交付

[STAGE-28-LANDING.md §三](docs/stages/STAGE-28-LANDING.md) 已记录完整可观测性四件套：

- `charts/prometheus`：4 scrape job（kubernetes-pods by annotation / apisix:9091 / skywalking-oap:1234 / prometheus-self:9090）
- `charts/grafana`：dashboard sidecar 自动加载
- `charts/loki` + Promtail DaemonSet：日志聚合
- `charts/alertmanager`：webhook 占位

6 个业务 svc + llm-service 的 deployment 都加了 `prometheus.io/scrape: "true"` annotation（`charts/.../{user,chat,analytics,assessment}-svc/templates/deployment.yaml` + ai-svc / llm-service）。

### 1.2 dev compose 路径：❌ 三层全缺

[deploy/docker-compose.infra.yml](deploy/docker-compose.infra.yml) 当前服务清单：

- postgres / redis / kafka / skywalking-oap / skywalking-ui / nacos / etcd / apisix

**没有 prometheus / grafana / loki / alertmanager / promtail**。

业务 svc 都启了 `SKYWALKING_OAP_ADDR` env（[deploy/docker-compose.apps.yml:49](deploy/docker-compose.apps.yml#L49) 等），但 6 个 svc（含 llm-service）的 yaml 写的是字面 `${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}`——go2sky 不会解析 bash 占位符，dial 报"too many colons in address"（[stage-35-system-feasibility.md:78](docs/stages/stage-35-system-feasibility.md#L78) 记录）。

**dev 启动后 SkyWalking OAP/UI 在跑，但 6 个 svc 全连不上**——trace / metrics / log 三层在 dev 全失效。

### 1.3 APISIX 三层配错（两条独立问题）

**问题 A：SkyWalking endpoint 配成 127.0.0.1**（[deploy/apisix/config.yaml:171](deploy/apisix/config.yaml#L171)）

```yaml
skywalking:
  endpoint_addr: http://127.0.0.1:12800  # ← APISIX 容器内回环，trace永远到不了 OAP
```

应该是 `http://emotion-echo-sw-oap:12800`。

**问题 B：seed.sh 路由 plugins 缺日志/trace 插件**（[deploy/apisix/seed.sh:124-162](deploy/apisix/seed.sh#L124-L162)）

```json
{
  "jwt-auth":    {...},
  "limit-count": {...},
  "limit-req":   {...},
  "api-breaker": {...},
  "cors":        {...},
  "prometheus":  {}   ← 仅 metrics，没 file-logger / skywalking-logger
}
```

即使 endpoint 修对了，skywalking-logger 插件也没挂，trace 仍上报不到。

### 1.4 access_log 落盘 + 容器销毁即丢

[deploy/apisix/config.yaml:269](deploy/apisix/config.yaml#L269) `access_log: logs/access.log`——容器内路径，docker-compose 没 mount volume 出去。容器重启即丢，没法跨重启分析。

### 1.5 与业务 svc /metrics 的关系

业务 svc 都已挂 [emotion-echo-shared/pkg/metrics](emotion-echo-shared/pkg/metrics/metrics.go) `GinMetricsMiddleware` + `PromHTTPHandler`：

- `/metrics` 端点已暴露（[stage-38-system-status.md:46](docs/stages/stage-38-system-status.md#L46)：BFF 201 series）
- ai-svc 有额外 4 个 fusion collector（[fusion_metrics.go](emotion-echo-shared/pkg/metrics/fusion_metrics.go)）

但**没人 scrape**——compose 里没有 prometheus，k8s 路径的 scrape job 只覆盖 k8s Pod。

---

## 二、为什么"没补齐"

| 原因 | 证据 |
|------|------|
| Stage 28 只交付了 k8s 路径 | [STAGE-28-LANDING.md §一](docs/stages/STAGE-28-LANDING.md) 不变量二："不动 docker-compose（deploy/ 保留作为 fallback）"——故意只做 k8s |
| compose 路径被定位为 dev/fallback | [stage-28-observability.md §一](docs/stages/stage-28-observability.md) "沿用 Stage 27 的不变量" |
| dev 用户当时没要求 | Stage 35 才实测发现 SkyWalking dial fail（[stage-35-system-feasibility.md:78](docs/stages/stage-35-system-feasibility.md#L78)）；但归类为"非致命噪音"放过 |
| APISIX endpoint 配错是 copy-paste 失误 | Stage 32 PR-14 commit 没补这行；Stage 33 P0 修复聚焦 BFF 没碰 APISIX |
| Loki/ES 选型未做 | 已补 [adr-2026-09-loki-aggregator-dev.md](docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md)（dev 单节点定 Loki，prod 留白）|
| yaml 占位符未根治 | go-zero conf 不解析 `${VAR:-default}`，是 Stage 22-B 已踩坑；本次用 shared helper 统一机制（见 §四 PR-2）|

---

## 三、目标（dev compose 三层"真正可用"）

按 OTel 三大支柱（metrics / logs / traces）拆：

### 3.1 Metrics：dev compose 装 prometheus + scrape

- [deploy/docker-compose.infra.yml](deploy/docker-compose.infra.yml) 加 `prometheus:9090` + `grafana:3000` 服务
- 配 4 个 scrape job（与 k8s 路径对齐）：
  - `kubernetes-pods` → 简化为静态 target（6 个 svc + APISIX + skywalking-oap）
  - `apisix` → `emotion-echo-apisix:9091`，metrics_path `/apisix/prometheus/metrics`
  - `skywalking-oap` → `emotion-echo-sw-oap:1234`（需 OAP 启用 SW_TELEMETRY，类似 [Stage28-E](docs/stages/STAGE-28-LANDING.md)改动）
  - `prometheus-self` → `localhost:9090`
- Grafana 加 Prometheus datasource sidecar
- dev compose 启动后 `curl http://localhost:9090/targets` 至少 6/6 UP

### 3.2 Logs：dev compose 装 Loki + Promtail + APISIX file-logger

依据 [adr-2026-09-loki-aggregator-dev.md](docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md)：

- 加 `loki:3100` + `promtail`（compose 路径用 service 替代 k8s DaemonSet）
- 镜像版本、retention、config 模式与 [charts/emotion-echo/charts/loki/values.yaml](charts/emotion-echo/charts/loki/values.yaml) 严格对齐
- promtail config 采集 docker container stdout（用 `docker.sock:ro` 或 `/var/lib/docker/containers` 卷）
- APISIX `seed.sh` 主入口路由 plugins 加 `"file-logger"`：
  ```json
  "file-logger": {
    "path": "/tmp/apisix-access.log",
    "log_format": "{\"client_ip\":\"$remote_addr\",\"...\":\"...\"}"
  }
  ```
- 加 volume mount：`/tmp/apisix-access.log → host ./tmp/apisix-access.log`，由 promtail 推 Loki

### 3.3 Traces：修 3 处错

**修1**：业务 svc + llm-service 的 yaml 占位符解析（7 个 svc 共享根因）

不是改 yaml 字面值——按用户决定走 **shared 包新增 `ExpandShellEnvDefaults` helper 统一机制**（选项 B，详见 §四 PR-2），避免下一个 env-driven 字段再次撞墙。

**修 2**：APISIX skywalking endpoint 改容器 DNS（[deploy/apisix/config.yaml:171](deploy/apisix/config.yaml#L171)）

```yaml
# before
endpoint_addr: http://127.0.0.1:12800
# after
endpoint_addr: http://emotion-echo-sw-oap:12800
```

**修 3**：seed.sh 主入口路由 plugins 加 `"skywalking-logger"`插件

### 3.4 验收条件（DoD）

| # | 项 | 验证方式 |
|---|----|---------|
| 1 | dev compose 启动后 `curl :9090/targets` 至少 6 个 scrape target UP | 脚本断言 |
| 2 | dev compose 启动后 `curl :3000` Grafana 可登录 + 看到 Prometheus datasource | smoke |
| 3 | dev compose 启动后 `curl :3100/ready` Loki ready | 脚本断言 |
| 4 | 浏览一次 `/api/v1/conversations`，Loki 能在 5s 内查到 access.log 行 | e2e |
| 5 | 浏览器请求 /api/v1/* 后，SkyWalking UI 服务拓扑出现 emotion-echo-web-bff → chat-svc 边 | 手动截图 |
| 6 | `deploy/apisix/seed_test.js` 加 4 项断言（file-logger / skywalking-logger / endpoint 容器 DNS / Prometheus plugin enabled）| `node seed_test.js` |
| 7 | 业务 svc 各加 `metrics_test.go` 断言 `/metrics` 暴露 ≥ N 个 series（保持现有 N≥201 for BFF）| `go test ./...` |
| 8 | `shared/pkg/config/expand_test.go` 覆盖 `${VAR}` / `${VAR:-default}` / 嵌套 / 未定义 / 多词 default 场景 | `go test ./shared/pkg/config/...` |

---

## 四、任务拆分（按 TDD 循环）

| PR | 范围 | RED 测试 | GREEN 改动 | 估 commit |
|---|------|---------|----------|---------|
| **PR-1** SkyWalking endpoint + skywalking-logger + file-logger 插件 | APISIX 配置 + seed.sh 插件链 | `seed_test.js` 加 4 项断言（endpoint 改容器 DNS、skywalking-logger 插件挂上、file-logger 插件挂上、access_log 路径非默认）| `config.yaml` 改 1 行 + `seed.sh:124-162` 加 2 个插件 | 2（test+feat） |
| **PR-2** shared 包新增 `ExpandShellEnvDefaults` helper + 7 个 svc 接入 | `emotion-echo-shared/pkg/config/expand.go` + 6 个 Go svc + llm-service yaml | `expand_test.go` 覆盖 `${VAR}` / `${VAR:-default}` / 嵌套 / 未定义 / 多词 default；各 svc `config_override_test.go` 加 SKYWALKING_OAP_ADDR 解析后断言 | 新增 helper + 7 个 svc 的 main.go 加载顺序调整（`conf.MustLoad → expand → conf.MustLoad → applyEnvOverrides`）| 3（test+feat+refactor）|
| **PR-3** compose 加 prometheus + scrape config | docker-compose.infra.yml + prometheus.yml 卷 | smoke 脚本断言 `:9090/targets` ≥ 6 UP | compose 新增 prometheus + grafana 服务 + scrape config 卷 | 2 |
| **PR-4** compose 加 loki + promtail + APISIX file-logger 落盘卷 | 同 PR-3 + seed.sh file-logger | smoke 断言 `:3100/ready` 200 + Loki query 返 access.log 行 | compose 新增 loki/promtail + promtail config + APISIX access.log volume mount | 2 |
| **PR-5** Grafana dashboard provisioning | grafana provisioning config + 1 张基础看板 | smoke 断言 `/api/dashboards/uid/emotion-echo-overview` 200 | Grafana sidecar 挂 dashboard json | 1（沿用现有） |
| **PR-6** runbook 文档 | docs/deployment/runbook/observability-compose.md | — | 写 dev 调试手册（Grafana 看什么 / Loki 查什么 / SkyWalking 怎么 trace）| 1 |

**总：6 PR / 11 commit（与 Stage 28 体量相当）**

### 4.1 PR-2 shared 占位符 helper 设计要点（选项 B 展开）

**动机**：

- go-zero `conf.MustLoad` **不解析** `${VAR:-default}` bash 占位符（[Stage 22-B](docs/stages/stage-22-B-deployment.md) 已踩）
- 当前 6 个 Go svc + llm-service 的 yaml 字面写 `${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}` 是**反模式**——5 处都在复制同一个错误
- 治本：在 `shared/pkg/config` 加解析层，避免下一个 env-driven 字段撞墙

**helper 命名**：`ExpandShellEnvDefaults`

**签名**（建议）：

```go
// ExpandShellEnvDefaults 把 YAML/JSON 字符串中的 ${VAR} 与 ${VAR:-default}
// 替换为 os.Getenv("VAR")（空时用 default）。支持递归展开（同一字符串内多个
// 占位符；占位符不可嵌套——${${INNER}} 视为字面）。
//
// 行为与 bash "${VAR:-default}" 一致：
//   - ${VAR}            → os.Getenv("VAR")（空时保留字面 "${VAR}" 作为未定义信号）
//   - ${VAR:-default}   → os.Getenv("VAR") ?? default（default 允许空格）
//   - $${VAR}           → 字面 "${VAR}"（转义，与 bash 一致）
//
// 未定义且无 default：返回 error（fail-fast，与 AGENTS.md §一.2 fail-fast 在
// bootstrap 对齐）。
func ExpandShellEnvDefaults(s string) (string, error)
```

**加载顺序调整**（每个 svc main.go）：

```
1. configFile := flag.String("f", "etc/<svc>.yaml", ...)
2. raw, err := os.ReadFile(*configFile)              // 读 raw bytes
3. expanded, err := config.ExpandShellEnvDefaults(string(raw))  // 占位符解析
4. conf.MustLoad(&expanded, &c)                       // 用解析后的字符串喂 go-zero
5. config.ApplyEnvOverrides(&c)                       // 兜底：env 全量覆盖 yaml
```

**测试覆盖**（expand_test.go）：

| 用例 | 输入 | 期望 |
|------|------|------|
| `${VAR}` 已定义 | `${HOME}/x` → `HOME=/root` | `/root/x` |
| `${VAR:-default}` 已定义 | `${HOME:-/tmp}/x` → `HOME=/root` | `/root/x` |
| `${VAR:-default}` 未定义 | `${UNSET:-/tmp}/x` → 无 UNSET env | `/tmp/x` |
| `${VAR}` 未定义 | `${UNSET}/x` → 无 UNSET env | error |
| `$${VAR}` 转义 | `$${HOME}` → `HOME=/root` | 字面 `${HOME}` |
| 多词 default | `${X:-hello world}/y` | `hello world/y` |
| 多占位符同字符串 | `${A}-${B}` → `A=1 B=2` | `1-2` |
| 不替换无关 `$` | `price=$10` | `price=$10`（只有 `${...}` 触发）|

**回滚策略**：helper 失败时返回 error，main.go 启动期 fail-fast（与 [internal/bootstrap/deps.go](emotion-echo-ai-svc/internal/bootstrap/deps.go) 的 `STARTUP_STRICT` 模式一致）。

---

## 五、风险与缓解

| 风险 | 缓解 |
|------|------|
| Promtail 需要 docker.sock 权限 | dev compose `volumes: - /var/run/docker.sock:/var/run/docker.sock:ro`；prod 走 k8s 已有方案 |
| Grafana provisioning 重启会丢临时面板 | 用 `dashboards sidecar` ConfigMap 模式（与 [Stage28-B](docs/stages/STAGE-28-LANDING.md) 一致）|
| APISIX file-logger 写性能 | 默认路径够 dev 用；高频访问 prod 前置 nginx access_log |
| SkyWalking trace 占内存 | OAP 9.7 默认 1.5GB；dev compose 已用 `SW_STORAGE=h2` 内存模式（[deploy/docker-compose.infra.yml:108](deploy/docker-compose.infra.yml#L108)）|
| 三层全加后 dev compose 资源翻倍 | 默认 profile 不启动 AI 模型（fer/sensevoice/xtts）来节省；observability 单独 profile 化 |
| `ExpandShellEnvDefaults` 引入新 helper 影响所有 7 个 svc 启动路径 | 单元测试覆盖 + 加载顺序封装成 helper 函数（避免每个 svc 手抄 3 步）|
| `${VAR}` 未定义时 fail-fast 阻断本地 IDE 调试（无 env 注入）| 在 helper 内部对常用变量（`SKYWALKING_OAP_ADDR` / `KAFKA_BROKERS` / `POSTGRES_DSN` 等）走"未定义且无 default → 静默保留字面 `${VAR}` 作为占位"，仅对**显式带 default** 的占位符 fail-fast |

---

## 六、不在本计划范围

- prod 长期存储（Loki S3 / minio 备份）→ 走 [adr-2026-09-loki-aggregator-dev.md §五](docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md) ADR-2 候选
- Alertmanager webhook 真实集成 → 走 Stage 28-D 已有 placeholder
- 业务自定义告警规则（HighErrorRate / PodOOMKilled）→ [STAGE-28-LANDING §九](docs/stages/STAGE-28-LANDING.md) 已列候选
- 跨服务 trace sampling 策略 → 走独立 ADR
- 多 region Prometheus federation → 长期

---

## 七、调研依据

> 满足 AGENTS.md §〇 "写文档前先调研"

| 文件 | 调研内容 |
|------|---------|
| `deploy/apisix/seed.sh:1-287` | 当前路由表 + plugins 链 |
| `deploy/apisix/config.yaml:150-156,168-172,269` | APISIX prometheus / skywalking / access_log 配置 |
| `deploy/docker-compose.infra.yml:103-260` | dev compose 服务清单（缺 prometheus/loki/grafana）|
| `emotion-echo-ai-svc/main.go:226-245` | 业务 svc SkyWalking tracer 初始化 |
| `emotion-echo-shared/pkg/metrics/metrics.go` | 业务 svc `/metrics` 中间件 |
| `charts/emotion-echo/charts/prometheus/templates/configmap.yaml:71-75` | k8s 路径 apisix scrape job |
| `charts/emotion-echo/charts/loki/values.yaml` | Loki 镜像与持久化 |
| `docs/stages/STAGE-28-LANDING.md §三/§七` | k8s 路径已交付的 4 scrape job |
| `docs/stages/stage-28-observability.md §一/§五` | k8s 不变量与 scrape config 完整版 |
| `docs/stages/stage-35-system-feasibility.md:78-80` | dev 模式 SkyWalking dial fail 现象 |
| `docs/stages/stage-38-system-status.md:46` | BFF /metrics 201 series 验证 |
| `docs/plans/nacos-enablement-dev.md` | 同构的 plan 模板（front-matter + §一现状 / §二原因 / §三目标 / §四任务）|
| `docs/architecture/decisions.md:101` | prod 日志后端选型意向（Loki vs ELK）|
| `docs/README.md` "按需找文档" | plans vs stages vs adrs 路由表 |

---

## 八、决策记录

| 问题 | 决定 | 理由 |
|------|------|------|
| Loki 是否独立 ADR？ | ✅ 轻量 ADR，只定 dev | 与 k8s 路径对齐需要锚点；prod 留作 ADR-2 |
| yaml 占位符修法 | ✅ 选项 B：shared `ExpandShellEnvDefaults` helper | 治本避免下一个字段撞墙；用户偏好扩展性 |
| 是否包含 llm-service | ✅ 包含 | 同一根因同修，trace 全链路打通 |