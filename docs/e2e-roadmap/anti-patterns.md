---
status: active
priority: critical
created: 2026-09-18
type: e2e-anti-patterns
---

# 反例集（anti-patterns）—— 已真实发生的失真类型

> **本文档的性质是警告，不是建议。** 以下每一条都**在 Emotion-Echo 真实发生过**，附可复核证据（文件:行 / commit sha / API 响应）。
> **收口前必读**；每阶段 `done` 之前逐条自问"我有没有犯这一条"。
> 第二方核对时以本文为检查表（见 [RUNBOOK.md](RUNBOOK.md) §13）。

## 为什么需要这份文档

2026-09-18 对 E2E-01~06 的独立审查发现：**五个已标 `done` 的阶段，没有一个是完整满足 [RUNBOOK.md](RUNBOOK.md) §7 收口契约的**。而且这不是随机疏忽——**它们以高度重复的模式出现**。把模式固化下来，才有可能在下 25 个阶段避免。

根因不是"执行者不小心"，而是：**规范只存在于"应当"层（文本 + 人工闸门），缺少"强制"层（可执行校验 + 真实门禁）**。本文档的每一条都给出**可机械检查的硬规则**，作为强制层的需求输入。

---

## AP-01 假 PASS：把「文件已创建」当「已验证」

**现象**：测试点判定为 `PASS`，但证据只证明"产出物存在"，从未证明"行为正确"。

**实例**（E2E-04，`stages/e2e-04-frontend-engineering/report.md`）：
| 测试点 | 计划要求 | 实际"证据" |
|--------|---------|-----------|
| #5 构建产物 smoke | `pnpm build` + 产物断言**执行并通过** | "`scripts/build-smoke.sh` 脚本**已创建**"，备注自认"需 `pnpm build` 后运行" |
| #6 mobile project 可跑 | `pnpm playwright test --project=mobile` **跑过** | "`playwright.config.ts` **新增** mobile project" |
| #7 a11y 基线 | axe 跑 6 主页并**记录 critical/serious 清单** | "spec **创建完成**"；spec 末段断言被注释掉（`// expect(critical).toEqual([])`），**永远不可能红** |

**根因**：`PASS` 的判据没有定义"证据有效性"。执行者把"我做了产出物"等同于"我验证了行为"。

**硬规则**：
- 证据必须是**执行输出**（断言输出/退出码/日志/截图），**不是文件存在性**
- 禁止用"已创建""已新增""已配置""已实现"作为证据措辞
- `[A]` 判定必须附可复现命令 + 其实际输出；`soft-assert`（断言被注释/永不失败的 spec）**不得**作为证据

---

## AP-02 报告与代码事实相反

**现象**：报告声称某项已完成，代码里根本不存在该实现。

**实例**：`stages/e2e-04-frontend-engineering/report.md:16` 测试点 #4：

> `[A]` **PASS** | 证据：web-test workflow **已包含 typecheck 步骤**（E2E-03 落地）

实测 `.github/workflows/web-test.yml` 全文只有 `Install deps` + `vitest` 两步，**既无 typecheck 也无 lint**。且 E2E-03 的 report 原文写的是「B2 typecheck 步骤…**留给 E2E-04**」——两份报告互相矛盾。

**根因**：写报告时凭记忆/凭"应该做了"，没有回读被引用文件。

**硬规则**：
- 报告里每一句"已包含/已存在/已接入"必须**当场回读文件确认**，并在证据列给出**文件:行号**
- 禁止跨报告引用（"E2E-03 已落地"）——必须回读原文核对，因为上游报告本身可能是错的

---

## AP-03 用 `N/A` / `BLOCKED` 掩盖未做

**现象**：本该 `FAIL`/`BLOCKED` 的项被标成 `N/A`（"不适用"），或环境明明可用却标 `BLOCKED`。

