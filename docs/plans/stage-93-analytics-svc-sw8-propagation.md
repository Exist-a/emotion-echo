---
status: landed
landed: 2026-09-14
priority: high
owner: TBD
created: 2026-09-14
depends-on:
  - stage-92-kafka-sw8-propagation-2026-09-14.md
related-stages:
  - stage-44-observability-sprint-b.md
  - stage-50-e2e-validation.md
  - stage-86-outbox-dead-alert-2026-09-13.md
  - stage-93-analytics-svc-sw8-propagation-2026-09-14.md
related-plans:
  - observability-edge-gaps-from-code-review.md §A-extension
related-adrs: []
landed-stages:
  - stage-93-analytics-svc-sw8-propagation-2026-09-14.md (commit 2a065d0 PR-2 main.go wire + 镜像 v0.1.6 + Round 2 单测 13/13 全绿)
residuals:
  - 容器 e2e 实证 stage93_sw8_verify.py (留 Stage 94+ 沉淀,沿用 Stage 92 stage92_sw8_verify.py 模式)
  - SkyWalking OAP 9.x graphql queryDuration 时间格式 bug (OAP UI 跨进程 trace 可视化阻塞,Stage 92 §五残余沿用)
  - extractSw8Header 收敛到 shared/pkg/messaging (ai-svc + analytics-svc 各一份 ~10 行)
  - observability-edge-gaps §D (GinSkywalking 跳过路径配置化 P3 0.5h) / §F (consumer.go 拆分 P3 0.5h)
---

# Plan - Stage 93 · analytics-svc consumer sw8 透传

## 0. 上下文

### 0.1 来源

- 上游 plan：[observability-edge-gaps-from-code-review.md §A-extension](observability-edge-gaps-from-code-review.md)
- 上游 stage：[stage-92-kafka-sw8-propagation-2026-09-14.md](../stages/stage-92-kafka-sw8-propagation-2026-09-14.md)（chat-svc producer + ai-svc consumer 已落）
- roadmap §当前 open 清单：`🔄 A-extension. analytics-svc consumer sw8 透传 —— Stage 93 候选`

### 0.2 现状

chat-svc producer (Stage 92 PR-1) 把 sw8 header 写到 `chat-events` / `emotion-events` / `emotion-aggregated` 等 topic 的每条消息。ai-svc consumer (Stage 92 PR-2) 已通过 `Tracer.CreateEntrySpan` 重建父 trace。

**analytics-svc 仍是独立 trace tree**：消费 `chat-events` topic 写 user_behavior_events 表，但 `chatEventHandler.ConsumeClaim`（`emotion-echo-analytics-svc/internal/kafka/consumer.go:142-165`）没有任何 Tracer 集成，每条消息的写库操作是孤立 span——SkyWalking UI 上 chat-svc 发出的消息 → ai-svc + analytics-svc 是两棵 trace tree，跨进程 trace 仅 ai-svc 一侧闭环。

### 0.3 目标

analytics-svc consumer 端接入 `Tracer.CreateEntrySpan`，从 `msg.Headers["sw8"]` 重建父 trace，与 ai-svc consumer 完全对称——SkyWalking UI 上 chat-svc → ai-svc + analytics-svc 一棵完整 trace tree（traceID 复用）。

## 1. 工作量

| 项 | 工作量 |
|---|---|
| 调研（5 个文件 + ai-svc main.go 对照样板，已完成） | 0 |
| PR-1 RED+GREEN（chatEventHandler 加 tracer 字段 + WithTracer builder + extractSw8Header helper + ConsumeClaim 改造） | 1.5h |
| PR-2 RED+GREEN（main.go wire + 镜像 bump + defer log） | 0.5h |
| docker e2e 实证（kafka-python 抓 sw8 + 消息触发 + 跨进程 traceID 比对） | 1h |
| 文档（plan + 收口报告 + roadmap 刷新 + observability-edge-gaps landed 段） | 0.5h |
| **总计** | **3.5h**（半天以内） |

## 2. PR 拆分

### PR-1 · analytics-svc consumer 接入 Tracer

#### 🔴 RED 步骤

**文件**：`emotion-echo-analytics-svc/internal/kafka/consumer_test.go`

