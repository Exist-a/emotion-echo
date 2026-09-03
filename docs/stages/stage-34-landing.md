# Stage 34 · 多模态情绪融合 — 落地报告

> **状态**：✅ **全部落地**（数据层 + 算法 + Worker + analytics 报表 + BFF /fused + proto 扩）
> **日期**：2026-09-01
> **前置规划**：[stage-34-multimodal-fusion.md](stage-34-multimodal-fusion.md)

## 一、收口条件核对

| # | 条件 | 状态 | 证据 |
|---|------|------|------|
| 1 | ai-svc 多模态数据层（face/voice/fused 三表 + repo） | ✅ | PR-1/2/3/4/5/6 + migrations 002/003/004 + 集成测试 6/6 PASS |
| 2 | Fusion Worker（每 5s tick + LLM-as-Fusion + late_fuser 兜底） | ✅ | PR-9/10/11/12/13/14 + 单测全绿 |
| 3 | multimodal 端点 persist 分支（FER/SenseVoice 写库） | ✅ | PR-7/8 |
| 4 | analytics-svc 报表扩展（按模态分布） | ✅ | PR-15/16 + 单测全绿 |
| 5 | 真 Postgres 端到端集成测试 | ✅ | `go test -tags integration` 6/6 PASS（~21s） |
| 6 | BFF `/api/v1/emotion/message/:id/fused` 端点 | ✅ | PR-17/18 + proto 扩 + ai-svc gRPC server |
| 7 | docs/stage-34-landing.md 落地报告 | ✅ | 本文档 |

## 二、git 历史（合并提交视图）

```
1304111 (HEAD) feat: BFF /fused endpoint + ai-svc gRPC GetFusedEmotion + proto 扩
d42e93f docs(stage-34-landing): 落地报告 + 收口条件核对
f798437 test: add failing tests for EmotionQueryHandler fused endpoint
254f0a1 feat: Stage 34 migrations + integration tests + JSONB/ARRAY fixes
35e60b9 merge: bring in Stage 34 PR-1..14 implementation
f81ba59 feat: implement ModalityReportRepo and populate EmotionDistributionByModality
e95e8a5 test: add failing tests for ModalityReportRepo
```

PR-1~14 的 14 个 commit 详情（已合并到 35e60b9）：
```
2434745 feat: implement FusionWorker to satisfy tests
f0a3026 test: add failing tests for FusionWorker
427a68d feat: implement LLMFuser to satisfy tests
331a255 test: add failing tests for LLMFuser
dc070cc feat: implement WeightedLateFuser and ModalitySnapshot
e7591ef test: add failing tests for WeightedLateFuser
ceb62e feat: implement PersistMultiModalAnalyzeLogic to satisfy tests
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

### 3.7 BFF /fused endpoint + proto 扩（PR-17/18）
```
proto/emotion_query.proto（新增 GetFusedEmotion RPC + FusedEmotion message）
emotion-echo-shared/pkg/emotionquery/emotion_query.pb.go（重生成）
emotion-echo-shared/pkg/emotionquery/emotion_query_grpc.pb.go（重生成）
emotion-echo-ai-svc/internal/grpcserver/server.go（emotionQueryServer 加 GetFusedEmotion 实现）
emotion-echo-web-bff/internal/downstream/emotion_query.go（EmotionQueryClient 加 ByFusedMessage）
emotion-echo-web-bff/internal/handler/emotion_query_handler.go（加 /fused 路由 + handler）
emotion-echo-web-bff/internal/handler/emotion_query_handler_test.go（4 个测试）
```

### 3.8 规划 + 落地文档
```
docs/stage-34-multimodal-fusion.md（规划）
docs/stage-34-landing.md（本文件）
```

## 四、改动文件清单

```
emotion-echo-ai-svc/main.go（openPostgres 多返 *gorm.DB + grpcserver.New 扩 fusedRepo）
emotion-echo-ai-svc/internal/svc/servicecontext.go（新增 FaceEmotionRepo/VoiceEmotionRepo/FusedEmotionRepo 字段）
emotion-echo-ai-svc/internal/grpcserver/server.go（grpcserver.New 签名扩 + GetFusedEmotion 实现）
emotion-echo-ai-svc/internal/grpcserver/server_test.go（New 调用更新）
emotion-echo-ai-svc/internal/fusion/late_fuser.go
emotion-echo-ai-svc/internal/fusion/late_fuser_test.go
emotion-echo-ai-svc/internal/fusion/llm_fuser.go
emotion-echo-ai-svc/internal/fusion/llm_fuser_test.go
emotion-echo-analytics-svc/internal/svc/servicecontext.go（新增 ModalityReportRepo 字段）
emotion-echo-analytics-svc/internal/repository/report_repository.go（DailyReport 扩字段）
emotion-echo-analytics-svc/internal/logic/reports_daily_logic.go（调新 repo）
emotion-echo-web-bff/internal/handler/emotion_query_handler.go（/fused 路由 + gRPC status 映射）
emotion-echo-web-bff/internal/handler/emotion_query_handler_test.go（PR-17 RED tests）
emotion-echo-web-bff/internal/handler/analytics_handler_test.go（fake 加 ByFusedMessage）
proto/emotion_query.proto（GetFusedEmotion RPC + FusedEmotion message）
emotion-echo-shared/pkg/emotionquery/emotion_query.pb.go（protoc 重生成）
emotion-echo-shared/pkg/emotionquery/emotion_query_grpc.pb.go（protoc 重生成）
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
| gRPC `GetFusedEmotion` | 新 RPC：按 `message_id` 查 `fused_emotions` 表 |

