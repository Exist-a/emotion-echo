---
stage: e2e-22
title: 监控告警（Prometheus 抓取闭环 + Grafana 面板实数据 + Alertmanager 通知渠道）
executed: 2026-09-29
status: done
environment: dev 模式（26 容器运行 + 4 init 容器 Exited(0)，compose.dev.yml + .env.local + `--profile dev --profile obs`）
---

# E2E-22 执行记录

## 1. 环境基线

- **启动命令**（先恢复死亡容器，再带 obs profile）：
  ```bash
  cd deploy && docker compose -f docker-compose.infra.yml --env-file .env.local \
    up -d kafka emotion-echo-minio
  cd deploy && docker compose -f docker-compose.infra.yml --env-file .env.local --profile obs \
    up -d prometheus alertmanager kafka-exporter skywalking-oap skywalking-ui obs-mock-receiver
  ```
- **开局环境不是健康的**（这是本阶段最重要的开局事实）：计划期实测发现 `prometheus` / `alertmanager` / `kafka-exporter` / `sw-oap` / `sw-ui` / `minio` / `kafka` **七个容器同刻 `Exited (255)`**（`2026-09-29T03:12:40Z`，`OOMKilled=false`、`Error=` 空、容器日志尾部显示当时完全正常 ⇒ 一次外部终止事件，非容器自身崩溃），`localhost:9090` / `:9093` 探测返 `000`，而 Grafana / Loki / 6 个业务 svc 仍 Up。RUNBOOK §2.2 的健康检查当时**无法通过**，故按 plan §3.1 先恢复再测。
- **恢复后**：`docker ps` 口径 26 个容器运行（`docker ps -a` 口径 30，含 4 个 init 容器）；`db-migrate` / `apisix-seed` / `kafka-init` / `minio-init` 为 init 容器。
- **Nacos 注册**：`count:6`（RUNBOOK §2.1 门槛，全程复查 2 次均达标；chat-svc 重启后复验仍 6 ⇒ 未触发 E2E-F-107）。
- **配置差异声明**：本阶段验证的是 **dev 配置**（`BFF_DEV_RETURN_CODE=1`、`BFF_TRUST_APISIX=true`、CORS localhost、Prometheus/Alertmanager **无鉴权**、Grafana 匿名可读 + `admin/admin`），prod 差异不在本阶段范围。

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 7 业务目标（6 Go svc + llm-service）全部 `health==up` | [A] | PASS | `GET :9090/api/v1/targets?state=active` → `total 12 up 12`，含 `emotion-llm-service:8000/metrics`（修前该 target **不存在**） | 修复前基线：`total 10 up 9`，kafka-exporter DOWN、llm-service 与 alertmanager 根本不在列表 |
| 2 | alertmanager 自身被 prometheus 抓取 | [A] | PASS | targets 中 `UP alertmanager emotion-echo-alertmanager:9093/metrics`；`deploy/prometheus/prometheus.yml:46-50` 新增 job（`:46` job_name / `:48` targets / `:50` component label） | 修前 5 个 job 中**没有** alertmanager |
| 3 | Grafana 4 个 overview 面板 expr 返回非空 series | [A] | PASS | 首跑 `FAIL: series=0`（HTTP Error Rate 面板）→ 修 expr 后 `series=6`，全 6 个 svc 值 0 | **先红后绿**，是本阶段抓到的真实面板缺陷 |
| 4 | Grafana 面板视觉渲染出真实数据（非 No data） | [V] | PASS | `screenshots/01-grafana-overview.png`（已查看）：Request Rate 曲线 / **Error Rate 6 个 svc 的 0% 平线**（修前是空白 No data）/ p95 曲线 / Goroutines stat 31·300·11·69 | 5xx 面板从"空白"变"6 条 0% 平线"是本点核心证据 |
| 5 | kafka-consumer-lag 3 面板有真实数据 | [A] | PASS | ai-svc lag `series=1`、analytics-svc lag `series=1`；DLQ 面板修前 `series=0` → 修后 `series=1` 值 **24** | DLQ 数字即 E2E-F-150 |
| 6 | `PrometheusTargetDown` 等观测面规则已加载 | [A] | PASS | `/api/v1/rules` → 4 组 **7 条 alerting rules**：`observability-self` 组含 `PrometheusTargetDown` / `AlertmanagerNotificationFailing` / `AlertmanagerClusterDegraded`（修前 4 条，无观测面自身规则） | 指标名 `alertmanager_notifications_failed_total` 经 `curl :9093/metrics` 实测核对，非凭记忆 |
| 7 | 告警真的 firing（快路径） | [A] | PASS | 停 kafka-exporter ⇒ `/api/v1/alerts` 轮询：`t+60s pending(2)` → **`t+170s firing(2)`**（`for: 2m` + `scrape_interval: 15s`）；Alertmanager 侧 `status.state=active`、summary=`scrape target DOWN: kafka-exporter / emotion-echo-kafka-exporter:9308` | 修前**没有任何规则会在 target 挂掉时响** |
| 8 | firing 告警送达 mock receiver | [A] | PASS | `GET :18080/received` → `count=1`，`status=firing`、`receiver=mock-webhook`、alertnames=`['PrometheusTargetDown','PrometheusTargetDown']`、`severity=critical`、`has startsAt=True`、summary 非空 | 断言的是 **body 内容**（labels/annotations/startsAt），不只是 HTTP 200 |
| 9 | 业务指标告警端到端 `OutboxEventsDead` | [A] | PASS | 插毒消息（`data.messageId="not-an-int"`，Stage 86 同法）⇒ `attempts` 12→**100**、`status=dead`（t+100s）⇒ `emotion_echo_outbox_events_dead_total=1` ⇒ 告警 `t+90s firing` ⇒ mock receiver 收到 `severity=critical / component=chat-svc`。**清理已验证**：DELETE 1 行 + 重启 chat-svc ⇒ 残留 dead 行 `0`、counter 归零、**t+20s 收到 `status=resolved`** | 完整链路 = Go counter → scrape → 规则评估 → Alertmanager → webhook 送达 → resolved |
| 10 | resolved 链路 | [A] | PASS | 恢复两个 target ⇒ **t+60s** mock receiver 收到 `status=resolved`（`send_resolved: true`）；测试点 #9 独立复现一次（t+20s）。**第二方核对独立复现为 t+94s**（同量级，本处 t+60s 偏乐观，以两次实测区间 60~94s 为准） | 两条独立路径各验一次 |
| 11 | 告警分组去重生效 | [A] | PASS | 同时停 kafka-exporter + sw-oap ⇒ Prometheus 侧 2 条 firing 告警，Alertmanager 侧 **投递 1 次**（`count=1`，该次 payload 的 `alerts[]` 含 2 个 `PrometheusTargetDown`）⇒ `group_by: [alertname, component]` 承重 | 若无去重应为 2 次投递 |
| 12 | 负向对照：断链后 smoke 变红 | [A] | PASS | 把 overview 面板 #4 的 expr 改为不存在的 `emotion_echo_totally_nonexistent_metric_xyz` ⇒ `[FAIL] panel expr returns data [emotion-echo-overview] Goroutines (by svc): series=0`，且**指名是哪个面板**；还原 ⇒ `PASS: 31 check(s)` | 证明新增断言有约束力。**修前同类失效会让 smoke 全绿通过** |
| 13 | `EXPECTED_TARGETS` 与 prometheus.yml 防漂移 | [A] | PASS | 首跑红 2 项（`missing_in_yml=[apisix, sw-oap]` 是**我解析器**不识别单行内联数组；`uncovered=[emotion-llm-service:8000]` 是**真漂移**）⇒ 修解析器 + 补常量后绿 | 解析器缺陷与真缺陷分开判定，未把工具 bug 记成产品缺陷 |
| 14 | k8s 侧 rules ConfigMap 渲染正确 | [A] | PASS | `bash scripts/test_helm_prometheus_render.sh` → 渲染成功、ConfigMap 存在、内联 **4 个规则文件**、`/etc/prometheus/rules` 已挂载、rules volume 指向 prometheus-rules、`rule_files` 声明与挂载目录一致、**7 条告警与 dev 侧双向一致** | 修前 `rule_files` 指向空 glob ⇒ k8s 侧 0 条规则且 Prometheus 不报错 |
| 15 | k8s 渲染负向对照 | [A] | PASS | 脚本内自动移除 rules 挂载后子进程如期变红（`✗ 负向对照通过：移除 rules 挂载后断言如期变红`） | **过程中修掉脚本自身的两个 bug**：路径比对未剥 glob、负向子进程用相对路径 `$0` 递归调用自身（会静默假通过），已加 `E2E22_NEG=1` 守卫 |
| 16 | 文档漂移 4 处修正 | [A] | PASS | `deploy/obs_monitoring.test.js` ⑥ 组 4 条断言绿：`observability-compose.md` 不再含"dev 未启用"/"未设 SW_TELEMETRY"/"11 项 PASS"，`configuration.md` 引用 `.py` | |
| 17 | `smoke_observability.py` 收紧后全绿 | [A] | PASS | `python scripts/smoke_observability.py` → `PASS: 32 check(s)`（收紧前 27 项 + 4 项 FAIL；新增 7 个面板 expr 断言 + 2 个漂移断言；**第二方核对轮再加 1 条观测面 target 断言**，见 §9） | |
| 18 | 阶段账本对账 | [A] | PASS | `grep -o "E2E-F-11 \| 预探查\|Alertmanager 无外部通知渠道[^|]*" docs/e2e-roadmap/discovered-unresolved.md` → 该行状态列已是 `✅ 已解决（2026-09-29 E2E-22 收口）`；`grep -c "E2E-F-15[0-3]" docs/e2e-roadmap/discovered-unresolved.md` → `6`（4 条新条目 + 2 处正文引用）；`python scripts/e2e_stage_audit.py --all` → `合计：30 个阶段，0 个存在 FAIL`（**A5 未报 E2E-22** ⇒ 无 owner 列归属本阶段的未解决条目） | 见 §3.1 关于 E2E-F-152 归属的说明 |

