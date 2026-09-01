# Stage 34 · 多模态情绪融合规划

> 状态：**Approved（已审批）** · 日期：2026-09-01 · 目标分支：`feat/stage-34-multimodal-fusion`
> ADR 编号：14（多模态融合算法：LLM-as-Fusion + late fusion 兜底）

## 一、问题陈述

当前 `emotion-echo-ai-svc` 的多模态路径（`internal/analyzer/multimodal.go`）**只做"分派 + 音频内串行"**，不是真正的多模态融合：

- **image 路径**：仅 FER → 输出 emotion，**完全不读 in.Text**
- **audio 路径**：SenseVoice → 转写 → 关键词器算 sentiment，最后做**算术平均**（`avgFloat`）
- **Kafka consumer**：收到 `message.created` 只跑**纯文本分析**（关键词器/gRPC llm-service），与 FER/SenseVoice 结果**没有任何关联**
- `emotion_echo_ai.emotion_analysis` 表**只有单标签 + 单 model 字段**，无法识别"这是文本还是语音还是人脸"的产物

**业务后果**：用户聊天的真实情绪应该是"文字 + 面部 + 语音"综合判断，但当前架构每条消息的情绪标签只反映其中一路（默认文本）。

## 二、设计目标

1. **同一 message_id 下聚合三路数据**（text/face/voice）
2. **前端零改动**——复用报表页的 emotionDistribution ECharts，扩展数据 schema 让 ECharts 自动多画 series
3. **TDD 严格执行**——每张表、每个 repo、每个 worker 都先 RED 再 GREEN
4. **接口注入**——FER/SenseVoice/LLM 客户端走已有 `aiclient` interface，repo 走 `repository.XxxRepo` interface（AGENTS.md §三.1）
5. **不破坏现有契约**——`emotion_analysis` 表保留，新增的 face/voice/fused 表是**旁路数据源**

## 三、数据模型

### 3.1 新增三张表（全部在 `emotion_echo_ai` schema）

```sql
-- 002_create_face_emotion_results.sql
CREATE TABLE IF NOT EXISTS emotion_echo_ai.face_emotion_results (
    id BIGSERIAL PRIMARY KEY,
    upload_id VARCHAR(64) UNIQUE,             -- 前端上传去重（client-side nonce）
    message_id BIGINT,                        -- ★ 与 messages 表关联（可空：用户可能上传无人脸）
    user_id BIGINT NOT NULL,
    conversation_id BIGINT,
    primary_emotion VARCHAR(32),              -- FER 7 类：happy/sad/angry/neutral/calm/anxious/...
    emotion_scores JSONB DEFAULT '{}',        -- 各类别概率
    confidence REAL,
    model VARCHAR(64),                        -- "fer:fer" / "fer:opencv-dnn" / ...
    raw_response JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_face_message ON emotion_echo_ai.face_emotion_results(message_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_face_user_time ON emotion_echo_ai.face_emotion_results(user_id, created_at DESC);

-- 003_create_voice_emotion_results.sql
CREATE TABLE IF NOT EXISTS emotion_echo_ai.voice_emotion_results (
    id BIGSERIAL PRIMARY KEY,
    upload_id VARCHAR(64) UNIQUE,
    message_id BIGINT,                        -- ★ 与 messages 表关联
    user_id BIGINT NOT NULL,
    conversation_id BIGINT,
    transcript TEXT,                          -- SenseVoice 转写文本（可空：识别失败）
    primary_emotion VARCHAR(32),              -- SenseVoice 情绪 token
    emotion_scores JSONB DEFAULT '{}',
    confidence REAL,
    model VARCHAR(64),                        -- "sensevoice:sensevoice-small"
    duration_ms INT,
    language VARCHAR(16),
    raw_response JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_voice_message ON emotion_echo_ai.voice_emotion_results(message_id, created_at DESC);

-- 004_create_fused_emotions.sql
CREATE TABLE IF NOT EXISTS emotion_echo_ai.fused_emotions (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL UNIQUE,        -- ★ 每条消息至多一个融合结果
    user_id BIGINT NOT NULL,
    conversation_id BIGINT NOT NULL,
    primary_emotion VARCHAR(32) NOT NULL,
    sentiment_score REAL,
    confidence REAL,
    modality_contrib JSONB DEFAULT '{}',      -- {"text": 0.4, "voice": 0.3, "face": 0.3}
    reasoning TEXT,                           -- LLM 输出（可空：走兜底时为空）
    fusion_method VARCHAR(32),                -- "llm" | "late_fusion_weighted"
    available_modalities TEXT[],              -- ["text","voice","face"]
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_fused_user_time ON emotion_echo_ai.fused_emotions(user_id, created_at DESC);
```

