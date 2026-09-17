---
status: active
priority: high
created: 2026-09-17
last-refresh: 2026-09-17
type: e2e-pretest-findings
---

# 建档预探查全景（2026-09-17）

> E2E roadmap 建档时做的三块代码级探察：① 人格测试与 AI 提示词链路 ② 数字人 ③ 横切模块。
> 所有结论均带文件路径与行号证据，可直接复核。发现已登记 [discovered-unresolved.md](../discovered-unresolved.md)。

---

## 一、人格测试 → 心理预测 → AI 提示词定制

### 结论：这条链路**完全不存在**

现在存在的是一个**症状自评量表**（symptom scale）的窄链路，且第四、五环断裂或从未实现。更根本的是：**量表题目本身就不是人格测试**，评分产出也不是人格维度/心理预测。**测评结果从未进入任何 AI 请求路径。**

### 逐环现状

| 环节 | 现状 | 证据 | 断点 |
|------|------|------|------|
| ① 量表题目设计 | 症状自评量表（PHQ-9/GAD-7/PSQI；legacy 为 SDS/SAS 20 题） | `deploy/db/02-create-tables-in-schemas.sql:162-174`、`assessment-svc/internal/scoring/scorer.go:32-222` | **方向性偏差**：无任何人格/特质维度题；**现工程零种子数据**（表空） |
| ② 评分产出 | `TotalScore + RiskLevel(none/mild/moderate/severe/extreme) + 逐题 Factors` | `scorer.go:20-30, 45-91, 269-280` | **只到症状分级**：无维度/特质/人格标签、无预测、无建议文案 |
| ③ 结果存储 | `answers/factor_scores/total_score/risk_level/duration` 落 `survey_results` | `assessment-svc/internal/logic/submitsurveylogic.go:65-93` | 存储可写；但**除 assessment-svc 自身外无人读** |
| ④ 结果展示 | 弹窗字段与后端不符（`level`/`suggestion` 恒空）；提交体数组 vs map；列表读 `list` vs 后端 `items`；"查看结果"路由单复数错位自带 TODO | `web/app/pages/question/[id].vue:72-86,132-137`、`web/app/types/api.ts:239-247,292-296`、`bff/internal/downstream/assessment.go:46-60`、`bff/internal/handler/survey_handler.go:40,57,81` | **断在 ④**：UI 层就跑不通（提交 400 / 列表报错 / 结果空字段） |
| ⑤ AI prompt 注入 | 完全不存在。BFF 写死静态 system prompt；llm-service 纯透传；proto 无画像字段 | `bff/internal/handler/ai_stream_handler.go:226,284`（`"你是一个温柔、共情的情绪疏导陪伴者..."`）、`llm-service/chat_completion.py:94-118`、`proto/emotion_llm.proto:61-72`、`bff/internal/downstream/llm_grpc.go:148-153` | **断在 ⑤**：chat-svc/user-svc/shared/proto 中 `personality\|人格\|用户画像\|MBTI\|profile` **零命中** |

### 关键佐证细节

1. **量表种子数据不存在**：`deploy/db/` 只有 `01-create-schemas.sql` ~ `05-drop-user-oauth.sql`，无任何 INSERT 量表。`docs/stages/stage-8b-assessment-surveys.md:69` 声称有 `deploy/db/seed-surveys.sql`（PHQ-9/GAD-7/PSQI），该文件**在 git 全历史中都不存在**（`git log --all --name-only` / `git log -S` 均只命中文档本身）。唯一真实种子在**已废弃单体**：`legacy/emotion-echo-gin/migrations/002_seed_surveys.up.sql`（SDS/SAS 各 20 题症状频率题）。

2. **意图风格注入实际不生效**：`llm-service/intent.py` 有按意图（emotional_support 等）注入风格的 `STYLE_INSTRUCTIONS`，但只在 `request.with_intent=True` 时生效（`grpc_server.py:276-285`）；**BFF 聊天主链路构造 `ChatCompletionRequest` 时从不设 `WithIntent`**（`llm_grpc.go:148-153`）→ 真实聊天 SSE 路径里永不触发。

3. **legacy 设计过挂点但是空壳**：`legacy/emotion-echo-gin/internal/service/ai_emotion.go:94-100` 的 `BuildSurveyContext()` 函数体直接 `return ""`，从未实现；配套的 `PsychProfile` 类型与 `GetLatestPsychProfile()` 查询均为死代码（全仓无调用方）。

