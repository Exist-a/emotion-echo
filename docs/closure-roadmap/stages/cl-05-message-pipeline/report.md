---
stage: cl-05
type: execution-report
status: in-progress
created: 2026-10-14
plan: plan.md
---

# CL-05 · 消息链路闭环 — 执行记录

> 体例（RUNBOOK §0 / §6.3）：起点 / 实施 / 证据 / 结论 / 经验。**执行中与收口时同步更新**。
> ⚠️ **阶段状态由审计 / 第二方判定，执行者不自行宣布 `done`**。

## §1 起点（建档时亲核的现状）

建档轮（2026-10-14）实读 14 个文件后，推翻来源计划（`docs/plans/frontend-message-cache-offline-resilience.md`，2026-10-11）三处已过期前提：

1. **消息游标分页已端到端落地**（proto `cursor`/`has_more` + `ListMessagesPage` 的 `id DESC`/`limit+1`/反转升序 + BFF 透传）⇒ 本阶段对消息是"验证 + 补钉"，不是"新建"；
2. **前端翻页触发点已存在**（`[id].vue` 的 `@scroll` + `scrollTop<=48`）⇒ 不必新增 IntersectionObserver；
3. **真正仍错位的是会话列表**（前端发 `cursor` / BFF 读 `offset`）、**缓存层是死代码且 Dexie 索引缺失**（一调用即抛）、**状态机与重试只有壳**。

另：建档时**漏读 CL-F 账本**，只看了 E2E 侧 `E2E-F-205/206` —— 实际 `CL-F-10`（发送后不自动滚到底）与 `CL-F-11`（刷新后出现两个对话）的**归属已被判给 CL-05**，已于本 number 轮补入 plan §2 为 T-21/T-22 + Phase 8（见 §5 经验 3）。

## §2 实施进度

| Phase | 状态 | commit | 测试点 | 备注 |
|---|---|---|---|---|
| **0** 基线取证 | ⛔ 未开始（🔴） | — | — | 需 dev 栈；`deploy/.devmode-session` 由 **B 轨**持锁（至 18:00），D 轨在契约 §3.1 **排第 4**（B → C → D） |
| **1** 类型与契约收敛 | ✅ **完成** | `2a2290e`(RED) → `af2eba9`(GREEN) | **T-13 PASS** | 见 §3.1 |
| **2a** 缓存层：Dexie 索引 | ✅ **完成** | `6328f57`(RED) → `e8be055`(GREEN) | **T-9 / T-10 PASS**（+ G8 治理） | 见 §3.3 |
| **2b** 缓存层：store 接线 | ✅ **完成** | `6328f57`… → `2495727`(GREEN) | **T-11 / T-12 PASS** | 见 §3.4 |
| 3 状态机 + 乐观插入 | ⬜ 未开始（🟢） | — | T-14、T-15 | |
| 4 重试 + 断网重扫 | ⬜ 未开始（🟢） | — | T-17、T-18 | |
| 5 会话列表游标化 | ⛔ 阻塞（待 C 轨 `proto/**` 握手答复） | — | T-4/T-5/T-6 | 已向 C 轨 PR #214 发握手请求 |
| 6 死路径 + HTTP 降级路径 | ⬜ 未开始（编码 🟢） | — | T-7、T-8 | 与 C 轨同文件（`chat_server.go` / `downstream/chat*.go`）⇒ 等 C 轨落地 |
| 7 端到端 + ADR + 收口 | ⬜ 未开始（🔴） | — | T-20 | |
| 8 账本继承（CL-F-10/11） | ⬜ 未开始（🔴 需栈复现） | — | T-21、T-22 | 先定位根因（账本明写"未查代码、不臆断"） |

## §3 证据

### §3.1 Phase 1 · T-13（`MessageStatus` 单一来源）

**RED（commit `2a2290e`，改实现之前跑的）**

