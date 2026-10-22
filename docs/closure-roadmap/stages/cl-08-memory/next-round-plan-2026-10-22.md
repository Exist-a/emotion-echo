---
status: accepted
type: next-round-plan
stage-context: CL-08a（done）→ CL-08c Phase 0 spike（pending）→ CL-08c Phase 1~4
created: 2026-10-22
author: 执行侧提案（**2026-10-22 用户已裁决**，见 §C.0）
related:
  - ../../roadmap.md（§2 CL-08 行 / §4 CL-08 例外注记）
  - plan.md（§3 Phase 0、§6 R1、§7.2 N1）
  - next-round-plan-2026-10-20.md（上一轮行动单，路径 A 已执行完）
  - ../../discovered-unresolved.md（CL-F-56/57/58/60/61/62/66）
  - ../../RUNBOOK.md（执行六步 / 收口契约）
  - ../../../../architecture/adr/adr-2026-10-agent-memory-graph.md（决策 45 · 四项待实测项）
---

# 下一轮行动单（2026-10-22）

> **性质**：执行侧按 2026-10-22 实况写出的行动单，**四项已由用户裁决**（见 §C.0）。

---

## §C.0 裁决结果（2026-10-22 10:39，用户已定）

| # | 事项 | **裁定** |
|---|---|---|
| C1 | 下一轮主线 | **A：CL-08c Phase 0 spike**（T-a~T-d，零生产代码） |
| C2 | CL-11 / CL-12 插入位置 | **B：CL-08c Phase 4 之后**；其中 **CL-F-66 提前单做**（见 C4） |
| C3 | 环境与栈归属 | **A：一次性容器 / 临时 venv，不取栈锁**；未跟踪物按建议清理（`_vrm-fps-test.html` 移出 `public/`、`cl07_t9_result.json` 移入 CL-07 证据目录） |
| C4 | CL-F-66（高危） | **A：提前单做**（复用 `x-internal-api-key` 范式 + compose 收口 + BFF 调用方同步 + 栈上负向验证） |

---

## §A 写前功课（按 AGENTS.md §〇）

**已读代码 / 文件（真实实读，非记忆）**

| 文件 | 读到的事实 | 用途 |
|---|---|---|
| `docs/closure-roadmap/roadmap.md`（全文 388 行） | CL-00~08a 全 done（CL-08a 2026-10-21 收口，PR #261/#267/#268/#269/#270）；CL-08b~e pending；CL-09/11/12 pending（插入位置待用户定） | 确定候选阶段 |
| `.../stages/cl-08-memory/plan.md`（全文 227 行） | Phase 0 = 4 项 spike（T-a~T-d，ADR 待实测项回填点）；Phase 4 前置 = `CL-F-56`；§7.2 N1 已由用户 10-20 裁为"内核优先 a+c+d" | 下一阶段的真实入口 |
| `.../next-round-plan-2026-10-20.md`（全文 326 行） | 路径 A（CL-08a 残留清销）D.0~D.5 全部已执行；§E 写明"路径 A 之后 = CL-08c Phase 0 spike" | 本轮是它的接续 |
| `.../discovered-unresolved.md`（全文，账本 70 条） | 30 条 ✅ / **40 条未解决**；与下一轮相关的：`CL-F-56`（BFF→llm-service 不传 user_id）、`CL-F-57`（embedding 未实测）、`CL-F-58`（Kuzu 文件未定）、`CL-F-60`（公共 tag 滞后）、`CL-F-61`（残留分支）、`CL-F-66`（gRPC 8892 越权面，**高**） | 候选项与前置 |
| `emotion-echo-web-bff/internal/downstream/llm_grpc.go`（grep `user_id\|UserId`） | **零命中** ⇒ `CL-F-56` 仍未做（与 proto `emotion_llm.proto:69` 字段存在但无人写对应） | CL-08c Phase 4 硬前置 |
| `emotion-llm-service/grpc_server.py:120-160` | llm-service **已有** `x-internal-api-key` 元数据鉴权拦截器（health 豁免、缺失/错误分别记 warn + abort UNAUTHENTICATED） | `CL-F-66` 修法有**仓内现成范式**可复用 |
| `deploy/docker-compose.apps.yml:326-327/404-414/706-709` | `INTERNAL_API_KEY` / per-svc `*_LLM_INTERNAL_API_KEY` 机制已下发三服务；`INTERNAL_API_KEY_REQUIRED` 开关存在（默认 0） | 同上 |
| `deploy/.env.local`（**只列变量名，未读值**） | 有 `TTS_API_KEY`/`TTS_API_BASE_URL`/`LLM_*`/`INTERNAL_API_KEY`；**无** embedding 专属 key | T-a 的打点材料 |
| `docs/closure-roadmap/RUNBOOK.md` §3/§6 | 执行六步（实测→记账→RED→GREEN→证据→收口）；收口契约 7 条 | 行动单结构依据 |
| `scripts/smoke_data_layer.py`（头部 80 行） | 基址 = 网关 `:19080` + Bearer 登录；每次跑触发 1 消息 + 1 会话；退出码 2 = 前置不可用 | §B.2 基线跑法 |

