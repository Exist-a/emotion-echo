---
status: in-progress
stage: CL-08
slug: cl-08-memory
scope: 跨会话长期记忆 —— 用户事实抽取 → 时序知识图谱（Graphiti + 嵌入式 Kuzu）→ 注入 prompt → 管理页可查看/删除 → 情绪模式与情节检索；含向量/全文/图三层召回
owner: TBD（开工时登记 Lane-CL-xx）
created: 2026-10-18
planning-round: 2026-10-18（用户授权提前规划 · `CL-D-37`；**本轮只落文档、零代码改动**）
source-plan: docs/plans/conversation-memory-pending-decision-2026-09-24.md（D-c 已由 `CL-D-35` 收敛为图记忆）
related:
  - ../../roadmap.md（§2 CL-08a~e 行 / §4 CL-08 例外注记）
  - ../../decisions.md（`CL-D-01` / `CL-D-24` / `CL-D-33` / **`CL-D-35`~`CL-D-37`**）
  - ../../parallel-contract.md（§三 占位登记 / §1.2 共享脊柱 / §1.3 共享 docker 资源）
  - ../../RUNBOOK.md（执行六步 / 证据有效性 / 收口契约）
  - ../../../architecture/adr/adr-2026-10-agent-memory-graph.md（形态与选型 · **含 3 项待实测项**）
---

# CL-08 · 记忆（跨会话 · 图记忆） — 执行计划

> **形态与选型的权威在** [`adr-2026-10-agent-memory-graph.md`](../../../architecture/adr/adr-2026-10-agent-memory-graph.md)（决策 45）。
> **本文只管执行**：上下文功课 / 测试点 / Phase 切分 / 证据 / 收口判据。改设计改 ADR，改执行改本文。
>
> ⚠️ **本文写于"提前规划轮"（2026-10-18，用户授权 `CL-D-37`）**：
> 与 roadmap §4「未到阶段不预设细节」的关系是——**这是用户当场授权的例外**，故提前写细；
> **本轮不写任何代码**，Phase 0 起全部为"待执行"。
> ⚠️ **阶段状态由审计 / 第二方判定，执行者不自行宣布 `done`**（RUNBOOK 纪律）。

---

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件，2026-10-18 实读）

