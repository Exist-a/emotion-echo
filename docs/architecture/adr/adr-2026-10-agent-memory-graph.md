# ADR-2026-10: 跨会话记忆 = 时序知识图谱（Graphiti + 嵌入式 Kuzu / 硅基流动嵌入），Postgres 为系统记录

## Status

🟡 **Proposed（形态由用户 2026-10-18 拍板；三项待实测项未证 ⇒ 转 Accepted 前必须先回填实测结论）**

- 范围 = **形态与选型**（本文）。**实施未开始**：CL-08 的执行计划见
  [`docs/closure-roadmap/stages/cl-08-memory/plan.md`](../../closure-roadmap/stages/cl-08-memory/plan.md)。
- 用户 2026-10-18 口径：「**我个人是想引入的**」「**硅基流动有向量化模型服务吗？有就直接复用**」
  「**先落文档**」⇒ 本轮只落文档、不动代码。
- 相关决策：`CL-D-35`（记忆形态）/ `CL-D-36`（embedding 供给）/ `CL-D-37`（本轮只提前改规划）
  见 [`docs/closure-roadmap/decisions.md`](../../closure-roadmap/decisions.md)。

## Context

### 1. 现状：记忆是**零实现**（不是"做得不好"）

| 事实 | 证据 |
|---|---|
| 全仓无任何记忆/向量/图设施 | `grep -rIl "memory_fact\|user_memory\|episodic\|graphrag"` 命中 0；无 `pgvector`、无向量列、无图库 |
| 唯一叫 "summary" 的东西不是记忆 | `emotion_echo_chat.msg_summary_v` 是按天统计消息条数的**报表视图**（`emotion-echo-chat-svc/migrations/c005_refresh_msg_summary_view.sql:7`），无 LLM 生成、无记忆语义 |
| `.env.local` 里只有 `LLM_*` 与 `TTS_*` | 无任何 embedding key（`deploy/env/.env.local.example` 同） |
| 跨会话记忆"做了没做"从来没被钉住 | `docs/plans/conversation-memory-pending-decision-2026-09-24.md` §B：`ai_stream_handler_test.go` 5 个测试**无一断言历史注入** |

### 2. 已就位的**可复用**骨架（本决策要挂上去的地方）

| 能力 | 位置 | 为什么重要 |
|---|---|---|
| 段式 prompt 组装层 | `emotion-echo-web-bff/internal/handler/prompt_assembly.go:29-91`：段序 `base → agent → personality → emotion → guard`，**空段不产生**（`:40-44` 注释 + `:157-166` 双保险） | 新增 `memory` 段时，**没有记忆就不产生该段** ⇒ 既有"逐字零回归"golden 自动保持绿 |
| 逐字零回归钉 | `prompt_assembly_test.go`（`TestPromptAssembly_ZeroRegression_GoldenBytes`）、`prompt_assembly_agent_test.go:101-114` | 新段的红线判据现成 |
| 异步可靠链路 | outbox（`emotion-echo-chat-svc/migrations/c001_create_outbox_events.sql:31`，含 `trace_sw8`）→ Kafka `chat-events` → 消费端 | 抽取走这条链路，**不需要新造队列** |
| "消费者里调 LLM"的现成模板 | `emotion-echo-ai-svc/internal/logic/consumehandler.go:45`（过滤 → 调 LLM → 落库，`EventID` 幂等去重 `:79-87`） | 抽取 worker 照此形态写 |
| Python 宿主 | `emotion-llm-service/`（FastAPI :8000 + gRPC :50051，已有 LLM 客户端 `chat_completion.py`、Nacos、metrics、pytest） | Graphiti 是 Python 库，宿主现成 |

### 3. 为什么**不能**只做"向量 RAG"

陪伴类场景的三个语义要求，向量 RAG 一条都答不了：

1. **事实会变，旧话必须失效**——"我住北京" → "我上个月搬到上海了"。向量库把这两句当两段相似文本，**会把旧事实当当前事实用**（这也是 Zep 论文的出发点，见 §调研依据）。
2. **来源可溯**——用户问"你怎么知道我住哪"，要能指回那条消息；`CL-D-24` 已定"明文存储 + 软删除 + 管理页可删"，**可删**要求知道每块记忆从哪来。
3. **关系可跳**——"我最近压力大吗"需要顺着 `用户 → 母亲 → 住院` 跳，而不是找"最像的段落"。

### 4. 硬约束（选型必须满足）

