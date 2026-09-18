---
status: active
priority: critical
created: 2026-09-18
type: e2e-remediation-plan
---

# 补救排期与执行路径（R 系列）

> **本文档性质：排期与规划，不包含任何已执行的修复。**
> 触发：2026-09-18 对 E2E-01~06 的独立审查。审查结论——**五个已标 `done` 的阶段无一完整满足 [RUNBOOK.md](RUNBOOK.md) §7 收口契约**，另有 1 个安全级缺陷、2 个功能性破坏、1 个持续红的 CI。
> 失真模式已固化为 [anti-patterns.md](anti-patterns.md)（14 类，带证据）。发现逐条登记在 [discovered-unresolved.md](discovered-unresolved.md) 的 `E2E-F-41`~`56`。

## 为什么单独开 R 系列而不改 E2E-NN 编号

- 这些问题**不是新功能工作**，是**既有阶段的欠账**；用独立编号可清楚区分"欠债归还"与"路线图推进"
- 不重排 30 个阶段编号（本项目已因重排两次踩过交叉引用失真的坑）
- R 系列明确「**插入前置**」：未完成 R-01，不得启动 E2E-07 及以后任何阶段

---

## 问题分类

| 类 | 含义 | 数量 | 归属 |
|----|------|------|------|
| **C0. 机制先行** | 先造出"能判定完成"的机器，否则下面两项自己也会造假账 | — | **R-00** |
| **A. 阻断** | 安全漏洞 / 功能不可用 / 提交即 CI 必红 | 7 | **R-01** |
| **B. 契约欠账** | 阶段标 done 但收口契约未满足（报告/截图/账本/状态） | 15 | **R-02** |
| **C. 机制收尾** | 剩余断言 + 门禁接通 + CI 严格化 | — | **R-03** |

**为什么 R-00 排在 R-01 前面**：本次审查的元教训是**"执行者自证的完成不可信"**。R-01/R-02 同样是执行者，同样会面临"证明做完了 vs 承认没做"的阻力不对称。**若先做 R-01/R-02 而没有校验器，补出来的可能又是一批"文件已存在"式的证据。** 故先把校验器的最小可用版（MVA）造出来、并**用 E2E-03/04/05 做回归样本验证它能复现本次审查结论**——校验器跑不出已知缺口 = 它无效，不能进入下一步。

---

## R-00 🔧 审计器 MVA（先行，最小可用版）

**状态：✅ 已完成（2026-09-18）** —— 交付 `scripts/e2e_stage_audit.py`，`--selftest` 通过。

**目标**：把 5 条最关键、最易造假的断言机械化，并证明它**能复现本次审查的结论**。

**依赖**：无 → **可立即启动**（体量小，单次会话可完成）
**阻塞**：R-01 / R-02（它们需要在审计器监督下收口）

### 实际交付与验证结果

| 项 | 结果 |
|----|------|
| 脚本 | `scripts/e2e_stage_audit.py`（MVA 5 条断言 + `--selftest` + `--json`） |
| **自校验** | ✅ **通过**：E2E-03 检出 A1/A2/A3/A5、E2E-04 检出 A4/A5、E2E-05 检出 A2/A4/A5 —— 与审查实测结论一致 |
| **反误报** | ✅ 对 E2E-02（最接近合规）无 FAIL；对 19 个未开工阶段（无详档，符合 just-in-time 约定）无 FAIL |
| 全量结果 | 30 个阶段中 **4 个 FAIL**：E2E-03 / E2E-04 / E2E-05 / E2E-06 —— 与审查结论吻合 |
| 退出码 | 检出 FAIL → 非零（可供 CI 消费） |

### 开发过程中修正的 3 个自身缺陷（记录以备参照）

| 缺陷 | 表现 | 修正 |
|------|------|------|
| roadmap 行首带标记导致漏解析 | `| E2E-04 🔧 |` 使 `fullmatch(E2E-\d+)` 失败 ⇒ **A5 永不触发** | 改为 `re.match(r"E2E-(\d+)(?:\s\|$)")`，且只在「排期总表」章节内解析（避免被后面的「详档约定」表覆盖） |
| A4 只看证据列 → 漏检 | E2E-05 的存在性措辞在**测试点名**里（"2 个 migration 脚本**已创建**"），证据列是纯路径 | 增加 A4b：证据**只有文件路径、无任何执行信号**同样判 FAIL |
| 状态判定靠全文关键词 → 误判 | E2E-02 状态为 `✅ done（…唯一瑕疵：#6 可验证却标 BLOCKED）`，因含 "BLOCKED" 被误判为 `blocked` | 改为按优先级 + 限定措辞（`blocked by` / `⏸` 前缀）判定 |
| 对未开工阶段喊狼来了 | `--all` 时 19 个无详档阶段全部 A0 FAIL | 仅在状态为 `done`/`partial` 时缺目录才算 FAIL |

### DoD 核对

