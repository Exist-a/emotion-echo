---
status: accepted
type: next-round-plan
stage-context: CL-08a（已合并 partial）→ CL-08b/c/d/e（pending）
created: 2026-10-20
author: 执行侧提案（**2026-10-20 用户已裁决**，见 §C.0）
related:
  - ../../roadmap.md §2 / §4
  - report.md（§2.2 / §3 残留）
  - plan.md（§3 Phase 0、§6 R1、§7.2 N1）
  - ../../discovered-unresolved.md（CL-F-56 / 57 / 58 / 60 / 61）
---

# 下一轮行动单（2026-10-20）

> **性质**：执行侧按实测写出的候选行动单，**三条待用户裁决**（见 §C）。
> 未裁前不动任何代码、不改 roadmap 状态位。

---

## §A 写前功课（按 AGENTS.md §〇）

**已读代码 / 文件（真实实读，非记忆）**

| 文件 | 读到的事实 | 用途 |
|---|---|---|
| `docs/closure-roadmap/roadmap.md`（全文 388 行） | CL-00~07 全 done；CL-08a `partial`；b/c/d/e `pending`；CL-11/CL-12 `pending`（插入位置待用户定） | 确定候选阶段 |
| `.../stages/cl-08-memory/report.md`（全文） | `status: partial`；残留 = T-5/T-17 运行时 A/B、PG 仓储无 integration test、`FinishExtractRun` 失败卡 pending、E2E 固定等待 | 残留清单来源 |
| `.../stages/cl-08-memory/plan.md`（全文 227 行） | Phase 0 是 4 项待实测 spike（T-a~T-d）；§6 R1 身份传不到 llm-service；§7.2 N1 范围切分待用户裁 | 下一阶段的真实入口 |
| `emotion-echo-ai-svc/internal/logic/memory_extract_handler.go:160-260` | 抽取链：`TryMarkExtractRun` → LLM → `ApplyFacts` → `FinishExtractRun(done)`；**`:209-211` 的失败只 return error，不回滚运行状态** | 定位"卡 pending" |
| `emotion-echo-ai-svc/internal/repository/memory_fact_repository.go:324-372` | `TryMarkExtractRun` 只在 `RowsAffected==0` 时**仅对 `status='failed'` 的行放行重试**；`pending` 行永远 `first=false` | **确证**残留 3 的机制 |
| `emotion-echo-ai-svc/internal/repository/memory_fact_repository_test.go` + `integration_test/testdb_test.go` | 仓储测试**只覆盖内存替身**；但 ai-svc 已有 `//go:build integration` + `testdb_test.go` 的现成 PG harness（8 个 integration 文件） | 残留 2 的修复成本很低 |
| `emotion-echo-web-bff/internal/downstream/llm_grpc.go` | `grep -n "user_id\|UserId"` **零命中** ⇒ CL-F-56 仍未做 | CL-08c Phase 4 的硬前置 |
| `proto/emotion_llm.proto:69` | `string user_id = 5;`（字段早已存在） | 同上 |
| `emotion-echo-web/e2e/cl-08a-memory.spec.ts:86` | `await page.waitForTimeout(20_000)` | 残留 4 实证 |
| `deploy/docker-compose.apps.yml`（ai-svc 段） | 镜像写死 `v0.1.10`、`restart: on-failure` | 解释当前崩溃循环 + CL-F-60 |

**已查 ADR / 决策**：`adr-2026-10-agent-memory-graph.md`（决策 45，含 T-a~T-d 四项待实测）；`CL-D-31`（会话摘要归 CL-08）、`CL-D-35~37`（图记忆形态）、`CL-D-15`、`CL-D-24`（明文 + 软删口径）。

**smoke 未跑**：dev 栈当前不在（见 §B），`scripts/smoke_data_layer.py` 无对象可验；本行动单**不主张任何运行时结论**。

---

## §B 现状快照（2026-10-20 08:40 实测）

### B.1 代码与分支

```
git status -sb            → ## main...origin/main（无 ahead/behind，工作树干净*）
git branch --merged main  → 仅 main
```
\* 三条未跟踪残留物：

| 文件 | 判断 |
|---|---|
| `.zcodeignore`（4.5 KB，10-11） | 本地 IDE 忽略清单，**从未提交** ⇒ 要么进 `.gitignore` 体系，要么删 |
| `cl07_t9_result.json`（924 B，10-16） | CL-07 T-9 的临时结果文件，属"该进证据目录或删掉" |
| `emotion-echo-web/public/_vrm-fps-test.html`（6 KB，10-17） | **落在 `public/`** ⇒ 会被打进生产镜像/站点，是典型 orphan output（`orphan-outputs` 守卫的同型风险） |