| 约束 | 依据 |
|---|---|
| **默认不新增容器** | `docs/closure-roadmap/roadmap.md`（CL-01 范围 Out：「默认不新增容器（内存红线）」）；`.wslconfig` **8GB** + 19 容器稳态 **~6G** ⇒ 有击穿前科（`docs/architecture/decisions.md:999`，决策 39） |
| 抽取**不得在关键路径** | `CL-D-33` 实测：首字路径多一次 LLM 调用 ⇒ TTFT **940ms → 1362ms（+45%）** |
| 现有容器内存限额 | `deploy/docker-compose.apps.yml`：除 `sensevoice`/`xtts` 各 **6144M** 外全是 **1024M** |
| 每服务只碰自己的 schema | `scripts/test_migrations_no_service_order.sh`；`deploy/db/README.md:116-125`（跨 schema JOIN 禁止，走 gRPC/Kafka） |

## Decision

**跨会话记忆 = 时序知识图谱**：实体 / 关系 / 事实 + **双时间轴**（`t_valid`/`t_invalid` = 事实何时为真；`t'_created`/`t'_expired` = 我们何时知道），检索 = 语义向量 + 全文 + 图遍历三层，注入 = system 段。

1. **图后端 = Kuzu 嵌入式**（`graphiti_core/driver/kuzu_driver.py` 实存）。
   Kuzu 是 MIT 许可的**嵌入式属性图库**（自述 *"Embedded property graph database built for speed. Vector search and full-text search built in. Implements Cypher."*），
   随 Python 进程加载、数据落本地目录 ⇒ **新增容器数 = 0**，满足 §4 约束①。
   Graphiti 支持的后端实测清单（仓库目录实读）：`neo4j` / `falkordb` / **`kuzu`** / `neptune`。
2. **embedding = 硅基流动 `POST https://api.siliconflow.cn/v1/embeddings`，模型 `BAAI/bge-m3`（1024 维）**。
   与现有 TTS **同账户同域名**（`deploy/env/.env.local.example:39` 已有 `TTS_API_BASE_URL=https://api.siliconflow.cn/v1`）
   ⇒ **不新增 key、不新增容器**（满足约束①）。
   维度 1024 同时对齐 Graphiti 实体消解所用维度。
3. **Postgres 与图的分工（关键）**：
   - **PG = 系统记录（system of record）**：episode 对消息的引用、抽取审计、**删除与合规**、管理页读取。
   - **图 = 检索索引（可重建）**：丢了能从 PG 重建，因此**不把图当唯一事实源**。
   - ⇒ **删除以 PG 为准，图随删**（`CL-08d` 的硬要求因此有解）。
4. **关键路径不查图**：注入段读的是"**用户级记忆摘要**"（预计算，落 PG/Redis），**取不到就降级为空**——
   沿用 `CL-D-07`（融合情绪：Redis 用户级最新、取不到降级）的既有先例。图查询发生在**异步侧**（抽取 worker）。
5. **抽取必须异步**：复用 outbox → Kafka 消费形态，**独立 consumer group**（不与情绪分析共用，避免一个失败阻塞另一个）；
   幂等键 = `message_id` 唯一索引；失败走既有重试 / DLQ（约束②）。
6. **禁止在业务代码里直写供应商域名/模型名**（经配置注入）——沿用决策 40 对 SiliconFlow TTS 的同一条纪律；
   `grep -rn "siliconflow" --include=*.go` 只允许命中 config 默认值与测试。

### 被否备选（记录理由，防"下次再来一遍"）

| 备选 | 否决理由 |
|---|---|
| **Microsoft GraphRAG** | 面向**批量语料**的全局问答：批量索引 + 社区摘要，增量更新支持有限、重建代价高。我们的输入是**每天新增的对话流**，方向相反 |
| **LightRAG** | 成本与增量友好（双层检索），但重心仍是"检索"，**不解决事实时效/取代**——而这恰是陪伴类最需要的 |
| **先 PG 三表自建、以后再引图** | 会把"事件源/审计/删除"这套无论如何都要做的工作**做两遍**；且在没有评测前无法判断"多跳/时序失效"是否是真瓶颈 |
| **Graphiti + Neo4j 容器** | 图查询能力最完整，但要新增一个 JVM 容器（通常 1~2GB）⇒ 直接撞约束①与决策 39 的击穿前科 |
| **把 embedding 做成自建容器**（bge-m3 本地推理） | 数据不出境是优点，但 +约 1GB 内存，与 `sensevoice`/`xtts`（各 6144M 限额）抢同一份预算 |

## 待实测项（**未证 —— 不得当作已成立**；Phase 0 逐条回填）

| # | 待实测 | 为什么必须先实测 | 回填位置 |
|---|---|---|---|
| T-a | 现有 `TTS_API_KEY` 在该账户下**能否调 `/v1/embeddings`**、配额是否与 TTS 共用、限流、单次批量上限、`bge-m3` 实际维度 | 官方文档证实"有服务、有这些模型"，但**账户权限与配额是另一回事**；共用 key 可能互相挤占（TTS 是主链路） | 本表 + `plan.md` T-0x |
| T-b | `graphiti-core` + `kuzu` 与 llm-service **现有依赖**（`grpcio>=1.60` / `openai>=1.40` / `pydantic>=2` / `pypdf` / `python-docx`，Python 3.12）是否冲突；镜像体积增多少 | 装不上=方案不成立；装得上但体积暴涨要重估 | 本表 + `plan.md` |
| T-c | Kuzu 数据文件的**持久化与备份**（容器重建不丢）；与 `deploy/backups/`（pg_dump 产物）的关系 | 嵌入式库的"文件在哪、谁备份"是它唯一的运维面；不实测就会出现"重建容器记忆全丢" | 本表 + `plan.md` |