4. **analytics 的 mental-health 报表与人格测试毫无关系**：数据源 `emotion_echo_assessment.assessment_v` 刻意排除了 `survey_results`（`analytics-svc/migrations/a001_create_views.sql:37-50`，注释明写"risk_level 在 survey_results 上（本表无此列）"）；`assessment_type` 只有 `daily|weekly|comprehensive`，是情绪日报/周报而非人格类型。

5. **前端无人格结果页**：`emotion-echo-web/app/pages/` 无 personality/画像页；前端从不调用 `/mental-health/*`；`apiRoutes.ts:63-65` 无"我的结果列表/结果详情"路由。

> **即使把 ④ 修好，注入 AI 的也只会是"PHQ-9 总分 15 / severe"这类症状分数**——"心理预测 → 提示词定制"在数据模型（`scoring.Result`、`SurveyResult`、`surveys.questions`）里就没有承载字段。要做，需先做量表设计决策（详见 roadmap 改造讨论项）。

---

## 二、数字人 + TTS

### 结论：口型同步**没实现**（是随机假口型）；动作/表情**部分实现**

| 问题 | 答案 |
|------|------|
| 口型/动作跟随 XTTS 音频？ | ❌ **口型没有**——随机轮播假口型，与音频内容零对齐。真正的 phoneme 同步是 Phase 3 预留项，映射表是死代码。<br>✅ **动作/表情有**——AI 情绪标签（`finish` 事件）驱动 VRM 表情（happy/sad/angry），5 秒后复位；身体 idle 动画（点头/眨眼/呼吸）是程序化的，与音频无关。 |
| 当前形态？ | **Three.js + @pixiv/three-vrm 渲染的 3D VRM 模型**（26MB `digital-human.vrm`），圆形悬浮窗可拖拽。不是 SVG、不是视频。 |
| XTTS 延迟问题有记录吗？ | ✅ 延迟有记录（legacy plans 风险表）；❌ "流式播放断点"**无文档记录**，但代码有结构性根因。 |

### 关键实现位置

- **3D 组件**：`web/app/components/digital-human/DigitalHuman.vue`（L66-68 three/vrm import；L200-236 程序化 idle 动画；L271-289 `setEmotion` 表情；L248-258 口型写入 BlendShape 权重 0.8）
- **假口型**：`web/app/composables/useTTSPlayer.ts` L91-103 `startRandomLipAnimation`（每 150ms 轮播 `['aa','ee','ih','oh','ou']`）；L243-247 首 chunk 启动；L267 流结束停止
- **死代码**：同文件 L38-75 `VOWEL_TO_LIP` / `CONSONANT_CLOSE` 映射表——全文件无引用（grep 全 app 仅定义行命中）
- **音频播放**：L157-278 `playStream`（POST `/tts/stream`，chunk 直接 feed 进 `PcmPlayer`，Int16/24kHz/flushTime 100ms）
- **Store**：`web/app/stores/digitalHuman.ts`（visible/position/voiceEnabled/currentLipShape/volume）

### TTS 链路（当前微服务架构）

```
前端 useTTSPlayer.playStream (POST /api/v1/tts/stream, Bearer JWT)
  → BFF tts_handler.go stream() → 直连 XTTS（不经 ai-svc）
  → XTTS FastAPI :8003 POST /tts_stream → StreamingResponse 逐 chunk
  → BFF io.Copy 逐块转发（X-Accel-Buffering: no）
  → 前端 PcmPlayer feed 播放
```

- **BFF**：`bff/internal/handler/tts_handler.go` L36/L53-73（`/api/v1/tts/stream` 流式）；L39-51 另有非流式 `/tts/synthesize` 走 ai-svc
- **BFF→XTTS**：`bff/internal/downstream/xtts.go` L70-97 —— **HTTP 直连（非 gRPC）**，L5-6 注明"XTTS :8003，无鉴权，BFF 直连不经 ai-svc"
- **XTTS**：`emotion-echo-models/XTTS/server.py`
  - L265-280 `/tts_stream`：`inference_stream(..., stream_chunk_size=20, enable_text_splitting=True)`（实际 yield 裸 PCM，media_type 标 `audio/wav` 有误但能跑通）
  - L153-204 `/tts` 非流式：**L165 `clipped = req.text[:100]` 截断到 100 字**（历史延迟妥协的证据）
  - **L283-341 `/tts_with_phonemes`：带字符级时间戳——专为口型同步设计，但前端从未调用**
