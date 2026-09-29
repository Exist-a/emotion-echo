---
stage: e2e-23
title: 健康检查与服务发现
executed: 2026-09-29
status: partial
environment: dev 模式（28 容器；compose.dev.yml + --env-file .env.local，profile dev + ai + obs）
---

# E2E-23 执行记录

> **本阶段未完成，判 `partial`。** 40 个测试点中 22 个已执行并有运行时/行为证据，
> 18 个未执行。**执行者不得自行宣布 done**（RUNBOOK §7 #10），需第二方按 §13.3 核对。
> 未执行项在 §2 逐条列明原因，不以 N/A 或 BLOCKED 掩盖。

## 1. 环境基线

- **启动命令**：dev 栈在本轮开工前已运行（28 容器），本轮按需 `--force-recreate` 单服务重建
- **dev 覆盖项声明**：`BFF_DEV_RETURN_CODE=1`（验证码回显）、`BFF_TRUST_APISIX=true`、CORS 含 localhost
- **本轮验证范围**：仅 dev 配置。prod 语义（`compose.prod.yml`）本机无法运行，相关结论均标 N/A 并写明理由
- **Nacos 注册基线**：`count: 6`（chat / ai / assessment / analytics / web-bff / user）
- **容器状态**：结束时 6 个 Go 服务 + APISIX + 6 个观测服务全部 `(healthy)`

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 6 服务 `/health` 可达且 schema 一致 | `[A]` | PASS | `docker exec emotion-echo-web-bff sh -c 'wget -qO- ...'` 五个服务均返 `{"status":...,"dbOk":...}`，仅 `:8894` 宿主可达 | 宿主直连 5 个端口返 `000`（端口未映射，见 plan §0 F-k）⇒ 探针须走容器网络 |
| 2 | 停 Postgres ⇒ user-svc `status` 变化 | `[A]` | PASS | 停 postgres 后 `wget -qO- :8888/health` → `{"status":"degraded",...,"dbOk":false}` | 修复前为 `"ok"`（`healthlogic.go:41` 硬编码），RED 实测 expected `degraded` / actual `ok` |
| 3 | 同上验证 analytics / assessment / ai | `[A]` | PASS | `go test ./internal/logic/ -run TestHealthLogic -count=1` 三服务均 `ok`（各 1.4~1.6s），表驱动覆盖 ok/degraded | 原缺陷同为硬编码（各 `:40`） |
| 4 | BFF 下游全挂 ⇒ HTTP 码与 `status` | `[A]` | PASS | `TestHealthHandler_D29LivenessAlwaysOK` + `TestHealthReadyHandler_D29ReadinessReflectsDownstream`（4 子用例全绿） | 修复前 degraded 却恒 200（`health_handler.go:109`） |
| 5 | `/health` 覆盖 Redis 依赖 | `[A]` | BLOCKED | — | 本轮未实施。6 个服务的 ServiceContext 均无 Redis client（D-27 决定"保留待接入"，当前零业务引用），**无可探测对象**；需先决定接哪个业务入口 |
| 6 | BFF 未注册 Nacos ⇒ 报 degraded | `[A]` | BLOCKED | — | 需在 ServiceContext 注入注册状态查询，属 C 组 #20 的基础设施，未在本轮做 |
| 7 | chat-svc `EventPublisher` 是真连还是仅判 nil | `[A]` | PASS | `healthlogic.go:43-52` 锁死现状并加注释：「此处只判非 nil，并不真连 Kafka」 | 现状 = **仅判 nil**。plan §2 A2 要求"二选一不得留模糊"，本轮选择**明确记录现状**而非改造真连（后者需 Kafka 连接探测设计，超出本轮） |
| 8 | `/health/ready` 存在且不通返 503 | `[A]` | PASS | `go test ./pkg/middleware/ -run HealthReadyRoute -count=1 -v` → 6 子用例 `PASS`；`go test ./internal/handler/ -run D29Readiness` → `ok 0.632s` | |
| 9 | compose healthcheck 指向 ready | `[A]` | PASS | `bash scripts/test_healthcheck_readiness.sh` → `PASS: 7  FAIL: 0` + `GREEN`；`docker inspect emotion-echo-user-svc --format '{{json .Config.Healthcheck.Test}}'` → `["CMD-SHELL","wget --quiet ... /health/ready || exit 1"]` | 负向对照：改回 `/health` → RED |
| 10 | `seed.sh` 自带探活在 readiness 变严后行为正确 | `[A]` | BLOCKED | — | 未实机验证 seed 重跑。`apisix-seed` 仍在跑且 `Exited(0)`，但未构造"某依赖降级"场景 |
| 11 | user-svc / chat-svc 补齐 `/health` handler 测试 | `[A]` | PASS | `go test ./internal/handler/ -run D29Readiness -count=1` → `ok`；`go test ./internal/logic/ -run TestHealthLogic -count=1` → `ok 0.576s` | 二者此前**零** `/health` 测试 |
| 12 | 5 服务 gRPC health service 名正确 | `[A]` | PASS | `go test ./internal/grpcserver/ -run GrpcHealth -count=1` → 4/4 `PASS`（20.7s） | |
| 13 | 优雅停机翻 `NOT_SERVING` | `[A]` | PASS | `--- PASS: TestGrpcHealth_FlipsToNotServingOnShutdown (5.01s)`，起真实 gRPC server + shared healthcheck 客户端 | 修复前**无任何 NOT_SERVING 写入**（plan §0 F-d） |
| 14 | 恢复后翻回 `SERVING`（`Resume()`） | `[A]` | N/A | — | 停机是单向终态，无"恢复"语义。`shared/pkg/healthcheck/server.go:158` 的 `Resume()` 面向"暂停后恢复"场景，进程停机不会调它。**属语义上不适用**，非未做 |
| 15 | `ai-svc` 客户端按状态分流 | `[A]` | BLOCKED | — | 未读 `grpc_analyzer.go:25` 的分流逻辑。`ai-svc` 全包测试耗时 76s（见 §6 已知债），本轮未深入 |
| 16 | web-bff 无 gRPC server 属设计现状 | `[A]` | N/A | — | 陈述性测试点，无可断言行为。已记入 plan §2 B3 |
| 17 | 6 服务注册齐全（`count:6`） | `[A]` | PASS | `curl .../ns/service/list?...namespaceId=emotion-echo-dev` → `count: 6` | |
| 18 | 停 Nacos ⇒ 重启 BFF ⇒ 自愈（**F-107 复现**） | `[A]` | PASS | `docker logs emotion-echo-web-bff` 实测 retry 序列 `attempt 1/10 → 5/10`（退避 2→4→8→16→30s 与代码一致）；Nacos 恢复后日志 `19:02:51 attempt 2/10 → 19:02:56 Starting web-bff`（**5s 内自愈**）；`curl /ns/service/list` → `count: 6`；网关 login → 400（路由通） | F-107 启动期修复获**运行时证据**，账本已翻 |
| 19 | 5 服务 fail-fast 退出码 | `[A]` | PASS | `docker logs emotion-echo-user-svc` 实测 `boot failed (fatal): [nacos] WaitForNacos: context deadline exceeded` ×5；`docker inspect` → `RestartCount=2`（on-failure 拉起）；Nacos 恢复后回 `(healthy)`、重新注册 | **新发现（plan H4 获答）**：Nacos 长期宕机时构成**崩溃-重启打鸣**（fatal 每 ~60s 一次）——dev 可接受，prod 需注意 compose restart 策略 |
| 20 | 运行期掉线 ⇒ 重注册 | `[A]` | PASS | **干净实验**：记录 6 服务 `StartedAt` → 停 Nacos 100s → 起 Nacos 90s → `count: 6`，全程**未重启任何业务服务** | **计划期"必 FAIL"预判被推翻**（AP-06 自纠）：grep 无显式重连代码，但漏了两条隐性通道——SDK gRPC 自动重连 + `Heartbeat()` watcher 每 5s `UpdateInstance`（upsert）。边界：watcher `_, _ =` 吞错误，SDK 若死透则静默失效；>90s 的宕机未测 |
| 21 | 心跳协议统一 | `[A]` | PASS | BFF 日志实测 `BeatInstance failed ... beat HTTP 501: no such api:POST:/nacos/v1/ns/instance/beat` ×9（Nacos 3.x 无该端点）→ 每次静默降级 SDK；5 服务日志 0 次同类告警（一直走 SDK） | **新发现**：BFF 的 HTTP beat 协议**从未生效过**——功能上等价（都靠 SDK），但 ① beat 代码是死的 ② `failCount>3` 后连 WARN 都不打（`nacos_beat.go` 的 `if failCount <= 3`）③ 观测盲区 |
| 22 | `NACOS_REQUIRED` prod 实情 | `[A]` | PASS | 全仓 grep → **0 命中**；`compose.prod.yml` 为 ADR-20 空壳占位（故意不填值） | 结论：**从未被任何编排声明过** ⇒ 促成 D-31 |
| 23 | Nacos 重启 ⇒ APISIX 节点自动跟随 | `[A]` | PASS | `docker restart emotion-echo-nacos` → 服务重注册 `count: 6` → **未重跑 seed、未碰 Admin API** → 网关 login 400（路由通）。反向：停 user-svc 75s 后 Nacos 实例数→0 | **D-30 落定**：RUNBOOK §2.4「重建服务后必须重跑 apisix-seed」判为**误导性文档**并已更正。边界：user 路由在实例摘除后仍 401（非 503）⇒ APISIX 摘除有滞后，属 F-154/E2E-25 的主动健康检查范围 |
| 24 | `GetConfig` 首帧失败仍能继续 | `[A]` | BLOCKED | — | 属 E 组 |
| 25 | APISIX 补 healthcheck | `[A]` | PASS | `docker ps` → `emotion-echo-apisix Up (healthy)`；探针命令**双向实测**（通→0、不通→1） | 该镜像内 wget/curl/nc/busybox **全缺**，只能用 bash /dev/tcp 测 9080 |
| 26 | 观测栈 + skywalking 补 healthcheck | `[A]` | PASS | `docker ps --format '{{.Status}}'` 逐个查询 → grafana/loki/prometheus/alertmanager/kafka-exporter/promtail 均 `Up (healthy)`；`bash scripts/test_obs_healthchecks.sh` → `PASS: 15  FAIL: 0` + `GREEN` | skywalking-oap/ui 与 obs-mock-receiver **未补**（见 §4） |
| 27 | **db-migrate 冷启动 `Exited(0)`** | `[A]` | PASS | **前后对照**：修复前 `ExitCode 1` + `FATAL: Postgres 30s 内未就绪`；修复后 `ExitCode 0` + 日志以 `全部迁移应用完成，共 31 个文件` 结尾 | **F-151 闭环**，账本已翻状态 |
| 28 | `migrate.sh` 有负向测试 | `[A]` | PASS | `scripts/test_migrate_pg_wait.sh` 4/4；负向对照：删掉递增退避 → RED | |
| 29 | `dev-up.sh` 批次等待语义 | `[A]` | PASS | `bash scripts/test_devup_batch_waits.sh` → `PASS: 3  FAIL: 0` + `GREEN`；抽出 `wait_healthy` 实机跑四条路径（redis/apisix/postgres/BFF）→ 全部 `exit=0`、0 秒返回 | 挖出**更深缺陷**：`wait_healthy` 对设了 `container_name` 的服务恒失效（见 §3） |
| 30 | chat-svc 4 个 Outbox 参数可热更 | `[A]` | BLOCKED | — | 属 E 组，未实施 |
| 31 | ai-svc 9 个参数可热更 | `[A]` | BLOCKED | — | 同上 |
| 32 | analytics-svc `MaxRetries` 可热更 | `[A]` | BLOCKED | — | 同上 |
| 33 | P1 敏感字段白名单（负向断言） | `[A]` | BLOCKED | — | 属 E 组前置缺陷，未实施 |
| 34 | P2 `LLM.Timeout` 接线 bug | `[A]` | BLOCKED | — | 同上。已确认 `main.go:487-491` 构造 `NewLLMFuser` 未传 Timeout |
| 35 | P3 `analytics` 重试默认值 | `[A]` | BLOCKED | — | 同上。已确认 `config.go:28` 注释承诺默认 3 但 `SetDefaults` 无该分支 |
| 36 | user/assessment 零候选如实记账 | `[A]` | PASS | plan §2.4 完整盘点表（`user-svc/internal/config/config.go:55-87`、`assessment-svc:49-83` 全文回读） | D-32 已拍板；**未硬造参数** |
| 37 | `smoke_health_discovery.py` | `[A]` | BLOCKED | — | 属 F 组，未建。现有守卫已覆盖部分（3 个 test_*.sh） |
| 38 | Playwright spec `health-discovery.spec.ts` | `[A]`+`[V]` | BLOCKED | — | 属 F 组，未建 |
| 39 | APISIX Admin 页面截图 | `[V]` | BLOCKED | — | 同上。**A7 审计对 `[V]` 点要求截图，本轮无 `[V]` 结论故不触发** |
| 40 | 文档漂移修正 | `[A]` | BLOCKED | — | 属 F 组。RUNBOOK §2.4「必须重跑 seed」待 #23 验证后才能改 |

