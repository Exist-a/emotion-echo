# Stage 26-T — 测试缺口 Backlog（多 Session TDD 推进规划）

> **范围声明**：本文档是 **测试补全的路线图留存**，先行于任何测试代码落地。
> 本次 session 的唯一动作：**新建本文档**，**不修改**任何实现代码 / 测试代码 / Helm / docker-compose / APISIX / 前端。
> 真正动手时按 §五 滚动执行表的优先级顺序，分多个 session 推进；每段循环在独立 commit 内走完 Red → Green → Refactor。

继承：
- `AGENTS.md §0.1 ALL CODE IS TDD 🌱🔴 🟢 ♻️`（项目硬规则，2026-07-15 生效）
- Stage 26-M（`55c27a5 test(stage-26-M): add analytics-svc internal tests` 等 6 个内部测试 commit）建立了 baseline
- Stage 26-K / 26-L（Postgres 集成测试 + smoke 脚本）建立了集成测试与脚本体系
- 当前状态（2026-08-29）：shared/pkg 是金标准（82% 覆盖）；5 个 Go svc 中 ai-svc / chat-svc / user-svc / assessment-svc 已基本合规；**analytics-svc 仅脚手架**（无业务实现）；**3 个 Python AI 服务 0 测试**；**前端 81% 文件无测试**

---

## 一、目标

| # | 决议 | 选择 |
|---|------|------|
| 1 | 文档定位 | **Backlog + 滚动执行表**，非一次性 commit；多 session 推进 |
| 2 | 测试栈 | **保留**：Go `stretchr/testify`（assert + require + suite + mock），Python `pytest + pytest-asyncio + httpx`，前端 `Vitest + Vue Test Utils + Pinia Testing`，E2E `Playwright` |
| 3 | 严格度 | **不简化**：禁止 snapshot-copy 字典/常量；禁止 `t.Skip`；禁止测后置；禁止 `init()` 注入全局状态 |
| 4 | 命名约定 | 遵守 `AGENTS.md §1.1`：`_test.go` 与实现文件同包同名 sibling；不允许多实现合并到一个 `_test.go` |
| 5 | 覆盖率底线 | service / handler / repository 80%+；pkg 工具包 90%+；三方适配层 70%+（DB/Kafka/gRPC/SkyWalking） |
| 6 | 单测时长 | `go test ./...` ≤ 5s；`pytest tests/unit/` ≤ 30s；`vitest run` ≤ 30s |
| 7 | 集成测试 | Go 用 `//go:build integration` build tag，默认不进入 `go test ./...`；Python 同理 |
| 8 | 排除项 | `legacy/emotion-echo-gin`（已 archived，弃用）不进入本 backlog；MediaStream/WebGL/IndexedDB 等需特殊环境的前端代码不在本次 backlog 范围（按 AGENTS §四「不写不可重现测试」原则） |
| 9 | 顺序 | Go 后端 → Python 后端 → Frontend，三轮 session 推进 |
| 10 | 提交前缀 | `test(<svc>): red — <场景>` / `feat(<svc>): <实现>` / `refactor(<svc>): <抽象>` |

---

## 二、TDD 三原则在本 backlog 中的具体含义

| 原则 | 含义 | 违规判定（命中任一即返工） |
|------|------|----------------------------|
| 🔴 RED 先于实现 | 任何新增 / 修改行为，先写失败测试 | 提交历史出现「先 feat 后 test」即违规 |
| 🟢 GREEN 最小实现 | 只写让测试通过的最少代码 | 实现中出现「未在测试断言中用到的字段」即冗余 |
| ♻️ REFACTOR 保持绿 | 重命名 / 抽象 / 提公共，重构后测试仍绿 | 测试行为被改即违规（测试不是橡皮泥） |

### 禁止条款（来自 AGENTS.md §四 + 本 backlog 加严）

1. ❌ **禁止 snapshot-copy 常量字典**：
   - 现状违反：`emotion-llm-service/tests/unit/test_analyze_pure.py`（拷贝 `EMOTION_KEYWORDS` / `SENTIMENT_WORDS` 快照）
   - 现状违反：`Emotion-Echo-LLM/FER/tests/unit/test_emotion_mapping.py`（拷贝 `EMOTION_MAPPING` 快照）
   - 重构要求：从被测模块 import；如被测模块未导出常量，则先在源文件中导出，再让测试 import