- **ai-svc**：只有非流式 `synthesizespeechlogic.go` L39-77 → `aiclient/xtts.go` L69-122（base64 WAV 一次性）。**ai-svc 无 TTS 流式端点**

### 延迟 / 断点根因分析

**延迟有记录**：`docs/legacy-plans/landed/digital-human-phase1-plan.md` L287-290 风险表列了"TTS 服务延迟 → 语音播放延迟 → 添加加载状态提示"；`xtts-lipsync-phase3-plan.md` L189-192 列"XTTS 延迟 → 预加载、分批调用"。

**断点未记录但根因明确**（三段叠加，段与段之间必然出现"上一次推理结束 + 下一次冷启动"的间隙）：
1. `useConversationSender.ts` L143-149：AI 文本按 **500ms debounce** 聚合成段
2. `useTTSPlayer.ts` L172：**每段新文本调用 `playStream` 都先 `stop()`**（abort 上一个 HTTP 流 + destroy 上一个 PcmPlayer），再重新发全新 HTTP 请求
3. `XTTS/server.py` L223-231：每段文本都是一次新的 `inference_stream` 推理（`stream_chunk_size=20`）

**Phase 2/3 预留项从未完成**：`digital-human-phase1.md` L27-29/L228-233/L475-478 的"口型动画同步、情绪动作预设"未勾选；`xtts-lipsync-phase3-plan.md` L34-45 的设计意图（30-50 字/句号触发分批 TTS + 字符级时间戳驱动口型）与当前随机口型实现不符。

---

## 三、横切模块现状

### 3.1 缓存层

| 项 | 现状 | 关键文件 |
|----|------|---------|
| Redis | **确认预留未使用**：容器在跑但无任何非 legacy Go 服务实例化 Redis 客户端；`limiter.go` 的 `RedisLimiterBackend: TODO`；BFF 登录锁定为 in-memory 单实例 | `deploy/docker-compose.infra.yml:45-60`、`shared/pkg/middleware/limiter.go:137-140`、`bff/internal/handler/auth_handler.go:14,66` |
| 本地缓存 | `ai-svc/internal/fusion/lru.go` **是**真正的 LRU（container/list + map，cap=1024/TTL=4min），但仅用于 FusionWorker 的 messageID 去重（省 LLM 配额），非通用缓存 | `ai-svc/internal/fusion/lru.go`、`ai-svc/main.go:496-501` |
| 前端本地存储 | 认证走 HttpOnly cookie + Pinia（**token 不落 localStorage**）；localStorage 仅用于忘记密码流程的步骤状态 | `web/app/composables/useApi.ts`、`forgetPwdState.ts` |

**缺口**：无跨模块可复用缓存抽象（LRU 是 ai-svc 私有）；Redis 空转。

### 3.2 数据库层

| 项 | 现状 | 关键文件 |
|----|------|---------|
| Schema | PG 15 单库 `emotion_echo`，5 业务 schema（user/chat/ai/assessment/analytics） | `deploy/db/01-create-schemas.sql` |
| 迁移 | **两套机制并存**：initdb.d（仅空卷首次 01~05）+ db-migrate 一次性容器（每次 compose up 跑 `migrate.sh`，glob 发现 `*/migrations/`） | `deploy/db/migrate.sh`、`docker-compose.apps.yml:47-66` |
| 迁移文件 | chat c001-c007、ai i001-i009、analytics a001-a009（含 **a008 按月 RANGE 分区**） | 各 svc `migrations/` |
| 连接池 | `ApplyPoolEnv`（PG_MAX_CONNS/PG_MIN_IDLE_CONNS/PG_MAX_LIFETIME_SECONDS，默认 10/5/1h），5 svc 均已接入 | `shared/pkg/dbconnect/pool.go` |
| 软删除 | users/ai 域有，**chat 的 deleteconversation 是物理删** | `deploy/db/02-*.sql`、`ai-svc/migrations/i007,i008` |

**缺口**：无 `schema_migrations` 版本表（靠幂等重放，无法回答"某环境跑过哪些迁移"）；`deploy/db/README.md` 已过时。

