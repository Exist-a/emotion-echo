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
| **T-e** | **选型可持续性** | ✅ **已裁决（② falkordblite）** | Kuzu 被 graphiti 标 deprecated ⇒ 换嵌入式 FalkorDB，仍 0 容器；追加实测通过 |

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

1. **T11 修订（falkordblite 后）**：不再要求"单 worker" —— falkordblite 多进程共享同一 server（实测 0.01s 接入）。
   但 llm-service 仍要**自己管理嵌入式 server 的生命周期**（无 `shutdown()`；靠 SIGTERM 落盘），且端口固定。
2. **重建工具**：Kuzu 时代的结论（COPY 65k 行/s）**仅作历史记录**；falkordblite 的重建路径（Cypher/批量导入）在 Phase 5 重新测。
3. **Dockerfile 写法（已按 T-e 更新）**：`pip install "graphiti-core[falkordblite]"`（少了 `[falkordblite]` 运行期才炸）；
   `openai` 建议钉 `>=1.91,<2`（graphiti 下界）或跟随解析结果并加回归测试。
4. **注入路径不动**：读走 Redis 摘要（T3 决议），**关键路径不查图**。

## §4 T-e：Kuzu 已弃用 → 用户裁决改用 falkordblite（2026-10-22 已闭环）

**发现**：graphiti 0.31.2 在 `KuzuDriver.__init__` 明确 `DeprecationWarning` —— Kuzu backend 将移除、**上游停更**。
用户当日裁决 **T-e = ② `graphiti-core[falkordblite]`**（嵌入式 FalkorDB，仍 0 容器），并追加实测：

| 项 | 结果 |
|---|---|
| 安装 | ✅ `pip check` 干净（graphiti-core 0.31.2 / falkordblite 0.11.0 / redis 8.1.0 / falkordb client 1.8.0） |
| 起服务 | ✅ 0.1~0.4s（`LiteFalkorDB(path, serverconfig={'port':'8002'})`） |
| graphiti 驱动 | ✅ `FalkorDriver(host, port, database)` 建索引 + 写 Entity + 读回；**不能**传同步实例（`await` 报错） |
| 并发 | ✅ 多进程**共享同一个 redis-server**（第二个进程 0.01s 接入，无锁）——比 Kuzu 的单写者宽松 |
| 持久化 | ✅ 容器 A 写 → 容器 B（全新）读回 `persist-check`；RDB 落 `falkordblite.db`（退出时写） |
| 内存 | ✅ ≈145MB（redis-server 26MB + Python 侧 120MB），llm-service 限额 1024M |

**已知边界（Phase 4 处理）**：falkordblite 仍 **Beta（0.11.0）**；客户端无 `shutdown()`（落盘依赖 SIGTERM，
`kill -9` 有丢最后窗口风险）；未验真实 `add_episode` 全链路与写入吞吐。

> ADR 决策 45 已把图后端改为 falkordblite 并转 **Accepted**；`T-d` 的 Kuzu 数据仅作历史记录（不再作为实施依据）。

## §5 未做 / 局限（如实声明）

- 未测「从 PG 全量导出 → 建图」的端到端耗时（重建工具 Phase 5 才交付）；只测了 Kuzu 侧 ingest（主导项）。
- 未测 edges / 全文索引 / 向量索引的构建耗时（只测了节点插入）。
- falkordblite：**未跑真实 `add_episode` 全链路**（需真 LLM + embedder，属 Phase 4）；未测写入吞吐 / `kill -9` 丢窗口。
- 未测 embedding 配额归属与 QPS 阈值。
- ~~未验证 falkordblite 的可行性~~ → ✅ **已验**（T-e②，见 §4）。
- 未在真实 llm-service 镜像里构建（只在一次性容器里验证依赖树）——避免动共享镜像。
