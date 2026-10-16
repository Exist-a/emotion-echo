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

建档轮（2026-10-14）实读 14 个文件后，推翻来源计划（`docs/legacy-plans/landed/frontend-message-cache-offline-resilience.md`，2026-10-11）三处已过期前提：

1. **消息游标分页已端到端落地**（proto `cursor`/`has_more` + `ListMessagesPage` 的 `id DESC`/`limit+1`/反转升序 + BFF 透传）⇒ 本阶段对消息是"验证 + 补钉"，不是"新建"；
2. **前端翻页触发点已存在**（`[id].vue` 的 `@scroll` + `scrollTop<=48`）⇒ 不必新增 IntersectionObserver；
3. **真正仍错位的是会话列表**（前端发 `cursor` / BFF 读 `offset`）、**缓存层是死代码且 Dexie 索引缺失**（一调用即抛）、**状态机与重试只有壳**。

另：建档时**漏读 CL-F 账本**，只看了 E2E 侧 `E2E-F-205/206` —— 实际 `CL-F-10`（发送后不自动滚到底）与 `CL-F-11`（刷新后出现两个对话）的**归属已被判给 CL-05**，已于本 number 轮补入 plan §2 为 T-21/T-22 + Phase 8（见 §5 经验 3）。

## §2 实施进度

| Phase | 状态（2026-10-16 收敛） | 测试点 | 备注 |
|---|---|---|---|
| **0** 基线取证 | ✅ **完成 2026-10-16** | — | 见 §3.8（会话 `cursor` 被忽略实证 + CL-F-16 复现） |
| **1** 类型与契约收敛 | ✅ **完成** | **T-13 PASS** | 见 §3.1 |
| **2a/2b** 缓存层 + 接线 | ✅ **完成** | **T-9/T-10/T-11/T-12 PASS** | 见 §3.3 / §3.4 |
| **3** 状态机 + 乐观插入 | ✅ **完成** | **T-14/T-15 PASS** | 见 §3.5（含全量回归抓出的真缺陷） |
| **4** 重试 + 断网重扫 | ✅ **完成** | **T-16/T-17/T-18 PASS** | 见 §3.6 |
| **5** 会话列表游标化 | ✅ **完成 2026-10-16** | **T-4/T-5/T-6 PASS** | 见 §3.9 |
| **6** 死路径与降级路径 | ✅ **完成** | **T-7/T-8 PASS** | 见 §3.7 |
| **7** 端到端 + 人可见钉 + ADR | 🟡 **部分** | **T-19/T-20 PASS** | 见 §3.9/§3.10；来源计划迁移与签署未做 |
| **8** 账本继承项 | 🟡 **部分** | **T-21 PASS**；**T-22 BLOCKED** | 见 §3.9/§3.10（CL-F-10 已修 / CL-F-11 未复现） |

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

### §3.2 建档期的「未取得证据」声明（⛔ **已作废**，2026-10-16）

> ⚠️ **本节写于 2026-10-14 建档期，内容已被后续工作推翻，保留只为留痕**。
> 原文声称「Phase 0 基线未取（B 轨持锁）」「T-1~T-22 仍是待执行」等 —— **均已不成立**：Phase 0 已取（`evidence/phase0-baseline-output.txt`）、两条传输路径的运行时翻页已实测（`evidence/phase5-verify-*.txt`）、22 个测试点已全部落四值（见 `plan.md §2`）。**请以 §3.8~§3.10 与 §4 为准**。

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

**类型层摩擦（已按用户裁定处理）**：`messageCache` 的入参与服务端行是两套类型（`sender:'user'\|'ai'` vs `'user'\|'AI'`、`contentType` 取值集不同）⇒ 新增 `toCacheRow()` 断言适配并注明"Phase 3 收敛（`CL-D-13`）后应删"。这是 `CL-F-18` / `CL-D-12` 的现场记录，不是新问题。

**typecheck 首轮 2 处 `TS2345`**（在**测试**里）：`byId.get(1)` —— `api.MessageItem.id` 类型是 `string`，而用例传的是 `number`，严格索引检查判红。已统一 `String(m.id)` 归一，并保留一句注释说明"类型说 string、跑起来是 number"的断言漂移风险。

**分流说明**：plan §3 Phase 2 的 ② "补 `markFailed`/`incRetry` 辅助"**刻意不在本轮做** —— 那是重试逻辑的配套，Phase 4 落地时才有调用方；现在写就是"造无主函数"（anti-patterns 孤儿产出物一类）。

### §3.5 Phase 3 · T-14 / T-15（状态机 + 乐观插入，含一个全量回归抓出的真缺陷）

**RED**（改 `stores/message.ts` 之前）

