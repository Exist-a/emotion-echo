# Stage 34 · 多模态情绪融合 — 落地报告

> **状态**：部分落地（数据层 + 算法 + Worker + analytics 报表扩展已落地；BFF /fused endpoint 待续）
> **日期**：2026-09-01
> **前置规划**：[stage-34-multimodal-fusion.md](stage-34-multimodal-fusion.md)

## 一、收口条件核对

| # | 条件 | 状态 | 证据 |
|---|------|------|------|
| 1 | ai-svc 多模态数据层（face/voice/fused 三表 + repo） | ✅ | PR-1/2/3/4/5/6 落地 + migrations 002/003/004 + 集成测试 6/6 PASS |
| 2 | Fusion Worker（每 5s tick + LLM-as-Fusion + late_fuser 兜底） | ✅ | PR-9/10/11/12/13/14 落地 + 单测全绿 |
| 3 | multimodal 端点 persist 分支（FER/SenseVoice 写库） | ✅ | PR-7/8 落地 |
| 4 | analytics-svc 报表扩展（按模态分布） | ✅ | PR-15/16 落地 + 单测全绿 |
| 5 | 真 Postgres 端到端集成测试 | ✅ | `go test -tags integration` 6/6 PASS（~21s） |
| 6 | BFF `/api/v1/emotion/message/:id/fused` 端点 | ☐ | **PR-17/18 待续**（涉及 proto 重生成 + ai-svc gRPC server + BFF handler） |
| 7 | docs/stage-34-landing.md 落地报告 | ✅ | 本文档 |

**端到端 smoke**（用户用真实链路）：要 PR-17/18 + ai-svc 新镜像构建后才能完全跑通。

## 二、git 历史（合并提交视图）

```
254f0a1 feat: Stage 34 migrations + integration tests + JSONB/ARRAY fixes
35e60b9 merge: bring in Stage 34 PR-1..14 implementation
f81ba59 feat: implement ModalityReportRepo and populate EmotionDistributionByModality
```

PR-1~14 的 14 个 commit 详情（已合并到 35e60b9）：
```
2434745 feat: implement FusionWorker to satisfy tests
f0a3026 test: add failing tests for FusionWorker
427a68d feat: implement LLMFuser to satisfy tests
331a255 test: add failing tests for LLMFuser
dc070cc feat: implement WeightedLateFuser and ModalitySnapshot
e7591ef test: add failing tests for WeightedLateFuser
ceb62a3 feat: implement PersistMultiModalAnalyzeLogic to satisfy tests
8bab584 test: add failing tests for PersistMultiModalAnalyzeLogic
e2ff921 feat: implement FusedEmotion model and FusedEmotionRepo to satisfy tests
6430e70 test: add failing tests for FusedEmotion model and FusedEmotionRepo
336b0b2 feat: implement VoiceEmotionResult model and VoiceEmotionRepo to satisfy tests
c0d48fe test: add failing tests for VoiceEmotionResult model and VoiceEmotionRepo
a8465ac feat: implement FaceEmotionResult model and FaceEmotionRepo to satisfy tests
dbd4957 test: add failing tests for FaceEmotionResult model and FaceEmotionRepo
```

## 三、新增文件清单

### 3.1 数据库 migrations（emotion_echo_ai schema）
```
emotion-echo-ai-svc/migrations/002_create_face_emotion_results.sql
emotion-echo-ai-svc/migrations/003_create_voice_emotion_results.sql
emotion-echo-ai-svc/migrations/004_create_fused_emotions.sql
emotion-echo-ai-svc/migrations/005_create_daily_emotion_by_modality_v.sql
```

