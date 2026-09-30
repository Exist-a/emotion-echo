---
stage: e2e-23
title: 健康检查与服务发现（/health 与 gRPC health 语义 + Nacos 注册/配置中心/热更新 + 编排健康门禁）
type: transformation
status: done
created: 2026-09-29
depends-on: []
blocks: []
gate: []
related-findings: [E2E-F-107, E2E-F-151, E2E-F-137, E2E-F-154, E2E-F-155, E2E-F-156]
---

# E2E-23 健康检查与服务发现 — 详档

> **类型**：transformation —— 编排与探针**配齐了但语义空转**：`/health` 在 4 个服务里把 `Status` 硬编码成字面量 `"ok"`（DB 挂掉也照报 ok）、BFF 探出 `degraded` 却仍返 HTTP 200（compose 的 healthcheck 只看状态码 ⇒ 语义形同虚设）、5 个 Go 服务的 gRPC health 只在启动时置一次 `SERVING` 再不翻转、观测栈 11 个服务（含网关 APISIX 自己）**零 healthcheck**、`db-migrate` 以 `Exited(1)` 结束。**本阶段把"报了健康"变成"健康时报得准、坏了时报得坏、被摘掉时真能被摘掉"。**
>
> **依据**：roadmap §第六批 E2E-23 行（"/health 与 gRPC health 语义 + Nacos 注册/配置中心/热更新"）+ 账本 **E2E-F-107**（Nacos 注册失败不重试）与 **E2E-F-151**（`db-migrate` 非零退出 + 观测栈零 healthcheck）—— 本阶段**唯一归属**的两条留账。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)（14 类已真实发生的失真）。
> **前置阶段**：无依赖前置（roadmap 排期 ✅）。上一阶段 E2E-22 ✅ done（2026-09-29，`python scripts/e2e_stage_audit.py --all` → 30 阶段 0 FAIL）。

---

## 0. 计划期实测抓到的核心事实（**先摆事实，再排计划**）

> 本节全部为本轮**亲自跑的命令 + 亲自回读的文件**，非引用历史结论。执行期若发现与本节不符，**以实测为准并回填本节**（不得让详档与事实脱钩 —— AP-02）。

| # | 事实 | 证据（命令输出 / `文件:行号`） |
|---|------|--------------------------|
| **F-a** | **4 个服务的 `/health` 把 `Status` 写死为 `"ok"`** | `emotion-echo-user-svc/internal/logic/healthlogic.go:41`、`emotion-echo-analytics-svc/internal/logic/healthlogic.go:40`、`emotion-echo-assessment-svc/internal/logic/healthlogic.go:40`、`emotion-echo-ai-svc/internal/logic/healthlogic.go:40` 均为字面量 `Status:  "ok"`，而同函数内 `DbOK: dbOK` 会如实变 false（`healthlogic.go:34-39`）⇒ **DB 挂掉时响应体自相矛盾** |
| **F-b** | **BFF 探出 `degraded` 仍返 HTTP 200** | `emotion-echo-web-bff/internal/handler/health_handler.go:102-107` 循环把 `resp.Status` 置 `degraded`，`:109` 却是 `c.JSON(http.StatusOK, resp)`。而 compose 的 healthcheck 是 `wget localhost:8894/health`（**只看状态码**）⇒ 探针形同虚设 |
| **F-c** | **全仓无任何 `/health` 检查 Redis 或 Nacos 注册状态** | 这正是账本 E2E-F-137 记录的悖论"**BFF /health 200 与网关 503 可同时成立**"的机制解释：进程活着 + DB 通 = 健康，但**它自己在 Nacos 里没有实例** = 网关找不到节点 |
| **F-d** | **5 个 Go 服务 gRPC health 只在启动置一次，永不翻转** | `grep -rn SetServingStatus` 全仓：5 处业务调用全在 grpcserver 构造函数内（`ai-svc:105-106`、`analytics-svc:83-84`、`assessment-svc:83-84`、`chat-svc:88-89`、`user-svc:113-114`），**无任何 NOT_SERVING 写入**。而公共包**已具备该能力**却没接：`emotion-echo-shared/pkg/healthcheck/server.go:145`（`Shutdown()` 置 NOT_SERVING）与 `:158`（`Resume()` 置回 SERVING）零业务调用方 |
| **F-e** | **观测栈 + 网关共 11 个服务零 healthcheck** | `grep -n healthcheck: deploy/docker-compose.infra.yml` 只有 **6 处**（`:34/:54/:93/:220/:251/:297` = postgres/redis/kafka/nacos/minio/etcd）。**缺失**：`kafka-init:128`、`skywalking-oap:149`、`skywalking-ui:175`、**`apisix:313`（全站入口）**、`prometheus:369`、`obs-mock-receiver:400`、`alertmanager:417`、`kafka-exporter:438`、`grafana:459`、`loki:484`、`promtail:497` |
| **F-f** | **`db-migrate` 无 `depends_on` + 30s 硬窗口 ⇒ 本轮实测仍 `Exited(1)`** | `deploy/docker-compose.apps.yml:47-72` 该服务**只有 `environment`/`volumes`/`entrypoint`/`networks`，无 `depends_on`**（跨文件限制见 `:609` 注释）；`deploy/db/migrate.sh:203-214` 是 `while [ "$i" -lt 30 ]` + `die "Postgres 30s 内未就绪"`。**实测**：`docker inspect emotion-echo-db-migrate --format '{{.State.ExitCode}}'` → `1`，日志尾部 `全部迁移应用完成，共 31 个文件` 紧接 `FATAL: Postgres 30s 内未就绪` ⇒ **迁移成功但进程非零退出**，E2E-F-151 当场复现 |
| **F-g** | **APISIX upstream 无主动健康检查** | `grep -c -e '"checks"' -e passes -e healthcheck deploy/apisix/seed.sh` → **0**。upstream JSON（`seed.sh:230-245`）只有 `discovery_type: nacos`，节点健康**完全依赖 Nacos 心跳**，APISIX 不会主动剔除"标 healthy 但实际拒连"的节点 |
| **F-h** | **配置热更新在 dev 全线是死代码** | `grep -rn HotReload emotion-echo-*/etc/*.yaml` → 5 个服务**注释掉**（如 `chat-api.yaml:37`）、`web-bff.yaml:29` 显式 `false` ⇒ `ListenConfig` 分支永不执行。且**即使开启，5 个服务的回调也只 log 不 apply**：`user-svc/nacos_boot.go:152-156` 拿到 ops 配置只打 `ops config loaded: %d bytes` 就丢弃，而 `web-bff/nacos_boot.go:100-110` 有真正的 `applyOps` |
| **F-i** | **ADR 有两条无实现落点的承诺** | `adr-2026-09-nacos-reintroduction.md:110` 承诺"健康探活 grpc health 5s/次，连续 3 次失败摘除"、`:111` 承诺"客户端拉取间隔 30s（env `NACOS_REFRESH_MS` 覆盖）"——`grep -rn NACOS_REFRESH_MS --include=*.go --include=*.py .` → **0 命中** |
| **F-j** | **账本 F-107 与代码事实相反（AP-04）** | 账本 `:169` 仍记 `🔴 未解决`、修法方向写"注册失败后进入后台重连循环"，描述 BFF 日志 `[nacos] boot failed (continuing)`。**实测代码**：`web-bff/main.go:150-171` 已有 dev backoff 重试（10 次 / 2s 起 / 上限 30s）且 `ShouldFailFast()` 分支直接 `os.Exit(1)`；`user-svc/main.go:127` 等 5 个服务经 `shareddiscovery.IsHardBootError` → `log.Fatalf`。**启动期"永不重试"已被修复**；残留的是**运行期掉线无重注册**（全仓无后台重连循环） |
| **F-k** | **5 个服务的 `/health` 端口未映射到宿主** | `docker ps --format '{{.Ports}}'`：仅 `web-bff 0.0.0.0:8894->8894` 与 `ai-svc 0.0.0.0:8892->8892`（且 8892 是 gRPC 口）暴露；user/chat/analytics/assessment 只有 `8887-8888/tcp` 形式的容器内暴露。**实测** `curl localhost:8888|8890|8893|8889|8891/health` → **全部 `000`**，只有 `:8894` 返 200 |
| **F-l** | **Nacos 注册基线正常（6/6）** | `curl "http://localhost:8848/nacos/v1/ns/service/list?...&namespaceId=emotion-echo-dev"` → `count: 6`，六个 `emotion-echo-*` 齐全；BFF `/health` 聚合字段 `downstream` 六项全 `ok` |

