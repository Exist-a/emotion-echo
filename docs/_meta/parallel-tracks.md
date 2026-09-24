---
purpose: E2E 主链路（Lane E）× 端侧化 stage1（Lane O）并行推进的约束协议
date: 2026-09-24
status: active
scope: 两轨全部 AI / 人类会话
binding: 约束力 = 已挂入 AGENTS.md §八（违反按 AGENTS.md §六 处置）
last-refresh: 2026-09-24
---

# 双轨并行约束协议（Lane E × Lane O）

> **本文件是约束文件**（同 AGENTS.md §〇 定位）：E2E-17 收口前后，E2E 主链路验证与端侧化
> stage1（[v0.3 §C.1](../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md)）
> 将在**不同会话、不同分支**并行推进。本文件定义两轨"互不影响"的硬规则。
>
> 与 AGENTS.md 冲突时：**业务类型强约束优先，工程/测试/提交约定以 AGENTS.md 为准**，
> 本文件在其之上叠加"并行隔离"约束。
>
> **两轨 session 开工前必读本文件；收工按 §五 checklist 执行。**

---

## §一 双轨定义

| | **Lane E**（E2E 主链路） | **Lane O**（端侧 stage1） |
|---|---|---|
| **目标** | E2E-17 收口 → E2E-18~30 逐阶段推进 | v0.3 §C.1 五任务：WebLLM Demo / MindChat A/B / 编译链路+CDN / 性能基线 / golden set 骨架+云端基线 |
| **上层文档** | [e2e-roadmap/RUNBOOK.md](../e2e-roadmap/RUNBOOK.md) + [anti-patterns.md](../e2e-roadmap/anti-patterns.md) | [plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md](../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md)（v0.3） |
| **治理** | RUNBOOK §13 收口 11 项 + `e2e_stage_audit.py` 全审 | **轻量收口 5 项**：STATUS.md 诚实记录 / 分支清理（§AGENTS §2.5）/ TDD / 账本回填（OND-F）/ ADR（如涉及 D-26）。**不进 `e2e_stage_audit.py`** |
| **分支前缀** | `fix/e2e-*` / `docs/e2e-*` / `test/e2e-*` | `feat/on-device-*` / `test/on-device-*` / `docs/on-device-*` |
| **账本** | [discovered-unresolved.md](../e2e-roadmap/discovered-unresolved.md)（E2E-F 续号，**仅 Lane E 写**） | 本阶段期间独立 **`docs/plans/on-device-findings.md`**（OND-F-01~ 续号，**仅 Lane O 写**）；stage1 收口时并入 E2E-F 账本并删除 |
| **决策** | E2E 决策登 [decisions.md](../e2e-roadmap/decisions.md) D-2x | 端侧主方案 **D-26**（[v0.3 §B.2 模板](../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md)）+ 分项 D-26.1~5；**§十二 5 项决策只有用户拍板，两轨均不得擅自决议** |
| **ADR 文件名** | `adr-2026-09-e2e-*` 或现有 `xtts-*` / `emotion-*` 系列 | `adr-2026-09-on-device-*`（前缀硬隔离） |
| **dev mode 依赖** | **重**（IAB + Playwright + Docker 全栈实测） | **前 3 PR 零依赖**（纯文档/脚本/调研）；**仅 WebLLM Demo IAB 验证需借窗口**（§三.资源1） |

**关键结构事实**：Lane O 的 stage1 五任务中只有 WebLLM Demo 的 IAB 验证需要 dev mode，
其余 4 项全程与 Lane E 零运行时冲突 —— 这是并行可行的根本前提。

---

## §二 文件面硬隔离（第一道墙）

**硬规则**：任何一轨 PR 触碰"共享"列文件，PR 描述必须含**握手声明**（改了什么、对方是否需跟进）。
**触碰对方独占列 = PR 直接 reject（AGENTS.md §六二次违规处置）**。

| 区域 | Lane E 独占（Lane O 禁触） | Lane O 独占（Lane E 禁触） | 共享（需握手声明） |
|------|---------------------------|---------------------------|--------------------|
| **文档** | `docs/e2e-roadmap/**`、`docs/stages/stage-*` | `docs/plans/on-device-*`、`docs/architecture/adr/adr-2026-09-on-device-*`、`docs/plans/on-device-findings.md` | `docs/plans/README.md`（各加一行索引）、`decisions.md`（各登记各自 D 号）、`AGENTS.md`（改动需双方知晓）、本文件 |
| **前端** | `useTTSPlayer*`、`DigitalHuman*`、`useConversationSender*`、`useTTSManager*`、`e2e/**/*.spec.ts` | `useOnDevice*`（新建）、`app/utils/offline/**`（新建）、`pages/demo/local-llm.vue`（新建）、`on-device-*` 测试 | `package.json` / `package-lock.json`（Lane O 加 optionalDependencies **一次性**，PR 注明） |
| **主链路插入口** | `useAIStreamHandler.ts` | **stage1 全程禁触**（阶段二才动，且属四道门后工作） | — |
| **后端** | BFF / 各 svc / proto / gRPC | **stage1 零触碰**（全部在前端+脚本层） | — |
| **部署** | `deploy/docker-compose.*`、`deploy/apisix/seed.sh`、`deploy/.env.local` | **零触碰**（性能基线只读） | — |
| **脚本** | `scripts/e2e_stage_audit.py`、`scripts/check_*.sh`、`scripts/smoke_data_layer.py` | `scripts/on-device-*`（新建） | — |