### 8.2 BFF 端
| 端点 | 变化 |
|---|---|
| `GET /api/v1/emotion/message/:id/fused` | 新增，调 ai-svc gRPC `GetFusedEmotion` |

### 8.3 analytics-svc 端
| 端点 | 变化 |
|---|---|
| `GET /api/v1/reports/daily` | `report` 对象新增 `emotionDistributionByModality` 字段（`{text, face, voice}` 三 map，老 `emotionCounts` 字段保留） |
| 其他报表端点（weekly/monthly/annual） | **未改动**（Stage 34 仅 daily） |

### 8.4 前端
- **零改动**——`emotionDistribution` 已存在；`emotionDistributionByModality` 自动多 series
- Stage 35+ 顺手接入 ECharts 多 series 渲染

## 九、Stage 34 边界

**已完成**（多模态融合数据流）：
- FER / SenseVoice / 文字 三路结果落同一 message_id
- Fusion Worker 调 LLM 主路径 + late_fuser 兜底
- 按模态聚合 VIEW 供报表消费
- BFF `/api/v1/emotion/message/:id/fused` 端点暴露融合结果
- 真 Postgres 端到端验证（单元 + 集成测试）
- **docker compose smoke 验证通过**（HTTP persist + Worker tick + gRPC GetFusedEmotion）

**待续**（生产部署）：
- yaml bool 占位符恢复 `${NACOS_ENABLED:-true}`（main.go `applyEnvOverrides` 已支持）
- docker compose 全栈（含 apisix / nacos / kafka，目前镜像需重打）
- 前端 ECharts 多 series 渲染 emotionDistributionByModality（前端零改动，schema 已就绪）

