# ADR · 2026-09 · Nacos 与 APISIX 演进引入

> **本文档是 Stage 31/32/33 引入 Nacos + APISIX 的决策论证**。配套文档：`architecture-decisions.md` 决策 10/11/12/13；阶段实施文档 `stage-31-nacos-reintroduction.md` / `stage-32-apisix-reintroduction.md` / `stage-33-p0-fix-bff-purify.md`。
>
> **基线纠正**：2026-08-31 审计（`architecture-audit-2026-08-31.md`）曾判定"当前不需要注册中心/配置中心"——该判断把 dev 单实例静态寻址误当作设计目标，违背项目"分布式微服务架构"定位；同时 Stage 30 把 BFF 当网关用导致 JWT 不验签 / 限流熔断缺失 / CORS 错位等 P0 问题。本 ADR 撤回原判断，改回引入。
>
> **决策状态**：✅ **Accepted**（2026-09-03，ADR 决策 10/11/12/13 落地）
> **实施状态**：🚧 Stage 31 进行中（PR-01 文档收口完成；PR-02..12 串行推进）

---

## 一、为什么必须演进

### 1.1 业务驱动

项目自我定位（见 `architecture-decisions.md` "项目定位"段）："构建一个完整的、生产级 Go 微服务架构（含跨语言 Python LLM），服务于 Emotion-Echo 情绪分析产品"，"长期路线：本地 Docker → K8s manifests → 未来上生产"。分布式微服务架构的核心治理能力——服务发现、配置中心、API 网关、限流熔断——都是**为生产形态准备的**，不能因 dev 阶段实例数为 1 而删除。

### 1.2 P0 漏洞驱动

`architecture-audit-2026-08-31.md` 暴露的 P0 问题本质上都是"网关层职责被错误地塞进 BFF"：

- **S-1 JWT 不验签**：shared `jwt_auth.go` 注释自承"信任 APISIX 已验过"，APISIX 退役后无人验签 → 伪造 token 任意冒充；
- **A-1 SSE 协议不匹配**：BFF 兼任网关后协议没有强制约束点，前端/BFF SSE 格式互不认识 → 主聊天链路断；
- **S-2 端口全暴露**：BFF 没有 CORS/限流保护，下游 5 个 svc 全部映射宿主端口 → 局域网内任意直连；
- **K-1 Kafka DL 不可空 / I-1 无幂等**：分散在 svc 内部，缺乏统一治理面。

这些**只有在独立的网关层 + 注册/配置中心落地后**，才能系统性解决。

### 1.3 历史教训（两次删除的错误）

| 阶段 | 删除 | 删除理由 | 复盘结论 |
|------|------|----------|----------|
| Stage 5（2026-07-14） | Nacos | "Nacos 假集成，没人读"（APISIX 不读 Nacos） | 删除的是"接入方式"，不是"Nacos 本身"；Nacos 配置中心能力未被利用 |
| Stage 30（2026-08-31） | APISIX + etcd | APISIX 3.9 nginx 301 bug + BFF 取代网关 | 删除的是"网关层"，不是"网关职责"；BFF 兼任网关把治理能力搞丢了 |

两次删除的共性：把"治理能力"误等同于"具体组件"，删掉组件就放弃了治理。本次演进吸取教训：**架构先定（决策 11/12/13），组件演进由架构引导**。

---

## 二、选型论证

### 2.1 注册中心/配置中心：Nacos vs 候选

| 维度 | Nacos 2.4.x | Consul | K8s-native (Service+ConfigMap) | etcd |
|------|-------------|--------|-------------------------------|------|
| 服务发现 | ✅ 双协议（gRPC + HTTP），主动注册 + 心跳 | ✅ 健康检查强、多多多数据中心 | ✅ K8s Service 自带（仅 K8s 场景） | ⚠️ 仅 K/V，需自实现 watcher |
| 配置中心 | ✅ 内置 K/V + 监听推送，UI 完备 | ⚠️ 内置 KV 但无命名空间/灰度 | ⚠️ ConfigMap 无热更新（需 Reloader） | ⚠️ 仅 K/V |
| 一体化（注册+配置） | ✅ 一体，简化架构 | ❌ 需额外部署 | ❌ K8s-only | ❌ 仅 K/V |
| 多语言 SDK | ✅ Go v2.3.5+/Python 3.2.0+/Java 全 | ✅ 官方 Go/Python | ⚠️ 需通过 K8s API，非传统 SDK | ⚠️ 官方仅 Go/Java/Python |
| 社区活跃（2026） | ✅ 444 commits、1.3k stars | ⚠️ HashiCorp 商业化倾向 | ✅ K8s 官方 | ✅ K8s 内部依赖 |
| 与 APISIX 集成 | ✅ APISIX 官方支持 Nacos discovery | ⚠️ 需自定义 | ⚠️ APISIX K8s discovery 路径长 | ⚠️ APISIX 默认后端 |
| 本地 compose 启动 | ✅ 单镜像 standalone，单机即可 | ⚠️ 单镜像 + 强健康检查 | ❌ 必须 K8s 集群 | ✅ 单镜像 |
| 上手成本 | 中（中文社区丰富） | 中（英文为主） | 高（需 K8s 体系） | 高（需自实现服务发现层） |

**结论**：Nacos 在"功能覆盖（注册+配置一体化）/ SDK 多语言 / 与 APISIX 集成 / 本地启动友好"四维度同时领先，且与项目未来 Stage 33+ 演进（feature flag、限流阈值动态化、模型路由表）天然契合。K8s-native 路径因 Stage 32 仍依赖 docker-compose 单机多节点形态被排除。

### 2.2 API 网关：APISIX vs 候选

