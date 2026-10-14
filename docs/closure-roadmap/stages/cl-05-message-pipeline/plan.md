---
stage: cl-05
title: 消息链路闭环（会话列表游标化 + 本地缓存接线 + 状态机 + 断网重试 + 分页契约端到端对齐）
type: transformation
status: in-progress
created: 2026-10-14
last-updated: 2026-10-14（D 轨建档 → **Phase 1 完成**（T-13 PASS：RED 3 红 → GREEN 5/5 + 全量 654/654 + typecheck 绿）；并入账本继承项 `CL-F-10`/`CL-F-11` 为 T-21/T-22（Phase 8）；更正 `tdd-gate` 纪律写法）
depends-on: []           # 不依赖任何阶段；**不阻塞于 T1**（CL-01/02/03）
blocks: []
gate: []                 # 无开工前阻塞决策门；3 个 [M] 见 §7
source: docs/plans/frontend-message-cache-offline-resilience.md（主，2026-10-11）
related-plans:
  - docs/plans/grpc-deadlink-actualization.md（CL-02/C 轨：`chat_server.go` 与旧 ASC 仓储方法同面，已握手排队）
  - docs/closure-roadmap/parallel-contract.md（D 轨占位 + §3.1 栈排队 + §四 握手区）
ledger: E2E-F-205 / E2E-F-206（本阶段收口时关闭，见 §4）+ CL-F-10 / CL-F-11（**账本已把归属判给 CL-05** → 本阶段内解决，见 §2 T-21/T-22）
---

# CL-05 · 消息链路闭环 — 执行计划

> **核心口径（roadmap §4 CL-05）**：**"消息链路闭环"** —— 分页契约对齐 + 缓存接线 + 断网可补发，**不再是半成品**。
>
> **现状一句话**（本次亲核，2026-10-14）：
> **消息**的游标分页**已经端到端落地**（proto `cursor`/`has_more` → chat-svc `ListMessagesPage`（`id DESC`+`limit+1` 探测）→ BFF 透传 → 前端 store 发 `cursor`/读 `cursor+hasMore`）；
> **会话列表**仍是 offset 语义、而前端发的是 `cursor` ⇒ **参数名对不上、"加载更多"重取第一页**；
> 本地缓存模块（Dexie）**全仓零调用**且**索引缺失（一调用即抛）**；消息状态机只有 3 处互不一致的枚举、`sending` **从未被赋值**；
> 重试计数只有字段无逻辑；断网重扫是 `TODO` 注释。
>
> **本阶段是 T2 对话地基的第二块**（第一块 = CL-04，已 done），承接 `E2E-F-205 / E2E-F-206` 两条挂账。
>
> **执行协议**：[RUNBOOK.md](../../RUNBOOK.md)（六步循环 / 证据有效性 / 收口契约）
> + [parallel-contract.md](../../parallel-contract.md)（D 轨占位与文件面隔离）
> + [anti-patterns.md](../../../e2e-roadmap/anti-patterns.md)（14 类失真反例）。
>
> **并行**：本阶段为 **D 轨**，与 **B 轨（CL-01）**、**C 轨（CL-02）** 并行（A 轨 CL-04 已 done）。
> 唯一串行资源 = **dev 栈**（B 持锁 → C → D）与 **`proto/**`**（C 轨预占，本轨**已申请握手**）。
>
> **并行协议遵从自检（H1~H9 / AGENTS §8.1 条款 7~10）** —— 见 §A.5。

---

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件，本次实读亲核）

