---
stage: e2e-28
title: 性能与延迟基线
type: verification
status: done
created: 2026-10-03
last-updated: 2026-10-06（**done：F-135 实测否决（D-44）+ TTS API 路线裁定（F-198 专项）销账**；原记录：2026-10-05 收口执行中：§8 回填 + audit + 第二方核对推进（续 2026-10-03 停工留档）；20/20 有结论 = 18 PASS + 2 FAIL-已分类（#4 测点加性差 / #19 回归钉未全绿 F-195/197 归账）；F-136/F-134/F-26 闭环；**F-135 未实施 ⇒ partial**（report §6 升级项①）；M1/M2 → D-42/D-43；账本 F-190~197；详见 report.md）
depends-on: [e2e-10, e2e-17, e2e-22]
blocks: []
gate: []            # 无开工前阻塞决策门；执行期 [M] 决策点见 §4
related-findings: [E2E-F-26, E2E-F-134, E2E-F-135, E2E-F-136]
---

# E2E-28 性能与延迟基线 — 详档（任务书）

> **类型**：verification —— 项目**从未有过**性能基线：全仓零压测脚本（k6/locust/vegeta/wrk grep 零命中）、零 p50/p95 目标、零资源预算（账本 **E2E-F-26** 🔴）。埋点面其实一半就位：6 个 Go svc + llm-service 全部挂了 HTTP duration histogram，但**从未有人消费这些数字**，且部分埋点（`FusionDurationSeconds`）生产零调用（§0.1 F-d）。本阶段**首次**把 `/analyze p50/p95`、`SSE TTFT`、`TTS 单段与段间 gap` 三条主延迟链测出数字、落档、与服务端埋点交叉验证，并给出小规模并发阶梯——**把"快不快"从印象变成可复现的数字**。
>
> **依据**：roadmap 排期总表 E2E-28 行（目标"首次建立基线：/analyze p50/p95、SSE TTFT、TTS 单段与段间 gap + 小规模阶梯"，边界列"全站压测"）+ 账本 E2E-F-26（owner=E2E-28）+ [adr-2026-09-xtts-cors-resource-quotas](../../../architecture/adr/adr-2026-09-xtts-cors-resource-quotas.md)（既有性能实证先例与 F-136 留账）。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。
> **前置阶段**：E2E-10（SSE 已通）/ E2E-17（TTS 改造后复测）/ E2E-22（Prometheus 抓取已通）——**全部 ✅ done，开工前置检查过**。
> **名下账本**：**4 条 —— E2E-F-26**（🔴 基线缺失，本阶段主目标）、**E2E-F-134 / F-135 / F-136**（🟡 TTS 三件套，均属产品/架构裁定 → §4 M2 决策点；A5 约束下未闭环则阶段只能 `partial`）。

---

## 0. 计划期调研（**先摆事实，再排计划**）

> 本节全部为本轮**亲自回读的文件 / 亲自执行的探针**（AGENTS §〇 文档功课），非引用历史结论。执行期若发现与本节不符，**以实测为准并回填本节**（AP-02）。

### 0.0 假设清单（本文假设，与现状对比见 §0.1）

| # | 本文假设 | 依据 / 现状 |
|---|---------|------------|
| A1 | 验证环境 = **dev compose 单机**（`--profile dev --profile ai --profile obs`），prod/k8s 压测不在本阶段 | roadmap 边界"全站压测"；charts 无压测资源定义（待 #5 复核） |
| A2 | roadmap 的 **"/analyze" = emotion-llm-service `POST /analyze`**（情绪分析端点，F-26 所指 `request_duration_seconds` 埋点所在、ai-svc `http_analyzer.go:67` 上游）；**多模态** `/api/v1/multimodal/analyze`（SenseVoice/FER）**不在本阶段** | `emotion-llm-service/main.py:220`；账本 F-26 原文点名该埋点；语音链路归 E2E-16 |
| A3 | 基线工具 = **自研 Python 脚本**（`scripts/perf_baseline.py`，stdlib only 零新依赖），不引入 k6/locust/vegeta | 全仓无压测工具（F-f）+ host `python 3.12.6` 实跑可用 + AGENTS"测试必须可一键跑过、不依赖网络装二进制" |
| A4 | 分位数计算用官方 `statistics.quantiles(n=100, method='inclusive')`（p50=`result[49]` / p95=`result[94]`） | 功课④ WebFetch **docs.python.org 官方 statistics 文档**（Bocha 配额尽，见 §smoke 段） |
| A5 | **无既有 SLA/延迟目标** ⇒ 首基线只落档 + 复现性断言，"是否把阈值设成 CI 硬门禁"是用户决策（M1） | 账本 F-26"无 p50/p95 目标"原文；RUNBOOK §8（改 merge 门禁必须升级） |
| A6 | XTTS **8 核**（F-132/ADR 已落地）为测量前提；**单 worker 串行**（F-136）为已知现状，阶梯/并发测量预期暴露排队 | compose `cpus: "8.0"` + ADR 实证表 8 核 19.9s + `server.py` uvicorn 单 worker |
| A7 | 演示账号 `echo/echo123` 可登录网关；`/api/v1/ai/stream`、`/api/v1/tts/phonemes` **均需 JWT** | `noAuthPathPrefixes` 仅放行 `/api/v1/auth/*` 与 `/api/v1/client-error`（main.go:312-315）+ 本轮登录探针 200 |

