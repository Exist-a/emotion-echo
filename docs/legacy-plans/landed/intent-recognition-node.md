---
status: landed
superseded-by: stage-26-N-bugfix.md
original-path: .trae/documents/意图识别节点实现计划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# 意图识别前置节点实现计划

## 目标

在在线工作流前添加意图识别节点，判断用户询问是否属于情感疏导类：
- 情感疏导类 → 执行情绪分析 + Prompt选择
- 其他类 → 跳过情绪分析，使用默认Prompt

---

## 用户需求总结

| 需求 | 说明 |
|------|------|
| 意图分类 | 仅需"情感疏导"和"其他"两类 |
| 处理逻辑 | 条件分支，非情感类跳过情绪分析 |
| 结果存储 | 需要存储，用于后续报告的疏导占比统计，添加字段即可 |
| 工作流 | 仅在线工作流添加 |

---

## 实现步骤

### 步骤 1: 创建意图识别节点

**文件**: `internal/workflow/text/nodes/intent.go`

**功能**:
- 调用 LLM 判断用户输入是否属于情感疏导
- 返回分类结果: `emotional_support` 或 `other`
- 支持详细调试日志

### 步骤 2: 修改消息模型添加字段

**文件**: `internal/models/message.go`

**新增字段**:
```go
type Message struct {
    // ... 现有字段 ...
    IntentType string `gorm:"column:intent_type;type:varchar(50);default:'other'"` // 意图类型
}
```

### 步骤 3: 修改在线工作流

**文件**: `internal/workflow/text/workflow.go`

**调整流程**:
```
IntentRecognition → [条件分支]
                      ├─ 情感疏导 → EmotionAnalysis → PromptSelector
                      └─ 其他 → 使用默认Prompt
```

### 步骤 4: 修改 AI 服务

**文件**: `internal/service/ai_service.go`

**修改**:
- 在执行工作流后获取意图类型
- 将意图类型保存到消息记录中

### 步骤 5: 更新数据库迁移

**文件**: `migrations/xxx_add_intent_type.sql`

**SQL**:
```sql
ALTER TABLE messages ADD COLUMN intent_type VARCHAR(50) DEFAULT 'other';
```

### 步骤 6: 更新报告服务

**文件**: `internal/service/report_service.go`

**新增功能**:
- 添加疏导占比统计查询方法

---

## 工作流流程图

```
用户输入消息
       │
       ▼
┌─────────────────────────────┐
│     IntentRecognition       │  ← 新增节点
│   判断是否情感疏导          │
└─────────────────────────────┘
       │
       ▼
    [条件分支]
       │
   ┌───┴───┐
   │       │
   ▼       ▼
情感疏导   其他
   │       │
   ▼       │
┌───────────────┐   │
│ EmotionAnalysis│   │
│   情绪分析    │   │
└───────────────┘   │
       │           │
       ▼           │
┌───────────────┐   │
│ PromptSelector│   │
│  选择Prompt  │   │
└───────────────┘   │
       │           │
       └───┬───────┘
           │
           ▼
    AI响应生成
```

---

## 字段设计

### IntentType 字段

| 字段名 | 类型 | 长度 | 默认值 | 说明 |
|--------|------|------|--------|------|
| intent_type | VARCHAR | 50 | 'other' | 意图类型: emotional_support / other |

---

## 风险评估

| 风险 | 描述 | 应对措施 |
|------|------|----------|
| LLM调用失败 | 意图识别失败 | 降级为 'other' |
| 分类不准确 | LLM分类错误 | 人工校验，后续优化Prompt |
| 性能影响 | 增加一次LLM调用 | 可配置开关控制是否启用 |

---

## 依赖关系

```
步骤 1 → 步骤 2 → 步骤 3 → 步骤 4 → 步骤 5 → 步骤 6
```

---

## 预期输出

### 调试日志示例

```
╔══════════════════════════════════════════════════════════════════════════╗
║                  [INTENT RECOGNITION NODE]                           ║
╚══════════════════════════════════════════════════════════════════════════╝
  [INPUT] User message: 我最近心情不太好
  [ACTION] Calling LLM for intent classification...
  [LLM RESPONSE] {"intent":"emotional_support","confidence":0.92}
  [OUTPUT] intent=emotional_support, confidence=0.9200
  [DECISION] Proceed to emotion analysis
╔══════════════════════════════════════════════════════════════════════════╗
║              [INTENT RECOGNITION NODE - END]                          ║
╚══════════════════════════════════════════════════════════════════════════╝
```

### 数据库存储

```sql
SELECT intent_type, COUNT(*) as count 
FROM messages 
WHERE user_id = ? 
GROUP BY intent_type;
```

---

## 下一步

请确认此计划是否符合预期，或是否需要调整。
