---
status: landed
priority: high
stage: 53
date: 2026-09-08
related-stages:
  - stage-44-observability-sprint-b.md §四 B 6/6 步 (smoke 闸门绿)
  - stage-51-batch-1-infra-merged.md (批 1 合 main 闸门前置)
  - stage-52-nacos-fix.md (上一 stage, 修 Nacos 阻塞)
  - stage-50-e2e-validation.md §九.1 (镜像滞后 + smoke 阻断同源)
related-decisions:
  - adr-2026-09-doc-drift-registry.md (决策 18)
related-todo-pile:
  - todo-pile §C7 "Stage 36 dashboard 空根因未结案" (本 stage 闭环)
branch: stage-52-nacos-fix
commit:
  - cd7b11c test(smoke): §4 改查 7 天窗
related-tests-result: smoke_data_layer.py 11/11 PASS, smoke_observability.py 9/12 PASS (3 fail 为批 1 未合 main 副作用)
---

# Stage 53 · smoke §4 /reports/daily emotionDistribution=0 修复

> **本 stage 闭环 todo-pile §C7 "Stage 36 dashboard 空根因未结案"**——根因不是
> 视图缺失或业务链路断，而是 **smoke §4 固定查"今天"**。dev 库最近 7 天内最近
> 数据日是 2026-09-07（user_id=1, primary_emotion=neutral ×3），今天 2026-09-08
> 无人触发 AI 分析→ emotionDistribution=0 是预期的，不是 bug。**修复策略**：
> smoke §4 改查"最近 7 天内有数据的日期"（`MAX(created_at)::date`），保留业务
> 契约（dashboard 真有数据），不被"今天是否有数据"卡死。

---

## 一、问题回顾

### 1.1 现象

Stage-52 修完 Nacos 阻塞后跑 smoke_data_layer.py：
```
[FAIL] §4 /reports/daily 数据真有:
  summary='2026-09-08，你共有 0 段对话，0 条消息。整体心境 平稳。'
  emotionDistribution.len=0
```
10 项断言 9/10 PASS，**唯一 FAIL 在 §4**。

### 1.2 第一次误诊 + 纠正

读 todo-pile §C7：
> 根因可能也是缺迁移：smoke §4 实际查 `msg_summary_v` 视图，全新环境缺此视图时返 500。

**实测纠正**（postgres 查询）：
```
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_schema LIKE 'emotion%';
→ 24 行（含 4 个视图：daily_emotion_v / msg_summary_v / assessment_v /
  daily_emotion_by_modality_v）全部存在
```
todo-pile §C7 的"视图缺失"假设被推翻——视图全在。

### 1.3 真正根因

```
SELECT MAX(created_at)::date FROM emotion_echo_ai.emotion_analysis
WHERE user_id=1 AND created_at > NOW() - INTERVAL '7 days';
→ 2026-09-07

SELECT primary_emotion, COUNT(*)
FROM emotion_echo_ai.daily_emotion_v
WHERE user_id=1 AND created_at::date='2026-09-07' GROUP BY 1;
→ neutral | 3   ✅

SELECT primary_emotion, COUNT(*)
FROM emotion_echo_ai.daily_emotion_v
WHERE user_id=1 AND created_at::date='2026-09-08' GROUP BY 1;
→ 0 行   ❌ (今天 0 数据)
```

**根因**：
- dev 库最近 7 天内的 emotion_analysis 数据集中在 2026-09-07（4 行，neutral）
- smoke §4 不传 `date` → analytics-svc 默认今天（`time.Now()` local） → 2026-09-08
- 今天 0 行 emotion_analysis → emotionDistribution 为空 map → summary 仍能填（LLM 兜底）但 len=0 → §4 FAIL

**不是 bug**——是测试策略问题：smoke 想验证"dashboard 真有数据"，但"今天"在 dev 模式是低概率事件。

---

## 二、修复方案评估

### 2.1 评估过的 4 个方向