| 文件 | 关键事实（file:line） |
|---|---|
| `proto/chat.proto` | `ListMessagesRequest` **已有 `int64 cursor = 4`**（`:126-130`，注释"返回 id < cursor 的消息（0 = 从最新开始）"）+ `ListMessagesResponse` **已有 `bool has_more = 2`**（`:136-137`）⇒ **消息分页契约已存在**；但 `ListConversationsRequest` **只有 `limit`/`offset`、无 cursor**（`:142-143`），`ListConversationsResponse.has_more` 在（`:148-149`）；`Message.client_msg_id` 幂等键在（`:94`） |
| `emotion-echo-chat-svc/internal/repository/conversation_repository.go` | `ListMessagesPage` `:350-372`：`if cursor > 0 { q.Where("id < ?", cursor) }`（`:356-358`）、`Order("id DESC").Limit(limit+1)`（`:360`）、`hasMore := len(out) > limit`（`:363-366`）、**最后反转为时间升序返回**（`:368-370`）⇒ **"取最早 N 条"已不在消息主路径**。**但旧 `ListMessages`（`Order("id ASC")`）仍在接口上**（`:335-346` + 接口声明 `:39`）—— 死路径。`ListConversations` `:417-432`：`Order("updated_at DESC, id DESC")` + `Limit/Offset`（`:427-429`）⇒ **offset 语义**。InMemory 镜像同语义（`:153-175` / `:249-282`） |
| `emotion-echo-chat-svc/internal/logic/listmessageslogic.go` | `:52` 调 `ListMessagesPage(ctx, id, limit, req.Cursor)` **cursor 已透传**；`:57` 回填 `HasMore`；`:47-50` `limit<=0 → 50` |
| `emotion-echo-chat-svc/internal/logic/listconversationslogic.go` | `:41-48` 默认 `limit=20`/`offset>=0`；`:51` 用 `limit+1` 探测；`:56-59` `hasMore` ⇒ **offset 驱动** |
| `emotion-echo-chat-svc/internal/grpcserver/chat_server.go` | `ListMessages` `:142-163`（`:150` `Cursor: req.GetCursor()`、`:157` `HasMore`）；`ListConversations` `:166-186`（`:172-173` `Limit/Offset`） |
| `emotion-echo-chat-svc/internal/handler/chat_handler.go` | `ListMessagesHandler` `:84-106` **只解析 `:id` 与 `limit`（`:94-98`）—— 不读 cursor** ⇒ **HTTP 降级路径下 cursor 被静默丢弃**（grpc 为默认 transport，故线上不显形） |
| `emotion-echo-web-bff/internal/downstream/chat.go` | 接口 `ListMessages(ctx, convID, limit, cursor) ([]MessageView, bool, error)` `:95`（**含 cursor+hasMore**）；`ListConversations(ctx, limit, offset)` `:98`（**无 cursor**）；`buildMessageQuery` `:275-287` 仅在 `cursor>0` 时附加参数；`ChatClientOptions.Transport` 默认 `"grpc"` `:115`/`:147-149` |
| `emotion-echo-web-bff/internal/downstream/chat_grpc.go` | `ListMessages` `:83-98`（`:88` `Cursor: cursor`、`:97` `resp.GetHasMore()`）⇒ **gRPC 路径全透传**；`ListConversations` `:101-115`（`Limit/Offset`）；`SendMessage` `:64-70` 透传 `ClientMsgId` |
| `emotion-echo-web-bff/internal/handler/chat_handler.go` | `listMessages` `:171-205`：`:182-188` **读 `cursor` 查询参数**、`:198-203` 计算 `nextCursor`、`:204` 返回 `{list, cursor, hasMore}` ⇒ **BFF 侧消息分页已完备**；`listConversations` `:106-129` 只读 `limit`/`offset`（`:107-118`）⇒ **前端发的 `cursor` 被忽略** |
| `emotion-echo-web/app/stores/message.ts` | `messageCursor` `:15`；`loadMoreMessages` `:38-82`（`:46-48` 发 `params.cursor`、`:50-54` 读 `{list,cursor,hasMore}`、`:70-71` 更新）；`switchSession` `:21-36`；`sendMessage` `:84-151` 直接置 `status:'sent'`（`:121`/`:131`）—— **`sending` 从未被赋值、无乐观插入**；`MessageStatus` 本文件另有一份定义 `:7`（5 值，含 `truncated`） |
| `emotion-echo-web/app/stores/conversation.ts` | `cursor = ref<string>('')` `:14`；`fetchConversations` `:91-137` **发 `params.cursor = lastItem.updatedAt`**（`:100-103`、`:119-123`）但只读 `{list, hasMore}`（`:105`）；BFF 读的是 `offset` ⇒ **参数名对不上、offset 恒 0 ⇒ "加载更多"重取第一页并可能重复**。`loadMore` `:142-147` |
| `emotion-echo-web/app/utils/db.ts` | Dexie 单例；**唯一定义 **`version(1).stores({ messages: 'id, sessionId, [sessionId+sendTime]' })`（`:20-22`）⇒ **无 `status`、无独立 `sendTime` 索引**；SSR-safe 懒初始化（`:9-28`） |
| `emotion-echo-web/app/utils/messageCache.ts` | 完整 CRUD（267 行）但**全仓零调用**；`getPendingMessages` `:141-144` / `getFailedMessages` `:150-153` 用 `where({ sessionId, status })` ⇒ **需 `[sessionId+status]` 复合索引，缺失 ⇒ 一调用即抛 SchemaError**；`trimOldMessages` `:204-218` 用 `where('sendTime')` ⇒ **需 `sendTime` 索引，缺失 ⇒ 抛错**；`exportSessionMessages` `:247-250` `.sortBy('sendTime')` 同风险；`retryCount` 恒置 0（`:37`/`:56`） |
| `emotion-echo-web/app/plugins/init.ts` | `setupNetworkListener` `:99-116`：`online` 监听**只有 `console.log` + TODO 注释**（`:105-111`），`retryFailedMessages` 全仓**无定义**；`init.ts:47/77-82` 初始化 user/conversation store，**不触 message store，也不触 messageCache** |
| `emotion-echo-web/app/pages/chat/conversation/[id].vue` | `@scroll="onMessagesScroll"`（`:3`）；`onMessagesScroll` `:524-532`：`scrollTop <= 48` ⇒ `loadMoreMessages()`（`:529-530`）⇒ **翻页触发点已存在**（非"无触发点"）；`scrollToBottom` `:517-519`；`switchSession` `:548` |
| `emotion-echo-web/app/types/conversation/conversationMessagesType.ts` | `MessageStatus` 第二份定义 `:4`（**4 值，无 `truncated`**）；`conversationMessageType` 用 `sender: 'user' \| 'AI'`（`:8`）与 `contentType: 'text'\|'audio'\|'img'`（`:11`）—— 与上游 DTO（`role`、`image`/`file`/`video`）**字段名与取值均不一致** |
| `emotion-echo-chat-svc/internal/model/conversation.go` + `migrations/c002_add_client_msg_id.sql` | `ClientMsgID` 部分唯一索引（`conversation.go:50`；`c002:21-25`）⇒ **幂等底座已在，重试可直接复用** |
| `emotion-echo-chat-svc/internal/logic/sendmessagelogic.go` | `:95-115` 按 `clientMsgId` 先查后插去重 ⇒ 重试同 key 不会重复入库 |
| 测试（现状） | chat-svc：`listmessageslogic_test.go`（8 测例，**无 cursor 分页断言**）、`listconversationslogic_test.go:81`（`HasMore`）、`conversation_repository_list_test.go:73`（`LimitOffset`）、`sendmessagelogic_test.go:394`（`DuplicateClientMsgID`）；BFF：`chat_grpc_test.go:162/182`；前端：`stores/__tests__/message.test.ts:93-116`（mock cursor）、`conversation.test.ts`；**`messageCache` / `retryCount` 在全部 `*.test.ts`/`*.spec.ts` 中零引用**；`e2e/*.spec.ts` 中 `cursor`/`hasMore`/`clientMsgId` **零命中** ⇒ 无 e2e 翻页/幂等钉子 |