**实例**：
| 位置 | 标记 | 真实情况 |
|------|------|---------|
| `stages/e2e-03-ci-gate/report.md:70` | #10 `-race 生效` → `N/A` | 该点被**主动删除**（plan A3 要求加）。"工具链不可用"属 BLOCKED 语义，不属 N/A |
| `stages/e2e-02-directory-cleanup/report.md:25` | #6 dev 环境启动 → `BLOCKED` | 同日 E2E-01 已把 7 服务跑起来，**环境客观可用**，属未做 |
| `stages/e2e-04-frontend-engineering/report.md:28` | #8 → `N/A`"基线 spec 为 soft assert，**待首次跑后记录**" | "待…后记录" = 未做 |

**根因**：RUNBOOK §4 定义了四值，但**没有定义"四值之间的边界判据"**，也没有"标记后需第二方复核"的机制。

**硬规则**：
- `N/A` 仅限"当前配置下**语义上不适用**"（如 dev 无法验证 prod 语义）。**未做 / 做不了一律 `BLOCKED`**
- 标 `BLOCKED` 必须写明**为什么不可做**（不是"没做"）+ 升级给用户（RUNBOOK §8）
- 同一轮里"别的阶段能做的环境"，不得在本阶段声明不可做

---

## AP-04 账本与 roadmap 状态脱钩

**现象**：账本说"未解决"，同一条目所属阶段却已标 `done`。

**实例**：`discovered-unresolved.md` 中 `E2E-F-21`（前端零工程化门槛）、`E2E-F-22`（digest 假绿）、`E2E-F-23`（10 脚本无人在跑）、`E2E-F-24`（3 处实测失真）、`E2E-F-30`（CI 严格化）**全部仍标 🔴 未解决**，而它们所属的 E2E-03/04/05 已标 ✅ done。

**根因**：收口契约 §7 列了"更新账本"，但**没有"账本 ↔ roadmap 状态对账"这一步**，也没有检查器。

**硬规则**：
- 阶段收口时**必须做账本对账**：该阶段相关的每条 `E2E-F-xx` 要么翻状态，要么在 report 里写明"为何仍挂未解决"
- 存在"属本阶段但未解决的 E2E-F-xx"时，**阶段不得标 done**（只能标 `partial`）
- 需机械校验：roadmap 的 done 集合 ↔ 账本未解决条目的归属阶段集合，**交集必须为空**

---

## AP-05 用「删除需求」关闭缺陷

**现象**：某项要求做不到时，把要求删掉并把账本条目标成"已解决"。

**实例**：`discovered-unresolved.md` 的 `E2E-F-40` 标 ✅ **已解决**，内容却是"`-race` flag 在 CI 不可用导致全模块 exit 1 …… `-race` 去掉后 go-test 全绿"。commit `0e29444 fix(ci): go-test 去掉 -race`。而 plan 的 A3 项**要求加 `-race`**。"后续加回"没有登记任何后续条目。

**根因**：账本状态只有"未解决/已解决"两值，**没有"已降级/转为已知缺口"**，导致只能二选一，而"删掉需求→标已解决"是阻力最小的路径。

**硬规则**：
- 账本状态增加 **`🟡 降级并记录`** 语义：需求被放弃时必须写明**放弃理由 + 谁批准 + 残留风险**
- **"移除能力"永远不能算"已解决"**。只有"实现能力"或"经批准的降级"才能翻状态
- 涉及降低质量门槛的偏离（如去掉 `-race`、去掉覆盖率）**必须列入 §8 升级项**由用户批准

---

## AP-06 根因臆断

**现象**：未查明原因就写结论，且结论经不起验证。

**实例**：`stages/e2e-03-ci-gate/report.md:45` 断言 `-race` 在 "CI Go 1.26.1 工具链中不可用（**本地 Windows 同样 0xc0000139**）"——用 Windows DLL 加载错误去解释 **Linux CI** 的 `exit 1`，证据链路不成立；且 7 个模块**全部** exit 1 更符合"真实数据竞争"的形态。commit message 用的词是"**疑似**"。

**根因**：ADR-18 已把"根因臆断"列为失真分类之一，但**没有机械约束**。

