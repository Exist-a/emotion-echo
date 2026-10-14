---
status: in-progress
priority: high
owner: lane-cl-d
created: 2026-10-11
type: implementation-plan
source: 2026-10-11 会话（用户："第二条前端缓存部分是想到补的，但没时间，做一份计划"）；承接 2026-10-08 亮点核对发现的前端消息链路问题
depends-on: []
related-plans: []
stage: docs/closure-roadmap/stages/cl-05-message-pipeline/plan.md
stale-notice: 2026-10-14（D 轨建档时亲核）——本计划 §0.3 A2/A3、§5 proto 字段、§2.1 G2 已**部分滞后**：消息游标分页（proto `cursor`/`has_more`）与前端翻页触发点（`[id].vue` 滚动）**均已落地**。**以阶段详档 §A.3 为准**，勿照抄本文件。
---

# Plan — 前端消息链路：游标分页 + 本地缓存 + 状态机 + 断网重试

> 本计划**只描述要做的事，不含任何代码改动**。
> 定位：这是**独立于 E2E 路线图**的一份实施计划（2026-10-11 用户明确：30 阶段已定稿，新工作与既有发现均**不挂 E2E 阶段、不登记账本**）。它解决 2026-10-08 代码核对发现的两个前端消息链路问题：**分页契约错位** 与 **本地消息缓存/状态机/重试半成品**。

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 已读代码（真实文件，非文档）

| 类别 | 文件 | 关键事实 |
|---|---|---|
| 前端 | `emotion-echo-web/app/utils/db.ts` | Dexie 单例，`version(1).stores({ messages: 'id, sessionId, [sessionId+sendTime]' })`，SSR-safe（服务端抛错） |
| 前端 | `emotion-echo-web/app/utils/messageCache.ts` | 完整 CRUD（save/get/update/trim/export…），**全仓零调用** |
| 前端 | `emotion-echo-web/app/stores/message.ts` | 纯内存 Pinia；`loadMoreMessages` 发 `{limit,cursor}`、读 `data.cursor/hasMore`；`sendMessage` 直接用服务端返回值、状态恒 `'sent'` |
| 前端 | `emotion-echo-web/app/stores/conversation.ts` | `cursor=ref<string>('')` 取 `lastItem.updatedAt`，发 `params.cursor` |
| 前端 | `emotion-echo-web/app/composables/useConversationSender.ts` | 生成 `crypto.randomUUID()` 作 clientMsgId；乐观插入 **AI** 临时消息（`status:'streaming'`）；用户消息**不做**乐观插入 |
| 前端 | `emotion-echo-web/app/plugins/init.ts` | `online` 监听仅 `console.log` + TODO（`:108`）；`retryFailedMessages` 未定义 |
| 前端 | `emotion-echo-web/app/pages/chat/conversation/[id].vue` | 仅 `mounted` 调 1 次 `switchSession`，**无翻页触发点**（无滚动/IntersectionObserver） |
| 后端 | `emotion-echo-web-bff/internal/handler/chat_handler.go` | `listMessages` 只读 `limit`（默认 50），返回 `{list}`（**无 cursor/hasMore**，`:171-192`）；`listConversations` 读 `limit`+`offset`，返回 `{list,hasMore}`（`:107-119`） |
| 后端 | `emotion-echo-web-bff/internal/downstream/chat.go` / `chat_grpc.go` | `ListMessages(ctx,convID,limit)` 仅 limit；gRPC 请求只带 `Limit`（offset 被丢） |
| 后端 | `emotion-echo-chat-svc/internal/types/types.go:32-35` | `ListMessagesReq{Id, Limit}` |
| 后端 | `emotion-echo-chat-svc/internal/repository/conversation_repository.go:305-317` | `WHERE conversation_id=? AND deleted_at IS NULL ORDER BY id ASC LIMIT ?`（**无 cursor/offset；且 ASC = 取最早 N 条**） |
| 契约 | `proto/chat.proto:124-137` | `ListMessagesRequest{conversation_id,limit,offset}`（offset 定义了但被丢）、`ListMessagesResponse{messages}`（无 cursor/hasMore） |
| 测试 | `emotion-echo-web/package.json` | vitest + happy-dom + @playwright/test + dexie；**无 fake-indexeddb** |
| 测试 | 全仓 `*.test.ts/*.spec.ts` | `messageCache`/`retryCount` **零引用** |