### A.2 相关 ADR / 决策（已查）

- **ADR：无**。`grep -rln "cursor|游标|分页|messageCache" docs/architecture/adr/` 仅命中 `adr-2026-09-nuxt-ssr-mode.md`（SSR，不涉分页）⇒ **"分页契约 + 本地缓存职责边界"无任何 ADR 覆盖**。来源计划 §8 已建议补一条 ⇒ 本阶段 Phase 7 必须补（架构级：跨 proto/chat-svc/BFF/前端）。
- 已查 `docs/closure-roadmap/roadmap.md` §4 CL-05：**已定决策**（用户 2026-10-13，按推荐）——
  - **D-B 会话列表 = B1 游标化**（与消息一致，改后端 proto+SQL）；
  - **D-G 多标签页 = defer**（本期不做）；
  - 计划内 [已定]：D-A 游标键 = `id` / backward；D-C 缓存只做"读加速 + 离线队列"；D-D 状态机单一来源；D-E 重试 `MAX=3` / 指数退避 / 复用 `clientMsgId`；D-F trim 治理（500 条 / 30 天）。
- 已查 `docs/closure-roadmap/decisions.md`：CL-D-01~06 均**已定**，本阶段无开工前阻塞决策门。
- 已查 `docs/closure-roadmap/parallel-contract.md`：D 轨文件面 + **3 处与 C 轨相交**（`proto/**`、`chat_svc` gRPC server、BFF `downstream/chat*.go`）⇒ 已在 §四 登记握手/排队（见 §A.5）。
- 历史边界：`docs/e2e-roadmap/stages/e2e-18-cache-layer/plan.md:83` 曾把"前端存储（localStorage/Dexie）验证"列为**不做**并推给 E2E-11/端侧，该归属方**从未接手** ⇒ 挂账至今（`E2E-F-206`）。
- **治理口径**：`E2E-F-205/206` 原属"前端专项（未排期）"；本阶段**正式接棒**，收口时关闭（roadmap §4 CL-05 验收 E 已写死）。

### A.3 架构假设清单（本文假设 vs 现状核对）

> 本节是本次建档的**主要产出**：来源计划（2026-10-11）写在"消息分页尚未实现"的前提上，**该前提已被后续 PR 推翻**。

| 来源计划的假设 | 现状核对（2026-10-14 亲核） | 结论 |
|---|---|---|
| A2「后端**不支持**游标分页 ⟹ 需后端先改契约」（来源计划 §0.3） | **消息已支持**：proto `cursor`（`chat.proto:126-130`）+ `has_more`（`:136-137`）+ `ListMessagesPage`（`conversation_repository.go:350-372`）+ BFF 透传（`chat_handler.go:182-188/204`） | ❌ **假设过期**。消息分页**不是"要做"而是"要验证+补钉"** |
| §5「proto 增 `before_id` / `next_cursor`」 | 实际采用**字段名 `cursor`**（语义同 `id < cursor`、`0`=最新开始），`has_more` 亦已存在 | ❌ **方案已以另一形态落地** ⇒ 本阶段**不重命名**（重命名属破坏性契约变更，收益为零） |
| G2「修'取最早 N 条'缺陷」（来源计划 §2.1-G2） | 消息路径已是 `id DESC`；**但旧 `ListMessages`（`Order("id ASC")`，`conversation_repository.go:335-346`）仍挂在接口上**（`:39`） | ⚠️ **半过期**：生产路径已修，**残留死方法**待按 CL-02 口径裁 |
| §1.3「状态机 3 处不一致枚举」 | 亲核为 **2 处**定义（`stores/message.ts:7` 5 值 vs `conversationMessagesType.ts:4` 4 值）+ `types/api.ts:195-196` 第三处（子代理核） | ⚠️ 数量需以本阶段 RED 断言为准（不做无证据的"3 处"表述） |
| A3「前端**无**翻页触发点」 | **已有**：`[id].vue:3` `@scroll` + `:529-530` `scrollTop<=48 ⇒ loadMoreMessages()` | ❌ **假设过期** ⇒ **不必新增 IntersectionObserver** |
| A1/A6「缓存模块在、零接线；索引缺失」 | ✅ 确认：`messageCache` 全仓零调用；`db.ts:20-22` 缺 `[sessionId+status]` 与 `sendTime` 索引 ⇒ `getFailedMessages`/`trimOldMessages` 调用即抛 | ✅ 成立（本阶段 Phase 2 主项） |
| A5「重试幂等有后端保障」 | ✅ 确认：`client_msg_id` proto 字段（`:94`）+ 部分唯一索引（`conversation.go:50`）+ `sendmessagelogic.go:95-115` 去重 | ✅ 成立（**复用，勿重造**） |
| §1.2「会话列表参数名对不上」 | ✅ 确认：前端发 `cursor`（`conversation.ts:100-103`）、BFF 读 `offset`（`chat_handler.go:107-118`）、chat-svc 用 offset（`conversation_repository.go:427-429`） | ✅ 成立（Phase 5 主项） |
| （未记）chat-svc HTTP 降级路径 | **新发现**：`handler/chat_handler.go:84-106` 不读 cursor ⇒ `CHAT_TRANSPORT=http` 时翻页静默失效 | 🆕 新增 Phase 6 |
| （未记）前端类型与上游 DTO 不一致 | **新发现**：`sender:'user'\|'AI'`（前端）vs `role`（DTO）、`contentType:'text'\|'audio'\|'img'` vs 上游含 `image`/`file`/`video` | 🆕 进 §7 [M] 裁定**是否本阶段统一**（可能牵动多处渲染） |
| 会话列表游标**键** | 排序是 `updated_at DESC, id DESC`（`conversation_repository.go:427`）⇒ **单一 `id` 游标不成立**（新消息会顶起 `updated_at`，行会"跳页"）⇒ 必须**复合游标 `(updated_at, id)`** | ⚠️ **对 D-B 的细化**：用户已定"B1 游标"，此处只需落实为**复合键**（非新决策，但是落地必须写死的一条） |