**已查 ADR / 决策**：`adr-2026-10-agent-memory-graph.md` §待实测项（T-a~T-d 逐条，含"未证不得当作已成立"）；`CL-D-35/36/37`（图记忆形态）、`CL-D-07`（Redis 用户级摘要先例）、`CL-D-33`（抽取必须异步的实测依据）。

**smoke 已跑（2026-10-22 10:4x，对当前在跑栈）**：`python scripts/smoke_data_layer.py` → **16/24 PASS，8 FAIL**。
8 个 FAIL **全部是 §8 镜像新鲜度 STALE**（CL-F-60 已知项 + CL-F-25 实证的"Created 可能不前进"现象）
⇒ **§契约 1~7 全绿**（user_behavior_events 行数 / event_type 细分 / analytics_reader 视图 / chartData / schema 一致性 / dev 路径）。
按 CL-F-08 口径：STALE 是必要条件不是充分条件，**本轮不据此断言在跑镜像缺代码**（下一轮做运行时验收时先行为核对）。

**架构假设清单（本文假设 vs 现状核对）**

| 假设 | 现状核对 |
|---|---|
| 当前 dev 栈可作 spike 的运行时底座 | ✅ 18 容器 healthy；但**无栈锁**，归属未登记 |
| spike 可以不碰在跑服务完成 | ⚠️ 待裁（§C3）：T-b/T-c/T-d 需要容器/venv，可用一次性容器实现 |
| `TTS_API_KEY` 能调 `/v1/embeddings` | ❌ **未证**（T-a 正是要证它） |
| `graphiti-core`+`kuzu` 装得进 Python 3.12 | ❌ **未证**（T-b） |
| CL-F-66 可复用 llm-service 的 internal-key 范式 | ⚠️ 范式实读存在，但 ai-svc gRPC 侧**未证**（拦截器装配 + BFF 调用方同步改） |

---

## §B 现状快照（2026-10-22 10:30~10:50 实测）

### B.1 代码与分支

```
git log --oneline -1   → e51f44d7 docs(cl-08a): 复核轮 9 项整改 + 撤销假的已知边界 (#270)
git status -sb         → ## main...origin/main（无 ahead/behind）
git status --short     → 5 条未跟踪：.workbuddy/ · .zcodeignore · cl07_t9_result.json
                          deploy/compose.track-override.yml · emotion-echo-web/public/_vrm-fps-test.html
git branch -vv         → 7 条残留分支（CL-F-61 部分解决）：6 条 origin gone + feat/e2e-28-f135-dual-endpoint-tts 从未推送（持 pcmStreamPlayer.ts，不得删）
```

未跟踪物处置自 10-20 §D.0.3 起**一直待裁**（本轮再提一次，见 §C3-D）。

### B.2 环境

