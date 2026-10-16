# ADR-2026-10: 前端消息链路 —— 游标分页契约、本地缓存接线与滚动跟随判据

## Status

🟢 **Accepted**（2026-10-16，CL-05「消息链路闭环」Phase 5 / 8 落地；决策 42 同日登记）

## Context

CL-05 执行期走**网关实测基线**（`evidence/phase0-baseline-output.txt`）抓到三个互相独立的缺陷，
它们共同构成"消息链路看起来能用、其实契约错位"的现状：

1. **会话列表的游标参数被完全忽略**（账本 `E2E-F-205`）。前端 `conversation.ts` 早就在
   `loadMore` 时把 `lastItem.updatedAt` 当游标发出去，而后端（chat-svc / BFF）只认
   `limit/offset` ⇒ **第 2 页响应 = 第 1 页**（实测交集 `{683,684}`）。
   响应体也只有 `{list, hasMore}`，前端**没有**可推进的 nextCursor。

2. **会话列表的游标天生不可能是单值**。稳定序是 `updated_at DESC, id DESC`，而"同一时刻"
   更新的多个会话（同一事务 `NOW()` 取事务开始时间 ⇒ 同值；种子数据也常同值）
   `updated_at` **完全相同**。只比 `updated_at` 时 `updated_at < cursor` 对这批同刻行**恒假**
   ⇒ 翻页会**整批跳过（漏行）**。另外前端当时用的是 **RFC3339 秒级**字符串，
   连"同一秒内区分"都做不到。

3. **本地缓存 / 状态机 / 重试是半成品**（账本 `E2E-F-206`）：`messageCache.ts`/`retryCount`
   全仓**零调用方**，Dexie 表缺 `sendTime` 索引（`trimOldMessages` 一调即抛
   `SchemaError`），用户消息一律 `sent`（没有"发送中"态）。
   另有用户实测两项：发送后**不自动滚到底**（`CL-F-10`）、发送并刷新后出现**两个对话**（`CL-F-11`）。

## Decision

### 1. 游标由 **chat-svc 独占编码**，对 BFF / 前端**不透明**

- 编码 `<unixMicros>:<id>` —— **复合键**（`updated_at` + `id` 做 tie-break），
  **微秒**精度（PG `timestamptz` 的精度就是微秒；降到毫秒会截断，截断后的值既不 `<` 也不 `=`
  真实值 ⇒ 连 tie-break 都失效，等于把 bug 换个形状）。
- 传输形态：proto `ListConversationsRequest.cursor`（string）+ `Response.next_cursor`；
  BFF 与前端**只做原样透传/回传**，不解析 ⇒ 将来换编码不会外溢到其他层。
- 分页语义：**keyset**（`WHERE updated_at < ? OR (updated_at = ? AND id < ?)`），
  仓库层做 `limit+1` 探测并返回 `hasMore`。
- 非法游标 **报 400**（`validation:` 前缀 ⇒ grpcerr ⇒ `InvalidArgument`），
  **不得**静默退回首页（否则翻页会悄悄重来而前端无法察觉）。
- 保留 `offset` 路径仅作旧客户端兼容（`cursor` 为空且 `offset>0` 时走）。

### 2. 消息缓存的合并语义**必须按场景分开**

"服务端快照替换"与"分页追加"语义**相反**：前者服务端给的是**权威全集**，
后者只给**一页**。若共用一个函数，第 2 页不含第 1 页的行 ⇒ 替换式合并会把已加载的旧页**整片删掉**
（表现为"上滚加载更早消息后，下方消息消失"）。
故拆成两个具名纯函数：`mergeServerMessages`（快照替换，服务端覆盖同 id、**保留本地
`failed`/`sending`**）与 `appendServerPage`（按 id 去重后追加）。

### 3. 滚动跟随判据：**"用户自己发的" 与 "远端流式增长" 分开**