汇总：PASS 23 / FAIL 0 / BLOCKED 15 / N/A 2

> ⚠️ **BLOCKED 占比 37.5%，仍超 RUNBOOK §4 的 1/3 红线** ⇒ **本阶段不得判 done**，
> 阶段状态 `partial`。破坏性实验 5 项（#18/19/20/21/23）已于 2026-09-29 用户批准后**全部执行完毕并 PASS**；
> 剩余 16 项 BLOCKED 分三类：① 属 E/F 组未开工（#30~35、#37~40）② 依赖尚未落地的基础设施
> （#5/#6 需 Redis 与 Nacos 状态注入）③ 需特定场景（#10 seed 降级场景 / #15 客户端分流阅读）。
> **未用 N/A 或"待后续"掩盖任何一项。**

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| 4 服务 `/health` 的 `Status` 硬编码 `"ok"` | 范围内 | 修复 commit `3034f52` |
| `/health/ready` 会被两处鉴权中间件 401 拦掉 | 范围内 | 修复 commit `52d0a4e` |
| BFF 探出 degraded 仍返 200 | 范围内 | 修复 commit `10e494c` |
| compose healthcheck 打 liveness ⇒ readiness 形同虚设 | 范围内 | 修复 commit `c230095` |
| gRPC health 永不翻转（`healthSrv` 是局部变量） | 范围内 | 修复 commit `5cab18c` |
| `NACOS_REQUIRED` 全仓零声明 | 范围内 | 修复 commit `1579038`（D-31） |
| `db-migrate` `Exited(1)`（F-151） | 范围内 | 修复 commit `f748d61` |
| **11 个常驻服务零 healthcheck（含网关 APISIX）** | 范围内 | 修复 commit `997111e`（7 个） |
| **`wait_healthy` 对设了 `container_name` 的服务恒失效** | 范围内 | 修复 commit `b5cea46` |
| 多个守卫脚本用 `python3`（本机不存在）⇒ 假绿 | 范围内 | 修复 commit `1579038` |
| APISIX upstream 无主动健康检查（`checks` 段） | **范围外** | 账本 E2E-F-154，归 E2E-25 |
| 5 服务 Nacos ops 配置"读了扔" | 范围外 | 账本 E2E-F-155，E 组处理 |
| 账本 F-107 描述与代码事实相反 | 范围内（部分） | 账本 E2E-F-156，**待 #18 运行时复现后才可翻状态** |
| ADR 两条承诺无实现落点 | 范围内（部分） | 账本 E2E-F-157，**待 #13/#14 结论落地** |
| skywalking-oap/ui、obs-mock-receiver 仍无 healthcheck | 范围外 | 本轮未登记（非本阶段目标） |
| `ai-svc` 的 `LLM.Timeout` 是死配置 | 范围外（E 组 P2） | 账本 F-159（见 §5） |
| `analytics-svc` `SetDefaults` 漏 `Kafka.MaxRetries` | 范围外（E 组 P3） | 账本 F-160（见 §5） |
| `configcenter.sensitivePrefixes` 只拦 dataId 不拦 YAML 内 key | 范围外（E 组 P1） | 账本 F-158（见 §5） |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| `3034f52` | 4 服务 `status` 说真话 | 4 份 `healthlogic_contract_test.go`，RED 实测 `expected "degraded" / actual "ok"` |
| `52d0a4e` | `/health/ready` 免鉴权 | `health_endpoint_auth_test.go`，RED 实测 `/health/ready` 无 header 返 401 |
| `10e494c` | 6 服务 readiness 端点 | `health_ready_handler_test.go`（user）、`health_ready_test.go`（bff）；接线守卫 RED 实测 `undefined: HealthReadyHandler` |
| `c230095` | compose healthcheck 改指 ready | `test_healthcheck_readiness.sh` RED 后 GREEN；负向对照 5/1 |
| `5cab18c` | gRPC health 停机翻转 | `server_health_transition_test.go` RED 实测 `MarkShuttingDown undefined` |
| `1579038` | NACOS_REQUIRED 编排声明 | `test_nacos_required_declared.sh` RED 实测 1/3 |
| `f748d61` | db-migrate 等 PG | `test_migrate_pg_wait.sh` RED 实测 2/4 |
| `997111e` | 7 服务补 healthcheck | `test_obs_healthchecks.sh` RED（守卫为新增，无存量缺陷） |
| `b5cea46` | dev-up 分批等待 | `test_devup_batch_waits.sh` RED 实测 4 条错配 |

