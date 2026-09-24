# Emotion-Echo · 架构决策记录（ADR）

> **本文档是 Emotion-Echo 微服务架构的"单一事实源"（Single Source of Truth）。**
> 所有 stage 文档、路线图、代码组织、配置都应与本文档一致。
> 决策变更时，**先改本文档，再改代码**，保持文档先行。

> 💡 **部署形态定位**：本项目当前采用「分布式架构 + 单机部署（单机多实例）」模式。详细差异、与多机部署的对比、以及当前部署栈的取舍见 [architecture-positioning.md](/docs/architecture/positioning.md)。
> 最后更新：2026-09-05（新增决策 19 · 仓库顶层命名规范与废弃件处置，详见 `adr-2026-09-repo-top-level-naming.md`；决策 18 · 文档失真治理，详见 `adr-2026-09-doc-drift-registry.md`；决策 17 · dev 模式日志聚合后端 = Loki，详见 `adr-2026-09-loki-aggregator-dev.md`）

> 2026-09-03：撤回 2026-08-31 决策 10 "不引入注册中心/配置中心" 判断；新增决策 11/12/13，演进路线 Stage 31/32/33；详见 `adr-2026-09-nacos-reintroduction.md`

---

## 🎯 项目定位

**目标**：构建一个完整的、生产级 Go 微服务架构（含跨语言 Python LLM），
服务于 Emotion-Echo 情绪分析产品。

**当前阶段**：dev / 本地 docker-compose，未上生产。

**演进路线**：本地 Docker（单机多实例，当前形态）→ K8s manifests 已备好（学习资产，**不启用部署**）→ 未来若业务规模增长到多机再评估 K8s。

---

## ✅ 已敲定的决策（不可变更）

### 决策 1：HTTP 框架 = **Gin**（不再用 go-zero）

> ✅ **2026-09-07 Stage 41 落地完成**（10 个 TDD PR 全部 merged，详见 [Stage 41 收口](../stages/stage-41-gozero-removal.md)）。
> 7 个 Go 模块（shared / user / chat / analytics / assessment / ai / web-bff）的 go-zero 完全移除：
> - `go-zero/core/conf` → `shared/pkg/config`（yaml.v3 + SetDefaults + 大小写归一）
> - `go-zero/core/logx` → `log/slog`（shared/pkg/logging 下沉，BFF/ai-svc 旧 internal/logging 改为 re-export）
> - `go-zero/rest.Middleware` → 自定义 `type Middleware = func(http.HandlerFunc) http.HandlerFunc`
> - `.api` 文件归档至 `legacy/goctl-apis/`
> - 22+ 个 goctl 文件头注释清理
>
> 验证（PR-8 落地后实测）：
> - `grep -rn "zeromicro" --include="*.go" --include="go.mod" .`（排除 legacy）= **0 命中**
> - 7 个 `go.mod` 全部 `zeromicro` 零引用
>
> 历史标注（2026-08-31 审计 E-2 触发）：决策 1 一度标"部分失效"（conf/logx/rest 残留）；Stage 41 完成后本标注行已移除。

| 维度 | 选择 |
|------|------|
| Go svc HTTP 框架 | **Gin** (`github.com/gin-gonic/gin`) |
| 原因 | 复用 legacy `emotion-echo-gin` 14 个 handler，团队零学习成本 |

**❌ 废弃**：go-zero 框架、goctl 代码生成器、tRPC 协议。

**理由**（决策记录）：
- 实际只用到 go-zero 的 30%（HTTP server + goctl 模板）
- go-zero 的核心特性（zrpc/breaker/limit/registry）我们都没用
- legacy 已经有完整的 Gin 业务代码，搬迁成本远低于重写

### 决策 2：服务发现 = **APISIX + etcd**（删除 Nacos）

> ❌ **2026-08-31 已退役**：Stage 30 删除 APISIX + etcd（commit `e9abac5`），服务入口由 web-bff 承担（决策 9）。"不引入注册中心"的结论不变——服务寻址回归静态 DNS，见决策 10 与审计 §七。

| 维度 | 选择 |
|------|------|
| 服务注册 | **APISIX** 直管 etcd 配置 |
| 配置存储 | etcd（APISIX 原生后端） |
| svc 主动注册 | **不需要**（svc 直接监听端口，APISIX 用固定 upstream） |

**❌ 废弃**：Nacos 作为注册中心。

**理由**：
- Nacos 注册了但没人读（APISIX 不读 Nacos）
- Nacos 挂了导致所有 svc 起不来（强耦合故障源）
- APISIX + etcd 已经把"路由 + 配置存储"包圆了
- 简化架构：少一个组件 = 少一个故障点

### 决策 3：分布式部署 = **本地 Docker（单机多实例）+ K8s manifests 备好不部署**

| 维度 | 选择 |
|------|------|
| 当前部署 | docker-compose（dev，单机多实例） |
| 未来部署 | **不启用 K8s 作为部署形态**；K8s manifests（Helm chart + kind）保留为学习资产与多机迁移预留，写好但不部署 |
| etcd 形态 | ~~dev: 单节点；K8s: 集群（Raft 3 节点）~~ ❌ 2026-08-31 已退役 |

**❌ 禁止**：在代码未完成时上 K8s。

> **✅ 执行状态（2026-09-07 收口）**：K8s manifests 已编写完成并在本地 kind 验证（Stage 27-30，`charts/emotion-echo` 24 subchart + `k8s/` 工具链），**云上从未部署**。经评估（单机场景 k8s 收益≈0、加重运维、学习已到位）**选定不启用 K8s 部署**；Helm chart / kind 保留为学习资产与未来多机迁移期权。stage-21-k8s-strategy.md 已标记为历史策略。

### 决策 4：跨服务调用 = **gRPC + .proto**（✅ 实施完成 · 2026-09-11）

| 维度 | 选择 | 实施状态 |
|------|------|---------|
| 外部 API（浏览器→svc） | HTTP REST + JSON + APISIX | ✅ 不变（决策 4 明文） |
| 内部 svc-to-svc | **gRPC + .proto** | ✅ **实施完成（核心业务路径 100% 覆盖）** |
| 异步事件 | Kafka + JSON | ✅ 不变 |

**实施里程碑**（2026-09-11 Sprint C/D/E/F1 收口）：

| Sprint | 落地内容 | commit |
|---|---|---|
| Sprint C | shared/pkg/ctxkey 重构 + 拦截器/middleware 两侧类型别名指向 ctxkey.UserID（解决策 18 #26） | `72c599d` |
| Sprint D | chat-svc gRPC 4 RPC 真实实现（SendMessage/ListMessages/ListConversations/DeleteConversation）（解决策 18 #27） | `3829154` |
| Sprint E | user.proto 扩 Login/Register RPC + user-svc gRPC server 实现 + BFF client 接入（解决策 18 #33） | `f755b71` |
| Sprint F1 | user.proto 扩 ResetPassword/Logout RPC + user-svc gRPC server 实现 + BFF client 接入 | `fa324df` |

**当前覆盖度**（2026-09-11 实测）：

| 维度 | 覆盖率 | 评注 |
|---|---|---|
| 核心业务路径（BFF→4 svc handler 调用） | **21/21 = 100%** | user/chat/assessment/analytics handler 调用的所有方法全走 gRPC |
| BFF→4 svc 全方法 | **100%** | user-svc 7/7（含 Logout）、chat-svc 4/4 handler、assessment-svc 5/5、analytics-svc 6/6 handler |
| 所有内部 svc-to-svc（决策 4 全文范围） | **~90%** | 12 条 gRPC + 4 条故意不做 |

**内部 svc-to-svc gRPC 化清单**（截至 2026-09-11）：

```
✅ chat-svc → ai-svc                          (UpsertNeutralEmotion)
✅ ai-svc → emotion-llm-service               (Python gRPC server)
✅ BFF → ai-svc (emotion_query)               (GetEmotionByMessage/Conversation/FusedEmotion)
✅ BFF → user-svc                              (7/7: Login/Register/ResetPassword/Logout/GetMe/UpdateProfile/GetUserById)
✅ BFF → chat-svc                              (4/4: ListConversations/SendMessage/ListMessages/DeleteConversation)
✅ BFF → assessment-svc                        (5/5: ListSurveys/GetSurvey/SubmitSurvey/ListMyResults/GetSurveyResult)
✅ BFF → analytics-svc                         (6/6 handler 触发: ReportsDaily/ReportsTrend/UserBehavior{3}/MentalHealthAssessment)

故意不做（plan §决策 A 已划定边界；🟢 = 2026-09-13 复核状态更新）：
- ❌ ai-svc → FER/SenseVoice/XTTS             (FastAPI 模型服务，AI profile 按需启用，调用量低；plan §决策 A 明确不做)
- ~~❌ BFF → ai-svc 业务方法（ai.go MultiModalAnalyze/SynthesizeSpeech/AIHealth）~~
  🟢 **Sprint F2 已落地**（2026-09-11，`6f64215`）：emotion_query.proto 扩 3 RPC + ai-svc server 实现 + BFF ai_grpc.go 接入
- ~~❌ BFF → llm-service (DeepSeek 外部 API)~~ 🟢 **Stage 81 起 gRPC 化**（超原划界预期：llm-service 独立微服务化后，BFF ai/stream 切 gRPC 上游 gRPC→HTTP→mock 优先级链；Stage 88 Nacos 注册接入）
- ❌ chat-svc → ~~PinConversation~~🟢 **Stage 72 已落地**（BFF→chat-svc gRPC Pin/Update 全链）/ StreamMessages（业务未触发，维持 Unimplemented 预留，见收口 ADR §八）
- ~~❌ chat-svc HTTP /api/v1/conversations 500~~ 🟢 **Stage 64 PR-3 关闭**（`3e07571`，#32：旧镜像时代 bug，v0.1.3 rebuild 实测 4 路径全 200/401）
```

**收口 ADR**：[`adr-2026-09-decision-4-closure.md`](adr-2026-09-decision-4-closure.md) — 决策 4 从"未来/待实施"翻"✅ 实施完成"的形式化收口，含覆盖度量化、故意不做的边界列表、后续 sprint backlog。

**关联**：
- 决策 5（Python LLM 独立微服务 + gRPC server）：不变，emotion-llm-service 维持 gRPC
- 决策 18（doc-drift registry）：本次收口登记 #25 #26 #27 #33 #34 #35 共 6 条失真全部关闭
- Stage 63 + Sprint C/D/E/F1 的端到端 docker 验证：详见各 sprint landed 文档