### 0.2 已查 ADR / 决策

- **无任何 ADR 覆盖"前端消息链路 / 分页契约 / 离线缓存"**。
- 相关边界（历史）：`docs/e2e-roadmap/stages/e2e-18-cache-layer/plan.md:83` 曾把"前端存储（localStorage/Dexie）验证"列为**不做**、推给端侧轨道，但从未被接手 ⇒ 本条悬空至今。**本计划独立于 E2E 路线图**（不挂阶段、不登记账本），此引用仅作历史来源。
- 相关既有机制（可复用，**不要重造**）：后端 `client_msg_id` 幂等（`chat-svc/internal/model/conversation.go:48` 部分唯一索引 + `sendmessagelogic.go:95-98` 去重）。

### 0.3 本文假设（与现状对比）

| 假设 | 现状 | 结论 |
|---|---|---|
| A1 前端有可用的本地缓存层 | 模块在、**零接线** | 需接线，不是从零写 |
| A2 后端支持游标分页 | **不支持**（messages 只 limit；conversations 只 offset） | 需后端先改契约 |
| A3 前端有翻页触发点 | **无** | 需新增滚动触底加载 |
| A4 消息状态机可驱动重试 | 仅字符串枚举，`sending` 从未赋值 | 需补转移逻辑 |
| A5 重试幂等有后端保障 | **有**（client_msg_id） | 前端重试可直接复用，无需新建幂等 |
| A6 缓存模块可直接调用 | **不能**——见 §3 D-C 的"索引缺失" | 需先修 Dexie schema |

---

## §1 问题陈述

一句话：**"前端本地消息缓存 + 游标分页 + 状态机 + 断网重试"目前是设计出来了、但没接线/没实现**，且前端与后端的**分页契约从未对齐**，导致真实链路上"消息只加载一页、翻页参数被静默忽略"。

具体清单：

1. **分页契约错位**（前端按 cursor 设计、后端按 limit/offset 实现）
   - 消息列表：前端按 cursor 设计（发 `cursor`、读 `cursor/hasMore`），后端只认 `limit` 且响应体无 cursor/hasMore ⇒ `hasMore=undefined` ⇒ `loadMoreMessages` 提前返回 ⇒ **恒只加载第一页**。
   - 会话列表：前端发 `cursor`，后端读 `offset` ⇒ **参数名都对不上**，offset 恒 0。
   - 下游 chat-svc SQL 既无 cursor 也无 offset，proto 的 offset 被丢弃。
2. **缓存层未接线**：`db.ts` + `messageCache.ts` 全仓零调用；线上是纯内存 store，刷新即丢。
3. **状态机名不副实**：3 处不一致的状态枚举，无转移表/守卫，`sending` 从未被赋值。
4. **重试计数只有字段**：`retryCount` 恒置 0，无自增/上限/失败后行为。
5. **断网重扫是 TODO**：`init.ts:108`，`retryFailedMessages` 全仓无定义。
6. **测试覆盖为 0**。
7. （新发现，本计划一并列入）**缓存模块自身有潜伏缺陷**：`getPendingMessages`/`getFailedMessages` 用 `where({sessionId,status})`、`trimOldMessages` 用 `where('sendTime')`，但 schema **未建** `[sessionId+status]` 与 `sendTime` 索引 ⇒ 一旦真被调用会**直接抛错**（Dexie 要求查询用到的复合键必须有对应索引）。见 §3 D-C。

---

## §2 目标与非目标

### 2.1 目标