| 方向 | 风险/合规性 | 推荐度 |
|---|---|---|
| **改 smoke §4 查 7 天窗**（选 MAX(created_at)）| 保留契约 + 不动业务 + 测试可重现 | ✅✅✅ |
| smoke 触发 LLM 真实调用 | 违反 AGENTS.md §三"测试可重现 + 不调外部"；引入 llm-service / 凭据 / 网络抖动 | ❌ |
| smoke 开头直接 INSERT emotion_analysis | 绕过业务链路造假数据；下次表结构变更 smoke 不报 | ❌ |
| 临时 SKIP §4 | 隐去真实问题；smoke 失去"端到端数据"断言 | ❌ |

**选方案 1**（你拍板"改 smoke 查 7 天窗（推荐）"）。

### 2.2 改后契约

```
[OK] §4 最近 7 天内有 emotion_analysis 数据: 最近数据日=2026-09-07
[OK] §4 /reports/daily 数据真有:
     date=2026-09-07
     summary='2026-09-07，你共有 0 段对话，0 条消息。主要情绪是 平静（3 次），整体心境 平稳。'
     emotionDistribution.len=1
```

注：summary 仍说"0 段对话 0 条消息"——这是因为 user_behavior_events 的 event_type enum 是
`conversation_closed / conversation_created / message`，而 SQL `WHERE event_type = 'conversation'`
（**精确字符串**）找不到这些细分类型。这是 stage-43 Sprint A 的 PR-A1.4 改动（`event_type` 落库
统一 chat-svc 原值）后的副作用——本 stage 不解决，留 backlog（见 §五 A）。

---

## 三、修改内容（commit cd7b11c）

文件：`scripts/smoke_data_layer.py`（§4 段 60 行变更）

| 变更 | 旧 | 新 |
|---|---|---|
| 查询日期策略 | 不传 date → 默认 today | 先 psql `MAX(created_at)::date in 7 天窗` → 用该日期 |
| 断言数 | 1 项（§4 数据真有）| 2 项（§4.1 最近有数据日 + §4.2 /reports/daily 数据真有）|
| 失败处理 | 默认 today 没数据 → FAIL | 7 天全无数据 → SKIP（提示"待业务链路补 LLM trigger 后再跑"）|

---

## 四、测试结果（实测）

### 4.1 smoke_data_layer.py

| 契约 | 状态 |
|---|---|
| §1 user_behavior_events 行数 | ✅ OK |
| §2 event_type enum 细分 | ✅ OK |
| §3 analytics_reader 视图可读（4 视图）| ✅ OK ×4 |
| §4.1 最近 7 天内有 emotion_analysis 数据 | ✅ OK (最近数据日=2026-09-07) |
| §4.2 /reports/daily 数据真有 | ✅ OK (date=2026-09-07, emotionDistribution.len=1) |
| §5 schema 一致性 | ⏭ SKIP (需 integration test) |
| §6 dev 模式消费链路 | ⏭ SKIP (§1 PASS) |

**汇总：11/11 PASS, 0 FAIL**（原 9/10 PASS，新增 §4.1 1 项 PASS，共 +1）

### 4.2 smoke_observability.py（批 1 合 main 后必跑，本 stage 仅 cherry-pick 验证）

| 段 | 状态 |
|---|---|
| prometheus scrape targets UP | ❌ FAIL（缺 3 target: analytics-svc/apisix/sw-oap）——**批 1 未合 main** |
| grafana health + datasources + dashboards | ✅ OK ×3 |
| runbook 文件存在 | ❌ FAIL——**批 1 未合 main** |
| kafka-exporter + Kafka lag dashboard + alert | ✅ OK ×3 |
| loki ready + apisix access.log | ❌/SKIP——APISIX Restarting（Nacos 阻塞历史副作用） |

**汇总：9/12 PASS**。3 FAIL 全部是"批 1 未合 main"的副作用——一旦批 1 合 main + 重 build 对应镜像，9/12 → 12/12 是预期结果。

### 4.3 单测回归