### 决策 5：Python LLM 服务 = **独立微服务 + gRPC server**

| 维度 | 选择 |
|------|------|
| 部署形态 | 独立进程/容器 `emotion-llm-service` |
| 框架 | FastAPI（HTTP 阶段）/ gRPC server（未来升级）|
| 协议 | 当前 HTTP → 升级为 gRPC（.proto 单一事实源）|

**理由**：
- 与 Go svc 解耦，可独立扩缩容
- proto 文件作为跨语言 API 契约
- Python LLM 可换实现（FastAPI → 直接 model serving）

### 决策 6：审计 = **白盒化 + JSON 日志 + trace_id 串联**

| 维度 | 选择 |
|------|------|
| 日志格式 | JSON（结构化） |
| 必含字段 | `ts`, `level`, `svc`, `trace_id`, `user_id`, `action` |
| API 访问日志 | APISIX access log（边缘） |
| 用户操作审计 | 各 svc 业务 logger |

> **DOC-6 (Round 1) 就地更正**：trace_id 跨 APISIX 边界串联在 Stage 95 P1-3 已修复
> （APISIX file-logger 现含 trace_id 字段）。Loki↔OAP 联动待 Promtail 多源采集落地后打通。
| 集中存储（dev） | 各 svc `out.log` 文件 |
| 集中存储（prod） | Loki + Grafana 或 ELK |

### 决策 7：鉴权 = **APISIX jwt-auth**（替换 svc mock 鉴权）

> ❌ **2026-08-31 已退役且未落地**：随 APISIX 一起退役。当前全链路 JWT **不验签**（shared `jwt_auth.go` 只 base64 解码 payload，签名验证点随 APISIX 消失），为审计 P0 问题 S-1；鉴权回归 BFF/svc 验签的修复见审计 §八 R-3。

| 维度 | 选择 |
|------|------|
| 鉴权位置 | **APISIX 网关** |
| svc 端 | **信任 APISIX 注入的 X-User-Id header** |
| 鉴权算法 | JWT（APISIX jwt-auth 插件） |

**❌ 废弃**：svc 内部 mock `X-User-Id` 鉴权（不安全）。

### 决策 8：限流 + 熔断 = **APISIX 插件**

| 维度 | 选择 |
|------|------|
| 限流 | APISIX `limit-count`（按 user_id） |
| 熔断 | APISIX `api-breaker`（保护下游 svc） |
| CORS | APISIX `cors`（统一一次配） |

> ❌ **2026-08-31 已退役且未落地**：限流/熔断随 APISIX 退役后**未在 BFF 实现**（stage-30 文档明示"当前未实现"）。恢复路径见 `stage-30-apisix-retirement.md` §五：BFF 内 `golang.org/x/time/rate` + `sony/gobreaker`（轻量）。

### 决策 9：服务入口 = **web-bff**（APISIX 已退役）

> ✅ 2026-08-31 生效（Stage 30 落地，commit `e9abac5`）。
> 🔧 **2026-09-04 关系说明补正**：本决策的"统一入口 = web-bff"措辞是 Stage 30 当时表述；
> Stage 32 决策 11 已引入 APISIX 网关层，**APISIX 才是 prod 视角下的唯一业务入口**，
> BFF 转为 APISIX 的 upstream。**本决策与决策 11/12 同时成立**——
> 决策 9 描述的是 BFF 在系统内的角色（聚合 5 下游 + SSE 编排），决策 11/12 描述的是
> 对外暴露路径（APISIX :19080 / dev BFF :8894 仅是 dev 调试例外）。
> 详见决策 12 末尾的"关系说明"段。

> **🔧 2026-09-10 决策 18 §4.4 / PR-C 正式收口**（登记于 doc-drift-registry #24 + #25）：
>
> 上方 2026-09-04 补正块已存在，但本决策**核心表头仍写"统一入口 = web-bff"**——
> 与决策 11/12 "APISIX 是唯一业务入口"长期字面冲突 4 个月。
> 经 PR-A（fail-fast helper 修复前端 fallback 字面值）+ PR-B（ADR-001 v2 决策栈收口）配套，
> 本决策正式收口如下：
>
> | 维度 | 状态 |
> |---|---|
> | **业务入口**（外部用户视角） | APISIX `:19080`（决策 11）|
> | **系统入口**（内部服务视角，BFF 与下游 5 svc 之间） | web-bff `:8894`（本决策）|
> | **dev 调试**（特殊例外） | web-bff `:8894` 直连（仅本机 dev 环境可接受；prod 必须关闭 8894 端口映射）|
>
> **正式表述**：**APISIX 是唯一业务入口**；BFF 是 APISIX 的 upstream + 系统内聚合层。
> 本决策标题"服务入口 = web-bff"措辞保留为历史快照，但实际语义以本收口段为准。
>
> **代码侧同步**（PR-A，commit 待落地）：
> - `emotion-echo-web/app/lib/apiBaseUrl.ts` 新增 fail-fast helper（决策 18 #24）
> - `useAIStreamHandler.ts:78` + `useTTSPlayer.ts:175` + `useApi.ts:29,31` 三处 fallback
>   字面值 `'localhost:8894/api/v1'` / `'localhost:8080/api/v1'` 全部移除
> - `nuxt.config.ts:19` 默认值 `'http://localhost:19080/api/v1'`（经 APISIX）作为唯一定义点
>
> **未做项**（不在 PR-C 范围）：
> - dev/prod compose override 已由 PR-ENV-1~4 落地（决策 20）
> - prod compose override 把 BFF 端口从宿主映射移除（compose.prod.yml 留 13 项 TODO 待远端部署时实现）

| 维度 | 选择 |
|------|------|
| 统一入口（系统内） | **web-bff** :8894（`/api/v1/*` 聚合 5 下游 + SSE 流式编排 + CORS） |
| 鉴权 | BFF 透传校验（当前 JWT **不验签**，审计 P0 问题 S-1，修复见审计 §八 R-3） |
| 网关演进 | 需要边缘层时：路 1 = BFF 内限流/熔断；路 2 = 重引 APISIX 3.10+（`stage-30-apisix-retirement.md` §五） |

> **🔧 2026-09-10 决策 18 §4.4 / doc-drift-registry #24 交叉引用**：
>
> 决策 9 写"统一入口 = web-bff :8894"是 Stage 30 视角，**但前端实际入口路径**由
> `nuxt.config.ts:19` + `.env:5` 决定（cf1c798 后默认走 APISIX :19080）——决策 11/12 是当前
> 准确视角（决策 12 末尾的"关系说明"已收口）。**代码侧残留**（决策 18 #24）：
> `useAIStreamHandler.ts:78` + `useTTSPlayer.ts:175` + `useApi.ts:29,31` 三处 fallback
> 字面值仍是 Stage 30 时代的 BFF 直连假设（`localhost:8894` / `localhost:8080`），
> 当用户漏配 `NUXT_PUBLIC_API_BASE_URL` 时静默回退——**仅端口表 / 文档措辞收口，
> 不足以根治语义未收口**。修正路径：3 处 fallback 统一改 `''` 或 throw + 强制
> 依赖环境变量（独立 sprint 处理，本决策文档仅登记）。

### 决策 10：配置中心 / 服务注册 = **演进引入 Nacos**（2026-09 撤回原"不引入"判断）

> ⚠️ **2026-09-03 撤回**原 2026-08-31"不引入"措辞。原判断把"当前 dev 单实例静态寻址"误当作"设计目标"，违背项目"分布式微服务架构"定位；且 Stage 30 把 BFF 当网关用导致 P0 问题（JWT 不验签 S-1、限流熔断缺失、CORS 错位 SSE 协议错位 A-1 等，详见 `architecture-audit-2026-08-31.md`）。本决策**改回引入**，分 Stage 31/32/33 演进；详见 `adr-2026-09-nacos-reintroduction.md`。

| 维度 | 选择 |
|------|------|
| 注册中心 | **Nacos 2.4.x**（`github.com/nacos-group/nacos-sdk-go/v2` ≥ v2.3.5；`nacos-sdk-python` ≥ 3.1.0 避开 3.0.x 断线重注册缺陷） |
| 配置中心 | 同 Nacos（一体化部署，避免引入多组件） |
| 配置中心**范围** | **仅放运营参数**（feature flag、限流阈值、模型路由表、Kafka 重试次数、A/B 分组）。`etc/*.yaml` 仍是启动默认值，Nacos 在启动后覆盖；**JWT secret、DATABASE_DSN、LLM_API_KEY 等敏感配置不进 Nacos** |
| 服务发现 | Nacos 主动注册 + 心跳；客户端定时拉取 + watch（30s 间隔）。**BFF 也参与发现**（Stage 32 APISIX `nacos-discovery` 上游拉取） |
| 命名空间 | `emotion-echo-dev` / `emotion-echo-prod`；group `DEFAULT_GROUP`；dataId `{service-name}`（注册）+ `{service-name}.ops.yaml`（运营参数） |
| 健康检查 | grpc health 探活（5s/次，连续 3 次失败自动摘除） |
| 演进路径 | Stage 31 注册+运营参数 → Stage 32 API 网关回归 → Stage 33 P0 修复 + BFF 净化 |

### 决策 11：API 网关 = **APISIX**（独立网关层，与 BFF 解耦）

> ✅ 2026-09-03 生效。纠正 Stage 30 "BFF 取代 APISIX"的错误归一；网关层独立。

| 维度 | 选择 |
|------|------|
| 网关 | **APISIX 3.18.0-debian**（Apache 顶级、OpenResty、插件丰富）+ `etcd v3.5.x` 配置后端 |
| 网关层职责（**仅本层做**） | 路由、鉴权（jwt-auth）、限流（limit-count/limit-req）、熔断（api-breaker）、CORS、全局日志、TLS 终结 |
| 3.9 SSL bug 绕过 | changelog 3.10–3.18 仍无 `ssl_certificate_by_lua_block` 修复条目（核实见 `stage-32-apisix-reintroduction.md` §三）；dev 走纯 HTTP，prod TLS 由前置 `nginx:alpine` 终结 |
| BFF 与网关关系 | BFF 是 APISIX 的 upstream（静态 upstream），BFF 不再承担任何网关职责 |

### 决策 12：BFF = **纯聚合层**（不再兼任网关）

