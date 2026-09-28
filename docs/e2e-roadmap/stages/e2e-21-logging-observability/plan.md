---
stage: e2e-21
title: 日志体系（结构化日志 + traceId 全链路注入 + Loki 采集链路打通）
type: transformation
status: done
created: 2026-09-28
depends-on: []
blocks: []
gate: []
related-findings: [E2E-F-07, E2E-F-13]
---

# E2E-21 日志体系 — 详档

> **类型**：transformation —— 日志**能产出但采不走、traceId **能消费但无人生成**，两端断点使整条日志链路静默失效。本阶段接通 + 实测可查。
> **依据**：roadmap §第六批 E2E-21 行（"结构化日志 + traceId 注入（含 gRPC 侧）+ Loki 采集链路（Go 日志现未进 Loki）"）+ 账本 **E2E-F-07** / **E2E-F-13** + [ADR-2026-09-loki-aggregator-dev](../../../architecture/adr/adr-2026-09-loki-aggregator-dev.md)。
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。

---

## 1. 阶段目标

让"日志能不能查"从**未知**变成**实测可查**，并让 traceId 从**消费侧已备好、生产侧全空**变成**端到端同一 ID**：

1. **采集入口**（E2E-F-07）—— promtail 的 `services` job 指向 `/var/log/services/*.log`，但**全仓无任何 volume 往该路径写**，是永不匹配的空壳 job；Go 服务日志全在容器 stdout，**当前只有 APISIX access log 进了 Loki**。→ 改为 ADR §2.1/§2.3 原定的 `docker_sd_configs` 采容器 stdout。
2. **traceId 生产侧**（E2E-F-13 的 HTTP 半边）—— `gin_skywalking.go:73-77` 消费 `X-Trace-Id`，但**全仓零处生产该 header**（`seed.sh:445` 只注入 `X-User-Id`），生产环境 HTTP 链路 `trace_id` **恒为空**。→ APISIX 注入，且与 access log 的 `trace_id` 用**同一个值**（可 join）。
3. **traceId gRPC 半边**（E2E-F-13）—— `tracing.go:126-164` 的 server interceptor 只打 SkyWalking span，**从不调 `logging.WithTraceID`** ⇒ gRPC handler 内日志无 trace_id。→ client 注入 metadata / server 读出注入 ctx。

---

## 2. 范围与边界

### 做

**A. 采集入口打通（E2E-F-07）**

- `deploy/docker-compose.infra.yml` promtail 块加两个 volume：`- /var/lib/docker/containers:/var/lib/docker/containers:ro` + `- /var/run/docker.sock:/var/run/docker.sock:ro`（ADR §四 原定的缓解手段，逐字采纳）。
- `deploy/loki/promtail-config.yaml`：删除空壳 `services` job，改为 `docker_sd_configs` + relabel 过滤 —— **必须过滤**，实测宿主有 **29 个容器目录**，不滤则全部进 Loki。过滤条件：容器名 `^/emotion-echo-(user|chat|web-bff|ai|analytics|assessment|llm)-svc` 等 + 排除 `loki`/`promtail`/`prometheus`/`grafana`（否则 Loki 读自己的日志形成回环）。
- `deploy/docker-compose.infra.yml` 应用服务侧：为 6 个 Go svc 加 `labels: logging: <svc名>`，供 relabel 提取为 `svc` label（也可纯靠容器名正则，二者取一，本阶段用容器名正则 + label 双保险）。
- 同步修正 `promtail-config.yaml:1-10` 的文件头注释 —— 现有注释自相矛盾（`:9` 说"用 static path 因 file 已挂到 host"，`:34` 又保留一个采不到的 `services` job），是典型"注释制造已覆盖假象"。

**B. traceId 全链路注入（E2E-F-13）**