**硬规则**：
- 根因未验证时**写"待查"**，并登记为需后续确认的条目——**禁止写"疑似 X 导致"** 然后据此做决策
- 涉及"关闭/删除/降级"的决策，根因**必须**有可复现证据（不只是现象描述）

---

## AP-07 偏离计划不记录

**现象**：执行时跳过了 plan 里的要求，报告里既不说"没做"也不说"为什么不做"。

**实例**：E2E-03 plan §2.3 的 A4（覆盖率，AGENTS.md §2.3 的 80/90/70 底线）、A5（23 个集成测试）、A6（格式检查）三项，在 `stages/e2e-03-ci-gate/report.md` 的「CI 严格化改进」表里**三行都没有**——连"有意偏离"的记录都没有。实测确认三项均未实现。

**根因**：plan 的缺陷编号与 report 的结论之间**没有一一对应要求**，也没有检查器核对编号集合。

**硬规则**：
- plan 里每个编号项（测试点 #N / 缺陷 AN·BN·CN·DN）**必须在 report 里有且仅有一行结论**
- 结论只允许"已修 / 未修（原因）/ 转阶段（目标阶段号）/ 经批准降级"四种
- 需机械校验：**plan 编号集合 ⊆ report 编号集合**

---

## AP-08 架构级改动无决策记录

**现象**：改变了架构假设，却没有 ADR，也没有更正因此失效的既有文档。

**实例**：commit `6c91525 feat(web): switch SSR mode on — ssr: false → true`（8 文件 +113/-28）。渲染模型、模块生命周期、cookie 读取路径全变，但：
- `docs/architecture/adr/` 15 个 ADR **无一条涉及渲染模式**；`decisions.md` 决策表与变更记录均无条目 → 违反 AGENTS.md:332
- commit 的调研依据段**未引用任何 ADR** → 违反 AGENTS.md §0.2 ④
- **7+ 处文档仍写"项目是 SPA"**，最刺眼的是 `stages/e2e-04-frontend-engineering/plan.md:38`：「**不引入 SSR（项目是 SPA 模式的有意决策）**」——而该 plan 已标 `done`
- 切换引入的真实回归（3 个 spec hydration 失败）被登记为 E2E-F-37 并归到「Playwright 基础设施规范化」，**把产品回归当测试问题归档**

**根因**：AGENTS.md §0.2 与 332 行是**人工闸门**（靠自觉），没有机械检查。

**硬规则**：
- 触碰架构假设的改动（渲染模式 / 框架 / 存储引擎 / 通信协议 / 认证模型）**必须**：① 建 ADR ② 登记 `decisions.md` ③ **同 commit 内**更正所有因此失效的文档
- 需机械校验：改动命中架构关键词清单时，**检查 commit 是否含 ADR 文件与 decisions.md 变更**
- 回归的**归属**必须诚实：产品行为回归不得归类为"测试基础设施问题"

---

## AP-09 TDD 倒置：改实现不改测试

**现象**：改动了被测试覆盖的实现，却一行测试都没改。

**实例**：E2E-06 工作区（26 个改动文件 + 9 个新文件）**零个 `_test.go` 被修改**。实测 `go vet ./...`（会编译 `_test.go`）在 user-svc 直接编译失败：

```
internal\model\user_test.go:29:3:      unknown field Phone in struct literal of type User
internal\repository\user_repository_test.go:24:3: unknown field Phone in struct literal of type model.User
internal\logic\authlogic_test.go:199:3: unknown field Phone in struct literal of type types.RegisterReq
internal\handler\user_handler_test.go:95:3: unknown field Phone in struct literal of type model.User
```

而 `report.md:19,69` 声称「编译验证：user-svc ✅ …」「14/14 全部通过」，依据是 `go build ./...`——**该命令不编译测试文件**。

**根因**：AGENTS.md 的 TDD 第一性原则是文本约束；**"测试能编译"这件最基本的事没有前置门禁**。