> ✅ 2026-09-03 生效。澄清 BFF 与网关的边界。
> 🔧 **2026-09-04 关系说明补正**（决策 18 §4.4 / 登记实例 #12）：本决策与决策 9、
> 决策 11 在"BFF 是否唯一入口"上**字面表述不一致但语义互补**——本文收口如下：
>
> | 视角 | "唯一入口"指什么 | 落在哪 | 依据 |
> |---|---|---|---|
> | **业务入口**（外部用户视角） | APISIX `:19080` | `deploy/docker-compose.apps.yml` + Helm | 决策 11 |
> | **系统入口**（内部服务视角，BFF 与下游 5 svc 之间） | web-bff `:8894` | BFF `main.go` + APISIX upstream | 决策 9 |
> | **dev 调试**（特殊例外） | web-bff `:8894` 直连 | `apps.yml:602-604` 注释明文"prod 应仅暴露 APISIX" | 决策 12 dev 例外段 |
>
> **对外正确说法**：APISIX 是**唯一业务入口**；BFF 是 APISIX 的 upstream + 系统内聚合层。
> **dev 例外**：BFF :8894 监听宿主（`apps.yml:602-604` 注释）以便 Postman / 调试直连，
> dev 模式下 `BFF_TRUST_APISIX=true`（默认）信任任何 `X-User-Id` header，
> **仅本机 dev 环境可接受**，prod 部署必须用 prod override compose 关闭 8894 端口映射。
>
> 与决策 9 的关系：决策 9 写于 2026-08-31，是 Stage 30 当时"BFF 取代 APISIX 网关职责"的描述；
> 决策 11/12 在 2026-09-03 撤回并细化（决策 11 引入 APISIX 网关层，决策 12 把 BFF 收回纯聚合）。
> 因此"web-bff 是统一入口"是 Stage 30 视角，**当前视角** = "web-bff 是聚合层，APISIX 是网关层"。
> 决策 9 末尾的关系说明段同步加此注脚。

> **🔧 2026-09-15 Stage 97 就地更正 — llm-service HTTP 端鉴权**（Round 2 §P0-R2-3）：
>
> `emotion-llm-service/main.py` HTTP 端（`/analyze` / `/health` / `/metrics`）
> **dev 模式**（`INTERNAL_API_KEY` 未配置）开放无鉴权，便于本地调试 + e2e。
> **prod 模式**（`INTERNAL_API_KEY` 配置且非空）必须强制 `Internal-API-Key` header 校验，
> `test_http_routes.py::TestHttpApiKey` 4 用例锁死契约。
> 生产通过 gRPC 调用（决策 5 + 决策 11），HTTP 端仅供 dev 调试，prod 部署如启用必须配置 key。
> 详见 [`stages/stage-97-round2-p0-closure.md`](../stages/stage-97-round2-p0-closure.md) PR-1。

> **✅ 2026-09-15 Round 0.2 验收 — BFF/APISIX 入口语义已收口**（[`docs/plans/multi-round-iteration-2026-09-15.md`](../plans/multi-round-iteration-2026-09-15.md) §二 Round 0.2）：
>
> 决策 9 / 11 / 12 在 "BFF 是否唯一入口" 上的语义冲突**已于 2026-09-10 决策 18 §4.4 / PR-C 正式收口**（本文件 line 189-194 + 196-220 + 228-237 + 267-284 共 4 处关系说明段）。
> 本决策末尾的关系说明（line 286-293）补 `bff` / `apisix` 关键词引用，
> 方便 `grep` 快速定位语义冲突的"已收口"证据。
>
> `QUICKSTART.md:43` 端口表行措辞同步：
> - "BFF (唯一入口)" → "BFF (APISIX 后端，dev 调试可直连；prod 仅 APISIX 访问)"
> - "APISIX（唯一业务入口，决策 11/12）" 一行同步加
> 现状已合，本段仅做交叉引用登记（不重复 line 60-62 就地更正块）。

| 维度 | 选择 |
|------|------|
| BFF 职责（**仅做**） | 多服务聚合、字段裁剪、SSE 流式编排、多端适配（PC/移动）、业务上下文（会话级） |
| BFF 不再做 | JWT 验签、CORS、限流、熔断、TLS 终结、全局路由表（迁至 APISIX） |
| BFF 入口 | 容器内 `web-bff:8894`，仅 APISIX 内部访问；宿主机**dev 例外**保留映射（`apps.yml:602-604`） |
| BFF 内部鉴权 | 不做（信任 APISIX 已验签 + 注入 X-User-Id），通过 shared 中间件透传；`BFF_TRUST_APISIX=false` 走 JWT 兜底（apps.yml:594-596） |

### 决策 13：演进路线 = **串行 3 阶段**（骨架先，胶水后）

> ✅ 2026-09-03 生效。Stage 编号 31/32/33，每 Stage 含独立 PR + TDD + 收口文档。

| Stage | 范围 | 依赖 |
|-------|------|------|
| 31 · Nacos 演进 | 注册中心 + 配置中心落地；6 Go svc + 1 Python svc 接入；compose + Helm subchart | 无 |
| 32 · APISIX 回归 | 独立网关层 + etcd；BFF 退出鉴权/CORS/限流；3.9 SSL bug 绕开 | Stage 31（路由配置可推送到 Nacos） |
| 33 · P0 修复 + BFF 净化 | 修 SSE 协议、恢复消息落库、真实登录、端口收紧；BFF 移除所有网关职责 | Stage 32（APISIX 接管鉴权后 BFF 才能移除） |

### 决策 14：多模态融合 = **LLM-as-Fusion + LateFusion 兜底**（Stage 34）

> ✅ 2026-09-01 生效。详见 `stage-34-multimodal-fusion.md`。

| 维度 | 选择 |
|------|------|
| 主路径 | LLMFuser（DeepSeek/OpenAI 兼容协议，复用 BFF `LLM_BASE_URL`） |
| 兜底 | WeightedLateFuser（加权平均 + 模态缺失重分配） |
| 调度 | FusionWorker 5s tick，遍历 `fused_emotions` 中 `pending` 行 |
| 写库 | `FusedEmotionRepo.Upsert`（`ON CONFLICT message_id DO UPDATE`） |

### 决策 15：LLM Fusion 生产加固策略（Stage 35）

> ✅ 2026-09-03 生效。详见 `adr-2026-09-llm-fusion-hardening.md`。

| 维度 | 选择 |
|------|------|
| LLM 输出包装容错 | `unwrapLLMContent` 三段去包（``` ``` ``` ``` + 空白 + 双重 JSON） |
| LLM 输出校验 | 白名单 emotion + sentiment ∈ [-1,1] + modality_contrib 总和 ≈ 1 |
| 同 msgID 限流 | LRU(cap=1024) + TTL=4min，单实例内存 |
| LLM 超时 | 默认 3s（可 env `LLM_TIMEOUT` 覆盖） |
| LLM 雪崩保护 | 三态熔断器（Closed→Open→HalfOpen），连续 5 失败开 30s；**不重试** |
| 可观测 | 4 个 Prometheus collector（LLM call / latency / fallback / worker tick） |
| yaml 配置 | `${VAR:-default}` 占位符恢复；main.go `applyEnvOverrides` 补 `LLM_TIMEOUT` / `LLM_MODEL` / `LLM_BREAKER_*` / `WORKER_TICK_INTERVAL` |

### 决策 16：Stage 35 系统缺口正式登记（8 项不再 deferred）

> ✅ 2026-09-03 生效。详见 `adr-2026-09-known-gaps.md` + `stage-36-fixes-roadmap.md`。

8 项已知缺口全部纳入 Stage 36 修复日程（不再 deferred 到 Stage 36+）：

| # | 缺口 | 严重度 | 批次 |
|---|------|--------|------|
| G1 | 4 个 Go svc yaml 占位符（user/chat/analytics/assessment） | 🟡 中 | 36-A |
| G2 | chat-svc 缺 list conversations 端点 | 🟡 中 | 36-B |
| G3 | BFF 缺 analytics / assessment 路由聚合 | 🔴 高 | 36-A |
| G4 | Kafka off 时消息无自动情绪分析 | 🔴 高 | 36-B |
| G5 | 真实 LLM endpoint 未配 | 🔴 高 | 36-C |
| G6 | FER / SenseVoice `profile: ai` 镜像未构建 | 🟡 中 | 36-C |
| G7 | APISIX dashboard 镜像不可拉 | 🟡 中 | 36-D |
| G8 | Nacos 配置中心未启 | 🟢 低 | 36-D |

**原则**：Stage 36 之后所有缺口默认进入修复日程；唯一例外是需外部资源（API key / 付费服务）且无 dev 环境的项，标记为 **blocked-external**。

### 决策 17：dev 模式日志聚合后端 = **Loki（filesystem 单节点）**（2026-09-04）

> ✅ 2026-09-04 生效。详见 `adr-2026-09-loki-aggregator-dev.md`。
>
> **范围限定**：本文仅约束 dev compose 路径的日志聚合后端。prod 长期存储（Loki S3 / minio / EFK 替换）留作 ADR-2 候选，本 ADR 不展开。

| 维度 | 选择 |
|------|------|
| dev compose 日志后端 | **Loki 3.2.0** + Promtail（filesystem 单节点模式）|
| 镜像 / 版本对齐 | 严格复用 [charts/emotion-echo/charts/loki/values.yaml](../../charts/emotion-echo/charts/loki/values.yaml)（镜像 `grafana/loki:3.2.0` / `grafana/promtail:3.2.0`，retention 1d）|
| 与 k8s 路径一致性 | ✅ dev/k8s 行为一致 |
| 候选 | ❌ EFK（资源重，dev 不必要） / ❌ 直 docker logs（无查询能力）|
| 推动 | `plans/observability-compose-gap.md` §3.2 落地 |

---

### 决策 18：文档失真治理 = **结论须带可复现证据 + 单一登记入口**（2026-09-04）

> ✅ 2026-09-04 生效。详见 `adr-2026-09-doc-drift-registry.md`。
>
> **缘起**：Stage 38 §四隐患 5 记为"ADR 与代码失真累计（至少 3 处），待 ADR-20 立项"，
> 该 ADR 一直未建。2026-09-04 一次合并前复核，在两天内的文档里又实测出 **6 处失真**，
> 失真产生速度已超过修正速度。（当时随手写的"ADR-20"即本决策，
> 实际决策序号排到 17，故取 **18**。）