2. ❌ **禁止 `t.Skip` 跳过无法写出的测试**：
   - 现状扫描：grep `t.Skip` / `pytest.skip` 在测试文件中必须为 0 命中（除非 `hasKindCluster` / `hasDocker` 这类环境守卫）
3. ❌ **禁止把多个 `_test.go` 合并为 `*_test.go`**：
   - 现状违反：`emotion-echo-ai-svc/internal/aiclient/ai_client_test.go` 单文件覆盖 fer / sensevoice / xtts（应拆为 `fer_test.go` / `sensevoice_test.go` / `xtts_test.go`）
4. ❌ **禁止删除 shared/pkg 已有的 fake / mock**：
   - `InMemoryEmotionRepo`、`KafkaEventPublisher` 的 inmemory fallback、`stubAnalyzer` 等不得删除
5. ❌ **禁止在测试包导生产 API**（污染仓库）

---

## 三、Go 后端部分（第 1 轮 session）

### 3.1 `emotion-echo-chat-svc`（补 2 个 logic 测试 + 1 个 handler 补强）

| 文件 | LOC | 缺口 | 测试场景（最小集） |
|------|-----|------|-------------------|
| `internal/logic/sendmessagelogic.go` | 127 | 无 `_test.go` | happy path（user owns conv → append msg → Kafka publish）；wrong owner → 403；empty content → 400；Kafka producer 抛错 → 5xx；ctx cancel → ctx.Err() |
| `internal/logic/listmessageslogic.go` | 75 | 无 `_test.go` | happy path（分页）；empty result；page out of range → empty list；ctx cancel |
| `internal/handler/chat_handler.go` | 88 | 已有 `chat_handler_test.go`（需扩覆盖） | 新增子测试：route 不存在 → 404；method not allowed；JSON 解析失败 → 400 |

**commit 链（5 个）**：
```
test(chat-svc): red — sendmessagelogic happy + owner check + kafka failure
feat(chat-svc): sendmessagelogic scaffold (uses existing logic body)
test(chat-svc): red — listmessageslogic pagination + empty
feat(chat-svc): listmessageslogic scaffold
test(chat-svc): red — chat_handler 404/405/400 routes
feat(chat-svc): chat_handler error mapping refactor
```

### 3.2 `emotion-echo-ai-svc`（补 8 个文件测试）

| 文件 | LOC | 缺口 | 测试场景（最小集） |
|------|-----|------|-------------------|
| `internal/logic/aihealthlogic.go` | 102 | 无 `_test.go` | happy（all deps up → healthy）；Kafka down → degraded；Postgres down → degraded；LLM stub → partial |
| `internal/logic/getemotionbymessagelogic.go` | 52 | 无 `_test.go` | happy（msgID 存在）；not found → 404；ctx cancel |
| `internal/logic/listemotionbyconversationlogic.go` | 55 | 无 `_test.go` | happy（convID 存在 + 多个 emotion）；empty list；ctx cancel |
| `internal/logic/multimodalanalyzelogic.go` | 81 | 无 `_test.go` | image → FER 客户端 happy / err fallback；audio → SenseVoice 同；text → emotion_llm；混合 → priority order |
| `internal/logic/synthesizespeechlogic.go` | 77 | 无 `_test.go` | happy（XTTS 200）；XTTS 5xx → 502；XTTS 4xx → 400；空文本 → 400；超长文本 → 截断 |
| `internal/handler/emotion_handler.go` | 61 | 无 `_test.go` | route 404；JSON 解析 400；Gin ctx 注入 userID |
| `internal/handler/multimodal_handler.go` | 123 | 无 `_test.go` | multipart 解析；content-type 不匹配 → 415；model 字段 unknown → 400 |
| `internal/analyzer/auth_wrapped.go` | 36 | 无 `_test.go` | inner success → success；inner error → wrapped error；mock inner Analyzer |
| `internal/aiclient/ai_client_test.go` | 227 | 违反 sibling 约定 | **重构**：拆为 `fer_test.go` / `sensevoice_test.go` / `xtts_test.go`（无功能变更，仅文件拆分） |
| `internal/grpcserver/server.go` | 148 | 无 `_test.go` | `RegisterServer` 调用注册到 grpc.Server；health check OK；reflection 注册 |
| `internal/svc/servicecontext.go` | 45 | 无 `_test.go` | `InitMultiModal` 注入顺序；missing dep → error |

**commit 链（10 个）**：每个文件一个 RED + 一个 GREEN。