### B.2 环境

```
docker ps -a  → 19 个业务容器 Exited（21h 前）；ai-svc 处于 Restarting（每 ~60s 一次）
              → 只有 llm-service / xtts / sensevoice / fer 4 个模型容器在跑（Up 3h）
deploy/.devmode-session → owner: lane-cl-08, until 2026-10-19 23:59（**已过期 ~8.7h**）
```

ai-svc 日志根因（非新缺陷）：

```
startup-strict required dependency unavailable, refusing to start
err="dial emotion-echo-postgres:5432: ... no such host"  → exit code=1 → restart: on-failure → 再起 → …
```

即：**栈已被拆掉，但 ai-svc 因 `restart: on-failure` 在空转重启**（每次拉起都必然失败）。锁文件已过期且自称"用毕请删"。

### B.3 阶段状态（唯一权威 = roadmap §2）

| 阶段 | 状态 | 备注 |
|---|---|---|
| CL-00 ~ CL-07 | ✅ done | CL-07 的 T-10 仍 BLOCKED（挂 `CL-F-17`） |
| **CL-08a** | 🟡 **partial（已合并 #261 → `505e9134`）** | 4 条残留见 report §3 |
| CL-08b ~ CL-08e | pending | b 依赖 a；c 依赖 a；d 依赖 a~c |
| CL-09 | 常驻 | 已积累 60+ 条 CL-F 账 |
| CL-11 / CL-12 | pending | **插入位置用户未定**（roadmap §2 注） |

---

## §C 待用户裁决的三件事

### C.0 裁决结果（2026-10-20，用户已定）

| # | 事项 | **裁定** |
|---|---|---|
| C1 | 下一轮做什么 | **A：先清 CL-08a 残留 + 环境收口 → 再 CL-08c Phase 0 spike** |
| C2 | 环境怎么处置 | **①：删过期锁 + 停崩溃循环的 ai-svc**（已于 2026-10-20 08:45 执行，见 §D.0） |
| C3 | CL-08 范围切分 | **①：内核优先 a + c + d；b（情绪模式）/ e（风险信号）留后** |

> C3 = ① 的直接后果：**CL-08b / CL-08e 不进入本轮排期**，其测试点仍缺（plan §2 无 b/e 条目），
> 留到各自开阶段时补；**CL-08c 的 PR 只含"图后端 + 三层召回 + 实体消解"**，删除与管理页归 d。

下表为裁决前的选项与执行侧倾向（保留作决策依据）：

| # | 事项 | 选项 | 执行侧倾向 |
|---|---|---|---|
| **C1** | **下一轮做什么** | A 先清 CL-08a 残留 + 环境收口 → 再 CL-08c Phase 0 spike（推荐）<br>B 跳过残留，直接 CL-08c Phase 0 spike<br>C 先做 CL-11 + CL-12（前端/后端清账）<br>D 先做 CL-08b（情绪模式） | **A**：残留里有一条是**真缺陷**（见 D.1-4），会静默吞掉事实抽取；且收口成本低（半天、大部分 🟢） |
| **C2** | **环境怎么处置** | ① 删过期锁 + 停崩溃循环的 ai-svc（推荐）<br>② 只删锁<br>③ 都不动，保留现场 | **①**：ai-svc 已空转 8h+，每次重启必然失败，无信息价值 |
| **C3** | **CL-08 范围切分（plan §7.2 N1）** | ① 内核优先：a（已）+ c + d，b/e 留后（推荐）<br>② 一次做完 a~e<br>③ 进 c 之前再定 | **①**：b 的口径是"图上的关系 + 时序"，而**图后端在 c 里**（plan §3 Phase 4）⇒ 先 b 等于给一个还不存在的东西做查询层；e 是安全项，失败代价最高，应单独一轮 |

> C3 之所以现在问：它决定 CL-08c 的 PR 切分与测试点归属（b/e 在 plan §2 **没有测试点**，是既存的范围缺口）。

---

## §D 路径 A 的行动单

### D.0 环境收口（🟢，已执行 2026-10-20 08:45）

| 项 | 动作 | 结果 |
|---|---|---|
| 过期锁 | `rm deploy/.devmode-session` | ✅ 已删（该文件**未被 git 跟踪**，属运行时状态文件）⇒ 串行资源（dev 栈）在队列中释放 |
| 崩溃循环 | `docker stop emotion-echo-ai-svc` | ✅ 已停；不再每 60s 空转（栈仍为「全停」状态，与收口前一致，未改用户可测环境） |
| 模型容器 | `llm-service / xtts / sensevoice / fer` 仍 Up 3h | ⏸ **未动**（非本阶段资产，可能为用户自起；提醒：sensevoice + xtts 各限额 6144M，接近内存红线） |
| 未跟踪残留物 | 未动，仅登记 | ⏸ 待裁（见 D.0.3） |
| 残留分支 | 只读核对，未删 | ⏸ 见 D.0.4 |