### 已读实现文件（≥3，AGENTS §〇 ①）

- `emotion-llm-service/main.py`——`POST /analyze`（:220-236，进程内 `analyze(req.text)` 关键词模型）+ `INTERNAL_API_KEY` 鉴权（:125-133，空 key 跳过）+ `MetricsMiddleware` ASGI 挂载（:118-120，path=`/analyze` 标签实证于 `test_metrics_setup.py:131`）+ `AnalyzeRequest`（:138-140，≤4096 字）
- `emotion-echo-shared/pkg/metrics/metrics.go`——`GinMetricsMiddleware`（:150-183）：`HTTPRequestDuration.Observe(time.Since(start))` 在 **`c.Next()` 之后**（:179，即 SSE 端点记的是**流总时长非 TTFT**）+ `c.FullPath()` 路由模板防高基数 + `/metrics` 自循环跳过
- `emotion-echo-web-bff/internal/handler/ai_stream_handler.go`——SSE 契约头（:1-19 `X-Accel-Buffering: no`）+ `aiStreamReq` 双格式（:338-356）+ `ServeHTTP`：decode → mock fallback → llm gRPC 优先（:364-420，`writeDelta0` 逐块 Flush）
- `emotion-echo-web-bff/internal/handler/tts_handler.go`——`POST /api/v1/tts/stream`（:36）+ `POST /api/v1/tts/phonemes`（:40，转发 XTTS `/tts_with_phonemes` :79）
- `emotion-echo-models/XTTS/server.py`——uvicorn **单 worker**（:25/:362-370）+ `/tts` :153 / `/tts_stream` :265 / `/tts_with_phonemes` :283 + `/metrics` :148
- `emotion-echo-ai-svc/internal/fusion/fusion_metrics.go`（全文 56 行）——薄 wrapper：`ObserveLLMLatency`（有调用方 `llm_fuser.go:183`）vs `ObserveFusionDuration`（**零生产调用**，见 F-d）

### 已读测试文件（AGENTS §〇 ①）

- `emotion-echo-ai-svc/internal/fusion/fusion_metrics_test.go`——prometheus counter/histogram 断言范式（`RegistryGather` delta > 0，容忍共享 registry 噪声），本阶段交叉验证断言可参照其读法

### 已查 ADR / 决策 / stage（AGENTS §〇 ②）

- **adr-2026-09-xtts-cors-resource-quotas.md（全文 89 行）**——既��性能实证先例：40 字同步 188s→19.9s（8 核 9.5x）、流式 10 字首字节 21.2s→4.0s、端到端 27.6s；Consequences 明写"XTTS 仍单 worker 串行（F-136 留账）"；守卫 `check_xtts_cpu_limit.sh` 在位（但接线缺口见 F-h）
- **e2e-roadmap/decisions.md**：已用至 **D-41**，**本阶段决议从 D-42 起**
- **architecture/decisions.md**：已用至**决策 38**（均为 Lane O 端侧条目）；**若产生架构级决议（如 F-136 多 worker）从决策 39 起**（双编号体系，两套都避开，见并行协议 §三.资源3）
- **E2E-22 plan**——Prometheus 查询先例：`/api/v1/query` + `histogram_quantile` + `EXPECTED_TARGETS` 比对；Grafana `emotion-echo-overview` 已有 p95 Latency 面板（E2E-22 D1 修过）
- **E2E-17 report / ADR 实证表**——TTS 历史数字基线（本阶段对照锚点）
- **账本对账**：归属 E2E-28 = **F-26 🔴 / F-134 🟡 / F-135 🟡 / F-136 🟡**（+F-06 ✅ 已闭环，其"前端 gap 微任务级"结论由本阶段 #12 复验）；**F-189（全量 Playwright 基线红）owner = E2E-30/专项，不阻本阶段**

### smoke / 运行时探针（AGENTS §〇 ③）