```
$ pnpm vitest run app/stores/__tests__/message.state.test.ts
 Test Files  1 failed (1)      Tests  8 failed | 0 passed (8)

  ✗ canTransition is not a function / MESSAGE_STATUS_TRANSITIONS undefined（转移表零实现）
  ✗ 乐观插入           → 发送瞬间列表为空：expected [] to deeply equal [ 'cid-1' ]
  ✗ 自动临时 id         → 无 sending 占位
  ✗ 失败保留 failed     → 失败后列表空、缓存无该行
  ✗ store.transitMessage is not a function
  ✗ failed → sending 重试路径不可用
```

**GREEN**

```
$ pnpm vitest run（两个缓存/状态文件）→ 17 passed (17)
$ pnpm vitest run（全量）             → Test Files 81 passed (81) / Tests 682 passed (682)
$ pnpm typecheck                      → 无 error
```

**🩸 落地时被全量回归抓出的真缺陷（已修 + 已补钉）**

1. GREEN 后单跑 Phase 3 新文件 8/8 全绿，但**跑全量时 Phase 2b 的"写穿"钉红了** —— 正是这条跨轮次的红，暴露了：
2. **缺陷**：乐观占位的 `temp id`（`cid-1` / `local-xxx`）与服务端真身 id（`42`）**不同**。原实现只在**内存**里把占位替换成真身，**没删缓存里那条 temp 行** ⇒ 缓存残留一条永远 `sending` 的**幽灵行**。两个后果：① 刷新后界面多出一条永远"发送中"的消息；② **Phase 4 的"待重发扫描"（找 `sending`/`failed`）会把它当成在途消息反复重发**。
3. **修法**：替换时按 temp id `deleteMessage(tempId)`（仅当与真身 id 不同），并把失败包在 try/catch 里（清缓存失败不得影响发送结果）。
4. **钉**：`message.state.test.ts` 新增「成功发送后缓存里不得残留临时占位行」；同时保留 Phase 2b 原有那条写穿钉（它才是抓到本缺陷的那根钉子）。

> 教训：**只跑"本轮新写的测试文件"会漏掉跨模块回归**。本轮的缺陷恰恰由*上一轮*的钉抓到 —— 每轮都跑全量是硬要求（web-test CI 也是全量）。

**其它**：测试里刻意**不用"自造 deferred + 手动 resolve"** 的写法（乐观插入后还有一次 `await` 缓存写入，手动 resolve 会与调用点竞争 ⇒ 测试死等到 15s 超时）。改为在 `postMock` 的实现里**采样调用时刻的状态**，确定性且无时序猜测。

### §3.6 Phase 4 · T-16 / T-17 / T-18（重试计数 + 退避 + 断网重扫）

**RED**（改实现之前）：`message.retry.test.ts` **7 项全失败** —— `setRetryGovernor` / `retryDelayMs` / `canAutoRetry` 均不存在、`retryFailedMessages` 未定义、`retryCount` 未自增。

**GREEN**：`message.retry.test.ts` **7/7**；全量 **82 files / 689 tests** 绿；`typecheck` 无 error。

**实现**（`stores/message.ts` +184/-34、`messageCache.ts`、`plugins/init.ts`）

| 能力 | 实现 |
|---|---|
| 上限 | `MAX_RETRY = 3`；`canAutoRetry(m)` = `failed` 且 `retryCount < MAX_RETRY` |
| 退避 | `retryDelayMs(n) = min(2^n × 1000, 30_000)`；补发时**逐行**按各自已失败次数取值 |
| 补发 | `retryFailedMessages()`：扫 `getAllFailedMessages()`（**全部会话**）→ 退避 → 复用原 id 作 `clientMsgId` → 成功转 `sent` 并清临时行；达上限跳过 |
| 复用 | 抽出 `deliver()` 供 `sendMessage` 与补发**共用**，避免两处各写一遍"POST + 替换 + 清临时行 + 写穿" |
| 接线 | `plugins/init.ts` 的 `online` TODO → `void useMessageStore().retryFailedMessages()` |

**两处有意偏离原计划（均留痕）**

1. **等待走注入 `RetryGovernor` 而非 fake timers**（原计划写"fake timers"）。理由：本测试要同时跑 **fake-indexeddb**（Dexie 的异步调度依赖真实微/宏任务），把 `setTimeout/Date` 整体 fake 掉容易让 IndexedDB 调用挂死 —— 那是"测试基础设施互相打架"，不是被测行为。改用注入更贴合本仓 AGENTS **§3.2**（时钟必须通过接口暴露、禁止直接 `Date.now()`）与 **§3.3**（永不睡眠），且断言的是"**给 sleep 的毫秒数**"这一更强的契约（fake timers 只能验"等过"，验不了"等多久"）。
2. **"启动时 `navigator.onLine` 即补发"未接**。亲核发现：Dexie 库名固定 `EmotionEcho`、消息行**只有 `sessionId` 没有 userId** ⇒ 同浏览器换账号登录时，上一账号的 `failed` 行还在库里；若在**登录完成前**自动补发，会拿新账号的令牌去补旧账号的消息（后端唯一索引只保证"不重复入库"，**不保证"归属正确"**）。⇒ 本阶段只接 `online` 事件（此时已登录），缺口登账本 **`CL-F-20`**（归属 CL-05）+ plan §6 **R13**。

