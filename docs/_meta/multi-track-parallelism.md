---
status: active
priority: high
created: 2026-10-14
type: protocol-hardening
evidence-round: 2026-10-14 CL-04/CL-01/CL-02 三轨并行
related:
  - ../AGENTS.md §八（效力来源）
  - parallel-tracks.md（Lane E/O 跨大阶段协议）
  - ../closure-roadmap/parallel-contract.md（CL 阶段内并行契约；§3.1 dev 栈排队表）
---

# 多轨并行协议（硬化版）

> **一句话**：**一轨一 worktree 一分支，只动自己的 PR；共享登记走独立小 PR 先落地；合并前机器复验"不会回退别人"。**
> 本文件是 [AGENTS.md §八](../AGENTS.md) 的**细则补充**，适用于任何"N 条轨同时开工"的场景（CL 阶段、Lane E/O、以及未来的多 Agent 并行）。

## §1 为什么有这份文件（2026-10-14 三轨并行的六类实证问题）

> 这一轮 A（CL-04）/ B（CL-01）/ C（CL-02）三轨并行，暴露的不是"纪律不好"，而是**协议本身缺了机器可检的约束**。
> 每条都附本轮真实证据，便于后续复查是否真的被堵住。

| # | 问题 | 本轮证据 | 后果 |
|---|---|---|---|
| **P1** | **占用登记只活在分支里** → main 的占用信息失真 | main 的 §三 长期只显示 A 轨 `open（待另一会话）` 且**完全没有 B 行**（登记写在各轨分支上，PR 都是 draft） | C 轨按契约"先读本文件确认无人占用"时读到**错误状态**；最终由 C 轨**按事实回填 A/B**（PR #215）——同一条登记被写三遍 |
| **P2** | **多轨同改同一份登记文件** | `git merge-tree --write-tree feat/cl-04-prompt-assembly 4687e8a` 命中 `parallel-contract.md`（§三 相邻行 + §四 同一空行被双方替换） | 谁后合谁手工解冲突；B 轨停工后冲突悬空，A 轨无法单独收口 |
| **P3** | **基于旧 main 的分支会把别人已合的东西回退** | ① A 轨首次合并前发现会回退 CL-00 收口（#210）的"清占位/标 done"；② A 轨一度做出 `docs/cl-parallel-registry` PR（#216），而 main 已由 #215 完成同一件事 ⇒ **若合并会删掉 C 轨的 C 行与 §3.1**，已主动撤回 | **最危险**：不报错、静默回退别人的成果（本轮已发生 2 次，均在合并前被 `merge-tree`/人工核对拦下） |
| **P4** | **worktree 缺 gitignored 运行态文件** | `deploy/.env.local` 在 worktree 不存在 ⇒ 起栈**报错中断**（服务级 `env_file` 锚点）；`.devmode-session` 在 worktree 也不存在 ⇒ **锁对 worktree 会话不可见**（可静默双占）。账本 **CL-F-07** | 并行轨起栈/占锁都可能踩空 |
| **P5** | **两个 worktree 构建同一个镜像 tag** | `emotion-echo/web-bff:v0.1.37` / `llm-service:v0.1.2` 被 A 轨构建覆盖；later build 赢，下一次 `up --force-recreate` 就跑上"最后构建者"的代码。账本 **CL-F-09** | "本地验收通过"可能是**验了别人的代码**（AP 级失真） |
| **P6** | **本地守卫在 worktree/本机上有假红** | `check_git_layout.py:55` 用 `is_dir()` 判 `.git` ⇒ worktree 恒红（CL-F-06）；`gofmt -l` 在本机（工作区为 CRLF）把**全仓**文件都列出来 | 假红稀释真红（AP-11 家族），并浪费排查时间 |

## §2 业界怎么做（本轮实查，附出处）

| 做法 | 机制 | 对本仓的适用性 |
|---|---|---|
| **一 Agent / 一任务 = 一 worktree + 一分支** | 现在的主流 agentic 开发实践（Claude Code CLI 已原生支持 worktree）；文中点名的失败模式就是"两个 Agent 改同一文件 → 互相覆盖" | ✅ 与本仓 `parallel-contract` §五 一致，已实践；**仍不足**——它只隔离工作区，**不隔离 docker daemon / 共享登记文件 / main** |
| **append-only 文件用 `merge=union`** | `.gitattributes` 声明 `merge=union`：三方合并时**两侧的行都保留**、不产生冲突标记。官方文档明确其用途就是"多分支向同一文件追加行" | ⚠️ **评估后暂不启用**（理由见 §3 H3 注）：文档同时警告"行序可能随机"且"不了解影响勿用"；在 **rebase 重放**同一行时可能产出**重复行** |
| **Merge queue（FIFO 串行 + 复验）** | GitHub merge queue：把 PR 按 FIFO 排队，只对"**最新 base + 队列在前的 PR**"跑必需检查，通过才合 | ✅ 采纳其**语义**：合并串行 + 每次合并后对剩余在飞 PR 复验（本仓人工执行，见 H8） |
| **路径级所有权（CODEOWNERS）** | 用机器可读的路径规则把目录/文件划给 owner，越界即拒 | ✅ 本仓等价物 = `parallel-contract` §1.1/§1.2 文件面表 + 守卫脚本；本文件把"**非文件**共享资源"（镜像 tag / 容器 / 锁）一并纳入 |
| **共享可变资源 = 一把锁** | 单实例资源用租约/锁文件串行化 | ✅ 已在用（`deploy/.devmode-session` + contract §3.1 dev 栈时间片排队表） |