- **登录探针**：`POST :19080/api/v1/auth/login`（echo/echo123）→ **200 @0.19s**，token 217 字符
- **SSE TTFT 探针**：`curl -N POST /api/v1/ai/stream`（Bearer + `{"message":"你好，只回一句话","stream":true}`）→ **ttfb=11.717s / total=11.876s / 200 / 19 个 `data:` 块**；首块后 **0.16s 内 19/19 块全部到达**（突发而非渐进 → "伪流式整段缓冲"疑点，**根因待查禁臆断**，列为 #7）
- **TTS 探针**（17 字经网关 `/api/v1/tts/phonemes`）：**冷 36.27s → 热 2.02s（18x）**，200 + 合法 RIFF 头；冷=容器空闲后首请求
- **/analyze 探针**（`docker exec emotion-llm-service python`，宿主**无** 8000 端口映射）：5 采样 `[435.3, 3.9, 1.2, 1.3, 1.3]ms`——**冷 435ms（模型加载）→ 热 ~1.3ms**，响应 `model:"keywo..."`（关键词模型进程内计算，非 LLM 网络调用）
- **obs 栈现状**：`emotion-echo-prometheus` / `grafana` / `alertmanager` **Exited (0) 22h 前**（profiles `["obs"]` 未拉起）⇒ Prometheus 交叉验证开工时须 `--profile obs` 起栈
- **守卫接线核查**：`scripts/check_xtts_cpu_limit.sh` 在 `.github/workflows/` 与 `docs/ci-workflows/README.md` 守卫清单 **grep 零命中**（脚本头"用法（CI 接入）"自证未接——AP-10 同型）→ F-h
- **业务契约 smoke（§2.4）不适用**：本 PR 纯文档（plan + roadmap），不触碰业务代码路径——按 AGENTS §〇③ 记录豁免理由
- **外部官方文档（功课④）**：Bocha search 返回 **403 配额尽**（与 E2E-27 计划期同）→ 改 WebFetch **docs.python.org 官方 statistics 文档**成功，分位数方法（A4）以官方原文为准并引用 URL：`https://docs.python.org/3/library/statistics.html`
- **解释器实跑**：host `python --version` → **Python 3.12.6**（真解释器）；`python3` 是 Windows Store 别名桩（既往坑）⇒ 本阶段脚本与命令**一律 `python`**

### 0.1 计划期实测事实表