- **G1** 消息分页改为**游标（cursor）分页**并端到端打通：前端 → BFF → chat-svc → SQL → proto，字段与语义一致。
- **G2** 修复"取最早 N 条"缺陷：聊天默认**先给最新**，向上滚动加载更早（backward pagination）。
- **G3** 会话列表分页参数两端对齐（cursor 或 offset 二选一，见 §3 D-B）。
- **G4** 把 `messageCache` 接入真实链路：**读加速（快照恢复）+ 写穿（落缓存）+ 离线队列**。
- **G5** 规范化消息状态机：单一状态来源 + 明确转移 + 乐观插入（`sending` 真正被赋值）。
- **G6** 实现 `retryCount` + 退避 + 上限。
- **G7** 实现断网重连重扫（`online` 事件 → `retryFailedMessages`），复用后端 `client_msg_id` 幂等。
- **G8** 缓存容量治理（trim）与必要索引修复。
- **G9** 测试补齐（单测 + Playwright 回归钉），使上述两类问题闭环。

### 2.2 非目标（本期不做）

- 多设备/多端消息同步的冲突解决（不做 CRDT / OT）。
- 消息端到端加密、离线全文检索。
- SSE 实时推送接入本地缓存（本期只覆盖"拉取 + 发送"两条路径；SSE 收到的消息落缓存可作 stretch）。
- 端侧（Lane O）离线能力整合——本计划只做 Web 前端。
- 后端 `StreamMessages` 流式接口的启用（该接口当前 `Unimplemented`，属另一条账）。

---

## §3 设计决策

> 标记 **[需裁]** 的在开工前需用户/负责人拍板；**[已定]** 为推荐且可默认执行。

### D-A 消息游标用什么键、哪个方向？ **[已定]**

- **键**：`id`（int64，主键，单调、唯一、稳定）。
- **方向**：**backward（最新在前）**。首屏 `ORDER BY id DESC LIMIT n` 取最新 n 条；"加载更早"用 `WHERE id < :cursor`。
- **cursor 定义**：一页里**最小**的 id（DESC 结果的最后一条）。
- **hasMore**：本页返回条数 == limit 即认为还有（配 `id < cursor` 再取）。
- **理由**：聊天场景用户先看最新；`id` 上有主键索引，`WHERE id < c ORDER BY id DESC LIMIT n` 走索引，深页成本恒定（这正是"游标分页避免深分页"的落点）。

### D-B 会话列表分页参数？ **[需裁]——二选一**

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| **B1（推荐）游标** | 后端支持 `cursor = base64(updatedAt|id)`，`WHERE (updated_at,id) < (...)` | 与消息一致、无深分页问题 | 改动较大，需新增 proto 字段 |
| B2 前端改发 offset | 前端把 `cursor` 改成累加 `offset` | 后端几乎不动 | 与"游标分页"主张相悖，深页仍慢 |

> 建议 **B1**：既然主打"游标分页避免深分页"，会话列表也应游标化；且前端已有 `cursor` 变量，改后端比改前端契约更省。

### D-C 缓存的职责边界？ **[已定]**

- **服务端是唯一权威**；本地缓存只做两件事：
  1. **读加速**：进入会话先渲染本地快照，再拉服务端数据覆盖/合并。
  2. **离线队列**：承载"未发送成功"的用户消息，供重试。
- **合并策略**：服务端数据覆盖本地同 id 记录；**本地独有**的 `failed/pending` 消息保留（不能被服务端快照抹掉）。
- **前置修复（重要）**：先修 Dexie schema——新增 `[sessionId+status]` 与 `sendTime` 索引（升 `version(2)`），否则现有查询函数会抛错（§1 第 7 条）。

### D-D 状态机状态集与转移 **[已定]**

- **单一来源**：收敛到 `types/conversation/conversationMessagesType.ts` 一处，其余文件引用它（删掉 `stores/message.ts`、`types/api.ts` 里重复且不一致的定义）。
- **状态集**：`sending`（已发出、等确认）/ `sent`（成功）/ `failed`（失败、可重试）/ `streaming`（AI 流式进行中）/ `truncated`（超长截断）。
- **转移**：
  - `sending → sent | failed`
  - `streaming → sent | failed`
  - `failed → sending`（重试触发）
  - `sent` 终态；`truncated` 终态