```
$ pnpm vitest run app/types/conversation/message-status-single-source.architecture.test.ts

 FAIL  app/types/conversation/message-status-single-source.architecture.test.ts > ... > 全仓 `export type MessageStatus` 定义点唯一，且落在 types/conversation/conversationMessagesType.ts
 AssertionError: 重复/错误的定义点：stores/message.ts, types/conversation/conversationMessagesType.ts

 FAIL  ... > 唯一来源的取值集合 = 5 值超集（含 truncated）
 AssertionError: 取值集合应与 5 值超集完全一致
   - Expected     + Received
     Array [        Array [
       "failed",      "failed",
       "sending",     "sending",
       "sent",        "sent",
       "streaming",   "streaming",
       "truncated",   ← 缺
     ]              ]

 FAIL  ... > 其它文件不得内联重声明状态字面量联合（须引用单一来源）
 AssertionError: 内联重复声明：stores/message.ts, types/api.ts

 Test Files  1 failed (1)
      Tests  3 failed | 1 passed (4)
```

**GREEN（commit `af2eba9`）**

```
$ pnpm vitest run app/types/conversation/message-status-single-source.architecture.test.ts
 Test Files  1 passed (1)
      Tests  5 passed (5)

$ pnpm vitest run            # 全量，防回归
 Test Files  78 passed (78)
      Tests  654 passed (654)

$ pnpm typecheck             # 首轮报过 1 个 TS2532，修后无 error
 (no error)
```

**改动（4 文件）**

| 文件 | 改动 |
|---|---|
| `app/types/conversation/conversationMessagesType.ts` | `MessageStatus` 补齐为 5 值超集（+`truncated`），并标注为全仓唯一来源 |
| `app/stores/message.ts` | 删除本地重声明（核过：该类型在本文件内**无使用**、且**无外部引用**），改为 `export type { MessageStatus } from …` 保住导入路径 |
| `app/types/api.ts` | 内联字面量联合 → `import type { MessageStatus }` |
| `app/types/conversation/message-status-single-source.architecture.test.ts`（新） | 架构钉：定义点唯一 / 取值集合唯一 / 无内联重声明 / 两个历史重复点必须真的引用单一来源 |

**新增架构钉**：`app/types/conversation/message-status-single-source.architecture.test.ts`（5 个断言，源码级静态扫描）

### §3.2 本轮**未**取得的证据（诚实声明，按 RUNBOOK §4）

- **Phase 0 基线未取**：dev 栈由 **B 轨**持有（`deploy/.devmode-session`，`until 2026-10-14 18:00`），D 轨排第 4，**未强占** ⇒ 无一条运行时证据（无 curl 翻页输出、无 DB 比对、无真实 prompt 抓包）。
- **无 Go / Playwright 证据**：Phase 1 是纯前端类型层，未触 Go / proto / e2e。
- 因此 §2 中 T-1~T-8、T-11~T-12、T-14~T-15、T-17~T-22 **仍是"待执行"**，不得因已完成项绿而整体乐观。

### §3.3 Phase 2a · T-9 / T-10（Dexie 缓存索引）

**RED（commit `6328f57`，改 db.ts 之前）**

```
$ pnpm vitest run app/utils/messageCache.test.ts
 Test Files  1 failed (1)
      Tests  3 failed | 8 passed (11)

  ✗ Dexie schema 必须逐项声明缓存在用的索引
    → 实际索引：id | sessionId | [sessionId+sendTime]（缺 sendTime、[sessionId+status]）
  ✗ schema 以**增量** version(2) 升级
    → Dexie 版本应为 2: expected 1 to be 2
  ✗ trimOldMessages 不应抛错
    → SchemaError: KeyPath sendTime on object store messages is not indexed
```

**GREEN（commit `e8be055`）**

```
$ pnpm vitest run app/utils/messageCache.test.ts → 11 passed (11)
$ pnpm vitest run（全量）                        → Test Files 79 passed (79) / Tests 665 passed (665)
$ pnpm typecheck                                 → 无 error
```