| # | 事实 | 证据（`文件:行号` / 探针输出） |
|---|------|--------------------------|
| **F-a** | **SSE 首字节 11.7s，但首块后呈突发到达**：19/19 data 块挤在 0.16s 内 → 流式渐进性存疑（整段缓冲/上游非流式聚合，**根因待查**）；对照：mock 路径瞬时返回，11.7s 说明走的是真 LLM | `curl -sN -w ttfb=%{time_starttransfer}` 输出（§smoke 段）+ `ai_stream_handler.go:364-420` |
| **F-b** | **TTS 单段冷 36.3s / 热 2.0s（18x）**：17 字同文本两连测；冷路径（F-06 记过 ~25s/4 字符同型）远慢于热路径 | 两连 `curl /api/v1/tts/phonemes` 输出（§smoke 段） |
| **F-c** | **/analyze 是进程内关键词模型**：热路径 ~1.3ms、冷 435ms；**无宿主端口映射**（`docker port` → `8000/tcp` 裸端口）⇒ 测量必须走容器网（`docker exec -i emotion-llm-service python -` 已实证可行） | 容器内 5 采样（§smoke 段）+ `main.py:220-236` + `docker port` 输出 |
| **F-d** | **埋点面一半是空的**：① `GinMetricsMiddleware` 6 svc 全挂 + llm `MetricsMiddleware` 挂（活）；② 但 `ObserveFusionDuration`/`FusionDurationSeconds` **全仓仅 wrapper + test，零生产调用**（F-26"fusion histogram 已就位"对该指标不成立）；③ SSE 端点的 histogram 记录的是**流总时长**（observe 在 `c.Next()` 后）⇒ **TTFT 无任何服务端埋点** | `metrics.go:179` + `fusion_metrics.go:53-55` + grep `ObserveFusionDuration(` 仅 :13/:54/test:86 |
| **F-e** | **obs 栈 Exited 22h**（prometheus/grafana/alertmanager）；`prometheus.yml:33-38` **已含 llm-service target**（E2E-22 修复在位，job `emotion-echo-services`） | `docker ps -a` + `deploy/prometheus/prometheus.yml:33-38` |
| **F-f** | **零压测工具**：k6/locust/vegeta/wrk 全仓 grep 零命中（`scripts/on-device-perf/` 属 **Lane O 独占列，本阶段禁触**）；host `python` 3.12.6 可用 | grep 输出（§smoke 段）+ `python --version` |
| **F-g** | **测量入口鉴权面**：`/api/v1/ai/stream` 与 `/api/v1/tts/phonemes` 均需 JWT（白名单仅 auth/* + client-error）；echo/echo123 登录 200 @0.19s | `main.go:312-315` + 登录探针 |
| **F-h** | **`check_xtts_cpu_limit.sh` 未接 CI、未入守卫清单**（AP-10 同型：写了守卫没接线）——它是本阶段"数字可比性"的前提钉（8 核漂移会让所有 TTS 数字失真） | `.github/workflows/` grep 零命中 + `docs/ci-workflows/README.md` grep 零命中（本轮实测） |
| **F-i** | **环境基线（2026-10-03 计划期）**：核心栈 Up 3-8h（web-bff/apisix/postgres/6 svc/minio/xtts/fer/sensevoice/sw-oap healthy）；obs 栈 Exited；web 容器 Exited（按 RUNBOOK §2.1b 用本地 `pnpm dev`）；`db-migrate`/`apisix-seed` Exited(0)；**`.devmode-session` 无锁**；XTTS `cpus: "8.0"` 在位（compose :584） | `docker ps -a` + 锁文件不存在 + `grep cpus` |

### 0.2 开工复核清单（第一天执行，防止任务书事实表过期）

| # | 复核项 | 通过标准 |
|---|--------|----------|
| 1 | 登记 `.devmode-session` → 按 RUNBOOK §2.1 起栈**加 `--profile ai --profile obs`**（TTS 依赖 ai / 交叉验证依赖 obs）→ Nacos `count:6` → 全 healthy + **prometheus targets 全 UP** | `docker ps` + `curl localhost:9090/api/v1/targets` 留档 |
| 2 | F-a 复核：重测一条 TTFT（首字节秒级 + 块到达时间戳序列），确认"突发到达"仍复现 | curl 时间戳输出 |
| 3 | F-b 复核：TTS 冷热各测 1 条——**冷样本必须在容器空闲 ≥5min 后取**（防上一探针余温污染） | 两向耗时输出 |
| 4 | F-d 复核：grep `ObserveFusionDuration(` 仍仅 wrapper/test；`curl localhost:9090` 查 `emotion_echo_http_request_duration_seconds` series 非空（若 obs 起得来） | grep + query 输出 |
| 5 | F-h 复核：`bash scripts/check_xtts_cpu_limit.sh` 本地跑绿 + 接线现状仍为未接（接线是 #5 测试点的工作） | rc=0 |
| 6 | 内存余量（19 容器 ≈6G 贴顶前科，`.wslconfig` 8GB；阶梯压测会加压） | `docker stats` 余量 ≥1G |

---

## 1. 范围

**做**：
- 组 A（测量工具与 /analyze 基线）：自研测量脚本（percentile 方法官方对齐 + 负向对照）、`/analyze` p50/p95 落档、1/2/4/8 小阶梯、Prometheus 服务端埋点交叉验证、配置事实回读（8 核钉 + 守卫接线）
- 组 B（SSE TTFT）：TTFT 基线 N≥5、**流式渐进性定性（F-a 疑点）**、SSE 服务端埋点缺口定性、浏览器视觉对照
- 组 C（TTS 单段与段间 gap）：单段冷/热 p50/p95、并发串行量化（F-136 实证）、前端段间 gap 复验（F-06/F-129 面）、端到端分段时间、**M2 裁定 F-134/135/136 处置**
- 组 D（阶梯与资源对照）：SSE 小阶梯（1/2 并发，LLM 成本受控）、压测期间资源采样、Grafana 延迟面板数据核对
- 组 E（落档与收口）：基线报告 + 截图、Playwright 回归钉、账本对账与审计

**不做（明确划出边界）**：
- **全站压测**（roadmap 边界列）、容量规划、生产/k8s 压测、soak 长跑与内存泄漏长时测试
- **引入外部压测工具**（k6/locust/vegeta/wrk）——选型理由见 A3；换工具属工具治理另议（若 M1 裁定要 CI 门禁再评估）
- **端侧 on-device 性能**（`scripts/on-device-perf/**` = Lane O 独占列，协议 §二禁触）
- **多模态 `/api/v1/multimodal/analyze`（SenseVoice/FER）延迟**——归 E2E-16 语音链路（A2 边界）
- **阈值 CI 门禁**（除非 M1 裁定）与**基线红绿告警**（E2E-22 告警面已收口，不重开）
- **F-134/135/136 的实施改造**（除非 M2 裁定——裁定要动则按 §3 C4 展开并先 ADR）
- 前端性能预算（bundle/lighthouse/首屏）——非本阶段主题
- 观测面扩展（补 FusionDuration 调用点等 F-d 缺口）：**只定性记账**，除非裁定纳入（见 #8）

---

## 2. 测试点清单（20 个）

> 判定分级：`[A]` 自动/脚本断言 · `[V]` 视觉/IAB 实测 · `[M]` 需裁定（执行期决策点，见 §4）。**基线类断言只证"数字可信、可复现、被落档"，不发明 SLA**（A5）；阈值门禁是 M1 的事。

### 组 A：测量工具与 /analyze p50/p95（F-26 核心 / F-c / F-h）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 1 | [A] | **测量脚本方法自检 + 负向对照**：percentile 计算对官方方法（A4）用已知样本断言（如 `[1..100]` p50=50/p95=95 容差内）；指向**未监听端口** → 脚本**非零退出**且错误显式输出（非静默 PASS、非 traceback 裸崩） | 单测断言输出 + 负向运行退出码 ≠0 |
| 2 | [A] | **/analyze p50/p95 基线**（容器网内 N≥50，F-c）：冷首请求**单列**不入热样本；热样本 p50/p95 落 `baseline/analyze.json`；**复现性断言**：独立跑两轮 p50 比值 ∈ [0.5, 2] | 两轮 p50/p95 数字 + 比值 + JSON 落档路径 |
| 3 | [A] | **小阶梯 1/2/4/8 并发对 /analyze**：每档 N≥20，HTTP 200 率 100%、零超时；每档 p50/p95 记录成退化表（**记录不设门禁**——无 SLA，A5） | 四档状态码统计 + 分位数表（阶梯 JSON 落档） |
| 4 | [A] | **Prometheus 交叉验证（埋点消费闭环）**：obs 栈 UP → `histogram_quantile(0.5/0.95, llm_http_request_duration_seconds{path="/analyze"})` 与 #2 脚本 p50/p95 **同数量级（0.5x~2x）**——首次有人消费这些 histogram。**执行期回填（2026-10-03，F-193）**：比值带宽对亚毫秒端点结构性失效（client/server 测点加性差 ~0.65ms > 数值本身）⇒ 判读按**三段式**：①同数量级 ②带宽命中与否如实判 ③测点差定性；p50 带外判 FAIL-已分类，桶分辨率修复为范围内处置 | PromQL 输出 + 脚本数字对照表 + target UP 截图/输出 |
| 5 | [A] | **配置事实回读（数字可比性）**：① `bash scripts/check_xtts_cpu_limit.sh` 绿（8 核在位）；② 该守卫**接线**（e2e-guards workflow 步骤 + `docs/ci-workflows/README.md` 清单行——F-h 缺口本轮补，或给出不接理由并记账）；③ `INTERNAL_API_KEY` 状态记录（影响 /analyze 鉴权路径） | rc=0 + workflow/清单文件:行号回读（RUNBOOK §4.1 证据要求） |