### A.4 未跑 smoke / 未取运行时证据声明（诚实性，按 RUNBOOK §4）

**本次建档只做静态核实**：**未启动 dev 栈**（19 容器）——
`deploy/.devmode-session` 现由 **B 轨（`owner: lane-cl-b`，`until: 2026-10-14 18:00`）持有**，按契约 §3.1「不得强占」，D 轨排在 **C 之后**；
**未跑** `scripts/smoke_data_layer.py`（§2.4 数据契约 smoke），**未抓** 一次真实 `/api/v1/conversations/:id/messages` 的翻页请求。
⇒ 因此 **Phase 0（基线取证）尚未执行**；本文所有"现状"结论依据均为**源码**（§A.1 的 file:line）而非运行时观测。
**Phase 0 完成后必须回填真实证据**（网关 `:19080` + Bearer 的 curl 输出 / DB 行）；若与源码结论冲突，**以运行时为准并回改本文**。

### A.5 并行协议遵从自检（H1~H9 / AGENTS §8.1 条款 7~10）

| 条款 | 本轨做法 |
|---|---|
| **H1 / 条款 7 只在自己的分支与 PR 上工作** | D 轨 = `feat/cl-05-message-pipeline`（worktree `../Emotion-Echo-cl05`）+ 登记用 `docs/cl-05-registry`；**未** push/rebase 他人分支；他人分支仅 `git show <ref>:<path>` / `git log` 只读 |
| **H2 / 条款 8 登记独立落地，先落地再开工** | 占位登记 + 握手 + roadmap 状态走**独立小 PR**（`docs/cl-05-registry`）；**阶段 PR 不含登记表改动**（只含 `stages/cl-05-message-pipeline/**` + 来源计划 front-matter） |
| **H3 登记表 append-only** | §三 只追加 D 行；§3.1 只追加排 4；§四 只追加本轨行；roadmap 只改 CL-05 自身行 + **追加**一段 D 轨注（不改 B/C 已写的行） |
| **H4 / 条款 9 合并前三查** | 阶段 PR 合并前跑：① `git merge-tree --write-tree HEAD origin/main` = 0 冲突；② `git diff origin/main...HEAD -- docs/closure-roadmap/{parallel-contract,roadmap}.md` = **空**；③ `docker images \| grep <tag>` + `docker ps --format '{{.Image}}'` 确认镜像归属（H5 后缀 tag） |
| **H5 / 条款 10 docker 隔离** | 自建镜像一律 **`emotion-echo/<svc>:cl05`** 后缀 tag；`up` **只指名自己的服务**；用栈前在契约 §3.1 登记、用毕删 `.devmode-session` |
| **H6 worktree 运行态文件** | 起栈 `--env-file <共享目录>/deploy/.env.local`（worktree 内无此文件）；改 Go 的验收 = **「worktree 构建镜像 + 共享目录 up 容器」两步** |
| **H8 合并 FIFO 串行** | 本轨不与 B/C 争合并窗口；**每次他人合并后**重跑 H4① |
| **H9 文件面是硬边界** | 本轨**不动** `deploy/**`、`charts/**`、`shared/pkg/skywalking/**`（B 轨面）、`scripts/check_dead_grpc_inventory.sh`（C 轨面） |

---

## §1 目标与非目标

**目标**（对齐 roadmap §4 CL-05 验收 A~E）

- **G1 会话列表游标化端到端**（D-B=B1）：proto `ListConversationsRequest` + 游标字段 → chat-svc 复合游标 `(updated_at, id)` → BFF（读游标而非 offset）→ 前端 `conversation.ts` 对齐 ⇒ **"加载更多"真能取第二页、不重复、不跳页**。
- **G2 消息分页"验证 + 补钉"**：把**已存在**的 `cursor`/`has_more` 语义用**回归钉钉死**（首屏最新在前、上滚更早、无重复无遗漏），并**消灭残留的 ASC 死方法**。
- **G3 HTTP 降级路径契约一致**：`CHAT_TRANSPORT=http` 下 chat-svc HTTP handler 也认 `cursor`（消除"切回滚开关就静默失效"）。
- **G4 本地缓存接线**：`db.ts` 升 `version(2)` 补索引；`messageCache` 修复后接入真实链路 = **读加速（快照先渲染）+ 写穿（落缓存）+ 离线队列**。
- **G5 状态机规范化**：`MessageStatus` **单一来源** + 明确转移表 + **乐观插入**（用户消息先 `sending`，服务端返回后替换为真实 id 并置 `sent`）。
- **G6 重试**：`retryCount` 自增 + 指数退避 + 上限（`MAX=3`，超限停自动重试、UI 给手动重发）。
- **G7 断网重连重扫**：`online` 事件 → `retryFailedMessages()`，**复用 `clientMsgId`** 保证幂等不重复入库。
- **G8 缓存治理**：`trimSessionMessages(500)` 写后触发 + 启动 `trimOldMessages(30)`。
- **G9 测试补齐**：vitest（缓存/状态机/重试，需新增 `fake-indexeddb` devDep）+ Go（游标语义）+ Playwright 回归钉；**关闭 `E2E-F-205/206`**。