**改动**：`app/utils/db.ts` 新增 `version(2)` 声明，`messages` 索引
`'id, sessionId, [sessionId+sendTime]'` → `'id, sessionId, sendTime, [sessionId+sendTime], [sessionId+status]'`；
**v1 声明保留**（加法式升级 ⇒ Dexie 自动迁移，不删旧 store、不丢用户本地缓存，钉已锁"v1 与 v2 声明都在且 verno=2"）。
新增依赖：`fake-indexeddb ^6.2.5`（devDep，`package.json` 追加式）；`tests-setup.ts` **首行** `import 'fake-indexeddb/auto'`。

**⚠️ 本轮**修正**了一个此前的**错误断言**（文档与来源计划都写错了）

来源计划 §1 第 7 条与 plan §A.1 原文写：
> `getPendingMessages`/`getFailedMessages` 用 `where({sessionId,status})`、`trimOldMessages` 用 `where('sendTime')` —— **一旦真被调用会直接抛错**（Dexie 要求查询用到的复合键必须有对应索引）。

**实测结果否掉了一半**：

| 查询 | 实测 | 原因 |
|---|---|---|
| `where('sendTime')`（`trimOldMessages`） | **真抛** `SchemaError: KeyPath sendTime ... is not indexed` | Dexie 的 `where(key)` 必须命真实索引 |
| `where({sessionId, status})`（`getPendingMessages`/`getFailedMessages`） | **不抛**（返回结果正确） | Dexie 用存在的 `sessionId` 索引取集，剩余条件在内存里过滤 |
| `where('sessionId').sortBy('sendTime')`（`exportSessionMessages`） | **不抛** | Dexie 在内存中排序（不强制该字段有索引） |

⇒ 结论：补 `[sessionId+status]` 是**效率/契约**改进（走索引而非全会话扫描），**不是**修崩溃；本阶段唯一真崩溃点只有 `trimOldMessages`。
这一条已回填 plan §A.1 / §2 T-9，并作 §5 经验 5 留档。

### §3.4 Phase 2b · T-11 / T-12（本地缓存接进消息链路）

**RED（改 `stores/message.ts` 之前）**

```
$ pnpm vitest run app/stores/__tests__/message.cache.test.ts
 Test Files  1 failed (1)      Tests  6 failed | 2 passed (8)

  ✗ 读加速              → 发请求时 currentMessages 长度 0（根本没读快照）
  ✗ 合并·服务端覆盖同 id  → expected [ 1 ] to deeply equal [ 1, 99 ]（本地 failed 被丢）
  ✗ 合并·排序            → expected [ 1, 2 ] to deeply equal [ 99, 1, 2 ]
  ✗ 写穿·loadMoreMessages → 缓存 []（期望 [7]）
  ✗ 写穿·sendMessage      → 缓存 []（期望 [11]）
  ✗ 快照不污染（切会话）   → 会话 B 快照为空
  ✓ 降级（缓存读失败不挡服务端）—— 现状本就无缓存调用 ⇒ **"碰巧通过"**，GREEN 后仍要保住
  ✓ getFailedMessages 查询
```

**GREEN**

```
$ pnpm vitest run app/stores/__tests__/message.cache.test.ts → 8 passed (8)
$ pnpm vitest run（全量）                                    → Test Files 80 passed (80) / Tests 673 passed (673)
$ pnpm typecheck                                             → 无 error（首轮报过 2 处 TS2345，见下）
```

**改动**（`stores/message.ts` +103/-18）

| 能力 | 实现 |
|---|---|
| 读加速 | `switchSession` 先 `getLatestMessages(sessionId, 50)` 渲染快照，再调 `loadMoreMessages()`；**读失败只 `console.warn` 并降级**（缓存是加速不是依赖）；`await` 期间会话被切走则不写入旧快照 |
| 写穿 | `loadMoreMessages` 拉到的页 + `sendMessage` 成功的行（含**幂等命中**分支）落缓存；缓存写失败不影响加载/发送结果 |
| 合并 A | `mergeServerMessages(local, server)` —— 快照替换：服务端覆盖同 id，本地独有 `failed`/`sending` 保留 |
| 合并 B | `appendServerPage(existing, page)` —— 分页追加：按 id 去重、同 id 以服务端为准 |

