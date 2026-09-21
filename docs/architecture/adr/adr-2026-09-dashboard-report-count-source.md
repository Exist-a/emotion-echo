---
adr: 2026-09-dashboard-report-count-source
title: 报表「会话数」数据源 = msg_summary_v 同源（不得依赖 user_behavior_events）
status: accepted
date: 2026-09-21
owners: [analytics-svc, web-bff]
references: [E2E-15 FU, user-feedback-2026-09-21]
supersedes: null
---

# ADR-2026-09 · 报表「会话数」数据源 = msg_summary_v 同源

## 上下文

用户 2026-09-21 实测反馈日报卡片显示 **「0 段对话，33 条消息」** —— 自相矛盾（有 33 条消息却 0 段对话）。核查发现两处独立缺陷：

### 缺陷 1：日报 conversationCount 依赖已停更的事件链

| 指标 | 原数据源 | 当天状态 |
|------|---------|---------|
| `messageCount` | `emotion_echo_chat.msg_summary_v` | ✅ 33 条（有数据） |
| `conversationCount` | `emotion_echo_analytics.user_behavior_events`（`event_type LIKE 'conversation%'`） | ❌ **0**（该表最新事件停在 2026-09-14） |

⇒ 两个指标**数据源不同步**，产生自相矛盾的数字。DB 实测：当天真实 16 会话 / 33 消息。

### 缺陷 2：周/月/年报 conversationCount **恒为 0**（硬编码）

`emotion-echo-web-bff/internal/handler/analytics_view.go` 原代码：
```go
ConversationCount: 0, // TrendReport 没有 conv 维度；前端目前模板用 0 等同"未提供"
```
且 `messageCount` 从 `points` 累计（那是**情绪记录数**，不是消息数）⇒ 周/月/年报的「消息数」也是错值。

根因链：`TrendReport` 结构体 / proto `ReportsTrendResponse` 都**没有**这两个字段 ⇒ BFF 无处可取 ⇒ 硬编码。

## 决策

**报表的「会话数」与「消息数」必须同源，统一取自 `emotion_echo_chat.msg_summary_v`**：

1. **日报**（`GetDailyReport`）：
   - `message_count` = `COUNT(*)` WHERE 当天
   - `conversation_count` = **`COUNT(DISTINCT conversation_id)`** WHERE 当天（替换原 `user_behavior_events` 查询）
2. **趋势**（`GetTrendReport`）：
   - 新增区间聚合查询（`BETWEEN start AND end`），填 `TrendReport.MessageCount` / `ConversationCount`
   - proto `ReportsTrendResponse` 加 `message_count = 4` / `conversation_count = 5`
   - gRPC server 填充 + BFF gRPC client 透传 + `downstream.TrendReport` 加字段
   - BFF view 层删掉硬编码 `0`，直传真值；**兼容回落**：下游未提供（=0）时 messageCount 回落 points 累计（保持旧行为）

**语义定义**：「会话数」= 该时间窗内**有消息的会话数**（`COUNT(DISTINCT conversation_id)`）。这比"事件表里的 conversation 事件数"更贴近用户直觉，且与消息数天然一致（不会出现"0 会话 33 消息"）。

## 后果

### 正面
- 数字自洽：日报 `16 段对话 / 33 条消息`（实测）；周报区间 `123 / 195`（DB 核对一致）
- 周/月/年报从「恒 0」变为真实值（此前从未实现）
- 消除对 `user_behavior_events`（Kafka 事件链，dev 会滞后/停更）的报表期依赖 —— 该链路问题归 E2E-24 单独治理

### 风险 / 边界
- `msg_summary_v` 是消息表视图 ⇒ 只统计**有消息**的会话。空会话（建了但没发消息）不计入 —— 符合"会话数"直觉，但与 `user_behavior_events` 的 `conversation.created` 计数口径不同（后者含空会话）。**这是有意的语义选择**，已在 report 中记录
- `conversation_count` 全表 `COUNT(DISTINCT)` 在大数据量下需索引支撑（`msg_summary_v` 底层 `chat.messages(user_id, conversation_id)`），当前 dev 规模无压力；生产需评估

### 配套
- 回归钉：`e2e/dashboard-reports.spec.ts` #1 断言 UI 数字 = API 值 + summary 文案含「N 段对话」；#6/#7/#8 断言区间 conversationCount > 0
- Go 单测：`analytics_view_test.go` +2 用例（透传真值 / 缺值时回落）

## 关联

- 用户实测反馈（2026-09-21，截图 + 元素检查）
- 同轮修复：`ReportScaffold.vue` summary 卡片排版（`.summary-text`/`.stats-row`/`.stat-*` 此前**全仓零 CSS** ⇒ 数字与标签挤成一行裸文字）
- 相关：ADR `adr-2026-09-dashboard-responsive-breakpoints.md`（同批次报表 UI 修复）

## 变更记录

- 2026-09-21：决策落地（用户实测反馈驱动；全链路 proto + gRPC + BFF + SQL 修复；端到端 curl + Playwright 24/24 验证）