**调研出处**：[git-attributes（`merge=union` 机制与警告）](https://git-scm.com/docs/gitattributes) ·
[GitHub merge queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue) ·
worktree-per-agent 实践与"同文件互覆盖"失败模式（[掘金实战指南](https://juejin.cn/post/7648477668240785423)） ·
append-only 合并驱动实例（[DFHack 对 changelog 配 `merge=union`](https://github.com/DFHack/scripts/pull/381/files)）。

## §3 硬化条款（H1~H9，本仓生效）

> 前 4 条直接回答"**只在自己的 PR 上工作**"与"**不得影响另一轨**"；后 5 条是共享资源与合并纪律。

**H1 —— 只在自己的分支 / 自己的 PR 上工作（硬）**
一轨 = 一分支 = 一 PR。禁止：push 到他人分支、rebase 他人分支、代他人解冲突、把他人分支的提交合进自己分支（`rebase --onto <他人分支>` 同理）。
只能**只读**他人分支：`git log` / `git ls-remote` / `git merge-tree` / `git show <ref>:<path>`（**不 checkout**）。

**H2 —— 登记独立落地，先落地再开工（硬）**
占位登记（§三）与握手（§四）走**独立小 PR**（纯文档、秒合），**合并进 main 之后**才算占用生效、才允许开工。
阶段 PR **不再修改登记表**（避免 P1/P2：登记只活分支 ⇒ main 失真；阶段 PR 动登记 ⇒ 同表冲突）。
> 例外：本轨自己的 stage 目录（`stages/<cl-NN>/`）、账本自己的新增行，仍随阶段 PR 走。

**H3 —— 登记表 append-only（硬）**
每条轨**只追加自己那一行**，**不得改动他人行**；合并冲突时**保留双方行**。
**注（`merge=union` 为何暂不启用）**：它能让两侧行自动共存，但在 **rebase 重放**"同一行的修改"时会产出**重复行**（如 `A | open` 与 `A | in-progress` 并存），而重复行比冲突更难发现；本仓选择"人工保留双方行 + H4 机器复验"。
**下一步候选**：把两表改成**每轨一片**（`parallel-contract.d/<track>.md`），从结构上消灭同表冲突——待三轨收口时评估。

**H4 —— 合并前三查（硬，机器可检）**
1. `git merge-tree --write-tree HEAD origin/main` ⇒ **必须 0 冲突**；
2. `git diff origin/main...HEAD -- <共享文档>` ⇒ **必须为空**（非空 = 你在动共享登记/别人也在动的文件；先问"我这版是不是比 main 旧"）；
3. `docker images | grep <tag>` + `docker ps --format '{{.Image}}'` ⇒ 确认**自己没被别人覆盖、也没覆盖别人**（H5）。
任一项不通过 ⇒ **停手上报**，不要硬合。

**H5 —— 共享 docker 资源的隔离（硬）**
两个 worktree 共用同一个 docker daemon ⇒ 镜像 tag / 容器是**共享可变资源**：
- 构建用**带轨后缀的 tag**（如 `emotion-echo/web-bff:cl04`），不覆盖公共 tag；
- `up` **指名自己的服务**，不整栈 `up`；
- 用栈前在 contract §3.1 排队表登记，**用毕删 `.devmode-session`**；
- 交付验收前用 H4③ 复核"跑的是不是我的代码"。

**H6 —— worktree 运行态文件规则（硬，CL-F-07 解法）**
- 起栈必须 `--env-file <共享目录>/deploy/.env.local`（worktree 内没有该文件）；
- 锁文件写在**共享目录**（`deploy/.devmode-session`），并**显式声明"worktree 不可见"**；
- 改 Go 后的验收 = **「worktree 构建镜像 + 共享目录 up 容器」两步**（已验证可行）。

**H7 —— 他人分支只读（硬）**：见 H1 末段。

**H8 —— 合并 FIFO 串行 + 合后复验（硬）**
同一时刻只合一个 PR；**每次合并后**，对尚未合并的在飞 PR 重跑 H4①（`merge-tree`），把"落后于新 main"的立即暴露出来（这正是 P3 的根因）。

**H9 —— 文件面表是硬边界，且必须覆盖"非文件"资源（硬）**
`parallel-contract` §1.1/§1.2 的独占列表是 PR 拒绝依据；新增触碰须先握手。**本文件补充**：共享资源不只文件，还包括 **镜像 tag / 容器 / dev 栈 / 锁**，一并在 §3.1 与 H5 里管。

## §4 可选的机器化（下一步）

| 项 | 内容 | 收益 |
|---|---|---|
| `scripts/check_multitrack_safety.sh` | 把 H4 三查脚本化（`merge-tree` / 共享文档空 diff / 镜像归属），纳入收口检查 | 把 P3 从"人记得查"变成"忘了就红" |
| CI 对 PR 跑 `merge-tree` | 在 `pull_request` 上断言与 base 无冲突，冲突即红 | 同 P3/H8 |
| 登记 fragment 化 | `parallel-contract.d/<track>.md` 每轨一片 | 结构性消灭 P2（H3 注的候选） |

## §5 与本仓既有文档的关系

- [AGENTS.md §八](../AGENTS.md)：**效力来源**（违反按 §六 处置）；本文件是它的细则补充。
- [parallel-tracks.md](parallel-tracks.md)：Lane E/O 跨大阶段协议（含握手行格式、收工三查）。
- [closure-roadmap/parallel-contract.md](../closure-roadmap/parallel-contract.md)：CL 阶段内契约（文件面表 / 占位登记 / 握手区 / §3.1 dev 栈排队表）。
- 账本：**CL-F-06 / 07 / 08 / 09** 是本文 P4/P5/P6 的原始记录。