### 3.3 `emotion-echo-analytics-svc`（补 1 个 handler 测试 + 未来业务实现 backlog）

| 文件 | LOC | 缺口 | 测试场景（最小集） |
|------|-----|------|-------------------|
| `internal/handler/health_handler.go` | 20 | 无 `_test.go` | svcCtx nil → panic 保护；DB up → JSON 健康；DB down → degraded JSON |

**承认**：`analytics-svc` 是脚手架状态，**本 backlog 仅补齐文件层**。业务端点（`/reports/*`、`/user-behavior/*`、`/mental-health/*`）的实施不在本 backlog 范围，需独立 TDD cycle（见 §六 不在本次范围）。

**commit 链（2 个）**：
```
test(analytics-svc): red — health_handler happy + degraded
feat(analytics-svc): health_handler trivial adjustment
```

### 3.4 `emotion-echo-shared/pkg/`（补 0 个文件 / 仅 sanity check）

`shared/pkg/` 已经是金标准（23 测试 / 28 源 = 82%）。本 backlog 不动 shared/pkg。

**例外**：`auth/` 与 `discovery/` 子目录为空（合规 vacuously）。本 backlog 不引入任何代码。

---

## 四、Python 后端部分（第 2 轮 session）

### 4.1 `Emotion-Echo-LLM/FER`

| 文件 | LOC | 缺口 | 测试场景（最小集） |
|------|-----|------|-------------------|
| `tests/unit/test_emotion_mapping.py` | 69 | **重构**：删除快照，从 server.py import | 重构后保留 happy/边界测试，但 mapping 字典从 `server.EMOTION_MAPPING` import |
| `tests/unit/test_analyze_route.py`（新增） | ~80 | 无此文件 | 用 `fastapi.testclient.TestClient` 测 `/analyze` 在 `backend=neutral-fallback` 路径（不需 fer 包），断言返回 `{emotion, confidence, scores, source}`；empty file → 400；non-image → 415 |
| `tests/unit/test_health_route.py`（新增） | ~30 | 无此文件 | `GET /health` 返回 `{status, model_loaded, backend}`；backend 字段值在合法集内 |
| `tests/unit/test_logging_setup.py`（新增） | ~40 | 无此文件 | `setup_logging()` 设置 `JsonFormatter`；log record 含 `timestamp/level/logger/message` |

**commit 链（4 个）**：
```
refactor(FER): export EMOTION_MAPPING and EMOTIONS constants
test(FER): refactor test_emotion_mapping to import (no snapshot)
test(FER): red — /analyze route happy + empty file
feat(FER): analyze route validates empty file early
```

### 4.2 `Emotion-Echo-LLM/sensevoice-small`

| 文件 | LOC | 缺口 | 测试场景（最小集） |
|------|-----|------|-------------------|
| `tests/unit/test_emotion_parser.py`（新增） | ~120 | 0 测试 | `EMOTION_TOKEN_RE.findall` 解析 7 种 token；`extract_emotion_from_raw` happy + 多 token 取第一个；未知 token → neutral；`extract_text_only` 去除所有 `<|...|>` 标记 |
| `tests/unit/test_health_route.py`（新增） | ~30 | 0 测试 | `GET /health` schema；`model_loaded: bool` 字段 |
| `tests/unit/test_logging_setup.py`（新增） | ~40 | 0 测试 | 同 FER |

**commit 链（3 个）**：
```
test(sensevoice): red — emotion token regex + extract_emotion_from_raw
feat(sensevoice): export extract_emotion_from_raw for testability
test(sensevoice): red — /health schema + logging JSON formatter
feat(sensevoice): logging_setup supports JSON
```

### 4.3 `Emotion-Echo-LLM/XTTS`

| 文件 | LOC | 缺口 | 测试场景（最小集） |
|------|-----|------|-------------------|
| `tests/unit/test_request_validation.py`（新增） | ~80 | 0 测试 | `TTSRequest` 字段校验：空 text → 422；language 不在合法集 → 422；超长 text → 截断到 200；speed 越界 → clamp 到 [0.5, 2.0] |
| `tests/unit/test_pcm_chunk_shape.py`（新增） | ~100 | 0 测试 | `float32 → int16` 转换：`(chunk * 32767).astype(np.int16)`；`np.clip(-1, 1)` 边界；`volume=0` → 静音；`volume=2.0` → 不超 int16 上界 |
| `tests/unit/test_logging_setup.py`（新增） | ~40 | 0 测试 | 同 FER |