**非目标（本期不做）**

- 多标签页一致性（**D-G 已定 defer**；`BroadcastChannel` 留后续）。
- 多设备/多端同步的冲突解决（不做 CRDT / OT）；消息端到端加密；离线全文检索。
- 后端 `StreamMessages` 流式接口启用（属 CL-02 面，已由 C 轨处置）。
- 重做消息分页契约（`cursor`/`has_more` **已存在**，**不重命名、不改语义**）。
- 端侧（Lane O）离线能力整合——本阶段只做 Web 前端。
- 改动 `sender`/`contentType` 字段命名**若**裁定为"超出本阶段"（见 §7 M-3）——则只登记 `CL-F` 账，不在本阶段动渲染层。

---

## §2 测试点清单（22 点，逐点独立编号）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|---|---|---|---|
| T-1 | **消息首屏取"最新 N 条"**（`id DESC` 语义，非最早 N 条） | [A] | curl 走网关取 `limit=2`，断言返回的是最新两条（与 DB `ORDER BY id DESC LIMIT 2` 比对） | 待执行 |
| T-2 | **消息游标翻页端到端**：带上一页最小 id 作 cursor ⇒ 取更早一页，**无重复、无遗漏** | [A] | curl 两页 + DB 全量比对（并集= 全量、交集= ∅） | 待执行 |
| T-3 | **消息页序稳定**：返回体为时间升序（最早在前、最新在末），前端不再二次排序错乱 | [A] | curl 断言 + Go 单测（`ListMessagesPage` 反转语义） | 待执行 |
| T-4 | **会话列表游标化**：loadMore 带游标 ⇒ 取到第二页、顺序稳定（`updated_at DESC, id DESC`）、无重复 | [A] | curl（会话列表第二页）+ DB 比对 | 待执行 |
| T-5 | **会话游标为复合键**：同 `updated_at` 的不同会话不丢/不重（边界：并发同秒更新） | [A] | Go 表驱动单测（构造同 `updated_at` 多行） | 待执行 |
| T-6 | **会话"加载更多"不再重取第一页** | [A] | 前端 store 单测：第二页请求参数 ≠ 第一页；mock 返回第二页 ⇒ 列表长度增加且 id 无交集 | 待执行 |
| T-7 | **HTTP 降级路径认 cursor**（`CHAT_TRANSPORT=http`） | [A] | 单测（handler 读 `cursor`）+ 运行验证（transport=http 下翻页生效） | 待执行 |
| T-8 | **旧 ASC 死方法清零**：`Order("id ASC")` 的生产调用方 = 0（删或归档，口径对齐 CL-02） | [A] | `grep` 断言 + 架构测试 | 待执行 |
| T-9 | **Dexie 索引修复**：`version(2)` 后 `getPendingMessages` / `getFailedMessages` / `trimOldMessages` **不再抛错** | [A] | vitest + `fake-indexeddb`（**修前先红**） | 待执行 |
| T-10 | **缓存往返**：`saveMessage` → `getLatestMessages`/`getMessagesBySession` 往返一致（含分页 cursor 语义） | [A] | vitest + `fake-indexeddb` | 待执行 |
| T-11 | **读加速**：`switchSession` 先渲染本地快照，再以服务端覆盖/合并 | [A] | store 单测（断言先读缓存后发请求） | 待执行 |
| T-12 | **合并策略**：服务端覆盖同 id；**本地独有 `failed/pending` 不被服务端快照抹掉** | [A] | store 单测（构造本地 failed + 服务端无该条） | 待执行 |
| T-13 | **状态机单一来源**：`MessageStatus` 全仓**仅一处定义**、取值集合唯一 | [A] | 架构测试（`grep` 定义点）+ 类型测试 | ✅ **PASS**（Phase 1，2026-10-14）—— 钉 `app/types/conversation/message-status-single-source.architecture.test.ts` **5/5 绿**；**RED 实测 3 项失败**（2 处定义 / canonical 仅 4 值 / 2 处内联联合），逐项见 `report.md §3.1` |
| T-14 | **乐观插入**：发送瞬间用户消息 `status === 'sending'`（**现恒 `'sent'` ⇒ RED**）；服务端返回后置 `sent` + 换真实 id | [A] | store 单测 | 待执行 |
| T-15 | **非法转移被拒**：`sent → sending` 等非法转移不生效 | [A] | 转移表单测 | 待执行 |
| T-16 | **重试计数 + 上限**：失败后 `retryCount` +1；达 `MAX=3` 停自动重试 | [A] | vitest（fake timers） | 待执行 |
| T-17 | **退避策略**：间隔 = `min(2^n × 1000ms, 30s)` | [A] | vitest（fake timers 断言调用时刻） | 待执行 |
| T-18 | **断网重连重扫**：离线发送 → 置 `failed`；触发 `online` → 按退避重试、**同一 `clientMsgId`**、成功置 `sent` 并移出待重试集 | [A] | store 单测（模拟 `navigator.onLine` + fake timers） | 待执行 |
| T-19 | **重试幂等（不重复入库）** | [A] | 运行验证：断网补发后 DB 该 `client_msg_id` 行数为 1（复用 `c002` 唯一索引） | 待执行 |
| T-20 | **人可见三连**：① 上滚翻到更早页；② 刷新后消息仍在；③ 断网发送 → 联网自动补发成功 | [V] | Playwright（`e2e/message-pipeline.spec.ts`）+ IAB 截图/录屏 | 待执行 |
| T-21 | **账本继承 · 发送后聊天区不自动滚到底**（`CL-F-10`，用户 2026-10-14 IAB 实测；账本已归属 **CL-05**） | [V] | 先**定位根因**（不臆断——账本明写"未查代码"），再 IAB/Playwright 实测：长回复流式增长时视口跟随到末尾 | 待执行（Phase 8，🔴 需栈） |
| T-22 | **账本继承 · 发送并刷新后出现两个对话**（`CL-F-11`，用户 2026-10-14 IAB 实测；账本已归属 **CL-05**） | [A] | 先**定位根因**（疑 `createNewConversation` 建会话→navigateTo→落库链路重入），再运行验证：建会话→发送→刷新，会话列表只 +1；根因定后补单测 | 待执行（Phase 8，🔴 需栈） |