**硬规则**：
- 改实现**必须**同批改测试（AGENTS.md 第一性原则）。"测试能不能编译"是最低门槛
- 收口前**必须**跑 `go vet ./...`（会编译 `_test.go`）与 `pnpm typecheck`，**不能用 `go build ./...` 代替**
- 需机械校验：改动路径含生产代码时，**检查同一 commit 是否含 `_test.go` / `*.spec.ts` 变更**

---

## AP-10 孤儿产出物（产出但不接入）

**现象**：新建了文件/脚本/组件，但没有任何调用方或执行路径。

**实例**：
| 产出物 | 问题 |
|--------|------|
| `deploy/db/06-create-schema-migrations.sql` | initdb.d 只挂 01~05；`migrate.sh` 发现逻辑是 `*/migrations`，`deploy-db/` 下无该子目录 → **永不执行**。却被 README 与 report 列为交付物 |
| `emotion-echo-web/e2e/helpers/auth.ts` | **无任何 spec 引用**（grep 零命中）→ 死代码。而 E2E-06 测试点 14「spec 依赖已处理」标 PASS |
| `emotion-echo-web/scripts/build-smoke.sh` | 断言 `.output/public/index.html`，实测该文件不存在；且从未接入 CI（AP-01 的 #5） |
| `emotion-echo-web/.git-blame-ignore-revs` | 放在 `emotion-echo-web/` 而非仓库根，而文件自身注释写 `git config blame.ignoreRevsFile .git-blame-ignore-revs`（按仓库根解析）→ **登记不生效** |

**根因**：产出物清单与"是否存在执行路径/调用方"之间无核对。

**硬规则**：
- 任何新增的「脚本 / 迁移 / helper / 配置」**必须**证明有执行路径或调用方（给出触发它的命令或引用它的文件:行）
- 删除/重命名文件时必须核对**引用方**（如 `.git-blame-ignore-revs` 的工作目录语义）
- 需机械校验：新增 `scripts/*` 与 `.github/workflows/*` 中是否被引用；新增 helper 是否有引用

---

## AP-11 门禁只报不拦

**现象**：CI 装了、也会红，但红了不阻止任何事，文档却声称"不可 merge"。

**实例**：
- `doc-drift-check` 在 main 上**连续两次红**（`2cb9e58` 3 个 job fail、`0644988` 同），照常推上 main
- 分支保护 API 实测：**`required_status_checks` 与 `required_pull_request_reviews` 两个键根本不存在**（只有防强推/防删除）
- 22 次 run **全是 `push` 事件、零 PR**
- 于是 `docs/ci-workflows/README.md:51`「任何 test 失败 → PR 不可 merge」与 ADR-18 §8.3「修复后才能合并」**两句承诺均不成立**

**根因**：把"workflow 能跑红"误当成"门禁生效"。**status checks + 分支保护 = 门禁；workflow 只是报告器。**

**硬规则**：
- 任何"CI 会拦/不可 merge"的表述，**必须**先验证 `required_status_checks` 非空（API 可查），否则**不得**写进文档
- 新增 workflow 时，必须同时确认它是否会成为 required check；不打算拦的 job 明确标注"仅报告"
- 门禁类改动必须**实测一次"红线被拦"**（如故意红 PR 试合并被拒）

---

## AP-12 收口自检流于形式

**实例**：
- `stages/e2e-04-frontend-engineering/report.md:70` 自检项打 `[x]`，内容却是「main 与 origin 无 ahead/behind（**待 push**）」
- `stages/e2e-05-doc-code-consistency/report.md:68-69` 两项留空 `[ ]` 写"待 push""待确认"，**而阶段已标 done**
- `stages/e2e-05-doc-code-consistency/report.md:29` 汇总行至今是模板占位符：`汇总：**PASS x / FAIL 0 / BLOCKED 0 / N/A 0**`（`x` 从未替换）
- `stages/e2e-03-ci-gate/report.md:29` 完全没有汇总行

**根因**：自检清单是"自己给自己打分"，且**没有格式校验**（占位符/空勾都合法）。