> ⚠️ **本 ADR 的 Accepted 转换条件** = 上述三项均有实测结论（含命令与输出），而非"文档说支持"。

## Consequences

**正**
- 新增容器 0 / 新增 key 0 ⇒ 不动内存红线，也不动 digest 固定与 env 模板那套门禁面（除新增 embedding 配置项本身）。
- 事实时效、来源可溯、关系遍历三条语义一次到位，直接支撑 `CL-08b`（情绪模式 = 图上的关系/时序查询）与 `CL-08c`（情节 + 图检索）。
- 图是**可重建索引** ⇒ 出问题可 drop 重建，不承担"唯一事实源"的风险。

**负 / 代价（如实记）**
- **新增一个 Python 依赖面**（graphiti-core + kuzu）：CI 的 `llm-test.yml`、Dockerfile 体积、依赖锁定都要跟着动。
- **每轮对话多出若干次 LLM 调用**（实体抽取 / 消解 / 事实去重 / 矛盾判定）⇒ 成本与延迟都在异步侧，但**不再是零成本功能**；必须配指标 + 面板 + 告警（对齐 CL-01「埋点必须被消费」的口径）。
- **中文抽取质量未证**：指代消解（"我妈" ↔ "母亲"）、口语与方言都是真实风险，必须靠**评测集**说话（`plan.md` Phase 6），不能凭演示一两次通过就宣布成功。
- **删除的完整性需要在图上验证**（删实体要连带处理边与社区摘要），这是 `CL-08d` 的必测项。

**中立 / 遗留缺口（本轮核出，须先补）**
- **BFF 当前不往 llm-service 传 `user_id`**（`emotion-echo-web-bff/internal/downstream/llm_grpc.go` 的 pb 构造无该字段；proto `emotion_llm.proto` 有 `user_id` 但无人填）⇒ 若把记忆检索放 llm-service，**它拿不到"这是谁"**。已登账本（见 `discovered-unresolved.md`）。

## 调研依据（本次实读）

**代码 / 配置（实读文件）**
- `emotion-echo-web-bff/internal/handler/prompt_assembly.go:29-91,157-166`（段序与空段丢弃）、`prompt_assembly_agent_test.go`（零回归钉形态）
- `emotion-echo-ai-svc/internal/logic/consumehandler.go:45-97`（异步 LLM 消费者模板）、`internal/consumer/consumer.go`（重试/DLQ）
- `emotion-echo-chat-svc/migrations/c001_create_outbox_events.sql:31`、`c010_add_trace_sw8_to_outbox.sql`、`internal/outbox/relay.go`
- `emotion-llm-service/requirements.txt`、`chat_completion.py`（LLM 客户端与 mock 降级）
- `deploy/docker-compose.apps.yml`（内存限额：6144M×2 / 1024M×11）、`deploy/env/.env.local.example:39`
- `docs/architecture/decisions.md:999`（决策 39：8GB WSL + 19 容器 ~6G 击穿前科）
- `docs/closure-roadmap/{roadmap,decisions}.md`（CL-08a~e 行、`CL-D-01`/`CL-D-24`/`CL-D-33`）

**决策 / ADR**
- `CL-D-01`（跨会话 = RAG，形态未定 → 本文收敛为图记忆）、`CL-D-24`（明文 + 软删 + 管理页可删）、`CL-D-07`（Redis 用户级最新、取不到降级）、`CL-D-33`（TTFT +45% 实测）
- `adr-2026-10-tts-api-cosyvoice2.md`（SiliconFlow 供应商纪律：配置注入、禁硬编码域名）

**外部信息（WebFetch 实读，2026-10-18）**
- Zep/Graphiti 架构论文（双时间轴、三层子图、摄入流水线与检索三阶段）：<https://arxiv.org/abs/2501.13956>
- Graphiti 驱动目录实读（`neo4j` / `falkordb` / `kuzu` / `neptune`）：<https://api.github.com/repos/getzep/graphiti/contents/graphiti_core/driver>
- Kuzu 仓库元信息（MIT / 嵌入式属性图库 / 自带向量与全文检索）：<https://api.github.com/repos/kuzudb/kuzu>
- 硅基流动嵌入端点与模型清单（`POST /v1/embeddings`，含 `BAAI/bge-m3`）：<https://docs.siliconflow.cn/cn/docs/api/embeddings-post>
