# Stage 85 — 趋势报告意图维度（weekly/monthly/yearly 意图分布饼图）

> 日期：2026-09-12
> 类型：feat（TDD：RED 三层编译红 → GREEN 逐层转绿）
> 关联：Stage 83 §五 open 表第 1 项、Stage 82 PR-3b（日报意图链，本批为同构推广）

---

## 一、背景

Stage 82 PR-3b 给**日报**打通了意图分布链（messages.intent → msg_summary_v →
DailyReport.IntentCounts → proto intent_distribution → BFF intentDistribution →
dailyReport.vue 饼图）。三张**趋势报告**（weekly/monthly/annualReport.vue）当时
未覆盖，是 PR-3 残余。

设计决策：趋势报告的意图维度采用与日报同构的**区间聚合**（整个 [start, end]
窗口一个分布），不做 per-bucket（桶 × 6 类的 per-point 分布对饼图场景无意义且
负载高）。前端渲染与日报同款 ECharts 饼图，字段缺失时隐藏（旧下游兼容）。

## 二、落地内容（按层）

| 层 | 内容 |
|---|---|
| proto | `ReportsTrendResponse` 加 `repeated IntentCount intent_distribution = 3`（复用现有 IntentCount message）+ gen.sh 重新生成 shared pb |
| analytics repo | `TrendReport.IntentCounts map[string]int64`；`PostgresReportRepo.GetTrendReport` 加区间 SQL（msg_summary_v，`send_time::date BETWEEN $2 AND $3 AND intent <> ''`，与日报同源同义） |
| analytics grpcserver | 提取 `intentDistToProto()` helper（6 类白名单确定性顺序，白名单外不输出），ReportsDaily/ReportsTrend 共用；ReportsTrend 响应映射 |
| BFF downstream | `TrendReport.IntentCounts`；`analyticsGRPCClient.TrendReport` 映射 proto 重复字段 → map（防"第 N 处丢字段"——Stage 79/83 同类 bug 的第 7 个潜在点） |
| BFF view | `FrontendEmotionTrend.IntentDistribution`（omitempty）；提取 `intentCountsToItems()` helper 与日报共用确定性排序 |
| 前端 | `EmotionTrend.intentDistribution?: IntentCount[]`；weekly/monthly/annualReport.vue 三页加"意图分布"饼图（getIntentLabel 中文标签，字段缺失隐藏） |

## 三、RED→GREEN 过程

RED（三处编译红，全部"unknown field"于目标契约字段）：
1. `TestAnalyticsServer_ReportsTrend_IntentDistribution_DeterministicOrder`
   （metric_server_test，InMemoryReportRepo 注入乱序 IntentCounts + 白名单外
   unknown_intent，断言输出白名单相对顺序且 unknown 不出现；须带 x-user-id
   metadata 过拦截器）
2. `TestAnalyticsHandler_TrendReport_IntentDistribution_Transparent` +
   `_OmittedWhenEmpty`（BFF handler，透传顺序 + 空时字段整体缺席）
3. `TestAnalyticsGRPCClient_TrendReport_MapsIntentDistribution` + `_NilIntentDistribution`
   （新文件 analytics_grpc_test.go，bufconn mock analytics server，锁 proto→map
   映射不丢字段）

GREEN：proto/repo/metric_server → BFF downstream/view → 前端，逐层实现后全部转绿。

## 四、验收

- analytics-svc / web-bff / chat-svc（含 Stage 84 回归）`go test ./...` 全绿 +
  `go vet` 0 err；shared `go build` 过
- web vitest **253/253**（首跑 1 例偶发失败未复现，与 Stage 83 记录一致）；
  typecheck 错误总数 96 = 历史基线（改动文件零新增）
- smoke_data_layer **11/11** PASS
- **真实容器 e2e**：重建 analytics-svc + web-bff 镜像并滚动重启（healthy）后，
  `GET /api/v1/reports/trend?user_id=1&type=weekly&start=2026-09-06&end=2026-09-12`
  → `intentDistribution=[{tech_help,1},{lifestyle,1},{other,4}]`，与
  `SELECT intent, COUNT(*) FROM msg_summary_v ... BETWEEN ... GROUP BY 1`
  DB 实证逐条吻合（即 Stage 83 验收时落库的同一批数据，交叉验证通过）

## 五、本批未做（open）

| 项 | 说明 |
|---|---|
| LLM 式分类增强 / 歧义消息消歧 | 规则式兜底已就位；key 可用时 LLM 重分类可选（Stage 83 §五遗留不变） |
| 前端三页 vitest 页面级测试 | 仓库无页面级测试先例（dailyReport.vue 亦无），与现状一致；e2e 已实证 |
| 其余 open 项 | llm-service Nacos 注册 / Kafka P3 / web typecheck 96 处（不变） |

## 六、调研依据

- 已读：analytics-svc/internal/{repository/report_repository.go,
  grpcserver/{metric_server,server}.go, logic/reports_trend_logic.go,
  svc/servicecontext.go, types/reports.go}、web-bff/internal/{downstream/{analytics,
  analytics_grpc,chat_grpc_test},handler/{analytics_handler,analytics_view,
  analytics_handler_test}}、proto/metric.proto、proto/gen.sh、
  web/app/{types/api.ts,pages/chat/dashboard/{daily,weekly,monthly,annual}Report.vue,
  utils/intent.ts}、scripts/{smoke_data_layer.py,build_dev_images.sh}、
  deploy/docker-compose.apps.yml
- 已查：Stage 83 §五 open 表、Stage 82 PR-3b 日报意图链模式、决策 4（gRPC 契约）
- 命令证据：RED 编译红 3 处输出；e2e 响应 JSON + psql GROUP BY 输出（上录）；
  smoke 11/11；vitest 253/253；typecheck 96（= 基线）
- 关联：AGENTS.md §2.4（§契约 3/4 范围 PR，已跑 smoke + e2e）

---

> 最后更新：2026-09-12 by Stage 85 实施 session
> 关联：stage-83（日报意图链先例）、stage-84（合并门槛恢复）