**负向对照清单**（每条都实跑过，证明断言有约束力）：

| 守卫 | 负向操作 | 结果 |
|------|---------|------|
| `test_healthcheck_readiness.sh` | 8888 改回 `/health` | RED（5 PASS / 1 FAIL） |
| `test_grpc_health_shutdown.sh` | 删掉 chat-svc 的 `MarkShuttingDown()` 调用 | RED（2 条 FAIL） |
| `health_routes_wiring_test.go` | 删掉 ai-svc 的 `r.GET("/health/ready")` | RED |
| `test_nacos_required_declared.sh` | 删掉 dev 的 `NACOS_REQUIRED: "0"` | RED |
| `test_migrate_pg_wait.sh` | 删掉等待间隔递增逻辑 | RED |
| `test_obs_healthchecks.sh` | 删掉 grafana 的 healthcheck | RED（14 PASS / 1 FAIL） |
| `test_devup_batch_waits.sh` | `emotion-echo-$name` 回退为 `$name` | RED |
| `healthlogic_contract_test.go` | `if !dbOK` → `if false && !dbOK` | RED（2 个测试） |

> 最后一条的记录方式：第一次尝试直接改回字面量 `"ok"`，被编译器先拦下
> （`declared and not used: status`），**测试根本没跑到断言**。改用保留变量
> 的方式重做才算真正的行为级负向对照。**编译错误不等于负向对照成功。**