### 3.2 现有 `emotion_analysis` 表**不变**（保持向后兼容）

新融合结果存 `fused_emotions`，老路径文本情绪继续写 `emotion_analysis`。两条路径独立，前端先消费 `fused_emotions`（有则用），fallback 到 `emotion_analysis`。

### 3.3 跨 schema VIEW 扩展（analytics-svc 可读）

```sql
-- 005_create_daily_emotion_by_modality_v.sql
CREATE OR REPLACE VIEW emotion_echo_ai.daily_emotion_by_modality_v AS
-- 文本：来自 emotion_analysis
SELECT user_id, DATE_TRUNC('day', created_at)::date AS day,
       primary_emotion, 'text' AS modality, COUNT(*) AS cnt,
       AVG(sentiment_score) AS avg_sentiment, AVG(confidence) AS avg_confidence
FROM emotion_echo_ai.emotion_analysis
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion
UNION ALL
-- 人脸
SELECT user_id, DATE_TRUNC('day', created_at)::date, primary_emotion, 'face', COUNT(*),
       NULL::float8, AVG(confidence)
FROM emotion_echo_ai.face_emotion_results
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion
UNION ALL
-- 语音
SELECT user_id, DATE_TRUNC('day', created_at)::date, primary_emotion, 'voice', COUNT(*),
       NULL::float8, AVG(confidence)
FROM emotion_echo_ai.voice_emotion_results
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion;
```

## 四、模块设计

### 4.1 ai-svc 改动

**新文件**：
```
emotion-echo-ai-svc/internal/
├── model/
│   ├── face_emotion.go            # FaceEmotionResult model + TableName + test
│   ├── voice_emotion.go           # VoiceEmotionResult model + test
│   └── fused_emotion.go           # FusedEmotion model + test
├── repository/
│   ├── face_emotion_repository.go # FaceEmotionRepo interface + InMemory + Postgres
│   ├── voice_emotion_repository.go
│   └── fused_emotion_repository.go
├── fusion/
│   ├── fusion.go                  # FusionEngine：扫三路齐→融合
│   ├── llm_fuser.go               # LLM-as-Fusion 实现（调 BFF_LLM_BASE_URL）
│   ├── late_fuser.go              # 加权兜底
│   └── *_test.go
├── logic/
│   └── persistmodalanalyzelogic.go # 现有 multimodalanalyzelogic 的 persist=true 分支
```

**改动文件**：
- `main.go`：在 `buildServiceContext` 里新增 FaceEmotionRepo / VoiceEmotionRepo / FusedEmotionRepo 装配；启动 Fusion Worker goroutine
- `internal/svc/servicecontext.go`：扩字段
- `internal/logic/multimodalanalyzelogic.go`：加 `persist bool` 参数分支，FER/SenseVoice 成功后写对应结果表
- `internal/handler/multimodal_handler.go`：multipart form 增加 `persist` + `message_id` 字段

### 4.2 Fusion Worker（核心新组件）

**触发**：每 5 秒一轮扫描（用现有 `Clock` interface + ticker）