- **HTTP 入口**：`deploy/apisix/seed.sh` 的 `PLUGINS_JSON`（所有 route 共用，见 `:365` 注入方式）里加一条 `serverless-pre-function`：`core.request.set_header(ctx, 'X-Trace-Id', ctx.var.request_id)`。
  - 放**共用**段而非只在 catch-all：白名单路由（登录/注册等 110~117，见 `seed.sh:540-546`）不走 catch-all 的 `_rewrite.lua`，若只改 catch-all 则**登录链路恒无 trace_id**（而登录恰恰是最需要排查的链路）。
  - 值用 `ctx.var.request_id`：`config.yaml:148` 已设 `trace_id_source: x-request-id`，nginx access log 尾部带 `$apisix_request_id`（`config.yaml:302`），file-logger 的 `"trace_id": "$http_x_request_id"`（`seed.sh:354`）—— 三者同源，故 **APISIX access log 与 Go 服务日志可用同一 ID join**。
  - 顺带把 `X-Trace-Id` 加进 CORS `allow_headers`/`expose_headers`（`seed.sh:401-402,476-477`），与 `X-User-Id` 同样式，使浏览器端也能看到该 ID。
- **gRPC 侧**：
  - client 侧：`grpcinterceptor/tracing.go` 的 client interceptor 从 ctx 取 `logging.TraceIDFromCtx(ctx)`，写入 outgoing metadata `x-trace-id`（**照抄现成的 `x-user-id` 模式**：`userid.go` + `tracing.go:231` 保留 outgoing md 的写法，不另创机制）。
  - server 侧：`NewServerTracingInterceptor` 读 incoming metadata `x-trace-id`，非空则 `ctx = logging.WithTraceID(ctx, tid)` 后传给 handler。
  - **不改 SkyWalking 的 sw8**：`sw8` 是 APM 链路（E2E-26 范畴），本阶段不动，避免与 OAP 的 ID 体系纠缠。
- **消费侧已就位**：`gin_skywalking.go:73-77` 无需改，只需补回归钉（现状只有单测手工塞 header，见 `gin_skywalking_test.go:601`）。

**C. 验证闭环收紧（防止假 PASS）**

- `scripts/smoke_observability.py:438-444`：现状"query 端点 200 但结果为空"也判 PASS（SKIP 文案），即**空 Loki 一样全绿**。改为按 `{job="services"}` 或 `{svc="<name>"}` 断言**非空**。
- 该脚本当前只查 `{job="apisix"}`，**从不查业务服务**。

**D. 结构化收敛 —— 只做最小必要（见 §2.3 修正说明）**

- `emotion-echo-shared/pkg/configcenter/nacos_config.go` 用 `fmt.Printf`（连 stderr 都不是，绕过 slog）→ 改为 `logging.Printf`。shared 其余 13 处 stdlib `log` **保持不动**（理由见 §2.3）。

### 不做（边界）

| 边界项 | 理由 | 去向 |
|--------|------|------|
| 159 处 `log.Printf` 调用点迁成结构化 KV | Go 1.26 `log/slog/logger.go:34` 明示"`SetDefault` 后默认 `log.Logger` 的调用传给 slog handler" ⇒ **这些调用已在输出 JSON 并带 `svc` 字段**，无需迁移即可被 Loki 检索（全文 `\|~` 正则）。159 处机械改动无行为收益、风险大于收益 | 账本记账 |
| 前端日志上报（Sentry / beacon 等） | 58 处 `console.*` 无任何 SDK；属端侧/前端治理议题 | 账本记账 |
| k8s 侧 promtail `runAsUser: 10001` 与 hostPath `/var/log/pods` 权限冲突 | `charts/emotion-echo/charts/loki/templates/daemonset-promtail.yaml:17-20`；dev 环境验不了 k8s 语义 | 账本记账（归 E2E-23/部署轮） |
| Loki 版本漂移（dev `2.9.4` vs chart `3.2.0` vs ADR `3.2.0`） | 非阻塞本阶段功能；若实测阻塞再升级为范围内 | 账本记账 |
| APISIX 双份 access log 冗余（`seed.sh:340` file-logger + `config.yaml:293` nginx access_log） | 属网关治理 | 账本记账（归 E2E-25） |
| SkyWalking OAP trace ↔ 日志 join | OAP 9.x `queryDuration` bug 属 E2E-26 | 归 E2E-26 |