> **负向对照（必做，防"钉子不会红"）**：Phase 2 修索引**前**先跑 T-9 必须**红**；Phase 3 落地**前**先跑 T-14 必须**红**（现状恒 `'sent'`）。
>
> **关联账本（**三处**，收口须逐条对账）**：
> ① `E2E-F-205`（分页契约错位）与 ② `E2E-F-206`（缓存/状态机/重试半成品）置 ✅（`docs/e2e-roadmap/discovered-unresolved.md:439-440`）；
> ③ **账本继承项 `CL-F-10` / `CL-F-11` 的归属方就是 CL-05**（2026-10-14 用户 IAB 实测建立）——已并入本阶段为 **T-21 / T-22**（Phase 8），收口时一并置 ✅。
> 范围外新发现一律登 `docs/closure-roadmap/discovered-unresolved.md`（`CL-F-xx`，**本阶段下一个可用号须现场核对**）。

---

## §3 分阶段实施（TDD：先 RED 回归钉，再 GREEN）

> **栈依赖标记**：🟢 = 不需 dev 栈（单测/静态/架构钉）；🔴 = 需 dev 栈（**排队在 C 之后**，`deploy/.devmode-session` 不得强占）。
> 🔴 的**编码与单测**仍可先行，只有**运行验证**排队 —— 表中"🔴"专指运行验证部分。

| Phase | 内容 | 栈 | RED 断言 |
|---|---|---|---|
| **0** | **基线取证**：起栈（排队）→ curl 走网关 `:19080` 取消息/会话两页现状 → DB 比对；确认 `CHAT_TRANSPORT` 实际值 | 🔴 | —（人工产证据；**未做，见 §A.4**） |
| **1** | **类型与契约收敛（无行为变更）**：`MessageStatus` 收敛到**单一来源**；补 `cursor/hasMore` 类型；`messageCursor`/`cursor` 是否需导出按需定 | 🟢 | T-13（现状两处定义取值集合不一致 ⇒ **红**）—— ✅ **已完成 2026-10-14**（RED 3 项失败 → GREEN 5/5；全量 vitest 654/654 + typecheck 绿；证据 `report.md §3.1`） |
| **2** | **缓存层修复 + 接线**：① `db.ts` 升 `version(2)` 补 `[sessionId+status]` + `sendTime` 索引；② 修 `messageCache.ts`（索引可用后不抛错，补 `markFailed/incRetry` 辅助）；③ `stores/message.ts` 接线（`switchSession` 快照→服务端→`saveMessages`；`loadMoreMessages` 落页；`sendMessage` 落条）；④ 合并策略：服务端覆盖同 id、本地 `failed/pending` 保留 | 🟢 | **T-9（索引缺失下 `getFailedMessages` 抛错 ⇒ 红）** + T-10 / T-11 / T-12 |
| **3** | **状态机 + 乐观插入**：单一转移表（`sending→sent/failed`、`streaming→sent/failed`、`failed→sending`、`sent`/`truncated` 终态 + 非法转移守卫）；用户消息先插 `sending`（temp id = `clientMsgId`），服务端返回后换真实 id 并置 `sent` | 🟢 | **T-14（发送瞬间恒 `'sent'` ⇒ 红）** + T-15 |
| **4** | **重试 + 断网重扫**：`retryCount` 自增 + 指数退避 `min(2^n×1000ms,30s)` + `MAX=3`；`stores/message.ts` 新增 `retryFailedMessages()`；`plugins/init.ts` 把 `:105-111` 的 TODO 换成真实调用（`online` 事件 + 启动时 `navigator.onLine`）；**复用 `clientMsgId`** | 🟢 | T-16 / T-17 / T-18（现状 `retryFailedMessages` 无定义 ⇒ **红**） |
| **5** | **会话列表游标化**（D-B=B1，**需与 C 轨握手 `proto/**`**）：proto `ListConversationsRequest` 增游标字段 → chat-svc `ListConversations` 复合游标 `(updated_at, id)` → BFF `listConversations` 读游标 → 前端 `conversation.ts` 对齐 | 🟢 编码 / 🔴 运行 | T-5（Go 表驱动：同 `updated_at` 边界）+ T-4 / T-6 |
| **6** | **死路径与降级路径**：① 旧 `ListMessages`（`Order("id ASC")`，`conversation_repository.go:335-346`）**调用方清零**（按 CL-02 口径：删 or 归档 + 桩）；② chat-svc `handler/chat_handler.go:84-106` 读 `cursor` | 🟢 编码 / 🔴 运行 | T-8（架构钉）+ T-7 |
| **7** | **端到端 + 人可见钉 + ADR + 收口**：Playwright `e2e/message-pipeline.spec.ts`（T-20 三连）；补 **ADR** `adr-2026-10-frontend-message-pagination-and-cache.md` + `decisions.md` 索引；`report.md`；来源计划迁 `legacy-plans/landed/`；关 `E2E-F-205/206` | 🔴 | T-20（`[V]`） |
| **8** | **账本继承项**（**账本已把归属判给 CL-05**，故属本阶段范围）：① 定位并修复 `CL-F-10`（发送后不自动滚到底）；② 定位并修复 `CL-F-11`（刷新后出现两个对话）。两项均**先定位根因**（账本明写"未查代码、不臆断"），再按 TDD 补钉 | 🔴（需栈复现） | T-21 / T-22（先复现 → 再写钉 → 再修） |