## 5. 账本对账（RUNBOOK §7 #9）

| 账本条目 | 归属 | 本轮处理 | 状态 |
|---------|------|---------|------|
| E2E-F-107 | E2E-23 | **启动期部分闭环（有运行时证据）**：#18 实测 retry 序列 + Nacos 恢复后 5s 自愈。运行期部分由 #20 证明**本就成立**（SDK 重连 + Heartbeat watcher），账本条目收窄为"beat 协议死代码 + watcher 吞错误"（见 F-156 说明） | 🟡 部分解决 |
| E2E-F-151 | E2E-23 | **db-migrate 部分已闭环**（前后对照 ExitCode 1→0）；观测栈部分随 `997111e` 补齐 7 个，skywalking/obs-mock-receiver 仍缺 | 🟡 部分解决 |
| E2E-F-154 | E2E-25 | 本阶段范围外，仅记录；#23 的反向验证（实例摘除后路由仍 401 非 503）补充了其证据 | 🔴 未解决 |
| E2E-F-155 | E2E-23 | E 组未开工 | 🟡 待决策（已由 D-32 拍板范围） |
| E2E-F-156 | E2E-23 | **主体闭环**：F-107 描述与代码相反的失真已用运行时证据修正；#20 推翻了"无重注册"的预判。残留：BFF beat 协议死代码（501）+ watcher `_, _ =` 吞错误的观测盲区，转入下轮小修 | 🟡 部分解决 |
| E2E-F-157 | E2E-23 | #13/#14 已落地（commit `5cab18c`），ADR 文本回填待 F 组 | 🟡 部分解决 |
| E2E-F-158/159/160 | E2E-23 | E 组 P1/P2/P3，本轮仅确认缺陷存在、未修 | 🔴 未解决 |