**commit 链（3 个）**：
```
test(xtts): red — TTSRequest pydantic validation
feat(xtts): TTSRequest fields + validators
test(xtts): red — PCM chunk shaping (volume, clip, dtype)
feat(xtts): export pcm_chunk_shape helper for testability
```

### 4.4 `emotion-llm-service`

| 文件 | LOC | 缺口 | 测试场景（最小集） |
|------|-----|------|-------------------|
| `tests/unit/test_analyze_pure.py` | 131 | **重构**：删除 `EMOTION_KEYWORDS` / `SENTIMENT_WORDS` 快照，从 main.py import | 重构后保留全部原有用例 |
| `tests/unit/test_grpc_server.py`（新增） | ~200 | 无此文件 | 4 个 interceptor（auth / logging / tracing / recovery）的 happy + 异常路径；2 个 RPC handler（`Analyze` / `Health`）的 happy + 边界（空 text / None / emoji / 4096 chars） |
| `tests/unit/test_http_routes.py`（新增） | ~80 | 无此文件 | `TestClient` 测 `/health` `/metrics` `/analyze`；超长 text → 200（带截断日志）；空 text → 400 |

**commit 链（4 个）**：
```
refactor(llm-service): export EMOTION_KEYWORDS / SENTIMENT_WORDS
test(llm-service): refactor test_analyze_pure to import (no snapshot)
test(llm-service): red — grpc_server interceptors + Analyze handler
feat(llm-service): grpc_server health endpoint
```

---

## 五、滚动执行表（95 个文件总览）

### 5.1 Go 后端（~50 文件 / ~2400 LOC）

| # | 文件 | 服务 | 缺口类型 | LOC | 优先级 | 依赖 | Session |
|---|------|------|---------|-----|--------|------|---------|
| 1 | `internal/logic/sendmessagelogic.go` | chat-svc | pure-logic | 127 | P0 | kafka publisher fake | S1-T1 |
| 2 | `internal/logic/listmessageslogic.go` | chat-svc | pure-logic | 75 | P0 | repo fake | S1-T1 |
| 3 | `internal/handler/chat_handler.go` (扩) | chat-svc | web-api | 88 | P1 | 已有 | S1-T1 |
| 4 | `internal/logic/aihealthlogic.go` | ai-svc | pure-logic | 102 | P0 | bootstrap fakes | S1-T2 |
| 5 | `internal/logic/getemotionbymessagelogic.go` | ai-svc | pure-logic | 52 | P0 | InMemoryEmotionRepo | S1-T2 |
| 6 | `internal/logic/listemotionbyconversationlogic.go` | ai-svc | pure-logic | 55 | P0 | InMemoryEmotionRepo | S1-T2 |
| 7 | `internal/logic/multimodalanalyzelogic.go` | ai-svc | pure-logic | 81 | P1 | ai client fakes | S1-T2 |
| 8 | `internal/logic/synthesizespeechlogic.go` | ai-svc | pure-logic | 77 | P1 | XTTS client fake | S1-T2 |
| 9 | `internal/handler/emotion_handler.go` | ai-svc | web-api | 61 | P1 | svcCtx fake | S1-T2 |
| 10 | `internal/handler/multimodal_handler.go` | ai-svc | web-api | 123 | P1 | multipart parser | S1-T2 |
| 11 | `internal/analyzer/auth_wrapped.go` | ai-svc | pure-logic | 36 | P2 | Analyzer interface fake | S1-T2 |
| 12 | `internal/aiclient/ai_client_test.go` (拆) | ai-svc | refactor | 227 | P2 | 已有 | S1-T2 |
| 13 | `internal/grpcserver/server.go` | ai-svc | boundary | 148 | P1 | grpc test bufconn | S1-T2 |
| 14 | `internal/svc/servicecontext.go` | ai-svc | boundary | 45 | P2 | n/a | S1-T2 |
| 15 | `internal/handler/health_handler.go` | analytics-svc | web-api | 20 | P0 | svcCtx fake | S1-T3 |

### 5.2 Python 后端（~14 文件 / ~900 LOC）

