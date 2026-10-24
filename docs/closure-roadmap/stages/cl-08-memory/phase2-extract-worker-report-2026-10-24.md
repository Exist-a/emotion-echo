---
status: done
type: phase-report
stage: CL-08c
phase: 2
created: 2026-10-24
related:
  - plan.md（§3 Phase 2 / §2 T-4、T-5）
  - phase1-pg-model-report-2026-10-24.md（两表的来源）
  - evidence/phase2-t4-red-handler-2026-10-24.txt
  - evidence/phase2-t4-integration-pg-2026-10-24.txt
---

# CL-08c Phase 2 · 抽取 worker 报告（2026-10-24）

> **落地载体**：阶段 PR [#275](https://github.com/Exist-a/emotion-echo/pull/275)；登记 PR #276 → `3b2acab5`。
> **纪律**：未取 dev 栈锁（全程 testcontainers / 单测）；`main.go`（spine）无并发持有者。

---

## §1 范围（相对 plan.md §3 Phase 2 原文的落定）

plan 原文写「新增独立 consumer group + 调 llm-service 的 `MemoryService.UpsertEpisode`」。
实读后发现两处需按现状落定（均已在 PR 描述与本报告声明）：

1. **独立 consumer group 已存在**：CL-08a 已建「ai-svc-memory」group（`main.go` 装配 + `KAFKA_MEMORY_GROUP`），
   消费 `message.created` / `conversation.closed`，自带重试/DLQ/幂等门 ⇒ **不重复建**，Phase 2 是在这条链路上加产物。
2. **「调 llm-service MemoryService」归 Phase 4**：该 gRPC 服务连同 graphiti/图后端在 Phase 4 落地
   （roadmap CL-08c 注「Phase 1~3 与图后端无关」）；Phase 2 落的是 **PG 侧产物**（episode/entity），
   这正是 Phase 1 两表的写入方。

## §2 交付物

| 改动 | 内容 |
|---|---|
| `internal/logic/memory_extract_handler.go` | `WithGraphRepo` 可选依赖 + `persistGraphArtifacts`（1 episode + N entity）；插在 **ApplyFacts 之后、FinishExtractRun(done) 之前**——一旦标 done 该来源永久跳过，episode 就补不上了；失败 ⇒ finishFailed ⇒ 重投重抽，facts 侧 ApplyFacts 幂等 + episode/entity 侧唯一索引 ⇒ 重试收敛不重复 |
| `internal/logic/memory_extract_handler_graph_test.go` | 8 组契约：持久化 / 多主体+最新摘要 / **T-4 三次消费幂等（LLM 只调 1 次）** / episode 写失败·entity 写失败·恢复收敛（T-5 单测面）/ nil 零回归 / note 来源 / 空内容跳过 |
| `integration_test/memory_extract_graph_integration_test.go` | 真 PG 端到端：同一消息消费 3 次 ⇒ episode/entity/fact 各 1 行 + 运行行 done |
| `main.go` | `MEMORY_ENABLED` 路径注入 `PostgresMemoryGraphRepo`（CL-08a 装配钉不受影响，main 包测试全绿） |

**设计口径（执行侧，随实现声明）**：
- episode.name = 原文前 24 字符（rune 安全），空则 `<kind>-<id>`；content = 原文（图重建原料，ADR 决策 8）；valid_at = 事件时间（`occurredAt` 的 ms/s 自适应已由 CL-08a 修复）。
- entity 摘要 = 本批该 Subject 下 `ValidFrom` 最新一条的 `FactText`；同批写入顺序按 name 排序（重放结果确定）。
- 空内容（如全空白自述）跳过 episode，facts 照常——不因无可存档把整个抽取打成 failed。

## §3 测试点结论

| # | 测试点 | 判定 | 证据 |
|---|---|---|---|
| T-4 | 抽取幂等（同消息重复消费不重复） | **PASS** | 单测：Handle×3 ⇒ episode/entity 各 1 行 + `ex.calls==1`；真库：`phase2-t4-integration-pg` 三表 COUNT 各 1 |
| T-5 | 抽取失败不阻塞主链路 | **单测面 PASS / 运行时 A/B 待栈窗口** | 单测：episode/entity 写失败 ⇒ error 传播 + 运行行 failed + 恢复后重试收敛；消息路径与抽取异 group 异进程（CL-08a 结构性隔离）。**运行时 A/B（停 LLM ⇒ 发消息仍成功）需 dev 栈**：2026-10-24 栈仅剩 3 容器，未在本轮重建 ⇒ 按证据纪律**不宣称完成**，挂待栈窗口（与 Phase 4/收口轮共用一次栈窗口取证） |

全量：`go test ./... -count=1` ai-svc **17/17 包 ok**；`go vet` 干净；`-tags integration`（ryuk 禁用，CL-F-72）绿。

## §4 未做 / 下一步

- T-5 运行时 A/B + 面板指标（T-18 有一部分已在 CL-08a 埋）→ 与 Phase 4 运行时验收共用 dev 栈窗口。
- **Phase 3 = BFF 注入段**（`segmentKindMemory` + Redis 用户级摘要读取）：CL-08a 已做 Redis 摘要与 BFF 注入半场——开工前需先核对 CL-08a 已落范围，避免重复（与 Phase 1 同型的「范围校正」）。
- Phase 4 = 图 + `proto/memory.proto` + 补 CL-F-56（BFF→llm-service user_id）。