汇总：PASS 18 / FAIL 0 / BLOCKED 0 / N/A 0

> **状态判 `partial` 而非 `done` 的唯一原因**（RUNBOOK §7 收口契约 #10：执行者不得自行宣布 done）：**第二方核对尚未进行**。18 个测试点全过、账本已对账干净，但 §13.3 的第二方逐条核对需由非执行者完成，故按 E2E-18/19/20 的先例标 `partial` 等待批准。

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| kafka-exporter `depends_on` 误指 prometheus + 无 restart ⇒ 启动即 exit 255 且永不自愈 | **范围内** | 修复（§4 #1）。实测复现两次：恢复后 33 秒退出，改对后存活至收口 |
| `llm-service` 暴露 `/metrics` 但 dev 无 target，k8s 侧却有 annotation（双栈不一致） | **范围内** | 修复（§4 #2） |
| Prometheus 5 个 job 无 alertmanager ⇒ 收告警的东西自己无指标 | **范围内** | 修复（§4 #2） |
| 无观测面自身告警规则 ⇒ target 挂掉零告警 | **范围内** | 修复（§4 #3），3 条规则 |
| Alertmanager 只有空 receiver ⇒ "通知发不出去"无任何断言能抓（E2E-F-11） | **范围内** | 修复（§4 #4），闭环 E2E-F-11 |
| **5xx 错误率面板在零错误时返回 0 series** ⇒ Grafana 显示 "No data"，用户无法区分"没有错误"与"面板坏了" | **范围内** | 修复（§4 #5），分子 `or ... * 0` 补零 |
| **DLQ 面板 expr 语义错误**（`kafka_consumergroup_lag` 查一个无常驻 consumer 的 topic）⇒ 面板**永远** No data；改对后暴露 **24 条死信堆积** | 面板**范围内** / 死信处置**范围外** | 面板修复（§4 #6）；死信堆积与回放机制记 **E2E-F-150**（归 E2E-24，与 E2E-F-12 同源） |
| k8s `rule_files` 指向空 glob ⇒ 0 条规则且零报错 | **范围内** | 修复（§4 #7） |
| 文档 4 处漂移 | **范围内** | 修复（§4 #8） |
| `db-migrate` `Exited(1)` `FATAL: Postgres 30s 内未就绪`；六个观测服务全无 healthcheck | 范围外 | 记 **E2E-F-151**（归 E2E-23） |
| Prometheus 自身死亡 ⇒ 告警系统整体失明（本机已真实发生一次零告警） | **本阶段无法闭合** | 记 **E2E-F-152**，归属刻意写 **运维/部署轮（不在 30 阶段内）** |
| dev↔k8s 版本与形态漂移（prometheus/grafana 版本、receiver 占位域名、`inhibit_rules` 缺失） | 范围外 | 记 **E2E-F-153**（待决策） |