| # | 文件 | 服务 | 缺口类型 | LOC | 优先级 | 依赖 | Session |
|---|------|------|---------|-----|--------|------|---------|
| 16 | `tests/unit/test_emotion_mapping.py` (重构) | FER | refactor | 69 | P0 | import from server.py | S2-T1 |
| 17 | `tests/unit/test_analyze_route.py` (新增) | FER | web-api | ~80 | P0 | fastapi TestClient | S2-T1 |
| 18 | `tests/unit/test_health_route.py` (新增) | FER | web-api | ~30 | P1 | fastapi TestClient | S2-T1 |
| 19 | `tests/unit/test_logging_setup.py` (新增) | FER | pure-logic | ~40 | P2 | caplog | S2-T1 |
| 20 | `tests/unit/test_emotion_parser.py` (新增) | sensevoice | pure-logic | ~120 | P0 | 无 | S2-T2 |
| 21 | `tests/unit/test_health_route.py` (新增) | sensevoice | web-api | ~30 | P1 | fastapi TestClient | S2-T2 |
| 22 | `tests/unit/test_logging_setup.py` (新增) | sensevoice | pure-logic | ~40 | P2 | caplog | S2-T2 |
| 23 | `tests/unit/test_request_validation.py` (新增) | XTTS | pure-logic | ~80 | P0 | pydantic | S2-T3 |
| 24 | `tests/unit/test_pcm_chunk_shape.py` (新增) | XTTS | pure-logic | ~100 | P1 | numpy | S2-T3 |
| 25 | `tests/unit/test_logging_setup.py` (新增) | XTTS | pure-logic | ~40 | P2 | caplog | S2-T3 |
| 26 | `tests/unit/test_analyze_pure.py` (重构) | llm-service | refactor | 131 | P0 | import from main.py | S2-T4 |
| 27 | `tests/unit/test_grpc_server.py` (新增) | llm-service | boundary | ~200 | P1 | grpc protobuf mock | S2-T4 |
| 28 | `tests/unit/test_http_routes.py` (新增) | llm-service | web-api | ~80 | P1 | fastapi TestClient | S2-T4 |

### 5.3 Frontend（~10 文件 / ~500 LOC，本轮 session 内）

| # | 文件 | 服务 | 缺口类型 | LOC | 优先级 | 依赖 | Session |
|---|------|------|---------|-----|--------|------|---------|
| 29 | `app/utils/Regs.ts` | Web | pure-logic | 122 | P0 | 无 | S3-T1 |
| 30 | `app/utils/function.ts` | Web | pure-logic | 73 | P0 | 无 | S3-T1 |
| 31 | `app/utils/date.ts` | Web | pure-logic | 47 | P0 | 无 | S3-T1 |
| 32 | `app/utils/url.ts` | Web | pure-logic | 28 | P1 | 无 | S3-T1 |
| 33 | `app/utils/file.ts` | Web | pure-logic | 43 | P1 | 无 | S3-T1 |
| 34 | `app/utils/index.ts` | Web | pure-logic | 20 | P2 | 无 | S3-T1 |
| 35 | `app/utils/db.ts` | Web | boundary | 13 | P2 | fake-indexeddb | S3-T1 |
| 36 | `app/composables/useConversationGrouper.test.ts` (扩) | Web | pure-logic | n/a | P1 | 已有 | S3-T1 |
| 37 | `app/composables/useNotify.test.ts` (扩) | Web | pure-logic | n/a | P2 | 已有 | S3-T1 |
| 38 | `app/stores/conversation.test.ts` (扩) | Web | pure-logic | n/a | P2 | 已有 | S3-T1 |

### 5.4 Defer / 不在本次 backlog（~57 文件）

**模型加载类（必须真模型才能测，本 backlog 不写）**：
- FER `analyze_emotion` happy-path 真实 fer / OpenCV DNN（需 `emotion_net.caffemodel`）
- sensevoice `_load_model_sync` / `_infer_sync`（需 funasr + ~230MB 模型）
- XTTS `load_xtts_model` / `model.synthesize` / `model.inference_stream`（需 ~1.8GB 权重）

**前端浏览器 API 类**：
- `useApi.ts`（435）、`useAIStream.ts`（143）、`useAIStreamHandler.ts`（202）、`useConversationSender.ts`（251）、`useDigitalHumanTTS.ts`（70）、`useFaceEmotion.ts`（188）、`useFileUpload.ts`（158）、`usePrompt.ts`（39）、`useTTSManager.ts`（96）、`useTTSPlayer.ts`（327）、`useVoiceRecorder.ts`（211）、`forgetPwdState.ts`（57）、`verificationCodeCountDown.ts`（43）— 需 network / MediaStream / AudioContext
- `digital-human/DigitalHuman.vue`（641）— WebGL/Three.js
- `face/FaceCamera.vue`（252）— MediaStream + canvas
- `voice/VoiceRecorder.vue`（220）— MediaRecorder
- `app/stores/{user.ts, message.ts, digitalHuman.ts}`（620 LOC）— Pinia + 跨 composable 依赖

