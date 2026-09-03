# Stage 34 · 运维手册（Ops Runbook）

> **目标读者**：Stage 34 维护者 / QA / 后续开发者
> **目的**：沉淀 docker smoke 验证经验 + 标记"已验证 / 未验证"矩阵
> **配合文档**：[stage-34-landing.md](/docs/stages/stage-34-landing.md) 落地报告 / [stage-34-multimodal-fusion.md](/docs/stages/stage-34-multimodal-fusion.md) 规划

---

## 一、Docker Smoke 复现步骤

### 1.1 启动 Postgres

```bash
cd "D:/源码/Emotion-Echo"
# 只起 postgres（其它组件 apisix/nacos/kafka 镜像不可用，按需启动）
docker compose -f deploy/docker-compose.infra.yml up -d postgres

# 等 healthy
for i in $(seq 1 30); do
  status=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-postgres 2>&1)
  if [ "$status" = "healthy" ]; then echo "postgres ready"; break; fi
  sleep 2
done
```

### 1.2 apply Stage 34 Migrations（首次启动 / 升级时）

```bash
# init SQL 已由 postgres 容器首次启动自动跑（initdb 机制）
# Stage 34 新增的 4 个 migration 需要手动 apply：
for f in emotion-echo-ai-svc/migrations/00{2,3,4,5}_*.sql; do
  echo "=== $f ==="
  docker exec -i emotion-echo-postgres \
    psql -U postgres -d emotion_echo -v ON_ERROR_STOP=1 < "$f"
done

# 验证：6 张表应在 emotion_echo_ai schema
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo -c "\dt emotion_echo_ai.*"
```

### 1.3 构建 ai-svc 镜像（含 Stage 34）

```bash
docker build -t emotion-echo/ai-svc:v0.2.0-stage34 -f emotion-echo-ai-svc/Dockerfile .
```

**首次构建会失败**——Dockerfile 需 `deploy/tls/{ca.crt,ai-client.crt,ai-client.key}`（Stage 18 mTLS）。
**临时方案**：创建空文件让 build 通过（生产 mTLS 证书到位后删掉）：
```bash
mkdir -p deploy/tls && touch deploy/tls/ca.crt deploy/tls/ai-client.crt deploy/tls/ai-client.key
# 重新 build
docker build -t emotion-echo/ai-svc:v0.2.0-stage34 -f emotion-echo-ai-svc/Dockerfile .
```

### 1.4 启动 ai-svc（smoke 模式）

```bash
docker run -d --name emotion-echo-ai-svc-smoke \
  --network emotion-echo_app-network \
  -p 8891:8891 -p 8892:8892 \
  -e POSTGRES_DSN="host=emotion-echo-postgres port=5432 user=postgres password=postgres dbname=emotion_echo sslmode=disable search_path=emotion_echo_ai" \
  -e LLM_BASE_URL="" -e FER_BASE_URL="" -e SENSEVOICE_BASE_URL="" -e XTTS_BASE_URL="" \
  emotion-echo/ai-svc:v0.2.0-stage34

# 关键启动日志（必须看到）
sleep 8 && docker logs emotion-echo-ai-svc-smoke | grep -E "FusionWorker started|worker Run loop entered|http listening|grpc listening"
```

**预期日志**：
```
FusionWorker started (tick=5s)
worker Run loop entered, tick=5s
ai-svc HTTP server listening on 0.0.0.0:8891
ai-svc gRPC server listening on :8892
```

**已知 INFO 但不影响主流程的 ERROR**（strict=false 时不阻塞）：
- `dep=skywalking addr=${SKYWALKING_OAP_ADDR:-localhost:11800}: too many colons`（go-zero conf 不展开 `${VAR:-default}`）
- 同 `llm` / `kafka` 同理

**修复**：用 `etc/ai-api.yaml` 硬编码 `false` 让这些 dep 不被 check，或用 main.go `applyEnvOverrides` 的 env 注入。

### 1.5 Smoke 场景 1：HTTP multimodal persist