**由 F-k 推出的执行约束**：本阶段测试点**不能**依赖宿主 `curl :8888` 之类直连，必须走 ① 容器网络内探针（`docker exec` / 临时 curl 容器）② BFF 的聚合 `/health` ③ 新增的 smoke 脚本。**这是本阶段最容易踩空的地方**——E2E-22 的教训是"静态读配置不算证据"，此处更进一步：**连探针都得选对视角**。

---

## 1. 阶段目标

| # | 目标 | 现状（有证据） | 目标态 |
|---|------|--------------|--------|
| 1 | **`/health` 说真话** | 4 服务 `Status` 硬编码 `"ok"`（F-a）；BFF `degraded` 仍 200（F-b）；无 Redis / Nacos 注册检查（F-c） | 依赖挂掉 ⇒ `status` 字段如实变 `degraded`；**依赖集合补齐**（DB + Redis + **自身 Nacos 注册状态**） |
| 2 | **探针的判定信号与语义对齐** | compose healthcheck `wget /health` 只看状态码（F-b） | 引入 liveness/readiness 分离：`/health` 保 200 不破兼容（`seed.sh:148-184` 自己的探活也打它），新增 `/health/ready` 承载 200/503，**compose healthcheck 改指 ready** |
| 3 | **gRPC health 会翻转** | 5 服务启动置 `SERVING` 后永不翻转（F-d）；公共包 `Shutdown()`/`Resume()` 能力闲置 | 优雅停机时置 `NOT_SERVING`、恢复后置回 `SERVING`；唯一消费方 `ai-svc/internal/analyzer/grpc_analyzer.go:25` 的行为一并验证 |
| 4 | **Nacos 注册生命周期完整** | 启动期已 fail-fast/retry（F-j），**运行期掉线无重注册**；心跳协议不统一（BFF 用 HTTP `BeatHeartbeat`、5 服务用 SDK `Heartbeat`） | 注册丢失可自愈；心跳统一；`emotion-llm-service` 的 `NACOS_REQUIRED` 语义与 Go 侧对齐 |
| 5 | **编排层有健康门禁** | 11 个服务零 healthcheck，含**网关自己**（F-e）；`db-migrate` `Exited(1)`（F-f） | 网关 + 观测栈补 healthcheck；`db-migrate` 等 Postgres 走可重试退避，`Exited(0)` 收口 |

---

## 2. 范围与边界

### 做

#### A. `/health` 语义说真话（目标 1–2）

- **A1** 4 个服务的 `healthlogic.go` 按依赖真实计算 `Status`：任一必需依赖不通 ⇒ `degraded`。**先写红测**（当前 `TestHealthHandler_*` 全部只断言 `DbOK`，**从无一个断言 `Status`** ⇒ 缺陷从未被测试挑战过）。
- **A2** `chat-svc` 的 `EventPublisher != nil` 判定（`chat-svc/internal/logic/healthlogic.go:36`）当前**只判 nil 不真连** ⇒ 补真实探测或明确写进文档为"仅结构检查"（**二选一，不得留模糊**）。
- **A3** **新增 readiness 端点** `/health/ready`（见 §6 决策 1）：liveness 恒 200 保兼容，readiness 承载 200/503。
- **A4** 依赖集合补齐：**Redis**（全仓当前零检查）与**自身 Nacos 注册状态**（直接堵 F-c 的悖论：BFF 没注册时 `/health` 必须 `degraded`）。
- **A5** compose healthcheck 由 `/health` 改指 `/health/ready`（`docker-compose.apps.yml` 的 11 处 `healthcheck:` 逐条改），**并同步修 `seed.sh:148-184` 自己的探活**（若它用 `curl -f` 则 readiness 变严会让它提前 die —— 须先回读确认语义）。
- **A6** 补 `user-svc` / `chat-svc` 的 `/health` handler 测试（**当前两服务零测试**），并修正 `analytics-svc/internal/handler/health_handler_test.go:81` 用例名 `...ReturnsOKWithDbOKTrue` 与其"DB down"意图相反的命名漂移。

#### B. gRPC health 状态翻转（目标 3）