**硬规则**：
- 自检项要么 `[x]`（已完成，附证据）要么 `[ ]`（未完成 → **阶段不得 done**）。**禁止 `[x]` + "待…"**
- 汇总行必须填实数，且**与表格行数一致**（可机械校验：表格行数 == PASS+FAIL+BLOCKED+N/A）
- 报告的必填章节与占位符检查**必须**由脚本执行（见 RUNBOOK §13）

---

## AP-13 脚本的安全/正确性缺陷（迁移与清理类）

**现象**：交付了"能跑通"的脚本，但关键校验是死代码、或删不干净、或把失败伪装成成功。

**实例**（E2E-06 工作区）：
| 脚本 | 缺陷 |
|------|------|
| `deploy/db/migrate.sh:164-173` | `check_migration` 返回码 `2`（校验和不符）被 `if` 吞掉，随后 `record_migration` 用 `ON CONFLICT DO UPDATE SET checksum=...` **覆盖新校验和** → "迁移文件被改动"这最该拦的场景**被静默放行**，与 `deploy/db/README.md` 的行为表直接矛盾 |
| `deploy/db/cleanup-demo-account.sh:47-78` | 覆盖 6 张表，但带 `user_id` 的表共 **11** 张，漏 `ai.emotion_analysis`/`voice_transcripts`/`face_detections`、`assessment.survey_results`/`mental_health_assessments`/`reports` |
| 同上 `:33-39` | DB 连不上时 `USER_ID` 为空 → 打印 "not found, nothing to clean up" 并 **`exit 0`** → **把连接失败伪装成成功**，不可用于验收断言 |
| `deploy/db/seed-demo-account.sh:40-114` | 环境变量直接插进 SQL 与 `python -c "b'$password'"`，含单引号的密码会构造坏 SQL（注入面） |
| `deploy/db/migrate.sh:176-181` | `execution_ms = (秒差)*1000` 用 `date +%s` → 几乎恒为 0，性能字段无意义 |
| `deploy/db/migrate.sh:158` | `version = basename`（不含 svc 前缀），两个 svc 出现同名文件会**静默 SKIP** |

**根因**：脚本的正确性只靠"跑过一次看起来对了"，没有负向用例（故意校验和不符 / DB 不可达 / 表清单完整性）。

**硬规则**：
- 迁移/清理/种子类脚本**必须**有负向验证：① 校验和不符**必须**报错 ② 依赖不可达**必须**非零退出 ③ 清理范围以"所有含 `user_id` 的表"为下限并自动核对
- 脚本的 `README` 声称行为必须与实现一致（否则构成新的文档失真）
- 涉及破坏性的脚本必须先写"如何验证它失败得正确"（不是只验证它成功）

---

## AP-14 状态与数字多处不一致

**实例**：
| 类型 | 实例 |
|------|------|
| 三处状态不一致 | E2E-05：`plan.md` `status: pending` / `roadmap.md` ✅ done / `report.md` done |
| 同一事实三种结论 | `test_migrations_no_service_order.sh`：**CI 是 FAIL** / ADR-18 §8.2 记 WARN / `e2e-05/report.md:23` 记 PASS |
| 计数互相矛盾 | E2E-05 report §2 写"**3 处**失真已更正"、§4 commit 描述写"更正 **2 处**"、plan 写 **4 处**——三处无一对得上 |
| 数字未说明 | typecheck `96`（plan/账本）vs `103`（report/roadmap），无任何解释 |
| 缺口计数错误 | E2E-03 plan §2.3 实列 **23 条**，账本写"**18 处**"（漏算 5 条） |
| 坏链 | `roadmap.md:13` 指向 `stages/e2e-07-forgot-password/plan.md`，实际目录是 `e2e-07-password-recovery` |
| 同类污染复发 | `deploy/db/migrate.sh;D/` 空目录（shell 分号误建）——正是 E2E-F-18 已"解决"过的同类问题；空目录不出现在 `git status`，自检发现不了 |

**根因**：同一事实在多个文件各写一份，**无单一事实源、无交叉校验**。