- **补齐"乐观插入"**：用户发消息时**先本地插入**一条 `status:'sending'` 的用户消息（temp id = clientMsgId），服务端返回后用真实 id 替换并置 `sent`。这样 `sending` 才真正被赋值（当前从未赋值）。

### D-E 重试策略 **[已定]**

- **可重试对象**：`role=user` 且带 `clientMsgId` 且 `status='failed'` 的消息。
- **上限**：`MAX_RETRY = 3`（可配）。
- **退避**：指数 `min(2^n × 1000ms, 30s)`。
- **幂等**：重试复用**同一个 `clientMsgId`** + 同样内容，命中后端 `client_msg_id` 唯一索引 ⇒ 不会重复入库。
- **失败终态**：超上限即停自动重试，UI 给"手动重发"。

### D-F 缓存治理 **[已定]**

- 单会话保留上限 `trimSessionMessages(sessionId, 500)`，写后触发。
- 启动时 `trimOldMessages(30)` 清 30 天前已发送消息。

### D-G 多标签页一致性 **[需裁]——建议本期 defer**

- 两标签页同时开会各自持有一份状态与缓存，可能出现"一边重试成功、另一边仍显示 failed"。
- 方案：`BroadcastChannel` 广播状态变更；或干脆接受"每标签页独立、刷新即以服务端为准"。
- **建议先 defer**，作为后续增强（避免首期复杂度）。

---

## §4 分阶段实施计划（每阶段一个或多个 TDD 循环）

> 纪律：**先 RED 再 GREEN**（AGENTS §〇）。每阶段列出"改动文件 / 先写的失败测试 / 通过判据"。

### Phase 0 — 类型与契约对齐（前端内部去重）**[无行为变更]**

- **改动**：`types/conversation/conversationMessagesType.ts` 定 `MessageStatus` 单一来源；`stores/message.ts`、`types/api.ts` 改为引用；补充 `cursor/hasMore` 类型定义。
- **RED**：单测断言"三处 MessageStatus 取值集合一致"（当前 4 vs 5 不一致会红）。
- **GREEN**：三处统一。
- **判据**：`pnpm test` 绿；`grep` 确认 MessageStatus 仅一处定义。

### Phase 1 — 后端游标分页（proto → chat-svc → BFF）

- **改动**：
  - `proto/chat.proto`：`ListMessagesRequest` 增 `int64 before_id = 4;`；`ListMessagesResponse` 增 `int64 next_cursor = 2; bool has_more = 3;`。`ListConversationsRequest` 增 cursor 字段（按 D-B 结果）。
  - `chat-svc`：`types.go` `ListMessagesReq` 增 `BeforeID`；`conversation_repository.go` 的 `ListMessages` 改签名为 `(ctx, convID, limit, beforeID)`，SQL 改 `... AND (? = 0 OR id < ?) ORDER BY id DESC LIMIT ?`；`logic` + `grpcserver/chat_server.go` 透传。
  - `web-bff`：`downstream/chat.go` + `chat_grpc.go` 的 `ListMessages` 增参数；`handler/chat_handler.go` 的 `listMessages` 读 `before`/`cursor`，返回 `{list, cursor, hasMore}`。
- **RED**：
  - chat-svc 单测：给定 5 条消息、limit=2、before=0 → 返回最新 2 条（id 降序）；before=min(id) → 返回更早 2 条。
  - BFF 单测：`listMessages` 解析 `before` 且响应含 `cursor/hasMore`。
- **GREEN**：实现上述。
- **判据**：`go test ./...` 绿；**消灭"取最早 N 条"**（新增断言钉住 DESC 语义）。

### Phase 2 — 前端分页对齐 + 翻页触发点

- **改动**：`stores/message.ts`（对齐 cursor 语义、prepend 更早消息）；`stores/conversation.ts`（按 D-B 结果对齐参数名）；`pages/chat/conversation/[id].vue`（新增滚动触底/IntersectionObserver 触发 `loadMoreMessages`）。
- **RED**：
  - store 单测：mock `get` 返回 `{list,cursor,hasMore}`，断言第二页请求带上正确 `before`、消息被 append/prepend 且无重复。
  - 会话 store：断言"加载更多"带对参数、后端能识别。