- **B1** 5 个 Go 服务接入 `healthcheck.Server` 已有的 `Shutdown()` / `Resume()`（`emotion-echo-shared/pkg/healthcheck/server.go:138-160`）——**能力已存在，本项只是接线**，不是造轮子。
- **B2** 验证消费侧：全仓唯一客户端封装 `emotion-echo-shared/pkg/healthcheck/client.go` 的生产调用方只有 `ai-svc/internal/analyzer/grpc_analyzer.go:25` ⇒ 确认它是否真的按状态码分流，还是只当 ping 用。
- **B3** 记录 `web-bff` **无 gRPC server**（无 `internal/grpcserver/` 目录）这一事实：APISIX 对它只能做 HTTP 探活 ⇒ 属设计现状，非缺陷，但需写进 report 避免下轮重复调查。

#### C. Nacos 注册生命周期（目标 4）

- **C1** **复核 E2E-F-107 的真实状态**：按 F-j，启动期已修。执行期须用**运行时复现**（停 Nacos → 重启 BFF → 观察是否自愈）给出结论，然后**改账本**——当前账本文本已与代码相反（AP-04）。
- **C2** **运行期重注册**：注册成功后又丢失的场景（Nacos 重启 / 网络分区）当前无任何自愈路径 ⇒ 补后台重连循环（`emotion-echo-shared/pkg/discovery` 公共层，6 服务共用）。
- **C3** **心跳协议统一**：BFF 用 `BeatHeartbeat`（HTTP `/instance/beat`），5 服务用 SDK `Heartbeat` ⇒ 统一并说明取舍。
- **C4** `emotion-llm-service` 的 `NACOS_REQUIRED` 语义对齐：`main.py:87-94` 默认不 raise（dev 继续、prod fail-fast）⇒ 明确 prod compose 是否真的设了该变量；**若没设 = prod 静默降级**，是真缺陷。
- **C5** Nacos 服务实例数变化 ⇒ APISIX 侧 upstream 节点是否**自动跟随**（`deploy/apisix/config.yaml:202-212` 的 `fetch_interval: 30`）——这条同时决定 RUNBOOK §2.4"重建服务后必须重跑 seed"的说法是否成立（见 D2）。

#### D. 编排健康门禁（目标 5）

- **D1** **网关 APISIX 补 healthcheck**（`docker-compose.infra.yml:313`）——全站入口自己不可观测，属最高优先级缺口之一。
- **D2** 观测栈 6 服务（`prometheus:369` / `alertmanager:417` / `grafana:459` / `loki:484` / `promtail:497` / `kafka-exporter:438`）+ `skywalking-oap:149` / `skywalking-ui:175` / `obs-mock-receiver:400` / `kafka-init:128` 补 healthcheck。**注意**：`kafka-init` / `minio-init` 是一次性任务，healthcheck 对它们无意义 ⇒ 排除，只补常驻服务。
- **D3** **`db-migrate` 修 F-151**：`migrate.sh:203-214` 的 30 次 × 1s 硬窗口改**指数退避 + 更长上限**（并给出与 `scripts/dev-up.sh:79-80` 批 1 给 60s / 批 2 给 30s、却等错对象 `postgres` 的关系）。目标态：冷启动下 `Exited(0)`。
- **D4** 补 `scripts/dev-up.sh` 的 `wait_*` 编排语义（当前批 2 阶段等 `postgres` 疑似 copy-paste 错误）。

#### E. 配置中心与热更新（目标 4 附带）—— **E1/E2 已由用户拍板（2026-09-29），范围见 §2.4**

> 用户原话「我觉得还是加上比较好吧，这样服务是完整的」+「我并不知道选什么比较好」⇒ 授权执行者按盘点事实定范围。
> **盘点结论与"五个服务都加上"的预期不同**，如实记录（AP-06 精神：先摆事实再定方案）。

### 2.4 配置热更的实测盘点结果（**E1/E2 的事实依据**）

| 服务 | 候选数 | 依据 |
|------|-------|------|
| `chat-svc` | **4** | `Outbox.MaxAttempts`(100) / `SentRetentionDays`(7) / `DeadRetentionDays`(30) / `CleanupIntervalS`(3600)，权威来源 `internal/config/config.go:113-127`；`Outbox.MaxAttempts` 消费点**已每轮现读**（`internal/outbox/relay.go:99,106`）⇒ 改造最轻 |
| `ai-svc` | **9** | `LLM.Timeout`(3) / `FER.Timeout`(10) / `SenseVoice.Timeout`(30) / `XTTS.Timeout`(60) / `XTTS.Language`(`zh-cn`) / `XTTS.Speed`(0.75) / `Kafka.MaxRetries`(3) / 熔断 `FailThreshold`(5) / `OpenSeconds`(30)。`XTTS.Language`/`Speed` 已**每请求现读**（`internal/logic/synthesizespeechlogic.go:51,57`）但受 `svcCtx.Config` 值拷贝阻断；`CircuitBreaker` 自带 `sync.Mutex`（`internal/fusion/llm_breaker.go:57`） |
| `analytics-svc` | **1** | `Kafka.MaxRetries`(3) |
| `user-svc` | **0** | `internal/config/config.go:55-87` 全文回读：仅 `Name`/`Host`/`Port`/`SkyWalking.OAPAddr`/`Postgres.MaxOpenConns`/`MaxIdleConns`/`Nacos.*`/`GRPC.*` —— **全为连接类或监听地址类**，无一个超时/批量/阈值/开关类字段 |
| `assessment-svc` | **0** | `internal/config/config.go:49-83` 全文回读，与 user-svc 同构 |

**用户采纳的方案**：给有真参数的 3 个服务（chat/ai/analytics）接上热更，共 **14 个参数**，全部**从各自 yaml 已有配置项取，不新编业务概念**；`user-svc` / `assessment-svc` **不硬造参数**（保留代码现状 + 账本记账说明"无运营参数需求"）。

**不做的**（如实记录理由，防日后当成遗漏）：
- `analytics-svc` `TriggerQueueCap` —— Go channel 的 `cap` 创建后不可变，真要热更需重写队列实现（`internal/trigger/trigger_queue.go:55`）⇒ 列为 P2 不做。
- fusion worker tick / relay `interval`+`batchSize` —— 当前是**硬编码字面量**（`ai-svc/main.go:522`、`chat-svc/main.go:225`），属**新增配置项**而非暴露既有字段，超出"接线"范围。

### 2.5 盘点顺带挖出的三个前置缺陷（**接线前必须先修，否则"推了不生效"**）

