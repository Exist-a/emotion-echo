---
stage: cl-01
title: 可观测闭环 —— 执行记录（report）
status: partial
created: 2026-10-14
last-updated: 2026-10-14
---

# CL-01 · 可观测闭环 — 执行记录

> 口径（roadmap §4 CL-01）：端到端跑通「**埋点 → 暴露 → 采集 → 面板 → 被看/告警**」，杜绝"埋了点没人看"。
> 执行协议：[RUNBOOK.md](../../RUNBOOK.md)（六步循环 / 证据有效性 / 收口契约）+ [parallel-contract.md](../../parallel-contract.md)（**B 轨**）。
> **状态 = `partial`**：本阶段范围全部做完且有运行时证据，但**名下仍挂一条未解决账**（CL-F-14）+ **一处裁定偏差待追认**（§10.2 D-05），按 RUNBOOK §6.4 不得判 `done`。

---

## §0 汇总

- **测试点**：19 个（T-01~T-19），**全部有结论**（PASS 16 / 改判落地 1 / 非本阶段范围 2）。四值统计见 §测试点总表。
- **起栈实测**：27 容器 dev + obs profile，持 `.devmode-session`（owner `lane-cl-b`）。
- **机器门禁**：`smoke_observability.py` **60 PASS / 0 FAIL**；`promtool check rules` SUCCESS（总规则 7 → 13）；`go test ./pkg/skywalking/...` 绿。
- **新增产出**：3 个看板（business / reliability / logs，共 25 面板）、1 个告警规则文件（6 条）、1 个共享包（`skywalking/trace`）、1 个共享包（`skywalking/redistrace`）。
- **新账**：CL-F-13（**假账，当轮销账**）、CL-F-14（tracer 启动竞态 → **已按 D-07 修复**）、CL-F-15（outbox 静默回退，归 CL-03）。
- **销账**：CL-F-04（GORM 埋点错挂 processor）✅、CL-F-13（探针错误）✅、CL-F-14（代码侧）✅。
- **裁定**：D-05 追认（接受 `status` 标签替代直接 scrape `/health`）+ **D-07**（CL-F-14 改后台重试）。
- ✅ **CL-F-14 的起栈运行时验证已完成**（2026-10-14 晚）：先因 D 盘满导致 Docker 起不来（containerd SIGBUS），清出空间后恢复；随后用**新/旧镜像 A/B 负向对照**证明修复生效（详见 §3 证据表与 §3.1）。

---

## §1 起点（做之前是什么样）

`docs/closure-roadmap/stages/cl-01-observability/plan.md` §A 的建档结论，全部经实读亲核：

| 面 | 建档期现状 |
|---|---|
| GORM 埋点 | `InstrumentGORM` 把 5 个 op 的回调**全注册到 `db.Callback().Raw()`** —— ORM 的 `Find/Create/…` 永不进 Raw ⇒ **5 svc 的 ORM 查询零 span**（等于全废） |
| Redis 埋点 | `InstrumentRedis` 实现完整但**生产零调用** |
| HTTP 出站 | 全部裸 `&http.Client{}`（无 Transport）⇒ 无 exit span、无 sw8 传播 |
| fusion 两指标 | `RecordFusionCall`/`ObserveFusionDuration` **定义了但生产零 caller** |
| BFF | 无 `SetTracer`，且不在 `svc_wiring_test.go` 覆盖内 |
| 面板/告警 | 19 个自定义指标**只画了 2 个**；4 文件 7 条告警规则 |

---

## §2 实施（做了什么）

按 plan §3 的 9 个 Phase，**全程 TDD（先 RED 再 GREEN）**：

