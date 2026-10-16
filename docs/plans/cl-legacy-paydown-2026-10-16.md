---
status: partial
priority: high
created: 2026-10-16
type: backlog-paydown-plan
owner: Lane-CL-P（本轮）
related:
  - ../closure-roadmap/discovered-unresolved.md（CL-F-xx 账本，唯一事实源）
  - ../closure-roadmap/roadmap.md（§2 阶段总表：CL-09 = backlog 清销）
  - ../closure-roadmap/parallel-contract.md（§1.0 字母体系 / §三 占位）
---

# 「闭环」阶段 · 遗留问题整理与清销（CL legacy paydown）

> **范围**：只清 **「闭环」阶段账本 `docs/closure-roadmap/discovered-unresolved.md`（CL-F-xx）** 的未解决项。
> **不含** `docs/e2e-roadmap/` 的 `E2E-F-xx`（用户 2026-10-16 明示「e2e 残留不用」）。
> **收口（2026-10-16）**：第 1 批（PR [#245](https://github.com/Exist-a/emotion-echo/pull/245) → `2a3301cf`）与第 2 批（PR [#246](https://github.com/Exist-a/emotion-echo/pull/246) → `9059735a`）**均已合并**。
> 本计划**未 fully done** —— **Wave 2（需 dev 栈的 5 条）尚待起栈窗口执行**，故 `status: partial`。

> **口径**：沿用 `docs/e2e-roadmap/debt-paydown-plan.md` 的单一判据 ——
> **必要 = 现在不做的确会造成未被发现的危害；不必 = 只是账面整洁，当前无实际危害。**
> 并遵守反自欺三规则（P1 不许改措辞消账 / P2 复核后发现没解决就退回未解决 / P3 补账≠重跑）。

---

## §0 一句话结论

CL 账本共 **26 条**，两批清销后：**已解决 / 已关闭 20 条**、
**2 条如实降级为「已知限制」/「待用户裁定」**（CL-F-08 / CL-F-19）、
**仅剩 5 条未解决**（CL-F-16 / 22 / 23 / 24 / 26 —— 全部**需要 dev 栈** = Wave 2，待起栈窗口执行）。

- **第 1 批**（PR [#245](https://github.com/Exist-a/emotion-echo/pull/245)，已合并 `2a3301cf`）：修 CL-F-06 / 17 / 18 / 20；
  文档收口 CL-F-21；账本对账翻正 CL-F-01 / 03。
- **第 2 批**（PR #246）：流程硬化 —— CL-F-07（RUNBOOK §2.1）/ CL-F-09（契约 §1.3）关闭；
  CL-F-08 降级为已知限制（附核对程序）；CL-F-19 转 `CL-D-20` 待用户裁定。

---

## §1 清点（2026-10-16 逐条复核，非照抄账本）

### 1.1 已在历史阶段关闭（12 条，无需动作）

| 编号 | 关闭依据（复核方式） |
|---|---|
| CL-F-02 | CL-07 前端半（PR #243/#244）已消费融合情绪；账本已标 ✅ |
| CL-F-04 | CL-01 Phase 1 修 GORM processor + 注册期快照；账本已标 ✅（附 A/B 证据） |
| CL-F-05 | CL-02 逐条终态（清除 3 / 归档 4）+ 守卫升级；账本已标 ✅ |
| CL-F-10 | CL-05 Phase 8 / T-21 修复滚动跟随；账本已标 ✅ |
| CL-F-11 | 无法复现（用户裁定 CL-D-15）；账本已标 🟢关闭 |
| CL-F-12 | CL-04 收口后追加 T-18（BFF 侧生成标题）；账本已标 ✅ |
| CL-F-13 | 假账，当轮自纠（探针选择器臆造）；账本已销账 |
| CL-F-14 | CL-01 修 tracer 竞态 + 起栈 A/B 负向对照；账本已标 ✅ |
| CL-F-15 | CL-03 Phase 5 改 `UnavailablePublisher` + 后台重试；账本已标 ✅ |

### 1.2 本轮对账翻正（2 条 —— 代码已通、账本滞后，属 AP-04「账本与状态脱钩」）

| 编号 | 复核事实（可复跑） | 处置 |
|---|---|---|
| **CL-F-01** | `utils/messageCache.ts` **已被接线**：`stores/message.ts:6-12` 导入并调用 `getLatestMessages/getAllFailedMessages/saveMessage/saveMessages/deleteMessage`；`plugins/init.ts:111` 的 `online` 监听调 `retryFailedMessages()`（**不再是 TODO**） | 翻 ✅（依据 CL-05 Phase 2/4） |
| **CL-F-03** | **多轮历史已注入**：BFF `internal/handler/prompt_assembly.go:150-155` `assembleMessages` 组装 `[system, ...history, user]`，`historyMessages(in)` 取同会话历史（CL-04 落地物，`prompt_assembly_test.go` 覆盖） | 翻 ✅（依据 CL-04） |

### 1.3 本轮修复并关闭（4 条 —— 全部 TDD，无 dev 栈）

| 编号 | 现象 | 修法 | 证据 |
|---|---|---|---|
| **CL-F-06** | `scripts/check_git_layout.py` 在 git **worktree** 下恒红（`.git` 是指针文件非目录） | 抽出纯函数 `is_git_initialized(root)`：认「目录」也认「内容以 `gitdir:` 开头的文件」 | 新增 `scripts/test_check_git_layout.py`（6 例，含"普通文件不算仓库"负向对照）→ RED(1 fail+5 error) → GREEN(6/6)；工具本体 `--strict` exit 0 |
| **CL-F-17** | 数字人 VRM 不显示，`TypeError: … reading 'add'` | **真实成因链**（亲核）：`loadVRM` 的 `await loader.loadAsync`（26.7MB VRM）有秒级窗口 → 期间卸载触发 `disposeThreeJS()` 把 `scene` 置 null（`:444`）→ await 恢复后**不再看 scene** 直接 `scene.add` ⇒ 空指针。入口早退检查发生在 await **之前**，挡不住。修 = 置 `disposed` 标记（`onUnmounted` 先置位）+ await 后重新校验 | 新增 `app/components/digital-human/clf17-vrm-load-guard.architecture.test.ts`（结构性不变量：await→首次 `scene.add` 之间必有 scene/disposed 再校验）→ RED(2/3 fail) → GREEN(3/3) |
| **CL-F-18** | `MessageWithStatus` 全仓 **3 处**同名声明、形态不一致 | 删**逐字冗余**的第③份（`utils/messageCache.ts` 内私有版 ≡ 已导出的 `StoredMessage`），统一改用 `StoredMessage`；保留的①②各自补**边界注释**并加契约钉。⚠️ ①②**结构性不等价**（`sender: 'user'\|'ai'` vs `'user'\|'AI'`），合并须先统一拼写 ⇒ 被用户裁定 **`CL-D-12`「只登记不动」**阻塞 | 新增 `app/types/message-with-status-single-source.architecture.test.ts`（5 例：声明点恰为 2、③不得复现、必须引用 StoredMessage、两处须带边界注释）→ RED(3 fail) → GREEN(5/5) |
| **CL-F-20** | 本地缓存不按用户隔离 ⇒ 换账号后会拿新令牌补旧账号的消息 | `StoredMessage` 加 `userId`；`db.ts` v3 索引（`userId` + `[userId+status]`）；`saveMessage/saveMessages` 落归属、`getAllFailedMessages(userId)` 按用户过滤；`stores/message.ts` **补发前门控**：未登录一律不补发、只补当前用户自己的行 | 新增 `app/stores/__tests__/message.user-isolation.test.ts`（6 例：两账号互不可见 / 换账号不补旧账号 / 本账号正常补 / 未登录不补 / schema 含 userId 索引）→ RED(4 fail) → GREEN(6/6) |

### 1.4 本轮文档收口（1 条）

| 编号 | 处置 |
|---|---|
| **CL-F-21** | 契约并存「阶段面字母 A~G」与「会话字母 Lane-CL-X」且无对照说明 ⇒ 「独占列」可被读反。已在 `parallel-contract.md` **新增 §1.0 字母体系（唯一权威）**：给两套字母的**逐行对照表** + 判定口径「**认阶段号，不认字母**」，并在 §1.1 表头标注字母来源。仅追加，未改他人行 |

### 1.5 仍待解决（9 条）

| 编号 | 归属 | 阻塞/触发条件 | 分批 |
|---|---|---|---|
| ~~CL-F-07~~ | CL-00 | ✅ **已解决（第 2 批）**：RUNBOOK **§2.1** W1~W4 + 契约 §1.3 | ✅ |
| CL-F-08 | CL-00 | 🟡 **第 2 批：显式降级为已知限制 + 文档化核对程序**（判据修正需起栈验证，改错判据更危险；根治触发 = 引入 build revision label） | 🟡 降级 |
| ~~CL-F-09~~ | CL-00 | ✅ **已解决（第 2 批）**：契约 **§1.3**「共享 docker 资源」R1~R4 | ✅ |
| CL-F-19 | CL-00 | 🟡 **第 2 批：转 `CL-D-20` 待用户裁定**（分片化 vs 单文件串行合并 = 用户口径，执行者不单方决定）；过渡措施 H3 + §1.0 已生效 | 🟡 待裁 |
| CL-F-16 | CL-09 | `/health/ready` 启动期快照语义 ⇒ 依赖后起时永久 503 不自愈（已定位，未做 postgres 重启自愈对照） | Wave 2（需栈） |
| CL-F-22 | CL-09 | outbox DDL **两份事实源漂移**（`migrations/c001` vs chat-svc `main.go` 内联） | Wave 2（需栈） |
| CL-F-23 | CL-09 | DLQ「真实剩余深度」无指标（需实测 `kafka_topic_partition_oldest_offset` 可采性） | Wave 2（需栈） |
| CL-F-24 | CL-09 | Grafana provisioning 的 PG 数据源**明文口令且 dev/prod 同一份** | Wave 2（配置，需栈验证面板） |
| CL-F-26 | CL-09 | `EmotionDistributionByModality` 算了但从不出（生产未接线 + proto 无字段） | Wave 2（跨 proto/analytics，需栈） |

> CL-F-25 属 **E2E-30 面**（§8 镜像新鲜度判据），按用户 2026-10-16「e2e 残留不用」**不在本轮范围**。

---

## §2 分批执行计划

### Wave 1（本轮 · 已执行）：无 dev 栈、可 TDD 的修复
CL-F-06 · CL-F-17 · CL-F-18 · CL-F-20 + 文档 CL-F-21 + 对账 CL-F-01/03。
**共同特征**：纯仓库/前端，`vitest` + `python unittest` 可测，不碰生产链路。

### Wave 2（需 dev 栈 / 碰生产链路，须按 RUNBOOK 取锁）：CL-F-16 · 22 · 23 · 24 · 26
建议**一次取锁跑完**（同一栈窗口）：
1. **CL-F-22**（DDL 收敛）先做 —— 它会动 `migrations/`，与 CL-F-23/26 的起栈验证共用一次迁移；
2. **CL-F-26**（modality 透传）—— 生产接线 + proto 字段，属"只算不消费"同族，与 CL-F-07 的融合面一致；
3. **CL-F-23** —— 起栈**实测** `kafka_topic_partition_oldest_offset` 是否存在；存在则补真实深度告警，不存在则维持现状并在规则注释写明；
4. **CL-F-24** —— Grafana datasource 改只读账号 / secret 注入；**验证要落在面板真的还能出数**（不是只改 yaml）；
5. **CL-F-16** —— 先补"postgres 重启后是否自愈"的对照实验（第二方核对曾指出控制实验不充分），再决定要不要给 health 加惰性重连。
**DoD**：每条给出运行时证据（面板截图 / psql 输出 / 容器状态），并在账本标 ✅ 或如实降级。

### Wave 3（流程硬化）—— ✅ **已完成（第 2 批，PR #246）**
- **CL-F-07 → 关闭**：`RUNBOOK.md §2.1`「在 git worktree 里起栈的安全做法」W1~W4（缺什么 / 症状 / ✅正确做法 / ⛔错误做法），
  点明「用 `--env-file` 指他处来绕开报错」会**静默降级 mock**（服务级 `env_file` 仍缺 + `LLM_API_KEY` 只从 env_file 读）。
- **CL-F-09 → 关闭**：`parallel-contract.md §1.3`「共享 docker 资源（镜像 tag / 容器）」R1~R4
  （带轨后缀 tag / 只 up 自己的服务 / 用前播报 / 用毕交代）。
- **CL-F-08 → 如实降级**：判据**未改**（无栈无法验证新判据；未验证的新判据比有文档的旧判据更危险）。
  改为文档化：把该脚本的 §8 判据限制写进 RUNBOOK §2.1 的"判真起栈成功"门槛（须叠加 Nacos `count:6` + 一次**真实 LLM 往返**）。
- **CL-F-19 → 转 `CL-D-20` 待用户裁定**：分片化是流程结构性改动（会动"账本单一事实源"的读取惯例 + 审计器），
  属用户口径 ⇒ 不在执行者权限内。已把三个候选（分片 / 单文件串行 / `merge=union`）及其代价写入 `decisions.md`。

---

## §3 本轮改动清单（可复跑验证）

| 文件 | 类型 | 命令 |
|---|---|---|
| `scripts/check_git_layout.py` + `scripts/test_check_git_layout.py` | 修复 + 新测试 | `python scripts/test_check_git_layout.py`（6/6）；`python scripts/check_git_layout.py --strict`（exit 0） |
| `app/components/digital-human/DigitalHuman.client.vue` + `clf17-vrm-load-guard.architecture.test.ts` | 修复 + 新测试 | `pnpm vitest run app/components/digital-human` |
| `app/utils/messageCache.ts` + `app/types/message-with-status-single-source.architecture.test.ts` | 收敛 + 新测试 | `pnpm vitest run app/types/message-with-status-single-source.architecture.test.ts` |
| `app/types/conversation/conversationMessagesType.ts`（+`userId`）/ `app/utils/db.ts`（v3）/ `app/stores/message.ts` + `message.user-isolation.test.ts` | 修复 + 新测试 | `pnpm vitest run app/stores/__tests__/message.user-isolation.test.ts` |
| `docs/closure-roadmap/parallel-contract.md` §1.0 | 文档 | 人工核对 |

**全量**：`pnpm vitest run` = **90 文件 / 746 用例全绿**；`pnpm typecheck` = **0 error**；
`eslint app/` = **4 error（= 基线，本轮未新增）**；相关文件 `prettier --check` 通过。

---

## §4 调研依据（本文写作前实读，遵守 AGENTS §〇「文档功课」）

- 账本：`docs/closure-roadmap/discovered-unresolved.md`（26 条逐行）、`docs/closure-roadmap/roadmap.md` §2/§3/§4
- 契约：`docs/closure-roadmap/parallel-contract.md`（§1.1/§1.2/§三/§3.1/§四）
- 代码（至少 3 实现 + 测试）：`scripts/check_git_layout.py`、`app/components/digital-human/DigitalHuman.client.vue`、
  `app/utils/messageCache.ts`、`app/utils/db.ts`、`app/stores/message.ts`、`app/types/api.ts`、
  `app/types/conversation/conversationMessagesType.ts`、`emotion-echo-web-bff/internal/handler/prompt_assembly.go`；
  测试：`app/utils/messageCache.test.ts`、`app/stores/__tests__/message.retry.test.ts`、
  `app/types/conversation/message-status-single-source.architecture.test.ts`
- 既有计划体例：`docs/e2e-roadmap/debt-paydown-plan.md`（判据与反自欺三规则沿用）
- **未跑**：`scripts/smoke_data_layer.py`（本轮不涉业务数据契约；Wave 2 需栈时再跑）