```
docker ps → 18 容器全 healthy；ai-svc / web-bff / web = :cl08ares（10-19~10-20 构建）
            user-svc v0.1.5 · chat-svc v0.1.16 · analytics-svc v0.1.11 · assessment-svc v0.1.5 · llm-service v0.1.2 · xtts v2.0.1（公共 tag）
deploy/.devmode-session → 不存在（无锁；上一轮已按契约释放）
smoke → 16/24 PASS；8 FAIL 全为 §8 STALE（§契约 1~7 全绿）
```

⚠️ 若「大创结题」轨要截图，它与下一轮**争同一个 dev 栈**：回基线镜像命令见 10-22 工作记忆
（不带 `compose.track-override.yml` 重建三服务）。**两条轨不能同时对栈动手**——需先裁归属（§C3）。

### B.3 阶段状态（唯一权威 = roadmap §2）

| 阶段 | 状态 | 备注 |
|---|---|---|
| CL-00 ~ CL-07 | ✅ done | CL-07 T-10 仍 BLOCKED（挂 CL-F-17 的 VRM，已解决但数字人情绪接线未验） |
| **CL-08a** | ✅ **done（2026-10-21 收口）** | 21 测试点 PASS 17 / N/A 4；第二方核对两轮 FAIL→整改→合并 |
| CL-08b/c/d/e | pending | 用户 10-20 裁定：内核优先 a+c+d，**b/e 留后** |
| CL-09 | 常驻 | 账本 40 条未解决里最大的族 |
| CL-11 / CL-12 | pending | 插入位置待用户定（§C2） |

账本计数（本次实读）：**70 条登记 / 30 ✅ / 40 未解决**。

---

## §C 待用户裁决的四件事

### C1 下一轮主线做什么

| # | 选项 | 代价 / 风险 |
|---|---|---|
| **A** | **CL-08c Phase 0 spike**（T-a~T-d，零生产代码，🟢 为主） | 不碰业务代码；结论直接决定 ADR 能否转 Accepted |
| B | 先补 `CL-F-56`（BFF→llm-service 传 user_id） | 动 `proto/**`（脊柱资源）+ 重生成 + `rpc_status.tsv`；是 Phase 4 前置而**非** Phase 0 前置，不必抢 |
| C | 先做 CL-11（前端体验族 7 项：CL-F-30/31/32/46/51/52/53/54/64） | 用户可见、但需重建 web 镜像才能 IAB；与"记忆"主线换轨 |
| D | 先做 CL-12（后端可靠性族 11 项，含高危 CL-F-66） | 同 C；且 CL-F-56/57 本就是 CL-08 前置，会被迫来回切 |

**执行侧倾向 A**：10-20 已裁路径（"先清 CL-08a 残留 → 再 CL-08c Phase 0 spike"）的残留已清零；
ADR 决策 45 明确"四项待实测项未证 ⇒ 不许硬上"——**spike 越晚，后续 Phase 1~4 返工风险越大**。
CL-11/12 插入位置是独立决策（C2），不一定要排在 spike 前。

### C2 CL-11 / CL-12 的插入位置

| # | 选项 |
|---|---|
| A | spike 之后、CL-08c Phase 1 之前 |
| **B** | **CL-08c Phase 4 之后**（Phase 4 与 CL-F-56 同动 proto/BFF 面，连着做省一次上下文切换）；其中 **CL-F-66 高危安全项建议提前**（见 C4） |
| C | 现在就插到 spike 之前 |
| D | 继续"看整体再定" |

**执行侧倾向 B**：CL-08c Phase 1~3（PG 模型 / 抽取 worker / BFF 注入段）与 CL-11/12 文件面基本隔离，
可以各自 `in-progress`（并行契约 §1.2）；Phase 4 动 `proto/memory.proto` + BFF `user_id`，与 CL-12 的
CL-F-56、CL-11 的前端面有接力关系。

### C3 环境与栈归属

当前栈**无锁**、跑 `:cl08ares`（含 CL-08a 代码）。spike 的资源需求：T-a 只需一次外网 curl；
T-b/T-c/T-d 需要 Python 3.12 + kuzu 的**实验环境**。