- [x] 可对单个阶段与 `--all` 运行
- [x] `--selftest` 通过：准确检出已知缺口
- [x] 对 E2E-02 不报 A1~A5 FAIL（无误报）
- [x] 输出机器可读（`--json`）
- [x] 退出码语义正确（检出 FAIL → 非零）

### 边界（不做）

- 不做截图存在性、坏链、TDD 同批、ADR、孤儿产出物、`required_status_checks` 等（归 R-03）
- 不修改任何阶段文档（只读审计）
- **不接入 CI**（门禁接通归 R-03，且需按 D-08 决定 required 范围）

---

## R-01 🔴 阻断项修复（最高优先）

**状态：⛔ 未完成（2026-09-18 第二方核对）** —— 6/7 项实测为真，但 **#2 未通**（链路仍不可达），#5 为降级。**不得据此解除 E2E-07 阻塞。**

> 本节原记"✅ 基本完成"，由**非执行者的第二方核对**（[RUNBOOK](RUNBOOK.md) §7 #10）推翻。这是 AP-01/AP-14 在补救文档内部的复发——记录下来作为"执行者自证不可信"的又一实例。

### 第二方核对结果（2026-09-18，逐条独立复现命令与观测）

| # | 声称 | 核对方式 | 结果 |
|---|------|---------|------|
| 1 | BFF 密保 fail-open 已修 | 读 `bff/.../auth_handler.go:458-497` | ✅ **为真**：不再用 `Login(username,"dummy")` 探测，改调 `VerifySecurityAnswerByUsername`，失败统一 401 |
| 2 | 找回密码端点已补 | 读 `user-svc/main.go:158` + `deploy/apisix/seed.sh:540-546` | ⛔ **未通**：两端已实现，但 **APISIX 白名单只有 110~115**；`/api/v1/auth/verify-security-answer` 落到 route 100（`/api/v1/*`，含 jwt-auth）⇒ 未登录调用必 401。前端经 `:19080`（=APISIX）调用，链路不可达 |
| 3 | 注册链路已恢复（D-05） | 读 `authlogic.go:96-104`、`bff/.../auth_handler.go:160-195` | ✅ **为真**：密保改为可选（`>2` 才拒）；BFF 不再强校验验证码。遗留：SPA 仍强制填写验证码输入框（E2E-09 范围） |
| 4 | 测试可编译 | `go vet ./...` × 7 模块 | ✅ **为真**：7/7 通过 |
| 5 | Register 非事务 | 读 `authlogic.go:124-148` | 🟡 **降级**：仍 `Create` 后 `Save` 无事务；文档如实记为降级 |
| 6 | migrate.sh checksum | `bash scripts/test_migrate_checksum.sh` | ✅ **为真**：`rc -eq 2 → die`；负向测试 2/2 PASS |
| 7 | CI 转绿 | `gh run list` 按 workflow 分别查 | 🟡 **部分**：main 上 `go-test` + `doc-drift-check` 绿；`web-test` 最后运行于 `e31f419`、`llm-test` 于 `30d6b66`（`paths` 过滤未覆盖 R 系列提交）⇒ "4 workflow 全绿"在 HEAD 不成立 |
| — | DoD #3 负向测试 | `go test ./internal/logic/ -run VerifySecurityAnswer` | ✅ 5/5 PASS（含 `WrongAnswer_ReturnsMismatch`）。**但覆盖在 user-svc logic 层，BFF handler 层零测试**（`bff/.../auth_handler_test.go` 对 `verifySecurityAnswer` 零引用）⇒ 原缺陷所在的层没有回归钉 |

**给执行者的最小修复**：在 `deploy/apisix/seed.sh` 增加 `put_auth_route 116 "/api/v1/auth/verify-security-answer"`（116 号已在 Step 4.5 被列入漂移清理，需同步从 `for drift_id in 116` 移除），并更新该行下方的 route 计数文案；补一条 BFF handler 层的负向测试；然后重跑 `bash deploy/apisix/seed.sh` + 真实 curl 验证 401/200 两种路径。


**目标**：消除安全漏洞与功能性破坏，让 `main` 的 CI 恢复可绿状态。

**依赖**：R-00（修复结论需审计器可验）
**阻塞**：E2E-07 及以后全部阶段；E2E-06 的 `done` 状态有效性
**已定决策**：D-05（注册修法）/ D-06（`-race` 调查）/ D-07（doc-drift-check 处置）

### 范围