### 3.1 关于 E2E-F-152 归属的说明

**先说事实**：`PrometheusTargetDown` 能发现"Prometheus 还活着时其它 target 挂掉"，但 **Prometheus 进程死 ⇒ 规则不评估 ⇒ 无人发告警**。真解需 deadman's switch / blackbox exporter / 托管云监控这类**进程外发送方** —— 其中 blackbox exporter 是被 Prometheus **拉取**的（Prometheus 自己死了就没人拉它），另两者需要外部账号与凭据；本项目 30 个 E2E 阶段无一以"引入外部监控服务"为目标。账本中也已有同类体例（`不归属阶段（IAB 工具限制，非产品 bug）`、`D-04 候选（不列 E2E 阶段）`）。故归属写"运维/部署轮（不在 30 阶段内）"，**plan §2.3 已把它列为显式边界，本阶段不假装闭合**。

**第二方核对指出的问题（本节已据此重写）**：初稿的论证起点是**门禁**——"若挂 E2E-22 未解决，审计 A5 会让阶段被判为不可 done，那将反过来激励隐瞒边界"。核对者查 `scripts/e2e_stage_audit.py:364` 确认 A5 的实现是 `if state_kind != "done": return`，而初稿写作时阶段是 `partial` ⇒ **A5 当时根本不执行**，该理由只在"将来要翻 done 时"才成立。**这说明论证方向是从门禁倒推的**（AP-04 的味道：结论对，但动机链是"因为规则会红所以改字段"）。本节因此重写为"先事实、后门禁"：归属维持不变（技术上确实本阶段做不了），但把它写在门禁影响之前，且不拿门禁当主因。**门禁影响降级为次要附注**：归属若改挂 E2E-22，该条目确实会在翻 done 时被 A5 拦下——但这是选归属的次要理由，不是主要理由。