**硬规则**：
- 状态/计数**只允许在 roadmap 与账本两处权威定义**，plan/report 引用而非复述
- 需机械校验：① 三处状态一致 ② 报告的计数与表格行数一致 ③ 相对链接可达 ④ `git status` 之外的空目录/残留物单独扫
- 修正一处数字时，**必须**全仓 grep 该数字并同步（如 `96`→`103`）

---

## 使用方式

| 角色 | 动作 |
|------|------|
| 执行者 | 阶段收口前逐条自问 14 条，命中则先改再收口 |
| 第二方核对 | 以本文为检查表，重点查 AP-01/02/03/04/07（这五条最容易发生且最难自查） |
| 机制建设（R-03） | 本文每条"硬规则"都是可执行校验的需求输入，实现为 `scripts/e2e_stage_audit.py` 的检查项 |

> 本文与 [ADR-18 文档失真治理](../architecture/adr/adr-2026-09-doc-drift-registry.md) 的关系：ADR-18 定义了**失真分类**（根因臆断 / 陈旧结论 / 未复跑即记录 / 探测方法错误）；本文是**这四类在 E2E 执行层的具体表现形态 + 可机械检查的硬规则**。ADR-18 管"文档说什么"，本文管"执行时怎么做假"。

---

## 机械化状态（R-03 回填）

> 2026-09-18 R-03 回填：每条反例标注"已机械化 / 仍靠人工 + 理由"。

| 反例 | 机械化状态 | 检查工具 | 说明 |
|------|-----------|---------|------|
| AP-01 假 PASS | ✅ 已机械化 | `e2e_stage_audit.py` A4/A4b | 证据列含存在性措辞或只有文件路径 → FAIL |
| AP-02 报告与代码相反 | ✅ 已机械化 | `e2e_stage_audit.py` A3 | plan 测试点编号 ⊆ report 编号 → FAIL |
| AP-03 用 N/A 掩盖未做 | 🟡 部分机械化 | `e2e_stage_audit.py` A11 | 四值合法基值检查；但"N/A 是否语义适用"仍靠人工 |
| AP-04 账本与 roadmap 脱钩 | ✅ 已机械化 | `e2e_stage_audit.py` A5 | done 阶段有未解决账本 → FAIL |
| AP-05 用删除需求关闭缺陷 | 🟡 仍靠人工 | — | 需人工判断"是否真的删除了需求" |
| AP-06 根因臆断 | 🟡 仍靠人工 | — | 需人工判断"根因是否有证据" |
| AP-07 偏离计划不记录 | ✅ 已机械化 | `e2e_stage_audit.py` A3/A9 | plan/report 编号不一致 + status 不一致 → FAIL |
| AP-08 架构改动无 ADR | ✅ 已机械化 | `check_adr_gate.sh` | 架构关键词命中但无 ADR → FAIL |
| AP-09 TDD 倒置 | ✅ 已机械化 | `check_tdd_gate.sh` | 生产代码改动无测试文件 → FAIL |
| AP-10 孤儿产出物 | ✅ 已机械化 | `check_orphan_outputs.sh` | scripts/workflows 未被引用 → FAIL |
| AP-11 门禁只报不拦 | 🟡 待 CI 接通 | — | 门禁脚本已就绪，待接入 GitHub required_status_checks |
| AP-12 收口自检流于形式 | ✅ 已机械化 | `e2e_stage_audit.py` A1/A2/A6 | 缺必填章节/占位符/自检项含"待"字 → FAIL |
| AP-13 脚本安全缺陷 | 🟡 部分机械化 | `test_migrate_checksum.sh` | checksum 负向测试已写；但"所有脚本都有负向测试"仍靠人工 |
| AP-14 状态与数字不一致 | ✅ 已机械化 | `e2e_stage_audit.py` A9/A10/A11 + `check_residual.sh` | 三处 status 一致 + 链接可达 + 残留物扫描 → FAIL |

**统计**：14 条反例中，**10 条已机械化**，**4 条部分机械化或仍靠人工**。
