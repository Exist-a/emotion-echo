# Stage 83 — llm-chat-real-pipeline PR-3b：意图分布报表全链（intent 落库 → 日报饼图）

> 日期：2026-09-12
> 来源：Stage 82 收口后 roadmap 首项 → 用户"那就继续"
> 计划：[docs/legacy-plans/landed/intent-classification-6-types.md](../legacy-plans/landed/intent-classification-6-types.md)（本批随迁 landed）
> 性质：TDD 多层批次（6 commit：`0262730`/`5bb2c1d`/`25a3e68`/`b86147a`/`aaf234f` 等）

## 一、架构决策（本批最重要的调研结论）

intent 数据通路放弃"事件链透传"（MessageCreatedData→eventrow→consumer→analytics schema，
需 4 层改动 + analytics 迁移），改走 **DB 聚合**：

```
BFF 发送前调 ClassifyIntent → chat.messages.intent（migration 004）
  → msg_summary_v 视图（analytics_reader 只读，已有授权先例）
  → analytics ReportRepo 跨 schema 聚合（零事件链改动、零 analytics 迁移）
  → metric.proto intent_distribution → BFF VM → 前端日报意图饼图
```

## 二、落地内容（按层）

| 层 | 内容 |
|---|---|
| chat-svc | migration 004（messages.intent VARCHAR(16) DEFAULT ''，§契约 5：allowedIntents 白名单校验，非法落 ''；三方一致：DDL 注释 / logic / llm intent.INTENTS）+ migration 005（msg_summary_view DROP+CREATE 重建含 intent 列 + **GRANT 重授**——CREATE OR REPLACE 无法变更列集）+ model/types/SendMessage·ListMessages·grpcserver 透传 |
| BFF | chat_handler.sendMessage 发送前经 llm-service ClassifyIntent 标注（失败/未装配→空=未分类，不阻断）；SendMessageReq/MessageView/MessageItemVM 透传（intent omitempty）；main.go intentClassifier 与 streamer 共用同一 gRPC client；analytics_view 加 intentDistribution |
| analytics | DailyReport.IntentCounts（PostgresRepo 从 msg_summary_v 聚合，确定性 6 类顺序）+ metric.proto IntentCount/ReportsDailyResponse.intent_distribution=7 + metric_server 映射 |
| 前端 | utils/intent getIntentLabel（与后端白名单键一致性测试防漂移）+ types IntentCount/DailyReport.intentDistribution + dailyReport.vue 意图分布饼图（字段缺失隐藏，旧报表兼容） |

## 三、验收（dev 栈全链 e2e）

1. 发消息（UTF-8 经网关）→ 响应回带 intent：
   "我的代码报错了怎么调试"→**tech_help**；"推荐一部轻松的电影"→**lifestyle**；
   "面试被拒了好沮丧"→other（**设计行为**：面试=职业 1 分 + 沮丧=情感 1 分并列歧义归 other）
2. DB 实证：`SELECT intent, COUNT(*) ... WHERE conversation_id=28` → tech_help 1 / lifestyle 1 / other 4
3. `GET /reports/daily` → `intentDistribution=[{tech_help,1},{lifestyle,1},{other,4}]` ✓
   前端 dailyReport.vue 意图饼图直接消费
4. 回归：smoke_data_layer **11/11**（§契约 3 抓到 DROP 视图丢授权，已修）+
   §契约 7 6/6；web vitest 253/253；4 模块 go vet 0 err + test 绿

## 四、e2e 揪出并修复的 3 个问题

1. **前端日报 VM 丢 intentDistribution**（第 6 处丢字段点，与 Stage 79 contentType 同款
   ——BFF downstream 有、view 没映射）：补 analytics_view 透传（`aaf234f`）
2. **msg_summary_v 视图升级**：CREATE OR REPLACE 无法变更列集 → migration 005 DROP+CREATE
   （`25a3e68`）
3. **DROP 视图连带丢 GRANT** → §契约 3 smoke 抓到 permission denied → migration 005 补
   GRANT 重授（`b86147a`）

## 五、本批未做（open）

| 项 | 说明 |
|---|---|
| 趋势报告意图维度 | weekly/monthly/annual 暂无 intent 饼图（日报已有）；旧计划"4 页改 N 分类"的剩余部分 |
| LLM 式分类增强 | 规则式兜底已就位；key 可用时 LLM 重分类可选 |
| 歧义消息归 other 的体验 | 并列→other 是防误路由的设计；可后续用 LLM 消歧 |
| llm-service Nacos 注册 / prod bff-client 证书 | stage-81 遗留 |
| chat-svc events 包 3 个存量测试失败 | kafka_publisher Data nil 分支，Stage 73 起已有（stash/包改动史实证），与本批无关，待修 |
| Kafka P3 / §1.4 | 不变 |

## 六、调研依据

- 已读：chat-svc/{migrations/,internal/{model,types,logic,grpcserver}}、
  analytics-svc/internal/{repository/report_repository.go,grpcserver/metric_server.go,
  logic/reports_daily_logic.go}、web-bff/{internal/{downstream/{chat,chat_grpc,analytics,
  analytics_grpc,llm_grpc},handler/{chat_handler,analytics_view}}},main.go}、
  proto/{chat,metric}.proto、web/{app/types/api.ts,app/pages/chat/dashboard/dailyReport.vue,
  app/utils/emotion.ts}、deploy/db/{02,04}.sql
- 已查：msg_summary_v 授权先例（04-create-views §GRANT）、§契约 5 定义、
  user_behavior_events properties jsonb（评估后弃用事件链方案）
- 命令证据：e2e 三消息响应回带 intent + psql 分组计数 + reports/daily JSON（上录）；
  smoke 11/11（修复 GRANT 后）；vitest 253/253（复跑两次确认，首跑 1 例偶发未复现）；
  4 模块 go vet/test 绿
- 关联：决策 4、stage-82（PR-3a）、AGENTS.md §2.4（§契约 3/5 双双发挥作用）

---

> 最后更新：2026-09-12 by Stage 83 实施 session
> 关联：intent-classification-6-types.md（已迁 landed）、ai-response-structured.md（已迁 landed）