### 2.3 关于"结构化收敛"的范围修正（写计划期的重要自查）

初稿曾把"5 个服务仍用 stdlib `log.Printf`、绕过 slog"列为 P0，据此计划迁移 159 处调用点。**该判断经回读 Go 源码后推翻**：

```
$ go version → go1.26.1
$ sed -n '34p' $(go env GOROOT)/src/log/slog/logger.go
// After [SetDefault] is called, calls to the default [log.Logger] are passed to the
```

各服务 `main()` 均已调 `logging.Init()`（其内部 `slog.SetDefault`，`logging.go:111`）⇒ **stdlib `log.Printf` 已经走 JSON handler**，输出形如：

```json
{"time":"2026-09-28T14:51:58.964+08:00","level":"INFO","msg":"[nacos] registered emotion-echo-web-bff at 0.0.0.0:8894","svc":"web-bff"}
```

（该样本取自实测容器日志，见 §8 证据 7。）**唯一真实差异**是 stdlib 路径不经过 `splitModule`（`logging.go:222`），故 `[nacos]` 前缀留在 `msg` 里而非拆成 `module` 字段——这是可检索性问题（可用 LogQL 正则绕开），不是采集问题。

**结论**：本阶段不做 159 处迁移，只做 D 节的 `fmt.Printf` 修正（1 处）。此条记录在案，防止后续阶段再次误判。

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| 依赖阶段完成 | **无**（E2E-21 不依赖任何前置阶段；E2E-20 已 done，E2E-19/18/17/16/15/14/13/12 均 done） |
| 决策门（RUNBOOK §9） | ✅ 无阻塞项（D-04 i18n 不阻塞任何阶段） |
| 环境 | dev 模式全栈 + `obs` profile（Loki / Promtail / Grafana / Prometheus 均在 `profiles: ["obs"]`，`docker-compose.infra.yml:369-481`） |
| 数据准备 | 无需造数；触发链路用一次登录 + 一次发消息即可产生日志 |

环境启动命令（**必须带 `--env-file .env.local`**，见 AGENTS.md §四）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev --profile obs up -d
```

> 本阶段需要 `--profile obs`（RUNBOOK §12 速查表未含此 profile，执行时以本节为准）。
> `.devmode-session` 当前为空（实测），无双轨冲突（AGENTS.md §八）。

### 3.1 开工前必须先跑的可行性探针（RED 前置）

计划期已实测 Docker Desktop 上采集所需的宿主路径**确实可见**，但**当时用的是 Git Bash**，路径转换是已知陷阱，执行期必须用同一探针复测：

```bash
MSYS_NO_PATHCONV=1 docker run --rm \
  -v /var/lib/docker/containers:/probe:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro alpine:3 \
  sh -c 'ls /probe | wc -l; ls -l /var/run/docker.sock'