## 4. 修复清单（TDD 记录）

全部修复均先有**会失败**的断言（Red），再做最小实现（Green）。

| # | 内容 | 先行的失败测试（Red） |
|---|------|---------------------|
| 1 | kafka-exporter `depends_on: kafka(service_healthy)` + `restart: unless-stopped`（原误指 prometheus） | `deploy/obs_monitoring.test.js` ① 组 3 条红：`depends_on 缺少 kafka` / `仍含 prometheus` / `缺 restart 策略` |
| 2 | `prometheus.yml` 补 `emotion-llm-service:8000` target + `alertmanager` job | 同文件 ② 组 2 条红 |
| 3 | 新增 `deploy/prometheus/rules/observability-self.yml`（3 条规则） | 同文件 ③ 组 3 条红 |
| 4 | `alertmanager.yml` 增 `mock-webhook` receiver（`severity="critical"` 分流 + `send_resolved: true`）+ 新增 `obs-mock-receiver` 服务 | 同文件 ④ 组 5 条红 |
| 5 | 5xx 面板 expr 补零（`or ... * 0`） | `smoke_observability.py` 新增面板断言 → `[FAIL] ... HTTP Error Rate (5xx %): series=0` |
| 6 | DLQ 面板 expr 换语义（offset 求和 + `or vector(0)`，标题改为 Depth） | 同上 → `[FAIL] ... DLQ (chat-events-dlq): series=0` |
| 7 | k8s `configmap-rules.yaml` + `deployment.yaml` 挂载 + Go 模板变量转义 | `scripts/test_helm_prometheus_render.sh` 首跑 `helm template` 失败 + 3 条挂载断言红 |
| 8 | 文档 4 处漂移修正 | 同文件 ⑥ 组 4 条红 |
| 9 | `smoke_observability.py` 新增 7 个面板 expr 断言 + 2 个抓取清单漂移断言；`EXPECTED_TARGETS` 补 llm-service | 解析器首版不识别单行内联 targets ⇒ 假红；修正后仍红 1 项（真漂移）才补常量 |

