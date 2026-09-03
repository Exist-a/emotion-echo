# ADR-17 · 图表数据契约三层对齐（2026-09-03）

> **状态**：已落地 · **类型**：契约修复 · **commit 系列**：`fix/chart-contract-alignment` (90a3338 + 1590f24 + 789dd4b + 044cb58)
> **来源**：盘点 feat/bff-fused-emotion-endpoint 分支时发现，4 个 dashboard 页面（daily / weekly / monthly / annual）事实上 HTTP 200 但渲染塌掉——chartData.length === 0 → 显示"暂无数据"骨架。

---

## 上下文（Context）

前端 Emotion-Echo-Web 的 4 个 dashboard 页面（`app/pages/chat/dashboard/{daily,weekly,monthly,annual}Report.vue`）调 BFF 的两个报表端点：

- `GET /api/v1/reports/daily?user_id=&date=`
- `GET /api/v1/reports/trend?user_id=&type=&...&...`

期待拿到前端 `DailyReport` / `EmotionTrend` 形状。但实际三层契约全错位：

### 错位 1：BFF 用单数 key 包了一层

```go
// stage-30-A 写 BFF 时的代码（bug）
OK(c, gin.H{"report": report})
```

前端 `useApi.get<T>()` 把整个 `data.data` 当 `T` 返回，所以前端拿到 `{report: {...}}`——模板写 `reportData.summary` 实际是 `undefined`。

### 错位 2：analytics-svc 返回 map 而前端期望数组

```go
// downstream.AnalyticsClient.DailyReport.EmotionCounts 是 map[string]int64
EmotionCounts map[string]int64 `json:"emotionCounts"`
```

前端期望 `[{name, value}]` 形态（`api.ts:EmotionDistribution`）。

### 错位 3：缺字段

前端期望但 BFF/analytics-svc 完全不输出的字段：

- `summary: string` —— analytics-svc 0 处生成逻辑
- `wordCount: number` —— SQL 未 SUM(content_len)

### 错位 4：前端 query 参数名不一致

- weekly: `?type=weekly&start=...&end=...`
- monthly: `?type=monthly&month=YYYY-MM`
- annual: `?type=yearly&year=YYYY`

BFF / analytics-svc 期望 `start_date=` + `end_date=`。

---

## 决策（Decisions）

### §A. 修复在 BFF（presentation 层），不改 analytics-svc

理由：analytics-svc 是数据源服务，不应承担前端形状转换。BFF 已经做下游聚合和 X-User-Id 注入等职责，加 presentation 层一致。

### §B. summary 用 rule-based 模板，不调 LLM

理由：用户决策（交互中拍板）"rule-based 模板（推荐）"。LLM 路径留待 stage-37 + emotion-llm-service 重新设计的 summary pipeline。

### §C. wordCount 字段整体删除

理由：用户决策（交互中拍板）"wordCount 这玩意有啥用啊，就要今日对话次数得了呗"。产品价值低 + SQL 多加 SUM(content_len) 子查询。前端 4 个 .vue + api.ts 都删。

### §D. query 参数不一致由 BFF alias 解析承担

理由：用户决策"BFF 加 alias（推荐）"。前端 0 改动，BFF 多 30 行 normalizeTrendQuery + monthEndDay 工具。

### §E. 5 个无消费端点（day-night / depth / frequency / mental-health/assessment）本轮不接

理由：用户决策"暂不接"。是单独功能开发，需要 UX 设计，不在契约修复范围。

### §F. dev 模式下 `user_behavior_events` 表空白（KAFKA_ENABLED=false）本轮不修

理由：用户决策"不处理，但是记录"。入 stage-37 路线图。

### §G. summary 阈值语义：avg=0 算"平稳"

副作用：在 commit-3 RED 阶段发现原 `moodWord(0)` 返回"低落"。修复 `avg > 0` → `avg >= 0`。

---

## 实施细节

### BFF 新增（`emotion-echo-web-bff/internal/handler/`）

| 文件 | 用途 |
|---|---|
| `analytics_view.go` | `FrontendDailyReport` / `FrontendEmotionTrend` 结构体 + `toFrontendDailyReport` / `toFrontendTrendReport` 转换函数 |
| `summary.go` | `BuildDaily` / `BuildTrend` rule-based 模板 + `pickTopEmotion` / `moodWord` / `translateEmotion` / `monthEndDay` 辅助 |
| `summary_test.go` | 11 个 case 覆盖所有纯函数 + 阈值边界 + 闰年 2 月 |
| `analytics_handler.go` | dailyReport / trendReport 改 OK(c, toFrontendXxx(r))；trendReport 加 `normalizeTrendQuery` alias |
| `analytics_handler_test.go` | 新增 4 个 contract test（daily 形状 + trend 形状 + 2 个 alias） |

### 前端改动（`Emotion-Echo-Web/`）

- `app/types/api.ts`：`DailyReport` 删 `wordCount`；`EmotionTrend` 删 `wordCount` + `type` literal union 补 `'yearly'`
- `app/pages/chat/dashboard/{daily,weekly,monthly,annual}Report.vue`：删"总字数" stat-item

---

## 后果（Consequences）

### ✅ 正向

- 4 个 dashboard 页面真实渲染：summary 中文文本 + 饼图（情绪分布）+ 折线图（weekly/monthly/annual）+ 统计卡片（会话数/消息数）
- emotionDistribution 排序稳定（value 倒序 + name 升序）——前端 ECharts 饼图视觉一致
- BFF presentation 层可单测，纯函数无副作用
- RED 阶段发现 1 个真 bug（moodWord 阈值）
- 跨层契约测试守住未来不会回退

### ⚠️ 代价

- 破坏性 API 升级：`/reports/daily` 与 `/reports/trend` 响应从 `{report: {...}}` 变 `{...}`——前端 dashboard 必须用本次新版
- 5 个未消费端点的响应形状未变（保留 `{pattern: ...}` / `{counts: ...}` 等单数 key）
- `wordCount` 删除是产品决策，不是技术修复；如果未来要恢复，SQL + BFF + 前端需同时改

---

## 待办（stage-37 路线图）

[ ] `dev` 模式下 `user_behavior_events` 同步写库（chat-svc dev fallback 扩到 user_behavior_events）
[ ] 5 个无消费端点接入前端（day-night / depth / frequency / mental-health/assessment）
[ ] analytics_reader role GRANT `daily_emotion_by_modality_v`（Stage 34 silent bug）
[ ] summary 升级为 LLM 生成（emotion-llm-service summary pipeline）
[ ] `user_behavior_events.target` 写的是 Event.UUID 而非 message_id——数据建模 bug
[ ] `conversation.created` 与 `closed` 都映成 event_type='conversation'——细分丢失

---

## 验收

| 维度 | 结果 |
|---|---|
| BFF 单测 | 7 包全绿，新增 ≥ 15 个 case（4 contract + 11 summary） |
| BFF vet | `go vet ./...` 无 warning |
| 前端类型 | `vue-tsc --noEmit` 静默通过 |
| 浏览器实测 | dev 环境启 BFF + analytics-svc + chat-svc + Postgres，访问 4 个 dashboard 页面，summary + 饼图 + 折线均渲染 |