| # | 项 | 账本 | 关键位置 |
|---|----|------|---------|
| 1 | **BFF 密保校验 fail-open**：`Login(username, "dummy")` 当用户存在性探测，永远走 `err != nil` → 恒返回 `success:true`，答案从不校验 | E2E-F-41 | `bff/internal/handler/auth_handler.go:478-481` |
| 2 | **找回密码端点不存在**：BFF HTTP 客户端调 `/api/v1/users/verify-security-answer`，user-svc 无此路由（`main.go:152-168`），HTTP 路径 404 | E2E-F-41 | `bff/internal/downstream/user.go:214` |
| 3 | **注册链路断裂**：后端强制 1~2 个密保，前端仍只发 `{username,password,verificationCode}` → 唯一注册入口 100% 400 | E2E-F-42 | `bff/.../auth_handler.go:172-180`、`authlogic.go:96-102`、`web/app/pages/login/index.vue:147` |
| 4 | **测试未同步 → 编译失败**：26 个改动文件零 `_test.go` 变更；`go vet ./...` 在 user-svc 报 4 个 `unknown field Phone`；BFF avatar 测试的 fake 不满足新 `UserClient` 接口 | E2E-F-43 | `user-svc/internal/{model,repository,logic,handler}/*_test.go`、`bff/internal/handler/avatar_handler_test.go:59` |
| 5 | **Register 非事务**：先建用户再存密保，后者失败则用户行已落库 → 库里留下"无密保用户" | E2E-F-44 | `user-svc/internal/logic/authlogic.go:119-146` |
| 6 | **迁移 checksum 校验是死代码**：返回码 2 被吞掉，`ON CONFLICT DO UPDATE SET checksum` 覆盖新校验和 → "迁移文件被改动"被静默放行 | E2E-F-45 | `deploy/db/migrate.sh:164-173` |
| 7 | **CI 在 main 持续红**：`doc-drift-check` 3 个 job 失败（env 变量 8 项未文档化 / 6 个占位 digest / migration 顺序 Fail 13） | E2E-F-50 | run on `2cb9e58`、`0644988` |

### 执行路径（顺序不可调换）

```
① 测试编译修复（#4）
     └─ 先让 go vet ./... 在 user-svc / web-bff 通过
     └─ 这是 TDD 补课的第一步：删字段的断言要同步删、改签名要同步改
② 功能修复（#1 #2 #3 #5）
     └─ 密保校验改为真实校验路径：#2 的端点缺失必须先解决（要么补 HTTP 路由，要么让 BFF 只走 gRPC）
     └─ 注册：前端补密保录入 或 把强制校验降级留给 E2E-09（二者择一，需用户决定）
     └─ Register 包进事务
③ 迁移脚本正确性（#6）
     └─ 显式判 `-eq 2` → die；并给 `deploy/db/test_migrations_contract.sh` 加一条 schema_migrations 断言
④ 让 CI 转绿（#7）
     └─ 先跑本地基线，把 3 个 job 的失败逐项定性：是脚本错 / 数据错 / 文档错
     └─ 定性后分别修（注意：不能给 job 加 continue-on-error 来"变绿"——那是 AP-11）
⑤ 复跑 CI 断言全绿 + 一次故意红线验证门禁
```

### 验收标准（DoD）

- [x] `go vet ./...` 在 7 个 Go 模块全部通过（含 `_test.go` 编译）—— 2026-09-18 实测 7/7
- [x] `pnpm typecheck` 0 errors（退出码 0）、`pnpm test` 366/366 全绿 —— 2026-09-18 实测
- [x] 密保校验**真的有校验**：错误答案必须被拒 —— user-svc logic 层 5/5 PASS；**但不覆盖 BFF handler 层**（原缺陷所在）
- [ ] 注册流程端到端可用（后端已按 D-05 恢复；**未做端到端实测**）
- [x] migrate.sh 校验和不符**必须报错**（负向用例）—— `scripts/test_migrate_checksum.sh` 2/2 PASS
- [ ] main 上 `go-test` / `web-test` / `llm-test` / `doc-drift-check` **全部绿** —— HEAD 仅 `go-test`/`doc-drift-check` 实跑；后两者受 `paths` 过滤未覆盖 R 系列提交
- [ ] 所有修复走 TDD（#2 未修，该项无从满足）
- [ ] 每个修复项在 report 里有对应行；发现登记账本 —— **账本未回填**（E2E-F-46~49/58/59 实测已修仍挂"未解决"）
- [ ] **#2 补齐 APISIX 白名单路由 + 实测 401/200 双路径**（2026-09-18 核对追加）
- [ ] **BFF handler 层补密保负向测试**（2026-09-18 核对追加）

### 边界（不做）

- 不做 CI 严格化的 14 项未完成项（归 E2E-03 阶段 2 / R-03）
- 不做 E2E-07 的找回密码流程改造（R-01 只保证"校验是真的"）
- 不补历史阶段的报告（归 R-02）
- 不顺带重构 user-svc 其它模块

### CI 红态精确诊断（2026-09-18 实测，供执行者直接照做）

`doc-drift-check` **每次 push 都红**（连续 4 次：`0644988` / `2cb9e58` / `69eda7c` / `9039475`），其余 workflow 绿。11 个 job 中 **3 个失败**：

#### ① Dockerfile digest pin —— **客观上无法本地修复**（D-07 第 2 项）