### 组 B：SSE TTFT（F-a / F-d）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 6 | [A] | **TTFT 基线**：登录 → `POST /api/v1/ai/stream` → 计时**首个 `data:` 块**到达，N≥5 短 prompt；p50/p95/min/max 落档（计划期样本 11.7s 作对照锚点；**计时点=首个含 `delta.content` 的 data 块**，非 HTTP 响应头） | 5+ 条原始样本 + 分位数 + 与 11.7s 对照结论 |
| 7 | [A] | **流式渐进性定性（F-a 疑点）**：逐块记录到达时间戳 → 计算"首块后 0.5s 内到达的块占比"；**≥80% ⇒ 定性为整段缓冲（伪流式）** → 进修复队列（根因在 BFF/llm-service 流式链内可定位则 TDD 修，查不出根因**记账写"待查"禁臆断**）；<80% ⇒ 渐进正常，结论反转回填 F-a。**执行期回填（2026-10-03，F-190）**：短流（总时长 < 2×burst 窗口=1s）的 burst **必然 ≥0.8**（判据饱和）⇒ 必须以**长回复鉴别探针**（tail>3s）交叉定性，短流单凭 burst 不得判伪流式 | 每块时间戳序列 + 占比数字 + 定性结论/账本行 |
| 8 | [A] | **SSE 服务端埋点缺口定性**（F-d③）：代码回读确认 `HTTPRequestDuration` 对 `/api/v1/ai/stream` 观察的是**流总时长**（observe 在 `c.Next()` 后）⇒ 结论入 report；**处置**：裁定补 TTFT 埋点（则走 C3 TDD）或**记账不修**（客户端脚本已覆盖测量）——执行者择优，存疑升级 | `metrics.go:179` + `ai_stream_handler.go` 行号回读 + 处置结论 |
| 9 | [V] | **浏览器流式体验对照**：IAB/Playwright 聊天页发消息 → 观察输出到达形态（逐字渐进 vs 一次性整段）+ DOM 级首 token 到达计时 + 截图 `28-*.png` ——与 #7 时间戳结论**互证**（两法结论一致才 PASS） | 截图被查看 + DOM 计时数字 + 与 #7 对照结论 |

