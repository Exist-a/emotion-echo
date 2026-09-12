---
status: landed
landed: 2026-09-12
landed-stages:
  - stage-82-intent-classification-styled-replies-2026-09-12.md（PR-3a：规则式 6 类分类 + ClassifyIntent RPC + with_intent 风格指令注入）
  - stage-83-intent-report-pipeline-2026-09-12.md（PR-3b：intent 落库 → msg_summary_v → analytics 意图分布 → 日报饼图）
  - stage-85-trend-report-intent-2026-09-12.md（趋势报告意图维度：weekly/monthly/annual 饼图全链）
residuals:
  - LLM 式分类增强（规则式为兜底）待真实 key 可用
  - 歧义消息（多意图并列）归 other 为防误路由设计，可 LLM 消歧
original-path: .trae/documents/消息分类扩展规划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-C
---

> **2026-09-12 Stage 78 排期核查注记**：本文所列文件在当前分布式代码库**全部不存在**
> （`internal/workflow/text/nodes/intent.go` / `internal/repository/message_repo.go` /
> `internal/service/report_service.go` 均为 2026-07 单体时代路径）。
> 当前事实（grep 实证）：
> - 意图分类**零命中**：ai-svc / analytics-svc / web-bff / web 均无 intent /
>   emotional_support / EmotionalSupportRate 相关代码——旧单体该链路未迁移
> - 现有"分类"是**情绪 9 分类**（happy/sad/...，`emotion-llm-service` 规则式情感词命中，
>   非 LLM），落到 analytics `emotion_analysis.primary_emotion`
> - AI 聊天回复是 BFF mock（`ai_stream_handler.go`），无 LLM prompt 可扩展
>
> **结论**：6 类意图的前置 = 真实 LLM 对话/分析链路（`llm-chat-real-pipeline.md`）。
> 链路落地后本文需按新架构重写：分类挂 llm-service（gRPC 扩展 intent 字段）→
> ai-svc 消费管道透传 → analytics 报表加 intentDistribution → BFF summary → 前端饼图。

> **Stage 82 状态注记（2026-09-12，PR-3a 已落地）**：分类侧按重写架构落地（stage-82 报告）——
> 规则式 6 类分类（`emotion-llm-service/intent.py`，关键词打分，零命中/并列 → other）+
> `ClassifyIntent` RPC + `ChatCompletion.with_intent` 首帧回带 intent + 按意图注入回复风格
> 指令。真实容器 e2e 4 类分类 + 首帧回带全过。**残余（PR-3b，报表链路）**：intent 落库 →
> analytics intentDistribution → BFF → 前端饼图，独立批次；分类器可后续用 LLM 增强规则式兜底。
> 6 分类定义与前端结构（intentDistribution 数组）仍可复用。

> **Stage 85 状态注记（2026-09-12，趋势报告意图维度已落地）**：PR-3b 残余中
> "weekly/monthly/annual 趋势报告暂无意图维度" 已销账（stage-85 报告）——
> ReportsTrendResponse 加 intent_distribution → repo 区间聚合（msg_summary_v，
> 与日报同源同义）→ BFF intentCountsToItems 确定性排序 → 三张趋势页饼图
> （字段缺失隐藏，旧下游兼容）。真实容器 e2e + psql GROUP BY 交叉实证一致。
> 剩余残余仅 LLM 式分类增强 / 歧义消歧两项（依赖真实 LLM key，非近期）。

# 消息分类扩展规划（6类）

---

## 📋 新分类定义

| 分类英文名 | 中文描述 | 说明 |
|----------|--------|------|
| `emotional_support` | 情感疏导 | 心情不好、压力大、需要安慰 |
| `study_help` | 学习问题 | 作业、学习方法、考试焦虑 |
| `tech_help` | 技术问题 | 代码、工具使用、技术选型 |
| `career_help` | 职业问题 | 职业规划、工作压力、人际关系 |
| `lifestyle` | 生活问题 | 日常建议、兴趣爱好、娱乐资讯 |
| `other` | 其他 | 无法分类、闲聊、测试消息 |

---

## 🔧 修改内容

### **1. 后端 - 意图识别 Prompt**
**文件**：`internal/workflow/text/nodes/intent.go`
**修改**：更新 Prompt 让 Kimi 返回 6 分类

### **2. 后端 - 统计逻辑扩展**
**文件**：
- `internal/repository/message_repo.go`：修改 `CountIntentTypeByUserIDAndDate` 返回所有分类的数量
- `internal/service/report_service.go`：修改报表结构，从 `EmotionalSupportRate`（单个百分比）改为 `IntentDistribution`（所有分类分布）

### **3. 后端 - 数据模型**
**文件**：`internal/service/report_service.go`
**修改**：定义新的结构体 `IntentDistribution`

### **4. 前端 - 报表显示**
**文件**：
- `dailyReport.vue`
- `weeklyReport.vue`
- `monthlyReport.vue`
- `annualReport.vue`
**修改**：从 2 分类饼图改为 N 分类饼图

### **5. 前端 - 类型定义**
**文件**：前端 API 类型定义
**修改**：更新 `DailyReport` 和 `TrendReport` 的类型

---

## 📊 数据结构变化

### **旧结构**
```typescript
emotionalSupportRate: 30  // 单个百分比
```

### **新结构**
```typescript
intentDistribution: [
  { name: '情感疏导', value: 30 },
  { name: '学习问题', value: 15 },
  { name: '技术问题', value: 25 },
  { name: '职业问题', value: 10 },
  { name: '生活问题', value: 10 },
  { name: '其他', value: 10 }
]
```

---

## ⚠️ 需要确认的问题

### 问题 1：分类中文名
| 英文名 | 你希望显示的中文名？ |
|-------|------------------|
| `emotional_support` | 情感疏导（默认） |
| `study_help` | 学习问题（默认） |
| `tech_help` | 技术问题（默认） |
| `career_help` | 职业问题（默认） |
| `lifestyle` | 生活问题（默认） |
| `other` | 其他（默认） |

### 问题 2：饼图显示
- 是否需要所有 6 分类都显示？
- 还是只显示占比 > 0 的分类？

### 问题 3：报表名称
- 从"疏导占比"改成什么？
- "意图分布"？"消息分类"？

---

## 📝 确认后我就开始修改！