| 维度 | 选择 |
|------|------|
| 结论性断言 | **必须附可复现命令 + 关键原始输出**；无证据者降级标注"（未验证）" |
| "已知问题 / 预存在 FAIL" | **必须带最后验证日期**；跨 stage 引用前需先复验，不得直接继承 |
| 端点 / 路径类结论 | 必须先核对**代码侧路由注册处**的真实路径，禁止仅凭一次 curl 404 断言功能缺失 |
| 发现失真时 | **就地追加更正块**（日期 + 实测依据 + 结论变化），保留原文，不静默改写 |
| 登记入口 | 统一登记到 `adr-2026-09-doc-drift-registry.md` §二，不再另开 ADR |
| 候选 | ❌ 文档 lint / CI 校验（失真在事实正确性而非格式，自动化判不了"根因是否为真"） / ❌ 批量清洗 Stage 1~37 历史文档 |
| 与 AGENTS.md §0.2 关系 | **补充而非替代**：§0.2 约束动笔前的功课，本决策约束产出结论的质量 |

**已归纳的失真类型**：根因臆断 / 陈旧结论 / 未复跑即记录 / 探测方法错误——
共同点是**都能被"当场跑一次"证伪**，成本极低但没人跑。

### 决策 19：仓库顶层命名规范与废弃部署件处置（2026-09-05）

> ✅ 2026-09-05 生效。详见 `adr-2026-09-repo-top-level-naming.md`。

| 维度 | 选择 |
|------|------|
| 顶层目录命名 | **一律小写 kebab-case**；PascalCase 仅作 prose 展示名（展示名见 git-layout §六） |
| 前端目录 | `Emotion-Echo-Web` → **`emotion-echo-web`**（与 compose 容器名/服务键一致） |
| AI 模型推理仓 | `Emotion-Echo-LLM` → **`emotion-echo-models`**（内容为 FER/ASR/TTS，名不符实；与文本 LLM 服务 `emotion-llm-service` 区分） |
| BFF 目录 | `emotion-echo-web-bff` 保持不改（目录 = 服务键 = Nacos 注册名 = 网关上游，自洽；仅文档语义澄清"非前端"） |
| 曾用名 | `Emotion-Echo-Web` / `Emotion-Echo-LLM` / `emotion-echo-front` / `emotion-echo-llm-service`（容器名）均退役，仅历史文档保留 |
| 根目录 20 个残留 json | **已 git rm**（apisix-*.json / msg1-2 / query.json，内容被 deploy/apisix/seed.sh 取代） |
| 废弃部署件 | 根 `docker-compose.yml` → **`legacy/dev-root-compose/`**（归档）；`Chart.lock` 为可再生物且已忽略 → 直接清理 |

**理由**：目录命名大小写混用 + `Emotion-Echo-LLM` 语义名不符实（实为 FER/ASR/TTS），加上根目录 20 个无引用残留 json，造成组件辨识、文档引用与 clone 视图长期混乱（详见 `adr-2026-09-repo-top-level-naming.md` §上下文）。

**落地**：2026-09-05 一次性提交（git rm 残留 / git mv 改名 + 全仓引用同步 / .gitignore 修复 / 废弃件归档），commit 前缀 `chore(repo)` + `docs(deploy)`。

---

### 决策 20：环境配置分层策略（dev 本地 vs prod 远端）= **✅ Accepted**（2026-09-11 owner sign-off）

> 🟡 2026-09-08 立项，**待用户最终拍板**。详见 `adr-2026-09-env-profile-strategy.md`。
>
> **缘起**：Stage 57 批 3 收口时修 `SKYWALKING_ENABLED` 默认 false 的真 bug，触发"本项目到底要不要 dev/prod 分层"的内部讨论。**默认推荐方案**：候选 C（compose override 文件分层）+ apps.yml 用 `${VAR:-default}` 形式保持中性。
>
> **4 个候选**：
> - A. 保持现状不分层（散落 dev 假设）——❌ 否决
> - B. 单一 compose + env 驱动 ——🟡 部分接受（作为 C 的实施细节）
> - C. override 文件分层（compose.dev.yml + compose.prod.yml + apps.yml 中性）——✅ **倾向**
> - D. 启动脚本分流 ——❌ 否决（候选 C 的变体，丢失 compose 原生可读性）
>
> **关键差异清单**（13 项）：BFF 直连端口 / BFF_DEV_RETURN_CODE / SKYWALKING_ENABLED / KAFKA_ENABLED / AI profile / 日志级别 / JWT secret / Nacos 命名空间 / STARTUP_STRICT_DEPS / 数据持久化卷 / 资源限制 / TLS。
>
> **待决策问题**：候选 C 实施时机（立即落地 vs 等真要远端部署时做）+ `compose.prod.yml` 是建空壳还是写完整 + `BFF_DEV_RETURN_CODE` 默认值在 compose 文件里如何分布。

> **🔧 2026-09-10 决策 18 §4.4 就地更正块（登记于 doc-drift-registry #23）**：
>
> 上面"🟡 Proposed / 待决策"**与事实严重失真**——候选 C（compose override 文件分层 + `${VAR:-default}` 中性化）已于 **2026-09-09** 由 PR-ENV-1~4 全部 landed：
>
> | Commit | 日期（+0800） | 内容 |
> |---|---|---|
> | `ac0299e` | 2026-09-09 06:31 | PR-ENV-1 抽 `deploy/compose.dev.yml`（61 行） |
> | `f400e65` | 2026-09-09 06:32 | PR-ENV-2 `deploy/docker-compose.apps.yml` 中性化（18 处硬编码 → `${VAR:-default}`） |
> | `b89ecab` | 2026-09-09 06:33 | PR-ENV-3 `deploy/compose.prod.yml` 空壳占位（66 行） |
> | `b2ea516` | 2026-09-09 06:35 | PR-ENV-4 `deploy/configuration.md`（179 行）+ `QUICKSTART.md` 同步（3 处启动命令加 `-f compose.dev.yml`） |
>
> **TDD 测试脚本**（stage-58-q3-followups.md §二）：
> - `scripts/test_compose_override.sh`：10/10 PASS
> - `scripts/test_apps_yml_neutral.sh`：8/8 PASS
> - `scripts/test_prod_yml_stub.sh`：11/11 PASS
> - `scripts/test_docs_update.sh`：14/14 PASS
>
> **结论修正**：
> - **决策 20 实质已落地**，应改为 ✅ **Accepted**（2026-09-09）
> - 三个"待决策问题"的状态：① C 实施时机 = 已"立即落地"；② prod.yml = 已选"空壳"；③ configuration.md = 已写
> - 唯一遗留：owner 拍板正式 sign-off（决策 18 是失真台账，决策本身由 owner 签字收口）
>
> **原始 ADR 文档失真同步修复**：`docs/architecture/adr/adr-2026-09-env-profile-strategy.md` 头 4 行于本次会话追加就地更正块（不动原行，保留历史快照）。
> **详细落地报告**：[`docs/stages/stage-58-q3-followups.md` §二 PR-ENV-1~4](../stages/stage-58-q3-followups.md)。

> **✅ Owner Sign-off**（2026-09-11）：候选 C（compose override 文件分层 + `${VAR:-default}` 中性化）已由 PR-ENV-1~4 全 landed + 43/43 TDD 脚本 PASS。本决策从 🟡 Proposed 翻 ✅ Accepted。后续如需切换/扩展 profile，遵循 ADR-20 §C 候选方案 + Stage 58 §二测试脚本模式新增。

---

### 决策 21：OAuth DDL 残留清理 = **✅ Accepted**（2026-09-10）

> ✅ 2026-09-10 生效。详见 [`adr-2026-09-drop-user-oauth-ddl.md`](adr/adr-2026-09-drop-user-oauth-ddl.md)。
>
> **触发**：Stage 62 PR-5 调查发现 `emotion_echo_user.user_oauth` 表零引用（契约测试 [`scripts/test_user_oauth_zero_ref.sh`](../../scripts/test_user_oauth_zero_ref.sh) 5/5 PASS），确认 Stage 38-A 已主动弃 OAuth 路径但 DDL / 注释残留。
>
> **范围**：
> - 删除 `emotion_echo_user.user_oauth` 表（`deploy/db/05-drop-user-oauth.sql`，挂 `initdb.d`）
> - 清理 `01-create-schemas.sql` + `02-create-tables-in-schemas.sql` 中 OAuth DDL 块
> - 清理 `emotion-echo-web/.env.example` 中 WECHAT/QQ 模板注释
> - **保留**：`legacy/emotion-echo-gin/oauth_handler.go` 按决策 19 归档不动
>
> **撤销流程**（ADR 21 §三）：满足"用户明确决策 + 新建撤销 ADR + 新建 migration + 新 plan"四条件才能恢复 OAuth。
>
> **未做项**：已存在 dev 环境的 user_oauth 残留（数据卷非空）由后续 `migrate.sh` 单独 PR 收口。

---

### 决策 22：chat-svc 表依赖清单 + 字段变更契约 = **✅ Accepted**（2026-09-11）

> ✅ 2026-09-11 生效。详见 [`adr-2026-09-chat-svc-table-deps.md`](adr/adr-2026-09-chat-svc-table-deps.md)。
>
> **触发**：todo-pile §D5「chat-svc 表依赖清单 ADR」—— Stage 36-FU 报告 dashboard 4 个 chartData.length===0（实际根因见 [stage-65-dashboard-empty-root-cause.md](../stages/stage-65-dashboard-empty-root-cause.md)），历史多次因 chat-svc 表字段变更未与下游对齐导致静默失效（cf Stage 36-D Bug 2 initdb 容错包错语句）。
>
> **范围**：
> - chat-svc 拥有 `emotion_echo_chat` schema 的 3 张表（`conversations` / `messages` / `outbox_events`）的字段 + 索引 + 跨域依赖清单
> - 跨域依赖矩阵（chat-svc 表 → analytics-svc 视图 / ai-svc Kafka consumer / 前端 dashboard）
> - 跨域写入约束（chat-svc 零跨 svc 写入；event_id 生成是跨 svc 契约）
> - 字段变更 9 步 checklist（migration → model → types → proto → 跨域评估 → 集成测试 → regression 守护 → 文档同步）
> - 6 类禁止事项（跨 schema 外键 / JSONB 字段塞关键数据 / 删除 outbox 行 / 改 UNIQUE 约束 / 跨 svc 直接写库）
>
> **撤销流程**（ADR 22 §四）：任何字段变更必须按 §四 checklist 顺序执行，缺一步 reviewer 必须 reject PR。
>
> **未做项**（本决策不覆盖，留待下次 sprint）：chat-svc PinConversation / StreamMessages gRPC 实现（决策 4 ADR §八 backlog）。