| # | 选项 | 说明 |
|---|---|---|
| **A** | **全部用一次性容器 / 临时 venv，不碰在跑服务、不取栈锁** | 符合契约 §1.3「镜像是共享可变资源」；结题轨可随时用栈 |
| B | 取 `deploy/.devmode-session` 锁，在 llm-service 里试装 | 会改共享镜像 tag（除非带轨后缀，但那又污染 llm-service 的生产镜像定义） |
| C | 先停 `:cl08ares` 三服务、回基线镜像 | 仅当结题轨马上要截图时才需要 |

**执行侧倾向 A**。附带两条小额待裁（10-20 起挂起）：
- **D.0.3 未跟踪物**：`emotion-echo-web/public/_vrm-fps-test.html` **会打进生产站点**（建议删或移出 `public/`）；
  `cl07_t9_result.json`（建议移入 `stages/cl-07-emotion-display/evidence/` 或删）；`.zcodeignore`（建议进 `.gitignore` 体系或删）。
- **D.0.4 残留分支**（CL-F-61）：6 条 origin gone 仍须逐条内容核对，**禁止批量 `git branch -D`**；
  `feat/e2e-28-f135-dual-endpoint-tts` 不得删。

### C4 `CL-F-66`（ai-svc gRPC 8892 越权面，**高**）是否提前单做

现象：`grpcserver/server.go:143-151` 拦截器链无 internal auth；compose `8892:8892` 发布到宿主 ⇒
任意本机进程可冒他人 `user_id` 读写其记忆。

| # | 选项 |
|---|---|
| **A** | **提前单做**：复用 llm-service 的 `x-internal-api-key` 拦截器范式（已实读）+ compose 改 `127.0.0.1:8892:8892` 收口 + 运行时负向验证（无 key ⇒ UNAUTHENTICATED） |
| B | 留在 CL-12 排队 |

**执行侧倾向 A**：高危 + 修法有仓内现成范式 + 主体 🟢（Go 单测 + compose + 一次栈上负向实验）。
⚠️ 注意联动面：BFF→ai-svc 的调用方要同步带 key（`downstream` 侧），否则修完线上全 401——
**这正是它必须连运行时验证一起做的原因**。

---

## §D 路径 A 行动单（CL-08c Phase 0 spike）

> **纪律**：四项各出一条**命令 + 输出**归档 `stages/cl-08-memory/evidence/phase0-*.txt`；
> 任一不成立 ⇒ 回 ADR 改选型（不许"硬上"）；**零生产代码改动**。
> 回填位置 = ADR §待实测项表 + `plan.md` T-0x + 账本 `CL-F-57`/`CL-F-58` 状态列。

| 步 | 内容 | 命令级做法 | 不成立的后果 |
|---|---|---|---|
| **D.1** | **T-a embedding 端点实测** | 从 `.env.local` 取 `TTS_API_KEY`（**变量传递、不打印**）→ `POST {TTS_API_BASE_URL}/v1/embeddings`，model=`BAAI/bge-m3`，输入含中文句 → 记录 HTTP 码 / 维度 / 延迟 / 批量上限 / 错误体 | 401/403 ⇒ 账户无权限 ⇒ 换独立 key 或换端点；配额与 TTS 共用且受限 ⇒ 抽取要让路或限速 |
| **D.2** | **T-b 依赖兼容性** | 一次性 `python:3.12-slim` 容器（或临时 venv）→ `pip install graphiti-core kuzu` → `pip check`；记录 llm-service 镜像体积增量（对照当前 `emotion-echo/llm-service:v0.1.2` 大小） | 冲突 ⇒ 换驱动（falkordb/neo4j = **新增容器，撞内存红线**）或换嵌入方式 |
| **D.3** | **T-c Kuzu 持久化** | 容器 A 挂命名卷写数据 → 删容器 → 容器 B 同卷起 → 断言读得到；记录卷方案 + 与 `deploy/backups/` 的关系 | 重建即丢 ⇒ 必须把"从 PG 重建"做成常规工具（D.4 同时验耗时） |
| **D.4** | **T-d 并发 + 重建耗时** | 两进程同时打开同一 Kuzu 目录（预期锁冲突/报错）；用 dev 库合成 N 条 episode bulk insert 到 Kuzu 计时 | 只许单 worker ⇒ 抽取吞吐受限，需队列化（T11 决议据此定稿） |