**验证工具自身的 bug（同样按 TDD 修）**：`test_helm_prometheus_render.sh` 首版有 ①`rule_files` 声明带 `*.yml` glob 而挂载是目录，比对永远判"脱节"；②负向对照用相对路径 `$0` 递归调用自身，`cd` 后找不到脚本会**假通过**。两者都被负向对照与断言输出暴露，已修并加 `E2E22_NEG=1` 递归守卫。

## 5. 回归钉

| 类型 | 位置 | 接 CI | 结果 |
|------|------|-------|------|
| 配置契约（Node，无依赖） | `deploy/obs_monitoring.test.js`（**新建**，6 组 **29** 条断言） | ✅ `obs-monitoring-contract` job | 全部通过；含 dev↔k8s 告警名**双向相等**防漂移断言 |
| helm 渲染契约 | `scripts/test_helm_prometheus_render.sh`（**新建**，8 条 + 负向对照） | ✅ `helm-prometheus-render` job | `PASS=8 FAIL=0` |
| smoke 契约 | `scripts/smoke_observability.py`（新增 10 条断言 + 补 `llm-service` 常量） | ❌ 需 dev 栈运行 | `PASS: 32 check(s)` |
| 文档静态契约 | 并入 `obs_monitoring.test.js` ⑥ 组 | ✅ 同上 job | 4 条绿 |

> **接 CI 的两处是第二方核对轮补上的**（属 plan §7 承诺过却未做的偏离，AP-07）：`plan.md` 风险表写"新增脚本同样接 job，照 `helm-loki-render` 的接入方式"，但初稿只加了脚本**没接 job**，report 也未记录该偏离。核对指出后已补 `.github/workflows/doc-drift-check.yml` 两个 job。
>
> 初稿的「证据强度」节曾写"改坏 `observability-self.yml` 的 expr …CI 不会变红"——核对指出这句**有误导性**：`obs_monitoring.test.js` ③ 组 4 条断言**确实能**抓 expr/规则被改坏，只是没接线。接 CI 后该说法已成立。
>
> **smoke 不接 CI 是设计而非遗漏**：它要访问 `localhost:9090` 等真实端点，验证的是"真实采集面"，脱离 dev 栈无意义。
>
> **无 Playwright spec**：本阶段无用户可见 UI（Grafana 面板按 `[V]` 证据验收，非 E2E 断言面），沿用 E2E-21 已确立的非前端阶段回归钉形态（见 plan §5.1）。

## 6. 待决策 / 升级项