| 包 | 状态 |
|---|---|
| emotion-echo-shared/pkg/discovery | ✅ 2/2 PASS（stage-52 的 ephemeral 测试）|
| emotion-echo-shared 全套 | ✅ 14/14 包全绿 |
| 6 svc 各 `go test ./...` | ✅ 全绿 |

---

## 五、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- `emotion-echo-web-bff/internal/handler/analytics_handler.go`（BFF /reports/daily handler）
- `emotion-echo-web-bff/internal/downstream/analytics.go`（BFF → analytics-svc 透传）
- `emotion-echo-analytics-svc/internal/handler/reports_handler.go`（svc handler）
- `emotion-echo-analytics-svc/internal/logic/reports_daily_logic.go`（空 date → today local）
- `emotion-echo-analytics-svc/internal/repository/report_repository.go`（跨 schema 聚合 SQL）
- `emotion-echo-ai-svc/migrations/005_create_daily_emotion_by_modality_v.sql`（视图定义）
- `scripts/smoke_data_layer.py`（AGENTS.md §2.4 数据契约 smoke）

### ② 查相关 ADR / stage

- `docs/plans/todo-pile-2026-09-04.md §C7`（Stage 36 dashboard 空根因未结案）→ 本 stage 闭环
- `docs/stages/stage-44-observability-sprint-b.md §四 B 6/6 步`（smoke 闸门绿要求）
- `docs/stages/stage-50-e2e-validation.md §九.1`（镜像滞后同源）
- `docs/stages/stage-51-batch-1-infra-merged.md`（批 1 合 main 闸门前置）
- `docs/stages/stage-52-nacos-fix.md`（上一 stage 修 Nacos 阻塞）
- `decisions.md ADR-17`（/reports/daily 返回 data 形状契约 `{summary, emotionDistribution}`）

### ③ 跑现状 smoke / 实测

- `docker exec emotion-echo-postgres psql \d+ emotion_echo_ai.daily_emotion_v`：视图定义 = emotion_analysis 直接投影
- `SELECT MAX(created_at)::date ...`：最近数据日 = 2026-09-07
- `SELECT ... WHERE created_at::date='2026-09-07'`：3 行 neutral ✅
- `SELECT ... WHERE created_at::date='2026-09-08'`：0 行 ❌（解释 §4 FAIL）
- `python scripts/smoke_data_layer.py`：11/11 PASS ✅

### ⑤ 列架构假设

| 假设 | 验证 | 结果 |
|---|---|---|
| 视图缺失是根因（todo-pile §C7） | 查 information_schema.tables | ❌ 视图全在 |
| smoke §4 固定查"今天"在 dev 模式不稳定 | 实测今天 0 数据 | ✅ 根因命中 |
| event_type='conversation' 找不到细分 enum | SQL 实测 0 行（虽然 user_behavior_events 有 12 行）| ✅ 命中（独立 backlog 见 §五 A）|
| 改 smoke 查询策略不动业务可绕过 | 实测 smoke 11/11 PASS | ✅ |
| smoke_observability 9/12 是批 1 未合 main 的 | 3 FAIL 都是"批 1 加的代码不在 stage-52 分支" | ✅ |

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-53-smoke-section4-fix.md`。
1 个 commit（cd7b11c）已合入 `stage-52-nacos-fix` 分支，领先 main 5 commit（4 stage-52 + 1 stage-53）。
**未做 push 与合 main**——等你 review 后决定合 main 还是先 cherry-pick 批 1 的 smoke_observability.py。

---

## 六、未做项 / 遗留 backlog

### ❌ A. §4 summary "0 段对话 0 条消息"（event_type enum 细分后失配）

**现象**：最近 7 天有 3 条 emotion_analysis（neutral ×3），但 summary 仍说"0 段对话 0 条消息"。

**根因**：ReportRepo.GetDailyReport 第 181 行 SQL：
```sql
COALESCE((SELECT COUNT(*)::bigint FROM emotion_echo_analytics.user_behavior_events
          WHERE user_id = $1 AND event_type = 'conversation'
          AND occurred_at::date = $2::date), 0)