```
WARN: 发现 6 个占位 digest（sha256:000...000）:
  GOLANG_1_26_ALPINE_DIGEST / PYTHON_3_10_SLIM_DIGEST / PYTHON_3_12_SLIM_DIGEST
  NODE_20_ALPINE_DIGEST / ALPINE_LATEST_DIGEST / UBUNTU_22_04_DIGEST
FAIL: 占位 digest 等同于未 pin（形式通过、实质为空）
```

**实测确认不可回填**：`curl https://registry-1.docker.io/v2/` → `HTTP 000`（21 秒超时）；`docker manifest inspect golang:1.26-alpine` → 连接失败。故 `scripts/sync_docker_digests.sh` 在本环境跑不通。

**按 D-07 处置**：改为**显式已知缺口声明**——脚本打印醒目警告（含"原因：需 registry 可达环境回填"+ "复检条件：网络可达时跑 `sync_docker_digests.sh`"），退出码为 0。**不是静默通过**，也**不加 `continue-on-error`**。

#### ② 环境变量 lint —— **可直接修，低风险**

```
[FAIL] Vars used in apps.yml but not in .env.local.example:
  APISIX_ADMIN_KEY / BFF_DEV_RETURN_CODE / BFF_JWT_SECRET / CORS_ALLOW_ORIGINS
  NACOS_GROUP / NUXT_PUBLIC_API_BASE_URL / SKYWALKING_ENABLED / XTTS_MODEL_PATH
```

**修法**：把上述 8 个变量补进 `deploy/env/.env.local.example`（及 `docs/env-templates/.env.local.example` 副本，注意 `lint_env_vars.sh` 会同时校验两者）。**不得**写入真实值——只写占位与说明。

#### ③ Migration 服务顺序独立性 —— **规则与其自述目的不符，需修规则而非加豁免**

```
FAIL: ./emotion-echo-analytics-svc/migrations/a001_create_views.sql - 引用了其他服务的 schema (emotion_echo_ai / _assessment / _chat)
FAIL: a003_create_mv_daily_emotion.sql / a004_create_analytics_reader_role.sql / a007 / a009 同类
（其余服务 0 失败；另有 1 条 legacy 的 WARN）
```

**根因分析**：脚本自述目的是「不依赖**服务启动顺序**」（`test_migrations_no_service_order.sh:1-8`），但实现成的是绝对规则「**migration 不得引用其他服务的 schema**」（`:66-102`，且**无任何豁免机制**）。

而 `analytics-svc` **在架构上就是跨域聚合器**——它的 `analytics_reader` 视图必须读 `emotion_echo_ai` / `_chat` / `_assessment` 才能出报表。且顺序有保证：`migrate.sh` 的 `PRIORITY_ORDER` 是 chat → ai → analytics，analytics 排最后。**ADR-18 §8.2 也把它记为「⚠️ WARN（analytics 视图跨 schema）」**，即一直被视为已知且可接受。

**结论：这是一个"按设计必然失败"的检查项**——它不会发现真问题，只会让 CI 永久变红，进而训练所有人忽略 CI（比没有检查更糟）。

**修法（二选一，推荐 A）**：
- **A（推荐）**：把规则收敛回它的自述目的——只检查"是否依赖**启动顺序**"，允许声明式豁免：在脚本里加一张 `CROSS_SCHEMA_ALLOWED` 表，登记 `analytics-svc`（附理由：跨域聚合器 + `PRIORITY_ORDER` 保证顺序），其余服务仍严格禁止
- **B**：若认为 analytics 的跨 schema 引用本身要改（改为运行时 JOIN 而非视图），那是**架构改动**，需先出 ADR，不应由本检查项驱动

**禁止**：直接给该 job 加 `continue-on-error` 或把 13 条 FAIL 改判为 WARN 而不修规则——那是 AP-11 变体（假装门禁存在）。

---

## R-02 🔧 收口补账（契约补齐）

**状态：🟡 部分完成（2026-09-18 第二方核对修正）** —— 10/15 项实测为真；#1~#3 未做；**#4 不实**、**#6 缺 ADR**、**#15 的处置违背 D-06**。

**目标**：让 E2E-01~06 的状态**诚实**——要么补齐契约，要么把状态降为 `partial`。

**依赖**：R-01（补账要基于事实，而事实现在是错的）
**阻塞**：R-03（先知道要审计什么）

### 范围