### 决策 23：Helm chart 残余处置 = **冻结（学习资产不追加投入）**（2026-09-12）

> ✅ 2026-09-12 生效（Stage 74，owner sign-off）。上游依据：决策 3（K8s 备好不部署，2026-09-07 收口）。
>
> **触发**：backlog-order-2026-09-12 项 4 "Helm chart ↔ compose dev 全面对齐" 的 Stage 72 机械对齐后仍登记 5 项残余；Stage 74 计划时对照决策 3 复核——残余投入的对象是一个"不部署的学习资产"，收益不成立。
>
> **冻结清单**（K8s 不启用期内不处置，与 compose 演进保持已知偏差）：
> 1. NACOS_ADDR 硬编码 `nacos.ee-app.svc.cluster.local`（依赖 `--namespace ee-app` 安装约定，换 ns 即断）
> 2. web 子 chart apiBaseUrl 直连 web-bff（绕过 APISIX，违背决策 11；且无 apisix-seed chart，路由不会就位）
> 3. xtts / sensevoice 子 chart 镜像源与 compose 基线不一致（`emotion-echo/xtts` vs `ai4all/coqui`；sensevoice 缺 ACR 前缀）
> 4. 缺 MinIO / db-migrate / apisix-seed 三个 chart 等价物（compose 的 一次性容器在 K8s 侧无对应）
> 5. values-prod.yaml 停更于 Stage 28-F（镜像 tag / NACOS_* / KAFKA_* / gRPC 端口均落后 Stage 72 对齐基线）
>
> **纠偏（随本决策执行）**：Chart.yaml / values.yaml 中"APISIX 已退役"过期注释已改为指向决策 11（Stage 32 网关回归）；values-prod.yaml 头部加停更声明。
>
> **重启条件**：多机迁移正式启动时，按上述清单逆序重建（先 values-prod 与 namespace 约定，再补三个一次性 Job chart，最后接 APISIX 入口），以当时 compose 基线为唯一事实源。

### 决策 24：Nuxt 渲染模式 = **SSR（`ssr: true`）**（2026-09-17 实施 / 2026-09-18 追认补档）

> ✅ Accepted。**本文档性质：追认记录** —— 切换实际发生在 E2E-01 阶段（commit `6c91525`，8 文件 +113/-28），当时**未写任何决策记录**（ADR 目录 15 个文件无一条涉及渲染模式、本文件亦无条目），该缺口由 2026-09-18 独立审查发现并登记为 `E2E-F-52`，`R-02 #6` 要求补齐。本决策不改变实现，只补记录。
>
> **完整论证见 [ADR · 2026-09 · Nuxt 渲染模式切换到 SSR](adr/adr-2026-09-nuxt-ssr-mode.md)**。要点：
>
> **触发**：SPA 模式下 `useCookie` 异步写入与 `fetchUserInfo` 之间存在时序窗口 ⇒ 登录成功后立刻被踢回 /login、刷新页面闪现登录页。根因是服务端不参与首屏鉴权判定。
>
> **决策**：改用 SSR，由服务端在首屏渲染前读 cookie 判定鉴权状态（`nuxt.config.ts:8` `ssr: true`；`app/middleware/auth.global.ts:57` 的 `import.meta.server` 分支承担该职责）。
>
> **伴随约束（新增代码必须遵守）**：① 依赖浏览器专属 API 的代码必须做 SSR 隔离——Dexie/IndexedDB、`AudioContext`、WebGL 一律走懒初始化 / 懒 import / `ClientOnly` / `.client.vue`；② Playwright 必须等"可交互"而非仅 `visible`（SSR 下元素可见 ≠ 处理器就绪，E2E-01 有 3 个 spec 曾因此失败）；③ `pnpm build` 产出含 `index.html`。
>
> **备选被否理由**：保持 SPA 只改"登录后等待"能修第一条路径，但**刷新页面**时服务端仍不参与，首屏闪烁无法消除。
>
> **登记意义**：渲染模式属"架构关键词"，此后该类改动由 `scripts/check_adr_gate.sh` 检查是否附带 ADR + 本文件变更（R-03 #4）。

### 决策 25：前端 UI 组件策略 = **不引入 UI 框架，统一原生 + 设计 Token**（2026-07-17 实施 / 2026-09-19 追认补档 + 残留收口）

> ✅ Accepted。**本文档性质：追认 + 收口记录** —— 该策略由 `docs/legacy-plans/landed/elementplus-to-native.md`（2026-07-17）确立并已标 `landed`，但**未完全落地**：2026-09-19 E2E-11 浏览器实测发现 `chat/user/index.vue` 仍用 `<el-dialog>` / `<el-upload>`，控制台报 `Failed to resolve component` ⇒ "弹框"被当作普通元素内联恒渲染、`<template #footer>` 命名插槽被静默丢弃 ⇒ 「保存资料 / 确认退出 / 取消 / 留下」四个按钮在 DOM 中**根本不存在**。本决策一并追认策略并清理残留。
>
> **完整论证见 [ADR · 2026-09 · 前端 UI 组件策略](adr/adr-2026-09-frontend-ui-component-strategy.md)**。要点：
>
> **触发**：Element Plus 默认风格偏中后台，与情绪陪伴类产品调性不符；`:deep(.el-*)` 覆盖难以维持设计语言；体积/tree-shaking 不稳。
>
> **决策**：不下任何 UI 组件框架。模态弹框用 `Teleport + v-if + role="dialog"`（参考 `SecurityQuestionDialog.vue`）；文件选择用原生 `<input type="file">`；toast 用自有 `useNotify` + `NotifyHost`；图标用内联 SVG；按钮/输入用 `.ee-btn/.ee-field/.ee-input` + CSS Token。
>
> **伴随约束（新增代码必须遵守）**：① 页面中不得出现 `<el-*>` 标签（契约钉：`app/pages/chat/user/e2e-11-my-space-contract.architecture.test.ts`）；② **新增 layout 必须挂 `<NotifyHost />`**，否则该 layout 下所有 `notify()` 静默（契约钉：`app/layouts/nav.test.ts`）；③ 前端拦截（如头像 2MB）不得替代服务端校验，两侧都要有。
>
> **遗留（已记账，未消除）**：`package.json` 仍依赖 `@element-plus/icons-vue`（`FaceCamera.vue` 在用），`nuxt.config.ts` 仍 transpile 该包；原生弹框未做 focus trap（a11y 欠账，归 E2E-04）。
>
> **登记意义**：UI 组件选型命中 `check_adr_gate.sh` 的架构关键词（`vue`/`Vue`），此后该类改动需附 ADR + 本文件变更。

### 决策 26：用户个性化配置（config）持久化 = **服务端持久化（`users.config JSONB`）**（2026-09-19 决议 D-09 / 2026-09-20 E2E-12 实施）

> ✅ Accepted。**本文档性质：决策记录 + 实施回填** —— 该决议由用户于 2026-09-19 在 E2E-12 建档时作出（原编号 **D-09**），2026-09-20 随 E2E-12 实施落地。
>
> **完整论证见 [ADR · 2026-09 · 用户个性化配置（config）服务端持久化](adr/adr-2026-09-user-config-persistence.md)**。要点：
>
> **背景**：设置页字号/主题切换"操作看似成功、刷新即失效"（账本 `E2E-F-82`）。建档实测确认**四层契约各缺一环**：DB 无 `config` 列、proto `UpdateProfileRequest` 无该字段、BFF `UpdateProfileReq` 无该字段、`toProfileVM` 硬编码空 map ⇒ Go `json.Unmarshal` 忽略未知字段，接口返 200 但数据丢弃。关键事实：前端早已按服务端持久化写好（`stores/user.ts:59-95` 先调 API 成功再更新本地），缺陷在契约缺失而非前端设计。
>
> **决策**：选 **A. 服务端持久化** —— `emotion_echo_user.users` 新增 `config JSONB`（`NULL` = 未设置，区别 `{}` = 主动清空），经 proto（`optional string` JSON 文本）→ BFF → user-svc 全链路透传。
>
> **备选（未选）**：B. 仅本地存储（localStorage/cookie 镜像，删掉前端服务端写入）—— 换浏览器/清缓存即丢，且等于把「已实现却失效」主动降级为「设计如此」，与页面既有意图相反。
>
> **伴随约束（新增代码必须遵守）**：① `NULL` 与 `{}` 语义不得混用；② `config` 为开放 JSONB，服务端只保证存取往返，**值域不做服务端强校验**，未知值由前端 `getUserConfig` 的 `|| 'light'` 兜底回落且**不得崩溃**（E2E-12 测试点 #12）；③ proto 重生成必须复现原方式 `bash proto/gen.sh user.proto`，不得换生成器版本。
>
> **登记意义**：`config JSONB` 属 schema + proto 双变更，命中 `check_adr_gate.sh` 的架构关键词（`grpc`/`Postgres` 等），此后该类改动需附 ADR + 本文件变更。

> **🔧 2026-09-10 Stage 62 PR-2 微调**：下方 `## 🏗 当前架构全景` 已对齐决策 11/12
> 关系说明（APISIX = 唯一业务入口；BFF = 聚合层 / APISIX upstream）。
> 早期决策 18 #24 登记时基于"作者推断"误以为全景图含 '唯一前端入口' 措辞——实测全景图本身合规。
> 本段提醒后来者：全景图含义以本收口为准，**不要再写类似'web-bff 是唯一入口'的措辞**。

### 决策 27：仪表盘空态渲染模式 = **v-else-if 与图表卡片互斥**（2026-09-21 E2E-15 实施）

> ✅ Accepted。**防止 E2E-F-14 同型 bug 复发**——`ee-empty` 占位必须 `v-else-if="chartData.length === 0"`，禁止裸 `v-if` 单独条件或裸 `<div>`。修 4 dashboard / E2E-15 静态契约钉 12/12 PASS。

**完整论证见 [ADR · 2026-09 · 仪表盘空态渲染模式](adr/adr-2026-09-dashboard-empty-state.md)**。

### 决策 28：mental_health_assessments 表写入链 = **trigger runner 末尾补 INSERT**（2026-09-21 E2E-15 实施 / 用户决议 D-10）

