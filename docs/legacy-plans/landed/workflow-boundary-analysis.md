---
status: landed
superseded-by: specs/text-workflow-refactor + stage-33
original-path: .trae/documents/工作流边界分析.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# 后端工作流边界分析

## 当前三个工作流的定位

| 工作流 | Package | 用途 | 执行时机 | 输出 |
|--------|---------|------|----------|------|
| `workflow.NewWorkflow()` / `AnalysisWorkflow()` | `workflow` (root) | 批量会话情绪分析 | 离线/定时 | `EmotionAnalysis` 写入 DB |
| `chat.BuildEmotionWorkflow()` | `chat` | 实时情绪检测+Prompt选择 | 在线/同步（200-500ms） | 选择 AI 回复风格 |
| `assessment.Workflow` | `assessment` | 深度心理健康评估 | 离线/按需 | `MentalHealthAssessment` 报告 |

---

## 边界问题分析

### 问题 1: `workflow` (root) 与 `chat` 都有情绪分析节点

**表现**:
- `workflow` root: `EmotionAnalysisNode` → 输出 `state.EmotionScores`, `state.DominantEmotion`
- `chat`: `emotion_analysis` → 输出 `state["emotion"]`, `state["confidence"]`

**问题**:
- 两者功能重复，都是做情绪分析
- `workflow` root 已有 `chat` 的功能（情绪分析），但 `chat` 包又单独实现了一套

**建议**: `chat` 包应直接复用 `workflow` root 的情绪分析能力

---

### 问题 2: `workflow` (root) 与 `assessment` 功能部分重叠

**表现**:
- `workflow` root: 消息提取 → 情绪分析 → 关键词提取 → 摘要生成
- `assessment`: 数据收集 → ReAct深度分析 → 风险评估 → 干预建议 → 报告生成

**问题**:
- `workflow` root 是轻量级分析，输出 `EmotionAnalysis`（单会话）
- `assessment` 是深度分析，输出 `MentalHealthAssessment`（多维度）
- 但两者都会处理消息和进行情绪分析，职责有重叠

**建议**: 明确 `workflow` root 是否为遗留代码，或者作为轻量级分析保留

---

### 问题 3: `EmotionWorker` 使用 `workflow` root，但实际未在 main.go 初始化

**表现**:
```go
// main.go 第 99 行
emotionWorker := worker.NewEmotionWorker(cfg, convRepo, msgRepo, analysisRepo, userRepo)
// 注意：这里没有传入任何 workflow 实例！
```

而 `EmotionWorker.AnalyzeConversation` 内部调用 `workflow.AnalysisWorkflow()`

**问题**:
- 工作流被硬编码在 worker 内部，main.go 无法统一配置
- `emotionWorker` 和 `aiService`（用 `chat.BuildEmotionWorkflow`）各自持有不同的工作流

---

### 问题 4: 命名混淆

| 当前名称 | 问题 |
|----------|------|
| `workflow.AnalysisWorkflow` | 太通用，不体现"情绪分析" |
| `chat.EmotionWorkflow` | 实际上还做了 Prompt 选择 |
| `assessment.Workflow` | 清晰，但包名 `assessment` 与整体命名风格不一致 |

---

## 建议的边界划分

### 方案 A: 明确三层架构

| 层级 | 工作流 | 职责 |
|------|--------|------|
| **在线层** | `chat.EmotionWorkflow` | 实时情绪检测 + Prompt选择（快，~500ms） |
| **离线轻量层** | `workflow.AnalysisWorkflow` | 单会话情绪分析 + 摘要（批处理） |
| **离线深度层** | `assessment.Workflow` | 多维度心理健康评估（复杂，5阶段） |

**问题**: `workflow` root 与 `chat` 仍有功能重叠

---

### 方案 B: 合并重复，去掉 `chat` 中的情绪分析

将 `chat.EmotionWorkflow` 改为只做 **Prompt 选择**，情绪分析直接调用 `workflow` root 的能力：

```
emotion_analysis（复用 workflow） → prompt_selector
```

---

### 方案 C: 将 `workflow` root 标记为遗留，逐步废弃

如果 `assessment` 已经覆盖了 `workflow` root 的所有功能，考虑：
1. 将 `EmotionWorker` 迁移到使用 `assessment.Workflow`
2. 移除 `workflow` root 包中的冗余节点

---

## 结论

**核心问题**: `workflow` root 的 `EmotionAnalysisNode` 与 `chat` 的 `emotion_analysis` 功能重复，两者边界不清晰。

**推荐处理**: 
1. 如果 `chat` 只需要实时情绪检测用于选择 Prompt → 保留现有实现（因为 `chat` 需要快速响应）
2. 如果 `workflow` root 正在被 `EmotionWorker` 大量使用 → 考虑将情绪分析能力统一到一个地方
3. 明确 `EmotionWorker` 是否可以被 `assessment.Workflow` 替代

需要我进一步分析或给出具体的重构方案吗？