```

判据：目录数 > 0 且 socket 存在 ⇒ 采集路径可行；否则测试点 #1 判 `BLOCKED` 并按 §8 升级（备选方案：改 `logging.Init()` 支持 `LOG_FILE` 落盘 + bind mount，成本见 §6）。

---

## 4. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（截图并被查看）· `[M]` 需人工/设计裁定（必须升级给用户）。详见 [RUNBOOK.md](../../RUNBOOK.md) §4。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 6 个 Go svc 的日志**实际出现在 Loki**（非"配置已改"） | [A] | `curl '{job="services"}'` 按 `svc` label 逐个查询非空；给出实际返回的 stream 数与样本行 | | ⬜ |
| 2 | 采集到的是**结构化 JSON**（含 `svc`/`level`/`msg` 字段）而非纯文本 | [A] | Loki 返回体中 `log` 字段 `json_decode` 成功且 `svc` 非空 | | ⬜ |
| 3 | HTTP 请求日志的 `trace_id` **非空** | [A] | 走一次经 APISIX 的请求 → 对应服务日志行含 `trace_id` 字段且值非空（修前恒空） | | ⬜ |
| 4 | **登录链路**（白名单路由）也有 trace_id | [A] | 登录请求后查 user-svc 日志含非空 trace_id。**若只改 catch-all，此点必红**——专治"改了但只覆盖部分路由" | | ⬜ |
| 5 | Go 日志 trace_id **与 APISIX access log trace_id 相同**（可 join） | [A] | 取一次请求的 ID，两侧 Loki 查询交叉命中 | | ⬜ |
| 6 | gRPC 下游服务日志带**同一 trace_id** | [A] | BFF→user/chat/ai-svc 的 gRPC 调用，下游 handler 内日志 trace_id == 入口 ID | | ⬜ |
| 7 | Loki 未采到无关容器日志（无回环） | [A] | 查询确认 `loki`/`promtail`/`grafana`/`postgres` 等容器不在结果内；总 stream 数 ≈ 预期服务数而非 29 | | ⬜ |
| 8 | Grafana Explore 中**按 trace_id 查出完整链路** | [V] | 浏览器打开 Grafana → Explore → Loki → 输入 LogQL → 截图（截图**必须被查看**） | `screenshots/08-grafana-traceid-query.png` | ⬜ |
| 9 | Grafana 中**按 svc 维度**看日志 | [V] | Explore 查 `{job="services"}` 分组显示 → 截图并查看 | `screenshots/09-grafana-by-svc.png` | ⬜ |
| 10 | `smoke_observability.py` 收紧后**全绿** | [A] | 实跑输出 | | ⬜ |
| 11 | **负向对照**：人为断链后 smoke **变红** | [A] | 临时停掉 promtail / 改坏 relabel 后重跑 smoke，必须出现 FAIL。**证明收紧后的断言有约束力**（防 AP-11 假绿） | | ⬜ |
| 12 | Go 侧 traceId 注入的**单元回归钉** | [A] | `go test` 覆盖：gin 中间件消费 `X-Trace-Id`；gRPC client 注入 `x-trace-id` metadata；server 读出注入 ctx（照 `E2E-F-125` 的 fake 记录 incoming metadata 模式） | | ⬜ |
| 13 | seed.sh **静态契约钉**：X-Trace-Id 注入语句存在且在共用 PLUGINS_JSON 段 | [A] | 契约测试断言 `seed.sh` 含 `X-Trace-Id` 注入且位于非 catch-all 专用段 | | ⬜ |

**BLOCKED 预算**：RUNBOOK §4 规定 `BLOCKED` 超过总数 1/3（13 点中 >4 点）不得判 done。

---

## 5. 验收标准（DoD）

- [ ] 13 个测试点全部有结论（PASS / FAIL / BLOCKED+理由），`FAIL` 已按 §5 分类并修复
- [ ] 修复项走完 TDD（Red → Green → Refactor），且测试点 #11 负向对照通过
- [ ] 回归钉落地：`smoke_observability.py` 收紧项 + Go 单测 + Playwright spec（若适用）
- [ ] `scripts/e2e_stage_audit.py --stage e2e-21` 0 FAIL + `python scripts/e2e_stage_audit.py --all` 0 FAIL
- [ ] 账本 E2E-F-07 / E2E-F-13 状态翻转；本阶段新发现按 §5 格式追加（编号连续）
- [ ] `roadmap.md` 状态表 + 顶部「当前激活阶段」同步更新
- [ ] §2.5 收口自检三连通过
- [ ] §7 收口契约 11 项全过（含 §13.3 第二方核对 —— **执行者不得自行宣布 done**）

---

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| `docker_sd_configs` 采到**全部 29 个容器**导致 Loki 被无关日志淹没 | relabel 硬过滤 + 测试点 #7 显式验证 |
| Loki 采到自身日志形成**回环** | relabel 排除 `loki`/`promtail` |
| Windows 宿主路径差异：`- /var/lib/docker/containers` 在 compose 中是否被 Git Bash 转成 Windows 路径 | **本机实测：Git Bash 会把 `/var/...` 传给 `docker.exe` 时做 MSYS 转换，导致挂载静默失效**（探针首跑返回 `ls: /probe: No such file or directory`，加 `MSYS_NO_PATHCONV=1` 后正常）。compose 文件由 `docker compose` 读取不经 Git Bash 转义，但**手工执行验证命令时必须加该变量** |
| json.log 权限 `-rw-r----- root root`，promtail 容器若以非 root 跑会读不到 | promtail 当前 compose 未设 `user`（root）⇒ 可行；测试点 #1 实测确认 |
| 白名单路由（登录/注册）不走 catch-all rewrite ⇒ trace_id 缺失 | X-Trace-Id 注入放**共用** `PLUGINS_JSON`；测试点 #4 专门盯这条 |
| Go 服务日志量大撑爆 Loki（retention 24h，`loki-config.yaml:40`） | 本阶段不动 retention；若实测磁盘压力，记账本并在报告说明 |
| `LOG_LEVEL` / `LOG_FORMAT` 环境变量在 compose 中未显式配置 | 现状走默认（json / INFO），符合预期；不做改动，仅在报告记录 |

---

## 7. 产出物

- 执行记录：`docs/e2e-roadmap/stages/e2e-21-logging-observability/report.md`（按 RUNBOOK §10 模板）
- 截图：`docs/e2e-roadmap/stages/e2e-21-logging-observability/screenshots/`（测试点 #8 / #9 各 ≥1 张）
- 回归钉：`scripts/smoke_observability.py`（收紧断言）+ Go 单测（`emotion-echo-shared/pkg/grpcinterceptor/*_test.go`、`emotion-echo-shared/pkg/middleware/gin_skywalking_test.go`）
- 可能的 ADR：若 X-Trace-Id 注入位置（共用段 vs catch-all）或采集方式偏离 ADR-2026-09-loki-aggregator-dev，需更新该 ADR 或新立 ADR（RUNBOOK §13.3 断言 15 会查）

---

## 8. 调研依据（AGENTS §〇.6 — 计划期 2026-09-28）

### 8.1 已读代码文件（≥3 实现 + ≥1 测试）

| 文件 | 读到的关键事实 |
|------|--------------|
| `emotion-echo-shared/pkg/logging/logging.go`（全文） | `:83-85` `Init()`→stdout；`:102-112` JSON/Text handler；`:126-139` `enrichHandler` 注入 `svc`/`trace_id`/`action`；`:111` `slog.SetDefault`；`:222` `splitModule` |
| `emotion-echo-shared/pkg/middleware/gin_skywalking.go:60-90` | `:73-77` **消费** `X-Trace-Id` header；`:31` 注释错误假设该 header 由 APISIX jwt-auth 注入 |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing.go:120-170` | server interceptor 打 6 个 span tag，**无 `logging.WithTraceID`** ⇒ gRPC 日志无 trace_id |
| `deploy/loki/promtail-config.yaml`（全文） | `:32-40` `services` job → `/var/log/services/*.log`；`:19-20` push 到 loki:3100；`:1-10` 头注释与实现矛盾 |
| `deploy/docker-compose.infra.yml:456-485` | promtail 只有 2 个 volume（apisix 日志文件 + 自身配置），**无 docker 目录/socket**；loki/promtail/grafana/prometheus 均 `profiles: ["obs"]` |
| `deploy/apisix/seed.sh:334-358, 401-402, 445, 540-546` | `:445` `_rewrite.lua` 只注入 `X-User-Id`；`:354` file-logger `trace_id=$http_x_request_id`；`:540-546` 白名单路由 110~117 |
| `deploy/apisix/config.yaml:148, 302` | `trace_id_source: x-request-id`；nginx access_log 含 `$apisix_request_id` |
| `scripts/smoke_observability.py:400-450` | `:438-444` 空结果判 PASS（SKIP 文案）；只查 `{job="apisix"}` |
| `emotion-echo-shared/pkg/grpcinterceptor/userid.go` | `x-user-id` 的 client 注入 / server 提取现成模式 —— **gRPC trace_id 照此实现，不另创机制** |

### 8.2 已查 ADR / 既有决策

| 文档 | 结论 |
|------|------|
| `adr-2026-09-loki-aggregator-dev.md`（Accepted） | §2.1/§2.3/§四 **原决策就是** `docker.sock` + `/var/lib/docker/containers` 采容器 stdout，并给出该 volume 作为风险缓解 ⇒ 本阶段 A 是**补实现 ADR 已有决策**，不是新决策。⚠️ 该 ADR 同时定 3.2.0 / retention 1d，落地成了 2.9.4 / 24h（版本漂移，记账本） |
| `adr-2026-09-e2e-20-multi-instance-state-sharing.md` | D-28 裁定：能不动就不引入新依赖 —— 本阶段不新增中间件 |
| `decisions.md` | 无阻塞 E2E-21 的未决项；D-04 i18n 不阻塞任何阶段（RUNBOOK §9） |

### 8.3 已跑探针（计划期实测，非引用）

1. **docker 日志目录可见性**：
   `MSYS_NO_PATHCONV=1 docker run --rm -v /var/lib/docker/containers:/probe:ro -v /var/run/docker.sock:/var/run/docker.sock:ro alpine:3 sh -c 'ls /probe | wc -l'`
   → **`count=29`**，`/var/run/docker.sock` 存在（`srw-rw---- root root`）。
   ⚠️ **首跑无 `MSYS_NO_PATHCONV=1` 时返回 `ls: /probe: No such file or directory`** —— Git Bash 把 `/var/...` 转成了 Windows 路径。若不重跑即据此断定"方案不可行"，会得出**方向性错误结论**（对照 memory `invalid-probe-evidence-pattern`）。
2. **日志内容形态**：同一探针读 `*-json.log` 首行 →
   `{"log":"{\"time\":\"2026-09-28T14:51:58.964+08:00\",\"level\":\"INFO\",\"msg\":\"[nacos] registered emotion-echo-web-bff at 0.0.0.0:8894\",\"svc\":\"web-bff\"}\n","stream":"stdout",...}`
   ⇒ **Go 日志已是结构化 JSON 且带 `svc`**，采集侧只需解决"采得到"。
3. **`slog.SetDefault` 与 stdlib log 的桥接**：`go version` → `go1.26.1`；`$GOROOT/src/log/slog/logger.go:34` 注释 ⇒ `SetDefault` 后 stdlib `log` 调用**传给 slog handler**。据此推翻"159 处需迁移"的初判（见 §2.3）。
4. **stdlib log 调用点计数**（排除 `_test.go`）：chat-svc 50 / analytics-svc 39 / web-bff 29 / user-svc 22 / assessment-svc 19 / ai-svc **0** / shared 13。
5. **git 状态**：`git status -sb` 干净且与 origin/main 同步；`git branch --merged main` 仅 main；无残留 worktree ⇒ 满足 AGENTS §2.5。
6. **`deploy/.devmode-session` 不存在** ⇒ 无 Lane 双轨锁冲突（AGENTS §八）。

### 8.4 引用的既有结论（来自账本，非本轮新发现）

- **E2E-F-07**「Go 服务结构化日志实际未进 Loki，只有 APISIX access log 被采集」→ 本阶段主目标之一。
- **E2E-F-13**「gRPC 路径的 trace_id 未注入结构化日志」→ 本阶段主目标之二。
- 预探查原始记录：`findings/2026-09-17-pretest-panorama.md:129`（"实际进 Loki 的只有 APISIX access log"）。

### 8.5 尚未验证、需执行期证实的假设（列入 plan 假设清单）

| 假设 | 若不成立 |
|------|---------|
| A1 `PLUGINS_JSON`（共用段）确实挂在**全部** route 上（不只 catch-all） | X-Trace-Id 注入范围不完整 ⇒ 需改为给每条 route 显式追加 |
| A2 `ctx.var.request_id` 在 `serverless-pre-function` 中可用 | 改用其他 nginx var 或走 `_rewrite.lua` + 补白名单路由 |
| A3 promtail 2.9.4 的 `docker_sd_configs` 支持所需 relabel 写法 | 升级 promtail 版本（会牵动版本漂移记账本条目） |
| A4 `deploy/` 下 compose 被 `docker compose` 读取时不做 Git Bash 路径转换 | 需改用 named volume + sidecar 方案 |
