---
status: planned
priority: low
owner: TBD
created: 2026-10-14
type: implementation-plan
source: CL-02（2026-10-14 用户裁定 D1）—— 三条 mental-health RPC 归档时另登的 backlog 需求
depends-on: []
related-plans:
  - grpc-deadlink-actualization.md（CL-02 来源计划；本需求是它归档项的重启路径）
---

# Plan — 「我的空间」评估历史 / 趋势 / 立即评估入口

> **只描述要做的事，不含代码改动。**

## §0 上下文（写文档前的功课，按 AGENTS.md §〇）

### 0.1 现状（CL-02 亲核，2026-10-14）

| 层 | 现状 |
|---|---|
| proto | `metric.proto` 三条 RPC（`MentalHealthHistory` / `Trigger` / `Trend`）**已归档**：声明保留 + `option deprecated = true` + 注释写明重启条件（CL-02 裁定 D1） |
| server | `MentalHealthTrigger` / `MentalHealthTrend` 是**真实现**（trigger runner + job store，E2E-15 已验）；`MentalHealthHistory` 是**恒返空占位**（`Records: nil`） |
| analytics-svc HTTP | 三条路由在（`/api/v1/mental-health/{history,trigger,trend}`），**无消费方** |
| BFF | `AnalyticsClient` 接口**无**任何 `MentalHealth*` 方法（只有 `MentalAssessment`，即"综合评估"那条，已接线到 `analytics_handler.go:227`） |
| 前端 | 无入口（`apiRoutes.ts` 只留注释） |

### 0.2 相关 ADR / 决策

| 文档 | 结论 |
|---|---|
| `adr-2026-09-chart-contract-alignment.md` §E | 「5 个无消费端点（day-night / depth / frequency / mental-health/assessment）本轮不接」——用户决策「暂不接」，理由「**单独功能开发，需要 UX 设计，不在契约修复范围**」；待办清单里仍挂着未勾选的「接入前端」 |
| `adr-2026-09-mental-health-trigger-save.md` | 写入链（trigger runner 末尾 `Save`）已修通；表由 `assessment_v` 视图暴露给 analytics-svc |
| `docs/closure-roadmap/decisions.md` **CL-D-08** | CL-02 裁定：三条归档；**另登本需求** |

### 0.3 架构假设清单

| 假设 | 现状核对 |
|---|---|
| "归档"意味着要重做后端 | ❌ Trigger/Trend 的 server 是真实现 ⇒ 重启成本主要在 **BFF 端点 + 前端 UX** |
| History 可直接用 | ❌ 它是**恒返空占位** ⇒ 要走 History 必须先实现（读 `mental_health_assessments`） |
| 前端已有类似入口 | ❌ 无（`apiRoutes.ts` 只有注释） |

## §1 目标与非目标

**目标**：在「我的空间」提供 ① 评估历史列表 ② 情绪/风险趋势 ③「立即评估」按钮，数据经 **BFF → analytics-svc（gRPC）** 取，**不再走 HTTP 旁路**。

**非目标**：不改 trigger runner 的写入链；不引入新的存储；不在本计划内做记忆系统（CL-08 系列）。

## §2 待定（进入本计划时先拍板）

| # | 事项 | 说明 |
|---|---|---|
| D1 | History 后端：实现真读 vs 直接隐藏该入口 | 若只做"趋势 + 立即评估"，History 可继续归档 |
| D2 | 趋势的口径与图表形态 | 需 UX 设计（这正是 ADR §E 判它"不在契约修复范围"的原因） |
| D3 | 「立即评估」的触发频率限制 | 复用现有 job store 还是加限流 |

## §3 实施骨架（进入本计划时细化）

1. **proto**：把三条 RPC 的 `option deprecated = true` 去掉（= 重启归档项）；
2. **BFF**：`AnalyticsClient` 补对应方法 + `analytics_handler.go` 暴露端点（走 gRPC，**不加 HTTP 旁路**）；
3. **前端**：「我的空间」入口 + 图表 + 「立即评估」按钮；
4. **回归钉**：BFF handler 单测 + Playwright 端到端（含"纯文本不误标多模态"式负向对照）。

## §4 调研依据

- CL-02 阶段档 `docs/closure-roadmap/stages/cl-02-grpc-contract/plan.md` §A.1 / §7（D1）
- 证据 `…/evidence/phase2-3-terminal-states.md`
- `adr-2026-09-chart-contract-alignment.md` §E、`adr-2026-09-mental-health-trigger-save.md`
- 代码：`proto/metric.proto`、`analytics-svc/internal/grpcserver/metric_server.go`、`web-bff/internal/downstream/analytics.go`