**存在归属本阶段且未完全解决的账本（F-151/155/156/157/158/159/160）⇒ 按 RUNBOOK §7 #9 与审计 A5，阶段只能标 `partial`。**

## 6. 回归钉

- 新增 `scripts/test_healthcheck_readiness.sh`（7/7）
- 新增 `scripts/test_grpc_health_shutdown.sh`（25/25）
- 新增 `scripts/test_nacos_required_declared.sh`（3/3）
- 新增 `scripts/test_migrate_pg_wait.sh`（4/4）
- 新增 `scripts/test_obs_healthchecks.sh`（15/15）
- 新增 `scripts/test_devup_batch_waits.sh`（3/3）
- 新增 `scripts/_extract_compose_block.py`、`scripts/_check_devup_batches.py`（守卫辅助，非独立门禁）
- Go 单测：5 份 `healthlogic_contract_test.go` + 1 份 `health_ready_handler_test.go` + 1 份 `health_ready_test.go` + 1 份 `server_health_transition_test.go` + 1 份 `health_endpoint_auth_test.go` + 1 份 `health_routes_wiring_test.go`
- **无 Playwright spec** —— 测试点 #38 属 F 组，本轮未建

**全量回归**：7 个 Go 模块 `go test ./...` + `go vet ./...` 全绿。