**Lane O 阶段一新增依赖白名单**（超出即 reject）：`@mlc-ai/web-llm`（进 `optionalDependencies`
或经 dynamic import 隔离，**production bundle 不得打包**）、`opfs-polyfill`、WebGPU 类型垫片。

---

## §三 四个共享资源的仲裁（第二道墙）

### 资源 1：dev mode（19 容器栈）——时间片 + 归属锁（双保险）

- **锁文件**：`deploy/.devmode-session`（**gitignored，不进版本库**）。谁启动 dev mode 谁写入：
  ```
  owner: lane-e | lane-o
  session: <会话简述>
  until: YYYY-MM-DD HH:MM
  ```
  用完**删除文件**。
- **规则**：
  1. 启动 dev mode 前**必须**读该文件：存在且未过期 → 等待或协商，**不得强占**；
  2. Lane O 前 3 个 PR（D-26 ADR / golden set / MindChat A/B）**禁止启动 dev mode**；
  3. Lane O 唯一申请点 = WebLLM Demo IAB 验证，**按半天粒度**在本文档 §四 资源日历预约；
  4. 两轨任何一方收工必须删除锁文件（§五 收工三查第 2 条）。
- **启动铁律（两轨通用，E2E-F-70/99 教训）**：dev mode 验收必须
  `--env-file .env.local --profile dev`（RUNBOOK §2.1），且**核对被验镜像 `docker inspect <image> Created` 时间戳**
  晚于最新修复 commit，写入本轨 STATUS.md 环境基线。E2E-17 的 web v0.1.5 / BFF v0.1.28 已重建，
  **两轨均不得覆盖这两个 tag**；Lane O 后续如需新镜像用独立 tag（`on-device-*`）。

### 资源 2：main PR 合并窗口——串行 + update-branch

- 两轨 PR 互不依赖，合并顺序任意，但**同一时刻只合一个**；
- squash merge + **立即删源分支**（AGENTS.md §2.5）；
- 同轮第二个 PR 需先 update branch（memory：Emotion-Echo PR 交付机制）；
- 每次合并后跑 `python scripts/e2e_stage_audit.py --all` 确认 0 FAIL 不被打破
  （Lane O 合入同样适用——**禁破坏 Lane E 审计**）。

### 资源 3：编号空间——分区独占

| 空间 | 归属 | 规则 |
|------|------|------|
| E2E-F-1xx 续号 | Lane E 独占 | Lane O 发现 E2E 侧问题 → **记账不修**，登 `discovered-unresolved.md` 但行内标注 `（发现于 lane-o）` |
| OND-F-01~ | Lane O 独占（`docs/plans/on-device-findings.md`） | stage1 期间端侧自身问题登此；**stage1 收口时并入 E2E-F 续号并删独立账本** |
| **D-NN 系列**（`e2e-roadmap/decisions.md`） | D-26 及分项归 Lane O（2026-09-24 立项）；Lane E 用 **D-27 起** | 已用：D-25；D-10~24 保留 |
| **决策 N 系列**（`architecture/decisions.md`） | **决策 33 归 Lane O**（D-26 主方案 ADR 同日登记）；Lane E 用 **34 起** | 已用至 32；两套编号并行（先例 D-09↔26、D-14↔31、D-26↔33），**登记时两套都要避开对方已占号** |
| ADR 文件名 | 各自前缀（§一） | — |

### 资源 4：决策带宽——5 项决策只有用户拍板

- Lane O 在 golden set / MindChat A/B 产出后，汇总成一页材料
  `docs/plans/on-device-decision-pack.md`（决策 1 隐私定位 / 2 模型选型 / 3 来源告知 /
  4 离线范围 / 5 摘要存储）；
- **两轨任何会话不得擅自拍板 v0.2 §十二任一项**（v0.3 §B.1 + AGENTS.md §〇）；
- 用户一次拍完 → Lane O 立 D-26.1~5 ADR + decisions.md 登记。

---

## §四 时间线与资源日历