### 3.2 ai-svc 数据层
```
emotion-echo-ai-svc/internal/model/face_emotion.go + _test.go
emotion-echo-ai-svc/internal/model/voice_emotion.go + _test.go
emotion-echo-ai-svc/internal/model/fused_emotion.go + _test.go
emotion-echo-ai-svc/internal/repository/face_emotion_repository.go + _test.go
emotion-echo-ai-svc/internal/repository/voice_emotion_repository.go + _test.go
emotion-echo-ai-svc/internal/repository/fused_emotion_repository.go + _test.go
emotion-echo-ai-svc/internal/repository/jsonb_helper.go
```

### 3.3 ai-svc 多模态持久化
```
emotion-echo-ai-svc/internal/logic/persistmodalanalyzelogic.go + _test.go
emotion-echo-ai-svc/internal/svc/servicecontext.go（新增 3 个 repo 字段）
```

### 3.4 ai-svc 融合算法 + Worker
```
emotion-echo-ai-svc/internal/fusion/snapshot.go
emotion-echo-ai-svc/internal/fusion/late_fuser.go + _test.go
emotion-echo-ai-svc/internal/fusion/llm_fuser.go + _test.go
emotion-echo-ai-svc/internal/fusion/worker.go + _test.go
emotion-echo-ai-svc/internal/fusion/helpers_test.go
```

### 3.5 ai-svc 集成测试
```
emotion-echo-ai-svc/integration_test/multimodal_repo_integration_test.go
```

### 3.6 analytics-svc 报表扩展
```
emotion-echo-analytics-svc/internal/repository/modality_report_repository.go + _test.go
emotion-echo-analytics-svc/internal/svc/servicecontext.go（新增字段）
emotion-echo-analytics-svc/internal/logic/reports_daily_logic.go（调新 repo）
emotion-echo-analytics-svc/internal/logic/reports_daily_logic_test.go（新增测试）
emotion-echo-analytics-svc/internal/repository/report_repository.go（DailyReport 扩字段）
```

### 3.7 规划 + 落地文档
```
docs/stage-34-multimodal-fusion.md（规划）
docs/stage-34-landing.md（本文件）
```

## 四、改动文件清单

```
emotion-echo-ai-svc/internal/svc/servicecontext.go（新增 FaceEmotionRepo/VoiceEmotionRepo/FusedEmotionRepo 字段）
emotion-echo-ai-svc/internal/fusion/late_fuser.go
emotion-echo-ai-svc/internal/fusion/late_fuser_test.go
emotion-echo-ai-svc/internal/fusion/llm_fuser.go
emotion-echo-ai-svc/internal/fusion/llm_fuser_test.go
emotion-echo-analytics-svc/internal/svc/servicecontext.go（新增 ModalityReportRepo 字段）
emotion-echo-analytics-svc/internal/repository/report_repository.go（DailyReport 扩字段）
emotion-echo-analytics-svc/internal/logic/reports_daily_logic.go（调新 repo）
```

## 五、测试覆盖

| 层 | 测试 | 结果 |
|---|---|---|
| ai-svc 单元 | `go test ./...` | 全绿（含 fusion 包 5 个测试文件） |
| ai-svc 集成 | `go test -tags integration` | 6/6 PASS（~21s，真 Postgres via testcontainers） |
| analytics-svc 单元 | `go test ./...` | 全绿（含 modality_report_repository + reports_daily_logic） |

## 六、集成测试发现并修复的 4 个真问题

1. **JSONB 空串**：GORM 把 Go `""` 直接 insert → PG `invalid input syntax for type json`
   - 修复：`internal/repository/jsonb_helper.go` 的 `normalizeJSONB` 在 Create/Upsert 前归一化为 `'{}'`

2. **`[]string` → PG `TEXT[]` 类型不匹配**（SQLSTATE 42804）
   - 修复：`model.FusedEmotion.AvailableModalities` 改为 `string`（JSONB 数组字符串）
   - 加 `model.AvailableModalitiesFromSlice` 序列化帮手
   - migration 004 把 `available_modalities TEXT[]` 改为 `JSONB`

3. **UNIQUE INDEX 查询位置**：查 `pg_constraint` 找不到 migration 显式建的 INDEX
   - 修复：测试改查 `pg_indexes`