### 3.3 日志体系

| 项 | 现状 | 关键文件 |
|----|------|---------|
| 结构化日志 | `shared/pkg/logging`：stdlib slog → JSON stdout，自动注入 `svc`/`trace_id`/`action`，env `LOG_FORMAT`/`LOG_LEVEL` | `shared/pkg/logging/logging.go` |
| traceId 注入 | HTTP 侧有（优先级：X-Trace-Id header → SkyWalking span）；**gRPC 侧没有** | `shared/pkg/middleware/gin_skywalking.go:17-21`、`grpcinterceptor/tracing.go` |
| 采集 | Loki 2.9.4 + Promtail 2.9.4（两 job：apisix / services） | `deploy/loki/loki-config.yaml`、`promtail-config.yaml` |

**缺口（大）**：Promtail 的 services job 指向 `/var/log/services/*.log`，但 **compose 里没有任何 volume 把 Go svc 日志挂到该路径**，也没用 docker-sd 采 stdout → **实际进 Loki 的只有 APISIX access log，6 个 Go 服务的结构化日志没有采集入口**。

### 3.4 监控告警

| 项 | 现状 |
|----|------|
| Prometheus | v2.51.2，抓取 6 业务 svc /metrics + APISIX + OAP + kafka-exporter |
| 告警规则 | 3 份：kafka-lag（warning）、outbox-dead（critical）、kafka-dlq（critical） |
| Alertmanager | v0.27.0，**只接 dev-ui 空 receiver，无外部通知渠道** |
| Grafana | 两块 dashboard：emotion-echo-overview、kafka-consumer-lag |

**缺口**：无通知渠道；无 Postgres/业务 SLI 告警；dashboard 无 Loki 日志面板。

### 3.5 健康检查与服务发现

| 项 | 现状 |
|----|------|
| gRPC health | `healthcheck/v1` 标准实现，支持多 service 状态 + Shutdown/Resume 优雅下线 |
| HTTP /health | 各 svc 均有；BFF 为聚合下游探测 |
| compose healthcheck | infra 与 apps 全覆盖，服务间 `depends_on: service_healthy` |
| Nacos | 5 Go svc 均有 `nacos_boot.go`（注册中心 + 配置中心 `GetConfig("{svc}.ops.yaml")` + 热更新 `NACOS_HOT_RELOAD`） |

**缺口**：基本无（各 svc /health 是否查 DB 依赖度不一，无统一 readiness 语义）。

### 3.6 其他

| 模块 | 现状 | 缺口 |
|------|------|------|
| 限流 | `shared/pkg/middleware/limiter.go` 内存令牌桶（per-user）；APISIX limit-count | 多实例限流不可用（Redis 后端 TODO） |
| CORS | 统一由 APISIX cors 插件处理，BFF 已移除自有中间件 | 无 |
| MinIO | 头像上传/下载专用（`avatars` bucket，minio-init 建桶 + 匿名下载） | 仅 avatars 一个 bucket |
| 消息重试/DLQ | 链路成型：outbox relay（MaxAttempts 100 → dead + cleanup）；consumer 重试 N 次发 `chat-events-dlq`；3 条 Prometheus 告警 | **DLQ 无自动回放工具**（规则注释里是手工 psql UPDATE） |

---

## 四、补充核实：user 界面图表现状

> 触发：用户 2026-09-17 提到"user 界面之前有图表记录用户对话频率等等，不确定现在有没有"。核实结论：**有，共 3 个，且都由真实 API 驱动。**

### 位置与构成

`emotion-echo-web/app/pages/chat/user/index.vue`（275 行），图表由 `chartData` computed 生成（L190-232），渲染 pie/line/bar 三种（L21-24）：

| 图表 | 类型 | 数据源 API | 字段 |
|------|------|-----------|------|
| 昼夜行为分布 | pie | `API_ROUTES.userBehaviorDayNight` | `periods[]` |
| 消息频率趋势 | line | `API_ROUTES.userBehaviorFrequency` | `dates[]` / `messageCount[]` |
| 会话深度统计 | bar | `API_ROUTES.userBehaviorDepth` | `avgSessionRounds` / `maxConsecutiveDays` / `totalConversations` / `totalMessages` / `avgMessagesPerDay` |