> ✅ Accepted。修账本 E2E-F-10：`MentalHealthRunner.Run` 末尾调 `repo.Save(ctx, *MentalAssessment)`（assessment 为 nil 时写 placeholder 让表非空）；`PostgresMentalHealthRepo.Save` 实现 INSERT INTO + RETURNING id；rebuild `emotion-echo/analytics-svc:v0.1.8`。端到端：BFF mental-health 端点返回真实数据（overallScore + dimensions + riskLevel）。
>
> **残余（留账下一轮）**：① GetLatestAssessment SQL 未按 `assessment_type` 过滤（weekly/monthly 端点返 daily 最新一条）；② dev mode trigger HTTP JWT 401（analytics-svc secret 与 user-svc 不一致）；③ dashboard EmotionDistribution 可空（dev 没 AI 分析触发，归 §契约 4 整改）。

**完整论证见 [ADR · 2026-09 · mental_health_assessments 表写入链](adr/adr-2026-09-mental-health-trigger-save.md)**。

### 决策 29：仪表盘网格响应式断点 = **ref 追踪窗口宽度**（2026-09-21 E2E-15 合规补完轮 IAB 实测发现）

> ✅ Accepted。**IAB 内置浏览器实测抓到的真实缺陷**（Playwright + 单测均未覆盖）：`chartsCard.vue` 的 `computed` 内直接读 `window.innerWidth`（非 Vue 响应式依赖）⇒ resize 后列数不重算；`handleResize` 为空实现且注释"计算属性会自动更新"是错误假设。
>
> **实测**：1280px 加载后缩到 800px 视口 → 网格仍 `190.5px 190.5px`（2 列，应 1 列）→ 图表被挤压。修复（TDD）后：800px → `405px`（1 列）、1800px → `313.328px ×3`（3 列）。
>
> **规则**：响应式布局尺寸必须经 `ref` 参与 `computed`，禁止在 `computed` 内直接读非响应式全局值；禁止空 `handleResize` + "会自动更新"式注释。
>
> **为何既有测试漏掉**：原 2 条列数用例都在 `mount 之前` setInnerWidth ⇒ 只覆盖首次渲染，未覆盖 resize 重算。已补用例（`recomputes columns after window resize`）钉住。

**完整论证见 [ADR · 2026-09 · 仪表盘网格响应式断点](adr/adr-2026-09-dashboard-responsive-breakpoints.md)**。

### 决策 30：报表「会话数」数据源 = **msg_summary_v 同源**（2026-09-21 用户实测反馈驱动）

> ✅ Accepted。用户实测反馈日报显示「**0 段对话，33 条消息**」自相矛盾。核查出**两处独立缺陷**：
>
> ① **日报** `conversationCount` 取自 `user_behavior_events`（Kafka 事件链，dev 已停更在 09-14）而 `messageCount` 取自 `msg_summary_v` ⇒ 两指标数据源不同步。
> ② **周/月/年报** `conversationCount` 在 BFF `analytics_view.go` **硬编码 0**（注释称"TrendReport 没有 conv 维度"），且 `messageCount` 从 points 累计（那是情绪记录数不是消息数）—— 根因是 `TrendReport` 结构体 + proto `ReportsTrendResponse` 都无此二字段。
>
> **决策**：「会话数」与「消息数」统一取自 `msg_summary_v`；`conversation_count = COUNT(DISTINCT conversation_id)`。全链路补 proto（`message_count=4` / `conversation_count=5`）+ gRPC server 填充 + BFF 透传 + view 层去硬编码。
>
> **实测**：日报 `0` → **`16 段对话 / 33 条消息`**（与 DB 一致）；周报区间 `123 / 195`（DB 核对一致）。
>
> **语义选择（有意）**：只统计**有消息**的会话；空会话（建了未发言）不计入。与 `user_behavior_events` 的 `conversation.created` 口径不同，已在 ADR 记录。

**完整论证见 [ADR · 2026-09 · 报表会话数数据源](adr/adr-2026-09-dashboard-report-count-source.md)**。

### 决策 31：融合结果（face/voice emotion）注入 system prompt = **emotion context 段拼接**（2026-09-23 E2E-16 D-14 实施）

> ✅ Accepted。E2E-16 plan §B.10 要求多模态采集结果参与情绪计算。修法：
>
> - 前端 `useConversationSender` 从 `useFaceEmotion.getRecentEmotion()`（3 秒有效窗口）+ `useVoiceRecorder` 携带 `faceEmotion/faceConfidence/voiceEmotion/voiceConfidence` 字段到 `/ai/stream`
> - 后端 `aiStreamReq` 同步新增字段 + `buildSystemPromptWithEmotion` 新方法 = base + (可选) personality 段 + (可选) emotion context 段
> - emotion context 段中性句式 + 三句护栏（不点破来源 / 不贴标签 / 不过火）+ 中英映射（happy→愉快 等）
> - 无情绪上下文时 prompt 与 base 逐字相同（不污染）
>
> **决策**：最小模式 = 前端 payload 自带 emotion（emotionSource 高级模式 = DB 查历史 + `AIStreamDeps.Emotion` 字段留账 E2E-F-122 独立轮次）。
>
> **IAB 端到端 5/5**：happy → "愉快的事吗" / sad → "压着些什么" / angry → "烦心事" / anxious → "不安/放不下" / neutral baseline 不注入。
>
> **ADR**：[`adr-2026-09-emotion-context-injection.md`](adr/adr-2026-09-emotion-context-injection.md)

## 🏗 当前架构全景

```
                          浏览器 / 客户端
                                │
                                ▼ HTTP (dev) / HTTPS (prod)
                    ┌─────────────────────────┐
                    │  APISIX :19080           │  ← 唯一业务入口（决策 11）
                    │  路由 / 鉴权 / 限流      │     （dev 默认 HTTP；prod TLS 由前置 nginx 终结）
                    └────────────┬────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │  web-bff :8894           │  ← 聚合层（决策 9 / 12；APISIX upstream）
                    │  /api/v1/* 聚合 + SSE    │     鉴权透传（信 APISIX 注入 X-User-Id）/ 流式编排
                    │  (dev :8894 监听宿主 — 仅调试例外) │
                    └────────────┬────────────┘
                                 │ 静态寻址：compose 容器 DNS / K8s Service FQDN
                                 │ (Stage 31 演进：svc 主动注册到 Nacos，APISIX 通过 nacos-discovery 插件拉实例)
        ┌────────────┬───────────┼───────────┬────────────┬────────────┐
        ▼            ▼           ▼           ▼            ▼            ▼
   user-svc    assessment-svc  chat-svc   ai-svc    analytics-svc   llm-svc
   :8888          :8889         :8890     :8891       :8893        :8000
   (Gin)          (Gin)         (Gin)    (Gin)       (Gin)        (Python FastAPI)
       │             │             │   │       │             │
       │             │             │   ▼       │             │
       │             │             │  Kafka    │             │
       │             │             │  (chat-   │             │
       │             │             │  events)  │             │
       │             │             │   │       │             │
       │             │             │   └───────┤             │
       │             │             │           ▼             │
       │             │             │    emotion-llm-service ◄───────┘
       │             │             │    (gRPC + mTLS)
       │             │             │
       ▼             ▼             ▼           ▼             ▼
   emotion_      emotion_      emotion_   emotion_       emotion_
   echo_user     echo_assess   echo_chat  echo_ai        echo_analyt

   ┌────────────────────────────────────────────────────────────┐
   │  治理层（Stage 31/32 落地后）                                    │
   │  ✅ Nacos 2.4.x 注册中心（8848/9848/9849）+ 配置中心（运营参数）      │
   │     namespace: emotion-echo-dev / emotion-echo-prod               │
   │     group: DEFAULT_GROUP；dataId: {svc}.ops.yaml                  │
   │  ✅ APISIX 3.18 网关层（Stage 32 已落地；cf1c798 后 dev 端到端经 APISIX）│
   └────────────────────────────────────────────────────────────┘
   ┌────────────────────────────────────────────────────────────┐
   │  基础设施层                                                  │
   │  Postgres (5 schema) + Kafka + SkyWalking + Redis（闲置）       │
   │  ❌ etcd（Stage 30 已删；APISIX 默认后端，Stage 32 重新引入）       │
   └────────────────────────────────────────────────────────────┘
```

---

## 📋 服务清单（权威）

| svc | 端口 | 框架 | DB schema | 业务职责 | 状态 |
|-----|------|------|-----------|---------|------|
| **APISIX** | 19080 (HTTP) / 7943 (etcd) | OpenResty | — | **唯一业务入口**（决策 11/12）：路由 + jwt-auth + 限流 + CORS | ✅ Stage 32 |
| **web-bff** | 8894 | Gin | — | 聚合层（决策 9/12）：APISIX upstream，聚合 5 下游 + SSE 编排；dev 监听宿主 :8894（仅调试例外，apps.yml:602-604） | ✅ Stage 30 完成 |
| **user-svc** | 8888 | Gin | emotion_echo_user | 用户/Auth/上传 | ✅ Stage 1 完成 |
| **assessment-svc** | 8889 | Gin | emotion_echo_assessment | 量表/评估/报告 | ✅ Stage 1 完成 |
| **chat-svc** | 8890 | Gin | emotion_echo_chat | 会话/消息 + outbox | ✅ Stage 1 完成 |
| **ai-svc** | 8891 / gRPC 8892 | Gin | emotion_echo_ai | 情绪分析编排 | ✅ Stage 1 完成 |
| **analytics-svc** | 8893 | Gin | emotion_echo_analytics | 行为事件/报表 | ✅ Stage 1 完成 |
| **emotion-llm-service** | 8000 / gRPC 50051 | FastAPI | — | 文本情绪分析（当前为关键词器） | ✅ Stage 3 完成 |
| **emotion-echo-web** | 3000 | Nuxt 3 | — | 前端 SPA | ✅ |
| **FER / sensevoice / XTTS** | 8004/8002/8003 | FastAPI | — | 人脸/语音识别、语音合成；`--profile ai` 启用；dev 默认 `ai-api.yaml` BASE_URL 留空（仅文本情绪降级，cf. `apps.yml:387-389`） | ✅ |

---

## 📁 项目结构（权威）