| 文件 | 关键事实（决定了本计划怎么落） |
|---|---|
| `emotion-echo-web-bff/internal/handler/prompt_assembly.go:29-91` | 段类型常量 + `systemSegments` 段序 `base → agent → personality → emotion → guard`；**空段不产生**（`:40-44`），`assembleSystemPrompt` 再过滤一次（`:157-166`） |
| `.../prompt_assembly.go:50-66` | `assembleInput` 结构——新字段（记忆）加在这里 |
| `.../prompt_assembly_test.go`（`TestPromptAssembly_ZeroRegression_GoldenBytes`）/ `prompt_assembly_agent_test.go:101-114,120-136` | **逐字零回归 + 段序位置断言**的现成手法：无信号时输出必须**逐字等于**基线 golden |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:425-436` / `:497-507` | `assembleMessages` 的**两个**调用点（llm-gRPC 路径 / HTTP 直连路径）⇒ 新字段**两处都要传**，否则路径间漂移；`ai_stream_handler_agent_test.go:198-209` 钉住"恰好 2 个字面量" |
| `.../internal/handler/prompt_history.go:50-109` | 同会话历史的取数口径（fetch 21 / trim 到 10 条 / token 预算 2000 / 至少留 2 条）——**记忆段的预算要与它一起算总账** |
| `emotion-echo-web-bff/internal/downstream/llm_grpc.go`（pb 构造）+ `proto/emotion_llm.proto:69` | proto 有 `user_id` 字段但 **BFF 不填** ⇒ 若记忆检索放 llm-service，**它不知道"这是谁"**（见 §6 风险 R1） |
| `emotion-echo-ai-svc/internal/logic/consumehandler.go:45-97` | **"消费者里调 LLM"的现成模板**：类型过滤 → 解析 data → 只处理 user 角色 → 调 LLM → 写库（`EventID` 幂等：`:79-87` `ON CONFLICT DO NOTHING`） |
| `emotion-echo-ai-svc/internal/consumer/consumer.go` + `consumer_failure.go` | 重试（`deliverWithRetry`，指数退避、预算 = MaxRetries+1）与 DLQ 由**框架统一**处理 ⇒ 抽取 worker 只需 `return error` |
| `emotion-echo-chat-svc/migrations/c001_create_outbox_events.sql:31` + `c010_add_trace_sw8_to_outbox.sql` + `internal/outbox/relay.go` | outbox 是 chat-svc 的**唯一**事件出口（业务行 + outbox 行同事务写）；`status`/`attempts`/`trace_sw8` 齐备 ⇒ 抽取**不需要新造队列** |
| `deploy/db/migrate.sh` + `deploy/db/README.md:53-55` | 迁移自动发现 + `schema_migrations` checksum 追踪；**幂等是硬要求**（每次 `compose up` 都会跑） |
| `scripts/test_migrations_no_service_order.sh` / `deploy/db/README.md:116-125` | **每个服务只碰自己的 schema**；跨 schema JOIN 禁止（走 gRPC/Kafka）⇒ **记忆表落哪个 schema 由"谁写入"决定**（见 §7 N2） |
| `emotion-llm-service/requirements.txt` + `chat_completion.py:41-70` | Python 宿主现成：`openai>=1.40`（OpenAI 兼容端点）、`grpcio`、`pydantic>=2`、`pypdf`、`python-docx`，Python 3.12；**无 Kafka 依赖** |
| `deploy/docker-compose.apps.yml`（内存限额） | 除 `sensevoice`/`xtts` 各 **6144M** 外全为 **1024M** ⇒ 新增容器是**真代价** |
| `docs/architecture/decisions.md:999`（决策 39） | `.wslconfig` **8GB** + **19 容器**稳态 **~6G** ⇒ **击穿前科**（2026-09-22 三连冻结） |
| `emotion-echo-web/e2e/cl-04-prompt-memory.spec.ts` | 已有"**同日多轮**记得上文"的人可见钉（埋姓名+岗位 → 第二轮只问"还记得吗"）⇒ 本阶段要的是它的**跨会话**版本 |
| `emotion-echo-chat-svc/internal/events/kafka_publisher.go:88-96` | **分区键 = `ConversationID`**（消息/会话事件皆然）+ `sarama.NewHashPartitioner`（`:38`）⇒ **同会话事件天然落同一分区、天然有序** ⇒ **时效判定最需要的顺序已有，不必改 topic 键**（改它会牵动 analytics/ai-svc 两个既有消费者） |
| `getzep/graphiti`：`graphiti_core/llm_client/`、`graphiti_core/embedder/`、`graphiti_core/driver/`（**API 实读**） | `llm_client/client.py` 与 `embedder/client.py` 是**可继承接口**（另有 `openai_generic_client.py` / `openai.py` 适配 OpenAI 兼容端点）⇒ **单测可注入确定性假实现、不打真 LLM**；驱动实存 `neo4j`/`falkordb`/**`kuzu`**/`neptune`；另内建 `cache.py` 与 `token_tracker.py`（成本/缓存抓手，喂 T-18） |

### A.2 相关 ADR / 决策（已查）

- **本阶段权威**：`adr-2026-10-agent-memory-graph.md`（决策 45，含 4 项待实测项 T-a~T-d）。
- `CL-D-01`（跨会话 = RAG，**形态未定** → 本轮由 `CL-D-35` 收敛）、`CL-D-35/36/37`（本轮三项）。
- `CL-D-24`（角色人设隐私口径：**明文 + 软删除 + 管理页可删**，不做应用层加密/字段脱敏）⇒ 记忆沿用同一口径。
- `CL-D-07`（融合情绪：PG=历史/真相，Redis=用户级热点最新，TTL 24h，**取不到优雅降级**）⇒ 记忆注入的读路径沿用此先例。
- `CL-D-33`（首字路径多一次 LLM 调用 ⇒ TTFT 940→1362ms，**+45%**）⇒ 抽取必须异步的**实测依据**。
- `adr-2026-10-tts-api-cosyvoice2.md`（SiliconFlow 纪律：配置注入、**禁在业务代码硬编码供应商域名/模型名**）⇒ embedding 复用同一账户，沿用同纪律。
- `adr-2026-09-incremental-rpc-adoption.md:41`（**新增服务/新增调用一律 gRPC + proto**）⇒ 记忆检索若跨进程，必须走 gRPC 并登记 `scripts/rpc_status.tsv` 终态。

### A.3 架构假设清单（本文假设 vs 现状核对）

| 假设 | 现状核对 |
|---|---|
| 已有记忆/向量/图设施 | ❌ **零**：全仓 `grep memory_fact\|user_memory\|episodic\|graphrag` = 0；无 `pgvector`；无图库；`.env.local` 无 embedding key |
| 有现成异步通道可承载抽取 | ✅ outbox（chat-svc）→ Kafka `chat-events` → 消费端；ai-svc 已是"消费调 LLM"的活样本 |
| 有现成 prompt 组装层可挂新段 | ✅ `prompt_assembly.go` 段式组装 + 空段丢弃 + 逐字 golden 钉 |
| 有 embedding 供给 | ⚠️ **有服务、未实测**：硅基流动 `POST /v1/embeddings`（`BAAI/bge-m3`）文档可查（ADR 调研依据），但**该 key 的权限/配额未实测**（待实测项 ①） |
| 图能力可零新增容器落地 | ⚠️ **假设待证**：Graphiti 有 `kuzu` 驱动（实读目录），但 `graphiti-core` 与 llm-service 现有依赖是否兼容**未实测**（待实测项 ②） |
| 消息能带上"是谁"到 llm-service | ❌ **当前不能**：BFF 不填 proto 的 `user_id`（`downstream/llm_grpc.go`）⇒ 必须先补（已登账 `CL-F-56`） |
| `msg_summary_v` 可当会话摘要用 | ❌ 它是**按天统计消息条数**的报表视图，与会话摘要无关（`CL-D-31` 同款误读，勿再犯） |

### A.4 未跑 smoke 声明

本轮为**纯文档规划轮**，未起 dev 栈、未跑 `scripts/smoke_data_layer.py`（§2.4 数据契约不适用于"尚未写代码"的状态）。
**Phase 0 的 spike 结论必须附命令与输出**，不得以"文档说支持"代替（ADR §待实测项）。

---

## §1 目标与非目标

**目标**：让 AI 在**跨会话**场景下"记得用户"——用户事实/偏好/重要事件被抽取进**时序知识图谱**，
在后续对话中被检索并注入 prompt；用户可在管理页**查看与删除**这些记忆；事实会变时**新值生效、旧值留史**。

**非目标（本阶段不做）**：
- 不做会话摘要器（那是**另一个**子项，仅登记在 `CL-D-31` 口径内，非本阶段前置）。
- 不做跨用户/群体侧写（每个用户一个独立记忆空间，**绝不共享实体**）。
- 不做端侧记忆（Lane O 的 `on-device-hybrid-inference` 是另一个问题，见来源计划 §E）。
- 不做"情绪模式"的自动化建议（`CL-08b` 只做"查得出趋势/触发/缓解因素"，不给干预建议）。
- 不做风险信号的处置策略（`CL-08e` 的安全拦截设计另列，含危机通道）。

---

## §2 测试点清单

> 判定分级见 RUNBOOK §4：`[A]` 自动可判 / `[V]` 视觉（人看截图）/ `[M]` **仅用户可裁**。
> 结果四值 `PASS / FAIL / BLOCKED / N/A` + **执行输出**（命令 / 退出码 / 截图）。"已创建"不算证据。

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-1 | 记忆相关迁移（episode 引用 / 实体 / 事实 / 抽取审计）**幂等可重跑** | [A] | 迁移工具连跑两次全绿 + checksum 追踪一致 | |
| T-2 | PG 系统记录**写入读回一致**：事实行含 `valid_from`/`valid_to`/`source_message_id`/`user_id`/`status` | [A] | Go 契约测试（表驱动 + 边界） | |
| T-3 | **双时间轴语义**：新事实与旧事实时间重叠且矛盾 ⇒ 旧行 `valid_to` 被置为新行 `valid_from`（**不删除**，历史可查） | [A] | 契约测试 + 运行时 DB 快照 | |
| T-4 | **抽取幂等**：同一条消息重复消费不产生重复事实（唯一索引 + `ON CONFLICT`） | [A] | 消费两次 ⇒ 事实数为 1 | |
| T-5 | **抽取失败不阻塞主链路**：LLM 不可用时发消息仍成功；抽取进重试/DLQ，恢复后自愈重发 | [A] | 运行时 A/B（停 LLM ⇒ 消息照发；恢复 ⇒ 自动补齐） | |
| T-6 | **实体消解（中文指代）**：同指表述（"我妈"/"母亲"）合并为同一实体，不产生重复节点 | [A] | 评测集子集（见 T-19）+ 人工抽检 | |
| T-7 | **注入零回归**：无记忆时 `assembleSystemPrompt` **逐字等于**现状 golden（字节级） | [A] | 复用 `TestPromptAssembly_ZeroRegression_GoldenBytes` 手法 | |
| T-8 | **段序不变量**：有记忆段时 `guard` 仍在末位；**两个 `assembleMessages` 调用点都传**新字段 | [A] | 位置断言 + 字面量计数钉 | |
| T-9 | **记忆段内容正确**：给定固定记忆集 ⇒ 注入文本含事实且标出时间/来源（**不断言模型措辞**） | [A] | 组装层契约测试 | |
| T-10 | **关键路径不查图**：注入路径不产生图查询（图查询只发生在异步侧） | [A] | 架构钉（依赖注入计数 / 调用面扫描） | |
| T-11 | **降级**：记忆源报错 / 超时 / 为空 ⇒ 段为空、对话正常（**增强不是依赖**，不编造） | [A] | 契约测试（注入错误源 ⇒ 输出等于无记忆基线） | |
| T-12 | **token 预算**：记忆段超预算被截断，且 system+history 总量不超上限 | [A] | 预算契约测试（与 `prompt_history.go` 的 2000 预算一起算） | |
| T-13 | **图后端可用**：一条 episode 抽取后图上可查（节点 / 边 / 时间字段齐全） | [A] | Python 单测（Graphiti + Kuzu）+ 运行时实查 | |
| T-14 | **三层召回各生效**：语义向量 / 全文 / 图遍历各返回预期项（含**中文**查询） | [A] | 检索层单测 + 评测集 | |
| T-15 | **事实更新端到端**："我住北京" → （跨会话）"上个月搬到上海" ⇒ 注入只用上海，且**能说出北京是前值** | [V] | IAB（生产构建）+ DB/图快照 | |
| T-16 | **删除闭环**：删单条 / 删全部 ⇒ PG 与图上均不可再检索，**下一次注入不再出现** | [A]/[V] | 运行时：删除 → 新会话提问 → 断言不含被删事实 | |
| T-17 | **用户隔离**：A 的记忆绝不出现在 B 的注入（含**同浏览器换账号**） | [A] | 契约测试 + 运行时（两账号交叉验证） | |
| T-18 | **指标被消费**：抽取次数 / 失败数 / token 消耗出现在**面板**上（对齐 CL-01「埋点必须被消费」） | [A]/[V] | `smoke_observability.py` 逐 expr 非空 + 面板截图 | |
| T-19 | **评测集（价值证明）**：跨会话事实召回 / 事实更新 / 时间推理 / **不相关不注入（精确率）** / 删除生效 —— **开记忆 vs 关记忆 A/B** | [A] | 评测脚本 + 逐题结论（**不做"感觉变好了"式断言**） | |
| T-20 | **人可见钉**：会话 A 埋入事实 → **新会话 B** 问"还记得吗" ⇒ 回复复述该事实 | [V]/[A] | Playwright（`cl-04-prompt-memory.spec.ts` 的跨会话版）+ IAB 截图 | |
| T-21 | **记忆的可见性口径**：管理页呈现形态 / 用户能否**关闭**记忆 / 删除的确认文案 | [M] | 用户过目（执行侧不自拟） | |

> 数量：21 点（`[A]` 15 / `[V]` 4 / `[M]` 1，含 3 项混合）。`[M]` 点未裁不得判 `done`（可先 `BLOCKED`）。

---

## §3 分阶段实施（TDD：先 RED 回归钉，再 GREEN）

| Phase | 内容 | RED 断言 | 备注 |
|---|---|---|---|
| **0** | **前置实测 spike（不写生产代码）**：① 真打一次 `POST /v1/embeddings`（`BAAI/bge-m3`）——**可用性 / 维度 / 延迟 / 批量上限 / 是否与 TTS 共享配额**；② 试装 `graphiti-core` + `kuzu`（与 llm-service 现有依赖是否冲突、镜像涨多少）；③ 定 **Kuzu 数据文件的持久化与备份**方案；④ **Kuzu 并发形态**（多 worker 打开同一目录）+「从 PG 重建图」的实际耗时 | 四项各出一条命令 + 输出（**不是"文档说支持"**） | ✅ **已完成（2026-10-22）**：T-a 1024 维可调（417ms/单条）；T-b 装得上 + **llm-service 215 单测全绿**（+161MB）；T-c 命名卷跨重建不丢；T-d **严格单写者** + COPY 65k 行/s。证据 `evidence/phase0-*`。✅ **T-e 已裁决（2026-10-22 用户）= `falkordblite`**：追加实测通过（多进程共享 / 跨容器持久化 / 内存 ~145MB），ADR 已转 Accepted；**Phase 4 用 `graphiti-core[falkordblite]`，不再用 Kuzu** |
| **1** | **PG 数据模型（系统记录）**：`memory_episode` / `memory_entity` / `memory_fact`（含 `valid_from`/`valid_to`/`source_message_id`/`user_id`/`status`）/ `memory_extract_run` + 迁移（幂等 + checksum + 若也有内联 DDL 则补 parity 守卫） | 引用不存在的表 ⇒ 编译/查询失败；契约测试全红 | 对应 T-1/T-2/**T-3**；**schema 落点按"谁写入"推导**：写入方 = ai-svc（Phase 2）⇒ 落 **`emotion_echo_ai`**（`test_migrations_no_service_order.sh` 会拦跨 schema）。✅ **已完成（2026-10-24）**：**范围校正**——`memory_fact`/`memory_extract_run`/`memory_note` 已由 CL-08a i011 落地 ⇒ 本轮只补 `memory_episode` + `memory_entity`（i012）；T-1（migrate 两次全 SKIP + checksum）/T-2（真库往返 4/4）/T-3（fact 侧沿用 CL-08a）全 PASS；全包 17/17 绿；证据见 `phase1-pg-model-report-2026-10-24.md`。deploy/db 无内联 memory DDL ⇒ parity 守卫不适用 |
| **2** | **抽取 worker（异步，拓扑已定）**：**ai-svc** 内新增**独立 consumer group**（不与情绪分析共用），照 `consumehandler.go:45` 形态；消费到 `message.created` ⇒ 调 llm-service 的 `MemoryService.UpsertEpisode`；`message_id` 唯一索引做幂等；失败 `return error` 交给既有重试/DLQ | 消费两次 ⇒ 事实重复（T-4 红）；LLM 挂 ⇒ 消息失败（T-5 红） | 对应 T-4/T-5；**禁在发消息关键路径做 LLM**（`CL-D-33`）；**顺序**：同会话已天然有序（key=`ConversationID`），**不改 topic 键**；跨会话用事件 `send_time` 兜底 。✅ **代码级完成（2026-10-24，PR #275）**：consumer group 沿用 CL-08a 既有「ai-svc-memory」（独立 group 已存在，不重复建）；增量 = `WithGraphRepo` 把 episode/entity 持久化接进 `extractSource`（位于 FinishExtractRun(done) **之前**，失败 ⇒ failed ⇒ 重投收敛）；**图侧 gRPC（llm-service MemoryService）仍归 Phase 4**。T-4 ✅（单测 3 次消费 + 真库集成：三表各 1 行 + LLM 只调 1 次）；T-5 单测面 ✅（失败传播/failed 回填/恢复收敛/nil 零回归），**运行时 A/B（LLM 挂发消息仍成功）待 dev 栈窗口**（栈 2026-10-24 仅剩 3 容器，未重建）|
| **3** | **注入段（BFF）**：新增 `segmentKindMemory` + `assembleInput.memory` + `memorySegment()`；**关键路径只读 Redis 用户级摘要**（`CL-D-07` 同构：用户级键 + TTL + 取不到降级），两处调用点都传；段序 `base → agent → personality → memory → emotion → guard`、记忆段空则不产生 | 无记忆时输出与 golden **不逐字相等**（T-7 红）；段序位置断言红（T-8） | 对应 T-7/T-8/T-9/**T-10**/T-11/T-12；预算 **≤600**（与 history 的 2000 分开记账） |
| **4** | **图（Graphiti + Kuzu）+ 契约**：`graphiti-core` 落 llm-service（`openai_generic_client` 吃现有 `LLM_*`；embedder 用 `openai.py` 指硅基流动 base_url）；**新增 `proto/memory.proto`**（`MemoryService`）+ 生成物 + `rpc_status.tsv` 终态 + **必须有调用方**；**顺带补 BFF → llm-service 的 `user_id`**（`CL-F-56`） | 图查询返回空 / 身份缺失导致取不到记忆（T-13/T-17 红） | 对应 T-13/T-14/T-17；改 proto 必须按仓库既有方式重生成（RUNBOOK §2）；**单测注入假 LLM/embedder**（接口可继承），不打真 LLM 。✅ **已完成（2026-10-25，PR #281）**：全链路真栈打通（web→…→Graphiti→falkordblite）；T-13（Episode 原文+实体+双时间轴）/T-17（group_id 物理分图零交集）/T-19（评测 A/B 双向全 PASS）/T-20（chromium 2/2）；CL-F-56 销账。**修正**：图写 best-effort 不进重试（重建工具兜底）；检索/删除 RPC 未进 proto（无生产调用方，归 CL-08d）；deepseek 需 json_object 模式（json_schema 400）。运行时调试修 4 个真问题（卷属主/事件循环绑死/response_format/mTLS）见 phase4 报告 §2 |
| **5** | **删除与管理（`CL-08d`）**：PG 删 + 图删 + **Redis 摘要失效**；管理页列表/详情/删除；补**从 PG 重建图**的工具（ADR 决策 8 的配套） | 删除后仍被检索到 / 仍出现在注入里（T-16 红） | 对应 T-16/T-21；口径沿用 `CL-D-24`（明文 + 软删 + 可删） |
| **6** | **评测（价值证明，贯穿）**：评测集 + 开/关记忆 A/B；把 `cl-04-prompt-memory.spec.ts` 扩成**跨会话**版 | 关记忆也"答得出" ⇒ 评测设计无效；开记忆精确率不达标 | 对应 T-19/T-20；**评测不通过不得宣布功能成立** |
| **7** | **运行态验收 + 收口**：生产构建 IAB + Playwright 回归钉 + 指标/面板（T-18）+ 第二方核对 | — | 对应 T-15/T-18/T-20；证据按 RUNBOOK §4.1 有效性要求归档 |

---

## §4 收口判据（"做到什么程度"）

**功能判据**
- 跨会话：会话 A 说过的事实，**新会话 B** 问起时答得出（T-20，人可见）。
- 时效：事实更新后**只用新值**，且旧值仍可追溯（T-15）。
- 可控：用户能在管理页**看见并删掉**记忆，删完不再被注入（T-16）。
- 不污染：用户自己写的角色人设优先于系统推断（沿用 `CL-D-32` 口径），记忆段不与护栏段冲突（T-8）。

**机器判据**
- 无记忆时输出**逐字等于**基线（T-7 字节级）；
- 迁移幂等 + 服务序独立（T-1）；
- 抽取幂等（T-4）+ **LLM 挂不阻塞发送**（T-5 运行时 A/B）；
- 指标在面板上**真被消费**（T-18）；
- **评测集通过**（T-19）：跨会话召回率与"不相关不注入"的精确率都有数字，且开/关 A/B 有差异。

---

## §5 回归钉

- **Playwright**：`emotion-echo-web/e2e/` 新增跨会话说"记得吗"的钉子（现有 `cl-04-prompt-memory.spec.ts` 是同日多轮，**不覆盖跨会话**）。
- **Go**：组装层零回归 + 段序（沿用 CL-04/CL-06 既有钉）+ 抽取幂等 + 降级。
- **Python**：Graphiti/Kuzu 检索层单测（含中文查询）；llm-service 现有 `pytest tests/unit/` 不得回归。
- **前端**：管理页 vitest（列表/删除交互）。

---

## §6 风险与坑

| # | 风险 | 说明与对策 |
|---|---|---|
| R1 | **身份传不到 llm-service** | BFF 当前不填 proto `user_id`（`downstream/llm_grpc.go`）⇒ 检索拿不到"是谁"。**必须先补**（Phase 4 的前置），已登账 `CL-F-56` |
| R2 | **LLM 成本与延迟** | 每条消息多轮 LLM 调用（抽取/消解/去重/矛盾判定）。对策：全异步 + 独立 group + 成本指标；**抽取粒度**待裁（§7 N3） |
| R3 | **中文抽取质量** | 指代消解（"我妈"/"母亲"）、口语、方言都可能失败。对策：T-6 评测集 + 人工抽检；**不许用"演示两次通过"代替评测** |
| R4 | **删除的完整性** | 图上删实体要连带处理边与社区摘要；漏一处 = "删了还在用"。对策：T-16 用**删除后提问**证明，而不是只看 DB 行没了 |
| R5 | **内存红线** | Kuzu 是嵌入式（无容器），但 Python 进程会变胖；**禁止顺手加容器**（CL-01 范围 Out 的口径） |
| R6 | **依赖面扩大** | 新增 `graphiti-core`/`kuzu` ⇒ Dockerfile 体积、CI `llm-test.yml`、依赖锁定都要动；Phase 0 先量体积 |
| R7 | **"埋了没人看"** | 记忆指标若不上面板，等于 CL-01 抓过的老病重演（T-18 兜住） |
| R8 | **图不是事实源** | 图必须可重建：任何"只能从图里查到、PG 里没有"的数据都是设计错误（ADR 决策 ③）。⇒ Phase 5 要交付**重建工具**，且 Phase 0 实测重建耗时（不成立则这条前提要改） |
| R9 | **双写不一致**（ADR 决策 8 的代价） | PG 写成功而图写失败（或反之）⇒ "管理页看得见、AI 却答不出"这类半状态。对策：以 PG 为准、图失败**不阻塞**（进重试，图侧幂等 upsert）；并给"PG 有图无"的**对账/重建**兜底 |
| R10 | **Kuzu 并发**（待实测） | 嵌入式库典型单写者 ⇒ 若 llm-service 多 worker 会锁冲突。对策：Phase 0 实测（T-d）；暂定单 worker + 内部串行 |

---

## §7 决议与仍开放项

### 7.1 技术实现决议（2026-10-18）

> **来源标注**：「**用户**」= 用户当日经 AskUserQuestion 逐条拍板；「执行侧」= 执行侧按实读给出的口径，
> **若不同意可在进入阶段时推翻**（这些不是用户背书的决定，别当既定事实读）。
> 详细展开见 ADR [`adr-2026-10-agent-memory-graph.md`](../../../architecture/adr/adr-2026-10-agent-memory-graph.md) §技术细节决议。

| # | 事项 | 决议 | 来源 |
|---|---|---|---|
| T1 | **进程拓扑** | **ai-svc 消费 Kafka（独立 consumer group）+ llm-service 持图**，两者走新增 gRPC `MemoryService`；不新增容器 | **用户** |
| T2 | **PG 与图** | **PG 为系统记录 + 图可重建**（⇒ 双写 + 需重建工具） | **用户** |
| T3 | **注入读路径** | **Redis 用户级摘要**（`CL-D-07` 同构，TTL + 取不到降级）；不做 Redis+PG 双读 | **用户** |
| T4 | **跨进程契约** | **新增 `proto/memory.proto`**（不塞进 `emotion_llm.proto`） | **用户** |
| T5 | 记忆段位置 | `base → agent → personality → **memory** → emotion → guard`（guard 恒末位；**记忆为空则不产生该段** ⇒ 既有逐字 golden 不动） | 执行侧 |
| T6 | 记忆表 schema | **`emotion_echo_ai`**（写入方 = ai-svc；守卫禁止跨 schema） | 执行侧（由 T1 推导） |
| T7 | 记忆 token 预算 | 记忆段独立 **≤600**（与 history 的 2000 分开记账）；超限按"最近 + 最相关"截断 | 执行侧 |
| T8 | 抽取模型 | 沿用现有 `LLM_*`（OpenAI 兼容端点；Graphiti `openai_generic_client` 适配）；**成本上限等 Phase 0 实测数再定** | 执行侧 |
| T9 | 顺序与幂等 | **不改 topic 分区键**（同会话已天然有序）；跨会话用 `send_time` 兜底；幂等键 = `message_id` | 执行侧（实读依据） |
| T10 | 测试替身 | 单测注入**假 LLM / 假 embedder**（接口可继承），不打真 LLM | 执行侧（实读依据） |
| T11 | Kuzu 并发 | 暂定 llm-service **单 worker + 内部串行**；Phase 0 实测后定（可能改按用户分片文件） | 执行侧（待实测） |

### 7.2 仍未定（属**产品口径**，不是技术实现；执行侧不自拟）

| # | 事项 | 为什么仍需用户定 | 影响 |
|---|---|---|---|
| N1 | **阶段范围切分**：CL-08 一次做完 a~e，还是先内核（a 用户事实 + c 情节/图检索 + d 管理页）把 b（情绪模式）与 e（风险信号）留后？ | 详档的 21 个测试点实际只覆盖 a/c/d（b、e 只有标题、**没有测试点**）⇒ 这个范围差是事实，得用户定；且 e 是安全项（失败代价最高） | 决定本阶段周期与 PR 切分 |
| N2 | **抽取粒度**：每条 user 消息 / 会话收尾汇总 / 混合 | 直接决定 LLM 成本与"当轮能否用上"（时效） | 决定 Phase 2 的触发设计与成本 |
| N3 | **删除语义**：PG 软删 + 图硬删 / 两边硬删 / 两边软删 | 是**对用户的隐私承诺**（"删了就是删了"到哪一步） | 决定 Phase 5 实现与文案 |
| N4 | **管理页形态**：新侧边栏标签 + 列表 + 关系图 / 只列表 / 并入「我的空间」 | 用户可见 + 演示价值（T-21 `[M]`） | 决定 Phase 5 的前端工作量 |
| N5 | **用户能否关闭记忆**（默认建议给开关、默认开；关闭 = 停抽取 + 停注入 + 保留已有） | 隐私开关属产品口径 | 影响 `users.config` 与设置页 |

---

## §8 调研依据（本次实读）

- **代码/配置（14 处）**：`prompt_assembly.go`、`prompt_templates.go`、`prompt_history.go`、`ai_stream_handler.go`、`downstream/llm_grpc.go`、`proto/emotion_llm.proto`、`ai-svc/internal/logic/consumehandler.go`、`ai-svc/internal/consumer/{consumer,consumer_failure}.go`、`chat-svc/migrations/{c001,c010}*.sql`、`chat-svc/internal/outbox/relay.go`、`llm-service/{requirements.txt,chat_completion.py}`、`deploy/docker-compose.apps.yml`、`deploy/db/{migrate.sh,README.md}`、`scripts/test_migrations_no_service_order.sh`（清单见 §A.1）。
- **决策/ADR**：`CL-D-01` / `CL-D-07` / `CL-D-24` / `CL-D-31` / `CL-D-33` / **`CL-D-35`~`37`**；ADR 决策 39（内存红线）、决策 40（SiliconFlow 纪律）、决策 45（本文形态）、`adr-2026-09-incremental-rpc-adoption`。
- **外部信息（WebFetch 实读，2026-10-18）**：Zep/Graphiti 论文 <https://arxiv.org/abs/2501.13956>；Graphiti 驱动目录 <https://api.github.com/repos/getzep/graphiti/contents/graphiti_core/driver>；Kuzu 元信息 <https://api.github.com/repos/kuzudb/kuzu>；硅基流动嵌入端点 <https://docs.siliconflow.cn/cn/docs/api/embeddings-post>。
- **smoke**：本轮未跑（纯文档轮，无代码可验；见 §A.4）。