**算法**（LLM-as-Fusion 主路径 + late fusion 兜底）：

```go
// 伪代码
func (w *FusionWorker) tick(ctx) {
    // 1. 找"有 text 但 face/voice 未到"的 message_id（5 分钟 TTL）
    candidates := w.fusedRepo.FindPending(ctx, ttl=5*time.Minute)

    for _, msgID := range candidates {
        text := w.emotionRepo.GetByMessageID(ctx, msgID)        // 来自 emotion_analysis
        face := w.faceRepo.GetLatestByMessageID(ctx, msgID)      // 可空
        voice := w.voiceRepo.GetLatestByMessageID(ctx, msgID)     // 可空

        modalities := buildModalitySnapshot(text, face, voice)   // 含 confidence + scores

        var fused *FusedEmotion
        if w.llmClient != nil && len(modalities) >= 2 {
            fused = w.llmFuser.Fuse(ctx, modalities)             // 调 BFF_LLM_BASE_URL
        }
        if fused == nil {
            fused = w.lateFuser.Fuse(modalities)                // 加权平均兜底
        }
        w.fusedRepo.Upsert(ctx, fused)                          // UNIQUE(message_id) 保证幂等
    }
}
```

**LLM prompt 模板**（注入 BFF_LLM_BASE_URL，OpenAI 兼容协议）：
```
你是一个情绪融合器。下面是同一消息的多路情绪识别结果（每路含 emotion/confidence/scores）。

{modalities_json}

请综合判断：
1. 综合情绪标签（happy/sad/angry/neutral/calm/anxious/...）
2. sentiment_score（-1 到 1）
3. 每路贡献度（0-1，总和=1）
4. 简短 reasoning（一句话）

输出 JSON：{"primary_emotion": ..., "sentiment_score": ..., "modality_contrib": ..., "reasoning": ...}
```

**接口注入**（AGENTS.md §三.1）：
- `LLMFuser` interface（`Fuse(ctx, modalities) (*FusedEmotion, error)`）
- `LateFuser` interface
- `Clock` interface（用 shared 的 `pkg/clock`，避免 `time.Sleep`）

### 4.3 analytics-svc 改动

**新文件**：
- `internal/repository/modality_report_repository.go`：`EmotionDistributionByModality(userID, dateRange) → {text: {...}, voice: {...}, face: {...}}`

**改动文件**：
- `internal/repository/report_repository.go`：`DailyReport` 加新字段 `EmotionDistributionByModality`（保留 `EmotionCounts` 老字段，老前端继续工作）
- `internal/logic/reports_daily.go`：调新 repo 方法填充新字段

**前端兼容**：前端 `dailyReport.vue:54` 读 `reportData.emotionDistribution`（来自老字段），**继续工作**。新字段 `emotionDistributionByModality` 由前端**可选**消费（不在 Stage 34 前端改动范围）。

### 4.4 BFF 改动

**新文件**：
- `internal/handler/fused_emotion_query_handler.go`：`GET /api/v1/emotion/message/:id/fused`
- `internal/downstream/ai_fused.go`：调 ai-svc gRPC `GetFusedEmotion(messageID)`

**改动文件**：
- `proto/emotion_query.proto`：加 `GetFusedEmotion` RPC
- `emotion-echo-shared/pkg/emotionquery/`：重新生成 stub
- `emotion-echo-ai-svc/internal/grpcserver/server.go`：实现 `GetFusedEmotion`（查 `fused_emotions` 表）

## 五、PR 拆分（严格 TDD，每个 PR 一个完整循环）