### 组 C：TTS 单段与段间 gap（F-b / F-134/135/136 数据面）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 10 | [A] | **TTS 单段冷/热基线**（`/api/v1/tts/phonemes`，标准 40 字文本 + 另一组 17 字对照 ADR/本轮锚点）：冷 N≥3（空闲 ≥5min 后）/ 热 N≥5 → 各自 p50/p95 + **冷热比**（计划期 17 字 36.3/2.0=18x 对照） | 冷/热样本表 + 分位数 + 冷热比 + 与 ADR 19.9s/27.6s 锚点对照 |
| 11 | [A] | **段间排队量化（F-136 实证）**：同时发 2 个 TTS 请求 → 记录两请求 start/end 时间戳；**断言串行**（第二 start ≈ 第一 end，间隙 <1s）→ 量化排队等待 = 第二请求多等的时间；`server.py` 单 worker 行号回读 | 时间戳对照 + 排队时长数字 + `server.py:25/:362` 行号 |
| 12 | [A] | **前端段间 gap 复验（F-06/F-129 面）**：Playwright 驱动真实播放两段（发两条消息凑 TTS 队列 ≥2 段），监听 `ended` → 下一段 `play()`/`playing` 的 gap；**断言 gap < 1000ms**（超出 → FAIL 进分类；enqueue 时序由既有 `queue.test.ts` 单测钉住，本点是运行时净 gap） | 每段 gap 毫秒数 + 断言结果 |
| 13 | [A] | **端到端 TTS 用户路径分解**：发消息 → SSE 流完 → `/tts/phonemes` 200，三段时间戳分列（N≥1，对照 ADR 端到端 27.6s 锚点）；**数字如实记录**，体验好坏留给 M2 裁定 | 三段分解时间 + 与锚点对照 |
| 14 | [M] | **M2：F-134/135/136 处置裁定**：基于 #10-13 量化数据向用户升级——① 首句切段 + `/tts_stream`（F-134，用户 2026-09-23 曾倾向"快"，代价丢真口型）；② 双端点两全（F-135，依赖 F-136 多 worker）；③ 多 worker 改造（F-136，**架构级 → ADR + 决策 39 + D-42**）；④ 暂缓转挂（须账本 owner 转移有据） | 决策登记 D-4x + 账本 F-134/135/136 状态同步 |

### 组 D：SSE 阶梯与资源对照

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 15 | [A] | **SSE 小阶梯 1/2 并发**：短 prompt、每档 N≤4（**LLM 成本受控**，全站压测在边界外）：全 200 + TTFT p50 记录 + 并发下无 5xx/超时 | 每档状态码 + TTFT 数字对照（1 并发 vs 2 并发） |
| 16 | [A] | **资源对照采样**：阶梯压测期间 `docker stats` 采样（≥3 次）：XTTS/llm-service/web-bff CPU、**栈内存余量 ≥1G**（8GB `.wslconfig` 前科）；瓶颈定性记录（如 llm-service 单核打满 / XTTS 空闲） | 采样表 + 余量断言 + 瓶颈定性结论 |
| 17 | [A] | **Grafana 延迟面板数据核对**：`emotion-echo-overview` 的 p95 Latency 面板 expr → Prometheus query **series 非空**（#4 期间产生过流量后）；与 #2/#6 数字方向一致（面板 p95 ≥ 脚本 p50，数量级不矛盾） | PromQL 输出 + 面板数据点截图进 §18 |