- **GREEN**：接线。
- **判据**：单测绿；**"分页契约错位"的复现步骤变为"能翻到第二页"**。

### Phase 3 — 缓存接线（读加速 + 写穿 + 索引修复）

- **改动**：
  - `utils/db.ts`：`version(2)` 增 `[sessionId+status]`、`sendTime` 索引。
  - `utils/messageCache.ts`：修 `getPendingMessages/getFailedMessages/trimOldMessages`（索引可用后不再抛错）；如有必要补 `markFailed/incRetry` 辅助。
  - `stores/message.ts`：`switchSession` 先 `getLatestMessages` 渲染快照 → 再拉服务端 → `saveMessages`；`loadMoreMessages` 拉到的页 `saveMessages`；`sendMessage` 成功 `saveMessage`。
  - `composables/useConversationSender.ts`：AI 流式 `onFinish/onError` 落缓存。
- **RED**：
  - 用 **`fake-indexeddb`**（需新增 devDep）测 `messageCache`：save→get 往返、cursor 查询分页、trim 生效、**修索引前 `getFailedMessages` 会抛错（先让它红）**。
  - store 单测：刷新(重新 mount)后能从未持久化→持久化（断言 store 初始化会读缓存）。
- **GREEN**：接线 + 索引修复。
- **判据**：单测绿；手动/自动化"刷新页面后会话消息仍在"有效。

### Phase 4 — 状态机 + 乐观插入 + 重试计数

- **改动**：`stores/message.ts`（乐观插入用户消息 `sending`、转移函数、`retryCount` 自增/上限）；`composables/useConversationSender.ts`（发送失败置 `failed` 并 `incRetry`）。
- **RED**：
  - 状态转移单测（`sending→sent/failed`、`failed→sending`、非法转移拒绝）。
  - 断言"发送瞬间"消息状态为 `sending`（当前恒 `sent`，会红）。
  - `retryCount` 在失败后 +1、达上限不再自增。
- **GREEN**：实现。
- **判据**：单测绿。

### Phase 5 — 断网重连重扫

- **改动**：`stores/message.ts` 新增 `retryFailedMessages()`；`plugins/init.ts` 把 TODO 换成真实调用（`online` 事件 + 启动时若 `navigator.onLine`）。
- **RED**：
  - 单测（fake timers + 模拟离线/在线）：离线时发送 → 置 `failed`；触发 online → 按退避重试并发同一 `clientMsgId`；成功置 `sent`、从待重试集移除。
  - 断言不会无限重试（达上限停）。
- **GREEN**：实现。
- **判据**：单测绿。

### Phase 6 —（可选）多标签页一致性

- 视 D-G 裁定；若做，`BroadcastChannel` 广播状态变更 + 缓存写入。

### Phase 7 — 回归钉 + 收口

- **改动**：新增 `emotion-echo-web/e2e/message-cache-offline.spec.ts`（Playwright）。
- **钉子内容**：① 翻页能加载第二页（cursor 生效）；② 刷新后消息仍在（缓存生效）；③ 断网发送 → 联网后自动重试成功（用 `page.route` abort 模拟断网）。
- **收口**：`docs/plans/` 计划迁 `legacy-plans/landed/`（本计划不挂 E2E 阶段、不动账本）。

---

## §5 契约变更明细（字段级）

| 层 | 位置 | 变更 |
|---|---|---|
| proto | `ListMessagesRequest` | `+ int64 before_id = 4`（0 = 从最新开始） |
| proto | `ListMessagesResponse` | `+ int64 next_cursor = 2`、`+ bool has_more = 3` |
| proto | `ListConversationsRequest` | 按 D-B：若选 B1，`+ string cursor = 3` |
| chat-svc | `types.ListMessagesReq` | `+ BeforeID int64` |
| chat-svc | `conversation_repository.ListMessages` | 签名 `+ beforeID`；SQL `ORDER BY id DESC` + `id < :before` |
| BFF | `downstream.ChatClient.ListMessages` | 签名 `+ before` |
| BFF | `handler.listMessages` | 读 `before`；响应 `+ cursor/hasMore` |
| 前端 | `stores/message.ts` | 请求 `+ before`；解析 `cursor/hasMore`；prepend 更早消息 |
| 前端 | `stores/conversation.ts` | 参数名与后端一致（cursor 或 offset） |