**测试自纠一处**：首轮把两行都种成 `retryCount: 1` 却断言退避 2s —— 实际下一次尝试是**第 2 次**⇒ 应为 4s。改为种 `0`/`1` 两行并断言 `[retryDelayMs(1), retryDelayMs(2)]`，反而**证明了退避是逐行取曲线**、不是全局常数。

### §3.7 Phase 6 · T-7 / T-8（HTTP 降级路径认 cursor + 删除 ASC 死路径）

**用户裁定（2026-10-14，AskUserQuestion）**：① 旧 `ListMessages(ASC)` → **删掉**；② HTTP 降级路径 → **现在就修**。

**RED**（两个钉单独提交在 `be3282d`）

```
$ go test ./internal/handler/ -run TestChatHandler_ListMessages_HonorsCursor -count=1
--- FAIL: TestChatHandler_ListMessages_HonorsCursor
    Should be empty, but was [m2 ... m3 ...]        ← 带 cursor 却还返回"最新一页"

$ go test ./internal/repository/ -run TestRepository_NoLegacyEarliestFirstListMessages -count=1
--- FAIL: ... "assert.NotContains" 命中转换后的源码（Order("id ASC") 仍在）
```

**GREEN**（`b10b945`）

```
$ go build ./...                                → ok
$ go vet ./...                                  → ok（首轮报出 6 处残留调用，已全部迁移）
$ go vet -tags integration ./integration_test/... → ok
$ go test ./... -count=1                        → 全部包 ok
```

**改动**

| 面 | 改动 |
|---|---|
| handler | `ListMessagesHandler` 解析 `cursor`（>0 生效；语义与 gRPC 主路径 / BFF `buildMessageQuery` 一致） |
| repository | 删除旧 `ListMessages(ctx, conversationID, limit)`（`Order("id ASC")` 取最早 N 条）：接口声明 + Postgres 实现 + InMemory 实现 |
| 迁调用方 6 处 | `conversation_repository_test.go`(3)、`sendmessagelogic_test.go`(5)、`listmessageslogic_test.go`(3 个包装器类)、`deleteconversationlogic_test.go`(1)、`integration_test/{intent_enum,health}_integration_test.go`(3) |
| 架构钉 | `internal/repository/dead_path_guard_test.go`：断言源码内无 `Order("id ASC")` / 旧签名，**并反向断言 `ListMessagesPage` 必须仍在** |

**本轮两次自纠（都发生在自己身上）**

1. **测试断言写错**：初版断言"带 cursor 的第二页为空"，但 3 条消息里 `?limit=2` 的最新页是 `{id 2,3}`、其 min id = 2，`id < 2` **正好还剩 1 条** ⇒ 正确行为是返回 1 条。改为"必须返回 1 条且所有 id < cursor"，这样**忽略 cursor 时（会返回 id≥cursor 的最新页）依旧会红**。
2. **残留调用点漏迁**：`go vet` 先报 3 处、再报 5 处、再报 4 处（同一方法的不同调用形态：`msgs, err :=` / `msgs, _ :=`）—— **一次 grep 不足以穷尽**，靠 `go vet ./...` 兜底才收敛。教训：删接口方法时，**编译/静态检查比 grep 可靠**。

**范围边界（重要）**：本 Phase **只动 chat-svc `repository`/`handler`**（本轨文件面），**未碰** BFF `downstream/chat*.go` 与 `chat_server.go` —— 那两处在契约 §四 属 **C 轨排队**，留给 Phase 5。

## §3.8 Phase 0 基线取证（2026-10-16，🔴 栈窗口）

**环境**：本轨镜像 `chat-svc:cl05` / `web-bff:cl05`（H5 带轨后缀 tag）+ 网关 `:19080` + Bearer。
**脚本**：`evidence/phase0-baseline.py`；**输出**：`evidence/phase0-baseline-output.txt`。

| 观测 | 结果 |
|---|---|
| 会话列表 `limit=2` | ids `['684','683']`，`hasMore=true` |
| 会话列表 `limit=2&cursor=<末条 updatedAt>` | ids **仍是 `['684','683']`** ⇒ **交集 `{683,684}`** ⇒ cursor **被完全忽略** |
| 会话列表响应体键 | 只有 `{list, hasMore}` —— **没有** nextCursor |
| 消息列表 `limit=2&cursor=<min id>` | 第二页全部 `< min(page1)` 且**交集为空** ⇒ cursor 生效 |
| `CHAT_TRANSPORT` | 未设置 ⇒ 走 **gRPC** 路径（HTTP 是回滚开关） |