**类型层摩擦（已按用户裁定处理）**：`messageCache` 的入参与服务端行是两套类型（`sender:'user'\|'ai'` vs `'user'\|'AI'`、`contentType` 取值集不同）⇒ 新增 `toCacheRow()` 断言适配并注明"Phase 3 收敛（`CL-D-13`）后应删"。这是 `CL-F-13` / `CL-D-12` 的现场记录，不是新问题。

**typecheck 首轮 2 处 `TS2345`**（在**测试**里）：`byId.get(1)` —— `api.MessageItem.id` 类型是 `string`，而用例传的是 `number`，严格索引检查判红。已统一 `String(m.id)` 归一，并保留一句注释说明"类型说 string、跑起来是 number"的断言漂移风险。

**分流说明**：plan §3 Phase 2 的 ② "补 `markFailed`/`incRetry` 辅助"**刻意不在本轮做** —— 那是重试逻辑的配套，Phase 4 落地时才有调用方；现在写就是"造无主函数"（anti-patterns 孤儿产出物一类）。

## §4 结论

- **T-13 = PASS**（§3.1）：`MessageStatus` 现为全仓唯一来源，取值集合 5 值，两处历史重复点改为引用。
- **T-9 / T-10 = PASS**（§3.3）：Dexie 缓存索引补齐（v1→v2 加法式升级）；缓存往返/分页/治理共 11 用例绿。
- **T-11 / T-12 = PASS**（§3.4）：缓存接进真实链路（读加速 + 写穿 + 双合并策略），8 用例绿。
- 三轮都是保守改动（类型收敛 / 只加索引 / 只加缓存读写与纯函数合并）：全量 **654 → 665 → 673** 全绿 + `typecheck` 绿 ⇒ **零回归**。
- 其余 **17** 个测试点**未执行**；阶段整体状态仍为 `in-progress`，**不得宣布 done**（§2 中 🔴 项还一条运行时证据都没有）。
- **[M] 决策已全部落定（2026-10-14 用户）**：4 项均按推荐 ⇒ 记为 `CL-D-11`~`CL-D-14`（独立小 PR [#221](https://github.com/Exist-a/emotion-echo/pull/221) 已合 main）。结论：SSE **不**落缓存；前端 DTO 字段**只登记不动**；`MessageWithStatus` 三形态**顺延 Phase 3**；`CL-F-10`/`CL-F-11` **按原计划排 Phase 8**。⇒ 阶段范围就此锁定，无待裁定项。
- 下一步（🟢，不需栈）：**Phase 2b**（`stores/message.ts` 的读加速 / 写穿 / 合并策略 → T-11 / T-12），随后 Phase 3（状态机 + 乐观插入）、Phase 4（重试 + 断网重扫）。

## §5 经验（可复用）

1. **`tdd-gate` 是按 commit 判的，不是按 PR 判的**。`scripts/check_tdd_gate.sh`：某 commit **新增 ≥5 行生产代码而同一 commit 内无测试文件**即 FAIL（`added < 5` 的行数会被跳过）。所以"纯 `test:` commit + 纯 `feat:` commit"的两刀切**会被拦** —— 实现 commit 必须**同时**带测试文件改动。本轨实际落法：RED commit 只加钉验红；GREEN commit 改实现**并同时补钉**（新增"必须真的引用单一来源"断言）。已回填 plan §3 更正（原 plan 写的规则是反的）。
2. **`pnpm typecheck`（`nuxt typecheck`）开着严格索引检查**：`m![1].matchAll(...)` 这种"非空断言 + 下标访问"会被判 `TS2532 Object is possibly 'undefined'`。写静态扫描型测试时用 `(m?.[1] ?? '')` 之类先收敛为 `string`。**vitest 绿 ≠ typecheck 绿**，两者都要跑。
3. **写 plan 前要把*两份*账本都 grep 一遍**（E2E 侧 `discovered-unresolved.md` 的 `E2E-F-xx` + CL 侧 `discovered-unresolved.md` 的 `CL-F-xx`）。本轮建档只查了 `E2E-F-205/206`，**漏掉 `CL-F-10`/`CL-F-11` 已被判归 CL-05** ⇒ 计划首版少了两条**用户实测**来的测试点，靠本轮补回。
4. **worktree 必须独立 `pnpm install`**（`node_modules` 不随 worktree 物化）；`--prefer-offline` 走本机 store，秒级完成。
5. **"看起来必然抛错"的推断必须实测过再写进文档**。本轮抓到自己的一处错：来源计划与 plan 都断言 `where({sessionId,status})` 会抛 `SchemaError`，实测**不抛**（Dexie 用存在的 `sessionId` 索引兜住，其余条件走内存过滤）。若照抄，就会把"效率改进"说成"修崩溃"，并把 RED 的失败项数算错。**凡"缺 X 必然 Y"的结论，先写一个最小复现**。
6. **测试点编号与自己写的注释要对账**。本轮一度把 `describe('T-16 ...')` 写成"缓存治理"（真 T-16 是"重试计数 + 上限"）⇒ 已在测试与 plan 双向更正，并把治理类断言明确标为"不占测试点编号（属 G8）"。**编号是审计的锚，注释写错等于制造审计假阳/假阴**。
7. **H4 三查有盲区：它证明"我不会回退别人"，证明不了"我不会让别人撞车"**。本轮实测：我按 H2 走独立小 PR（[#221](https://github.com/Exist-a/emotion-echo/pull/221)）登记 4 项决策，**H4 三查全过**（0 冲突 / 0 删除行），但合并后按 **H8** 复验发现 **C 轨 #214 与 main 冲突** —— 因为 C 轨在 `decisions.md` 的 `CL-D-07` 行之后追加 `CL-D-08~10`，我在**同一锚点**追加 `CL-D-11~14`，两侧语义零重叠却**插入点相同**。
   ⇒ 已登 **`CL-F-14`**（归属 CL-00），并按 **H1 上报而非代改**（未 rebase、未碰 C 轨分支，只在 #214 留诊断 + 建议解法）。教训：对**追加式共享文档**，合并前还应往前想一步"**下一轨会不会撞在同一锚点**"；结构性解法 = 每轨一片（`decisions.d/<track>.md`）。
8. **`pnpm add` 会连带跑 `nuxt prepare`**（输出里混着大量 "Duplicated imports … WARN"）——那些 WARN 是**仓库既有**的 auto-import 重复告警，与本次改动无关，别误读成自己引入的问题。
9. **"合并"有两种相反语义，必须先拆开再写**（Phase 2b 最容易写错的一处）：服务端返回**权威全集**时用"替换式合并"（同 id 覆盖、本地独有项保留），返回**一页**时只能用"追加式合并"（按 id 去重追加）。两者共用一个函数就必然出事——第 2 页不含第 1 页的行，替换式会把已加载的旧页整片删掉，用户看到的是"上滚加载更早消息后，下面的消息没了"。本轨因此落成两个具名纯函数（`mergeServerMessages` / `appendServerPage`）各带注释，并写进 plan §6 R12。
10. **`expect` 里传数字、类型里声明 string 会静默通过 vitest 但被 typecheck 拦**。本轮的 `byId.get(1)`（`id` 类型为 `string`）就是这种"测试绿、`pnpm typecheck` 红"的组合 ⇒ **vitest 与 typecheck 必须都跑**（web-test CI job 也是两步都跑）。