| # | 缺陷 | 证据 | 为什么必须先修 |
|---|------|------|---------------|
| **P1** | **敏感字段保护对本方案形同虚设** | `emotion-echo-shared/pkg/configcenter/nacos_config.go:39-71` 的 `sensitivePrefixes`（`jwt.`/`database.`/`kafka.`/`llm.` 等）**只拦 dataId**（`isSensitiveDataId` 仅在 `PublishConfig` 路径调用，`:191`），而 ops 是**单 dataId 打包全部运营参数** ⇒ 在 `emotion-echo-ai-svc.ops.yaml` 里写 `llm.api_key: xxx` 一路畅通 | 新 ops 解析**必须用固定 struct 反序列化**（未知 key 自然忽略）；**禁止** `map[string]any` 反射式写入或关闭 `KnownFields` 的宽松模式 |
| **P2** | **`ai-svc` 的 `LLM.Timeout` 是死配置** | `main.go:487-491` 构造 `NewLLMFuser` 只传 `BaseURL`/`APIKey`/`Model`（且走 `os.Getenv` 非 `c`），**未传 Timeout**；`internal/fusion/llm_fuser.go:64-67` 因 `cfg.Timeout <= 0` 永远回落内置 3s | 把它列进热更前必须先修接线，否则又造一个"配了不生效"的假能力（**正是本组测试点要抓的形态**） |
| **P3** | **`analytics-svc` 的 `SetDefaults` 漏了 `Kafka.MaxRetries`** | `internal/config/config.go:28` 注释承诺"默认 3"，但 `SetDefaults`（`:69-114`）**无该分支**；实际值来自 `internal/kafka/consumer.go:69,71` 硬编码。而 `main.go:200` 有 `>0` 守卫 ⇒ **推 0（想关重试）会被静默忽略** | 补默认值 + 明确"0 是否合法"的语义，否则 ops 推 0 无效且无人察觉 |

> 三条均**新登账本**（E2E-F-158 / F-159 / F-160），避免只在 plan 里存在（AP-07）。


### 不做（边界）

| 不做的事 | 理由 | 归属 |
|---------|------|------|
| **APISIX upstream 主动健康检查**（`checks` 段，F-g） | 属网关侧能力，且账本 F-137 已明确"根因归 E2E-25" | **E2E-25**（本阶段只把缺口记清并交接口，见 §2.3） |
| **DLQ 死信堆积 24 条**（E2E-F-150） | 消息链路能力 | **E2E-24** |
| **Prometheus 自身死亡告警**（E2E-F-152） | 需引入外部监控服务，非本阶段能力 | 运维/部署轮（账本已裁定） |
| **dev ↔ k8s 观测栈版本对齐**（E2E-F-153） | 决策 23 冻结期，形态差异长期存在 | 运维/部署轮（待决策） |
| **全链路业务功能测试** | 本阶段只测健康与发现语义本身 | 各业务阶段 |
| **prod 环境实测** | 本机无 prod 环境，dev 覆盖项差异见 RUNBOOK §2.4 | report 中显式声明 |
| **限流 / 鉴权插件语义** | 属 E2E-25 | E2E-25 |

### 2.3 本阶段**无法闭合**的缺口（显式记录，防日后被当成遗漏）

1. **进程外告警**（F-152）：Prometheus 死 ⇒ 无人发告警。彻底解需 deadman's switch / 托管云监控，**本机无法自举** ⇒ 账本已裁定归运维轮，本阶段不动。
2. **APISIX 主动健康检查**（F-g）：只交接口给 E2E-25，本阶段**不假装闭合**。
3. **prod 语义**：Nacos 集群模式 / 多实例 / 真实网络分区的自愈行为，dev 单机无法等价验证 ⇒ 相关测试点若降 `N/A` **必须写明是"prod 语义、dev 不可验证"**，不允许裸标（RUNBOOK §4.2）。

---

## 3. 前置条件

### 3.1 开工前置（RUNBOOK §1 三项）

| 检查项 | 状态 |
|--------|------|
| `depends-on` 全部 `done` | ✅ 本阶段 `depends-on: []` |
| 不在 §9 决策门阻塞列表 | ✅ §9 仅 D-04 i18n，不阻塞任何阶段 |
| `plan.md` 存在且前置满足 | ✅ 本文件；§3.2 探针**执行期首跑** |
| 阶段内决策已拍板 | ✅ D-29 / D-31 / E1-E2 已于 2026-09-29 由用户拍板（§6），**无阻塞开工的待决项** |

### 3.2 开工前必须先跑的可行性探针（**RED 前置**）

> 依据 RUNBOOK §4.1：**探针必须真跑**，且**先自检探针本身有效**（对照 memory「证据无效的三类模式」：容器无 `curl` 的 exec 报错被当超时、端口未映射的 host 测量）。

```bash
# 探针 1：Nacos 注册齐全（期望 count:6）—— RUNBOOK §2.1 已有，此处复跑确认基线
curl -s "http://localhost:8848/nacos/v1/ns/service/list?pageNo=1&pageSize=50&namespaceId=emotion-echo-dev" | head -c 400

# 探针 2：5 个 Go 服务的 /health 在**容器网络内**可达（宿主直连预期 000，见 F-k）
docker exec emotion-echo-nacos wget -qO- --timeout=5 http://emotion-echo-user-svc:8888/health
# 探针 2 前置自检：确认目标容器真有 wget/curl，否则报错会被误读成"服务不可达"
docker exec emotion-echo-nacos sh -c 'command -v wget || command -v curl || echo NO_PROBE_BINARY'

# 探针 3：BFF 聚合 /health（宿主可达，唯一一个）
curl -s http://localhost:8894/health | head -c 400    # 期望 downstream 六项全 ok

# 探针 4：db-migrate 退出码（当前预期 1，F-f；修后预期 0）
docker inspect emotion-echo-db-migrate --format '{{.State.ExitCode}}'

# 探针 5：APISIX 自身无 healthcheck 的运行时佐证（F-e）
docker inspect emotion-echo-apisix --format '{{json .Config.Healthcheck}}'   # 期望 null

# 探针 6：gRPC health 当前状态（需 grpc_health_probe 或复用 shared 客户端写个小程序）
#   —— 若无现成工具，**先记 BLOCKED 并说明**，不得用"代码里写了 SetServingStatus"充数（AP-01）
```

> **探针 2 是本阶段最关键的一条**：F-k 已证明宿主直连全 000，若不先解决"从哪个视角探测"，后续 10 余个测试点会集体产出无效证据。

---

## 4. 测试点清单

> 判定分级按 RUNBOOK §4：`[A]` 自动可判（退出码 / 状态码 / 字段值 / DB 结果）、`[V]` 需截图且**截图必须被查看**、`[M]` 需人工裁定**必须升级给用户**。
> **每个测试点在收口时必须有 report 结论**（A3 审计断言 plan 编号 ⊆ report 编号）。