| # | 项 | 涉及阶段 | 状态 |
|---|----|---------|------|
| 1 | **report 按 `_REPORT_TEMPLATE.md` 重写**：E2E-03（非模板、无环境基线、无汇总、无判定列）、E2E-06（缺环境基线/发现分类/修复清单/回归钉四节） | E2E-03 / E2E-06 | ⏳ 待后续 |
| 2 | **补齐 `[V]` 测试点截图**：E2E-01/03/04/05 的 screenshots 目录为空或缺失 | E2E-01/03/04/05 | ⏳ 待后续 |
| 3 | **账本对账**：E2E-F-21/22/23/24/30 全部仍挂 🔴 未解决，而其所属阶段已标 done → 逐条翻状态或在 report 写明理由 | E2E-03/04/05 | ⏳ 待后续 |
| 4 | **状态三处对齐**：E2E-05 `plan.md` 至今 `status: pending`；roadmap/report/plan 三处须一致 | E2E-05 等 | 🟡 **不实**（2026-09-18 核对）：`plan.md` 现为 `status: partial`，而 roadmap/report 为 `done` ⇒ 三处仍不一致，审计器 A9 实测 FAIL |
| 5 | **撤下 E2E-06 的 `done`**：`report.md:169` 的 `[ ] 补 integration test` 未勾选而标 done（违反状态机）→ 改 `partial` 或补齐 | E2E-06 | ✅ 已完成（partial） |
| 6 | **更正 SSR 失效文档 + 补 ADR**：7+ 处仍写"项目是 SPA"（含 `e2e-04/plan.md:38`「不引入 SSR」这句与 `done` 状态并存）；`docs/architecture/adr/` 与 `decisions.md` 均无渲染模式条目 | 全局 | 🟡 **部分**（2026-09-18 核对）：7+ 处文档**已更正**（现写 SSR）；但 **ADR 与 `decisions.md` 登记仍缺失**——`docs/architecture/adr/` 15 个 ADR 无一条涉及渲染模式，`git log --diff-filter=A 19c65e1..HEAD -- docs/architecture/` 为空 |
| 7 | **修坏链与占位符**：`roadmap.md:13` 的 `e2e-07-forgot-password` → 实际 `e2e-07-password-recovery`；E2E-05 report 汇总行 `PASS x` 占位符 | — | ✅ 已完成 |
| 8 | **孤儿产出物归置**：`06-create-schema-migrations.sql`（永不执行）删除或改为真正被执行的路径；`e2e/helpers/auth.ts`（无引用）接入或删；`.git-blame-ignore-revs` 移到仓库根 | E2E-04/06 | ✅ 已完成 |
| 9 | **演示账号与 initdb 解耦**：`03-seed-default-users.sql` 仍硬编码 `echo/echo123`，而 cleanup 默认目标正是 `echo` → "可删"不成立。改为"契约账号由 initdb 负责、不属可删演示账号"或改用其它用户名 | E2E-06 | ✅ 已完成 |
| 10 | **清理脚本补全**：`cleanup-demo-account.sh` 漏 6 张含 `user_id` 的表；DB 不可达时须非零退出（现伪装成功） | E2E-06 | ✅ 已完成 |
| 11 | **死字段从权威 DDL 移除**：`02-create-tables-in-schemas.sql:10,11,17` 仍有 `phone`/`email`/`status`，与 `chat-svc/migrations/008_p0r27_ddl_drift_test.go` 的"表定义唯一源=02"契约冲突 | E2E-06 | ✅ 已完成 |
| 12 | **同类污染复发清理**：`deploy/db/migrate.sh;D/` 空目录（E2E-F-18 同类问题） | — | ✅ 已完成 |
| 13 | **`main.go` 注释与代码相反**：注释仍写"Stage 77 退避重试"，代码已换单次连接 → 要么恢复重试，要么改注释 | E2E-06 | ✅ 已完成 |
| 14 | **E2E-04 假 PASS 更正**：4 个测试点（typecheck 进 CI / build smoke / mobile project / a11y 基线）的证据重填为真实状态，或降为未完成 | E2E-04 | ✅ 已完成 |
| 15 | **`-race` 状态改判**：`E2E-F-40` 把"移除 `-race`"标为已解决 → 改为 `🟡 降级并记录`（含放弃理由、批准人、残留风险），或在 R-01 后真正加回 | E2E-03 | ⛔ **处置违背 D-06**（2026-09-18 核对）：给出的唯一理由是本地 Windows `0xc0000139`，而 D-06 已明文判定"用 Windows DLL 错误解释 **Linux CI** 的 exit 1"**证据链不成立**，并规定了 3 步可复现调查（Linux 容器复现 / 空测试验证 `-race` 可用性 / 读 CI 失败首条错误行）。CI 运行于 Linux，账本结论"残留风险=无"无支撑；"批准人"亦无记录 |

### 第二方核对明细（2026-09-18）

**核对为真**（逐条独立复现）：#5（E2E-06 → `partial`）、#7（`roadmap` 坏链已指向 `e2e-07-password-recovery`）、#8（`deploy/db/06-create-schema-migrations.sql` 已删除；`.git-blame-ignore-revs` 已在仓库根）、#9（`03-seed-default-users.sql` 现只种契约账号 `smoke_user`，演示账号归 seed/cleanup）、#10（cleanup 覆盖 12 张含 `user_id` 表，DB 不可达时 `die` 非零退出）、#11（`phone`/`email` 已移出权威 DDL；残留的 `status` 属 conversations/量表表，为合法业务字段）、#12（无 `*;D` 残留）、#13（`user-svc/main.go:84` 注释与代码一致，`chat-svc/main.go:102` 的退避重试已恢复）、#14（E2E-04 报告行 #4~#7 已如实改写为"❌ 假 PASS / ⚠️ 未验证"）。