> **TDD 纪律（Phase 1 实跑 `scripts/check_tdd_gate.sh` 后更正）**：RED 先写钉验红、GREEN 再改实现（AGENTS §2.1）。
> ⚠️ 但 CI `tdd-gate` 是**按 commit 判**的：某 commit **新增 ≥5 行生产代码（`.ts/.vue/.go/.py`）而同一 commit 无测试文件**即 FAIL
> ⇒ **实现 commit 必须同时带测试文件改动**，"纯 `test:` commit + 纯 `feat:` commit"两刀切**会被拦**。
> 本轨 Phase 1 的实际落法（可复用）：`test:` commit 加钉验红 → `refactor:` commit 改实现**并同时补钉**（新增"必须真的引用单一来源"断言 + 修 typecheck 严格模式）。
> **改 Go 必须重建镜像再验收**（RUNBOOK §2）；**改 proto 必须复现原生成方式**（`protoc 32.1` + `protoc-gen-go v1.36.11` + `protoc-gen-go-grpc v1.6.2`）。

---

## §4 收口判据（"做到什么程度"，对齐 roadmap §4 CL-05 验收 A~E）

- **A 翻页真生效**：能翻第二页 + 默认先给最新（T-1/T-2/T-3 消息；T-4/T-5/T-6 会话）——**含"加载更多不重取第一页"**。
- **B 刷新不丢**：缓存接线后刷新页面消息仍在（T-10/T-11/T-12/T-20②）。
- **C 断网能补发**：联网自动重试、**幂等不重复**（T-18/T-19/T-20③）。
- **D 状态机 + 重试**：`sending`/`sent`/`failed` 真赋值、`retryCount` 有上限（T-13/T-14/T-15/T-16/T-17）。
- **E 回归钉**：Playwright 翻页/刷新/断网 + 单测全绿；**`E2E-F-205/206` 关闭**（T-20 + 账本对账）。
- **收口动作**：plan 每点四值 + 证据 → `report.md`（模板）→ 补 ADR + `decisions.md` 索引 → 来源计划迁 `docs/legacy-plans/landed/`（同步修链接）→ `roadmap.md` 状态改 `done` → 删占位（契约 §六）→ AGENTS §2.5 三连自检。

---

## §5 回归钉

- **vitest（主钉）**：
  - `app/utils/__tests__/messageCache.spec.ts` —— 索引可用性、save/get 往返、分页 cursor 语义、trim 生效（**`fake-indexeddb`，需新增 devDep**）。
  - `app/stores/__tests__/message.cache.spec.ts` —— 读加速顺序、合并策略（服务端覆盖 / 本地 failed 保留）。
  - `app/stores/__tests__/message.state.test.ts` —— 转移表 + 乐观插入（`sending` 真赋值）+ 非法转移拒绝。
  - `app/stores/__tests__/message.retry.test.ts` —— `retryCount`/退避/上限/`online` 重扫（fake timers）。
  - 架构钉：`MessageStatus` 定义点唯一（`grep` 断言）。
- **Go**：`emotion-echo-chat-svc/internal/repository/conversation_repository_list_test.go` 扩 —— 消息 `id DESC`/cursor 边界、会话**复合游标**（同 `updated_at`）；`listconversationslogic_test.go` 扩游标；BFF `downstream/chat_grpc_test.go` 扩 cursor 透传；架构钉：无 `Order("id ASC")` 生产调用。
- **Playwright（人可见钉，收口必写）**：`emotion-echo-web/e2e/message-pipeline.spec.ts` —— ① 上滚翻到更早页；② 刷新后消息仍在；③ `page.route` abort 模拟断网 → 发送 → 恢复 → 自动补发成功且**不重复**。
  （沿用既有 `e2e/` 约定与网关 `:19080`；memory 教训：`page.request` 不经 `page.route` ⇒ 拦网需两条路径。）

---

## §6 风险与坑