1. **GORM 埋点修复**（Phase 1）：按 op 分派注册到 `Query/Create/Update/Delete/Row`，并给裸 SQL 单列 `begin_raw`（analytics 有 22 处 `db.Raw/Exec`）。RED 断言 = `Callback().Query().Get("skywalking:begin_query") != nil`（修前 nil）。
2. **HTTP 出站**（Phase 2）：新增 `TracingRoundTripper` + `NewTracedClient`；**为让 BFF 能引用而拆出无 gorm 依赖的 `skywalking/trace` 子包**（否则 BFF import 会拖入 gorm，构建直接失败）。接线 8 处（ai-svc 5 + BFF 3）。
3. **Redis**（Phase 3）：实现迁 `skywalking/redistrace` 子包，BFF 在 `buildRedisClient()` 后装配。
4. **DLQ span**（Phase 4）：ai-svc + analytics-svc 的 `publishToDLQ` 建 `kafka-publish-dlq` exit span + sw8 注入；**并把 `kafka-consume` entry span 的 ctx 透传下去**（原实现丢弃 ctx ⇒ DLQ span 无父会被约束③守卫跳过）。
5. **fusion 指标**（Phase 5）：`worker.processOne` 记 `RecordFusionCall` + `ObserveFusionDuration`，新增低基数 `fusionModalityLabel`。
6. **BFF SetTracer**（Phase 6）：成功分支调 `sharedtrace.SetTracer(t)`，守卫扩为表驱动含 BFF。
7. **看板 + 告警**（Phase 7）：新建 business / reliability / logs 三看板 + `cl01-health-business.yml`（6 条规则）。三块看板 JSON 由一次性生成器 [scripts/_gen_cl01_dashboards.py](../../../../scripts/_gen_cl01_dashboards.py) 产出（面板结构统一，手写三份易漂移；生成器同时把「零值也必须出 series」的 `or vector(0)` 约定固化下来）。
   **CI 反查补刀**：首次推送后 `文档守卫总闸` 红 —— 根因是 `node deploy/obs_monitoring.test.js` 的「规则名**双向相等**」断言（dev `deploy/prometheus/rules/*.yml` ↔ k8s `charts/emotion-echo/charts/prometheus/templates/configmap-rules.yaml`），我只落了 dev 侧。**修法**：在 k8s ConfigMap 内联同一份规则；`test_helm_prometheus_render.sh` → PASS=8/FAIL=0（「chart 内联告警与 dev 侧 13 条完全一致」）。**这条断言的价值正在于此：改一侧忘另一侧 = k8s 环境那 6 条告警静默不存在。**
8. **采集增量**（Phase 8）：**D-05 按字面实现被实测证伪**（见 §10.2），改用既有 `status` 标签 + 告警/面板落地。
9. **负向对照 + 收口**（Phase 9）：告警真触发送达、span 真出现在 SkyWalking、smoke 全绿。
10. **CL-F-14 修复**（收口后追加，用户 **D-07** 裁定 = 后台重试）：① 新增 `grpcinterceptor.SetGlobalTracer/GlobalTracer`（可后置注册的全局槽）；② `trace.SetTracer` 同时维护两个全局槽（放 `trace` 包，让不依赖 gorm 的 web-bff 复用同一条路径）；③ 入口路径（gin 中间件 + gRPC server 拦截器）从「构造期快照」改为**请求期现读**全局；④ 新增 `trace.StartTracerBackgroundRetry`（指数退避 5s→60s 封顶、可 cancel / 可 MaxAttempts），6 个 svc 的启动失败分支调用它。**不依赖编排层重启策略。**

**起栈额外挖出的第二层真根因（已修）**：GORM 回调在**注册期快照了 nil tracer** —— 各 svc `main.go` 先 `InitGORM` 后 `SetTracer`（analytics：db connect @:97 → SetTracer @:178），旧闭包把 nil 存下来 ⇒ 回调永久 noop。这是"修好 processor 后 SkyWalking 仍 0 个 gorm span"才暴露的，**正是 CL-01 约束⑤"必须见 span"的价值**。

---

## §3 证据（怎么证明真的生效）