4. **`ON CONFLICT DO UPDATE SET id=id` ambiguous**（SQLSTATE 42702）
   - 修复：改"先 SELECT 拿 ID + INSERT 兜底 ON CONFLICT DO NOTHING"两步策略
   - 语义对齐 InMemoryEmotionRepo 的 backfill 行为

## 七、Docker / testcontainers 验证

- Docker daemon：29.7.2（已确认可用）
- postgres:15-alpine 镜像：本地已有
- testcontainers/ryuk:0.14.0：自动清理 OK
- 单次集成测试：从容器创建到查询完成约 2~12s

## 八、API 变更（前端零改动）

### 8.1 ai-svc 端
| 端点 | 变化 |
|---|---|
| `POST /api/v1/multimodal/analyze` | multipart 新增 `persist` 和 `message_id` 字段（默认 `persist=false`，向后兼容） |

### 8.2 analytics-svc 端
| 端点 | 变化 |
|---|---|
| `GET /api/v1/reports/daily` | `report` 对象新增 `emotionDistributionByModality` 字段（`{text, face, voice}` 三 map，老 `emotionCounts` 字段保留） |
| 其他报表端点（weekly/monthly/annual） | **未改动**（Stage 34 仅 daily） |

### 8.3 前端
- **零改动**——`emotionDistribution` 已存在；`emotionDistributionByModality` 自动多 series
- Stage 35+ 顺手接入 ECharts 多 series 渲染

## 九、Stage 34 边界

**已完成**（多模态融合数据流）：
- FER / SenseVoice / 文字 三路结果落同一 message_id
- Fusion Worker 调 LLM 主路径 + late_fuser 兜底
- 按模态聚合 VIEW 供报表消费
- 真 Postgres 端到端验证

**未完成**（PR-17/18）：
- ai-svc gRPC `GetFusedEmotion` 实现（数据层已就绪）
- proto 重生成 + BFF `/api/v1/emotion/message/:id/fused` 端点
- ai-svc 新镜像构建（含 Fusion Worker 启动逻辑）
- docker compose 全栈 smoke（含 ai-svc 启动时跑 Worker）

## 十、Stage 35+ 候选

| 议题 | 说明 |
|---|---|
| 前端 ECharts 多 series 渲染 emotionDistributionByModality | 顺手做 |
| 时序对齐融合（每 5s 窗口） | 用户聊完天后按时间段融合 |
| 数字人表情驱动消费 fused 结果 | 实时情绪反馈 |
| FER / SenseVoice 模型升级 | 模型侧工作 |

## 十一、回归影响

- **emotion_analysis 表结构未变**（Stage 30-C 已落地）；Stage 34 是旁路新增
- **现有 AI Stream / TTS / gRPC 查询端点未变**
- **前端 SPA 0 改动**——所有新字段通过 omitempty 自动序列化
- **APISIX 路由未变**

## 十二、commit 序列（最终）

```
254f0a1 (HEAD) feat: Stage 34 migrations + integration tests + JSONB/ARRAY fixes
35e60b9 merge: bring in Stage 34 PR-1..14 implementation
f81ba59 feat: implement ModalityReportRepo and populate EmotionDistributionByModality
e95e8a5 test: add failing tests for ModalityReportRepo
2434745 feat: implement FusionWorker to satisfy tests  (PR-14)
... (PR-13 ~ PR-1 共 14 commit)
dbd4957 test: add failing tests for FaceEmotionResult model and FaceEmotionRepo  (PR-1)
e25979b docs(stage-32/33): 入仓设计文档  ← docs/stage-31-landing 分支起点
```

## 十三、参考

- 规划文档：[stage-34-multimodal-fusion.md](stage-34-multimodal-fusion.md)
- 业界方案：LLM-as-Fusion（2024 工业实践）+ Late Fusion 兜底
- AGENTS.md：TDD Red-Green-Refactor + 接口注入 + 测试替身