**两项附带遗留**：
1. `emotion-echo-web/e2e/helpers/` 删除 `auth.ts` 后成为**空目录**，`git status` 不可见，`check_residual.sh` 也未覆盖（该脚本只查 `*;` 空目录与缺末尾换行）⇒ R-03 #8 的扫描范围需扩到"空目录"。
2. **账本未回填**：`E2E-F-46/47/48/49/58/59` 在磁盘上均已修复，`discovered-unresolved.md` 仍标 `🔴 未解决` ⇒ AP-04（账本与 roadmap 脱钩）复发，且会让审计器 A5 产生**假 FAIL**。


### 验收标准（DoD）

- [ ] 15 项逐条有结论（补齐 / 降级 / 转阶段），**不允许"已知不改"而无记录**
- [ ] E2E-01~06 的状态与账本、report、plan 三处完全一致
- [ ] 存在未解决账本条目的阶段**不得标 `done`**
- [ ] 所有 `[V]` 测试点有截图（或明确降级为 `[A]` 并说明）
- [ ] SSR 有 ADR 且 `decisions.md` 有登记；7 处失效文档全部更正

### 边界（不做）

- 不重新执行 E2E-01~05 的实测（只补证据与状态）
- 不做 R-01 范围外的功能修复

---

## R-03 🔧 约束机制收尾（防复发）

**状态：🟡 部分完成（2026-09-18 第二方核对修正）** —— #1/#3/#4/#6/#8/#9 实测为真；**#5 未接通**（`required_status_checks` 实测为**空集** ⇒ AP-11「门禁只报不拦」仍然成立）；#7/#10 未做。另有两项**工具自身缺陷**见下。

### 第二方核对明细（2026-09-18）

**核对为真**：`e2e_stage_audit.py --selftest` 通过（新增 A6~A11 后仍复现 E2E-03/04/05 结论、对 E2E-02 无误报）；`check_tdd_gate.sh` / `check_adr_gate.sh` / `check_orphan_outputs.sh` / `check_residual.sh` 本地均绿；`anti-patterns.md` 回填表存在且口径诚实（10 条已机械化 / 4 条部分或人工）。

**#5 未接通（实测证据）**：`gh api /branches/main/protection` → `required_status_checks.contexts = []`、`checks = []`，即**没有任何 CI job 被要求** ⇒ CI 红不影响 PR 合并（AP-11 仍成立）。另：为注册 status check 而建的 `chore/trigger-ci` 分支**仍在本地与远端且未并入 main**（违反 AGENTS.md §2.5），而目标（required checks）并未达成 ⇒ 遗留分支 + 空门禁 = 双输。

**⚠️ 但更要紧的是：branch protection 已自锁死（见 E2E-F-68）**。实测 `required_pull_request_reviews.required_approving_review_count = 1` + `enforce_admins = true` + 仓库**仅 1 个协作者** ⇒ 直推被拒（`remote rejected ... Changes must be made through a pull request`），而 PR 又无法凑够 1 个 approve（GitHub 不允许自我 approve）。**当前后果：`main` 完全不可写**，`48ac75b` 与 `3662fae` 两个 commit 无法交付。D-08 只评估了 `required_status_checks` 的锁死风险，未覆盖"强制 PR + 1 approve + 单协作者"这一更强形态。**建议**（常见做法）：单人维护仓库保留 PR 流程但把 `required_approving_review_count` 设为 0，并按 D-08 只把**无 `paths` 过滤**的 job（`go-test`、`doc-drift-check` 各 job）加入 required checks；如需真实评审则增加第二个账号（机器人账号亦可）。**此项须先解，否则任何 agent 的产出都无法落地。**

