---
status: done
type: spike-report
stage: CL-08c
phase: 0
created: 2026-10-22
related:
  - plan.md（§3 Phase 0）
  - ../../../architecture/adr/adr-2026-10-agent-memory-graph.md（§待实测项）
  - ../../discovered-unresolved.md（CL-F-57 / 58 / 71）
  - evidence/phase0-t-a-embeddings-2026-10-22.txt
  - evidence/phase0-t-b-deps-2026-10-22.txt
  - evidence/phase0-t-b2-compat-2026-10-22.txt
  - evidence/phase0-t-c-kuzu-persistence-2026-10-22.txt
  - evidence/phase0-t-d-kuzu-concurrency-timing-2026-10-22.txt
---

# CL-08c Phase 0 · Spike 报告（2026-10-22）

> **做法**：全部在**一次性容器**内完成（`python:3.12-slim` + 命名卷），**未取 dev 栈锁、未改动在跑的任何容器**。
> 每条结论都有命令 + 输出，见 `evidence/phase0-*.txt`。**本阶段零生产代码改动。**

## §1 结论摘要

| # | 项 | 结果 | 一句话 |
|---|---|---|---|
| T-a | embedding 端点 | ✅ **PASS** | `TTS_API_KEY` 可调 `bge-m3`，1024 维，单条 417ms，批量 64 条 972ms |
| T-b | 依赖兼容 | ✅ **PASS** | graphiti 0.31.2 + kuzu 0.11.3 装得上；**llm-service 215 单测全绿**；+161MB |
| T-c | 持久化/备份 | ✅ **PASS** | 命名卷跨容器重建不丢；单文件 1.1MB/3 节点；bind mount 亦可 |
| T-d | 并发 + 重建 | ✅ **PASS** | **严格单写者**（第二个写进程 fail fast）；COPY 20k 行 0.31s |
| **T-e** | **选型可持续性** | 🔴 **需裁决** | **graphiti 已把 Kuzu 标 deprecated（上游停更）** |

## §2 关键数据（可直接用于 Phase 1~4 的设计）

### 2.1 Embedding（T-a）
- 端点：`https://api.siliconflow.cn/v1/embeddings`，模型 `BAAI/bge-m3`，**1024 维**
- 延迟：单条 417ms / 2 条 394ms / 8 条 459ms / 32 条 691ms / **64 条 972ms**（15.2ms/条）
- 长文本：≈840 字（483 tokens）正常
- 记账：`usage.prompt_tokens` 可用 ⇒ T-18 成本指标有抓手
- ⚠️ **未证**：配额是否与 TTS 共用、QPS 阈值（单次测试证不了）⇒ 上线后按 429/延迟观察

### 2.2 依赖（T-b）
- `graphiti-core==0.31.2`（`kuzu` 是 **extra**，必须写 `graphiti-core[kuzu]`）
- 真实 requirements 下 `pip check` 无冲突；`openai` 被解析到 **3.36.0**（llm-service 只约束 `>=1.40.0`）
- **llm-service 单测 215 passed in 15.80s**（openai 1.x→3.x 无回归；导入 `chat_completion/intent_llm/grpc_server/main` 均 ok）
- 体积：site-packages **+161MB**（对照：llm-service 现镜像 394 MiB）

### 2.3 持久化（T-c）
- 命名卷（推荐，与 `postgres_data` 同构）→ `/var/lib/docker/volumes/<name>/_data`
- **单文件** `graph.kuzu`：3 节点 = 1.1MB；40k 节点 ≈ 9.5MB（≈0.24 KB/行）
- 删容器 → 新容器读回 3 行 ✅；Windows bind mount 也可用（但并发语义未测）

### 2.4 并发与重建（T-d）
| 场景 | 结果 |
|---|---|
| 第二个写进程（跨容器） | `RuntimeError: IO exception: Could not set lock on file` → **fail fast** |
| 第二个写进程（同容器多进程） | 同上 ⇒ **多 worker 会在启动期就炸** |
| 只读并发打开 | 可打开，但**读到了未提交行** ⇒ 不保证隔离，不可作设计依据 |
| 逐条插入 | **5.03 ms/行**（2000 与 20000 完全线性）⇒ 100 万行 ≈ 84 分钟 |
| COPY FROM CSV | **20k 行 0.31s ≈ 65k 行/s**（比逐条快 ~325×） |

## §3 对后续 Phase 的直接影响

1. **T11 定稿**：llm-service **单 worker + 内部串行**持图；不分片。任何"多 worker"部署改动都会启动失败（好的一面：不会静默写坏）。
2. **重建工具必须走 COPY**：Phase 5 的"从 PG 重建图"要先导出 CSV 再 `COPY`，禁止逐条 `execute`。
3. **Dockerfile 写法**：`pip install "graphiti-core[kuzu]"`（少了 `[kuzu]` 运行期才炸）；`openai` 建议显式钉一个下界 `>=1.91,<2` 或直接跟随解析结果并加回归测试。
4. **注入路径不动**：读走 Redis 摘要（T3 决议），**关键路径不查图** ⇒ T-d 的只读并发缺陷不构成风险。

## §4 未决（阻塞 Phase 4，不阻塞 Phase 1~3）

**T-e：图后端选型需用户重裁。** 四项技术实测都过了，但 graphiti 0.31.2 明确警告 Kuzu backend 将移除、上游停更。
四选一见 ADR §待实测项 T-e：

| 选项 | 代价 |
|---|---|
| ① 锁 `graphiti-core==0.31.2` 继续 Kuzu | 0 迁移、0 容器；但永不升级 + 上游停更 |
| ② `graphiti-core[falkordblite]` | 仍 0 容器；Beta 0.11.0、内存未测 ⇒ 需再 spike |
| ③ FalkorDB / Neo4j 容器 | 官方推荐但**撞内存红线** |
| ④ 自研 PG 图存储 | 0 新依赖；工作量最大 |

> Phase 1（PG 数据模型）/ Phase 2（抽取 worker）/ Phase 3（BFF 注入段）**与图后端无关**，可先行。

## §5 未做 / 局限（如实声明）

- 未测「从 PG 全量导出 → 建图」的端到端耗时（重建工具 Phase 5 才交付）；只测了 Kuzu 侧 ingest（主导项）。
- 未测 edges / 全文索引 / 向量索引的构建耗时（只测了节点插入）。
- 未测 embedding 配额归属与 QPS 阈值。
- 未验证 falkordblite 的可行性（若 T-e 选 ②，需另跑一次 spike）。
- 未在真实 llm-service 镜像里构建（只在一次性容器里验证依赖树）——避免动共享镜像。