| 面 | 证据（**执行信号，非"已创建"**） | 文件 |
|---|---|---|
| GORM 出 span | 修前 trace **0 个 gorm span** → 修后 **3 个 `gorm.row`** | `evidence/t02-trace-after-gorm-spans.json` |
| Redis 出 span | 登录 trace 含 `redis.hget` / `redis.del` 两个 Exit span；IAB 里肉眼可见 | `evidence/t03-redis-spans-trace.json`、`t03-skywalking-ui-redis-spans.png` |
| HTTP 出站 exit span | trace `113acc30…` 含 ai-svc **`Http`/`Exit` `http.POST /tts`**，挂 gRPC entry 之下 | `evidence/t04-http-exit-span-trace.json` |
| DLQ exit span | 毒消息重试 4 次耗尽 → `kafka-consume`(Entry) → **`kafka-publish-dlq`(Exit)** | `evidence/t05-dlq-span-trace.json` |
| fusion 指标有值 | 真 LLM 分析（`deepseek-chat`）→ 融合 → **`emotion_fusion_calls_total{text,success}=1`** | `evidence/t06-fusion-metrics.json` |
| 看板可读（[V]） | IAB 截图读出 `降级次数 0`、`Worker tick 0.200 ops/s`、图例 `text / success`、`llm_to_late` | `evidence/t11-grafana-business-dashboard.png`（+ reliability / logs 两图） |
| 告警真送达 | 停 kafka-exporter → `PrometheusTargetDown` **firing** → **Alertmanager 收到**（critical） | `evidence/t13-*.json` |
| Loki trace 关联 | `{job="services"} \|= "<trace_id>"` 走 `/query_range` → **2 条流**（web-bff + ai-svc） | `evidence/t14-loki-trace-query.json` |
| smoke 全绿 | **`PASS: 60 check(s)` / FAIL 0 / exit 0** | `evidence/t17-smoke-observability.txt` |
| CL-F-14 修复（单测 + 负向对照） | 回退实现后两条断言**如期变红**：interceptor `"[]" should have 1 item(s), but has 0`；middleware 同型。恢复后 3 包全绿 | `go test ./pkg/skywalking/... ./pkg/middleware/... ./pkg/grpcinterceptor/...` |
| CL-F-14 修复（起栈运行时） | ✅ **PASS（A/B 负向对照）**：新镜像 → 后台补挂第 5 次成功 → SkyWalking **出现 chat-svc（未重启容器）**；旧镜像 → **12 轮全缺席 + 0 条补挂日志** | `evidence/clf14_fixed_image_run.txt`、`evidence/clf14_negative_old_image_run.txt`、`evidence/clf14_{runtime_test,negative_control}.sh` |

---

### §3.1 CL-F-14 运行时验证（A/B 负向对照，2026-10-14 晚）

场景：**停掉 SkyWalking OAP** → 用待测镜像重建 chat-svc（启动期 tracer 初始化必失败）→ 起 OAP → **不重启 chat-svc** → 看它能否自己回到 SkyWalking 服务列表。

| | 旧镜像 `chat-svc:v0.1.16`（无修复） | 新镜像 `chat-svc:cl01-verify`（含修复） |
|---|---|---|
| 启动期日志 | `tracer init failed (warn mode, continue)` | 同（`tracer init failed`） |
| 之后的重试 | **无**（进程余生不再重试） | 后台补挂第 1~4 次失败（退避 5s/10s/20s/40s）→ **第 5 次成功** |
| `后台补挂` 日志条数 | **0** | 5（4 失败 + 1 成功） |
| OAP 恢复后 SkyWalking 服务列表 | **12 轮 × 25s 全部缺席** | **出现 `emotion-echo-chat-svc`（第 1 轮即命中）** |
| 结论 | 负向对照成立 | **修复生效** |

> 关键点：**全程没有重启 chat-svc**。这才证明"后台补挂 + 入口路径按请求现读"真的生效，
> 而不是靠编排层的重启策略兜底（后者是本次刻意排除的替代方案，见 D-07）。

复跑方式：`bash evidence/clf14_runtime_test.sh`（新镜像）/ `bash evidence/clf14_negative_control.sh`（旧镜像对照），
查服务列表用 `python evidence/clf14_sw_services.py chat-svc`（退出码 0=在列表 / 1=缺席 / 2=查询失败）。

## §测试点总表

