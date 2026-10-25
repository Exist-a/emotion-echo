---
status: done
type: phase-report
stage: CL-08d
created: 2026-10-25
related:
  - plan.md（§3 Phase 5 = CL-08d / §2 T-16、T-21）
  - phase4-graph-report-2026-10-25.md（图链路来源）
  - cl08d-t16-runtime-2026-10-25.txt（T-16 全流程原始输出）
  - cl08c-t5-rerun-2026-10-25.txt（第二方核对 P1 整改：T-5 重放原始输出）
  - cl08c-phase4-raw-2026-10-25.txt（第二方核对 P2 整改：Phase 4 原始输出补档）
  - evidence/iab-cl08d-memory-list-delete-2026-10-25.png
---

# CL-08d · 删除闭环 + 管理页 + 重建工具 报告（2026-10-25）

> **落地载体**：阶段 PR [#284](https://github.com/Exist-a/emotion-echo/pull/284)；登记 PR #285。
> **环境**：dev 栈 18 容器（`:cl08d` × 4；图 gRPC 独立 :50052）。

---

## §1 交付物

| 层 | 交付 |
|---|---|
| proto | `memory.proto` +`ListMemory`/`DeleteMemory`（ai-svc↔BFF 面）；`memory_graph.proto` +`DeleteUserData`/`RemoveEpisode` +`episode_uuid` 回传（图写入面）；生成物重生成；rpc_status +3（守卫 PASS，锚点修正后） |
| llm-service | remove 分图路由（`with_database(str(user_id))`——graphiti 按 group_id 物理分图，默认 database 恒空）+ 孤儿实体清扫（上游 remove 留 0-MENTIONS 节点）+ `delete_user_data`（GRAPH.DELETE 快删，图不存在幂等成功）+ AsyncBridge 复用；单测 240 |
| ai-svc | i013（memory_episode.graph_uuid）；repo 软删面（fact/note/all/uuid，InMemory 与 Postgres 双实现）；`DeleteMemory` 编排（**PG 即刻生效 → 图侧/摘要异步收敛**——图慢操作同步做必撞 BFF 5s 截止，运行时实测后改正）；`memory.RebuildUserGraph`；`cmd/memory-rebuild`（-user/-all） |
| BFF | GET /memory/items + DELETE /memory/facts/:id│notes/:id│memory + client + handler 契约测试 |
| 前端 | `MemoryList.vue`（列表 + 两段式确认删除，无原生 confirm）+ memory.vue 接线 + vitest 6 例（全仓 104 文件绿） |

**删除语义**（执行侧落定并声明）：删单条记忆 = 软删该 fact + 其来源情节行（"忘掉这件事"；同源其他 fact 保留，其图上内容被 remove 一并摘除 —— 已知边界）；PG 恒软删（审计可溯），图恒物理删（D3）；图侧异步收敛，失败只记日志（PG 为事实源、重建工具兜底）。

## §2 运行时调试战果（三个真问题，全部修复 + 钉）

1. **分图路由缺失**：graphiti `remove_episode` 沿用 driver 默认 database（'memory' 恒空）⇒ 恒 "node not found"。修：`with_database(str(user_id))` 路由 + 字面量钉。
2. **上游孤儿实体**：falkordblite 0.11 的 remove 留下 0-MENTIONS 实体（受控实验实证）⇒ 删除后清扫 + 钉。登账 `CL-F-74`。
3. **同步图操作撞 5s 截止**：图摘除/重建是 LLM 级慢操作（单条秒级、全图重建 ~40s），同步做 ⇒ BFF 502 + ctx 取消打断图/摘要（原始日志留档）。修：异步收敛（同 forward 范式），摘要刷新用独立 ctx。

## §3 测试点结论

| # | 测试点 | 判定 | 证据 |
|---|---|---|---|
| T-16 | 删除闭环 | **PASS** | `cl08d-t16-runtime`：删单条（uuid 路径：列表/PG/图三面消失 + 孤儿清扫）+ 删全部（PG 全软删 + 物理分图 drop → `graphs=[]` + 摘要键失效）；uuid 缺失回落全图重建有单测钉 |
| T-21 | 管理页形态/确认文案 | **[M] 待用户裁定** | 默认版已落地（两段式确认："删除"→"确认？"→执行；IAB 实测 52→51 + ack）；关闭记忆开关（N5）未做——两者均待用户过目 |
| 重建工具 | ADR 决策 8 配套 | **PASS** | 容器内实测：先弄乱图（种垃圾）→ `-user 1` 重建 → 图恰好 = PG 12 条 active episodes（uuid 全换新、边重建 6+16、垃圾清除） |
| T-19 复验 | 评测 | **PASS=10 FAIL=0**（:cl08d 栈重跑，原始输出 `cl08c-phase4-raw`） | 事实落图/召回/更新/A-B 双向 |

## §4 第二方核对整改（CL-08c，随本 PR 一并落地）

| # | 整改 | 状态 |
|---|---|---|
| P1 | T-5 A/B 证据未留档 ⇒ **重放归档**（`cl08c-t5-rerun-2026-10-25.txt`，base/down/up 全 PASS 原始输出）+ Phase 3 报告引用更正 | ✅ |
| P2 | Phase 4 证据为手写转写 ⇒ 原始输出补档（`cl08c-phase4-raw-2026-10-25.txt`：T-19 评测原文 + 受控二分 + graph list） | ✅ |
| P2 | 图服务独立 gRPC server :50052（独立线程池，不与 chat 抢 worker）+ 字面量钉 | ✅ |
| P2 | forward goroutine 补 recover | ✅ |
| P2 | 黑洞测试收紧 <900ms | ✅ |
| P2 | IAB 截图拍的是加载态 ⇒ 重拍（`iab-cl08d-memory-list-delete`，图+列表同框） | ✅ |
| P2 | roadmap CL-08c 行 / CL-F-70 注同步 | ✅（随本 PR 文档） |
| P2 | grpc.WithTimeout 废弃参数（no-op、无害）| 记录不修（调用点均自设 ctx deadline） |

## §5 未做 / 下一步

- **T-21 [M]**：管理页文案/关闭记忆开关（N5）待用户裁定（默认版已可用的前提下不阻塞合并）。
- T-6 中文同指消解专项评测（R3 风险）——归评测轮常备。
- CL-08c/CL-08d 的最终 done 由用户/第二方裁定。