**Stage 34 已知/未知矩阵**（详见 [stage-34-ops-runbook.md §二](stage-34-ops-runbook.md#二已知--未知矩阵关键)）：

| 状态 | 链路 |
|---|---|
| ✅ 已验证 | face/voice/fused repo CRUD、Worker 调度、gRPC GetFusedEmotion、BFF /fused、HTTP persist 分支 |
| ⚠️ 未验证（生产前必须） | **FER 真客户端**、**SenseVoice 真客户端**、**LLM-as-Fusion 真实调用**、LLM 输出 markdown 包装容错 |

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
b8b344c (HEAD) feat(ai-svc): Stage 34 wire-up + smoke fixes (handler / Worker / yaml)
1304111 feat: BFF /fused endpoint + ai-svc gRPC GetFusedEmotion + proto 扩 (PR-18)
d42e93f docs(stage-34-landing): 落地报告 + 收口条件核对 (PR-19)
f798437 test: add failing tests for EmotionQueryHandler fused endpoint (PR-17)
254f0a1 feat: Stage 34 migrations + integration tests + JSONB/ARRAY fixes
35e60b9 merge: bring in Stage 34 PR-1..14 implementation
f81ba59 feat: implement ModalityReportRepo and populate EmotionDistributionByModality
e95e8a5 test: add failing tests for ModalityReportRepo
2434745 feat: implement FusionWorker to satisfy tests  (PR-14)
... (PR-13 ~ PR-1 共 14 commit)
dbd4957 test: add failing tests for FaceEmotionResult model and FaceEmotionRepo  (PR-1)
e25979b docs(stage-32/33): 入仓设计文档  ← docs/stage-31-landing 分支起点
```

## 十三、Docker 端到端 smoke 验证（已完成）

Stage 34 完成后用 docker compose 起 postgres + 重建 ai-svc 镜像（v0.2.0-stage34）+ grpcurl 验证真实链路。

**启动**：
```
docker compose -f deploy/docker-compose.infra.yml up -d postgres  # only postgres（无 apisix/nacos/kafka 拉镜像失败）
docker run -d --name emotion-echo-ai-svc-smoke \
  --network emotion-echo_app-network \
  -p 8891:8891 -p 8892:8892 \
  -e POSTGRES_DSN="host=emotion-echo-postgres ..." \
  emotion-echo/ai-svc:v0.2.0-stage34
```

**apply migrations**：
```
for f in emotion-echo-ai-svc/migrations/00{2,3,4,5}_*.sql; do
  docker exec -i emotion-echo-postgres psql -U postgres -d emotion_echo < "$f"
done
# 6 tables in emotion_echo_ai: emotion_analysis, face_emotion_results, fused_emotions,
#                               voice_emotion_results, voice_transcripts, face_detections
```

**smoke 1：HTTP multimodal persist**
```bash
curl -H "X-User-Id: 7" \
  -F "kind=image" -F "file=@/tmp/t.jpg" -F "filename=t.jpg" \
  -F "upload_id=smoke-img-final" -F "persist=true" -F "message_id=5005" \
  -F "user_id=7" -F "conversation_id=50" \
  http://localhost:8891/api/v1/multimodal/analyze
# → {"kind":"image","emotion":"neutral",...}

# 验证落库
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo -c \
  "SELECT * FROM emotion_echo_ai.face_emotion_results WHERE message_id=5005"
# → 1 row (smoke-img-final)
```

**smoke 2：Fusion Worker tick → Upsert**
```sql
INSERT INTO emotion_echo_ai.emotion_analysis (message_id, user_id, ..., primary_emotion)
VALUES (6006, 7, ..., 'happy');
INSERT INTO emotion_echo_ai.fused_emotions (message_id, ..., primary_emotion)
VALUES (6006, 7, ..., 'pending');  -- 占位行
-- 等 5 秒
```

Worker 日志（每 5s tick）：
```
tick fired (counter=1)
tick: candidates=2 (msgIDs=[1001 6006])
msgID=1001 skipped (no text emotion)
msgID=6006 fused: emotion=happy sentiment=0.60 method=late_fusion_weighted modalities=["text"]
```

数据库验证（Upsert 覆盖）：
```sql
SELECT message_id, primary_emotion, sentiment_score, fusion_method, modality_contrib
FROM emotion_echo_ai.fused_emotions WHERE message_id=6006;
-- happy | 0.6 | late_fusion_weighted | {"text": 1}
```

**smoke 3：gRPC GetFusedEmotion**
```bash
grpcurl -plaintext -proto proto/emotion_query.proto \
  -H "x-user-id: 7" -d '{"message_id": 6006}' \
  localhost:8892 emotion_ai.v1.EmotionQueryService/GetFusedEmotion
```
```json
{
  "messageId": "6006",
  "userId": "7",
  "primaryEmotion": "happy",
  "sentimentScore": 0.6,
  "modalityContrib": "{\"text\": 1}",
  "fusionMethod": "late_fusion_weighted",
  "availableModalities": "[\"text\"]"
}
```

**smoke 发现的真问题（全部修复）**：

1. **handler 没接 PersistMultiModalAnalyzeLogic**：PR-7/8 实现完成但 multimodal_handler.go 仍调老的 NewMultiModalAnalyzeLogic，导致 persist=true 默默无效
2. **Worker LLMFuser nil panic**：main.go 在 LLM_BASE_URL 空时不构造 → worker tick 时 nil.Fuse() panic
3. **yaml bool 占位符坑**：go-zero conf 不展开 `${VAR:-default}` 给 bool 字段（type mismatch），需 yaml 硬编码 false 让 main.go applyEnvOverrides 生效
4. **Worker PendingLister 未装配**：main.go 漏了 PendingLister 字段，导致 tick 内 candidates=空 → 不处理任何消息
5. **svc.NewServiceContext 缺 faceRepo/voiceRepo/fusedRepo**：导致 multimodal handler 即便 persist=true 也没有 repo 写入

## 十三、参考

- 规划文档：[stage-34-multimodal-fusion.md](stage-34-multimodal-fusion.md)
- 业界方案：LLM-as-Fusion（2024 工业实践）+ Late Fusion 兜底
- AGENTS.md：TDD Red-Green-Refactor + 接口注入 + 测试替身