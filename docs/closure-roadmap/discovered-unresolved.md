---
status: active
priority: high
created: 2026-10-13
type: phase-ledger
---

# 「闭环」阶段发现账本（discovered-unresolved · CL-F-xx）

> 记录本阶段**用户实测 / 建档预探查**中发现、但**不属于当前阶段范围**、未修复的问题。
> 只增不删：已解决项标 ✅ 保留记录。**归属**写未解决的主责阶段（CL-xx，编号见 `roadmap.md`）。

## 账本

| 编号 | 来源 | 现象 | 根因 | 归属 | 状态 |
|------|------|------|------|------|------|
| CL-F-01 | 建档（2026-10-13） | 前端 `messageCache.ts` 全仓零调用（IndexedDB 缓存模块没接线）；`init.ts:108` 离线重试是 TODO | 功能未接线 | CL-05 | 待解决 |
| CL-F-02 | 建档（2026-10-13） | 多模态融合产物 `fused_emotions` 前端零消费（BFF `/emotion/*` 端点齐备无人调） | 功能未接线 | CL-07 | 待解决 |
| CL-F-03 | 建档（2026-10-13） | 对话 prompt 只发 `[system, 最后一条 user]`（多轮＝无记忆单轮） | 组装层缺历史注入 | CL-04 | 待解决 |
| CL-F-04 | 建档（2026-10-13） | GORM 埋点错挂 `Raw()` processor（`shared/pkg/skywalking/gorm_tracing.go:28-31`）⇒ ORM 查询零 span | 注册到错误 processor | CL-01 | 待解决 |
| CL-F-05 | 建档（2026-10-13） | 7 条死 RPC（StreamMessages/Logout/VerifySecurityAnswer/AnalyzeBatch + 3 mental-health）无生产调用方 | proto 预留/未接出 | CL-02 | 待解决 |
| CL-F-06 | A 轨开工（2026-10-14，worktree 内跑守卫） | `scripts/check_git_layout.py --strict` 在 **git worktree** 下恒红：`.git/ 存在` 判 FAIL（同一命令在主工作区 exit 0） | `check_git_layout.py:55` 用 `git_dir.is_dir()` 判定；worktree 里 `.git` 是 **gitdir 指针文件**而非目录 | CL-00（`.github/workflows/**` + `scripts/**` 面） | 待解决 |
| CL-F-07 | A 轨开工（2026-10-14，Phase 0 起栈前置） | 并行契约要求的 **worktree 里缺 `deploy/` 下全部 gitignored 运行时文件**：① 无 `deploy/.env.local` ⇒ 服务级锚点 `env_file: [".env.local"]` 在 worktree 起栈时**报错中断**（loud）；⚠️ 但若为规避而改用 `--env-file` 指向他处，**服务级 env_file 仍缺**，而 `LLM_API_KEY` 是"只从 env_file 读、不从 host 覆盖"（`docker-compose.apps.yml:329/:405`）⇒ **绕过路径上仍有静默降级 mock 的口子**；② 无/看不见 `deploy/.devmode-session` ⇒ dev mode 归属锁在 worktree 方案下**失去协调能力**（各会话 cwd 不同 ⇒ 互不可见，可静默双占）；③ 连带：`docker compose build` 的构建上下文按 compose 文件位置解析 ⇒ **worktree 里改的源码无法直接构建成镜像**（需"worktree build + 共享目录 up"两步） | `.gitignore:129` 忽略 `.env.local`；`git worktree` **只物化 tracked 文件**；RUNBOOK §2 与契约 §五 均默认"共享目录"语义 | CL-00（并行契约 + RUNBOOK 的 worktree 场景安全做法） | 待解决 |
| CL-F-08 | A 轨 Phase 0（2026-10-14，行为验证 `check_image_freshness.sh`） | 镜像新鲜度守卫会**假红**：`emotion-echo-chat-svc` 被判 STALE，但行为已验证**已含** 827f654 的改动（旧代码是 `Order("id ASC")` 取最早 N 条，运行栈 `?limit=2` 返回的是**最新**两条⇒新语义在跑）；同批 `emotion-echo-web-bff` 则为**真陈旧**（响应 `data` 只有 `list`，缺 827f654 新增的 `cursor`/`hasMore`） | 守卫判据是**纯时间比较**（镜像 `Created` vs 该路径最近 commit 的**提交**时间），无法区分"镜像建于工作副本已含改动、改动稍后才 commit"⇒ 8/8 RED 不能直接读作"8 个服务都缺代码" | CL-00（`scripts/check_image_freshness.sh` 判据精度） | 待解决 |
| CL-F-09 | A 轨 Phase 5（2026-10-14，构建后自查） | 两个 worktree 构建的是**同一个镜像 tag**（`emotion-echo/web-bff:v0.1.37`、`emotion-echo/llm-service:v0.1.2`）⇒ 共用 docker daemon 时**互相覆盖**：谁后 build tag 就指向谁，下一个人 `up -d --force-recreate` 就会跑上"最后构建者"的代码。实测：A 轨 10:10 的构建把 web-bff tag 指向 A 轨代码（运行中容器仍是旧镜像 ID） | compose service 用**固定 tag**（自建镜像豁免 digest 的那条门禁正源于此），而 worktree **只隔离工作区、不隔离 docker daemon**；契约 §1.2 的 spine 清单未包含"镜像 tag / 容器"这类共享资源 | CL-00（并行契约需补"共享 docker 资源"一节：tag 命名 / 独占约定 / 用前播报） | 待解决 |
| CL-F-10 | **用户 IAB 实测**（2026-10-14，CL-04 收口轮；A 轨验证多轮记忆时发现） | **发送消息后聊天区未自动滚动到最底部** —— 新回复流式增长时可能停在视口外，需手动滚动才看到末尾 | **待定位（未查代码，不臆断）**：现象在页面侧消息区，疑似 `pages/chat/conversation/[id].vue` 的滚动跟随逻辑 | **CL-05**（该文件在契约 §1.1 属 **C 轨独占列** ⇒ 改动须先握手；也可能牵涉前端公共滚动组件） | 待解决（**用户 2026-10-14 指示：先记录、本轮不修**，"不知道会不会影响到其他轨"） |
| CL-F-11 | **用户 IAB 实测**（2026-10-14，CL-04 收口轮） | **发送并刷新页面后出现两个对话**（同一轮交互在会话列表里出现两条） | **待定位（未查代码，不臆断）**：疑似 `createNewConversation` 的"建会话 → navigateTo → 落库"链路在刷新/重入时重复建会话（前端 `useConversationSender` 属 C 轨独占列） | **CL-05**（会话管理/消息链路；文件面在 C 轨） | 待解决（用户 2026-10-14 报，本轮**先记录**） |
| CL-F-12 | **用户 IAB 实测**（2026-10-14，CL-04 收口轮） | **会话标题没有按第一段对话自动概括生成** —— 列表里显示"对话 #NNN" | 三层现状（A 轨 Phase 0 已核）：① 前端 `AIStreamParams.shouldGenerateTitle` 存在但 `createNewConversation` 的调用点没传 ⇒ 恒 undefined；② **BFF `aiStreamReq` 结构里根本没有该字段 ⇒ 静默丢弃**；③ chat-svc 有 `UpdateTitle`/`UpdateConversation` 能力但**无人调用**（backlog R-13"当前无 GenerateTitle 实现"实证复活） | **CL-05**（chat 链路与 chat_handler/chat-svc 均为 C 轨独占列）；BFF `ai_handler`+llm-service 属 A 轨可动 | ✅ **已解决**（2026-10-14，CL-04 收口后追加 **T-18**：BFF 侧按第一轮对话生成标题，LLM + 规则兜底；只动 A 轨文件面） |
| CL-F-13 | D 轨 Phase 1（2026-10-14，收敛 `MessageStatus` 时顺手核出） | 同名类型 **`MessageWithStatus` 全仓 3 处形态不一致**：① `types/api.ts:195-197` `extends MessageItem { status?: MessageStatus }`（**无** `retryCount`）；② `types/conversation/conversationMessagesType.ts:23-26` `extends StoredMessage { status: MessageStatus; retryCount: number }`（`status` **必填**）；③ `utils/messageCache.ts:16-19` 文件内私有 `interface MessageWithStatus extends StoredMessage { status?; retryCount? }`（两者**都可选**、未导出）—— 同一语义三套可选性，赋值处仅靠结构兼容勉强通过 | T-13 只收敛了 `MessageStatus` **类型本身**（取值集合），未收敛**承载它的接口**；Phase 3（状态机）先只需其一，故顺延 | **CL-05**（消息链路类型面） | 待解决（**本阶段顺延**：Phase 3/7 触及 `MessageWithStatus` 时一并评估合并）。用户 **`CL-D-13` 已裁定**「顺延到 Phase 3 一并评估」 |
| CL-F-14 | D 轨登记 PR 合并后按 **H8** 复验（2026-10-14） | **append-only 共享文档的「同点插入」冲突**：`decisions.md` 上 C 轨在 `CL-D-07` 后插 `CL-D-08~10`、D 轨（PR #221）在同处插 `CL-D-11~14` ⇒ `git merge-tree` 报 `CONFLICT (content) in docs/closure-roadmap/decisions.md`。**两侧语义零重叠**（不相交的两段编号），纯粹是插入点相同；H4 三查全过仍无法避免（H4 只证"我不会回退别人已合内容"，不证"我不会让别人撞车"） | `decisions.md` / `discovered-unresolved.md` 这类**追加式共享文档**，多轨各自追加时必然落在同一锚点（表尾/某行之后）；H3「append-only 只改自己行」防的是"改他人行"，防不了"同点插入"。**已实测**：`merge=union` 被评估后暂不启用（rebase 重放会产重复行，见 multi-track-parallelism §3 H3 注） | **CL-00**（并行契约需补条款：追加式共享文档的**串行/分片**约定；候选解法 = 每轨一片 `decisions.d/<track>.md` / 账本分片，即 multi-track-parallelism §3 注 §4 早已列出的"登记 fragment 化"候选） | 待解决（**本轮已按 H1 把冲突上报 C 轨 PR #214**，由 C 轨自行 rebase 保留双方行；D 轨不代改他人分支） |

> 上表为建档期预探查的**种子账**（对应各待排期计划的核心缺口）；用户实测中新增的发现续号追加。

## 使用约定

- 6 列硬契约：`编号 | 来源 | 现象 | 根因 | 归属 | 状态`（缺列会被审计漏读）。
- 编号续 **CL-F-xx**（**不占用** E2E 的 `E2E-F-xx`）。
- 确认是运行时 bug 且修复落地后 → 标 ✅ 并回填到对应 landed 计划或 `docs/plans/known-issues-*`。
- 归属方未确定时写"待定 + 触发条件"，**不挂已 done 阶段**（防归属误判）。