**两项工具自身缺陷（会削弱审计器可信度，须先修工具再信结论）**：
1. **A4 会对"诚实陈述缺口"误报**。E2E-04 报告行 #4~#7 现已如实写"❌ 假 PASS / ⚠️ 未验证"，证据列形如"脚本已创建**但** `.output/public/index.html` 不存在"、"配置已新增，**但未实际运行**"——A4 按关键词命中即判 FAIL，无法区分"用存在性当完成证据"与"报告一个缺口"。更严重的是 `SELFTEST_EXPECT["e2e-04"]` 把这些命中**写成了期望值**，等于把误报固化进回归基线。修法建议：结果列为 `FAIL`/`未验证`/`N/A` 或证据含转折词（但/未/不/需/尚）时不再触发 A4。**当前后果**：E2E-04 的两个 FAIL（A4 + A5）均为**工具/账本假象**，非真实缺口。
2. **§13.3 的 #8（soft-assert 检测）与 #16（`required_status_checks` 非空）既无实现也无人工理由**：全仓 `grep -rn "soft.assert\|required_status_checks" scripts/ .github/` 零命中。#16 恰好就是仍然存活的 AP-11，属"最该机械化的那条"却未做。R-03 的 DoD 首条要求"17 条全覆盖，或对未覆盖项给出明确人工理由"⇒ 未满足（#16 有"待 CI 接通"的措辞，#8 完全无交代）。
3. **TDD 门禁范围漏洞**：`check_tdd_gate.sh` 的 `PROD_PATTERNS` 仅含 `\.go$`/`\.vue$`/`\.ts$`，不含 `\.sh$`/`\.py$` ⇒ 新增的 `scripts/check_residual.sh`、`scripts/e2e_stage_audit.py` 等一律免检，而脚本类产出正是 AP-13 的高发区（#7 未做与此互为因果）。且绿字声称"所有 5 个 commit 满足 TDD 要求"，实为**范围外绿灯**，措辞应限定为"已覆盖范围内"。


**目标**：R-00 只做了 5 条断言（MVA）。本节做**剩余机制 + 门禁接通 + CI 严格化**，让 14 类反例**全部**可机械发现或明确标注为何仍需人工。

**依赖**：R-00（审计器骨架）+ R-02（先补账，才知道基线在哪）
**已定决策**：D-08（分支保护取舍）/ D-07（第 3 项）

### 范围

| # | 机制 | 反例 | 具体做法 | 状态 |
|---|------|------|---------|------|
| 1 | 收口审计器（**补齐剩余 6 条断言**） | 全部 | 在 R-00 的 A1~A5 基础上补：A6 自检项无 `[x]+"待…"`；A7 `[V]` 点有截图；A8 账本编号连续；A9 plan/roadmap/report 三处 status 一致；A10 相对链接可达；A11 四值合法基值 | ✅ 已完成 |
| 2 | **证据有效性校验（强化）** | AP-01/02 | 检出 `soft-assert`（被注释的断言被当作 `[A]` 证据）；要求 `[A]` 项含可复现命令或输出片段 | ✅ A4/A7 已覆盖 |
| 3 | **TDD 门禁** | AP-09 | 校验"改动含生产代码路径时同批 commit 含 `_test.go`/`*.spec.ts`"；CI 增加 `go vet ./...`（会编译测试）**替代**仅 `go build` | ✅ `check_tdd_gate.sh` |
| 4 | **ADR 门禁** | AP-08 | 维护架构关键词清单（渲染模式/框架/存储/协议/认证）；命中时校验 commit 含 ADR 文件 + `decisions.md` 变更 | ✅ `check_adr_gate.sh` |
| 5 | **门禁真的能拦** | AP-11 | 按 **D-08** 落地：审计 job 设为 `required_status_checks`；**只 required"无 paths 过滤、每次必跑"的 job**（`go-test.yml`、`doc-drift-check.yml`），带 paths 过滤的 `web-test`/`llm-test` 仅报告；`strict: false`；**实测一次"红线被拒"** | 🟡 PR 要求已生效，status checks 待手动配置 |
| 6 | **孤儿产出物检测** | AP-10 | 新增 `scripts/*` 与 `.github/workflows/*` 必须被引用；新增 helper 必须有调用方 | ✅ `check_orphan_outputs.sh` |
| 7 | **脚本负向用例要求** | AP-13 | 迁移/清理/种子类脚本必须附负向测试（校验和不符报错 / 依赖不可达非零退出 / 清理范围以"所有含 `user_id` 表"为下限自动核对） | 🟡 `test_migrate_checksum.sh` 已写 |
| 8 | **残留物扫描** | AP-14 | 扫 `*;D` 类空目录、无末尾换行文件、`git status` 之外的未跟踪残留 | ✅ `check_residual.sh` |
| 9 | **反例集回填** | 全部 | 把 [anti-patterns.md](anti-patterns.md) 每条标注"已机械化 / 仍靠人工 + 理由"，形成活文档闭环 | ✅ 14 条标注完成 |
| 10 | **CI 严格化剩余 14 项** | E2E-F-30/31/32/34/35/55/56 | A3(`-race`，依 D-06 结论)/A4(覆盖率)/A5(集成测试)/A6(格式)/B2(typecheck)/B3(Playwright 进 CI)/B4(构建验证)/B5(engines+packageManager)/C1(锁版本)/C2(models pytest)/C3(pytest 覆盖率)/D2(README 措辞)/D3(secret 扫描)/D4(其余套件)；并给 `doc-drift-check.yml` 补 `timeout-minutes`/`concurrency`/`permissions` | ⏳ 待后续 |

### 验收标准（DoD）

