---
status: in-progress
stage: CL-07
slug: cl-07-fusion-consumption
scope: 后端切片（fusion 消费）—— 前端半（[id].vue 回显）不在本轮
owner: Lane-CL-G（接手原 C 会话位）
created: 2026-10-16
source-plan: docs/plans/fusion-result-consumption.md（Phase 1~3）
related:
  - ../roadmap.md（§2 末 G 轨注）
  - ../parallel-contract.md（§三 占位 / §四 握手）
  - ../../plans/chat-emotion-realtime-display.md（前端回显，另轮）
---

# CL-07 · 情绪回显 + 融合消费（**后端切片**）— 执行计划

> **本阶段只做"后端半"**：融合聚合视图 → analytics-svc 报表消费 → proto/BFF 透传。
> **前端半**（对话页顶部状态条 / 消息气泡融合标签 / 数字人情绪）要动
> `pages/chat/conversation/[id].vue` + `stores/message.ts` ⇒ 与在飞 **D 轨（CL-05）** 冲突，
> 待 D 释放该页后另开（见 `roadmap.md §2` G 轨注）。

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-ai-svc/internal/model/fused_emotion.go` | `FusedEmotion`：`message_id UNIQUE` / `primary_emotion` / `fusion_method`(`llm`\|`late_fusion_weighted`) / `available_modalities`(JSONB) / `sentiment_score` / `confidence`；`DeletedAt gorm.DeletedAt`（i007 加的软删除列） |
| `emotion-echo-ai-svc/migrations/i004_create_fused_emotions.sql` | 表 + `uq(message_id)` + `idx_fused_user_time(user_id, created_at DESC)`（按 user×时间聚合现成高效） |
| `emotion-echo-ai-svc/migrations/i005_create_daily_emotion_by_modality_v.sql` | 按模态视图**注释明确排除** fused（"fused 是收敛结果，不与单模态并列聚合"）⇒ 融合产物**无任何聚合消费方** |
| `emotion-echo-analytics-svc/internal/repository/modality_report_repository.go` | 既有"跨 schema 视图消费"范式：接口 + InMemory 替身 + Postgres 实现（本阶段照此复制） |
| `emotion-echo-analytics-svc/internal/logic/reports_daily_logic.go` | 日报装配：`ModalityReportRepo` 可选（nil 跳过）；本阶段并列新增 fused 装配 |
| `emotion-echo-analytics-svc/internal/grpcserver/metric_server.go` | `ReportsDaily` RPC 映射 `EmotionCounts`→proto；**此前 modality/fused 均不跨 gRPC** |
| `emotion-echo-analytics-svc/main.go:109-113,196-199` | svcCtx 生产装配点；`ModalityReportRepo` **从未在此赋值**（另登账 CL-F-24） |
| `proto/metric.proto:121-137` | `ReportsDailyResponse`（原仅 summary/emotion_distribution/message_count/conversation_count/intent_distribution） |
| `emotion-echo-web-bff/internal/downstream/analytics{,_grpc}.go` | BFF 侧 `DailyReport` DTO + gRPC 映射 |
| `emotion-echo-web-bff/internal/handler/analytics_view.go` | `toFrontendDailyReport` 前端契约变换 |
| `proto/gen.sh` | 生成方式：`bash proto/gen.sh metric.proto`（cwd=输出目录 + `paths=source_relative`） |

### A.2 相关 ADR / 决策

- 已查：`docs/closure-roadmap/decisions.md`（CL-D-07：融合情绪写侧=CL-07 / 读侧=CL-04；PG+Redis 双存）
- 已查：`docs/plans/fusion-result-consumption.md`（§2.2 链路设计 + §5 风险表）、`docs/plans/chat-emotion-realtime-display.md`
- 已查：`deploy/db/migrate.sh`（checksum 校验：改已应用迁移 ⇒ `die`）、`emotion-echo-analytics-svc/migrations/a004_create_analytics_reader_role.sql`（新视图授权范式）

### A.3 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| fused 可按 user×day 高效聚合 | ✅ `idx_fused_user_time(user_id, created_at DESC)` |
| 融合是实时产生的 | ❌ FusionWorker 5s 批处理 ⇒ 秒级延迟；日报按天聚合不受影响（计划 §5 已记） |
| 日报链路已有 proto→analytics→BFF 通道 | ✅ 通道在；**但新字段须逐层加**（proto/生成桩/gRPC 映射/BFF DTO/前端契约） |
| 新视图的 analytics_reader 授权可加进 a004 | ❌ a004 已应用，改它触发 `migrate.sh` checksum `die` ⇒ **必须新增独立迁移 a011** |
| 生产已接线 modality 维度 | ❌ **未接线**（新登 CL-F-24） |

### A.4 未跑 smoke 声明

✅ **已跑**（本轮，仅 🟢 单测/契约级，**未起 dev 栈** —— 栈被 D 轨持有）：
`go test ./...`（analytics-svc / web-bff / shared 全绿）、`bash proto/gen.sh metric.proto`（未改 proto 时**逐字节复现**）、
`python scripts/check_proto_layout.py`（通过）。
🔴 **未跑**（需栈，排第 6 位）：`smoke_data_layer.py` 端到端数据契约实拍（§契约 1~4）。

## §1 目标与非目标

**目标**：把"融合产物（`fused_emotions`）算了没人用"补成"被报表消费"——
① 融合情绪日聚合视图；② analytics-svc 消费仓储 + 日报装配；③ proto/BFF 透传（前端图表可接）。

**非目标**：不改 FusionWorker 融合算法；不改按模态视图（i005）；**不做前端**（顶部状态条/气泡标签/数字人，另轮）；
不做 G5（融合注入 prompt，属 CL-04 读侧）；不登记 E2E-F。

## §2 测试点清单

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-1 | 视图 i010 幂等 + 只读 fused + 过滤软删除 + 含 fusion_method 维度 | [A] | `go test ./migrations/ -run TestI010`（字面量契约钉） | PASS（1/1） |
| T-2 | 新视图授权 analytics_reader（防 A4 型 permission denied） | [A] | `go test ./migrations/ -run TestA011` | PASS（1/1） |
| T-3 | `FusedReportRepo`（InMemory）空态/预置/多用户/错误/ping/接口一致 | [A] | `go test ./internal/repository/ -run TestFusedReport` | PASS（6/6） |
| T-4 | 日报 logic 装配 fused（有值装配 / nil 跳过 / 错误冒泡） | [A] | `go test ./internal/logic/ -run TestReportsDailyLogic` | PASS |
| T-5 | gRPC `ReportsDaily` 映射 fused（分布/方法占比/多模态占比）+ 无数据不报错 | [A] | `go test ./internal/grpcserver/ -run 'ReportsDaily.*Fused'` | PASS |
| T-6 | BFF gRPC 客户端映射 fused 进 DTO + 无数据 nil | [A] | `go test ./internal/downstream/ -run 'DailyReport_MapsFused'` | PASS |
| T-7 | BFF 前端契约 `toFrontendDailyReport` 透传 fused（倒序/空态/nil） | [A] | `go test ./internal/handler/ -run Fused` | PASS |
| T-8 | 生产接线：svcCtx.FusedReportRepo 在 main.go 赋值 | [A] | 源码断言（main.go:196 起） | PASS（代码审查） |
| T-9 | **端到端数据契约实拍**：dev 栈下 `/api/v1/reports/daily` 返回非空 `fusedEmotionDistribution` | [A] | `smoke_data_layer.py` / curl（**需栈，排第 6 位**） | **BLOCKED**（D 持栈；非"未做"） |

## §3 分阶段实施（TDD：先 RED 再 GREEN）

| Phase | 内容 | RED 断言 | 状态 |
|---|---|---|---|
| 1 | migration：`daily_emotion_fused_v`（i010）+ analytics_reader 授权（a011） | 视图/授权契约钉失败（文件缺失） | ✅ GREEN |
| 2 | analytics-svc `FusedReportRepo`（接口+InMemory+Postgres）+ svcCtx + 日报装配 + main 接线 | logic 测试断言字段被装配 → 失败 | ✅ GREEN |
| 3 | proto `ReportsDailyResponse` 融合字段 + 重新生成 + gRPC 映射 + BFF DTO/前端契约 | gRPC/BFF 测试断言 fused 存在 → 编译失败/断言失败 | ✅ GREEN |

**排序理由**：Phase 1 是数据源；Phase 2 是消费端（🟢 单测）；Phase 3 是跨层契约（含 proto 生成）。全部 🟢 不需栈。

## §4 收口判据

- **功能**：融合产物从 DB → 视图 → 报表 API → （前端图表可接）**全链贯通**；无融合数据时优雅降级（字段缺失）。
- **机器**：上表 T-1~T-8 全绿；`go build ./...`（3 模块）；`go vet`；proto 生成未改时逐字节复现；`check_proto_layout.py` 通过。
- **诚实边界**：T-9（端到端实拍）**未做**（栈锁在 D），阶段判 **partial**、**不得判 done**。

## §5 回归钉

- Go 契约/单元钉（本阶段新增 6 个测试函数 + 3 个文件）：
  `i010_..._test.go` · `a011_..._test.go` · `fused_report_repository_test.go` ·
  `reports_daily_logic_fused_test.go` · `metric_server_fused_test.go` ·
  `analytics_grpc_fused_test.go` · `analytics_view_fused_test.go`。
- 前端 Playwright 钉：**不在本轮**（无前端改动）。

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| 改已应用迁移触发 checksum `die` | `migrate.sh` rc=2 直接 die | 授权走**新迁移 a011**，不改 a004 |
| proto 生成方式不一致 ⇒ 全量 diff | 工具链版本漂移 | 先跑 `gen.sh` 在**未改 proto** 时逐字节复现再改（已验） |
| `available_modalities` 是 JSONB | `?` 操作符需 `::jsonb` | 视图显式转换（i010） |
| InMemory 替身时区键不匹配 | `time.Time` 作 key 含时区 | 归一到 UTC 零点（`fusedDayKey`）—— **本阶段 TDD 实际抓到并修复** |
| 跨 schema 读需 GRANT | 缺授权 = A4 型 permission denied | 新迁移 a011 授权 + 契约钉 |

## §7 调研依据（本次实读）

- 文件：`emotion_echo_ai` 的 `fused_emotion.go` / `i004` / `i005` / `i007`；analytics-svc 的 `modality_report_repository.go` / `reports_daily_logic.go` / `metric_server.go` / `main.go`；BFF 的 `downstream/analytics{,_grpc}.go` / `handler/analytics_view.go`；`proto/metric.proto` / `proto/gen.sh`；`deploy/db/migrate.sh`；`emotion-echo-analytics-svc/migrations/a004_*.sql`
- 计划：`docs/plans/fusion-result-consumption.md`、`docs/plans/chat-emotion-realtime-display.md`
- 决策：`docs/closure-roadmap/decisions.md`（CL-D-07）
- 命令：`go test ./...`（3 模块）/ `bash proto/gen.sh metric.proto` / `python scripts/check_proto_layout.py`