```bash
echo "fake-jpeg" > /tmp/t.jpg
curl -s -H "X-User-Id: 7" \
  -F "kind=image" -F "file=@/tmp/t.jpg" -F "filename=t.jpg" \
  -F "upload_id=smoke-img-final" -F "persist=true" -F "message_id=5005" \
  -F "user_id=7" -F "conversation_id=50" \
  http://localhost:8891/api/v1/multimodal/analyze

# 验证 face_emotion_results 落库
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo -c \
  "SELECT * FROM emotion_echo_ai.face_emotion_results WHERE message_id=5005"
# 预期：1 row (upload_id=smoke-img-final, primary_emotion=neutral)
```

### 1.6 Smoke 场景 2：Fusion Worker tick → Upsert

```sql
-- 1) 写一条 emotion_analysis（模拟 Kafka 路径）
INSERT INTO emotion_echo_ai.emotion_analysis
  (message_id, user_id, conversation_id, primary_emotion, sentiment_score, confidence, model)
VALUES (6006, 7, 50, 'happy', 0.6, 0.9, 'text-v1');

-- 2) 写占位 fused_emotions（Worker 才能找到这个 candidate）
INSERT INTO emotion_echo_ai.fused_emotions
  (message_id, user_id, conversation_id, primary_emotion, sentiment_score, confidence,
   modality_contrib, fusion_method, available_modalities)
VALUES (6006, 7, 50, 'pending', 0, 0, '{}', 'pending', '[]'::jsonb);
```

**等 5 秒**（Worker tick 周期），看日志：
```
docker logs emotion-echo-ai-svc-smoke | grep -E "tick|fused:" | tail
```
**预期**：
```
tick fired (counter=N)
tick: candidates=2 (msgIDs=[1001 6006])
msgID=1001 skipped (no text emotion)
msgID=6006 fused: emotion=happy sentiment=0.60 method=late_fusion_weighted modalities=["text"]
```

**验证 Upsert 覆盖**：
```sql
SELECT message_id, primary_emotion, sentiment_score, fusion_method, modality_contrib
FROM emotion_echo_ai.fused_emotions WHERE message_id=6006;
-- 预期: happy | 0.6 | late_fusion_weighted | {"text": 1}
```

### 1.7 Smoke 场景 3：gRPC GetFusedEmotion

```bash
# 装 grpcurl（一次）
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
export PATH=$HOME/go/bin:$PATH

# 调用（必须带 x-user-id metadata，否则 Unauthenticated）
grpcurl -plaintext \
  -proto proto/emotion_query.proto \
  -H "x-user-id: 7" \
  -d '{"message_id": 6006}' \
  localhost:8892 emotion_ai.v1.EmotionQueryService/GetFusedEmotion
```

**预期 JSON**：
```json
{
  "messageId": "6006",
  "userId": "7",
  "conversationId": "50",
  "primaryEmotion": "happy",
  "sentimentScore": 0.6,
  "confidence": 0.9,
  "modalityContrib": "{\"text\": 1}",
  "fusionMethod": "late_fusion_weighted",
  "availableModalities": "[\"text\"]",
  "createdAtMs": "..."
}
```

### 1.8 清理

```bash
docker rm -f emotion-echo-ai-svc-smoke
docker stop emotion-echo-postgres
```

---

## 二、已知 / 未知矩阵（关键）

### 2.1 ✅ 已验证（docker smoke / 单测 / 集成测试）

| 链路 | 验证方式 |
 |  |  |
| face repo CRUD + UploadID UNIQUE | 集成测试 6/6 |
| voice repoCRUD + UploadID UNIQUE | 集成测试 6/6 |
| fused repo UNIQUE message_id + Upsert 覆盖 | 集成测试 6/6 |
| LLMFuser OpenAI 兼容协议 + JSON 解析 | 单测 httptest mock |
| LateFuser 加权平均 + 模态缺失重分配 | 单测 |
| FusionWorker tick + LLM/late fallback + panic recover | 单测 + smoke |
| multimodal_handler persist 分支 | docker smoke |
| gRPC GetFusedEmotion 端到端 | docker smoke + grpcurl |
| BFF /api/v1/emotion/message/:id/fused 端点 | 单测 |