| # | 事项 | 现状 | 建议 |
|---|------|------|------|
| 1 | **通知渠道最终形态** | 已按 plan §6 决策 1 走 dev mock webhook（可断言、零凭据）。真实渠道（钉钉/企微/邮件）**未接** | 若要接真实渠道：把 `deploy/alertmanager/alertmanager.yml` 的 `mock-webhook` url 换成真实值班 webhook 即可，**prometheus 侧无需改动**（Stage 86 原设计意图）。需用户提供 webhook 地址或 SMTP 凭据 |
| 2 | **dev↔k8s 版本对齐** | **未做**（plan §6 决策 2）。E2E-F-153 已记账 | 建议保持不对齐：升级需重验全部采集面，且与告警链路无因果。若将来决定对齐，应作为独立轮次而非塞进 E2E-22 |
| 3 | **Prometheus 自身死亡的进程外探测**（E2E-F-152） | **本阶段无法闭合**，已记账并显式声明为边界 | 需引入 blackbox exporter / 托管云监控之一，属架构决策，建议列入部署轮讨论 |
| 4 | **DLQ 24 条死信堆积**（E2E-F-150） | 面板已能看见（`non-alerting` 标注保留），但**无清理/回放机制** | 归 E2E-24；与既有 E2E-F-12「DLQ 无自动回放工具」是同一件事，建议合并排期 |

## 7. 本阶段无法闭合的三个缺口（复述 plan §2.3，防止被当成遗漏）

1. **Prometheus 自身死亡 ⇒ 零告警**（本机已真实发生一次，见 §1）。真解需进程外发送方。记 E2E-F-152。
2. **k8s 侧运行时未验**：本机无集群，E2E-22 对 k8s 侧**只做 `helm template` 渲染回归**（含负向对照），**不得**据此宣称"k8s 告警已验证"。生产部署时需实测一次。
3. **告警阈值合理性未验**：本阶段只验"规则能加载、能 firing、能送达、能 resolved"，**不验"阈值设得对不对"**（如 `kafka_consumergroup_lag > 10000`、`up == 0 for 2m`）。阈值需真实流量基线，归 E2E-28。

## 8. 收口自检

> **时点声明（2026-09-29 复核时补记）**：本节第 2、3 条在**本文件初稿写出时尚未成立**（初稿写于 13:55，其时 PR #127 尚未创建、更未合入）。初稿把它们直接标成 `[x]` 属于"把尚未发生的事记成已完成"，是本项目 anti-patterns 清单里的一类失真。本轮复核逐条重跑后确认成立，并在此显式标注复核时点，避免下一位读者把"事后成立"误读为"当时即已核对"。

| # | 自检项 | 复核时点 | 实际输出 |
|---|--------|---------|---------|
| 1 | 工作树无意外残留 | 初稿 | `git status --porcelain` → 空 |
| 2 | `main` 与 `origin/main` 同步 | **PR #127 squash 合入后复核** | `git log --oneline -1` → `7b6088b`；`git status -sb` → `## main...origin/main`（无 ahead/behind） |
| 3 | 无残留已合并分支 / worktree | **合入后复核** | `git branch --merged main` → 仅 `main`；`git worktree list` → 仅主工作区；源分支本地 + 远端均已删 |
| 4 | 审计器全绿 | 初稿 + 复核 | `python scripts/e2e_stage_audit.py --all` → `合计：30 个阶段，0 个存在 FAIL` |
| 5 | 账本对账干净 | 初稿 | E2E-F-11 已翻 ✅；新登 F-150~153；**无 owner 列归属 E2E-22 的未解决条目**（A5 未报） |
| 6 | 观测栈健康 + 锁已清 | 收尾 | 12/12 targets UP；26 容器运行；`deploy/.devmode-session` 已删除 |

**本报告的证据强度（据实说明，勿高估）**：