| # | 风险 | 说明 | 规避 |
|---|---|---|---|
| R1 | **来源计划已部分滞后 ⇒ 照抄会做无用功** | 消息分页/翻页触发点都已落地（§A.3） | 本文 §A.3 已逐条核对；Phase 顺序按**现实**重排（先补钉、后会话游标） |
| R2 | **`proto/**` 被 C 轨预占** | Phase 5 需改 `ListConversationsRequest` | **不硬闯**：已在契约 §四 申请握手；**先做 🟢 Phase 1~4**，等 C 轨答复/落地再动 proto |
| R3 | **`chat_server.go` / `downstream/chat*.go` 与 C 轨同文件** | 同名文件串行（§1.2） | 已登记排队；改前确认 C 轨该文件已落地 |
| R4 | **Dexie 升版导致老数据不可读** | 用户本地已有缓存 | `version(2)` **增量加索引**（Dexie 自动迁移），不删旧 store；写迁移测试 |
| R5 | **happy-dom 无 IndexedDB ⇒ 假绿/报错** | 历史教训（memory：TDD 契约 vs 行为测试） | 用 **`fake-indexeddb`**；拒绝 stub 绕过；优先契约/字面量断言 |
| R6 | **乐观插入 + 重试造成重复展示** | UI 错乱 | 统一以 `clientMsgId` 为本地去重键（`message.ts:113-125` 已有雏形）+ 服务端唯一索引兜底 |
| R7 | **改会话列表排序/分页破坏既有 e2e** | 回归 | Phase 5 前先跑 `e2e/chat-core.spec.ts` / `conversation-management.spec.ts` / `chat-flow.spec.ts` 看影响面；行为变更写进 commit message |
| R8 | **dev 栈单锁被强占** | 违反契约 §3.1 / AGENTS §8.1 条款 10 | **不碰 `.devmode-session`**；B 持锁至 18:00，C 在其后；本轨排队第 4 |
| R9 | **改完 Go 未重建镜像 ⇒ 验收假绿** | 历史高频坑 | RUNBOOK §2 铁律；H6：worktree 构建镜像 + 共享目录 `up` 两步 |
| R10 | **登记表与阶段 PR 混在一起（P1/P2 复发）** | 三轨并行的实证问题 | 本轨**严格分离**：登记走 `docs/cl-05-registry`，阶段 PR 只含 `stages/cl-05-message-pipeline/**` + 来源计划 front-matter |
| R11 | **`fake-indexeddb` 是新 devDep** | 触 `package.json`（脊柱，追加式） | 按契约 §1.2「追加式」加行；PR 描述显式标注；与 B/C 无同依赖冲突 |

---

## §7 调研依据（本次实读）

- **代码（14 个文件，本次实读亲核）**：`proto/chat.proto`；chat-svc `internal/{repository/conversation_repository.go, logic/{listmessageslogic.go,listconversationslogic.go,sendmessagelogic.go}, grpcserver/chat_server.go, handler/chat_handler.go, model/conversation.go}` + `migrations/c002_add_client_msg_id.sql`；BFF `internal/{downstream/{chat.go,chat_grpc.go}, handler/chat_handler.go}`；前端 `app/{stores/{message.ts,conversation.ts}, utils/{db.ts,messageCache.ts}, plugins/init.ts, pages/chat/conversation/[id].vue, types/conversation/conversationMessagesType.ts}`（明细见 §A.1）。
- **测试现状**：chat-svc / BFF / vitest / Playwright 四层清单见 §A.1 末行（关键结论：`messageCache`/`retryCount` **零引用**；e2e 无 `cursor`/`hasMore`/`clientMsgId` 断言）。
- **文档**：`docs/plans/frontend-message-cache-offline-resilience.md`（来源计划）、`docs/closure-roadmap/{roadmap.md,RUNBOOK.md,parallel-contract.md,decisions.md,stages/_TEMPLATE.md}`、`docs/_meta/{parallel-tracks.md,multi-track-parallelism.md}`、`AGENTS.md §〇/§2.4/§八/§8.1`、`docs/e2e-roadmap/{discovered-unresolved.md（:439-440 = E2E-F-205/206）,anti-patterns.md}`、`stages/cl-04-prompt-assembly/plan.md`（体例参照）。
- **`grep` 断言**：`messageCache` 全仓仅自身文件命中（零调用方）；ADR 无分页/缓存覆盖；`cursor|hasMore|clientMsgId` 在 `e2e/*.spec.ts` 零命中；`MessageStatus` 两处定义（取值集合 5 vs 4）。
- **未执行**：dev 栈未起（B 持锁）、§2.4 smoke 未跑、运行时翻页未抓（§A.4 声明）。

### [M] 待用户裁定（3 项，均不阻塞 🟢 Phase 1~4）

| # | 事项 | 备选 | 本轨默认建议 |
|---|---|---|---|
| M-1 | **多标签页一致性是否本期做** | 做 / **defer** | **defer**（roadmap §4 已定，**请追认**；避免首期复杂度） |
| M-2 | **SSE 收到的 AI 消息是否也落缓存** | 做 / **stretch 留后** | **留后**（本期只覆盖"拉取 + 发送"两条路径，来源计划 §2.2 亦如此） |
| M-3 | **`sender:'user'\|'AI'` / `contentType:'text'\|'audio'\|'img'` 与上游 DTO（`role` / `image`/`file`/`video`）是否本阶段统一** | 统一 / **只登记不动** | **只登记不动**（牵动多处渲染，超出"消息链路闭环"口径）；若用户要求统一 ⇒ 另立阶段 |