```
Emotion-Echo/
├── AGENTS.md                                ← TDD 强约束
├── docs/
│   ├── architecture-decisions.md            ← 🆕 本文档（单一事实源）
│   ├── microservices-architecture.md        ← 当前架构总览
│   ├── distributed-roadmap.md               ← 5-Phase 路线图
│   ├── distributed-architecture.md          ← 选型说明
│   ├── microservice-decomposition-plan.md   ← 拆分规划
│   ├── stage-0-learnings.md                 ← Stage 0 复盘
│   ├── stage-1-completion.md                ← 阶段报告
│   ├── stage-2-async-pipeline.md
│   ├── stage-3-llm-integration.md
│   └── stage-4-emotion-query.md
├── deploy/
│   ├── docker-compose.infra.yml             ← 容器编排（含 Nacos，Stage 31）
│   ├── apisix/                              ← 网关配置（Stage 32 引入）
│   └── db/                                  ← schema 脚本
├── emotion-echo-shared/                     ← 共享代码（Gin middleware 等）
│   └── pkg/
│       ├── skywalking/                      ← tracer + gorm + redis hooks
│       ├── messaging/                       ← Kafka Producer/Consumer
│       ├── middleware/                      ← Gin middleware (auth/cors/recover)
│       ├── discovery/                       ← Nacos Registry（Stage 31 PR-02/03）
│       └── configcenter/                    ← Nacos ConfigCenter（Stage 31 PR-04/05）
├── emotion-echo-user-svc/                   ← 5 svc 各自独立（Gin）
├── emotion-echo-assessment-svc/
├── emotion-echo-chat-svc/
├── emotion-echo-ai-svc/
├── emotion-echo-analytics-svc/
├── emotion-llm-service/                     ← Python FastAPI
├── legacy/emotion-echo-gin/                 ← 旧单体（Gin，业务参考 + handler 来源）
└── emotion-echo-web/                      ← Nuxt 前端
```

---

## 🔧 各 svc 的标准目录（Gin 风格）

```
emotion-echo-{domain}-svc/
├── cmd/main.go                              ← main 入口
├── go.mod                                   ← replace → shared
├── etc/{domain}-api.yaml                    ← 配置
├── {domain}-svc.exe                         ← 编译产物
├── internal/
│   ├── config/                              ← yaml struct
│   ├── handler/                             ← Gin HandlerFunc（从 legacy 搬）
│   ├── logic/                               ← 业务实现（手写 TDD）
│   ├── model/                               ← GORM 模型
│   ├── repository/                          ← Repo interface + InMemory + Postgres
│   ├── svc/servicecontext.go                ← 依赖注入容器
│   └── middleware/                          ← svc 专属中间件（如有）
└── tests/                                   ← 集成测试（可选）
```

---

## 📡 协议分层（权威）

| 流量类型 | 协议 | 序列化 | 入口 | 状态 |
|---------|------|--------|------|------|
| **外部 API**（浏览器→BFF）| HTTP REST + SSE | JSON | web-bff :8894 | ✅ 已通 |
| **内部 RPC**（svc↔svc）| gRPC | Protobuf | 直接连 | ⏳ 待实施 |
| **异步事件**（svc→svc）| Kafka | JSON | Kafka broker | ✅ chat-events |

**当前阶段细节**：
- ai-svc → emotion-llm-service：gRPC + mTLS（Stage 18 落地）
- 其他 svc 之间：无直接调用

---

## 🎯 TDD 原则（不变）

1. **RED 先行**：先写测试，看到编译错误 / 测试失败
2. **GREEN 实现**：最小代码让测试通过
3. **测试是文档**：每个测试描述一个业务规则

测试统计目标：≥ 50 PASS（当前 70+ PASS，已超）。

---

## 🚦 启动 / 验证命令（权威）

```bash
# 1. 启动基础设施（含 Nacos 2.4.x，Stage 31 PR-11）
cd deploy && docker compose -f docker-compose.infra.yml up -d

# 2. 启动 6 个 Go svc（各自目录；启动后自动注册到 Nacos）
cd emotion-echo-user-svc && ./user-svc.exe &
cd emotion-echo-assessment-svc && ./assessment-svc.exe &
cd emotion-echo-chat-svc && ./chat-svc.exe &
cd emotion-echo-ai-svc && ./ai-svc.exe &
cd emotion-echo-analytics-svc && ./analytics-svc.exe &
cd emotion-echo-web-bff && ./web-bff.exe &

# 3. 启动 Python LLM（启动后自动注册到 Nacos）
cd emotion-llm-service && python main.py &

# 4. 验证（通过 BFF；各 svc 自带 /health）
curl http://localhost:8894/health          # BFF 聚合下游健康探测
curl http://localhost:8888/health          # user-svc
curl http://localhost:8890/health          # chat-svc
curl http://localhost:8889/health          # assessment-svc
curl http://localhost:8891/health          # ai-svc
curl http://localhost:8893/health          # analytics-svc

# 5. 验证 Nacos 注册中心（Stage 31 验收）
open http://localhost:8848/nacos           # 默认 nacos/nacos
# 服务管理 → 服务列表 → 应看到 7 个 service-name（user/chat/assessment/analytics/ai/web-bff/emotion-llm-service）且 health=true
./scripts/list_nacos_instances.sh

# 6. 看 trace
open http://localhost:18080
```

---

## 📊 当前进度

```
Phase 0 基础设施    ████████████████████ 100% ✅
Phase 1 微服务拆分   ████████████████████ 100% ✅ (5/5 svc 上线 + 接 DB)
Phase 2 Kafka       ████████████████████ 100% ✅ (异步管道跑通)
Phase 3 LLM 接入    ████████████████████ 100% ✅ (跨语言情绪分析)
Phase 4 业务深化     █████████████░░░░░  75%  (emotion 查询完成)
Phase 5 韧性+网关鉴权 ████████████████████ 100% ✅ (jwt-auth/limit-count/api-breaker 由 APISIX 统一处理，Stage 32 落地；Stage 74 CORS 修复后浏览器链路端到端通过)
Phase 6 K8s         ██████████████████░░  80%  (manifests 已编写+本地 kind 验证；云上部署不启用——决策 3，Helm 残余冻结——决策 23)

# Stage 31/32/33 演进（ADR 决策 10/11/12/13）——2026-09-12 销账：三项均已落地收口
Stage 31 Nacos 治理层  ████████████████████ 100% ✅ (Stage 31 落地；后续持续修复：Stage 62 心跳存活、Stage 72 PR-1 集成测试、Stage 75 PR-2 BFF gRPC 走 Discover)
Stage 32 APISIX 网关   ████████████████████ 100% ✅ (Stage 32 落地，dev 端到端经 APISIX)
Stage 33 P0 修复+BFF净化 ████████████████████ 100% ✅ (Stage 33 落地)
```

> 2026-09-12 Stage 75 收口时销账：上方 Stage 31/32/33 三行停留在收口前状态逾 1 个月
> （Stage 31 停在 20%、32/33 停在 0%），实际均已 100% 落地；Phase 5 同步纠正。
> 进度条是快照——以 `docs/stages/` 最新收口报告为准。

---

## 🔮 下一步（按优先级）

> ⚠️ 本节为 2026-07-14 遗留清单，多项已过期；**当前优先行动见 `architecture-audit-2026-08-31.md` §八**（R-1 起）。

1. **删除 Nacos 代码 + docker-compose**（决策 2 落地）— ✅ 已完成
2. **APISIX P0/P1 插件**：jwt-auth + limit-count + api-breaker
3. **svc 框架迁移**：go-zero → Gin（每个 svc 改 main.go + handler）
4. **从 legacy 搬业务 handler**：14 个 handler 按域分配
5. **proto 文件起草**：emotion-llm Analyze 接口
6. **gRPC 升级**：ai-svc → emotion-llm-service 从 HTTP 升级
7. **K8s manifests**：每个 svc 一个 deployment + service

---

## 📝 决策变更记录