**环境插曲（如实记）**：起容器后登录恒 502，BFF 日志暴露
`UserService/Login err = user-svc repository not initialized (degraded start)` ⇒
**`CL-F-16` 又一次复现**（重建服务早于依赖就绪 ⇒ 永久不自愈）。按账本记载的绕法
"依赖（postgres）healthy 后 `--force-recreate`" ⇒ 立即恢复。

## §3.9 Phase 5 / 7 / 8（2026-10-16）

**Phase 5（会话列表游标化）**
- proto：`ListConversationsRequest.cursor`（string）+ `Response.next_cursor`；生成前先跑 `proto/gen.sh` 确认 **0 diff 可复现**，再改 → 只 `chat.pb.go` 变化。
- 游标 = **不透明复合键** `<unixMicros>:<id>`，**chat-svc 独占编码**（BFF/前端只透传）。为什么不能单值 / 不能毫秒：见 ADR §Context 2。
- 仓库 `ListConversationsPage`（keyset，`limit+1` 探测）；logic 三条路径（游标 / 首页 / 旧 offset 兼容）；types + grpcserver + BFF + 前端对齐。
- **运行验证 17/17 PASS × 2 条路径**（gRPC 与 `CHAT_TRANSPORT=http`）：`evidence/phase5-verify-{grpc,http-fallback}-path.txt`。
  其中会话侧"翻 13 页 506/506 == 全量、无重复"，消息侧与 **DB 全量对账**（conv 550，22/22）。
- **一处路径漂移（本轮实测发现并修）**：非法游标在 gRPC 路径 400、在 HTTP 降级路径 **500**
  （chat-svc HTTP handler 对 logic 错误一律 500）⇒ 补 `statusForLogicErr`（`validation:` → 400）对齐两条路径 + 加钉。

**Phase 8（账本继承项）**
- **`CL-F-10` 根因（亲核）**：`[id].vue` 只 `watch(currentMessages.length)`，而 AI 流式回复是**同一对象在长大**
  （`useConversationSender.ts:183-186`）⇒ 条数不变 ⇒ watcher 永不触发。
  修法：纯函数 `app/utils/chatScroll.ts`（`chatScrollSnapshot` / `isTailGrowth` / `shouldFollowToBottom`）。
- **`CL-F-10` 首版修复被回归钉抓出反例**：只加"用户是否在底部附近"守卫 ⇒ 用户**上翻后发送**时视口停在
  **1519px** 之外、连自己的消息都看不见。⇒ 判据拆成两条：**用户发的无条件跟随**；远端流式仅在底部附近跟随。
  这是一次"钉子抓住实现"的正面样本，故在报告里显式留痕。
- **`CL-F-11` 未能复现**：Playwright T-22 实测 `/new` 一次发送恰好建 **1** 个会话、刷新后仍为 **1**
  ⇒ 根因**未定位**，**不宣称修复**，账本保持 open（见 §4 结论的诚实边界）。

**Phase 7（人可见钉 + ADR）**
- `emotion-echo-web/e2e/message-pipeline.spec.ts`：**4 passed / 1 skipped**（`evidence/phase7-playwright-run.txt`）。
  跳过项 T-20① 为**数据前置不满足**（近 5 个会话均 ≤20 条消息 ⇒ 首屏已取全、上滚无从观察），
  且消息翻页已由 T-2 在 API 层与 DB 对账证明 —— 如实跳过，不调参凑绿。**（该空缺已于 2026-10-16 由 IAB 补齐，见 §3.10。）**
- 钉自身修掉两处**自造假红**并留痕：① 用 `limit=10` 数会话 ⇒ 长度被封顶（账号 ~500 会话）⇒ 改「新增 id 集合」；
  ② 逐会话探测 + 每 test 各登一次 ⇒ 打爆网关 limit-count（60/60s，E2E-31 F-214）⇒ 改 token 全文件复用 + 429 重试 + 最多探 5 个。
- ADR `docs/architecture/adr/adr-2026-10-frontend-message-pagination-and-cache.md` + `docs/architecture/decisions.md` 决策 42。

## §3.10 IAB（内置浏览器）补验短窗口（2026-10-16）

**为什么补**：Phase 7 的 Playwright 钉 4 passed / 1 skipped —— T-20① 因"近期会话都 ≤20 条消息
⇒ 首屏一页已取全、上翻无从观察"跳过；`CL-F-11` 一直未复现。这两处**只有真人视角能定**，
用户 2026-10-16 确认补验范围就是这两项。