| # | 分支 | 范围 | TDD 步骤 |
|---|---|---|---|
| **PR-1** | `test/ai-svc-face-model-and-repo` | model + repo RED | 写 FaceEmotionResult model/repo 测试（fail）→ 实现（pass） |
| **PR-2** | `feat/ai-svc-face-model-and-repo` | GREEN | 接 PR-1 |
| **PR-3** | `test/ai-svc-voice-model-and-repo` | 同 PR-1 模式（voice） |
| **PR-4** | `feat/ai-svc-voice-model-and-repo` | GREEN |
| **PR-5** | `test/ai-svc-fused-model-and-repo` | FusedEmotion + repo（含 UNIQUE 幂等） |
| **PR-6** | `feat/ai-svc-fused-model-and-repo` | GREEN |
| **PR-7** | `test/ai-svc-multimodal-persist` | multimodalanalyzelogic 扩 persist=true 分支 |
| **PR-8** | `feat/ai-svc-multimodal-persist` | GREEN（含 message_id 透传） |
| **PR-9** | `test/ai-svc-late-fuser` | late_fuser 单元测试（加权平均） |
| **PR-10** | `feat/ai-svc-late-fuser` | GREEN |
| **PR-11** | `test/ai-svc-llm-fuser` | LLM fuser 测试（httptest mock LLM） |
| **PR-12** | `feat/ai-svc-llm-fuser` | GREEN |
| **PR-13** | `test/ai-svc-fusion-worker` | FusionWorker tick 测试（fake repo + fake fuser） |
| **PR-14** | `feat/ai-svc-fusion-worker` | GREEN（含 main.go 装配 + ticker） |
| **PR-15** | `test/analytics-svc-modality-report` | 新 repo 方法测试 |
| **PR-16** | `feat/analytics-svc-modality-report` | GREEN（含 SQL + VIEW） |
| **PR-17** | `test/bff-fused-emotion-endpoint` | BFF handler 测试（mock downstream） |
| **PR-18** | `feat/bff-fused-emotion-endpoint` | GREEN（含 proto 重生成 + ai-svc gRPC 实现） |
| **PR-19** | `docs/stage-34-landing` | Stage 34 落地报告 + 收口条件核对 | 文档 |

**集成测试**（build tag `integration`，不参与 `go test ./...`）：
- ai-svc：`fusion_integration_test.go`（真 Postgres + testcontainers + httptest fake LLM）
- analytics-svc：`modality_report_integration_test.go`（真 Postgres，验 VIEW 数据）
- BFF：`fused_emotion_integration_test.go`

## 六、TDD 红绿节奏（以 PR-1/2 为例）

```
# PR-1 RED
$ git checkout -b test/ai-svc-face-model-and-repo
$ # 编辑 emotion-echo-ai-svc/internal/model/face_emotion.go（不存在 → 编译失败）
$ # 编辑 emotion-echo-ai-svc/internal/model/face_emotion_test.go
$ go test ./internal/model/...  # 红：no such file / undefined
$ git add -A && git commit -m "test: add failing test for FaceEmotionResult model"

# PR-2 GREEN
$ git checkout -b feat/ai-svc-face-model-and-repo
$ # 写最小实现
$ go test ./internal/model/...  # 绿
$ go test ./...  # 全绿
$ git add -A && git commit -m "feat: implement FaceEmotionResult model to satisfy test"

# PR-2 REFACTOR（如需要）
$ go test ./...  # 保持绿
$ git commit -m "refactor: extract FaceEmotion gorm tags"
```

## 七、文件清单（最终态）

**新增**（20 个文件）：
```
emotion-echo-ai-svc/internal/model/{face_emotion,voice_emotion,fused_emotion}.go + _test.go ×3
emotion-echo-ai-svc/internal/repository/{face_emotion,voice_emotion,fused_emotion}_repository.go + _test.go ×3
emotion-echo-ai-svc/internal/fusion/{fusion,llm_fuser,late_fuser}.go + _test.go ×3
emotion-echo-ai-svc/internal/logic/persistmodalanalyzelogic.go + _test.go
emotion-echo-ai-svc/migrations/00{2,3,4,5}_*.sql ×4
emotion-echo-analytics-svc/internal/repository/modality_report_repository.go + _test.go
emotion-echo-web-bff/internal/handler/fused_emotion_query_handler.go + _test.go
emotion-echo-web-bff/internal/downstream/ai_fused.go + _test.go
proto/emotion_query.proto（扩字段）
emotion-echo-shared/pkg/emotionquery/emotion_query.pb.go + _grpc.pb.go（regen）
```