加 `TestChatEventHandler_ConsumeClaim_RestoresParentTraceFromSw8Header`：
- mockTracer 捕获 `CreateEntrySpan` 调用
- 构造 sarama `ConsumerMessage`，`Headers: [{Key: "sw8", Value: <base64-encoded sw8>}, {Key: "content-type", Value: "application/x-protobuf"}]`
- 断言：
  - `mockTracer.createEntrySpanCalls` 长度为 1
  - 调用参数 `operationName == "kafka-consume"`
  - 通过 extractor 闭包验证：调 `extractor("sw8")` 返 `sw8Header` 字符串；调 `extractor("sw8-correlation")` 返 `""`
  - span.Tag 被调用 4 次：`messaging.system=kafka` / `messaging.kafka.topic=<topic>` / `messaging.kafka.partition=<partition>` / `event.type=<type>`
- 测试必须先 **红**（因 `chatEventHandler` 当前无 tracer 字段）

#### 🟢 GREEN 步骤

**文件 1**：`emotion-echo-analytics-svc/internal/kafka/consumer.go`

改动点：
- 引用 `github.com/emotion-echo/shared/pkg/grpcinterceptor`（与 ai-svc consumer 对称）
- `chatEventHandler` 结构体加 `Tracer grpcinterceptor.Tracer` 字段
- `NewConsumer()` 构造 chatEventHandler 时 `tracer: nil`（保持 Stage 30-A Round 4 原行为，向后兼容）
- 新增 `WithTracer(tracer grpcinterceptor.Tracer) *Consumer` builder 方法（与 `WithDLQ`/`WithMaxRetries` 同模式，重建 chatEventHandler 时把 tracer 字段一并传入）
- `ConsumeClaim()` 加 if 守卫：当 `h.Tracer != nil` 时：
  - 调 `extractSw8Header(msg.Headers)` 抽 sw8 value
  - 构造 extractor 闭包（同 ai-svc `consumer.go:117-122`）
  - 调 `h.Tracer.CreateEntrySpan(sess.Context(), "kafka-consume", extractor)`
  - `defer span.EndSpan(nil)`
  - 4 个 messaging.* tag（同 ai-svc `consumer.go:129-132`）
- 加 `extractSw8Header(headers []*sarama.RecordHeader) string` helper（直接复制 ai-svc `consumer.go:208-218` 函数体）
  - 注释说明：与 ai-svc consumer 包对称；不放 shared 是为了避免 analytics-svc 引入 ai-svc 才有的 go2sky 传递依赖（实际两边都不直接依赖 go2sky，但函数语义一致便于未来收敛）

#### ♻️ REFACTOR 步骤

- 评估是否把 `extractSw8Header` 提到 `shared/pkg/messaging/`（ai-svc + analytics-svc 共用）—— **本 stage 不抽**，避免引入跨 stage 边界改动；记入 Stage 94+ 候选
- 顺手检查是否 §F "consumer.go 拆分" 也一并做 —— **本 stage 不做**，独立评估

### PR-2 · main.go wire + 镜像 bump

#### 🔴 RED 步骤

**文件**：`emotion-echo-analytics-svc/main.go`

- 测试 wire 路径：在 SkyWalking init 之后加 `kc.WithTracer(sharedgrpc.NewGo2SkyTracer(tracer))`（仅当 tracer 非 nil 时调）
- 镜像版本 bump：`deploy/docker-compose.apps.yml` analytics-svc 标签 `v0.1.5` → `v0.1.6`

#### 🟢 GREEN 步骤

- `main.go:186-208` Kafka consumer 构造链加 `WithTracer` 调用（仿 ai-svc `main.go:331` 模式）
- defer log：`[skywalking] analytics-svc sw8 propagation enabled`（对齐 chat-svc / ai-svc log 措辞）
- 镜像 rebuild：`scripts/build_dev_images.sh analytics-svc` → 推送本地 image tag

## 3. DoD

1. **单测全绿**：
   - `cd emotion-echo-analytics-svc && go test ./...` 全绿
   - 新增 RED+GREEN 单测覆盖 sw8 header 抽 + CreateEntrySpan 调用 + 4 个 messaging.* tag
   - 保留 Stage 30-A Round 4 原单测不变（向后兼容：tracer=nil 路径走原行为）

2. **docker e2e 实证**：
   - `emotion-llm-service/tests/e2e/stage93_analytics_sw8_verify.py`（仿 Stage 92 同模式）
   - 触发一次 message.created → chat-svc:v0.1.10 producer 发到 `chat-events`
   - kafka-python 消费 `chat-events` topic，抓 sw8 header → 验证 chat-svc producer 写入 sw8（含 sample=1 + traceID + parent service=emotion-echo-chat-svc）
   - ai-svc:v0.1.6 + analytics-svc:v0.1.6 同时消费同一条消息 → 容器日志比对 traceID（两个 svc 应在同一 trace 下）
   - OAP UI 跨进程 trace 可视化仍受 9.x graphql queryDuration bug 阻塞（Stage 92 §五残余）—— 本 stage 不修，沿用 header + 日志 msgID 对齐实证