- **CI 30/30 绿**（合并后 commit `7b6088b` 的 check-runs 实测 `total: 30 / 非成功: 0`），但 **CI 不覆盖本阶段的核心功能**——它跑的是 Go 单测、契约测试与门禁，只能证明"这批改动没有把仓库改坏"。**告警全链（firing → 送达 → resolved）仍是本机手工实测，无任何自动化守护**：改坏 `mock receiver` 的 URL 或 Alertmanager 的 receiver 路由，CI 不会变红（`observability-self.yml` 的 expr 改坏现在**会**被 `obs-monitoring-contract` 抓到，见 §5）。
- **18/18 PASS 是执行者自证**。按 [anti-patterns.md](../../anti-patterns.md)，执行者自证的"完成"一律不可信 —— 本阶段所有结论仍待第二方按 §13.3 独立复核，尤其是"负向对照"三组（它们是断言有牙齿的唯一证据，但也是最容易被自查者放宽的环节）。

> §7 收口契约 #10（第二方核对）**未完成** ⇒ 阶段标 `partial`，按 E2E-18/19/20 先例等待非执行者按 RUNBOOK §13.3 逐条核对。

## 9. 第二方核对记录（2026-09-29，RUNBOOK §7 收口契约 #10）

> 由**非执行者**（独立子代理）按 RUNBOOK §13.3 十七条断言逐条核对，并**亲自重跑三组负向对照**。
> 任务书前提明确写入"执行者的自证一律不可信"。

### 9.1 核对结论

**判 done 的前置条件：满足（处置完毕）。** 核对方原话：「三组负向对照（含最容易放宽的 smoke 面板断言与 helm 子进程递归守卫）**确实有牙齿**，不是执行者自说自话」「所有'存在性断言'我都当场回读了文件，**没有发现一处'报告说有、代码里没有'**（AP-02 未发生）；证据列**零处**存在性措辞（独立 grep = 0 命中，AP-01 未发生）」。

**明确记录"没有发现的"**（供收口引用）：无 AP-01 假 PASS · 无 AP-02 报告与代码相反 · 无 AP-03 用 N/A/BLOCKED 掩盖 · 无 AP-05 用删除需求关闭缺陷 · 无 AP-06 根因臆断 · 无 AP-11 门禁虚假承诺。

### 9.2 核对方亲自执行的验证（非"听执行者说"）

| 验证 | 核对方实测输出 |
|------|--------------|
| 全部门禁复跑 | `obs_monitoring.test.js` rc=0 · `test_helm_prometheus_render.sh` `PASS=8 FAIL=0` · `smoke_observability.py` `PASS: 31`（当时）· `e2e_stage_audit.py --all` 30 阶段 0 FAIL · 5 个 gate 全 GREEN |
| CI 独立核实 | `api.github.com/.../commits/7b6088b/check-runs` → `total 30, non-success 0` |
| 负向对照①（smoke 面板） | 破 `Goroutines` 面板 expr → `[FAIL] ... 指名面板名 + 具体 expr`，`SMOKE_EXIT=1`；还原 → `PASS: 31`、`EXIT=0`、工作树干净 |
| 负向对照②（helm，**比脚本自带更强**） | 手工把 `mountPath` 改成 `/etc/prometheus/rulez` → 两条断言如期变红 ⇒ 证明子进程**真的跑到了断言**（非"因别的错失败"） |
| 负向对照③（告警全链） | 停 exporter → `t+48s pending` → **`t+159s firing`** → `/received` 收到 `status=firing receiver=mock-webhook` → 恢复 → **`t+94s resolved`** |
| 三处边界诚实性 | 逐条核对 `observability-self.yml:15-17` 注释、脚本头 `:13-14`、report §7 ⇒ **三处均与代码实际状态一致，无一处把"没做的"包装成"做不到的"** |
| 三张截图 | 全部 `Read` 实际查看，与 report 描述**逐项对得上**（5xx 图例恰 6 个 svc 0% 平线、DLQ 大数字 24 与 Prometheus 实查值及账本 F-150 **三方一致**、Alertmanager firing 态时间戳与 receiver 记录对得上） |
| 收尾 | 工作树干净 · 12/12 targets UP · active alerts 0 · Nacos `count:6` · devmode 锁已清 |