| 日期 | 决策 | 旧→新 | 原因 |
|------|------|-------|------|
| 2026-07-14 | HTTP 框架 | go-zero → **Gin** | 复用 legacy，团队熟悉 |
| 2026-07-14 | 服务发现 | Nacos → **APISIX+etcd** | Nacos 假集成，没人读 |
| 2026-07-14 | 跨服务协议 | （未定）→ **gRPC+proto** | 跨语言契约标准 |
| 2026-07-14 | 鉴权位置 | svc mock → **APISIX jwt-auth** | 安全 |
| 2026-07-14 | 限流熔断位置 | （无）→ **APISIX 插件** | 网关层公共关注点 |
| 2026-07-14 | 部署形态 | docker-compose → **K8s manifests** | 生产演进 |
| 2026-08-31 | 服务入口 | APISIX :9080 → **web-bff :8894** | APISIX 3.9 bug 未修复，BFF 替代网关（Stage 30，决策 9）|
| 2026-08-31 | 服务发现 | APISIX+etcd → **静态 DNS**（compose/K8s Service） | 网关退役；注册中心无收益（决策 10）|
| 2026-08-31 | 配置中心/注册中心 | （roadmap 未勾选）→ **明确不引入** | 单实例静态寻址已够；触发条件见审计 §七（决策 10）|
| 2026-08-31 | 鉴权位置 | APISIX jwt-auth（已退役）→ **回归 BFF/svc 验签**（待修复） | 全链路 JWT 不验签 = 审计 P0 问题 S-1 |
| 2026-09-03 | 注册中心/配置中心 | 决策 10 不引入 → **演进引入 Nacos**（Stage 31） | 原判断混淆 dev 现状与设计目标；BFF 兼任网关引发 P0；改为 Stage 31 引入 |
| 2026-09-03 | API 网关 | BFF 兼任 → **独立 APISIX 网关层**（决策 11，Stage 32） | 纠正 Stage 30 "BFF 取代 APISIX" 的错误归一；网关职责回归独立层 |
| 2026-09-03 | BFF 定位 | 网关 + 聚合 → **纯聚合层**（决策 12，Stage 33） | 鉴权/CORS/限流/熔断迁出 BFF，BFF 仅做面向前端的业务编排 |
| 2026-09-03 | 演进路线 | 无明确分阶段 → **Stage 31/32/33 串行**（决策 13） | 骨架先胶水后；每 Stage 含 TDD + 收口文档 + 独立 PR |
| 2026-09-05 | 顶层目录命名 | 大小写混用 + `Emotion-Echo-LLM` 名不符实 → **统一小写 kebab**（`emotion-echo-web` / `emotion-echo-models`） | 消除前端/BFF/LLM 辨识混乱、对齐容器与文档（决策 19） |
| 2026-09-07 | 部署形态 | K8s 未来部署 → **维持单机多实例，K8s 不启用**（决策 3 收口） | 单机场景 k8s 收益≈0 且加重运维；Helm/kind 保留为学习资产「备好不部署」 |
| 2026-09-12 | Helm chart 残余 5 项 | open backlog → **冻结**（学习资产不追加投入） | 决策 3 已定 K8s 不部署；对不部署的资产追加投入收益不成立（决策 23，Stage 74） |
| 2026-09-15 | outbox sent/dead 清理 | 无清理 job → **OUTBOX_CLEANUP_ENABLED=true 启用 ticker**（Round 2.1） | Kafka D2 长期 open，触发条件 = 演示期前必做；commit `4d118f6` |
| 2026-09-15 | Kafka 契约卫生 | 漏 switch case 静默走 default → **反射枚举护栏**（Round 2.2） | D6+D8 防"加 EventType 时漏 5 处之一"；commit `6d6c3b1` |
| 2026-09-15 | DLQ 投递失败 | 仅 log 即丢弃 + 黑洞不可观测 → **counter + alertmanager 告警**（Round 2.3） | kafka-pipeline §P1-14；commit `0fbe2d0` + `deploy/prometheus/rules/kafka-dlq.yml` |
| 2026-09-15 | producer ctx 取消 | sarama SendMessage 阻塞期间 Gin 卡 10s → **goroutine + select**（Round 2.4） | P2-14；commit `caa100c`，11 测试全 PASS |
| 2026-09-15 | 多轮迭代 plan 状态 | planned → **landed**（Round 0-2 全部收口） | [stage-99-round-2-closure.md](../stages/stage-99-round-2-closure.md) 9 commits +1685/-41 行 33 测试 0 回归；Round 3-5 待启动 |
| 2026-09-20 | assessment-svc survey 端点传输方式 | gRPC 转换丢失 JSONB → **survey 5 端点走 HTTP 绕过** | [adr-2026-09-survey-http-bypass.md](adr/adr-2026-09-survey-http-bypass.md)；E2E-13 修复心理测验链路 6 处契约错位 |
| 2026-09-20 | 人格画像注入 AI 提示词的落点 | prompt 在两处硬编码 → **BFF 侧组装注入**（不改 proto / llm-service） | [adr-2026-09-personality-prompt-injection.md](adr/adr-2026-09-personality-prompt-injection.md)；E2E-14 落地 D-02；两条上游路径收敛为同一个 buildSystemPrompt |
| 2026-09-21 | 人格画像的提示词语义设计 | 注入「形容词罗列」且 34% 用户拿不到任何指令 → **给相处说明书（非 AI 性格）+ 可观察行为规格 + ipsative 相对档** | [adr-2026-09-personality-prompt-semantics.md](adr/adr-2026-09-personality-prompt-semantics.md)；账本 E2E-F-95；覆盖率 66% → 95.5%，去标识盲判 6/6 |
| 2026-09-21 | 验收断言口径 | 只断言形状（非空/正则/多处一致）→ **必须断言具体期望值 + 边界两端 + 覆盖真实 UI 驱动路径** | [adr-2026-09-assertion-value-not-shape.md](adr/adr-2026-09-assertion-value-not-shape.md)；触发 E2E-F-97（PHQ-9 算分错误连续逃过 E2E-13/14 两次收口）+ E2E-F-91（描述恒空） |

| 2026-09-22 | SenseVoice 模型服务的运行时约束与镜像分发 | 语音链路五个独立缺陷叠加（torch/funasr 漂移、VAD 请求期下载、无预热、healthcheck 假绿、runtime 缺 ffmpeg）+ 内存限额按旧 torch 估 → **torch/funasr 成对锁定 + 模型与 VAD 全烘焙 + 启动预热 + healthcheck 校验 model_loaded + runtime 含 ffmpeg + 限额按峰值 3072M + ACR 双层同 tag** | [adr-2026-09-sensevoice-runtime-constraints.md](adr/adr-2026-09-sensevoice-runtime-constraints.md)；账本 E2E-F-106/111/112；实测 /analyze 0.84s + 端到端 200 + IAB 语音气泡；**只有浏览器实测（真实 webm）才同时暴露五条** |
| 2026-09-22 | 客户端对象 URL 的下发方式 | 存储 `PublicBaseURL` 绝对地址（`http://localhost:9000/...`）→ **网关相对路径 + BFF 反代 MinIO 流式输出** | [adr-2026-09-client-object-url-bff-proxy.md](adr/adr-2026-09-client-object-url-bff-proxy.md)；账本 E2E-F-113（非宿主视角 `<audio>` 永不可达）；存量 avatar/uploads 同型债 E2E-F-116 归 E2E-27 |
| 2026-09-23 | APISIX upstream timeout（web-bff）+ BFF XTTS timeout | APISIX upstream 6 默认 60s 撞底 + BFF yaml `TimeoutMs: 30000` 与 config.go SetDefaults 90000 漂移（E2E-F-127 真 bug，yaml 不为 0 时 SetDefaults 不覆盖）→ **APISIX upstream 6 (web-bff) timeout 180s 持久化 seed.sh + BFF yaml 改 90000 + config_test.go 钉守卫** | [adr-2026-09-apisix-upstream-timeout-web-bff.md](adr/adr-2026-09-apisix-upstream-timeout-web-bff.md)；账本 E2E-F-127；Playwright 双 project 6/6 PASS；bind-mount + docker restart 是 build 撞 daemon 时的分钟级快路径 |

---

**所有文档（stage-X、roadmap、decomposition-plan）的具体实施细节以本文档为最终裁决。**

---

### 决策 32：APISIX upstream 6 (web-bff) timeout 180s + BFF XTTS timeout 90s = **✅ Accepted**（2026-09-23 E2E-17 step 5 收口）

> **真因**：v0.1.27 commit `c7ff203` 改 BFF config.go `SetDefaults` 默认 90000ms 但 yaml `TimeoutMs: 30000` 不为 0 时 SetDefaults 不覆盖 → **实际生效 30s**（E2E-F-127 真 bug）。yaml 修 90000 后，BFF→XTTS 等到 90s（90.011s 502）触发——**APISIX upstream 默认 60s read/send timeout 先 504 撞底**。

| 维度 | 选择 |
|------|------|
| BFF→XTTS http.Client.Timeout | **90000ms**（`emotion-echo-web-bff/etc/web-bff.yaml` XTTS.TimeoutMs，与 config.go SetDefaults 对齐） |
| APISIX upstream 6 (web-bff) timeout | **`send: 180, read: 180, connect: 10`**（覆盖 phonemes cold path 100s+ + LLM 流式） |
| 其它 nacos upstream (1~5) | 默认 60s 兜底（grpc 默认 5-30s 远低于此） |
| 配置位置 | `deploy/apisix/seed.sh` `put_nacos_upstream`（id=6 case 单独给 180） |
| BFF 镜像 | v0.1.28→v0.1.29 tag bump（**实际 bind-mount 改 yaml 即时生效，v0.1.29 mirror 重建未成功——docker daemon RPC 错误**） |

**❌ 废弃**：原 yaml 30000ms（v0.1.27 commit 漏改）+ APISIX upstream 6 默认 60s timeout。

**理由**（决策记录）：
- 仓 XTTS 模型 CPU 推理 4 字符 ~7s，按字符线性放大；1 字 warm path 实测 14-30s，cold path 100s+
- APISIX 默认 60s 撞底是 BFF→XTTS 90s timeout 之前先 504 的根因（双层 timeout 都需要 ≥ 100s 才能覆盖 cold path）
- bind-mount 改 yaml + docker restart 是 docker daemon 资源紧张时 build 撞错的分钟级快路径（替代重建镜像）

**关联 ADR**：[adr-2026-09-apisix-upstream-timeout-web-bff.md](adr/adr-2026-09-apisix-upstream-timeout-web-bff.md)

**账本**：[discovered-unresolved.md](../e2e-roadmap/discovered-unresolved.md) E2E-F-127 yaml/config.go 漂移

**测试**：
- `pnpm exec playwright test e2e/digital-human-tts.spec.ts` 双 project **6/6 PASS**（3.8m）
- `go test ./emotion-echo-web-bff/internal/config/` → `TestConfig_XTTSDefaultTimeoutIs90s` GREEN（钉 yaml/SetDefaults 对齐守卫）
---

### 决策 33：端侧化混合推理主方案 = **端侧优先 + 云端兜底（WebLLM + 云端 API 混合调度）**（2026-09-24 Lane O 阶段一开工立项 · D-26）

> 🟡 **proposed** —— 转 accepted 条件：v0.2 §十二 5 项决策全部由用户拍板 + 补立分项 ADR D-26.1~5（决议流程见 ADR §一；**5 项决策不在 ADR/本表决议权**，AGENTS §八 规则 4）。

| 维度 | 内容 |
|------|------|
| 技术选型 | WebLLM（WebGPU + OpenAI 兼容 + Worker 隔离）主力；MindChat 存在性 + license + A/B 验证后定（v0.2 §二） |
| 阶段切分 | 阶段一（可前置，**不依赖 §十二决策**）→ 准备期 → 阶段二（四道门）→ 阶段三 → 阶段四（v0.3 §C） |
| 单点插入口 | `useAIStreamHandler.sendAIStream`（**阶段一禁触**，阶段二改造；并行协议 §二） |
| 阶段一五任务 | WebLLM Demo / MindChat 双轨 / 编译链路+CDN / 性能基线 / golden set 骨架+云端基线 |
| 四道门 | v1.0 封版 + E2E-17~30 收口 + §十二 5 项拍板 + 本 ADR accepted → 才进阶段二/三/四 |

**两套编号说明**（本条为决策 N 系列）：同一决策在 E2E 轨索引登记为 **D-26**（`docs/e2e-roadmap/decisions.md`，E2E-17 收口时让位）；D-NN 与决策 N 是并行编号体系（先例 D-09↔决策 26、D-14↔决策 31）。

**关联 ADR**：[adr-2026-09-on-device-hybrid-main.md](adr/adr-2026-09-on-device-hybrid-main.md)

**关联计划**：[on-device-hybrid-inference-2026-09-23.md](../../plans/on-device-hybrid-inference-2026-09-23.md)（v0.2）/ [on-device-hybrid-inference-implementation-roadmap-2026-09-23.md](../../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md)（v0.3）

**并行协议**：[parallel-tracks.md](../../_meta/parallel-tracks.md)（Lane O × Lane E 隔离规则，AGENTS §八 挂钩）

**账本**：`docs/plans/on-device-findings.md`（OND-F-xx，stage1 收口并入 E2E-F）