**D.0.3 未跟踪文件（待用户定，执行侧未擅自删改）**

| 文件 | 建议 | 理由 |
|---|---|---|
| `.zcodeignore` | 保留在本地 / 或纳入 `.gitignore` 体系 | 本地 IDE 忽略清单，提交与否都无功能影响 |
| `cl07_t9_result.json` | 移入 `stages/cl-07-emotion-display/evidence/` 或删除 | CL-07 T-9 的临时结果，留在根目录会污染工作树判据 |
| `emotion-echo-web/public/_vrm-fps-test.html` | **删或移出 `public/`**（推荐） | 落在 `public/` ⇒ **会被打进生产站点**；这是一次性 VRM 帧率验证页，属 orphan output |

**D.0.4 `CL-F-61` 残留 7 条分支（只读核对结论）**

```
docs/camera-decisions                      [origin gone]  diff vs main: 3 files, +22/-11
docs/cl-f17-and-camera-plan                [origin gone]  diff vs main: 3 files, +83
docs/cl02-handshake                        [origin gone]  diff vs main: 1 file,  +4/-1
docs/cl02-queue-claim                      [origin gone]  diff vs main: 1 file,  +4/-4
feat/cl-04-prompt-assembly                 [origin gone]  diff vs main: 61 files, +3214/-525
fix/stage-113-emotion-chain-and-demo-fixes [origin gone]  diff vs main: 45 files, +1972/-104
feat/e2e-28-f135-dual-endpoint-tts         [无 upstream]  diff vs main: 11 files, +1809/-201
```

- **新证据（可复用的判据）**：7 条里 **6 条远端已 `gone`** —— 本仓按约定一律 squash 合并后删源分支，
  `[origin/x: gone]` 比 `git branch --merged` **灵敏得多**（本轮实测 `--merged main` 仍只认出 `main`）。
- **仍不能据此直接删**：`diff vs main` 的三点差异只反映"分支侧净变更"，main 后续演进会让旧分支看起来仍有差异
  （典型：`docs/cl02-*` 只有 4 行）。⇒ 仍须逐条核内容。
- **`feat/e2e-28-f135-dual-endpoint-tts`** 从未推送、持有 `app/lib/pcmStreamPlayer.ts`（205 行）—— **不得删**，先确认该文件是否已以别的形式进 main。
- ⚠️ **CL-F-61 的"根治"方案当前不可执行**：建议是"按 head 分支查 GitHub PR 状态"，但本机 `gh` **未登录**（`gh auth login` 提示），无 `GH_TOKEN` ⇒ 该方案落地前需先解决认证。

> 新增登记建议：`restart: on-failure` + 依赖缺失 ⇒ **无限重启空转**（无退避上限、无"依赖不可达就退出"的区分）——归 CL-12 或 CL-09，属运维债，非本阶段引入。

### D.1 修「抽取运行卡 pending，来源永久不再抽取」（🟢 纯 Go 单测）

**缺陷机制（已读码确证）**

1. `memory_extract_handler.go:201` `ApplyFacts` 已提交事实；
2. `:209` `FinishExtractRun(done)` 返回错误 ⇒ 直接 `return error`；
3. 消费框架重投 → `:157 TryMarkExtractRun` ⇒ 该行仍是 `pending` ⇒ `first=false` ⇒ `:168-173` **跳过**；
4. `memory_fact_repository.go:346-359` 的重试放行**只针对 `status='failed'`**，`pending` 永不放行；
5. 仓内**没有** sweeper 扫描 `pending` 行。
   ⇒ 结果：该 `source_id` 的事实**永久不再更新**（不报错、不告警，只有一行 pending）。

**TDD 顺序**
- 🔴 RED：`TestMemoryExtract_RetryAfterFinishFailure_ReExtracts` —— 造一个 `pending` 且 `updated_at` 超过租约的 run，断言 `TryMarkExtractRun` 返回 `true`；
- 🟢 GREEN：给 `TryMarkExtractRun` 加**租约判据**（`status IN ('failed') OR (status='pending' AND updated_at < now()-lease)`），或引入 `attempts` 列 —— **二选一，需在实现前定**；
- ♻️ 补一条反例：`pending` 且**未超租约** ⇒ 仍跳过（防并发双抽）。

### D.2 补 PG 仓储 integration test（🟢，复用现成 harness）