| 维度 | APISIX 3.18.0 | Kong 3.x | Envoy Gateway | Nginx + 自研 Lua |
|------|---------------|----------|---------------|------------------|
| 数据面性能 | ✅ OpenResty，极高 | ✅ OpenResty | ✅ C++ L7 | ⚠️ 受限于 Lua |
| 插件丰富度 | ✅ jwt-auth / limit-count / limit-req / api-breaker / cors / prometheus 全内置 | ✅ 插件市场 | ⚠️ filter chain | ❌ 需自实现 |
| 与 Nacos discovery | ✅ 官方支持 | ⚠️ 需自研 | ⚠️ 需自研 | ❌ 需自实现 |
| Helm/K8s 集成 | ✅ ApisixRoute CRD + 官方 chart | ✅ 官方 chart | ⚠️ Gateway API | ⚠️ Ingress |
| 上手成本 | 中（中文文档丰富） | 中 | 高（Envoy 概念重） | 高 |
| 已知未修 bug | ⚠️ 3.9 ssl_certificate_by_lua_block 301 bug（3.10–3.18 仍未官方修复） | — | — | — |

**结论**：APISIX 在"插件完备度 / 与 Nacos 原生支持 / Helm 集成"上领先，3.9 SSL bug 已知且有明确绕过方案（dev 走纯 HTTP / prod 前置 nginx 反代），不阻塞演进。详见 `stage-32-apisix-reintroduction.md` §三。

---

## 三、版本与依赖锁定

### 3.1 Nacos Server

- **镜像**：`nacos/nacos-server:v2.4.x`（standalone 模式即可，prod 上 3 节点集群）
- **端口**：8848 (HTTP console)、9848 (gRPC client)、9849 (gRPC server)
- **持久化**：dev 用 embedded derby；prod 改 MySQL（Stage 34+ 计划）

### 3.2 Nacos Go SDK

- **路径**：`github.com/nacos-group/nacos-sdk-go/v2`（master 444 commits，活跃）
- **版本锁定**：≥ v2.3.5（最稳定 tag，社区反馈良好）
- **架构选择**：**绕过 go-zero 官方 plugin**（go-zero v1.6/1.10 未做 v2 nacos plugin；v1 plugin 与 nacos-sdk-go/v2 API 不兼容），shared 直接裸调 SDK 工厂方法，向上提供 `Registry` / `ConfigClient` interface，便于测试时 mock

### 3.3 Nacos Python SDK

- **包名**：`nacos-sdk-python`
- **版本锁定**：**≥ 3.1.0**（关键约束：3.0.x 因"Agent registration logic fails to re-register after connection is disconnected" 缺陷被 yanked；3.1.0 起修复，3.2.0 为 2026-04-27 最新稳定版）
- **Python 版本**：≥ 3.10（SDK 要求）

### 3.4 APISIX

- **镜像**：`apache/apisix:3.18.0-debian`
- **Dashboard**：`apache/apisix-dashboard:3.18.0-alpine`（dev/admin 用，生产可关）
- **配置后端**：`quay.io/coreos/etcd:v3.5.x`
- **反代前置**（prod TLS 场景）：`nginx:alpine`（仅 dev 默认关闭）

---

## 四、命名约定

| 资源 | 命名 |
|------|------|
| namespace | `emotion-echo-dev` / `emotion-echo-prod` |
| group | `DEFAULT_GROUP`（默认 group，简化配置） |
| service name | `{service-name}`（小写连字符：`user-svc` / `web-bff` / `emotion-llm-service`） |
| dataId（配置中心） | `{service-name}.yaml`（例如 `user-svc.yaml`） |
| instance metadata | `stage=dev`、`version={git-sha}`、`profile=default` |
| 健康探活 | grpc health 5s/次，连续 3 次失败摘除 |
| 客户端拉取间隔 | 30s（可通过 env `NACOS_REFRESH_MS` 覆盖） |

---

## 五、与既有决策的关系

| 既有决策 | 影响 |
|----------|------|
| 决策 2（服务发现 = APISIX+etcd） | 已退役（Stage 30），被决策 11 替代 |
| 决策 7（鉴权 = APISIX jwt-auth） | 重新启用，由 APISIX 实际承担（BFF 不再做） |
| 决策 8（限流/熔断 = APISIX 插件） | 重新启用，由 APISIX 实际承担 |
| 决策 9（服务入口 = web-bff） | **保留**但限定职责——BFF 仅做纯聚合层，不再是"统一入口"（入口收敛到 APISIX） |

---

## 六、风险与缓解

| 风险 | 缓解 |
|------|------|
| APISIX 3.9 SSL bug 仍未修 | dev 纯 HTTP；prod 前置 nginx 终结 TLS；文档化"已知未修"作为运维提示 |
| Nacos 单节点脑裂（dev） | dev 单机足够；prod 路线注明 3 节点集群 |
| Python SDK 3.0.x 断线重注册缺陷 | 文档锁定 ≥ 3.1.0；require 文件钉死 |
| go-zero v1.10 与 nacos-sdk-go/v2 plugin 不兼容 | shared 绕开 go-zero plugin，直接裸调 SDK |
| BFF 仍可能背着网关职责"惯性" | 决策 12 明文禁止；Stage 33 删 `bffAuthMiddleware`/`corsMiddleware`；文档审计列入 R-3 |
| Helm subchart 重启链路（Nacos 启动顺序） | shared 注册逻辑加 `WaitForNacos` 重试（指数退避），不依赖 compose depends_on |
| Prometheus 死 scrape job（APISIX 9100） | Stage 32 修复 `prometheus configmap.yaml:71` 的 scrape job |

---

> 撰写时间：2026-09-03  
> 关联 ADR：决策 10/11/12/13（`architecture-decisions.md`）  
> 阶段实施：Stage 31/32/33（`stage-3X-*.md`）