## 7. 已知债与未验证项（如实记录）

1. **`-race` 本机无法执行**（Windows 报 `0xc0000139`，DLL 加载错误）。按 E2E-21 教训，不用 Windows 错误解释 CI 行为。`MarkShuttingDown` 用了 `RWMutex` 保护 `healthSrv`，逻辑上无竞争，但**未经 -race 实测**，须 CI 复核。
2. **`ai-svc/internal/grpcserver` 整包测试耗时 76s**，超 RUNBOOK §11 的 5s 护栏。对照实测：单跑既有测试 `TestGetEmotionByMessage_HappyPath` 也需 5.6s，慢因来自既有 `startTestServer` 的 cleanup 里 `GracefulStop` 等待 2s，**非本轮新增测试引入**。本轮未擅自重构。
3. **未实机跑完整 `dev-up.sh`** —— 它会重建 19 容器栈，且当前栈正在运行、多个服务镜像已在本轮改过，贸然重跑风险高于收益。改为抽出 `wait_healthy` 单独实机验证四种命名路径。**此项未完成，不假装验过。**
4. **`grpcurl` 无法探测 gRPC** —— 容器不注册 gRPC reflection（`failed to query for service descriptor`）。改用仓库内 `shared/pkg/healthcheck` 客户端（生产同一套封装）。
5. **APISIX healthcheck 只能测端口** —— 镜像内无任何 HTTP 客户端，`resty` 跑不通，且未启用 healthcheck/public-api 插件（`/apisix/status` 实测 404）。端口通**不反映路由是否可用**，后者归 E2E-25。

## 8. 待决策 / 升级项

- **无阻塞性待决策**。D-29 / D-31 / D-32 已于 2026-09-29 由用户拍板。
- **D-30 保持待实测**（非用户决策项，由测试点 #23 出结论后自动落定）。
- **需用户批准才能继续的部分**：本阶段 21 项 BLOCKED 中，E 组（14 个热更参数 + 3 个前置缺陷）与 C 组（运行期重注册）建议排入下一轮；其中 C 组的 #18/#19/#23 需要**停 Nacos 做破坏性实验**，属 RUNBOOK §8 的"破坏性操作"，已升级给用户，本轮未执行。

## 9. 收口自检

- [x] `git status` 干净（改动均已提交）
- [x] `main` 与 `origin/main` 无 ahead/behind（本轮改动全在 feature 分支）
- [x] 无残留已合并分支
- [x] `python scripts/e2e_stage_audit.py --all` → 30 阶段 0 FAIL
- [x] 账本对账完成（§5），5 条未解决项已登记
- [ ] **第二方核对未做** —— 执行者不得自行宣布 done

## 10. 下轮建议

按依赖与收益排序：

1. **C 组 #18/#19/#23**（停 Nacos 的破坏性实验，需用户批准）—— 做完才能翻 F-107/F-156 两条账
2. **F 组回归钉**（smoke 脚本 + Playwright spec + 截图）—— 收口契约第 3 项要求，且能补上唯一的 `[V]` 测试点
3. **E 组**（14 个热更参数 + P1/P2/P3）—— 工程量最大，D-32 已定范围
4. **#5/#6**（Redis 与 Nacos 注册态注入 `/health`）—— 依赖 D-27 的 Redis 业务接入决策