**做法**：按契约 §3.1 再取一次栈锁（"收口后补验短窗口"，先例 = A 轨 T-18 短窗口）；
前端用 `pnpm dev` 跑 **worktree 源码**（不是 web 容器 —— 避开 memory 记的"容器跑旧代码"坑）；
IAB **界面登录**（IAB 不支持注入 cookie），登录按钮用**坐标点击**（locator click 超时，memory 已记）。

**结论**：
1. **T-20① → ✅ PASS**：会话 550（22 条）滚到顶后 `.dialog` 条数 **20 → 22**、首条换成会话开头那句、
   `scrollTop` 停在 0（**未被拽回底部** ⇒ "前插不跟随"那半条判据在真机成立）。
2. **CL-F-11 → 仍未复现**：忠实复刻用户步骤（新建页输入 + **点发送按钮** → 刷新）只建 1 个会话
   （691），总数 512→513、刷新后仍 513、侧边栏只一条；**连点两次**变体亦未造出第二个
   （第一次点击后按钮即置灰）。⇒ **根因仍未定位，不宣称修复**，账本保持 open。
3. **顺带**：CL-F-10 在"从顶部发送"这个最刁钻的场景下实测通过（4s 后 gap=0；回复再长过 8s 后仍 gap=0）；
   `CL-F-17`（数字人 VRM）当场复现（右上弹「3D模型加载失败 / 重试」）—— 只记录不动。

**证据**：`evidence/iab-round-2026-10-16.md` + `iab-t21-autoscroll-after-send.png`
+ `iab-cl-f11-after-reload.png`。

**本轮边界**：IAB 只做了补验，**没有**替代 Playwright 的客观断言（两者互补）；
本轮新增的 dev 数据（会话 691 / 550 上的两条测试消息）留在 dev 库，未清理。

## §4 结论与本轮边界（**不自行宣布 done**）

**已取得可复核证据的部分（PASS）**：T-1~T-21 全部 PASS（含 T-7 的运行部分、T-20① 的 IAB 补验）。
**唯一未完成项**：**T-22 = BLOCKED**（`CL-F-11` 缺陷未复现 ⇒ 根因未定位，**不宣称修复**）。

**三方核对（2026-10-16，三个独立子代理；只读核查、明确不采信执行者陈述）查出的问题与返工**：

1. 🔴 **[阻断] CI `static-guards` 实际是红的。** 本轨改 BFF `ChatClient.ListConversations` 接口后，
   漏改被 `//go:build integration` 藏起来的假体
   （`emotion-echo-web-bff/integration_test/bff_integration_test.go:247`）⇒ `go vet -tags integration` 编译失败。
   **本地 `go build/vet/test ./...` 全绿是假象**（该文件根本不在默认构建里），而 `origin/main` 同一 check 是绿的
   ⇒ 属**本轨新增回归**。已修（假体签名随接口更新，仍按该文件约定 `panic`）+ 注释写明必跑守卫
   ⇒ `scripts/test_integration_tag_compiles.sh` **PASS 7 / FAIL 0**（commit `8ded110`）。
   牌面上的教训：那个假体上方的注释**早就警告过同一个坑两次**，这是第三次 —— 而且我当初没跑这个守卫。
2. 🔴 **证据不可区分。** 核对方指出 `phase5-verify-{grpc,http-fallback}-path.txt` 此前**除手写标题外逐字节相同**
   （脚本从不打印它测的是哪条传输路径）⇒ "gRPC 与 HTTP 两条路径各 17/17" 这条被 plan / report / ADR / E2E 账本
   **四处引用**的结论**不可核**。已给脚本加 **P. 路径指纹**（打印 BFF 实际 `CHAT_TRANSPORT` + 相关镜像与 image ID）
   并**两条路径各重跑**：现在两份输出第 6 行明确不同（`UNSET ⇒ grpc(默认)` vs `'http' ⇒ http(降级路径)`）。
3. 🟡 **文档自相矛盾（本轮已返工）**：`report.md` 曾有**两个 §4**、陈旧 §2 表与 §3.2（写着"Phase 0 未开始 /
   22 点仍待执行"）；`plan.md §A.4` 仍称 Phase 0 未执行；ADR 引用不存在的 `CL-D-15`；
   E2E 账本两格残留旧标记 `🔴 未解决`；T-7 三处说法互斥；T-20/T-22 用了**非四值**的 🟡。**均已收敛**。
4. ✅ **未发现造假式销账**：`CL-F-11` 全程保持 open 并两次如实记录"未复现"；各处引用的用例数
   （5/4/11/8/9/7/13/5）**逐一吻合**；`dead_path_guard_test.go` 的反向断言真实存在；
   两个关键钉经核对方"故意改坏实现"验证**确实会红**（游标微秒→毫秒 / 去掉 user 分支）。