数据加载：L174-181 `Promise.all` 并发请求三个端点。

### 关键约束（对排期的影响）

1. **图表是条件渲染**：每个图表都有 `?.length > 0` / 非空判断（L194/L206/L216）——**数据为空则整块不渲染，且无空态提示**（用户看到的是页面缺一块，不是"暂无数据"）。
2. **依赖 analytics 事件链**：三个端点都来自 analytics-svc 的 user-behavior 域，其数据源头是聊天产生的行为事件（outbox → Kafka → analytics 消费入库）。这意味着 **user 页面图表能否有数据，取决于 E2E-18（消息链）与 E2E-23（数据契约）是否健康**——与报表页 `chartData=[]` 历史问题是同一类风险（见 E2E-F-10）。
3. **无人格/测评图表**：现无任何测评结果可视化（与 user 页现有 3 个图表是不同维度）。

### 与 D-02 决议的关系

用户已决议"在 user 界面添加与测评相关的图表"（见 [decisions.md](../decisions.md) D-02）。落地时应在现有 `chartData` 机制上扩展（同一 computed 追加图表项），并注意上述"条件渲染 + 无空态"的既有缺陷。

---

## 五、补充核实：目录与数据库字段清理候选

> 触发：用户 2026-09-17 要求把"项目目录多余文件清理"与"数据库无用字段删除"加入排期。以下为核实后的具体候选，供 E2E-02 / E2E-03 阶段卡执行。

### 5.1 目录清理候选（E2E-02）

| 对象 | 现状 | git 跟踪 | 处置建议 |
|------|------|---------|---------|
| `.mimosa/`（根、`deploy/apisix/`、`emotion-echo-web/` 三处） | hook 运行时状态（history/hook-state/hook-status/reports/finding-ledger） | **未被 gitignore** → 持续出现在 `git status` 的 `??` 列表 | 加入 `.gitignore` |
| `emotion-echo-web;D` | **空目录（0 字节）**，shell 误建残留（`mkdir emotion-echo-web;D` 未转义分号） | 已 gitignore | 删除 |
| `docker-images-before.txt` | 一次性镜像快照产物（2026-09-09） | 已 gitignore | 删除 |
| `gui-test-screenshots/` | 测试证据，**25 个文件已被 git 跟踪** | **已跟踪** | 归档到 `docs/evidence/`（项目已有该目录）或移出跟踪 |
| `tmp/` | 临时目录 | 已 gitignore | 确认可清空 |
| 根 `node_modules/` | 根目录无 `package.json`（前端目录才有） | 已 gitignore | 确认是否需要 |
| `.zcode/plans/` | 会话计划 md（`plan-sess_*.md`） | 已 gitignore | 保留（工具产物） |

### 5.2 数据库死字段候选（E2E-03）

核实方法：对 `emotion_echo_user.users` 的每列，grep 全部非测试 Go 代码的读写点。

| 字段 | 读取点 | 写入点 | 结论 |
|------|--------|--------|------|
| `email VARCHAR(128)` | ❌ 无（仅 `model/user.go:15` tag 声明） | ❌ 无 | **死字段**，可删 |
| `phone VARCHAR(20)` | ✅ 仅 API 响应回显（`getmelogic.go:67`、`getuserbyidlogic.go:42`、`user_server.go:60`、`authlogic.go:113,136`） | ❌ **无任何写入点** → 恒为 NULL | **死字段**，可删 |
| `status SMALLINT` | ❌ 无（grep 命中的都是 gRPC `status` 包） | ❌ 无（仅 `model/user.go:21` `default:1`） | 无使用则删（可能有软禁用意图，需确认） |
| `gender` | ✅ `updateprofilelogic.go:54` 校验 | ✅ `UpdateProfile` | 保留（在用） |
| `birthday` | ✅ `UpdateProfile` | ✅ `UpdateProfile` | 保留（在用） |
| `nickname` | ✅ `updateprofilelogic.go:49` 校验 | ✅ `UpdateProfile` | 保留（在用） |
| `avatar_url` | ✅ | ✅ `user_repository.go:211` | 保留（在用） |

**附带事实**：系统**无管理员/角色概念**——`users` 表无 `role` 列（全仓唯一的 `role` 是 messages 表的 user/assistant），也无 admin 页面或端点。故"管理员重置密码"类方案在本项目无现成载体。