- [ ] 审计器覆盖 §13.3 全部 17 条断言（或对未覆盖项给出明确的人工理由）
- [ ] 审计器接入 CI 并**实测能拦住**（按 D-08 配置后，故意红 PR 试合被拒）
- [ ] 14 类反例逐条标注机械化状态
- [ ] `anti-patterns.md` 每条硬规则对应到一条检查项或明确的人工理由
- [ ] CI 严格化 14 项逐条有结论（已修 / 明确排除并记录理由 / 转阶段）

### 边界（不做）

- 不追求"零人工"——`[M]` 类判定与产品决策仍需人
- 不做通用代码质量平台（只做 E2E 收口相关）

---

## 与既有阶段的衔接

| 既有项 | 处置 |
|--------|------|
| **E2E-03 阶段 2（CI 严格化）剩余 14 项** | 未修 14 条：A3(`-race`)/A4(覆盖率)/A5(集成测试)/A6(格式)/B2(typecheck)/B3(Playwright 进 CI)/B4(构建验证)/B5(engines+packageManager)/C1(锁版本)/C2(models pytest)/C3(pytest 覆盖率)/D2(README 措辞)/D3(secret 扫描)/D4(其余测试套件)。归 **R-03 第 10 项**统一收口 |
| **`doc-drift-check.yml` 自身缺加固** | 11 个 job 全无 `timeout-minutes`/`concurrency`/`permissions`（正是刚被记为缺陷的 A7/A8/A9）→ 归 R-03 |
| **E2E-04 未完成项** | 4 个假 PASS 对应的真实工作（build smoke 真跑、mobile project 真跑、a11y 真跑）→ 归 R-02 补账 + 必要时重执行 |
| **E2E-06 状态** | `done` → `partial`（待 R-02 第 5 项处理） |
| **E2E-07 及以后** | **阻塞**，R-01 完成前不得启动 |

---

## 执行路径总览

```
R-00 审计器 MVA（5 条断言 + --selftest 复现本次审查结论）
  │   出口条件：能检出 E2E-03/04/05 的已知缺口；对 E2E-02 不误报
  │   ⚠️ 跑不出已知缺口 ⇒ 审计器无效，不得进入 R-01
  ▼
R-01 阻断修复（安全 + 功能 + 编译 + CI 转绿）
  │   出口条件：4 个 workflow 全绿；密保校验有负向测试；测试可编译；-race 已定性
  ▼
R-02 收口补账（15 项契约补齐 / 状态诚实化）
  │   出口条件：审计器对 E2E-01~06 全绿（或明确 partial 且有理由）；SSR 有 ADR
  ▼
R-03 约束机制收尾（剩余 12 条断言 + 门禁接通 + CI 严格化 14 项）
  │   出口条件：§13.3 17 条全覆盖或有人工理由；门禁实测能拦
  ▼
恢复路线图：E2E-07 找回密码 → … → E2E-30
```

**R-00 必须最先**（否则后续补救自己也会造假账）；**R-01 必须在任何新阶段之前**。
R-02/R-03 可与路线图后续阶段并行，但须先于"下一批阶段的收口"。

---

## 决策状态（原「待用户决策」→ 已按常见做法定案）

用户 2026-09-18 授权"参考常见做法自行定案，先行解决问题"。4 项决策已落 [decisions.md](decisions.md)，**不再阻塞开工**：

| 原决策项 | 结论 | 决策号 | 依据的通用实践 |
|---------|------|--------|--------------|
| 注册链路断裂修法 | **后端先回退为"密保可选"**，注册恢复可用；前端录入 UI 归 E2E-09 并**重新打开强制** | **D-05** | Expand → Migrate → Contract（先向后兼容再迁移）；避免"先强制、后补客户端"的自伤式中断 |
| `-race` 最终处置 | **先查明再定，禁止直接降级收口**（R-01 给出 3 步可复现调查：Linux 容器复现 / 空测试验 `-race` 可用性 / 读 CI 首条错误行） | **D-06** | Go 官方：`-race` 需 `CGO_ENABLED=1` + C 工具链，环境不满足的表现正是**全模块统一失败**；真实数据竞争则表现为**特定包**打印 `DATA RACE`。先定性再定手段 |
| `doc-drift-check` 红态 | **分类处置**：env 变量直接修；digest 占位改"显式 WARN + 已知缺口声明"（非静默通过）；migration Fail 13 先定性。**不使用 `continue-on-error`** | **D-07** | 主分支红 = broken build，标准做法是 fix-forward/revert 而非抑制；无法修复项用 known-debt allowlist（附原因 + 复检条件） |
| `enforce_admins` 与 required checks | **保持 `enforce_admins: true`**；**只 required"无 `paths` 过滤、每次必跑"的 job**（`go-test`/`doc-drift-check`），带 paths 过滤的仅报告；`strict: false` | **D-08** | 锁死风险只来自"被 required 却不会每次上报的 check"；GitHub 明确带 `paths` 过滤的 workflow 在路径不匹配时不运行、不产生 check run |

> 仍待用户决定的只剩 **D-04（i18n 是否立项）**，且它不阻塞任何阶段。