### A 组 — `/health` 语义（先证伪，再修）

| # | 测试点 | 判定 | 计划期预期 |
|---|--------|------|-----------|
| 1 | 6 个服务 `/health` 可达且响应 schema 一致（含 BFF 聚合六下游） | `[A]` | 部分 FAIL（F-k 视角问题） |
| 2 | **停 Postgres ⇒ user-svc `/health` 的 `status` 字段变化** | `[A]` | **必 FAIL**（F-a 硬编码 `"ok"`） |
| 3 | 同上验证 analytics / assessment / ai 三服务 | `[A]` | **必 FAIL**（F-a） |
| 4 | **BFF 下游全挂 ⇒ `/health` 的 HTTP 码与 `status` 字段** | `[A]` | **必 FAIL**（F-b：degraded 却 200） |
| 5 | **`/health` 覆盖 Redis 依赖**（当前全仓零检查） | `[A]` | **必 FAIL** |
| 6 | **BFF 未注册到 Nacos ⇒ `/health` 报 degraded**（直接堵 F-c 悖论） | `[A]` | **必 FAIL** |
| 7 | chat-svc 的 `EventPublisher` 判定是真连还是仅判 nil（A2 二选一已定则断言） | `[A]` | 待判定 |
| 8 | `/health/ready` 存在且依赖不通时返 503（A3） | `[A]` | FAIL（端点不存在） |
| 9 | compose healthcheck 已指向 `/health/ready` 且 11 个服务探针语义正确 | `[A]` | FAIL |
| 10 | `seed.sh` 自带探活在 readiness 变严后行为正确（不提前 die / 不掩盖） | `[A]` | 待判定 |
| 11 | `user-svc` / `chat-svc` 补齐的 `/health` handler 测试跑绿 | `[A]` | FAIL（当前零测试） |

### B 组 — gRPC health

| # | 测试点 | 判定 | 计划期预期 |
|---|--------|------|-----------|
| 12 | 5 个服务 gRPC health 注册的 service 名正确（`""` + `emotion.X`） | `[A]` | PASS（基线） |
| 13 | **优雅停机期间 gRPC health 翻 `NOT_SERVING`** | `[A]` | **必 FAIL**（F-d） |
| 14 | 恢复后翻回 `SERVING`（`Resume()` 已存在于 `server.go:158`） | `[A]` | **必 FAIL**（未接线） |
| 15 | `ai-svc/internal/analyzer/grpc_analyzer.go:25` 客户端按状态分流（非仅当 ping） | `[A]` | 待判定 |
| 16 | `web-bff` 无 gRPC server 属设计现状（写入 report，避免下轮重复调查） | `[A]` | N/A（陈述性） |

### C 组 — Nacos 注册生命周期

| # | 测试点 | 判定 | 计划期预期 |
|---|--------|------|-----------|
| 17 | 6 服务注册齐全（`count:6`） | `[A]` | PASS（F-l 基线） |
| 18 | **停 Nacos ⇒ 重启 BFF ⇒ 是否自愈注册**（F-107 运行时复现） | `[A]` | PASS（代码已修），**但账本文本需改** |
| 19 | 同上验证 5 个 Go 服务的 fail-fast 退出码 | `[A]` | 待判定 |
| 20 | **运行期 Nacos 掉线 ⇒ 实例丢失后能否自愈重注册** | `[A]` | **必 FAIL**（无后台重连） |
| 21 | 心跳协议统一性（BFF `BeatHeartbeat` vs 5 服务 SDK `Heartbeat`） | `[A]` | **必 FAIL**（不统一） |
| 22 | `emotion-llm-service` prod 是否真设 `NACOS_REQUIRED`（不设 = 静默降级） | `[A]` | 待判定 |
| 23 | **Nacos 重启 ⇒ APISIX upstream 节点自动跟随**（`fetch_interval: 30`） | `[A]` | 待判定（**决定 RUNBOOK §2.4 说法是否成立**） |
| 24 | Nacos 配置中心 `GetConfig` 首帧失败时服务能否继续 | `[A]` | 待判定 |

### D 组 — 编排健康门禁

| # | 测试点 | 判定 | 计划期预期 |
|---|--------|------|-----------|
| 25 | **APISIX 补 healthcheck 后可被正确判定**（D1） | `[A]` | FAIL（当前 null） |
| 26 | 观测栈 + skywalking 补 healthcheck 后状态正确 | `[A]` | FAIL（F-e） |
| 27 | **`db-migrate` 冷启动 `Exited(0)`**（F-151 目标态） | `[A]` | **必 FAIL**（本轮实测仍 `1`，F-f） |
| 28 | `migrate.sh` 退避逻辑有**负向测试**（AP-13：脚本必须有负向验证） | `[A]` | 待判定 |
| 29 | `dev-up.sh` 的 `wait_*` 阶段编排语义正确（D4） | `[A]` | 待判定 |

### E 组 — 配置中心（**已拍板接 14 个参数，E 组不再 BLOCKED**）

| # | 测试点 | 判定 | 计划期预期 |
|---|--------|------|-----------|
| 30 | chat-svc 4 个 Outbox 参数可热更（推 Nacos → 行为变化，**非"调用成功"**） | `[A]` | FAIL（当前无消费点接入） |
| 31 | ai-svc 9 个参数可热更（超时 / 语种 / 语速 / 熔断） | `[A]` | FAIL（当前无消费点接入） |
| 32 | analytics-svc `Kafka.MaxRetries` 可热更 | `[A]` | FAIL |
| 33 | **P1 敏感字段保护**：ops.yaml 写 `llm.api_key` **不生效**（负向断言） | `[A]` | 必 FAIL（当前无保护） |
| 34 | **P2 `LLM.Timeout` 接线**：不修 `main.go:487-491` 则 yaml 配 3 也不生效 | `[A]` | **必 FAIL**（死配置实测） |
| 35 | **P3 `analytics` 重试默认值**：`SetDefaults` 补齐后 `max_retries=0` 的语义明确 | `[A]` | 必 FAIL（当前漏设） |
| 36 | `user-svc` / `assessment-svc` 零候选**如实记账**（不改代码、不硬造参数） | `[A]` | PASS（记账动作） |

### F 组 — 回归钉与文档