---

## §6 测试策略

- **新增 devDep**：`fake-indexeddb`（vitest 环境无 IndexedDB，Dexie 单测必需）；在 vitest setup 里 `import 'fake-indexeddb/auto'`。
- **单测（vitest）**：
  - `messageCache` 契约/行为测试（save/get/cursor/trim/失败查询不抛错）。
  - 状态机转移表测试。
  - 重试逻辑（fake timers）。
  - store 集成（mock `useApi`）。
- **注意**（历史教训）：happy-dom 对依赖浏览器真实 API 的行为测试会失真 ⇒ 优先**契约/字面量测试**；IndexedDB 用 `fake-indexeddb` 而非 happy-dom 自带实现。
- **E2E（Playwright）**：Phase 7 的 3 条回归钉；沿用现有 `e2e/` 目录与网关 :19080 约定。

---

## §7 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| 改 SQL 排序（ASC→DESC）破坏既有 E2E | 回归 | Phase 1 先跑 `chat-core.spec.ts` / `chat-flow.spec.ts` 看影响面；行为变更入 commit message |
| Dexie schema 升版导致老数据不可读 | 数据丢失 | `version(2)` 增量加索引（Dexie 自动迁移），不删旧 store；写迁移测试 |
| happy-dom 无 IndexedDB | 单测假绿/报错 | 用 `fake-indexeddb`；拒绝用 stub 绕过 |
| 乐观插入 + 重试造成重复展示 | UI 错乱 | 统一以 `clientMsgId` 为本地去重键；服务端幂等兜底 |
| 触碰"发消息链路"与双轨并行冲突（AGENTS §八） | 协作冲突 | 本任务须在 **Lane E（或专设 lane）**执行，**不得**在 Lane O 会话内做；开工前查 `.devmode-session`；按 §八 登握手行 |
| 端到端验证需 dev 栈 | 环境依赖 | 启动 19 容器须 `--env-file .env.local`（红线）；离线测试优先用 Playwright `route.abort` 模拟，避免真断网 |
| 契约改动需同步 proto 两侧生成物 | CI 红 | proto + 生成物 + 前端同 PR；`go vet ./...` 兜底 |

---

## §8 交付物与收口

- **代码**：Phase 0~7 逐 PR（一 TDD 循环一 PR，squash + 删源分支，AGENTS §2.5）。
- **文档**：
  - 建议为"分页契约 + 缓存职责边界"补一条 **ADR**（`docs/architecture/adr/adr-2026-10-frontend-message-pagination-and-cache.md`）并在 `decisions.md` 索引——因为这是**跨前端/后端/契约的架构级决策**。
  - 本计划落地后迁 `docs/legacy-plans/landed/`。
- **证据**：Playwright 钉 + 单测（本计划不登记账本、不挂阶段）。
- **回归钉**：`e2e/message-cache-offline.spec.ts`。

---

## §9 预估、前置与依赖

- **粗估**：7 个阶段，约 **6~9 个 TDD 循环 / 6~9 个 PR**（Phase 0/6 可合并；Phase 1 与 Phase 3 为最大头）。纯工时以实际排期为准。
- **开工前置**：
  1. 用户/负责人裁定 **D-B（会话分页 B1/B2）** 与 **D-G（多标签页是否本期做）**。
  2. 确认执行 lane（避免与 Lane O 冲突，AGENTS §八）。
  3. 加 `fake-indexeddb` 依赖。
- **强依赖**：无外部阻塞；后端改动自包含于 chat-svc + BFF。
- **可并行**：Phase 1（后端）与 Phase 3（前端缓存接线）在契约确定后**可并行**推进。

---

> 一句话总结：这不是"从零做前端缓存"，而是**把已写好的缓存模块接线、把从未对齐的分页契约端到端打通、把状态机/重试从枚举补成逻辑**，并用 Playwright 钉住。