**Vue 组件类**（happy-dom 可测但工作量较大，本轮 session 仅覆盖已有 utils）：
- `ChatFile.vue`（210）、`report/chartsCard.vue`（357）、`charts/{BaseChart,RadarChart,barChart,lineChart,pieChart}.vue`（~250 总）

**DoD**（每轮 session 收尾时验证）：
1. `go test ./...` 绿且 ≤ 5s
2. `go vet ./...` 绿
3. `pytest tests/unit/` 绿且 ≤ 30s（Python 那一轮 session）
4. `vitest run` 绿且 ≤ 30s（前端那一轮 session）
5. `bash scripts/smoke_apps_26p.sh` 9/9 绿（仅当改动影响 service 行为时）
6. 新增 `_test.go` / `test_*.py` / `*.test.ts` 数量 = 滚动执行表 §五 行数 − 已存在数
7. **0 个 `t.Skip` / `pytest.skip` 命中**（用 `grep -r 't\.Skip\|pytest\.skip' tests/` 验证）

---

## 六、不在本次范围（显式 defer）

| 项目 | 原因 | 后续阶段 |
|------|------|---------|
| `analytics-svc` 业务端点实现（reports / user-behavior / mental-health） | 本 backlog 仅补齐 `health_handler_test.go`；业务实现需独立 TDD cycle，需先解决跨 schema 查询约束（`禁止跨库 JOIN` → 走 RPC） | 后续独立 PR |
| `legacy/emotion-echo-gin`（121 实现 / 1 测试） | 已 archived，弃用；AGENTS.md 2026-07-15 才生效，旧代码不溯及 | 不补 |
| 数字人 / 相机 / 语音前端测试 | MediaStream / WebGL / AudioContext 需特殊环境 | 后续 session + happy-dom/playwright 改造 |
| `useApi.ts`（前端最大 composable，含 401 刷新 / 429 重试 / 请求去重） | 网络依赖重，需 mock fetch 链；工作量约 1 session | 后续 session |
| `emotion-echo-shared/pkg/auth/` 与 `pkg/discovery/`（空目录） | 当前合规 vacuously | 不动 |
| Stage 30 Web BFF 实施 | 用户目标未包含 | Stage 30 独立 session |
| Let's Encrypt ACME ClusterIssuer | dev 用 self-signed 足够 | Stage 29-E |
| CI/CD 接入 | AGENTS 既有约束 | 后续 PR |

---

## 七、本次 session 的预期 commit

```
docs(stage-26-T): test backlog — multi-session TDD plan for 38 priority files
```

仅 1 个 commit。本次 session **不写任何测试代码**，仅落地 backlog。

**预提交自检**（commit 前必跑）：
- `grep -c '^## ' docs/stage-26-T-test-backlog.md` ≥ 6（一级章节数：目标 / TDD 原则 / Go / Python / 滚动表 / 不在范围）
- `wc -l docs/stage-26-T-test-backlog.md` ≥ 200
- `git diff --stat` 仅显示 1 个新文件

---

## 八、参考

- `AGENTS.md` §0.1 ALL CODE IS TDD / §1.1 Go 测试栈 / §四 禁止事项
- `docs/stage-26-M-coverage.md` — Stage 26-M coverage baseline（建立了内部测试 commit 模式）
- `docs/stage-26-K-integration.md` — Stage 26-K Postgres 集成测试 build tag 模式
- `docs/stage-26-L-smoke.md` — Stage 26-L smoke 脚本模式
- `emotion-echo-ai-svc/internal/analyzer/analyzer_test.go` — **金标准**：表驱动 + 子测试 + stubAnalyzer
- `emotion-echo-shared/pkg/grpcinterceptor/*_test.go` — **金标准**：sibling test + 边界 + error path
- `emotion-llm-service/tests/unit/test_analyze_pure.py`（待重构）— 当前 snapshot-copy 反例
- `Emotion-Echo-LLM/FER/tests/unit/test_emotion_mapping.py`（待重构）— 当前 snapshot-copy 反例

---

> 最后更新：2026-08-29 by 当前协作 Agent
> 适用版本：本 backlog 生效后的所有 PR（含跨多 session 滚动推进）