| # | 测试点 | 判定 | 计划期预期 |
|---|--------|------|-----------|
| 37 | 新增 `scripts/smoke_health_discovery.py`：探 6 个 `/health` + `/health/ready` + Nacos `count:6` + gRPC health 翻转，**可 CI 执行** | `[A]` | 新建 |
| 38 | 新增 Playwright spec `emotion-echo-web/e2e/health-discovery.spec.ts`：**经网关**验证业务端点 200（证明 discovery 在消费侧真的通），并断言 BFF `/health` 的 `downstream` 字段 | `[A]`+`[V]` | 新建，1 张截图 |
| 39 | **APISIX Admin 页面可见 6 个 nacos upstream 且节点非空** | `[V]` | 截图**必须被查看**（A7 审计） |
| 40 | 文档漂移修正：RUNBOOK §2.4"重建服务后必须重跑 seed" / ADR `:110-111` 两条无落点承诺 / 账本 F-107 文本 | `[A]` | 收口前必做 |

**汇总（2026-09-29 收口轮更新）**：**40 个测试点 → PASS 34 / FAIL 0 / BLOCKED 4 / N/A 2**。
计划期预期的 16 个 FAIL 全部修复并有运行时证据（含破坏性实验轮的 #18/19/20/21/23）。详结论见 [report.md](report.md) §2。
**剩余 4 项 BLOCKED**：#33（P1 负向断言的形式化测试，已由 ops_sanitize_test.go 覆盖主体）/ #34~#35（P2/P3 已修，但测试点编号的专项断言未单独建）/ #39 之外的截图类 —— 逐条见 report §2。
注：本表的"计划期预期"列保留原始判断，用于对照"实测是否推翻预判"（如 #20 的"必 FAIL"被实测推翻）。

> ⚠️ **AP-03 自查**：原 E 组 2 个 `[M]`（"接线还是删除"）已于 2026-09-29 由用户拍板，**不再是 BLOCKED 风险**。但新增的 P1/P2/P3 三条前置缺陷若在执行期只修一半（例如修了 P2 却漏 P1 的负向断言），**必须在 report 逐条给出结论**——RUNBOOK §4.2：需求被放弃 = `BLOCKED` + 用户批准 + 账本记 `🟡 降级并记录`，**不得写"待决策"了事**。

---

## 5. 验收标准（DoD）

| # | 标准 | 校验方式 |
|---|------|---------|
| 1 | 35 个测试点在 report 中**逐条有结论**（A3 审计：plan 编号 ⊆ report 编号） | `python scripts/e2e_stage_audit.py --stage e2e-23` |
| 2 | 计划期预期的 10 个 FAIL **要么修绿、要么转阶段、要么经批准降级**（AP-07） | report §修复清单逐条对应 |
| 3 | **TDD 痕迹**：每个修复有先行的失败测试；`go vet ./...` 通过（**不用 `go build` 代替**，AP-09） | `go vet ./...` + commit 序列 |
| 4 | 回归钉跑过且绿：smoke 脚本 + Playwright spec | `python scripts/smoke_health_discovery.py` / `pnpm playwright test e2e/health-discovery.spec.ts` |
| 5 | `[V]` 点有截图且**已被查看**（A7） | `screenshots/` 非空 |
| 6 | **账本对账**（RUNBOOK §7 #9）：F-107 / F-151 / F-154~156 状态与本阶段 `status` 无冲突 | A5 审计 |
| 7 | 证据列**无存在性措辞**，每条给 `文件:行号` 或命令输出（A4 / AP-01 / AP-02） | A4 审计 |
| 8 | 架构级改动（健康语义契约、readiness 端点）有 **ADR + `decisions.md` 登记**（AP-08） | `bash scripts/check_adr_gate.sh` |
| 9 | 第二方核对（§13.3 十七条） | **执行者不得自行宣布 `done`** |

### 5.1 回归钉形态说明

本阶段以**后端/编排**为主，故回归钉分三层：
1. **Go 单测**（主力）：`/health` 语义、gRPC health 翻转 —— 覆盖率下限按 AGENTS §2.3 核心包 80%。
2. **`scripts/smoke_health_discovery.py`**：编排级事实（Nacos count / readiness 码 / db-migrate 退出码），可接 CI（注意 AP-11：**须实测红线被拦**，否则只标"仅报告"）。
3. **Playwright spec**（薄层 1 个）：**从消费侧**证明服务发现真的通 —— 走网关打业务端点。这一点是 browser 层才能证实的（对照 memory「IAB 才能抓到的东西」：curl 通 ≠ 前端通）。

---

## 6. 待决策（**不阻塞开工**，按推荐方案先走；需用户拍板的已标注）

### 决策 1（**D-29，✅ 用户 2026-09-29 拍板**）：`/health` 失败时的 HTTP 状态码契约 = **liveness / readiness 分离**

| 方案 | 做法 | 影响 |
|------|------|------|
| **A（✅ 已采纳）** | **liveness / readiness 分离**：`/health` 恒 200（保兼容）+ 新增 `/health/ready` 承载 200/503，compose healthcheck 改指 ready | `seed.sh:148-184` 自带探活、现有 smoke、E2E-22 遗留引用全部不受影响；改动面 = 6 服务各加一路由 + compose 11 处改一行 |
| ~~B~~ | ~~直接让 `/health` 返 503~~ | ❌ **连锁风险**：`apisix-seed` 对 6 个服务有 `condition: service_healthy`（`docker-compose.apps.yml:722-735`），下游一降级 → seed 永不运行 → **网关连路由都没有**，故障面反而扩大 |

**用户裁定 A 的理由**：方案 B 会把"某个下游降级"放大成"整站无路由"。分离开后 liveness 恒 200 保住存量消费方，readiness 承载真正的依赖判定，compose 门禁改指 readiness。

**落地要求（新增，A3/A4/A5）**：
1. `/health` 语义**不变**（恒 200），但内部计算补齐：任一必需依赖不通时 `status` 字段返回 `degraded`（不静默）。
2. `/health/ready` = 同一套依赖检查，**依赖不通返 HTTP 503**。
3. compose 的 11 处 `healthcheck:` 改指 `/health/ready`。
4. **执行期必须实测**：`apisix-seed` 的 `depends_on: service_healthy` 语义随之变严（ready 才算 healthy）——须验证冷启动全栈不会因此死锁（plan 风险 R3）。

### 决策 2（**D-30，✅ 2026-09-29 实测落定**）：RUNBOOK §2.4"重建服务后必须重跑 `apisix-seed`"**判为误导性文档，已更正**