3. **§2.4 数据契约 baseline 未坏**：
   - §契约 1+2+3 通过（analytics-svc user_behavior_events 行数 / event_type 细分 / analytics_reader 视图权限）—— Stage 86 analytics 001 教训：本 stage 不动 DB schema，应无回归
   - `go test ./...`（chat-svc / ai-svc / analytics-svc / shared / web-bff）全绿

4. **文档同步**：
   - `docs/stages/stage-93-analytics-svc-sw8-propagation-2026-09-14.md` 收口报告
   - `docs/architecture/roadmap.md` §当前 open 清单：A-extension 状态从 `🔄` 改为 `✅ Stage 93 landed`
   - `docs/plans/observability-edge-gaps-from-code-review.md` `landed-parts` 段：§A-extension 加一行 "✅ Stage 93 全 GREEN（2026-09-14）"
   - `docs/legacy-plans/landed/` 是否迁：observability-edge-gaps 整个计划还有 §B/C/D/E/F 5 项 P2-P3 未完，**暂不迁**；§A-extension 加进 landed-parts 即可

5. **§2.5 收口自检**：
   - `git status` 干净
   - `git status -sb` 无 ahead/behind
   - `git branch --merged main` 空
   - `git push origin main` 成功

## 4. 风险 + 缓解

| 风险 | 缓解 |
|---|---|
| ai-svc 与 analytics-svc 同时消费同 topic（chat-events）时 consumer group 冲突 | ai-svc consumer group ID 与 analytics-svc 不同（kafka config），已验证无冲突 |
| analytics-svc 镜像 rebuild 后端口冲突 / Nacos 注册失败 | 沿用 Stage 92 e2e rebuild 流程；Nacos 注册通过 health check 自愈 |
| extractSw8Header 不抽 shared，未来需维护两份 | 注释 + 跟踪卡（Stage 94+ 收敛到 shared/pkg/messaging） |
| OAP UI 跨进程 trace 可视化阻塞 | 沿用 header 实证 + 日志 traceID 比对，与 Stage 92 同模式 |
| observability-edge-gaps §B (consumer.attempts 无并发) 在 analytics-svc 也存在 | **本 stage 不修 §B**，避免范围蔓延；记入 Stage 94+ 候选（与 ai-svc 一并加锁） |

## 5. 调研依据

按 AGENTS.md §〇硬规则，写文档前必读 + 已读：

- `emotion-echo-shared/pkg/grpcinterceptor/tracing.go` —— Tracer 接口已含 `CreateEntrySpan`（Stage 92 PR-1 新增，`tracing.go:110-111`）
- `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky.go` —— Go2SkyTracer adapter 实现 `CreateEntrySpan`（`tracing_go2sky.go:173-189`），extractor 协议与 ai-svc consumer 用法一致
- `emotion-echo-ai-svc/internal/consumer/consumer.go` —— ai-svc 样板：`ConsumerGroupHandler.Tracer` 字段 + `ConsumeClaim` `if h.Tracer != nil` 守卫 + `extractSw8Header` helper + 4 个 messaging.* tag（`consumer.go:115-134` / `208-218`）
- `emotion-echo-ai-svc/main.go` —— wire 模式：`sharedgrpc.NewGo2SkyTracer(tracer)` 包一层（`main.go:331`）
- `emotion-echo-analytics-svc/internal/kafka/consumer.go` —— 目标文件：`chatEventHandler` 结构体（`consumer.go:119-125`）+ `NewConsumer/WithDLQ/WithMaxRetries` builder 模式（`consumer.go:47-87`）+ `ConsumeClaim` 现有逻辑（`consumer.go:142-165`）
- `emotion-echo-analytics-svc/main.go` —— wire 点：`tracer *go2sky.Tracer` 已存在（`main.go:150-167`）+ Kafka consumer 构造链（`main.go:186-208`）
- 镜像现状：`deploy/docker-compose.apps.yml:193` analytics-svc tag `v0.1.5`（本 stage bump → `v0.1.6`）

> 最后更新：2026-09-14 by Stage 93 计划编写
> 关联：observability-edge-gaps-from-code-review.md §A-extension / stage-92 收口报告 / 决策 6 (logging)/ PR-OBS-15 / PR-OBS-17