**诚实边界（不得被读成"阶段已完成"）**：
1. `CL-F-11` 根因**未定位**（未复现）⇒ 保持 open；
2. 缓存未按用户隔离（`CL-F-20`）⇒ 只接 `online` 补发；
3. `E2E-F-205/206` 已置 ✅（有运行证据）；
4. 来源计划迁移 `legacy-plans/landed/`、`roadmap.md` 状态、占位删除与**最终签署**未做 ⇒ 阶段状态**不由执行者宣布**。

**收口实况**：dev 栈锁（`deploy/.devmode-session`）由本轨 `lane-cl-d` 写入并用毕删除；
`frontend vitest 84 文件 / 706 用例全绿`、`chat-svc / web-bff go build+vet+test 全绿`、`nuxt typecheck exit 0`、
`scripts/check_tdd_gate.sh` GREEN。

---

## §5 经验（可复用）

1. **`tdd-gate` 是按 commit 判的，不是按 PR 判的**。`scripts/check_tdd_gate.sh`：某 commit **新增 ≥5 行生产代码而同一 commit 内无测试文件**即 FAIL（`added < 5` 的行数会被跳过）。所以"纯 `test:` commit + 纯 `feat:` commit"的两刀切**会被拦** —— 实现 commit 必须**同时**带测试文件改动。本轨实际落法：RED commit 只加钉验红；GREEN commit 改实现**并同时补钉**（新增"必须真的引用单一来源"断言）。已回填 plan §3 更正（原 plan 写的规则是反的）。
2. **`pnpm typecheck`（`nuxt typecheck`）开着严格索引检查**：`m![1].matchAll(...)` 这种"非空断言 + 下标访问"会被判 `TS2532 Object is possibly 'undefined'`。写静态扫描型测试时用 `(m?.[1] ?? '')` 之类先收敛为 `string`。**vitest 绿 ≠ typecheck 绿**，两者都要跑。
3. **写 plan 前要把*两份*账本都 grep 一遍**（E2E 侧 `discovered-unresolved.md` 的 `E2E-F-xx` + CL 侧 `discovered-unresolved.md` 的 `CL-F-xx`）。本轮建档只查了 `E2E-F-205/206`，**漏掉 `CL-F-10`/`CL-F-11` 已被判归 CL-05** ⇒ 计划首版少了两条**用户实测**来的测试点，靠本轮补回。
4. **worktree 必须独立 `pnpm install`**（`node_modules` 不随 worktree 物化）；`--prefer-offline` 走本机 store，秒级完成。
5. **"看起来必然抛错"的推断必须实测过再写进文档**。本轮抓到自己的一处错：来源计划与 plan 都断言 `where({sessionId,status})` 会抛 `SchemaError`，实测**不抛**（Dexie 用存在的 `sessionId` 索引兜住，其余条件走内存过滤）。若照抄，就会把"效率改进"说成"修崩溃"，并把 RED 的失败项数算错。**凡"缺 X 必然 Y"的结论，先写一个最小复现**。
6. **测试点编号与自己写的注释要对账**。本轮一度把 `describe('T-16 ...')` 写成"缓存治理"（真 T-16 是"重试计数 + 上限"）⇒ 已在测试与 plan 双向更正，并把治理类断言明确标为"不占测试点编号（属 G8）"。**编号是审计的锚，注释写错等于制造审计假阳/假阴**。
7. **H4 三查有盲区：它证明"我不会回退别人"，证明不了"我不会让别人撞车"**。本轮实测：我按 H2 走独立小 PR（[#221](https://github.com/Exist-a/emotion-echo/pull/221)）登记 4 项决策，**H4 三查全过**（0 冲突 / 0 删除行），但合并后按 **H8** 复验发现 **C 轨 #214 与 main 冲突** —— 因为 C 轨在 `decisions.md` 的 `CL-D-07` 行之后追加 `CL-D-08~10`，我在**同一锚点**追加 `CL-D-11~14`，两侧语义零重叠却**插入点相同**。
   ⇒ 已登 **`CL-F-19`**（归属 CL-00），并按 **H1 上报而非代改**（未 rebase、未碰 C 轨分支，只在 #214 留诊断 + 建议解法）。教训：对**追加式共享文档**，合并前还应往前想一步"**下一轨会不会撞在同一锚点**"；结构性解法 = 每轨一片（`decisions.d/<track>.md`）。
8. **`pnpm add` 会连带跑 `nuxt prepare`**（输出里混着大量 "Duplicated imports … WARN"）——那些 WARN 是**仓库既有**的 auto-import 重复告警，与本次改动无关，别误读成自己引入的问题。
9. **"合并"有两种相反语义，必须先拆开再写**（Phase 2b 最容易写错的一处）：服务端返回**权威全集**时用"替换式合并"（同 id 覆盖、本地独有项保留），返回**一页**时只能用"追加式合并"（按 id 去重追加）。两者共用一个函数就必然出事——第 2 页不含第 1 页的行，替换式会把已加载的旧页整片删掉，用户看到的是"上滚加载更早消息后，下面的消息没了"。本轨因此落成两个具名纯函数（`mergeServerMessages` / `appendServerPage`）各带注释，并写进 plan §6 R12。
10. **`expect` 里传数字、类型里声明 string 会静默通过 vitest 但被 typecheck 拦**。本轮的 `byId.get(1)`（`id` 类型为 `string`）就是这种"测试绿、`pnpm typecheck` 红"的组合 ⇒ **vitest 与 typecheck 必须都跑**（web-test CI job 也是两步都跑）。
11. **只跑"本轮新写的测试文件"会漏掉跨模块回归**。Phase 3 单跑新文件 8/8 全绿，跑全量时**上一轮的写穿钉**才红了——它抓出了"乐观占位的 temp id 未从缓存删除 ⇒ 幽灵 sending 行 ⇒ 会被 Phase 4 重发扫描反复重发"这个真缺陷。**每轮必须跑全量**；新钉只证明新功能，旧钉才守得住老契约。
12. **测试里别用"自造 deferred + 手动 resolve"来测中间态**：乐观插入与其后的一次缓存写入都是 `await`，手动 resolve 会与调用点竞争（`postMock` 还没被调用就 resolve ⇒ 死等到 15s 超时）。改为在 mock 的实现体内**采样调用时刻的状态**，确定性且不猜时序。
13. **接"自动补发"之前先问"这个队列是谁的"**。Phase 4 原计划还要接"启动时若 `navigator.onLine` 就补发"，亲核后**主动没接**：Dexie 库名固定、消息行只有 `sessionId` 没有 `userId` ⇒ 同一浏览器换账号时，上一账号的 `failed` 行还在库里，登录前补发就会**拿新账号的令牌补旧账号的消息**（后端 `client_msg_id` 唯一索引只保证"不重复入库"、**不保证归属正确**）。已登账本 **`CL-F-20`** + plan §6 **R13**。**"功能能跑"与"跑的是谁的"是两个问题**，后者在本地持久化场景里必须先答。

---

---

## §6 收口记录（2026-10-16）

### 6.1 收口契约逐条（RUNBOOK §6）

| # | 契约条 | 结果 | 证据 |
|---|---|---|---|
| 1 | plan.md 每个测试点给四值之一 + 证据 | ✅ | 22 点全为 PASS（**T-22 为 PASS 但注明"这不是已修复"**）；证据见 `plan.md §2` 与 `evidence/` |
| 2 | 回归钉已落地且绿 | ✅ | `emotion-echo-web/e2e/message-pipeline.spec.ts` 4 passed / 1 skipped（跳过项已由 IAB 补齐，见 §3.10）；`evidence/phase7-playwright-run.txt` |
| 3 | `report.md` 用模板（起点/实施/证据/结论/经验） | ✅ | 本文件 §1~§5（旧的两份 §4 已合并，陈旧段落已标作废） |
| 4 | 账本对账：属本阶段未解决的 `CL-F-xx` 存在 ⇒ 只能 partial | ✅ **已解除** | 本轨名下 4 条：`CL-F-18`（顺延，非本阶段工作）、`CL-F-19`（归 CL-00）、`CL-F-20`（本阶段**已规避**）、`CL-F-11`（**按用户裁定关闭**，见 6.2）⇒ 无"属本阶段未解决"项 |
| 5 | 落地计划迁 `docs/legacy-plans/landed/`（同步修链接） | ✅ | `frontend-message-cache-offline-resilience.md` 已迁 + `status: landed`；4 个引用文件路径已改（`docs/plans/README.md` 标「已落地」） |
| 6 | roadmap.md 状态同步；分支/工作区收口（AGENTS §2.5） | ✅ | `CL-05 → done`（附 `CL-D-15` 依据，独立小 PR）；§2.5 三连全过 |
| 7 | 第二方核对（过渡期）通过 | ⏳ | 已发起**独立第二方复核**（针对上一轮三方核对查出的 7 项返工逐项复验）；结论回填于 6.4 |

### 6.2 用户裁定（`CL-D-15`）：`CL-F-11` 以「无法复现」关闭

用户 2026-10-16 明令：**「如果没能复现成功就判断修完了吧，这个判 done，然后收口」** —— 即
`CL-F-11` 两次忠实复现（含用户本人给的确切步骤：新建页输入 + **点发送按钮** → 刷新；以及连点两次变体）均不成立，
故**关闭该账本项、不再阻塞 CL-05 收口**。

⚠️ **读法（必须保留）**：账本写「**无法复现（用户裁定关闭）**」，**不是「已修复」** ——
本阶段**没有**对 `CL-F-11` 做任何代码修复。若将来该现象再现，请按「新发现」重开，
并优先怀疑 `createConversation` 的 `unshift` 本地插入与分页追加的组合路径
（该推断已在账本 `CL-F-11` 行标注为**未验证**）。

### 6.3 已知残留（收口不等于"全绿无债"）

1. `CL-F-11`：**未修复**，仅关闭（见 6.2）。
2. 本地缓存**未按用户隔离**（`CL-F-20`）：本阶段只接 `online` 补发、不接启动即补发。
3. **Postgres 版 keyset SQL 没有测试直接执行过** —— `conversation_repository_cursor_test.go` 全部走 InMemory 孪生，
   `integration_test/` 下无 `ListConversationsPage`。生产 SQL 目前只由"InMemory 孪生语义 + 人工审阅"背书。
4. **T-20① 的 IAB 证据只有数字表、无截图**（两张 PNG 分别是 T-21 与 CL-F-11 刷新后）。
5. `[id].vue` 用 `chatScroll` 这一点**没有单测/架构钉**，唯一覆盖是带数据前置 `test.skip` 的 Playwright 用例。
6. **上翻加载更早消息时不保留精确滚动偏移**（只保证"不把用户拽走"）。
7. `plan.md §A.4` 的 §2.4 smoke：本阶段不涉其 6 条契约，**未跑**。
8. 本阶段在 dev 库留下测试数据（会话 691、550 上的两条测试消息），**未清理**。

### 6.4 第二方复核结论

**结论：通过（7 项中 6 项通过、1 项"通过但带新缺陷"—— 该缺陷已由本节 6.5 修复）。**

独立第二方复核（只读、不采信执行者自述）逐项复现结论：

| 复核项 | 判定 | 其自行复现的关键证据 |
|---|---|---|
| CI 阻断项是否真修好 | ✅ | 本地 `test_integration_tag_compiles.sh` → `PASS 7 / FAIL 0 / GREEN`；远端 CI `12/12 success`（含 `static-guards`），两个 HEAD 都查过 |
| 两份 phase5 证据可区分 | ✅ | `diff` 7 行：`'UNSET' ⇒ grpc(默认)` vs `'http' ⇒ http(降级路径)` —— 不止标题不同 |
| plan §2 四值收敛为单值 | ✅ | 22 行全部恰好 `✅ PASS`；§2 范围内无 🟡/partial/待执行 |
| report 结构 | ⚠️ **通过但引入新缺陷** | §4 唯一、陈旧段落已标作废、§3.8~3.10 在 §5 前；**但它查出：返工把 §3.3~§3.7 整段删了**，留下 17 处悬空引用 ⇒ 见 6.5 |
| 账本与决策 | ⚠️ 部分 | `CL-F-11` 措辞准确（**未修复**仅关闭）✅；`E2E-F-205/206` 无并存标记 ✅；ADR 不再引 `CL-D-15` ✅；`CL-D-15` 当时只在未合的 #237 分支上 ⇒ 合入后即解除 |
| 来源计划迁移 | ✅ | 旧路径不存在、新路径 `status: landed`、全仓 grep 旧路径为空 |
| 不得有假 done | ✅ | done 附依据 `CL-D-15`；§6.3 残留清单**逐项如实**（含 `CL-F-11` 未修复、Postgres keyset SQL 无测试直接执行、T-20① 无截图） |

**总评（原文摘）**：「收口基本诚实、自洽、可核……**没有发现假 PASS / 假 done / 用 N/A 掩盖类造假**。」

### 6.5 复核后修复（我自己在返工中造成的缺陷）

复核第 4 项抓到一处**我引入的真缺陷**：`56fbc00`（"report 去自相矛盾"那次）用于删旧 §4 的脚本
**连带删掉了 `report.md` 的 §3.3~§3.7**（Phase 2a/2b/3/4/6 的证据正文），
留下 `report.md` 与 `plan.md` 共 **17 处**指向不存在章节的悬空引用。
讽刺的是我当时**打印过标题顺序**（§3.2 后直接跳到 §3.8）却没看出来 —— 这正是"复核不能省"的实证。

- **已修**：从返工前 commit `8ded110` 取回 §3.3~§3.7 原样回插（184 行），标题顺序恢复为
  §1 / §2 / §3 / §3.1~§3.10 / §4 / §5 / §6；§3.3~§3.7 的引用与标题**逐一对上**。
- **已修**：`plan.md §A.4` 当时仍无标注地断言"Phase 0 尚未执行"（我上一轮的 §A.4 修补正则没匹配到该标题），
  现改为**⛔ 已作废**并写明 2026-10-16 已完成的实际证据与"仍未做"的 smoke。