**改动**（7 个文件）：
```
emotion-echo-ai-svc/main.go（装配）
emotion-echo-ai-svc/internal/svc/servicecontext.go（扩字段）
emotion-echo-ai-svc/internal/logic/multimodalanalyzelogic.go（persist 分支）
emotion-echo-ai-svc/internal/handler/multimodal_handler.go（form 字段）
emotion-echo-ai-svc/internal/grpcserver/server.go（GetFusedEmotion 实现）
emotion-echo-analytics-svc/internal/repository/report_repository.go（新字段）
emotion-echo-analytics-svc/internal/logic/reports_daily.go（调新 repo）
```

## 八、风险与边界

| 风险 | 缓解 |
|---|---|
| LLM fuser 增加延迟（同步调用 200~500ms） | Worker 异步执行，不阻塞 chat 主链路；用户聊天不感知 |
| LLM 调用失败 | 自动 fallback 到 late_fuser 加权平均 |
| 融合延迟（5s tick + 5min TTL） | `fused_emotions` UNIQUE 约束兜底重复写入；前端 fallback 到 `emotion_analysis` |
| `fused_emotions` 与 `emotion_analysis` 双写 | 前端消费 fused 优先；老路径仍工作（向后兼容） |
| face/voice message_id 缺失 | 表 `message_id` 可空；融合算法处理单/双模态场景 |
| analytics-svc VIEW 跨 schema 读 ai-svc | 走 search_path，沿用现有模式（`analytics_reader_role`） |

## 九、收口条件（Stage 34 Done）

- [ ] 19 个 PR 全部合并，CI 全绿（`go test ./...` + `go vet ./...` + `go test -tags integration ./...`）
- [ ] ai-svc 测试覆盖率 ≥ 80%（service / handler / repository）
- [ ] analytics-svc / BFF 覆盖率 ≥ 80%
- [ ] 端到端 smoke：上传图片/语音到 `/api/v1/multimodal/analyze?persist=true&message_id=X` → 5 秒内 `fused_emotions` 出现 X 的融合结果
- [ ] 报表 API 返回 `emotionDistributionByModality` 字段（即便前端暂不消费）
- [ ] BFF `/api/v1/emotion/message/:id/fused` 返回 `{perModality: [...], fused: {...}}`
- [ ] `docs/stage-34-landing.md` 落地报告

## 十、不在 Stage 34 范围

- 前端 ECharts 多 series 渲染（schema 已就绪，下次前端迭代顺手做）
- 实时时序对齐融合（每 5s 窗口）—— Stage 35+
- 数字人表情驱动消费 fused 结果—— Stage 35+
- FER/SenseVoice 模型升级/重训
- `emotion_analysis` 表结构改造（保留老路径完全不变）

## 十一、参考

- **业界方案**：LLM-as-Fusion（2024 工业实践）+ Late Fusion 兜底
- **现有可复用**：
  - `emotion-echo-shared/pkg/clock`（Clock interface，避免 `time.Sleep`）
  - `emotion-echo-ai-svc/internal/aiclient/interfaces.go`（FERService/SenseVoiceService/XTTSService 模式）
  - `emotion-echo-ai-svc/internal/repository/emotion_repository.go`（InMemory + Postgres 双实现，event_id 幂等模式）
  - `emotion-echo-ai-svc/migrations/001_add_event_id_to_emotion_analysis.sql`（migration 写法参考）
- **AGENTS.md 强约束**：TDD Red-Green-Refactor、依赖接口、测试替身、覆盖率底线