| # | 测试点 | 判定 | 结论 |
|---|---|---|---|
| T-01 | GORM 回调注册在正确 processor | [A] | **PASS**（含修前 RED 留痕 + raw 单列） |
| T-02 | 真实 ORM 查询在 SkyWalking 出 span | [A] | **PASS**（0 → 3，负向对照） |
| T-03 | Redis 埋点接线 + 真命令出 span | [A] | **PASS** |
| T-04 | HTTP 出站建 exit span + sw8 | [A] | **PASS**（单测 + 起栈实测） |
| T-05 | Kafka DLQ 生产建 span | [A] | **PASS**（单测 + 起栈实测毒消息） |
| T-06 | fusion 两指标接线 | [M] | **PASS**（D-03=接线；起栈 counter=1） |
| T-07 | BFF SetTracer 对齐 | [M] | **PASS**（D-04=对齐；守卫 + 运行时） |
| T-08 | 无 ctx 不硬造根 span | [A] | **PASS**（`ActiveSpan(ctx)==nil` 守卫 + 单测） |
| T-09 | 新看板 JSON 落地 | [A] | **PASS**（Grafana 加载 6 个看板，新增 3 个） |
| T-10 | 每个面板 expr 返回非空 series | [A] | **PASS**（business 11 + reliability 10 面板逐条非空） |
| T-11 | business 看板能答"是否降级/是否 mock" | [V] | **PASS**（IAB 截图人核；此前误判 BLOCKED，见 CL-F-13） |
| T-12 | 新告警规则可加载 | [A] | **PASS**（promtool SUCCESS: 6 rules；总数 7→13） |
| T-13 | 一条告警真触发并送达 | [A] | **PASS**（负向对照，Alertmanager 实收） |
| T-14 | Loki trace_id 面板实机跑通 | [A] | **PASS**（`/query_range` 跨 2 服务命中） |
| T-15 | `/health` 进监控 | [M] | **改判落地**（D-05 字面被实测证伪，见 §10.2，**待追认**） |
| T-16 | 是否新增 exporter | [M] | **PASS**（D-02=不加容器；deferred 记账） |
| T-17 | 观测 smoke 全绿 | [A] | **PASS**（60/0/exit 0） |
| T-18 | 装配守卫仍绿 + 含 BFF | [A] | **PASS**（`go test ./pkg/skywalking/...` 绿） |
| T-19 | `redis-two-layer-cache` 是否纳入 | [M] | **N/A**（D-06 已裁定：本阶段只做埋点接线，双层缓存另议） |

---

## §4 结论

**本阶段范围（roadmap §4 CL-01 的 5 条约束）逐条达成**：

1. 不改指标语义 —— 未改任何既有指标名/标签语义，只新增。
2. 内存红线（默认不新增容器）—— **零新增容器**（D-02 裁定"先做免费那一半"）。
3. span 只在有 ctx 时建 —— `createExitSpan` + `RoundTripper` 都加了 `ActiveSpan(ctx)==nil → noop` 守卫（T-08）。
4. 面板以既有计划为准 —— 按 `observability-business-metrics-wiring.md` 与 `grafana-dashboard-gaps` 的缺口清单落，未重复造。
5. **诚实性** —— 每个"接了的埋点"都有**起栈运行时证据**（span 真出现在 SkyWalking / 指标真有值 / 告警真送达）。

**本阶段实质工作已全部闭环**：

- **CL-F-14 已修完并取得运行时证据**（§3.1 的 A/B 负向对照），满足约束⑤"必须见 span 真出现在 SkyWalking"。
- 两项裁定已回填：D-05 追认（接受替代）+ D-07（后台重试）。
- 落地计划已迁 `legacy-plans/landed/`。
- **唯一未满足的收口契约项 = 第 7 条「第二方核对」**——按 RUNBOOK §7「执行者不得自行宣布阶段 done」，此项需**用户本人或另一个会话**签署。**因此状态仍标 `partial`，等签署后翻 `done`。**

**CL-F-14 归属裁定（用户授权我定）**：该账**保留在 CL-01 名下并就地修复**（不转 CL-03）—— 因为它的文件面（`shared/pkg/{grpcinterceptor,middleware,skywalking}` + 6 svc `main.go`）全在 B 轨，且根因正是"埋点是否真生效"，与 CL-01 口径同源。⇒ 待运行时验证补做后即可销账，CL-01 随之可判 `done`。