F-g 的调查指向：**节点由 APISIX 内置 discovery 插件每 30s 拉取**（`deploy/apisix/config.yaml:202-212`），`seed.sh` 只写 upstream **定义**、不含 `nodes`（`seed.sh:230-245`）⇒ **节点变化应自动跟随，重跑 seed 非必需**。
但该结论**尚未运行时验证**（测试点 #23 就是干这个的）。若 #23 成立 ⇒ 这条写进 RUNBOOK §2.4 已 N 轮的"运维铁律"是**误导性文档**，须更正（AP-02 反向：文档说 A、代码是 B）。

### 决策 3（**D-31，✅ 用户 2026-09-29 拍板**）：`emotion-llm-service` 的 `NACOS_REQUIRED` = **编排层显式声明，不改代码默认值**

现状 dev 默认继续、prod 靠环境变量 fail-fast（`main.py:87-94`）⇒ **若 prod compose 未真设该变量，prod 就在静默降级**（HTTP 可达但 Nacos 无实例 ⇒ BFF Resolve 失败 ⇒ 502，与 F-137 同型）。

**用户裁定**：保持 `main.py` 代码默认值不变，在 **prod 编排显式设 `NACOS_REQUIRED=1`**、dev 显式设 `NACOS_REQUIRED=0` —— 意图显式化，dev 体验不变，且不改动 Python 侧默认行为。

**落地要求（C4）**：
1. `deploy/docker-compose.infra.yml` / `docker-compose.apps.yml` 的 `emotion-llm-service` 段**显式写** `NACOS_REQUIRED`（dev=0、prod profile=1），不留隐式默认。
2. 若仓内存在 prod 编排文件，同步补上。
3. 回归钉：新增契约测试断言"compose 里必须显式出现 `NACOS_REQUIRED`"（防将来又退回隐式默认）。

### 决策 4（可自决，不阻塞）：APISIX 主动健康检查（`checks` 段）本阶段只交接口

账本 F-137 已裁定根因归 E2E-25 ⇒ 本阶段**记录缺口 + 写清对 E2E-25 的接口期望**（"upstream 需能剔除标 healthy 但拒连的节点"），**不实现**。

---

## 7. 已知风险

| # | 风险 | 缓解 |
|---|------|------|
| R1 | **探针视角错误导致集体无效证据**（F-k 已证宿主直连全 000） | §3.2 探针 2 先跑并**自检探针二进制存在**；测试点 1 未过不进入后续 |
| R2 | 停 Postgres / 停 Nacos 属**破坏性环境操作**，可能连带影响其他阶段留账复现 | 全部安排在**计划期已确认基线**之后；每次停起后按 RUNBOOK §2.1 复查 `count:6`；不用 `docker compose down`（保留其它容器） |
| R3 | 改 compose healthcheck 指向 readiness 后，**启动编排可能死锁**（readiness 依赖链） | 先在单个服务试点；`dev-up.sh` 有 `wait_*` 编排，改完必须**冷启动全栈验证一次**（不只是 `docker restart`） |
| R4 | gRPC health 翻转可能影响既有客户端（`ai-svc` 的分析器） | 先读 `grpc_analyzer.go:25` 的分流逻辑再改；负向测试覆盖"下游 NOT_SERVING 时客户端行为" |
| R5 | 本机 dev 资源受限（D 盘 651GB / `.wslconfig` 8GB，memory 记录过 Docker 三次冻结） | 避免并发重建多镜像；单实例 CPU 服务（XTTS 等）不动 |
| R6 | 35 个测试点规模偏大，`BLOCKED > 1/3` 则不得判 done | 优先保证 A 组（语义说真话）+ D 组（编排门禁）两组闭环；C 组/E 组可降级并说明 |

---

## 8. 产出物

| 类型 | 文件 | 说明 |
|------|------|------|
| 详档 | `stages/e2e-23-health-discovery/plan.md` | 本文件 |
| 记录 | `stages/e2e-23-health-discovery/report.md` | 收口时按 RUNBOOK §10 模板 |
| 证据 | `stages/e2e-23-health-discovery/screenshots/` | `[V]` 点（#34 APISIX Admin） |
| 实现 | 各服务 `internal/logic/healthlogic.go` / `internal/handler/health_*` | A 组 |
| 实现 | 5 个 `internal/grpcserver/server.go` | B 组 |
| 实现 | `emotion-echo-shared/pkg/discovery/` | C 组（运行期重连） |
| 配置 | `deploy/docker-compose.infra.yml` / `.apps.yml` | D 组 |
| 脚本 | `deploy/db/migrate.sh` + 负向测试 | D 组 |
| 工具 | `scripts/smoke_health_discovery.py` | F 组 |
| 回归钉 | `emotion-echo-web/e2e/health-discovery.spec.ts` | F 组 |
| 决策 | `docs/architecture/adr/adr-2026-09-health-check-contract.md`（**待 D-29 拍板后建**） | AP-08 |

---

## 9. 调研依据（AGENTS §〇 "文档撰写前必须做的功课"）

### 9.1 亲自回读的文件（Read / sed / grep，非转述）

**健康检查**
- `emotion-echo-user-svc/internal/logic/healthlogic.go`（**Read 全文**）—— F-a 的直接证据：`:32-46` 全函数，`:41` 字面量 `"ok"`、`:34-39` `DbOK` 真实计算
- `emotion-echo-web-bff/internal/handler/health_handler.go:95-112` —— F-b
- `grep -rn SetServingStatus`（全仓）—— F-d
- `emotion-echo-shared/pkg/healthcheck/server.go:138-162` —— F-d 的"能力已存在"证据（`Shutdown()` / `Resume()`）
- `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go:25` —— 唯一客户端消费方（B2）

**Nacos**
- `emotion-echo-web-bff/main.go:137-175` —— F-j（BFF 的 dev backoff 重试 + prod fail-fast）
- `emotion-echo-user-svc/main.go:120-136` —— F-j（`IsHardBootError` → `log.Fatalf`）
- `emotion-echo-shared/pkg/discovery/failfast.go`（**Read 全文**）—— hard/soft 分类器，4 个 hard 前缀
- `emotion-llm-service/main.py:84-96` —— F-j / C4（`NACOS_REQUIRED` 默认继续）
- `emotion-echo-user-svc/nacos_boot.go:145-165` vs `emotion-echo-web-bff/nacos_boot.go:98-120` —— F-h（只 log vs 真 apply）
- `grep -rn HotReload emotion-echo-*/etc/*.yaml` —— F-h 全线 false/省略

