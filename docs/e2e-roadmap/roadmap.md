---
status: active
priority: critical
created: 2026-09-17
last-refresh: 2026-10-06 (**E2E-28 性能与延迟基线 ✅ done**（F-135 实测否决 D-44 销账；TTS 加速 = 流式 TTS API 专项 E2E-F-198，owner=下一阶段）：20/20 测试点终判（18 PASS + 2 FAIL-已分类）+ 8 修复 PR（#151~#158，D-42/43 双裁定）+ 基线 23 份 JSON 落档 + §8 三节回填 + audit 30 阶段 0 FAIL + 第二方核对两轮通过（首轮 3 项 FAIL 已修正）+ IAB 收口复测通过 + 收口 PR（本 PR）；**F-135（双端点流式播放）未实施 ⇒ 阶段 partial**（账本 owner=E2E-28 仍挂，D-43 第三件，升级选项见 report §6①）；收口新账 F-195/196/197；详见 [report.md](stages/e2e-28-performance-baseline/report.md)；**下一阶段 = E2E-29（待建档）**）
type: e2e-stage-roadmap
---

# E2E 阶段式测试路线图（长期）

## R 系列状态（2026-09-18 收尾后）

> **R-01 ✅ 完成；E2E-07 已解阻塞，可直接开工。** R-02 / R-03 各留少量条目（见下表），
> **可与 E2E-07 并行**，但须先于"下一批阶段的收口"落地。

### 本轮（2026-09-18 核对 + 修复）结论要点

**核对推翻的两处既有结论**（均附可复现证据）：

1. **找回密码链路是三层同时坏的**，不是一层。① APISIX 白名单缺 `/api/v1/auth/verify-security-answer`（落到 route 100 的 jwt-auth ⇒ 恒 401）；② **BFF 的 gRPC 客户端是 `not implemented` 桩**，而 BFF 默认走 gRPC；③ user-svc 拦截器匿名跳过清单漏配。三层全修后**端到端实测**：错答案 401 / 正确答案 200 / 未知用户 401。
2. **`-race` 不是环境问题，是真实数据竞争**：CI 上 `shared-test`/`web-bff` 在 `-race` 下通过，5 个业务模块一律报 `WARNING: DATA RACE`（根因同源：`grpcserver.(*Server).listener` 无同步）。原记"永久降级、残留风险=无"**错误**，已修 5 模块并恢复 CI `-race`。

**同时解除的阻断项**：

- **main 写保护自锁死**（强制 PR + 需 1 approve + 仓库仅 1 协作者 ⇒ 谁都交付不了）→ approvals 1→0，并接通 **18 个 required checks**（只纳入无 `paths` 过滤的 job，符合 D-08），**红线实测能拦**（失败 check → `mergeStateStatus=BLOCKED`）。
- **E2E-F-69 密钥明文**（公开仓库内联 APISIX 管理员密钥与 JWT 密钥）→ 轮换 + 明文清零 + admin API 收回 `127.0.0.1` + 新增密钥扫描器（接入 CI）。

**仍明确"不补"的项**（理由见 [remediation.md](remediation.md)「判定记录」，判据：*现在不做会不会造成未被发现的危害*）：
R-02 #1~#3（report 模板化 / `[V]` 截图 / 账本对账）、R-03 #7 批量脚本负向测试、R-03 #10 的覆盖率·集成测试·Playwright 进 CI 等大项。
**代价**：审计器对 E2E-03/04/05/06 仍报 A1/A2/A3/A4/A6 —— 这是**正确信号**（真实未还欠账），不是回归。

**E2E-20 多实例并发正确性 ✅ done（2026-09-28 §13.3 用户批准，PR #118 收口）** → 详档 [STATUS v4](stages/e2e-20-multi-instance-concurrency/STATUS.md)。下一阶段按 roadmap 排期推进

> **当前激活**：**E2E-29（待建档；含 E2E-F-198 TTS API 专项——用户已拍板 D-44，等 TTS_API_KEY）**——**E2E-28 性能与延迟基线 ✅ done（2026-10-06 收口：F-135 实测否决销账）**；20 测试点终判完成（18 PASS + 2 FAIL-已分类）+ 8 修复 PR + 基线 23 份 JSON + **§8 回填✅ / audit 30 阶段 0 FAIL✅ / #17 带数据截图✅ / 第二方核对两轮通过✅ / IAB 收口复测✅ / 收口 PR（本 PR）**；**F-135 🔴（⇒ partial 依据，D-43 第三件，owner=E2E-28 仍挂）**；收口新账 F-195/196/197。收口记录 [report.md](stages/e2e-28-performance-baseline/report.md)。上一阶段 E2E-27 ✅ done（收口记录 [report.md](stages/e2e-27-object-storage-minio/report.md)）。**E2E-28 进行中记录**：**详档 2026-10-03**：[plan.md](stages/e2e-28-performance-baseline/plan.md) 20 测试点 + [report.md](stages/e2e-28-performance-baseline/report.md)——D-42（只落档不门禁）/D-43（双端点+多 worker 全做）双裁定；**F-136 ✅**（根因深化：async handler 直调阻塞推理锁死事件循环为真因、"单 worker"是表象；synth_pool 线程池零内存解串行；ADR-2026-10 + 决策 39；修后 #11 双并发 [36.4,38.9] 重叠 vs 串行预期 75s）+ **F-134 ✅**（manager+sender **双层**切段——#13 首轮 ttsFirst==sseFinish==6785 同刻暴露 sender 第二层 debounce，PR #158 修后 5350<5613 中流发出）+ **F-26 ✅**（基线 23 份 JSON：/analyze 热 p50≈1.3ms 复现 1.021、SSE TTFT p50=653ms 冷热双峰、TTS 三层冷 50.9s/13-14s/热 13-30s、段间 gap=269ms、修前后排队对照）；9 个 PR（#151~#158）TDD 修 6 类真缺陷 + 环境 3 坑处置（Vite 504 缓存 / WSL 8GB 冻结恢复链 / 宿主端口重注册）；新账 **F-190~194**（burst 判据饱和✅ / TTFT 无埋点🔴 / 宿主 Reset 簇🔴 / 测点加性差✅ / TDD 门禁 <5 行阈值🔴）；回归钉 `performance-baseline.spec.ts` 5 用例。**上一阶段 E2E-27 对象存储 MinIO ✅ done**：**详档 2026-10-03**：[plan.md](stages/e2e-27-object-storage-minio/plan.md) 20 测试点 + [report.md](stages/e2e-27-object-storage-minio/report.md)——**D-40**（M1：avatar+uploads 网关相对+反代+存量惰性兼容，F-116 翻 ✅）/**D-41**（M2：MinIO 端口 127.0.0.1 限定+匿名读边界文档化）两裁决落地（来源如实标注：升级时用户未选定、执行者按推荐落地，待追认）；9 组 TDD 修复 6 真缺陷（voice 缺失对象 500 违 ADR→404 / 停机上传挂起 15s→503@2.6s / smoke 三缺陷致守卫从未真跑通→ALL PASS+CI 接线 / F-116 绝对地址全链相对化+双反代端点 / 反代 HEAD 双层 404→r.HEAD+seed 方法表 / 头像更新不删旧→孤儿治理）；回归钉 object-storage.spec.ts 2/2 + 双截图人工目视；新账 F-186（readiness 无 storage，归 E2E-23）/F-187（voice 同型两缺口，归 E2E-16）/F-188（digest 守卫不覆盖 compose，归 E2E-30）。**上一阶段**：E2E-26 ✅ done 2026-10-03 收口 20/20 PASS，收口记录 [report.md](stages/e2e-26-tracing-skywalking/report.md)）。**E2E-26 封存记录**：**详档 2026-10-03**：[plan.md](stages/e2e-26-tracing-skywalking/plan.md) 20 测试点 + [report.md](stages/e2e-26-tracing-skywalking/report.md)——计划期 4 新事实（UI/OAP 宿主端口全封 / smoke 契约 9 双重空转 / queryDuration 归因 / 双 trace ID）全部闭环为 **D-37/38/39**；途中修复 9 组 TDD（6 真缺陷：bff 拨号顺序、5svc SetTracer 全局 tracer 零调用、server 端 StartEntry 不提取 sw8+不返承载 ctx、client injector 空串覆盖 sw8、health 探针 span 噪声、TraceIDFromSW8 未 base64 解码半修）；回归钉 skywalking-trace.spec.ts 3/3 + 3 截图人工验收；新账 F-182/183/184 转挂、F-185 当轮闭环。**E2E-25 封存记录**：**详档已建档 2026-10-02**：[plan.md](stages/e2e-25-apisix-gateway/plan.md) —— 20 个测试点。计划期调研确认：① 名下 4 条账本全部成立——**F-154**（6 个 nacos upstream 无 `checks` 段，节点健康 100% 外包 Nacos 心跳，D-30 实测摘除滞后 ≥75s）+ **F-137**（BFF 侧 dev backoff retry 已是"部分实现"，APISIX 侧零治理；与 F-154 一并治、不拆两次改动）；**F-139**（seed 全量 PUT 无 GET-before-PUT、无漂移检测、无 CI 兜底，Stage 112 route 116 是先例）；**F-145**（policy=redis 已持久化但跨节点不放大零验证）；② 计划期 2 项新事实——`PLUGINS_JSON` 死变量（限流 policy 与实际生效链分叉）+ seed_test.js 弱断言（`set -euo pipefail` 断言被注释满足）+ etcd 停摆时 APISIX 容器 healthcheck（纯 TCP 9080）仍 healthy 的网关假绿；③ 官方文档**无明文承诺 active checks 对 discovery 动态节点生效** ⇒ 执行期 [M] 决策点 M1 先验证再定方案。**开工第一天先按 plan §0.2 复核清单恢复环境基线（infra 全停）并复核 F-a/F-c 运行时现状**）。**上一阶段**：**E2E-24 消息链路** ✅ **done**（2026-10-02 收口：[report.md](stages/e2e-24-message-pipeline/report.md) 20/20 PASS + 第二方核对两轮通过；outbox→Kafka→consumer→DLQ 全链 + D-33 回放工具，F-12/F-149/F-150/F-174/F-175 闭环）。
> **上一阶段（已完成）**：**E2E-24 消息链路**（outbox→Kafka→consumer→DLQ 全链 + 重试/死信/回放；无依赖前置。详档 [plan.md](stages/e2e-24-message-pipeline/plan.md) —— 20 个测试点；计划期调研确认三件事：① 账本 **E2E-F-149 未修**，consumer 写库 `ON CONFLICT (event_id)` 撞分区表 `UNIQUE (event_id, occurred_at)` ⇒ 每条消息消费必失败、重试 3 次进 DLQ，修复列为本阶段第一个 TDD 循环并转挂 owner；② **E2E-F-12/F-150 成立**：全仓无任何回放工具，`chat-events-dlq` 已堆积 24 条死信无处置机制，DLQ 回放 + outbox dead 行重置列为本阶段补齐能力（形态为执行期 [M] 决策点）；③ 历史决策 D1/D2/D4 已落地，D3（attempts 不跨 rebalance）/D5（relay 多副本互斥）未落地、明确划出边界。**开工第一天先复核 F-149 运行时现状与死信实时数**，防止任务书事实表过期）。
> **上一阶段**：**E2E-23 健康检查与服务发现** ✅ **done**（2026-09-30 用户批准收口）。**PASS 38 / FAIL 0 / BLOCKED 0 / N/A 2**。收口历程：未完成清单 7 → 0（剔 3 项不属本阶段、1 项失实、1 项早已完成）；**归属本阶段的未闭环账本归零**（F-107 / F-156 / F-163 全翻 ✅）。途中修掉 3 个真门禁/真产品缺陷：① 审计器账本解析器对跨行 Markdown 静默丢弃 ⇒ A5 看不见 4 条账本，阶段可带未解决账本判 done；② 解析器把转义竖线当分隔符 ⇒ owner/status 取错；③ 5 个服务健康探针在**降级启动时报健康**（`dbOK := true` + `if repo != nil` 跳过）⇒ `/health/ready` 返 200、容器判 healthy、APISIX 照常路由而后端全挂，零告警。详档 [plan.md](stages/e2e-23-health-discovery/plan.md) + [report.md](stages/e2e-23-health-discovery/report.md)：D-29 liveness/readiness 分离 + D-31 编排层 `NACOS_REQUIRED` 声明 + D-30/32 上游超时与探活语义修正；`db-migrate` ExitCode 1→0（F-151 闭环）+ infra healthcheck 6→16 + `NACOS_HOT_RELOAD` 14 个参数运行时逐项验证。**残留**：F-96 的「代码半」（svc 侧 Postgres 单次连接失败后无重试、进程内永不自愈）主归属 E2E-06，E2E-23 侧的编排与探针两半已闭环。
> **更早**：**E2E-22 监控告警** ✅ done 2026-09-29（18/18 测试点 PASS + **第二方核对通过**：RUNBOOK §13.3 十七条 + 三组负向对照亲自复跑，AP-01/02/03/05/06/11 均未发现；核对方提 6 项问题已全部处置。详档 [plan.md](stages/e2e-22-monitoring-alerting/plan.md) + [report.md](stages/e2e-22-monitoring-alerting/report.md)，核对记录见 report §9）：Prometheus 抓取 / Grafana 面板 / Alertmanager 通知渠道。**无依赖前置、RUNBOOK §9 无阻塞决策门**。计划期实测三处硬事实：① **观测栈当前是死的且零告警** —— `emotion-echo-prometheus` / `alertmanager` / `kafka-exporter` / `sw-oap` / `minio` / `kafka` 六容器于 `2026-09-29T03:12:40Z` **同刻 `Exited(255)`**（`OOMKilled=false`、日志尾部正常 ⇒ 外部终止事件非自身崩溃），`localhost:9090/9093` 探测返 `000`；**发告警的东西自己死了，故无人告警** —— 阶段核心测试点即针对此类"黑瞎子"缺口。② **`llm-service` 暴露 `/metrics` 但 dev 侧无 target**（`emotion-llm-service/main.py:215` vs `prometheus.yml:26-32`），而 k8s 侧有 annotation ⇒ 双栈不一致。③ **k8s 侧告警规则为 0 条且零报错** —— `charts/.../prometheus` 的 `rule_files` 指向 `/etc/prometheus/rules/*.yml`，但该 ConfigMap 的 `data` 只有 `prometheus.yml` 一个 key、deployment 也无对应 volume 挂载 ⇒ 空 glob 静默失效（与 E2E-F-147 权限静默失效同型）。另：`db-migrate` `Exited(1)`（`FATAL: Postgres 30s 内未就绪`，迁移本身 31 文件全成功）属启动竞态，**范围外归 E2E-23** —— ✅ **已于 E2E-23 闭环**（`migrate.sh` 加 `PG_WAIT_MAX_SECS`（默认 120s）+ 递增退避；ExitCode 1→0 有运行时前后对照，守卫 `scripts/test_migrate_pg_wait.sh`）。
> **更早**：**E2E-21 日志体系** ✅ done 2026-09-29（13/13 测试点 PASS + 三条留账 F-146/147/148 收尾闭环；→ [report.md](stages/e2e-21-logging-observability/report.md)）。其计划期实测：Docker Desktop 上 `/var/lib/docker/containers`（**29 个**容器目录）与 `/var/run/docker.sock` 均可见 ⇒ [ADR-2026-09-loki-aggregator-dev](../../architecture/adr/adr-2026-09-loki-aggregator-dev.md) §2.1/§2.3 原定的 docker_sd 采集路线可行（ADR 该决策**从未落地**）；Go 1.26 的 `slog.SetDefault` 已把 stdlib `log` 桥接进 slog handler ⇒ **"159 处 stdlib log 需迁移"的初判被推翻**，阶段范围收窄为采集入口 + traceId 生产侧 + smoke 断言收紧。
> E2E-18 缓存层（✅ done，2026-09-28 §13.3 第二方核对用户批准）：LRU 默认启用 TDD + D-27 Redis 保留决议，详档 [report.md](stages/e2e-18-cache-layer/report.md) + [STATUS v2](stages/e2e-18-cache-layer/STATUS.md)。

> **2026-09-19 治理轮（07~10 收口审计）**：对 E2E-07/08/09/10 跑 `scripts/e2e_stage_audit.py` 发现四个阶段**全部 FAIL**（E2E-11 是唯一干净的近期阶段），错误模式与 R-02 判定过的完全同型：`screenshots/` 全为 0 张、report 非 §10 模板（缺「收口自检」/无汇总行）、plan 与 roadmap 状态未同步。用户决议 = **轻量补账 + 四阶段降 `partial`**（取证补拍另排一轮，账本 E2E-F-90）。
> 即：**R 系列补救只回填了 E2E-01~06，07 之后的收口仍在复发同一模式** —— 这正是"执行者自证的完成不可信"的再次验证。

> **2026-09-19 旧账清偿计划**：[debt-paydown-plan.md](debt-paydown-plan.md) —— 90 条账本中未了结 48 条，实测分三类（陈旧行 ~8 / 真欠账 ~22 / 路线图工作 ~18）。三轮波次：**波 1 把账本说真话**（目标 `audit --all` = 0 FAIL）→ **波 2 堵漏**（5 个门禁接 CI 并设 required，**这是 07~10 复发的根治**）→ **波 3 补真证据**（取证补拍 + proto 合并变更）。波 4 明确不消项已逐条记录理由。

> **E2E-10 ✅ done**（2026-09-19 落地 → 2026-09-19 由 done 降 partial → **2026-09-21 取证补拍轮恢复 done**）：12/12 测试点结论未变，回归钉 `chat-core.spec.ts` 双 project **24/24 PASS**（chromium 12/12 + mobile 12/12），3 个 `[V]` 点（#9 长消息布局 / #10 空对话状态 / #12 Markdown 渲染）补 6 张截图（`screenshots/e2e-10-{09,10,12}-{long-message,empty-state,markdown}-{chromium,mobile}.png`）。降级原因全部解决：截图 ✓ / 汇总行 ✓ / §10 收口自检 8 条全勾 ✓。详档 [stages/e2e-10-chat-core/report.md](stages/e2e-10-chat-core/report.md) §10 补账记录

> **E2E-09 ⚠️ partial**（2026-09-19 由 done 降级）：12/14 测试点结论未变（密保弹框 + 验证码删除 + bcrypt 契约，本轮实跑 23 条契约 + 全量 410 条前端测试通过）；降级原因是 0 张截图、#12/#13 未执行、缺模板章节。#12/#13 留账 E2E-F-77/78。PR #11 已合并。

> **E2E-08 ⚠️ partial**（2026-09-19 由 done 降级）：12/12 测试点结论未变（软删除 6 条用例本轮实跑 6/6 PASS）；降级原因是 0 张截图、缺模板章节。详档 [stages/e2e-08-conversation-management/report.md](stages/e2e-08-conversation-management/report.md) §7~§9

> **E2E-07 ⚠️ partial**（2026-09-19 由 done 降级）：密保问题流程功能可用，但原报告**未按 plan 的 13 个测试点逐点记录结果** ⇒ 补账后 7 PASS / 6 BLOCKED（BLOCKED = 证据未记录，非功能坏）、0 张截图、回归钉只记"编写"未记运行。详档 [stages/e2e-07-password-recovery/report.md](stages/e2e-07-password-recovery/report.md) §七~§十一

## 下一阶段（进行中）

**E2E-15 报表 Dashboard** ✅ **done**（2026-09-21 落地 + 当日收口）：14/14 测试点 + 24/24 Playwright（chromium + mobile）+ 16 张截图。修 2 个真实缺陷：① E2E-F-10（mental_health_assessments 触发器补写 → trigger runner.Save + rebuild analytics-svc:v0.1.8）；② E2E-F-14 同型残留（4 dashboard 空态 div v-else-if + 静态契约钉）。详档 [stages/e2e-15-reports-dashboard/report.md](stages/e2e-15-reports-dashboard/report.md)

> E2E-14 人格量表与 AI 提示词定制 ✅ **done**（2026-09-20 落地 / 2026-09-21 关账轮收口：**16/16 测试点全 PASS**，含 §8.3 新增的算分修复（E2E-F-97）+ `scoreKind` 语义化两条；用户决议暂搁 LLM-as-judge 路线，原 §8.2 报告的"降 partial"理由——账本留 E2E-F-98 判官方法——已被本轮解除，详见账本关账轮记录）。E2E-13 心理测验 ✅ done。E2E-12 设置页 ✅ done。E2E-11 partial（留账 2 条）。E2E-07~10 partial（取证缺口）。
> **下一阶段开工前**：E2E-17 ✅ **done**（2026-09-24 最终收口，PR #77：F-140 volume clamp=「嘴动没声音」最终根因，web:v0.1.7 部署 + IAB play()→PLAYING 实证；用户签字；详档 [stages/e2e-17-digital-human-tts/report.md](stages/e2e-17-digital-human-tts/report.md)；留账 F-137/139→E2E-25、F-134/135/136→E2E-18 已转挂 E2E-28、F-130→E2E-03）。**当前 = E2E-19（数据库层验证）✅ done**（2026-09-28 §13.3 第二方核对用户批准；commit `a8978df`）+ **E2E-18 ✅ done**（同轮 §13.3 批准）。**下一候选 = E2E-20（多实例并发）**，plan 已建档 2026-09-28 (PR #103)，开工前置 = E2E-18 done（**已满足**）—— [report.md](stages/e2e-18-cache-layer/report.md)。

## 排期总表（30 阶段）

### 第一批：基础改造与门槛

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-01 | 登录会话持久化 | cookie 存储/刷新恢复/过期/登出/remember-me 全周期 | 注册、找回密码 | ⚠️ done**（契约欠账）**：`[V]` 截图 0 张、测试点 #10 无实质证据、report 误引账本编号（写 F-23 实为 F-38）→ [R-02](remediation.md) |
| E2E-02 🔧 | 项目目录清理 | 清理无关文件/目录、补 gitignore、归档测试证据 | 不动业务代码 | ✅ done（最接近合规；唯一瑕疵：#6 可验证却标 BLOCKED） |
| E2E-03 🔧 | **CI/CD 门槛** | 落地可跑的 `.github/workflows/`（现全仓零 CI，模板从未执行） | 复杂流水线/部署自动化 | 🟡 **partial**（契约欠账）：report 非模板、**21 点中 13 点无结果**、无汇总行、账本未闭环、`-race` 被降级当已解决 → [R-02](remediation.md) / [R-03](remediation.md) |
| E2E-04 🔧 | **前端工程化门槛** | ESLint/Prettier 引入 + typecheck 103→0 + SPA 产物 smoke + Playwright mobile project + browserslist + a11y 基线 | 全量代码重构 | 🟡 **partial**（契约欠账）：**4 个测试点假 PASS**（其中 #4 与代码事实相反）、账本一行未改、回归钉从未运行 → [R-02](remediation.md) |
| E2E-05 🔧 | **文档与代码一致性收口** | 11 个校验脚本接入 CI + 更正 3 处实测失真 + digest 假绿修复 + 2 个 migration 脚本 | 通用文档检查器 | 🟡 **partial**（契约欠账）：`plan.md` 至今 `status: pending`、汇总行留占位符 `PASS x`、账本未更新、**其接入的 `doc-drift-check` 在 main 上持续红** → [R-02](remediation.md) |

### 第二批：数据库与认证

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-06 🔧 | 数据库改造 | 删死字段（phone/email/status）+ 加密保问题字段（D-01）+ 加 schema_migrations 版本表 + 统一软删除 + 修 db README | 连接池调优 | ⚠️ **partial**：R-01 修复了安全漏洞/注册断裂/测试编译；剩余：integration test 未补、演示账号解耦（R-02 #9~10） |
| E2E-07 | 找回/重置密码 | 三步向导改造为**密保问题**流程（D-01=C）+ 端到端跑通 | 短信/邮件服务 | 🟡 **partial**（2026-09-19 由 done 降级）：功能可用（API 层实测 + DB 落库），但**原报告未按 13 个测试点逐点记录** ⇒ 补账后 7 PASS / 6 BLOCKED（=证据未记录）、0 张截图、回归钉只记"编写"未记运行 → [report §七](stages/e2e-07-password-recovery/report.md) |
| E2E-08 | 历史会话管理 | 会话列表/删除/pin/重命名/分组 | 消息内容同步 | 🟡 **partial**（2026-09-19 由 done 降级）：12/12 结论未变（软删除 6 条用例本轮实跑 6/6 PASS），降级因 0 张截图（4 个 `[V]` 点无视觉证据）+ report 缺模板章节 → [report §7](stages/e2e-08-conversation-management/report.md) |
| E2E-09 | 注册流程 | 注册全流程（含密保问题设定步骤） | — | 🟡 **partial**（2026-09-19 由 done 降级）：12/14 结论未变（密保弹框 + 验证码删除 + bcrypt 契约，本轮实跑 23 契约 + 全量 410 前端测试通过），降级因 0 张截图 + #12/#13 未执行（E2E-F-77/78 留账）+ 缺模板章节 |

### 第三批：聊天与周边

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-10 | 聊天核心链路 | 发送/SSE 流式/错误处理/中断重试 | 多模态 | ✅ **done**（2026-09-19 落地 / 2026-09-19 临时降级 / 2026-09-21 取证补拍轮恢复）：12/12 测试点结论未变。回归钉 `chat-core.spec.ts` **24/24 PASS**（chromium 12/12 + mobile 12/12），3 个 `[V]` 点补 6 张截图。降级原因全部解决。详档 [report.md](stages/e2e-10-chat-core/report.md) §10 |
| E2E-11 | 我的空间 | 资料修改/头像上传（MinIO）+ 现有 3 个对话行为图表有数据可渲染、空态可读 | 测评图表（归 E2E-14） | 🟡 **partial**（2026-09-19：**12/12 测试点全 PASS**；首轮 8 commit 修 4 组代码现状缺陷 + 3 组浏览器实测缺陷 + 头像链路 3 层丢弃点；**复查轮再修 5 项**：GetUserById/Login/Register 三处映射漂移收敛、maxConsecutiveDays 恒 0、E2E-F-36 内容裁剪、头像服务端 2MB 上限失效、InMemory 替身保真度。**判 partial 而非 done**：账本仍有 2 条归属本阶段未解决（E2E-F-80 年龄不落库、E2E-F-81 深度指标 NULL session 语义）。回归钉：Playwright 10 用例 + 静态契约钉 21+1 项 + Go 侧新增 25 条。详档 [stages/e2e-11-my-space/report.md](stages/e2e-11-my-space/report.md) §8 复查记录） |
| E2E-12 | 设置页 | 字体/主题切换与持久化 | — | ✅ **done**（2026-09-20：**12/12 测试点全 PASS**，BLOCKED=0。四层 config 契约 + 跟随系统运行时监听（matchMedia）+ SSR 首屏无闪烁（ee_theme 镜像 cookie）+ 契约漂移清理×3。Playwright 12/12、vitest 17 条（全量 427）、typecheck 0 错、IAB 实测 + 6 张截图。E2E-F-82 已解决。详档 [report.md](stages/e2e-12-settings/report.md)——含首轮假 BLOCKED / 挪球门的纠正记录） |

### 第四批：决策支持与多模态

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-13 | 心理测验链路修复 | 列表→答题→提交→结果查看 跑通（三层契约错位见 E2E-F-02）+ 补量表种子数据 + `/question` 页区分两类量表 | 人格量表内容设计 | ✅ **done**（2026-09-20：12/12 PASS，BLOCKED=0。6 处契约错位修复 + 种子数据 PHQ-9/GAD-7 + BFF HTTP 绕过 + ADR。Playwright 18/18 + Go 10/10 + 边界 10/10。E2E-F-02/03 关闭。详档 [report.md](stages/e2e-13-quiz/report.md)） |
| E2E-14 🔧 | 人格量表与 AI 提示词定制 | D-02：新增人格量表（设计+种子+维度评分器）→ 结果模型扩展 → 注入 AI prompt → user 页测评图表 | — | ✅ **done**（2026-09-20 落地，2026-09-21 关账轮收口）：**16/16 测试点全 PASS**（含 §8.3 新增 E2E-F-97 算分修复 + `scoreKind` 语义化两条）。BIG5 量表（NEO-FFI-30 改编，30 题 × Likert 5 点 + 6 反向题）+ `BigFiveScorer` 五维度评分 + 画像注入 BFF `system prompt`（基底人设不变、画像只调整"怎么说话"）+ 双通道分档（绝对档 + ipsative 个体内相对档，随机 10000 组覆盖率 66% → **95.5%**）+ `/question` 分区 tab + 结果雷达图 + 我的空间画像区块。**端到端抓包**证实注入报文（含降级路径：未做人格量表时不编造）。途中揪出并修 3 个真实缺陷：列表 description 恒空（proto 缺字段/gRPC 丢弃 ⇒ BFF `listSurveys` 走 HTTP 绕过）、雷达容器宽度塌缩 100px（grid 子项 stretch 丢失）、雷达轴标签裁剪（ECharts `radius` 显式 62%）。**客观证据**：对立画像 A/B 三条消息回复对照（字数/问句数/安抚词/追问推进词）方向 **5/5 类 × 3/3 条**全中（确定性文本度量，不依赖判官）。Playwright 20/20（双 project）+ Go 19 条 + 前端 31 条 + 全量 vitest 458/458、typecheck 0 错。账本 E2E-F-04/95/98 关闭（98 = 用户决议暂搁 LLM-as-judge 路线，触发复跑的条件已写入条目）。详档 [report.md](stages/e2e-14-personality-ai-prompt/report.md) |
| E2E-15 | 报表 Dashboard | 数据内容正确性/日期切换/历史 chartData=[] 复查 | — | 🟡 **partial**（2026-09-29 由 done 降级：新增 E2E-F-149——分区表化后 `ON CONFLICT (event_id)` 失效，**chat-svc 每发一条消息 analytics-svc 消费必失败**，报表数据源实际写不进任何行；原 14/14 测试点结论未变）：14/14 测试点 + 24/24 Playwright（chromium + mobile）+ 16 张截图。**修 2 个真实缺陷**：① E2E-F-10 mental_health_assessments 触发器补写（trigger runner.Save + PostgresMentalHealthRepo.Save + rebuild analytics-svc:v0.1.8 + 7 条 trigger 单测）；② E2E-F-14 同型残留（4 dashboard 空态 div v-else-if + 12 用例静态契约钉）。**端到端**：daily/trend/user-behavior 端点有真数据；mental-health 端点 SQL seed 后 BFF 返回非空；E2E-F-36 滚动复验 4 页面 `.page-content overflow-y` = auto。详档 [report.md](stages/e2e-15-reports-dashboard/report.md) |
| E2E-16 | 多模态（语音/表情/文件上传） | 语音输入/表情识别/文件上传链路 | 数字人、TTS | ✅ **done**（2026-09-23 收口：5 修复 + 9 测试 + 5/5 情绪 IAB 验证 + F-124 诊断关闭 + F-122 排下轮独立 sprint；18/24 PASS/0 FAIL/6 BLOCKED；BLOCKED 6 项均为真实摄像头端到端（IAB/happy-dom 无摄像头 + 用户侧权限）） |
| E2E-17 ✅ | 数字人 + TTS | D-03：做真口型同步（接 `/tts_with_phonemes` 时间戳）+ 排查段间播放断点 | — | ✅ **done**（2026-09-23 step 5 收口：Playwright 双 project 6/6 PASS + 4 张 IAB 截图 + 19/19 测试点 PASS；**真口型同步 + 段间断点队列化**端到端通；F-126/F-127/F-128/F-129 全部修复）。**修复真因清单**：① F-127 BFF yaml XTTS.TimeoutMs 30000→90000（v0.1.27 commit 改 config.go 默认但漏 yaml，yaml 不为 0 时 SetDefaults 不覆盖 → 实际生效 30s 真 bug）；② F-128 phoneme 时间戳驱动 useTTSPlayer；③ F-129 段间断点队列化 + 入队即预取；④ APISIX upstream 6 (web-bff) timeout 60s→180s 持久化 seed.sh（覆盖 phonemes cold path 100s+）；⑤ mobile #3 BLOCKED 真因 = about:blank 跨域 fetch 触发 chromium same-origin policy → page.goto('/login') 修。详档 [report.md](stages/e2e-17-digital-human-tts/report.md) |

### 第五批：数据与缓存

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-18 | 缓存层 | 本地缓存（ai-svc LRU）行为 + Redis 是否启用/下线的决策与验证 | Redis 后端实现（归 E2E-20）/ LRU 调优 / 前端存储 | ✅ **done**（2026-09-28 §13.3 第二方核对用户批准；v2 PR 三处 status 翻 done；17/17 断言过；详档 [STATUS v2](stages/e2e-18-cache-layer/STATUS.md)）。12/12 测试点 PASS、0 BLOCKED；D-27 = Redis 保留并接入业务（E2E-20 首个接入点）；F-08 关账、F-134/135/136 转挂 E2E-28、F-141 owner 转 E2E-19（已 done）。**E2E-20 开工前置已满足**。 |
| E2E-19 | 数据库层验证 | 连接池/迁移幂等重放/分区裁剪/视图可读/软删除行为 + **备份→破坏→恢复演练**（只验证不改 schema） | schema 变更 | 🟡 **partial**（2026-09-29 由 done 降级：A5 约束——新增未解决条目 E2E-F-149（分区表 `ON CONFLICT` 失效）；原 12/12 测试点结论未变。**2026-09-28 §13.3 第二方核对用户审过批准收口**：12/12 测试点全 PASS + audit 0 FAIL + F-141 根因修正+4 条 UPDATE 修复+migrate.sh 加固诊断三件套+真备份→真 DROP → pg_restore 417→417 一致；Playwright 双 project 2/2 + 2 截图 + 7 Go 模块全绿 + CI 门禁红线/绿线双向验证（PR #97/#98/#99 连环修 check name 失配 + paths 过滤 + 红线实测）。§13.3 17 条断言 17/17 过（详见 [STATUS v2](stages/e2e-19-database-verification/STATUS.md)）。详档 [report.md](stages/e2e-19-database-verification/report.md)） |
| E2E-20 🔧 | **多实例并发正确性** | 修 in-memory 限流/登录锁定/验证码防枚举的多实例失效 + APISIX limit-count 跨实例 + 双实例并发验证 | 分布式事务 | ✅ **done**（2026-09-28 §13.3 第二方核对用户批准收口：PR #113~#117 修复轮 12 测试点 11 PASS + **D-28 收尾裁定**（用户裁定验证码为 D-01 遗留物 → 其 Redis 化回退 in-memory + 契约测试锁死，登录锁定/policy=redis 保留，PR #118）+ F-142/143 GREEN 实测真缺陷当轮闭环 + F-144/145 新登留账 + 镜像 v0.1.31 运行时验收四项全过 + 回归钉收口复跑 4/4；audit 0 FAIL；详档 [STATUS v4](stages/e2e-20-multi-instance-concurrency/STATUS.md)） |

### 第六批：可观测性

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-21 | 日志体系 | 结构化日志 + traceId 注入（含 gRPC 侧）+ Loki 采集链路（Go 日志现未进 Loki） | 日志平台选型 | ✅ **done**（2026-09-28：**13/13 测试点全 PASS**；**2026-09-29 三条留账 E2E-F-146/147/148 全部闭环**，0 FAIL / 0 BLOCKED。补实现 ADR-2026-09-loki-aggregator-dev §2.1/§2.3/§四 写下但**从未落地**的 docker_sd 采集 ⇒ Go 6 服务日志进 Loki；traceId 三层断点全修（APISIX 注入 X-Trace-Id → 4 组插件变量全覆盖 / gRPC metadata 透传 / 拦截器日志改带 ctx）⇒ 一次登录的 trace_id 同时出现在 web-bff + user-svc + 网关 access log。**过程中另抓 3 个真缺陷**：bash 单引号串里 `` 被吞成空串 → **全站 500**（契约钉已补"校验 bash 展开后值"）；file-logger 取 `$http_x_request_id`（客户端不传 ⇒ 字段整条省略）改 `$apisix_request_id`；注释写进 JSON 片段致 route PUT 失败（同 Stage 106 trailing comma 一类）。5 组负向对照证明断言有约束力。**2026-09-29 收尾轮**：三条留账 F-146（ctx 日志门禁 + 迁 68 处 + sw8 解析）/ F-147（Loki 2.9.4→3.2.0 对齐 + k8s promtail 权限）/ F-148（Python 字段对齐 + 前端错误上报）全部闭环，阶段判 done。⚠️ 途中撞到既有缺陷 E2E-F-149（分区表化后 `ON CONFLICT (event_id)` 失效）→ E2E-15/E2E-19 连带降级。详档 [report.md](stages/e2e-21-logging-observability/report.md)） |
| E2E-22 | 监控告警 | Prometheus 抓取/Grafana 面板/Alertmanager 通知渠道 | — | ✅ **done**（2026-09-29 执行完毕 18/18 测试点 PASS + **第二方核对通过**（RUNBOOK §13.3 十七条 + 三组负向对照亲自复跑，AP-01/02/03/05/06/11 均未发现）+ 核对方 6 项问题已全部处置）：[plan.md](stages/e2e-22-monitoring-alerting/plan.md) + [report.md](stages/e2e-22-monitoring-alerting/report.md)。计划期实测：观测栈 6 容器同刻 `Exited(255)` 且**零告警**（发告警的 Prometheus 自己死了）、`llm-service` 有 `/metrics` 无 target、k8s 侧 `rule_files` 空 glob ⇒ **0 条告警规则零报错**；留账 E2E-F-11（Alertmanager 无通知渠道）为本阶段唯一归属留账 |
| E2E-23 | 健康检查与服务发现 | /health 与 gRPC health 语义 + Nacos 注册/配置中心/热更新 | APISIX 主动健康检查（F-137 根因，归 E2E-25） | ✅ **done**（2026-09-30 用户批准收口）—— **PASS 38 / FAIL 0 / BLOCKED 0 / N/A 2**；未完成清单 7 → 0（剔 3 项不属本阶段、1 项失实、1 项早已完成）；**归属本阶段的未闭环账本归零**（F-107 / F-156 / F-163 全 ✅）。途中修掉 3 个真缺陷：① 审计器账本解析器对**跨行 Markdown 静默丢弃** ⇒ A5 看不见 4 条账本，阶段可带未解决账本判 done；② 解析器把**转义竖线**当列分隔符 ⇒ owner/status 取错；③ **5 个服务健康探针在降级启动时报健康**（`dbOK := true` + `if repo != nil` 整段跳过）⇒ `/health/ready` 返 200、容器判 healthy、APISIX 照常路由而后端全挂，**零告警**。残留：F-96「代码半」（svc 侧 Postgres 单次连接失败后无重试、进程内永不自愈）主归属 E2E-06，E2E-23 侧的编排与探针两半已闭环。|

### 第七批：消息与网关

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-24 | 消息链路 | outbox→Kafka→consumer→DLQ 全链 + 重试/死信/回放 | — | ✅ **done**（2026-10-02 收口：[report.md](stages/e2e-24-message-pipeline/report.md) **20/20 测试点 PASS**；修复 F-149（消费 42P10）/F-175（序列 a010）/F-174（原地重试 D-34）/F-149 同型第二处（dev_publisher）+ D-33 回放工具（F-12/F-150 闭环，存量死信 26 处置 25 落库）；账本 E2E-24 名下 5 条全 ✅；第二方核对两轮通过（首轮抓 5 项失真全部处置）；下一阶段 = E2E-25 网关 APISIX） |
| E2E-25 | 网关 APISIX | 路由注册/JWT 插件/限流/CORS + **上游 nacos-discovery 健康检查/重连**（F-137 部分症状已由 E2E-17 PR #77 临时缓解） + **seed ↔ admin 持久化关系**（F-139） | — | ✅ **done**（2026-10-02 收口：[report.md](stages/e2e-25-apisix-gateway/report.md) **20/20 PASS** + 第二方核对通过（§9，5 项发现已处置/记账）；PR #137/#138/#139/#140/#141；**F-154 checks.active 落地**（摘除 75s→~7s，D-35）+ **F-137 同轮闭环**（BFF 重启自愈 ≤19s）+ **F-139 drift 工具+CI+D-36 B+ 接入 dev-up** + **F-145 双节点实测不放大**；新修 N1/N2/F-f/N3 = 账本 F-176/177/178；回归钉 apisix-gateway.spec.ts 12/12 + seed_test 72 断言；**下一阶段 = E2E-26**） |
| E2E-26 | 链路追踪 SkyWalking | sw8 传播 + OAP 查询 + UI 可视化（OAP 9.x queryDuration bug） | — | ✅ **done**（2026-10-03 收口：[report.md](stages/e2e-26-tracing-skywalking/report.md) **20/20 PASS**；**M1=客户端格式错(D-37)** 回填三处旧账 / **M2a=127.0.0.1:18080 UI 入口(D-38)** / **M2b=双 trace ID 保留+文档化(D-39)**；途中修复 9 组 TDD——C1 bff 拨号顺序、C2 5svc SetTracer、C3 server EntrySpan+承载 ctx、C3b client injector 空串覆盖 sw8、C4 health 探针噪声、#8 query_oap 工具、#12 契约 9 三重空转、F-181 dev-up 漂移路径、F-185 TraceIDFromSW8 base64 半修；回归钉 skywalking-trace.spec.ts 3/3 绿 + 3 截图人工验收；新账 F-182(归E2E-29)/F-183(归E2E-30)/F-184(工具治理)，**名下 F-185 ✅**；下一阶段 = E2E-27） |
| E2E-27 | 对象存储 MinIO | 头像上传/下载/匿名读权限 | 其他文件类型接入 | ✅ **done**（2026-10-03 收口：[report.md](stages/e2e-27-object-storage-minio/report.md) **20/20 处置** = 19 PASS + 1 FAIL 已分类；**D-40**（F-116 相对化+双反代+惰性兼容，账本 ✅）/**D-41**（127.0.0.1 限定+匿名读文档化）；9 组 TDD 修 6 真缺陷；回归钉 spec 2/2 + 双截图；新账 F-186/187/188/189 转挂（F-189=全量联跑基线红）；下一阶段 = E2E-28） |

### 第八批：质量属性与收口

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-28 | **性能与延迟基线** | 首次建立基线：/analyze p50/p95、SSE TTFT、TTS 单段与段间 gap + 小规模阶梯 | 全站压测 | ✅ **done（2026-10-06：F-135 实测否决 D-44 销账 + TTS API 专项 F-198 立项；收口执行 2026-10-05 续 2026-10-03 停工）**——20/20 终判完成（18 PASS + 2 FAIL-已分类：#4 测点加性差、#19 回归钉未全绿 F-195/197 归账）；8 个修复 PR 合并（#151~#158）；D-42/D-43 落地；**F-136 ✅**（根因深化=事件循环阻塞，synth_pool+ADR 决策39，#11 修后重叠实证）+ **F-134 ✅**（manager+sender 双层，#13 实测 3371<3747）+ **F-26 ✅**（23 份 JSON 基线）；§8 三节已回填 + audit 30 阶段 0 FAIL + 第二方核对两轮通过 + IAB 收口复测通过 + 收口 PR #159（2026-10-05）；**F-135 于 2026-10-06 实测否决（D-44，见 report §8.4）销账 ⇒ done**；收口新账 F-195/196/197 + F-198（TTS API 专项，owner=下一阶段）；plan [plan.md](stages/e2e-28-performance-baseline/plan.md)） |
| E2E-29 | 横切：异常与安全 | JWT 过期刷新/IDOR/限流/CORS/越权 + **JWT 密钥轮换机制**（BFF+APISIX 原子性） | 渗透测试 | ⏳ pending |
| E2E-30 | 数据契约收口 | §2.4 六项数据契约 smoke 全绿 + helm template/lint 渲染回归 | — | ⏳ pending |

## 改造项决议状态

完整选项分析与影响面见 **[decisions.md](decisions.md)**。

| 项 | 归属阶段 | 状态 | 决议 |
|----|---------|------|------|
| **D-01 找回密码方式** | E2E-07（+E2E-06 供字段） | ✅ **密保问题** | A 运维脚本体验不好、B 要接额外 API、C 只需改页面 + 数据库 → 选 **C** |
| **D-02 心理测验定位与人格画像** | E2E-13 / E2E-14 | ✅ **两种量表并存** | 新增人格量表（产心理画像→驱动 AI 提示词）+ 保留症状量表（风险预警）；`/question` 页区分两类；user 页新增测评图表 |
| **D-03 数字人口型同步与 TTS 断点** | E2E-17 | ✅ **真口型同步 + 排查断点** | 接 XTTS `/tts_with_phonemes` 字符级时间戳驱动 BlendShape + 解决段间播放间隙 |
| **D-14 融合结果注入 system prompt** | E2E-16 | ✅ **emotion context 段拼接** | 前端携带 faceEmotion/voiceEmotion → BFF `buildSystemPromptWithEmotion` 拼"情绪上下文"段 + 三句护栏（不点破来源/不贴标签/不过火）+ 中英映射（happy→愉快等）；最小模式用前端 payload，emotionSource 高级模式留账 E2E-F-122 |
| **D-04 多语言支持（i18n）** | 待定（候选） | 🟡 **候选未决** | 项目完全单语硬编码（无 vue-i18n、无 locales 目录，UI 文案硬编码中文）。是否做属产品决策，暂不列为 E2E 阶段 |

## 改造阶段明细

### E2E-02 🔧 项目目录清理

**范围**：清理仓库中无关文件/目录、补 .gitignore、归档测试证据。**不动业务代码。**

| 对象 | 现状 | 处置建议 |
|------|------|---------|
| `.mimosa/`（根、deploy/apisix、emotion-echo-web 三处） | hook 运行时状态，**未被 gitignore**，持续污染 `git status` | 加入 `.gitignore` |
| `emotion-echo-web;D` | **空目录（0 字节）**，shell 误建残留 | 删除 |
| `docker-images-before.txt` | 一次性快照产物（2026-09-09） | 删除 |
| `gui-test-screenshots/`（根） | **25 个文件已被 git 跟踪** | 归档到 `docs/evidence/` |
| `tmp/` | 已 gitignore | 确认可清空 |
| 根 `node_modules/` | 已 gitignore，根目录无 `package.json` | 确认可删 |

### E2E-03 🔧 CI/CD 门槛

**范围**：把 `docs/ci-workflows/` 的 3 份模板落地为真实可跑的 `.github/workflows/`。

| 现状 | 目标 |
|------|------|
| `.github/` 目录**不存在**，3 份模板（go-test/llm-test/web-test）从未执行 | `.github/workflows/` 真实注册 + 首条绿 run 证据 |
| 285 个 Go 测试、47 个前端测试全凭自觉运行 | push/PR 自动触发 |
| AGENTS.md §2.2 合并门槛（`go test` + `go vet` + lint + smoke）**无人机械执行** | 门槛机械化 |

**阻塞原因**（已记录在 `docs/ci-workflows/README.md`）：PAT 只有 `repo` scope，GitHub 拒绝 push `.github/workflows/*.yml`，需换 token 或手工在网页创建。

### E2E-04 🔧 前端工程化门槛

**范围**：给前端重度阶段（E2E-01/07~17）建立机械防护。

| 项 | 现状 | 目标 |
|----|------|------|
| ESLint | ❌ 无配置、devDependencies 无 eslint | 引入 + `lint` script + 接入 CI |
| Prettier | ❌ 无 | 引入 + 格式化基线 |
| typecheck | ⚠️ 脚本存在但**96 处历史错误基线** | 清零，或落"仅新增文件零错"基线并写入 CI |
| 构建产物 | ❌ 无 `nuxt build` 后 smoke（项目是 **SPA 模式** `ssr: false`） | 产物 smoke 脚本 |
| Playwright project | ⚠️ 仅 `chromium-headless-shell` | 加 `mobile`（Pixel 5）+ `firefox`（可选） |
| browserslist | ❌ 无支持范围声明 | 补声明 |
| a11y | ❌ 零工具链（仅零散手工 aria-label） | `@axe-core/playwright` 对 6 主页跑基线，只修 critical/serious |

### E2E-05 🔧 文档与代码一致性收口

**范围**：让 ADR-18 的防线真正生效。

| 项 | 现状 | 目标 |
|----|------|------|
| 10 个校验脚本（路由对齐/视图一致性/env 变量/迁移契约/JWT secret 一致/digest/布局/TLS…） | 全部 CI-shaped，**全部无人在跑** | 接入 E2E-03 的 CI |
| `check_docker_digests.sh` | **假绿**：只校验 FROM 格式，而 `Dockerfile.digests.lock` 7 个 digest 是 `sha256:000...000` 占位 | 校验 digest 非占位值 |
| 3 处实测失真 | 见下方账本 E2E-F-20~22 | 就地更正 + 登记 ADR-18 表 |

### E2E-06 🔧 数据库改造

**(a) 删除死字段**（已核实使用情况）：

| 字段 | 现状 | 结论 |
|------|------|------|
| `users.email` | 仅 model tag 声明，**无任何读写** | 死字段，可删 |
| `users.phone` | 仅在 API 响应里回显（4 处），**无任何写入点** → 恒为 NULL | 死字段，可删 |
| `users.status` | 仅 model tag，无逻辑读写 | 待确认（可能有意软禁用），无使用则删 |

**(b) 新增密保问题字段**（供 D-01=C）：`security_question` + `security_answer_hash`（bcrypt），或独立 `user_security_answers` 表支持多问题

**(c) 迁移治理**：加 `schema_migrations` 版本表（现靠"幂等 + 每次重放"）

**(d) 软删除统一**：chat 的 `deleteconversation` 是物理删，与 users/ai 域不一致

**(e) 文档修正**：`deploy/db/README.md` 仍列不存在的 `03-migrate-data.sql`

### E2E-20 🔧 多实例并发正确性

**范围**：修 3 处多实例下静默失效的防护（项目决策 3 是"本地 Docker 单机多实例"，故必须正确）。

| 缺陷 | 位置 | 多实例后果 |
|------|------|-----------|
| BFF 登录失败锁定 in-memory | `auth_handler.go:14,66` | 5 次错密码锁定**可被绕过**（打不同实例） |
| 验证码 60s 防枚举 in-memory | `auth_handler.go:15` | 防枚举**失效** |
| APISIX 限流 `policy: local` | `seed.sh:318-325` | 每节点各自计数，**总配额放大 N 倍** |

**修复路径已铺好**：`shared/pkg/middleware/limiter.go:133` 的 `LimiterBackend` 抽象接口已存在，:137-140 的 `RedisLimiterBackend: TODO` 待实现；Redis 容器现成。

## 依赖声明

- E2E-04 建议排在所有前端阶段之前（E2E-01/07~17 全是前端重度）
- E2E-05 强依赖 E2E-03（脚本要挂进 CI 才有意义）；E2E-05 与 E2E-03 可视为同一议题的两面
- E2E-07 依赖 E2E-06（密保问题字段就位）与 E2E-01（会话状态）
- E2E-14 依赖 E2E-13（人格画像是量表链路的延伸）
- E2E-15 依赖 E2E-10（报表数据来自聊天产生的行为事件）
- E2E-16 / E2E-17 依赖 E2E-10（多模态与数字人入口位于聊天页）
- E2E-20 依赖 E2E-18（缓存层先决定 Redis 用不用）；两者**应串行不可并行**
- E2E-28 依赖 E2E-10（SSE 已通）、E2E-17（TTS 改造后复测）、E2E-22（Prometheus 抓取已通）
- E2E-24 / E2E-25 / E2E-26 建议在浅层用户流程稳定后再测（浅层问题会污染深层观测）

## 每阶段标准流程与详档约定

**执行协议**：**开工前必读 [RUNBOOK.md](RUNBOOK.md)** —— 状态机、环境准备（含 `--env-file .env.local` 红线）、执行循环、测试点判定分级（`[A]`自动/`[V]`视觉/`[M]`需裁定）、账本写入契约、收口契约（8 项必做）、阻塞与升级协议、report 模板、迭代护栏、命令速查。

**方法论**：全局 skill `e2e-stage-testing`（为什么这么做）；RUNBOOK 是项目内的可执行版本，不依赖 skill 是否加载。

**详档约定（just-in-time）**：每个阶段的详细规划文档写在 `stages/<e2e-NN-slug>/plan.md`，**在轮到该阶段前 1 个阶段时撰写**，不提前批量写完全部 30 份。原因：本项目长期受"文档与代码漂移"之害（ADR-18），提前写出的详档会随前面阶段的发现而失效，反而制造新的失真。已写详档：

| 阶段 | 详档 |
|------|------|
| E2E-01 登录会话持久化 | [stages/e2e-01-login-session/plan.md](stages/e2e-01-login-session/plan.md) |
| E2E-02 项目目录清理 | [stages/e2e-02-directory-cleanup/plan.md](stages/e2e-02-directory-cleanup/plan.md) |
| E2E-03 CI/CD 门槛 | [stages/e2e-03-ci-gate/plan.md](stages/e2e-03-ci-gate/plan.md) |
| E2E-04 前端工程化门槛 | [stages/e2e-04-frontend-engineering/plan.md](stages/e2e-04-frontend-engineering/plan.md) |
| E2E-05 文档与代码一致性 | [stages/e2e-05-doc-code-consistency/plan.md](stages/e2e-05-doc-code-consistency/plan.md) |
| E2E-06 数据库改造 | [stages/e2e-06-db-transformation/plan.md](stages/e2e-06-db-transformation/plan.md) |
| E2E-07 找回/重置密码 | [stages/e2e-07-password-recovery/plan.md](stages/e2e-07-password-recovery/plan.md) |
| E2E-08 历史会话管理 | [stages/e2e-08-conversation-management/plan.md](stages/e2e-08-conversation-management/plan.md) |
| E2E-09 注册流程 | [stages/e2e-09-registration/plan.md](stages/e2e-09-registration/plan.md) |
| E2E-10 聊天核心链路 | [stages/e2e-10-chat-core/plan.md](stages/e2e-10-chat-core/plan.md) |
| E2E-11 我的空间 | [stages/e2e-11-my-space/plan.md](stages/e2e-11-my-space/plan.md) |
| E2E-12 设置页 | [stages/e2e-12-settings/plan.md](stages/e2e-12-settings/plan.md) |
| E2E-13 心理测验链路修复 | [stages/e2e-13-quiz/plan.md](stages/e2e-13-quiz/plan.md) |
| E2E-14 人格量表与 AI 提示词定制 | [stages/e2e-14-personality-ai-prompt/plan.md](stages/e2e-14-personality-ai-prompt/plan.md) |
| E2E-15 报表 Dashboard | [stages/e2e-15-reports-dashboard/plan.md](stages/e2e-15-reports-dashboard/plan.md) |
| **E2E-16 多模态（语音/表情/文件上传）** | [stages/e2e-16-multimodal/plan.md](stages/e2e-16-multimodal/plan.md) |
| **E2E-17 数字人 + TTS（真口型同步 + 段间断点）** | [stages/e2e-17-digital-human-tts/plan.md](stages/e2e-17-digital-human-tts/plan.md) |
| **E2E-21 日志体系（结构化 + traceId + Loki 采集）** | [stages/e2e-21-logging-observability/plan.md](stages/e2e-21-logging-observability/plan.md)（2026-09-28 建档；E2E-18/19/20 的详档在各自目录，见其 STATUS/report） |
| **E2E-22 监控告警（Prometheus 抓取 + Grafana 面板 + 通知渠道）** | [stages/e2e-22-monitoring-alerting/plan.md](stages/e2e-22-monitoring-alerting/plan.md)（2026-09-29 建档；18 个测试点，核心是把"配了告警"升级为"实测 firing + 送达 + resolved"） |
| **E2E-24 消息链路（outbox→Kafka→consumer→DLQ + 重试/死信/回放）** | [stages/e2e-24-message-pipeline/plan.md](stages/e2e-24-message-pipeline/plan.md)（2026-10-01 建档；20 个测试点；F-149 前置修复 + DLQ/outbox-dead 回放补齐 + 24 条存量死信处置） |
| **E2E-25 网关 APISIX（上游健康检查/重连 + seed↔admin 治理 + JWT/限流/CORS）** | [stages/e2e-25-apisix-gateway/plan.md](stages/e2e-25-apisix-gateway/plan.md)（2026-10-02 建档；20 个测试点；F-137/F-154 同轮治理 + F-139 漂移检测/CI 兜底 + F-145 双节点压测 + 3 项计划期新事实） |
| **E2E-26 链路追踪 SkyWalking（sw8 传播 + OAP 查询 + UI 可视化）** | [stages/e2e-26-tracing-skywalking/plan.md](stages/e2e-26-tracing-skywalking/plan.md)（2026-10-02 建档；20 个测试点；端口全封/契约 9 空转/queryDuration 定性/双 trace ID 4 项计划期新事实 + M1/M2 决策点） |
| **E2E-27 对象存储 MinIO（头像上传/下载/匿名读权限）** | [plan.md](stages/e2e-27-object-storage-minio/plan.md)（2026-10-03 建档；20 测试点；桶级匿名读/DB 持久化绝对地址/fileSourceURL 耦合/双守卫未接 CI 4 项计划期新事实 + M1/M2 决策点）→ 收口 [report.md](stages/e2e-27-object-storage-minio/report.md)（同日；19 PASS + 1 FAIL 已分类；D-40/41；9 组 TDD） |
| **E2E-28 性能与延迟基线（/analyze p50/p95 + SSE TTFT + TTS 单段与 gap + 小阶梯）** | [plan.md](stages/e2e-28-performance-baseline/plan.md)（2026-10-03 建档；20 测试点；TTFT 伪流式疑点/TTS 冷热 18x/fusion 埋点零调用+TTFT 无服务端埋点/xtts 守卫未接线 5 项计划期新事实 + M1 阈值门禁/M2 F-134~136 处置 2 个决策点） |

模板见 [stages/_TEMPLATE.md](stages/_TEMPLATE.md)，执行记录模板见 [stages/_REPORT_TEMPLATE.md](stages/_REPORT_TEMPLATE.md)。批次三及以后在轮到前补写。E2E-01 已按判定分级标注，其余已写详档在启动前补齐标记。

## 决策门（开工前核对，未落定不得开工）

| 阻塞项 | 阻塞阶段 | 需谁决定 | 现状 |
|--------|---------|---------|------|
| D-04 i18n 是否立项 | 不阻塞任何阶段 | 用户 | 🟡 候选 |

**已解除**：`users.status` → 删；注册验证码步骤 → 删；密保可否跳过 → 不可跳过、注册必设；GitHub token `workflow` scope → 已具备（实测真实 push 成功）；**存量未设密保用户 → 选 C 不处理**（数据库现有数据全是无用信息、未上线、测试阶段），改为**新建可重跑、可删除的演示账号**（带密保）；**main 分支保护 → 安全子集已开**（防强推 + 防删除 + `enforce_admins=true`，实测强推被拒 `GH006`；status checks / PR 要求待 CI 落地后再开）。

> E2E-03 已拆为两段：**阶段 1 落地**（模板 → 真 CI）+ **阶段 2 严格化**（18 处缺陷逐条加固，清单见 [E2E-03 plan](stages/e2e-03-ci-gate/plan.md) §2.3）。详细规则见 [RUNBOOK.md](RUNBOOK.md) §9。

## 历史与背景

- 前身：`docs/plans/test-coverage-tracker-2026-09-16.md`（业务路径覆盖追踪，Sprint 109-115）
- 建档预探查：2026-09-17 三大块代码级探察（人格测试/AI 提示词、数字人、横切模块），发现 19 项登记 `discovered-unresolved.md`
- 覆盖盲区排查：2026-09-17 对 12 个候选面向做覆盖性排查，发现 5 处缺口并补为阶段（CI/CD、前端工程化、文档一致性、多实例并发、性能基线）；其中 CI/CD 与文档一致性属于"治理基础设施"，是其余阶段的防退化机制
- 改造决议：D-01 密保问题 / D-02 两种量表并存 / D-03 真口型同步，详见 [decisions.md](decisions.md)
- 排期依据：用户指定前三顺序（登录 cookie → 密码找回 → 历史会话），改造项排到前面，并追加缓存层/数据库/日志等横切切分要求