### 组 E：落档与收口

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 18 | [V] | **基线报告可读性 + 视觉取证**：baseline 数字汇总（表格/JSON 可读呈现）+ Grafana 延迟面板截图 → **人眼查看**数字无明显错误（单位/错位/占位符） | `screenshots/28-*.png` ≥2 张且被查看 |
| 19 | [A] | **回归钉**：`emotion-echo-web/e2e/performance-baseline.spec.ts`——浏览器发消息断言**首 token < 30s**（宽松回退上限，防流式链彻底坏死；不钉 p50 因 LLM 抖动大）+ 本阶段收口跑绿；全量 `pnpm playwright test` 跑一次，**失败集对照 F-189 基线（不新增本域失败）** | spec 运行输出 + 全量失败集与 F-189 对照结论 |
| 20 | [A] | **收口对账与审计**：F-26 翻 ✅（基线落档 + 埋点被消费 + 复现性过）；F-134/135/136 按 M2 闭环或**如实挂账**（存在未解决 ⇒ 阶段只能 `partial`，A5）；新发现按 RUNBOOK §5 连续编号（**下一号 F-190**）；`python scripts/e2e_stage_audit.py --all` **0 FAIL** | 账本状态截图/输出 + audit 输出 |

---

## 3. TDD 循环划分（RED→GREEN→REFACTOR）

> 运行时验收类测试点（#2-7、#9-13、#15-18）是基线测量与验收断言，不作 TDD 对象；**工具、守卫、修复**必须先红后绿：

| 循环 | RED（先写失败的测试） | GREEN（最小实现） |
|------|----------------------|-------------------|
| C1 | `test_perf_baseline.sh`（照 `test_check_minio_health.sh` 先例）：① percentile 对已知样本 `[1..100]` 断言 p50/p95（官方方法 A4）② 指向关闭端口 → **退出码 ≠0 且 stderr 有显式错误** → 现状（脚本不存在）必红 | `scripts/perf_baseline.py`：stdlib 实现（`statistics.quantiles(n=100, method='inclusive')`）、目标不可达 fail-fast、`--json` 落档 |
| C2 | baseline JSON **schema 守卫**：缺必填字段 / 含 `x|?|TBD` 占位值 → FAIL（负向样本先行）→ 现状（无落档规范）必红 | 落档格式定稿（`stages/e2e-28-performance-baseline/baseline/*.json`）+ 守卫接 e2e-guards |
| C3 | SSE 块到达时间戳**缓冲判定逻辑**单测：喂合成序列（突发=19 块 0.16s / 渐进=均匀间隔）→ 断言判定翻转 → 现状（无此逻辑）必红 | #7 的占比判定实现（`scripts/perf_baseline.py --mode sse` 内） |
| C4 | （**M1/M2 裁定后**才启动）：M1 若裁定门禁 → 阈值守卫先红；M2 若裁定实施 F-134/136 → 对应行为测试先红（如首句切段单测、多 worker 并行度断言）；**裁定为"暂缓"则本循环关闭不执行** | 按裁定展开；架构级改动先 ADR |

---

## 4. 执行期 [M] 决策点

| # | 决策 | 背景 | 备选 |
|---|------|------|------|
| M1 ✅ | **基线数字是否升级为 CI 阈值门禁**——**已裁定 D-42：① 只落档不门禁**（2026-10-03 用户 AskUserQuestion 拍板；二轮数据后再议门禁） | 首基线只有一次；LLM/XTTS 抖动大，过窄阈值会造 flaky CI（训练人忽略红灯）；改 merge 门禁属 RUNBOOK §8 必须升级的事项 | ① **只落档不门禁** ② 宽松绝对上限进 CI（如 TTFT<30s，只拦"坏了"不拦"变慢"）③ 相对回归门（与落档基线比超 2x 报警——需二轮数据才可信） |
| M2 ✅ | **F-134/135/136 TTS 三件套处置**——**已裁定 D-43：② 双端点 + 多 worker 全做（F-135+136）**（2026-10-03 用户 AskUserQuestion 拍板；F-134 归并说明见 D-43，切段现状执行期核实） | 三条均 🟡 owner=E2E-28 且属产品/架构裁定：F-134（快 vs 真口型——用户 2026-09-23 明确倾向"快"但未正式拍板）、F-135（双端点依赖 F-136）、F-136（单 worker 排队，**架构级**） | ① 首句切段 + 流式 TTS（F-134，最快见效，丢真口型）② 双端点 + 多 worker（F-135+136 一起，代价 = ADR + 多实例部署改造）③ 只做多 worker（先解排队，保留真口型）④ 暂缓——本轮只留量化数据，owner 转挂后续阶段（须用户认可，否则 A5 卡 `partial`） |

> **裁决登记**：✅ **D-42（M1）+ D-43（M2）已登 [decisions.md](../../decisions.md)（2026-10-03）**；F-136 实施属架构级 → 同步 [docs/architecture/decisions.md](../../../architecture/decisions.md) **决策 39** + ADR 文件（`adr-2026-10-*`，随 C4 落地）。**两套编号都避开 Lane O 已占号**（并行协议 §三.资源3）。