**编排**
- `deploy/docker-compose.infra.yml`：`grep -n healthcheck:`（6 处）+ 服务名清单（20 个）—— F-e
- `deploy/docker-compose.apps.yml:47-72`（`db-migrate` 无 `depends_on`）、`:718-735`（`apisix-seed` 对 6 服务 `condition: service_healthy`）、`:609` 注释（跨文件 `depends_on` 限制）
- `deploy/db/migrate.sh:203-214` —— F-f
- `deploy/apisix/seed.sh`：`grep -c -e '"checks"' -e passes -e healthcheck` → 0 —— F-g
- `deploy/apisix/config.yaml:202-212`（`fetch_interval: 30`）—— D2 的依据

**文档**
- `docs/architecture/adr/adr-2026-09-nacos-reintroduction.md:105-112` —— F-i 两条无落点承诺
- `docs/e2e-roadmap/discovered-unresolved.md:169`（F-107）、`:221`（F-137）、`:255`（F-151）—— 归属与状态
- `docs/e2e-roadmap/roadmap.md` 第六批表格 —— E2E-23 排期与边界
- `docs/e2e-roadmap/decisions.md` 决策索引 —— **grep"健康/health/nacos/服务发现"0 命中 ⇒ 决策空白区**（本阶段需立 D-29~31）

### 9.2 已查的 ADR / 既有决策

| 文档 | 查了什么 | 结论 |
|------|---------|------|
| `adr-2026-09-nacos-reintroduction.md` §四 `:101-111` 命名与健康约定 | 健康探活 / 拉取间隔是否落地 | **未落地**（F-i）⇒ 记入 §2.3 与测试点 #35 |
| 同上 §六 `:126-136` 风险缓解 | 掉线重连是否被覆盖 | 只覆盖"启动前等待"（`WaitForNacos` 退避），**未覆盖"注册后掉线重连"** ⇒ C2 的缺口依据 |
| `docs/e2e-roadmap/decisions.md` | 有无健康/发现相关决议 | **无** ⇒ 本阶段立 D-29 / D-30 / D-31 |
| RUNBOOK §2.1 / §2.4 | 环境铁律是否与代码一致 | §2.4"必须重跑 seed"待 #23 验证（D2）；§2.1 期望 `count:6` 本轮实测**成立** |
| memory「第二方核对用子代理」 | 本阶段的核对方式 | 沿用：子代理按 §13.3 十七条独立复跑，任务书须明写"自证不可信 + 独立跑" |

### 9.3 计划期实测探针（本轮实跑，非引用）

| 探针 | 命令 | 实际输出 |
|------|------|---------|
| 审计器 | `python scripts/e2e_stage_audit.py --all` | `合计：30 个阶段，0 个存在 FAIL`；E2E-23 当前 `roadmap 状态: pending` |
| Nacos 基线 | `curl .../ns/service/list?namespaceId=emotion-echo-dev` | `count: 6`（chat/ai/assessment/analytics/web-bff/user 齐全） |
| `/health` 宿主直连 | `curl :8888/:8890/:8893/:8889/:8891/health` | **全部 `000`**；仅 `:8894` → 200，`downstream` 六项全 `ok`（F-k） |
| BFF `/health` | `curl -s localhost:8894/health` | `{"status":"ok","version":"dev-build","build_time":"2026-09-29T03:20:22Z","downstream":{...六项 ok}}` |
| `db-migrate` | `docker inspect ... '{{.State.ExitCode}}'` | **`1`**（finished `2026-09-29T03:17:36Z`） |
| `db-migrate` 日志 | `docker logs ... \| tail -6` | `全部迁移应用完成，共 31 个文件` → `FATAL: Postgres 30s 内未就绪`（**F-151 当场复现**） |
| infra healthcheck | `grep -n healthcheck: deploy/docker-compose.infra.yml` | 仅 6 处 ⇒ 11 常驻服务缺失（含 `apisix:313`） |
| APISIX checks | `grep -c -e '"checks"' -e passes -e healthcheck deploy/apisix/seed.sh` | `0` |
| `NACOS_REFRESH_MS` | `grep -rn --include=*.go --include=*.py` | 0 命中 |
| 端口暴露 | `docker ps --format '{{.Ports}}'` | 仅 `web-bff:8894`、`ai-svc:8892` 对宿主暴露 |
| 容器栈 | `docker ps` | 28 容器 running；`deploy/.devmode-session` **不存在**（无 Lane O 占位） |
| 账本水位 | `grep -oE "E2E-F-[0-9]{3}" \| sort -u \| tail` | 最大 **F-153** ⇒ 新登从 **F-154** 起 |

### 9.4 引用的既有结论（来自账本 / 历史，非本轮新发现）

- **E2E-F-107** 的"启动期不重试"已被代码修复（F-j）——本阶段**运行时复现后改账本**，不是新发现。
- **E2E-F-151** 的两个断言本轮**均已复核属实**（F-e 六观测服务零 healthcheck —— 实际扩大到 11 个常驻服务含 APISIX；F-f `Exited(1)` 当场复现）。
- **E2E-F-137** 归属 E2E-25，本阶段只交接口。
- **memory「E2E-22 监控告警 session」**：观测栈曾于 `2026-09-29T03:12:40Z` 六容器同刻 `Exited(255)` —— 与 F-e（无 healthcheck ⇒ 无从判定死活、无人感知）**互为因果**，本阶段 D 组是其编排侧根因之一。

### 9.5 尚未验证、需执行期证实的假设（列入 plan 假设清单）

| # | 假设 | 若不成立的后果 |
|---|------|--------------|
| H1 | 探针 2 的容器网络内 `wget/curl` 可用 | 11 个测试点的证据链全废 ⇒ 须先换探测手段 |
| H2 | `seed.sh:148-184` 探活用的是 HTTP 状态码而非仅连通性 | 决策 1 方案 A 也可能连锁 ⇒ 需重议 |
| H3 | APISIX 节点确由 `fetch_interval: 30` 自动跟随 | RUNBOOK §2.4 铁律成立 ⇒ 决策 2 反向，仍需在 report 说明"为何不能省" |
| H4 | 5 个 Go 服务的 `log.Fatalf` 在 `restart: on-failure` 下会被 Docker 拉起 | 若拉起后仍失败（注册持续失败）⇒ 无限重启打鸣，需加退避 |
| H5 | 现存 `/health` 消费方（`seed.sh`、smoke、E2E-22 遗留、前端）都不依赖 200-only 语义 | 若有依赖 ⇒ 决策 1 需加过渡期双端点 |