### 2.2 ⚠️ 未验证（生产环境真伪未知）

| 链路 | 原因 | 风险等级 |
|  |  |  |
| **FER 真客户端调用** | 无 Python 模型镜像（`profile: ai` 没起） | 高 |
| **SenseVoice 真客户端调用** | 同上 | 高 |
| **XTTS 真客户端调用** | 同上 | 高 |
| **face/voice 真实 emotion（非 neutral）** | FER/SenseVoice 没装 |  | 高 |
| **LLM-as-Fusion 真实调用（DeepSeek/OpenAI）** | smoke 时 `LLM_BASE_URL=""` 走 fallback |  | 高 |
| **LLM 输出 markdown 包装容错** | 代码未处理 ```json...``` 包装 |  | 高 |
| **LLM 延迟 / 限流 / 5xx** | mock 0ms，生产 1-5s |  | 中 |
| **Worker LLM 失败后 late_fuser 路径** | smoke 验证过 fallback 路径，但 LLM 真失败模式未测 |  | 中 |
| **docker compose 全栈**（含 apisix/nacos/kafka） | 部分镜像（apisix-dashboard 3.18.0-alpine）拉不到 |  | 中 |
| **yaml bool 占位符恢复** `${NACOS_ENABLED:-true}` | 当前硬编码 false，main.go `applyEnvOverrides` 已支持 |  | 中 |
| **analytical reports 端到端**（含 modality 字段） | 未起 analytics-svc 容器 |  | 低 |
| **前端 ECharts 多 series 渲染** | 前端 0 改动（schema 已就绪） |  | 低 |

---

## 三、Stage 34 待补的烟雾测试（按风险等级）

### 高风险（生产部署前必须）

1. **真实 LLM smoke（DeepSeek 或 OpenAI）**
   - 配 `LLM_BASE_URL=https://api.deepseek.com` + `LLM_API_KEY=...` + `LLM_MODEL=deepseek-chat`
   - 触发 Worker tick，看 Worker 日志：
     - `LLM miss (err=...), fallback fallback late`：LLM 真调通但失败
     - `msgID=N fused: emotion=... sentiment=... method=llm`：LLM 调通且解析成功
   - **常见失败**：
     - LLM 返回 markdown 包裹 → 加正则 strip `` ```json\n `` 与 `` ```\n ``
     - LLM 返回自然语言"该用户表达..."→ 加 prompt "只输出 JSON，不要任何其他文字" 或用 JSON mode 强制
     - LLM 返回字段名错 → 调整 prompt 模板

2. **真实 FER / SenseVoice 镜像启动**
   - 起 `docker compose --profile ai up -d`（需 emotion-echo/fer / emotion-echo/sensevoice 镜像）
   - 设 `FER_BASE_URL=http://emotion-echo-fer:8004` `SENSEVOICE_BASE_URL=http://emotion-echo-sensevoice:8002`
   - 真上传图片/音频 → 验证 face/voice emotion 字段非 neutral

### 中风险（生产稳定性）

4. **Worker 限流**：同一 messageID 在 tick 周期内不重复触发 LLM（加 LRU cache）
5. **LLM timeout 调短**（3-5s）避免 Worker 堆任务
6. **失败 metrics**：fallback 触发率 / JSON parse 失败率 / LLM latency（暴露到 Prometheus）
7. **LLM retry + circuit breaker**：连续 N 次失败降级到纯 LateFuser

### 低风险

8. **LLM 输出 schema validation**（go-playground/validator）
9. **yaml 硬编码恢复**（生产 `applyEnvOverrides`）
10. **端到端 latency 监控**（BFF → gRPC → LLM → DB 全链路）

---

## 四、容器启动故障速查表