用 `integration_test/testdb_test.go`，至少覆盖：
1. `ApplyFacts` 的 supersede 语义（新事实生效 + 旧行 `valid_to` 置位，**不删行**）；
2. `TryMarkExtractRun` 幂等（同源二次 ⇒ `first=false`）；
3. **并发两写**同一 `(subject, predicate)` ⇒ PG 报错 vs 内存静默跳过 的语义差异（report §2.2 明确点名的未验证项）。

### D.3 运行时 A/B（🔴 需起栈，2 条）

| 点 | 做法 | 断言 |
|---|---|---|
| T-5 | 停 llm-service → 发一条消息 → 恢复 | 消息链路 **HTTP 200 / 流式正常**；抽取走重试补齐；`memory_extract_run` 最终终态 |
| T-17 | 两个账号各写事实 → 交叉提问 | A 的事实**绝不出现在** B 的注入；摘要 Redis 键按 user 隔离 |

### D.4 E2E 去固定等待（🟢 前端测试）

`cl-08a-memory.spec.ts:86` 的 `waitForTimeout(20_000)` ⇒ 改为等**可判定的信号**（接口返回 / canvas 出现非空像素 / `_echarts_instance_` 后首帧）。避免固定等待既慢又假绿。

### D.5 收口

按 RUNBOOK：更新 report（残留逐条翻终态）→ 第二方核对 → 由**审计判定**是否 `done`（执行者不自行宣布）。同时把 D.2 的并发语义结论回填 report §2.2。

---

## §E 路径 A 之后：CL-08c Phase 0（spike，零生产代码）

> **为什么排在写代码之前**：ADR 决策 45 有 4 项**未实测**假设，任一不成立就要**改选型**（ADR 明确写"不许硬上"）。
> 这 4 项全部是脚本/容器级验证，**不需要起 19 容器全栈**。

| # | 待实测 | 命令级产出 | 不成立时的后果 |
|---|---|---|---|
| T-a | 硅基流动 `POST /v1/embeddings`（`BAAI/bge-m3`）：可用性 / 维度 / 延迟 / 批量上限 / **是否与 TTS 共享配额** | 一条真调用 + 输出；512 维/1024 维实测 | 记忆抽取可能挤占在线 TTS 配额 ⇒ 换端点或独立 key |
| T-b | `graphiti-core` + `kuzu` 与 llm-service 现有依赖（Python 3.12 / `openai>=1.40` / `grpcio`）兼容性 | 试装 + `pip check` + 镜像体积增量 | 换驱动（falkordb/neo4j = **新增容器**，撞内存红线） |
| T-c | Kuzu 数据文件的**位置 / 挂卷 / 重建容器是否丢 / 备份归属** | 挂卷方案 + 一次重建实测 | 丢数据不可接受 ⇒ 必须依赖"从 PG 重建"（则要把重建做成常规工具） |
| T-d | Kuzu **并发形态**（多 worker 同目录）+ **从 PG 重建图的实际耗时** | 两进程同时打开 + 计时 | 只能单 worker ⇒ 抽取吞吐受限，需要队列化 |

**前置硬依赖**：`CL-F-56`（BFF → llm-service 传 `user_id`）。已实测**仍未做**（`llm_grpc.go` 零命中）。
它必须在 CL-08c Phase 4 之前补：加字段 + 重生成 + `rpc_status.tsv` 终态 + 调用面一致。

---

## §F 备选路径的代价（给 C1 决策用）

| 路径 | 优点 | 代价 / 风险 |
|---|---|---|
| **B 直接 spike** | 最快拿到"图记忆到底能不能落地"的答案 | 留着一个**静默吞抽取**的缺陷过夜（D.1），且过期锁/僵尸容器继续污染后续判断 |
| **C 先 CL-11 + CL-12** | 用户可见的前端修复（对比度、重复标题、浮层） | 与"记忆"这条正在推进的主线**换轨成本高**；CL-12 里 `CL-F-57`（embedding 配额）本就是 CL-08 的前置 ⇒ 会被迫来回切 |
| **D 先 CL-08b** | 情绪模式是演示卖点 | b 要"图上的关系 + 时序"，而**图在 c**；且 b 在 plan §2 **没有测试点** ⇒ 先要补范围定义，反而更慢 |

---

## §G 不做（本行动单边界）

- 不动 roadmap 状态位（CL-08a 是否 `done` 由审计判定）；
- 不动 `charts/**`（k8s 冻结）、不新增容器（内存红线）；
- 不在本轮做 CL-08b/e 的任何设计细化（§4「未到阶段不预设细节」）；
- 不代改他人写入的文档行（`CL-F-59` 同族纪律）。