```
T0  现在 ──────────── E2E-17 收口（Lane E 主场，dev mode 占用）
    Lane O：D-26 ADR + golden set 骨架（零 dev mode）           ← 可即刻开工
T1  E2E-18~24 ─────── Lane E dev mode 重度
    Lane O：MindChat A/B + 编译链路/CDN + 性能基线脚本（零 dev mode）
T2  E2E-25~27 ─────── Lane E 中度
    Lane O：WebLLM Demo 开发（TDD 字面量契约测试，不开浏览器）
T3  E2E-28~30 ─────── Lane E 收口期
    Lane O：Demo IAB 验证（借 dev mode 半天窗口，见日历）+ 决策材料包
T4  E2E 全收口 + §十二拍板 → Lane O 进阶段二（v0.3 四道门）
```

**资源日历**（Lane O 申请 dev mode 窗口时在下表登记；Lane E 同步自己的占用）：

| 日期窗口 | Lane E 占用 | Lane O 申请 | 状态 |
|----------|-------------|-------------|------|
| （示例）2026-09-26 下午 | — | WebLLM Demo IAB 验证 | ⬜ 待预约 |

---

## §五 每 session 开工/收工 checklist（两轨通用）

**开工三查**：
1. `git fetch origin && git status -sb` —— 确认基线；读对方 STATUS.md 尾部 3 行
   （Lane E：`docs/e2e-roadmap/stages/<stage>/STATUS.md`；Lane O：`docs/plans/on-device-STATUS.md`）；
2. 读 `deploy/.devmode-session` —— 确认 dev mode 归属，有人占用不得强占；
3. 确认本次改动全部落在 §二 本轨独占列 —— 有共享列改动先在本文件 §六 记一行握手。

**收工三查**：
1. 写/更新本轨 STATUS.md（照 [E2E-17 STATUS.md](../e2e-roadmap/stages/e2e-17-digital-human-tts/STATUS.md)
   格式：已做 ✅ / 未做 ❌ 分列，**禁止美化**）；
2. 删除 `deploy/.devmode-session`（释放锁）；
3. AGENTS.md §2.5 三连自检：
   ```bash
   git status                    # working tree 干净（或改动是有意留下并写明）
   git status -sb                # 与 origin/<branch> 无 ahead/behind
   git branch --merged main      # 除 main 外为空
   ```

---

## §六 握手登记（共享文件改动记录）

| 日期 | 轨 | 改动的共享文件 | 内容 | 对方是否需跟进 |
|------|----|----------------|------|----------------|
| 2026-09-24 | Lane O | `docs/_meta/parallel-tracks.md`、`AGENTS.md §八`、`.gitignore` | 本协议首次落地 | Lane E 下次开工必读本文件 |
| 2026-09-24 | Lane O | `docs/e2e-roadmap/decisions.md`、`docs/architecture/decisions.md`、本文件 §三.资源3 | D-26 主方案 ADR 立项双登记（D-26 行 + 决策 33 行）；**编号口径勘误**：协议原表只写 D-NN 单系列，现补齐决策 N 并行系列（Lane O 占 33 / Lane E 从 34 起） | Lane E：决策登记时两套号都避开 26/33（资源3 表已更新） |
| 2026-09-24 | Lane E | `docs/plans/README.md`（索引 +1 行）、本文件 §六 | 新增 `conversation-memory-pending-decision-2026-09-24.md`（会话记忆缺失待决策；用户口头反馈，非 on-device 文件，不触对方独占列） | Lane O 无需跟进；若 v0.2 §十二决策 5「摘要存储」拍板，回读该文档 §D-c |

---

## §七 与既往教训的对应（防复发）

| 既往教训（memory / 账本） | 本协议防线 |
|--------------------------|------------|
| E2E-F-70/99 dev 容器跑旧代码 | §三.资源1 启动铁律（镜像时间戳写入 STATUS.md 环境基线） |
| 执行者自证的完成不可信 | §一 双治理：Lane E 全审 + Lane O 轻量但 STATUS 诚实格式；合并后必跑 audit |
| §2.5 分支/worktree 残留（2026-09-12 清理 32+34+11） | §五 收工三查强制 |
| 文档与代码漂移 | §五 STATUS.md 诚实格式 + §六 握手登记双向可查 |
| TDD 倒置 | Lane O 全程 ALL CODE IS TDD；WebGPU/Worker/OPFS 走字面量契约测试（memory「TDD 契约 vs 行为测试」） |
| 账本与状态脱钩 | §三.资源3 编号分区，stage1 收口并账对账 |
| 主链路插入口被误改 | §二 `useAIStreamHandler.ts` stage1 全程禁触 |
| `.env.local` 被覆盖 | §二 Lane O 零触碰 `deploy/`；启动铁律继承 AGENTS.md §四红线 |

---

## §八 生效与终止

- **生效**：2026-09-24，随本文件 + AGENTS.md §八 挂钩落地；
- **终止**：Lane O stage1 收口（v0.3 §G.1 阶段一收口契约满足）且 E2E-17 已 done 后，
  本协议降级为历史参考（改 `status: landed` 并迁 `docs/legacy-plans/`——按 AGENTS.md §七）；
- **修订**：修订须在 §六 登握手行，且注明"谁要求改、为什么"。
