---
purpose: 当前有效、未来排期的功能计划
status: Round 1 占位 · Round 2 首批内容迁入
---

# 当前有效的计划

> 这些计划**未被任何 stage 取代**，等待排期与实施。
> 与 `legacy-plans/`（已落地/已偏移/历史价值）不同，这里的文档描述"接下来要做的事"。

## 当前条目（Round 2 迁入）

| 文件 | 来源 | 主题 |
|------|------|------|
| `ai-response-structured.md` | `.trae/documents/ai-response-structured.md` | AI 回复结构化 + Markdown 渲染 |
| `three-vrm-usage-reference.md` | `.trae/documents/three-vrm-usage-reference.md` | Three-VRM API 参考手册 |
| `wechat-qq-login-and-upload.md` | `.trae/documents/微信QQ登录和文件上传实施计划.md` | QQ OAuth + 通用文件上传 |
| `nacos-enablement-dev.md` | 新增 | Nacos dev 模式从"半启用"到"全链路" |
| `observability-compose-gap.md` | 新增（2026-09-04） → 已被取代 | dev compose 可观测性三层补齐。**PR 拆分已被 `observability-sprint-b.md` PR-OBS-1~8 取代**，原文件 §一/§二现状作为参考保留 |
| `db-migration-auto-apply.md` | 新增（2026-09-04） | 13 个服务 migrations 无自动应用机制，dev 库实测一个都没跑过。**本轮已落地**（提交 0d17e85），文件作为方案设计保留 |
| `todo-pile-2026-09-04.md` | 新增（2026-09-04） | 本轮修复过程中实测暴露的未关闭项汇总：TTS 不可用 / 上传未实现 / 多模态不可用 / 文档失真登记 / dev-prod 端口策略 |
| `kafka-reliability-gaps.md` | 新增（2026-09-07） → Sprint A 已落地 | Kafka 管线 6 项健壮性缺口汇总。**Sprint A 已 100% 收口**（10 commit on `feat/chat-dev-event-publisher-a1-1-red`,7/7 服务绿），见 [stages/stage-43-kafka-reliability-sprint-a.md](../stages/stage-43-kafka-reliability-sprint-a.md)。Sprint B §1.4 lag 监控已并入 `observability-sprint-b.md` PR-OBS-7（Stage 74 补齐面板 datasource uid + Grafana 端口）；§1.5 Protobuf 迁移 **Stage 73 已落地**（含 eventrow 共享包）。**当前残余 = §1.6 P3**（outbox relay 死信告警接 alertmanager）+ §1.4 可选 consumer 进程级指标 |
| ~~`grpc-inter-service-migration.md`~~ | 已 landed 2026-09-13 | 后端微服务间调用 HTTP→gRPC 改造：**全线落地**（2026-09-11 Sprint C/D/E/F1/F2 + Sprint G error 映射；2026-09-12 销账 #32/ PinConversation）。决策 4 BFF→4 svc 全方法 100% gRPC 覆盖 + ai-svc 业务 3 方法 gRPC 化（Stage 81 起 BFF→llm-service 也 gRPC 化，超原计划预期）。**唯一残余 = chat-svc StreamMessages**（业务未触发，有意维持 Unimplemented，见决策 4 ADR §八）。详见 `legacy-plans/landed/grpc-inter-service-migration.md` |
| `bff-grpc-error-mapping-backlog.md` | 已迁 landed Sprint G (commit `f874d3f`) → 详见 `legacy-plans/landed/bff-grpc-error-mapping-backlog.md` | BFF 全局 gRPC error → HTTP code 映射 backlog：Sprint F2 V3 实测发现 tts/synthesize 走 gRPC Unavailable 时 BFF 误返 502（应 503）。5 个 gRPC client 都用 `fmt.Errorf` 简单 wrap，handler 侧硬编码 `http.StatusBadGateway`，需抽 `mapGRPCError` helper（7 类 gRPC code → HTTP code 映射）+ 5 client 改造 + handler 改造 + docker 端到端回归。半天工作量 |
| `observability-testing-gap.md` | 新增（2026-09-07） → 已被取代 | 可观测性链路测试完善。**PR 拆分已被 `observability-sprint-b.md` PR-OBS-9~16 取代** |
| `observability-sprint-b.md` | 新增（2026-09-08） → 已落地 | 观测链路 Sprint B 执行源。**16 个 PR-OBS-X + 1 fix = 34 commit 全落地**（2026-09-08），见 [stages/stage-44-observability-sprint-b.md](../stages/stage-44-observability-sprint-b.md)。**PR-OBS-17/18/23/15/19（2026-09-08）§四 B/C 收口**：Stage 45 接口抽象 + Stage 46 HTTP EntrySpan + Stage 47 logging 接入 + Stage 48 err 透传 + Stage 49 gRPC rpc.* tag。**业务功能 6/6 步 100% 收口**。**Stage 50 端到端验证归档**：单元测试 100% PASS + smoke 10/12，具体见 [stages/stage-50-e2e-validation.md](../stages/stage-50-e2e-validation.md)。剩余非业务（sw-oap telemetry / 分支 merge / Nacos / 运维 SQL）见 stage-44 §四 |
| ~~`gozero-removal.md`~~ | 已 landed 2026-09-07 | go-zero 完全移除收尾（决策 1）：conf→shared/pkg/config + logx→slog 下沉 shared + rest.Middleware 类型清零 + goctl 归档；10 个 TDD PR 全部 merged。详见 `legacy-plans/landed/gozero-removal.md` 与 `stages/stage-41-gozero-removal.md` |
| ~~`file-understanding-llm.md`~~ | 已 landed 2026-09-13（**Stage 89**） | 文件理解发给 LLM（文件+提问一起发，会话内持续引用）：chat.proto file_name + emotion_llm.proto FileAttachment.files；chat-svc file_name 五处映射 + migration 007 修 intent VARCHAR(16)→32 暗坑；BFF ai-stream 文件收集注入（≤2 条 + URL 重写）；llm-service file_context（pypdf + python-docx + 白名单 SSRF）；前端附件挂输入框 + 三分支发送流。txt 哨兵 e2e + 追问引用全通；PDF 路径注入正常但 DeepSeek 拒读记 residual。详见 `legacy-plans/landed/file-understanding-llm.md` 与 `stages/stage-89-file-understanding-llm-2026-09-13.md` |
| ~~`stage-92-kafka-sw8-propagation.md`~~ | 已 landed 2026-09-14（**Stage 92**） | Kafka 跨进程 sw8 trace 透传 P1 项：shared Tracer 接口扩 CreateExitSpan/CreateEntrySpan；chat-svc KafkaEventPublisher 注入 sw8 header（v0.1.10）；ai-svc ConsumerGroupHandler 用 CreateEntrySpan 替代 CreateLocalSpan（v0.1.6）。容器 e2e 实证 chat-svc producer 写入完整 sw8（221 chars）。详见 `legacy-plans/landed/stage-92-kafka-sw8-propagation.md` 与 `stages/stage-92-kafka-sw8-propagation-2026-09-14.md` |
| `observability-edge-gaps-from-code-review.md` | 新增（2026-09-13） → **§A 已 landed 2026-09-14（Stage 92）** | 2026-09-13 会话对可观测链路相关代码（SkyWalking / Prometheus / metrics / Kafka 链路）做细致审查时发现的问题汇总。6 个 issue：(A) Kafka 异步链路 sw8 没透传（P1，✅ Stage 92 全 GREEN）/ (B) consumer.attempts map 缺并发保护（P2） / (C) metrics path "unmatched" 兜底造成潜在指标污染（P2） / (D) GinSkywalking 跳过路径硬编码（P3） / (E) AI 模型客户端 env 静默失败（P2） / (F) consumer.go 文件职责混杂（P3）。每个 issue 含代码佐证 + 修复方案 + DoD + 工作量。**§A 后残余 ≈ 0.5 人天**（B/C/E 优先）。**注意：本计划由外部代码审查发现，非项目演进内部识别** |
| ~~`stage-93-analytics-svc-sw8-propagation.md`~~ | 已 landed 2026-09-14（**Stage 93**） | analytics-svc consumer 端接入 `Tracer.CreateEntrySpan` 从 `msg.Headers["sw8"]` 重建父 trace（与 ai-svc Stage 92 PR-2 对称）。代码 6 处全落地 + 镜像 v0.1.6 + Round 2 单测 13/13 全绿（实测 0.612s） + 容器 e2e sw8 221 chars + traceID 重建实证。**残余** = 容器 e2e 脚本归档到 Stage 94+ + OAP 9.x graphql queryDuration bug 阻塞 UI 可视化 + extractSw8Header 收敛到 shared/pkg/messaging。详见 [stages/stage-93-analytics-svc-sw8-propagation-2026-09-14.md](../stages/stage-93-analytics-svc-sw8-propagation-2026-09-14.md)；索引登记迁 `legacy-plans/landed/observability-edge-gaps-from-code-review.md` landed-parts §A-extension |
| `code-review-2026-09-14.md` | 新增（2026-09-14） → **已 landed 2026-09-14** | 2026-09-14 用户要求"分析代码找漏洞"任务中，4 个并发子 agent 分别覆盖 **服务注册链路 / 可观测链路 / Kafka 事件管道 / 中间件与基础设施** 4 个技术域的全面代码审查汇总：**10 个 P0 + 28 个 P1 + 25 个 P2 + ~25 个 P3**，含 15 项文档与代码偏移（含 edge-gaps §A"🟢 landed"实际仅"🟡 部分落地"）。**Stage 94 7 个 PR 已 100% 收口 10 个 P0**（累计 7 commits），详见 [`legacy-plans/landed/code-review-2026-09-14.md`](../legacy-plans/landed/code-review-2026-09-14.md) + [`stages/stage-94-code-review-2026-09-14-p0-closure.md`](../stages/stage-94-code-review-2026-09-14-p0-closure.md)（P0 全景收口）。**残余 = 28 个 P1 + 25 个 P2 + ~25 个 P3**（独立 sprint 排期）|
| `code-review-2026-09-14-round-2.md` | 新增（2026-09-14） → **已 landed 2026-09-15（Stage 97）** | Round 1 后的第二轮补充，覆盖 **前端 / AI 业务层 / 业务数据层 / 构建-CI-部署** 4 个 Round 1 未触及的技术域：**10 个 P0 + 17 个 P1 + 21 个 P2 + 5 个 P3**，含 8 项文档与代码偏移（含 README "compose 自动加载 .env.local" 与代码不符、`stage-91` 强指令性 prompt 副作用、INTERNAL_API_KEY env 名跨 svc 错配等）。最关键 P0：§P0-R2-3 llm-service HTTP 端零鉴权 / §P0-R2-5 ai-svc↔llm-service env 名错配 / §P0-R2-6 daily_emotion_by_modality_v 缺 GRANT（一行 SQL 修复 §契约 3 必抓 bug）/ §P0-R2-8 web 容器以 root 跑 Node.js。**总工作量 ≈ 28-35 人天**（紧凑 18-22）。**注意：本计划由外部代码审查发现，非项目演进内部识别**。**Stage 97 落地 = 10 个 P0 全部 + 顺手 Round 1/2 P1+P2 部分 + Round 2 P3-CI**。**Push 状态**：12 commits 已 push origin/main (`460b814..391f592`)，PR-9e (workflow commit `23419eb`) push blocked by PAT workflow scope——见 [`stages/stage-97-round2-p0-closure.md`](../stages/stage-97-round2-p0-closure.md) §6.1，详见 [`legacy-plans/landed/code-review-2026-09-14-round-2.md`](../legacy-plans/landed/code-review-2026-09-14-round-2.md)。**残余 = P0-R2-1 userInfo localStorage 残余 + Round 2 P1/P2 剩余项** |

## 写入规范

- 新增计划时文件名用 kebab-case 英文
- 文件顶部加 front-matter：`status: planned` / `priority: high|medium|low`
- 引用 ADR 与相关 stage 时用相对路径
- 完成后迁移到 `legacy-plans/landed/`