只 `watch(currentMessages.length)` 会漏掉**流式同条变长**
（`useConversationSender`: `tempAiMessage.content += delta`，条数不变）⇒ 这正是 `CL-F-10` 的根因。
但也不能简单地"凡变就滚"：① 上翻加载更早消息是**前插**，滚到底会把看历史的用户硬拽走；
② 只加"用户是否在底部附近"守卫，则**用户上翻后按发送时连自己的消息都看不见**
（Playwright 实测视口停在 1519px 之外）。最终判据两条并用：

- 末条 `sender === 'user'` ⇒ **无条件**跟随（按发送 = 明确意图"我要看这条的回复"）；
- 其余尾部增长（AI 流式变长 / 末条被占位行替换）⇒ 仅当原本就在底部附近时跟随。

判据实现为**纯函数** `app/utils/chatScroll.ts`（不碰 DOM ⇒ 可单测）。

## Consequences

- ✅ 会话列表翻页**真正生效**：实测 13 页 506/506 不漏不重；第二页与第一页零交集
  （改动前两者恒等）。消息列表同样与 **DB 全量对账**通过（`evidence/phase5-verify-*.txt`，
  gRPC 与 HTTP 降级路径各 17/17 PASS）。
- ✅ 刷新不丢消息、断网补发**幂等**（同一 `client_msg_id` 只入库 1 行，Playwright 实测）。
- ✅ 两套宿主路径（gRPC 与 `CHAT_TRANSPORT=http` 回滚开关）对同一个非法游标给出**同一状态码**
  （HTTP 侧补 `statusForLogicErr`；此前 400 vs 500 漂移——这条路径平时无人跑，漂移不会自己暴露）。
- ⚠️ **残留**：本地缓存未按用户隔离（`CL-F-20`）⇒ 本阶段**只接 `online` 事件**补发，
  **不接**启动即补发（登录前补发会拿新账号令牌补旧账号的消息）。正解 = 缓存行加 `userId`。
- ⚠️ `CL-F-11`（刷新后两个对话）在 `/new` 表单路径上**未能复现**（回归钉实测：一次发送恰好 1 个会话、
  刷新后仍为 1）⇒ 根因**仍未定位**，账本保持 open，**不宣称修复**。
- ⚠️ 上翻加载更早消息时**不保留精确滚动偏移**（只保证"不把用户拽走"）；要精确保持需在
  DOM 更新前后取 `scrollHeight` 差补偿，属后续项。

## Alternatives rejected

- **游标只带 `updated_at`（或 RFC3339 秒串）** —— 同刻行漏行；已用 DB 构造同刻多行实测反证。
- **游标用毫秒** —— 精度截断后 tie-break 失效（同上，形状变了的同一个 bug）。
- **BFF / 前端自己解析游标** —— 编码一旦要改就要三处同改，且各层都能"猜"错语义 ⇒ 不透明化。
- **非法游标静默回首页** —— 翻页会悄悄重来，前端与用户都无法察觉。
- **一个合并函数通吃快照与分页** —— 见 §Decision 2 的"整片删旧页"失败模式。
- **`MessageStatus` 多份字面量联合** —— 已收敛到 `types/conversation/conversationMessagesType.ts`
  单一来源（架构钉 `message-status-single-source.architecture.test.ts`）。

## References

- 账本：`docs/closure-roadmap/discovered-unresolved.md`（`CL-F-18`~`CL-F-21`）、
  `docs/e2e-roadmap/discovered-unresolved.md`（`E2E-F-205`/`E2E-F-206`）
- 计划与记录：`docs/closure-roadmap/stages/cl-05-message-pipeline/{plan,report}.md`
- 证据：`…/evidence/{phase0-baseline-output,phase5-verify-grpc-path,phase5-verify-http-fallback-path,phase7-playwright-run}.txt`
- 决策：`docs/closure-roadmap/decisions.md`（`CL-D-11`~`CL-D-14`）、`决策 42`