| 现象 | 原因 | 修复 |
|  |  |  |
| `error: config file ... type mismatch for field "Nacos.enabled"` | go-zero conf 不展开 `${NACOS_ENABLED:-true}` | yaml 硬编码 false |
| `dial ${KAFKA_BROKERS:-...}: too many colons` | 同上（仅日志 ERROR，不阻塞） | yaml 硬编码 false（KAFKA / Enabled） |
| `image ... not found`（apisix-dashboard 3.18.0-alpine） | 镜像不可用 | smoke 阶段只起 postgres；全栈用其他 profile |
| `failed to calculate checksum ... deploy/tls/ai-client.key` | Dockerfile 需 TLS 证书 | 创建 deploy/tls/ 占位空文件 |
| Worker 跑但 fused_em 不更新 | PendingLister nil（main.go 漏字段） | 装配 `PendingLister: svcFusedRepo`（已 commit b8b344c） |
| Worker panic: nil pointer at line 131 | `LLMFuser` 未构造（`LLM_BASE_URL=""`） | 加 nil 防御 + panic recover（已 commit b8b344c） |
| multimodal persist=true 不写库 | handler 调老 `NewMultiModalAnalyzeLogic` 不带 persist | 改用 `NewPersistMultiModalAnalyzeLogic`（已 commit b8b344c） |
| gRPC `Unauthenticated: missing x-user-id` | 拦截器需要 metadata | `grpcurl -H "x-user-id: 7"` |

---

## 五、Stage 34 关键路径速查

```
1. POST /api/v1/multimodal/analyze?persist=true&message_id=X (BFF → ai-svc)
   ↓
   ai-svc multimodal_handler.go → PersistMultiModalAnalyzeLogic.Analyze
   ↓ (按 kind)
   ├─ image → MultiModalAnalyzer.analyzeImage → FER (or fallback)
   │         ↓
   │         FaceEmotionRepo.Create (Postgres ON CONFLICT upload_id DO NOTHING)
   │
   └─ audio → MultiModalAnalyzer.analyzeAudio → SenseVoice (or fallback)
              ↓
              VoiceEmotionRepo.Create (Postgres ON CONFLICT upload_id DO NOTHING)

2. Kafka chat-events → ai-svc consumer (Stage 30-C)
   ↓
   MessageCreatedHandler → EmotionRepo.Create (emotion_analysis 表)
   ↓
   [5 秒后] Fusion Worker tick
   ↓
   PendingLister.ListPending → fused 表所有 message_id 候选
   ↓
   对每个 candidate:
     text := EmotionRepo.GetByMessageID(msgID)
     face := FaceEmotionRepo.GetLatestByMessageID(msgID)
     voice := VoiceEmotionRepo.GetLatestByMessageID(msgID)
   ↓
   ModalitySnapshot → LLMFuser (or LateFuser fallback)
   ↓
   FusedEmotionRepo.Upsert (Postgres ON CONFLICT message_id DO UPDATE)

3. BFF GET /api/v1/emotion/message/:id/fused
   ↓
   BFF handler.byFusedMessage → EmotionQueryClient.ByFusedMessage
   ↓
   gRPC GetFusedEmotion (message_id) → ai-svc EmotionQueryService
   ↓
   Postgres: SELECT * FROM fused_emotions WHERE message_id = ?
   ↓ (存在)
   model.FusedEmotion → proto.FusedEmotion → JSON response

4. analytics-svc GET /api/v1/reports/daily
   ↓
   ReportRepo.GetDailyReport (emotion_analysis) → 老字段 emotionCounts
   ModalityReportRepo.GetDailyEmotionByModality (daily_emotion_by_modality_v VIEW) → 新字段 emotionDistributionByModality
```

---

## 六、相关 git 历史

| Commit | 含义 |
|  |  |
| `35e60b9` | merge PR-1~14（data + worker + fuser） |
| `254f0a1` | migrations 002~005 + 集成测试 + JSONB 修复 |
| `f81ba59` / `e95e8a5` | PR-15/16 analytics-svc 报表扩展 |
| `1304111` / `f798437` | PR-17/18 BFF /fused endpoint + proto 扩 |
| `d42e93f` / `ff05ba8` | landing doc（两次收口） |
| `b8b344c` | **smoke 修复**（handler / Worker LLM nil / PendingLister / yaml） |
| `c7eb89d` | landing doc 加 docker smoke 章节 |

分支：`feat/bff-fused-emotion-endpoint`