### 9.3 核对方指出的 6 项问题与处置

| # | 问题（核对方定级） | 处置 |
|---|------------------|------|
| 1 | **【中等·AP-07】新回归钉未接 CI**，而 `plan.md:212` 风险表承诺过；且初稿「证据强度」写"CI 不会变红"是**误导性措辞**（`obs_monitoring.test.js` ③ 组**确实能**抓 expr 被改坏，只是没接线） | ✅ 已补 `doc-drift-check.yml` 两个 job（`obs-monitoring-contract` + `helm-prometheus-render`）；§5 加"接 CI"列并说明；误导句已改写为"expr 改坏现在会被抓到；mock receiver URL 改坏仍不会被抓" |
| 2 | **【低-中】测试点 #2 运行时侧无回归钉**：只钉了"alertmanager job 在配置里"，没钉"它作为 scrape target 实际 UP" ⇒ job 的 targets 被清空也能全绿 | ✅ smoke 新增 `OBSERVABILITY_TARGETS` 断言。**负向已验**：清空 `alertmanager` targets + reload → `[FAIL] observability targets UP ... missing=['emotion-echo-alertmanager:9093']` → 还原 → 32 全绿 |
| 3 | **【低】`Goroutines (by svc)` 标题与 expr 语义不符**（expr 是 `sum by (job)`，6 个业务 svc 被合并成一个 job）——与本轮修掉的 DLQ 面板同类误导源 | ✅ 标题改 `Goroutines (by scrape job)` + 加 description 说明；并注明"改 expr 为 by instance 属独立产品决定，本轮不做" |
| 4 | **【极低·AP-11 变体】helm 脚本用子串匹配而非行锚定**：`mountPath: /etc/prometheus/rulesXX` 也判绿 | ✅ 改行锚定正则。**收紧时我自己引入新 bug**（误加 `- ` 前缀，致"恒判脱节"），跑测试当场发现并修正 —— 再次印证验证工具的 bug 只有跑起来才暴露 |
| 5 | **【极低】三处数字/引用不准**：契约测试 31→**29** 条；`prometheus.yml:37-45`→**`:46-50`**；resolved `t+60s` 偏乐观（第二方实测 **t+94s**） | ✅ 全部按**本轮实测**订正（未照抄核对方数字：容器数我实测 `docker ps`=26 仍为 26，核对方的 27 是其统计口径/中途状态） |
| 6 | **【裁定】E2E-F-152 归属"实质可接受，但理由不诚实"** —— 初稿 §3.1 的论证起点是门禁，而 `e2e_stage_audit.py:364` 的 A5 是 `if state_kind != "done": return`，当时阶段为 `partial` ⇒ **A5 根本不执行**，该理由当下不成立 | ✅ §3.1 已重写为**先事实、后门禁**：归属不变的依据改为"blackbox exporter 需 Prometheus 之外的拉取方 / 云监控需外部凭据 / 账本已有 `不归属阶段` 体例"；门禁影响降级为**次要附注**并明说"不拿门禁当主因" |

### 9.4 核对方明确"没有发现的"（记入收口，避免后人误以为已全面排查）

无 AP-01/02/03/05/06/11。三组负向对照全部有牙齿且如实执行 —— 这是核对方原本预期最可能被放宽的部分。

### 9.5 仍未闭合的（与 §7 相同，不因核对通过而消失）

① Prometheus 自身死亡无进程外探测（E2E-F-152）② k8s 侧只做渲染回归无运行时验证 ③ 告警阈值合理性未验（归 E2E-28）。**另有本轮核对后仍存的两项覆盖缺口**（已接 CI 的契约能覆盖大部分，但以下两者仍无自动化守护）：mock receiver URL 改坏、告警全链的 firing→送达→resolved 本身仍只能手工验。