---

## 5. 收口门槛

1. 20/20 测试点四值判定（PASS/FAIL/BLOCKED/N/A），**BLOCKED ≤ 1/3 不得判 done**；`N/A` 必须附"为什么不适用"证据
2. 名下账本：**F-26 翻 ✅**（基线落档 + 埋点消费 + 复现性）；**F-134/135/136 按 M2 闭环或如实写明"为何仍挂"**（存在未解决条目 ⇒ 只能 `partial`）；新发现连续编号（下一号 **F-190**）
3. `e2e_stage_audit.py --all` 0 FAIL（plan 用「测试点清单」标题 + 整数编号首列，A3 可解析——吸取 E2E-25 F-180 / E2E-26 经验）
4. Playwright 回归钉落地：`emotion-echo-web/e2e/performance-baseline.spec.ts`（收口跑过且绿；全量失败集对照 F-189 不新增本域）
5. M1/M2 裁决登记 decisions.md（D-42 起，如产生）
6. RUNBOOK §12 命令速查补 `perf_baseline.py` 用法（容器网模式 `docker exec -i … python -`）+ C1/C2 守卫接线状态
7. report.md 按 §10 模板 + §7 收口 11 项全过 + 第二方核对（§13.3）后才可判 done

---

## 6. 风险与缓解

| 风险 | 缓解 |
|------|------|
| obs 栈 Exited 22h，交叉验证 #4/#17 依赖它 | 开工复核 #1 起 `--profile obs`；起不来则 #4/#17 按 §4.2 记 BLOCKED（写明为什么不可做），不得静默跳过 |
| LLM 上游抖动使 TTFT 方差大（11.7s 单样本） | N≥5 + 记 min/max + 复现性断言用 [0.5,2] 宽带；不设绝对阈值（M1 前不门禁） |
| 冷热样本互相污染（探针余温） | 冷样本强制"空闲 ≥5min 后首测"写进脚本流程；冷热分文件落档 |
| 阶梯压测烧 LLM 配额 / 拖慢栈 | /analyze 阶梯零成本；SSE 阶梯限 1/2 并发、短 prompt、N≤4；全程监控内存余量（复核 #6） |
| #7 定性"整段缓冲"若根因在上游 DeepSeek/llm-service gRPC 聚合 | 范围判定：BFF/llm-service（Lane E 独占列）内可修则 TDD 修；查不出根因**记账待查**，不臆断（RUNBOOK §5）；不阻其他组 |
| F-134/135/136 M2 未裁定 → A5 卡 done | M2 集中在 #14（≤1/3）；组 A/B/D 不依赖裁定先行；升级时给结论+建议方案（§8 协议），不问"要不要继续" |
| `python3` Store 桩 / Git Bash 路径转换 | 全程 `python`；`docker exec` 探针带 `MSYS_NO_PATHCONV=1`（E2E-27/25 实测坑） |
| F-189 全量 Playwright 基线红干扰收口判断 | #19 只对**照**不修（owner=E2E-30）：不新增本域失败即过；本阶段 spec 独立跑绿是硬门槛 |

---

## 7. 引用

- 账本：[discovered-unresolved.md](../../discovered-unresolved.md)（2026-10-03 对账：归属 E2E-28 条目 = **4，E2E-F-26 / F-134 / F-135 / F-136**；F-06 ✅ 复验面）
- ADR：[adr-2026-09-xtts-cors-resource-quotas](../../../architecture/adr/adr-2026-09-xtts-cors-resource-quotas.md)（8 核实证锚点 + F-136 留账原文）
- 决策：[decisions.md](../../decisions.md)（编号续 **D-42** 起）；[docs/architecture/decisions.md](../../../architecture/decisions.md)（**决策 39** 起，仅架构级）
- 历史阶段：[E2E-22 plan](../e2e-22-monitoring-alerting/plan.md)（Prometheus 查询先例）、[E2E-17 report](../e2e-17-digital-human-tts/report.md)（TTS 历史数字）、E2E-10（SSE 链路已验）
- 工具（本阶段新建）：`scripts/perf_baseline.py` + `scripts/test_perf_baseline.sh`（C1）+ baseline schema 守卫（C2）
- 官方文档：`https://docs.python.org/3/library/statistics.html`（分位数方法，功课④ WebFetch 引用）
- 并行协议：[parallel-tracks.md](../../../_meta/parallel-tracks.md)（Lane E 独占列：`docs/e2e-roadmap/**`、后端、`scripts/`（非 on-device-*）、`e2e/**/*.spec.ts`——本阶段改动全部落 Lane E 列，无需握手）