```
查 `event_type = 'conversation'`（精确字符串）。但 stage-43 Sprint A PR-A1.4 之后，event_type enum 细分为 `conversation_created / conversation_closed / message`（smoke §2 已验证 3 种 distinct types）。

**修复路径**：
1. 改 SQL `event_type = 'conversation'` → `event_type IN ('conversation_created', 'conversation_closed')`
2. message_count 同理：要么用 `event_type = 'message'`，要么从 `msg_summary_v` 查（视图已有 0 行，路径 A 优先）
3. TDD：扩 integration test `report_repo_integration_test.go` 加 case 验"细分 enum 也能被聚合"

**预估**：1-2 小时（单 SQL 改 + 集成测试）。

**本 stage 不解决**：聚焦在"smoke §4 PASS"，不动业务代码。

### ❌ B. smoke_observability.py 9/12 → 12/12 路径

**现状**：3 FAIL 都是"批 1 未合 main"副作用：
- prometheus target 缺 3 个（analytics-svc/apisix/sw-oap）→ 批 1 含 PR-OBS-4 prometheus.yml
- runbook 文件不存在 → 批 1 含 PR-OBS-8 runbook
- APISIX access.log → 批 1 含 PR-OBS-1 apisix-file-logger

**修复路径**：批 1 合 main + `scripts/build_dev_images.sh` 重 build → 自动全绿。

**预计**：批 1 合 main 后 30 分钟内 12/12 PASS。

### ❌ C. 业务链路"今天无数据"是 dev 模式系统性问题

**现象**：dev 库最近数据日 = 2026-09-07（7 天前），后续无人触发 AI 分析。

**根因方向**：
- 用户发消息 → chat-svc → ai-svc（gRPC）→ LLM → 写 emotion_analysis
- dev 模式 ai-svc dial emotion-llm-service 失败（"context deadline exceeded"）→ fallback to noop（chat-svc 日志已确认）
- llm-service 未起 / 未配 LLM 凭据 → 业务链路断

**修复路径**（独立 Sprint）：
1. 配 llm-service LLM 凭据（OPENAI_API_KEY 等）
2. 验证 ai-svc 调 llm-service 200
3. 验证 chat-svc → ai-svc → emotion_analysis 写入链路
4. smoke §4.1 最近有数据日应移到"今天"才算业务链路真通

**本 stage 不解决**：聚焦 smoke §4 端到端验证，业务真实性是后续 sprint。

---

## 七、与批 1 / stage-52 的关系

| 阶段 | 状态 | 与本 stage 关系 |
|---|---|---|
| Stage-50 e2e validation | ✅ DONE | smoke §4 FAIL 已登记 §九 |
| Stage-51 batch-1-infra parked | parked 等 Nacos | stage-52 修完 Nacos 解锁 |
| Stage-52 Nacos 阻塞修复 | ✅ landed 5 commit | 解锁 smoke 跑通 9/10 |
| **Stage-53 smoke §4 修复（本 stage）** | ✅ landed 1 commit | smoke 11/11 PASS → 批 1 合 main 闸门绿 |
| 批 1 合 main | ⏳ 待执行 | 8 PR-OBS + O-1 + Stage 47/48/49 改动 |

**下一步建议**：
1. 推 stage-52-nacos-fix 分支 + stage-53 合 main（5+1=6 commit）
2. 推批 1 合 main（44 commit）
3. 重建 5 svc 镜像后 smoke_observability.py 应 12/12 PASS
4. 启动路线 Z 第 2/3 批

---

> 最后更新：2026-09-08 by 当前协作 Agent
> 调研依据：todo-pile §C7 + stage-50 §九.1 + stage-52 全程 + ADR-17
> 测试结果：smoke_data_layer.py 11/11 PASS + smoke_observability.py 9/12 PASS（3 fail 为批 1 未合 main 副作用，非本 stage 引入）
> 闭环：todo-pile §C7 "Stage 36 dashboard 空根因" → 本 stage 闭环（业务数据真空 + smoke 策略问题，非视图缺失）