---
status: planned
superseded-by: stage-26-N 已落实基础 2 类，6 类扩展在 backlog
original-path: .trae/documents/消息分类扩展规划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-C
---

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