---

## §5 经验（可复用）

1. **"修好了"和"真生效"之间常隔着一层**：GORM 埋点修了 processor 后 SkyWalking 仍 0 span，才挖出"注册期快照 nil tracer"。**约束⑤（必须见 span）不是形式主义，它抓到的正是第二层 bug。**
2. **探针本身要先自证**：CL-F-13（"Grafana 面板渲染为空"）是**我自己的探针写错**（用了 Grafana 10.4.2 不存在的 testid）导致的假账。**"同一浏览器里另一个站点正常"不能证明选择器正确** —— 判"UI 坏了"前必须先让探针在同一页面读到已知为真的内容。
3. **`up=0` 与"服务不健康"不是一回事**：`/health` 返 JSON ⇒ Prometheus 解析失败 ⇒ 判抓取失败。**直接 scrape 非 Prometheus 格式的端点会制造永久假红**，比没有监控更糟。
4. **指标名必须从 `/api/v1/label/__name__/values` 现查**：建档期把 fusion 两指标写成 `emotion_fusion_*` 是对的，但同一批的另外 4 个 collector 是 `emotion_echo_fusion_*` —— 两套前缀并存，凭记忆写必错。
5. **"链路全 sent"是假绿**：outbox 行 `status=sent` 但 Kafka producer 初始化失败后会静默改投内存（CL-F-15），事件根本没进 Kafka。**唯一暴露它的是那个 fallback counter** —— 这正是 CL-01 存在的意义。
6. **worktree 起栈比契约写的更麻烦**：`--env-file` 只解决一半；服务级 `env_file: .env.local` 锚点是**相对路径**，worktree 内必须有该文件，且 `tls/`、`tmp/apisix-access.log` 等 gitignored 挂载点在 worktree 缺失时会被 Docker 建成**目录**。

---

## §收口自检（逐条过 RUNBOOK §6）

| # | 契约项 | 结论 |
|---|---|---|
| 1 | plan 每个测试点给四值之一 + 证据 | ✅ 19/19（T-19 = N/A，附 D-06 裁定） |
| 2 | 回归钉已落地且绿 | ✅ Go 单测（`gorm_tracing_test.go` 双层回归钉 / `svc_wiring_test.go` / `dlq_span_test.go` / `fusion_metrics_wiring_test.go`）+ `smoke_observability.py` 60 项 |
| 3 | `report.md` 用模板（起点/实施/证据/结论/经验） | ✅ 本文件 |
| 4 | 账本对账 | ✅ **本阶段名下无未解决 CL-F-xx**（CL-F-04 / CL-F-13 / CL-F-14 均已销；CL-F-15 归 CL-03） |
| 5 | 落地计划迁 `docs/legacy-plans/landed/` | ✅ **已做**（两份源计划均迁入 + `status: landed` + 修路径引用 + `docs/plans/README.md` 标已落地）。BFF 其余 HTTP 出站已在收口后补齐 4 个无归属冲突的文件；**唯一豁免** = `downstream/chat.go`（C 轨独占列，随 CL-02） |
| 6 | roadmap 状态同步 + 分支收口 | ✅ roadmap CL-01 行改 `partial`；PR #211 **保持 draft**（用户 2026-10-14 指示"完成先不要合"） |
| 7 | 第二方核对 | ⬜ **未做 —— 这是唯一未满足项**（需用户本人或另一会话签署；本轮已做机械复核见 §自审，但**不冒充**第二方核对） |

---

## §待决策 / 待追认

| # | 事项 | 需要谁 | 状态 |
|---|---|---|---|
| 1 | **D-05 偏差**：接受"用 `status` 标签替代直接 scrape /health" | 用户 | ✅ 2026-10-14 **已裁定：接受替代方案** |
| 2 | **D-07 CL-F-14 修法**：后台重试 vs fail-fast | 用户 | ✅ 2026-10-14 **已裁定：后台重试** |
| 3 | **CL-F-14 归属**：留在 CL-01 还是转 CL-03 | 用户授权我定 | ✅ **定为留在 CL-01 就地修复**（文件面全在 B 轨，根因同源） |
| 4 | **D 盘空间**：需**管理员**压缩 `D:\DockerDesktopWSL\disk\docker_data.vhdx`（298GB）以恢复 Docker | 用户（需提权） | ⛔ **阻塞中** |
| 5 | PR #211 何时合并 | 用户 | ⬜ 当前 draft（用户指示先不合）；合并前须先补 CL-F-14 运行时验证 |

