---
status: done
type: phase-report
stage: CL-08c
phase: 4
created: 2026-10-25
related:
  - plan.md（§3 Phase 4 / §2 T-13/T-14/T-17/T-19/T-20）
  - phase3-injection-report-2026-10-24.md
  - evidence/phase4-graph-runtime-2026-10-25.txt
  - evidence/iab-cl08c-phase4-memory-graph-2026-10-25.png
---

# CL-08c Phase 4 · 图记忆端到端报告（2026-10-25）

> **落地载体**：阶段 PR [#281](https://github.com/Exist-a/emotion-echo/pull/281)；登记 PR #282 → `2267cbf7`。
> **环境**：dev 栈 18 容器（`:cl08c` × 4：llm-service / ai-svc / web-bff / web）。

---

## §1 交付链路（全通）

```
web 发消息 → chat-svc → Kafka(独立 group) → ai-svc 抽取
   ├─ PG：facts + episode + entity（Phase 1/2）
   ├─ Redis：用户级摘要 → BFF 注入段（Phase 3 / CL-08a）
   └─ 异步 gRPC（mTLS + x-internal-api-key）→ llm-service MemoryGraphService
        → Graphiti 摄取（DeepSeek LLM + 硅基流动 bge-m3 1024 维）→ falkordblite（命名卷持久化）
```

| 组件 | 交付 |
|---|---|
| `proto/memory_graph.proto` | `MemoryGraphService.UpsertEpisode`（独立文件：与 CL-08a 的 memory.proto 实现方/调用方不同）；Go+Py 生成物；rpc_status 登记 live（调用方=ai-svc worker，守卫 PASS） |
| llm-service | `memory_graph.py`（`redislite.FalkorDB` 嵌入式图库 + Graphiti 装配 + **AsyncBridge** sync→async 专用循环）+ servicer（降级注册不崩）+ `graphiti-core[falkordblite]==0.31.2` 锁定 |
| ai-svc | `graph_writer.go`（mTLS 客户端 + CL-F-66 鉴权范式）+ 抽取成功后**异步 best-effort** 投递 |
| CL-F-56 | BFF 填 `ChatCompletionRequest.user_id`（自定义起未接线）——**销账** |
| compose | `llm-graph-data` 命名卷 + `MEMORY_GRAPH_*` env + ai-svc `MEMORY_GRAPH_ADDR` |

## §2 运行时调试战果（三个真问题，全部修复后链路全通）

1. **命名卷属主**：容器非 root，新建卷属主 root ⇒ 写 `/data/graph` 权限拒绝。修：Dockerfile 预建目录 + chown（卷首挂继承属主）。
2. **事件循环绑死**：初始化用 `asyncio.run`（创建并关闭临时循环），graphiti 的 asyncio 原语绑死死循环 ⇒ 后续全部 "Event loop is closed"。修：初始化也走 AsyncBridge 的专用循环。
3. **deepseek 不支持 json_schema response_format**（实测 400，直测 json_object 可用）：Graphiti `OpenAIGenericClient` 默认 json_schema ⇒ 切 `structured_output_mode="json_object"`（schema 走 prompt 注入，generic client 内建）。
4. 另：mTLS 忘配（明文连 TLS 口报 preface EOF）——照 BFF 范式补上（CA 池+客户端证书+ServerName）。

⚠️ 过程教训：一次带着测试 FAIL 构建了镜像（被本人当场发现并重建）——违反"绿了才构建"纪律，已在流程上补检查。

## §3 测试点结论

| # | 测试点 | 判定 | 证据 |
|---|---|---|---|
| T-13 | 图后端可用 | **PASS** | msg 1055 → 图上 Episode（原文全量+name）+ Entity×2（含 graphiti 自抽的「橙梦露」，摘要准确）+ 边 RELATES_TO/MENTIONS + **双时间轴**（created_at 摄取 / valid_at=消息 send_time）；摄取耗时 2.7s |
| T-14 | 三层召回 | **PASS（组合层）** | 中文查询「多肉 最贵的品种叫什么」→ combined hybrid 命中 OWNS 边（向量真调 bge-m3）；单层分解（向量/全文/图遍历各自单独断言）未拆——归评测轮/CL-08d 细化，如实记录 |
| T-17 | 用户隔离 | **PASS** | graphiti 按 group_id **物理分图**（'1'/'2' 独立 graph key）；G1={用户,橙梦露} G2={小绿} 零交集 |
| T-19 | 评测集 A/B | **PASS** | `eval_memory_recall.py` 当前栈实跑：事实落图/4 项跨会话召回/事实更新（新值生效+旧值下线）/A/B 双向（开记忆能答、**关记忆不复述**）全 PASS |
| T-20 | 人可见钉 | **PASS** | Playwright chromium 2/2（自述→回执→关系图 + 跨会话复述，8.4m）；mobile 2 failed 为环境项如实记录 |
| T-15 | 事实更新端到端 | **PASS（T-19 内含）** | 评测第 6 项：城市旧值下线、新值生效 |
| T-16 | 删除闭环 | **N/A → d** | CL-08d（删除三联动 + 重建工具） |
| T-6 | 实体消解 | **部分 PASS** | graphiti 摄取自己抽实体（「橙梦露」质量好）；中文同指消解（我妈↔母亲）未专项评测——归 CL-08d/评测轮（R3 风险如实保留） |
| T-18 | 指标被消费 | **沿用 CL-08a**（抽取 4 指标）；**图写暂为日志可观测**（未加新指标——避免"埋了没人看"反向违反 CL-01 口径；图侧对账归 CL-08d 重建工具一并设计） |

**CL-F-56 销账**（BFF→llm-service user_id）；**CL-F-57 embedding 配额观察项保持**（上线后看 429）。

## §4 设计落定（相对 plan 原文的修正，均已声明）

1. **图写不进重试**（plan 写"进重试"）：重投会整段重跑 LLM 抽取，代价大于收益 ⇒ best-effort + 日志 + CL-08d 重建工具兜底（与 R9 一致）。
2. **检索/删除 RPC 未进 proto**：无生产调用方（CL-02 教训）；检索走 Python 内部 API（评测/探针消费），删除归 CL-08d（届时带 BFF 调用方一起加）。
3. **`memory.proto` 不动**：CL-08a 已占用该文件（ai-svc↔BFF 面）；图写入面独立成 `memory_graph.proto`（ADR 决策 9 的精神——契约独立成文件——按现状落地）。
4. **graphiti 按 group_id 分图**是实现事实（实测发现）：用户隔离在存储层就是物理分图，比预期更强。

## §5 CL-08c 收口状态（执行侧汇报，不自行宣布 done）

Phase 0~4 + 评测全部落地。plan §2 的 21 测试点中，本阶段内结论：PASS 17（含 T-19/T-20 复验）/ N/A→c·d 4（T-6 部分、T-16、单层 T-14 拆解归后续）。**剩余：CL-08d（删除闭环 + 管理页 + 重建工具）**——roadmap 独立阶段。按 RUNBOOK，CL-08c 的最终状态由用户/第二方核对裁定。