**执行顺序建议**：D.1（纯外网，最先跑）→ D.2（装得上才谈得上 D.3/D.4）→ D.3 → D.4。
**全程不取栈锁、不改在跑容器**（除非 C3 裁 B）。

**若 C4 裁 A**，`CL-F-66` 作为并行小项（🟢 + 一次栈上负向验证），与 spike 互不争文件面
（spike 动 llm-service 实验环境；CL-F-66 动 ai-svc Go 码 + compose）。

---

## §E spike 之后：CL-08c 主体路径（预告，非本轮承诺）

```
Phase 0 spike（本轮）→ Phase 1 PG 数据模型（emotion_echo_ai schema）→ Phase 2 抽取 worker
（ai-svc 独立 consumer group）→ Phase 3 BFF 注入段（Redis 用户级摘要）→
Phase 4 图 + proto/memory.proto + CL-F-56（user_id 前置在此补齐）→ Phase 5 = CL-08d（删除闭环 + 管理页）
```

⚠️ Phase 4 前必须补 `CL-F-56`（已实测仍未做）；每阶段按 RUNBOOK 六步走，测试点见 `plan.md` §2（21 点）。

---

## §F 备选路径的代价（给 C1/C2 决策用）

| 路径 | 优点 | 代价 / 风险 |
|---|---|---|
| **A spike** | 拿到"图记忆能不能落地"的硬结论；零生产代码风险 | 不产出用户可见变化 |
| **C 先 CL-11** | 用户可见修复（对比度 / 重复标题 / 浮层 / 图标签重叠） | 需重建 web 镜像；与记忆主线换轨；CL-12 的 CL-F-57 仍是 CL-08 前置 |
| **D 先 CL-12** | 清高危 + 后端债 | 大体量；与 C 同型换轨成本 |
| **B 先 CL-F-56** | 小、硬前置 | 动 proto 脊柱；抢做 spike 也不冲突，但单独一轮 PR 偏薄 |

---

## §G 不做（本行动单边界）

- 不动 roadmap 状态位（CL-08c 是否开工由用户在 C1 裁后另行登记）；
- 不动 `charts/**`（k8s 冻结）、**不新增容器**（内存红线）；
- spike **不写生产代码**、不改 `proto/**`、不动在跑容器（除非 C3 裁 B）；
- 不在本轮做 CL-08b/e 的任何设计细化（§4「未到阶段不预设细节」）；
- 不代改他人写入的文档行（CL-F-59 同族纪律）；
- 不删任何分支（CL-F-61：逐条核对前禁止批量删）。

---

## §H 调研依据（本次实读）

- 代码/配置 10 处：`roadmap.md`、`stages/cl-08-memory/{plan.md,next-round-plan-2026-10-20.md}`、`discovered-unresolved.md`、`downstream/llm_grpc.go`、`emotion-llm-service/grpc_server.py:120-160`、`deploy/docker-compose.apps.yml:326-414/706-709`、`deploy/.env.local`（仅变量名）、`RUNBOOK.md §3/§6`、`scripts/smoke_data_layer.py` 头部（清单见 §A）。
- ADR/决策：`adr-2026-10-agent-memory-graph.md` §待实测项；`CL-D-07/33/35/36/37`。
- **运行时**：`docker ps`（18 容器）；`python scripts/smoke_data_layer.py` → 16/24 PASS（8 FAIL 全 §8 STALE，§契约 1~7 全绿）。
- **未做**：未跑 integration test（无代码改动可跑）；未验证 T-a~T-d 任何一项（正是本轮要做的）；未读 `.env.local` 的值。