## §待办（**已于 2026-10-14 晚完成**，保留供复跑参考）

### 0. 先恢复 Docker（**需管理员**；2026-10-14 已穷尽非管理员路径）

根因：D 盘剩余 0 → 数据盘 `docker_data.vhdx`（实际 298 GB）无法扩展 → containerd 读
bbolt 页 **SIGBUS** → `Docker Desktop is unable to start`。

**已备好双击脚本**（放在宿主桌面，不用记命令）：
`桌面 / 压缩Docker数据盘.cmd` → 调用同目录 `compact-docker-disk.ps1`。
它会①停 Docker 与 WSL ②提权跑 `diskpart`（attach readonly → compact → detach）
③打印压缩前后的 VHDX 大小与 D 盘可用空间。
**第②步会弹 UAC，必须点「是」**（不点即取消，压缩不会发生）。

**为什么不能自动完成（本轮实测排除，非推断）**：

- `Start-Process -Verb RunAs` → `InvalidOperationException: 操作已被用户取消`（UAC 弹窗无人点）。
  本机 `ConsentPromptBehaviorAdmin=5` + `PromptOnSecureDesktop=1` ⇒ 必然弹窗；当前 shell `isAdmin=False`。
- `wsl --manage docker-desktop --set-sparse true` → 报成功，但只作用于**发行版自己的**
  `main\ext4.vhdx`(0.1 GB)，与数据盘无关（D 盘可用 0 → 0.02 GB）。
- `wsl --mount --vhd ...docker_data.vhdx` 后 `fstrim -v` → 报 **712.6 GiB trimmed**，
  且 `/sys/block/sde/queue/discard_max_bytes` 非 0，但 `wsl --unmount` 后宿主文件**未缩**
  （仍是 320,203,653,120 B）⇒ 该挂载路径不把 TRIM 透传到动态 VHDX。

### 1. 然后做 CL-F-14 的起栈运行时验证（唯一未完成项，步骤已固化）

1. 确认 Docker 可用（`docker ps` 有响应）且 D 盘可用 > 5 GB。
2. `docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml -f deploy/docker-compose.cl01-verify.yml build emotion-echo-chat-svc`
   （override 文件已备好：把 chat-svc 镜像 tag 换成带轨后缀的 `cl01-verify`，不覆盖公共 tag）。
3. `docker stop emotion-echo-sw-oap` → `docker compose ... up -d --no-deps emotion-echo-chat-svc`（**OAP 停着**重启 chat-svc ⇒ 启动期 tracer 初始化必失败）。
4. 断言 `getAllServices` **不含** chat-svc（负向对照：确认它真的没在追踪）。
5. `docker start emotion-echo-sw-oap` → **不重启 chat-svc**，等 ≤90s（退避 5s/10s/20s/40s）。
6. 断言 `getAllServices` **出现** chat-svc + `docker logs emotion-echo-chat-svc | grep 后台补挂成功`。
7. 通过则把 CL-F-14 账本行改 `✅ 已解决（含运行时证据）`，CL-01 判 `done`。

## §自审（机械复核，**不是**第二方核对）

收口前对自己报告逐条做的机械回查（第二方核对仍需另一会话/用户）：

- 19 个测试点的"结论"与 `evidence/` 文件**逐一对应**，无"已创建"类存在性措辞。
- T-02/T-04/T-05/T-06/T-13 的负向对照都留有"修前/触发前"的记录，不是只有正例。
- CL-F-13 假账已按 `invalid-probe-evidence-pattern` 的教训**销账并写明探针错在哪**（不掩盖）。
- 所有裁定（D-01~D-07）都能在 `plan.md §7` 找到原始结论，无"我以为是这么裁的"。
