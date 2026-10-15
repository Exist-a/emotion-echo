---
stage: cl-02
title: gRPC 契约闭环（死链逐条终态 + 守卫升级 + "严格走 gRPC"坐实）
type: transformation
status: done（第二方核对三轮 PASS）
created: 2026-10-14
last-updated: 2026-10-15（运行时验证完成 → 收口：报告 + 账本对账 + 来源计划迁移 + 第二方核对）
owner: lane-cl-c（C 轨）
---

# CL-02 · gRPC 契约闭环 — 执行记录

> 阶段详档：[plan.md](plan.md) · 证据：[`evidence/phase1-scan.md`](evidence/phase1-scan.md) ·
> [`evidence/phase2-3-terminal-states.md`](evidence/phase2-3-terminal-states.md) ·
> [`evidence/phase2-6-runtime-verify.md`](evidence/phase2-6-runtime-verify.md)
> 执行协议：[RUNBOOK](../../RUNBOOK.md) · [parallel-contract](../../parallel-contract.md) · [anti-patterns](../../../e2e-roadmap/anti-patterns.md)

## §0 汇总

- **测试点**：24 个（T-01~T-24），**全部落在四值内**：**PASS 23 / N/A 1 / 未做 0 / BLOCKED 0**。
  （2026-10-15 第二方核对 C1 指出：此处原写 PASS 22 / 未做 1 与表格里 T-20 已 PASS 自相矛盾，已按表格更正。）
- **核心交付**：E2E-31「保留 + 标注 + 计数守卫」→ **每条 RPC 有终态**：**清除 3 条**（`Logout` / `VerifySecurityAnswer` / `AnalyzeBatch`）+ **归档 4 条**（`StreamMessages` + 三条 mental-health）。
- **守卫**：新增 `scripts/check_rpc_status.sh`（**6 条断言**：A1~A6，见 §5 经验 9）+ 状态清单 `scripts/rpc_status.tsv`（43 行）；旧守卫 `check_dead_grpc_inventory.sh` 的 `EXPECTED=10` 改为**从清单推导**；CI 新增「守卫 27/27」。
- **起栈实测**：dev + ai profile，持 `.devmode-session`（owner `lane-cl-c`，06:36–07:40）；7/7 服务 healthy、Nacos `count:6`、**smoke 24/24 PASS（exit 0）**。
- **裁定**：7 项 [M] 全部拍板（`decisions.md` CL-D-08~10）；其中 **2 处用户初选"接真"经查与已收口 ADR 冲突 ⇒ 按文档改判**。
- **销账**：**CL-F-05**（7 条死 RPC）✅；CL-F-07 / CL-F-08 补实例；新登 **CL-F-16**。
- **口径更正**：决策 4 ADR 加「更正记录」——§四「100%」应读作"方法面已具备"，不等于"每条 RPC 都有生产调用方"。

---

## §1 起点（做之前是什么样）

**用户口径（roadmap §4）**：后端各服务间调用**严格走 gRPC**；HTTP 仅作显式降级（`*_TRANSPORT=http` 回滚开关）；"gRPC 搞了很久，本阶段要把它**坐实**"。

**亲核到的现状**（§A.1 全表见 [plan.md](plan.md)）：

1. **主干已 gRPC 化**：5 个 `*_TRANSPORT` 默认值 = grpc；非测试 Go 代码里 `http://emotion-*` / `http://localhost:8*` **零命中**；assessment 的 `assessmentBase != ""` 恒真旁路 E2E-31 已删 ⇒ **本阶段不是"再迁一批 HTTP"，而是"减面 + 坐实留下的"**。
2. **但 7 条 RPC "挂着没站着"**：server 实现存在、全仓无生产调用方；处置仅是"注释 + 计数守卫"（E2E-31，用户 2026-10-09 裁定"保留 + 标注"）⇒ **标注 ≠ 可用 ≠ 消失**。
3. **三类死法并存**（不能一刀切）：① 无 client（`StreamMessages` / 三条 mental-health）；② 有 client 不在接口内（`Logout`）；③ 在接口内但零调用（`VerifySecurityAnswer`）；且三条 mental-health 里 `History` 是**恒返空占位**、`Trigger`/`Trend` 是**真实现**。
4. 🔴 **RPC 名 ≠ 客户端方法名（14 条）**：`ReportsTrend→TrendReport`、`ListMyResults→ListResults`、`UpdateProfile→UpdateMe`、`GetUserById→GetByID` …
   ⇒ "这条 RPC 有没有人调"**不能靠 RPC 名 grep**（照名字查会把 6 条 metric RPC 误判成死链）。
5. **同型撞名陷阱**：`SendMessage` 撞 Kafka producer、`Register` 撞 Nacos 与 BFF 路由注册。

## §2 实施（做了什么）

| Phase | 内容 | 结果 |
|---|---|---|
| 0 | 7 项 [M] 裁定（含"按文档复核"） | ✅ 见 §4 裁定表 |
| 1 | **守卫升级**：`rpc_status.tsv`（43×7 列）+ `check_rpc_status.sh`（A1 覆盖 / A2 live 必有**非注释**调用方证据 / A3 标记数与清单一致 / A4 终态规则（按 rpc 声明块）/ A5 HTTP 豁免 / A6 锚点自检） | ✅ 全绿 + 负向对照 3 组能红 |
| 2 | **清除 3 条**：proto + server + BFF client（含 HTTP 实现）+ 拦截器跳过清单 + 测试 全删；桩按原工具链重生成 | ✅ 7 模块 build/vet/test 全绿 |
| 3 | **归档 4 条**：`option deprecated = true` + 依据与重启条件注释 | ✅ 守卫 A4 通过 |
| 4 | 接真类 | ✅ **N/A**（裁定后无接真项） |
| 5 | 口径更正 + 账本回填 | ✅ 决策 4 ADR 加「更正记录」；E2E-F-208 状态说明更新 |
| 6 | 运行时验证 + CI 接线 | ✅ 见 §3 |

**两处"用户初选 → 文档冲突 → 复问改判"**（用户指令"根据文档来，不要看注释"）：

| 项 | 用户初选 | 查到的文档 | 改判 |
|---|---|---|---|
| `Logout` | 接真（实现真登出） | `adr-2026-10-jwt-key-rotation-dual-key`「**不做** token 黑名单/主动吊销」+ D-46 备选③被判"量大不成比" | **清除** |
| 三条 mental-health | 接真（接入） | `adr-2026-09-chart-contract-alignment §E`「5 个无消费端点本轮**不接**（需 UX 设计）」 | **归档** + 另登 backlog |

**顺带完成的移交**：B 轨 CL-01 收口时把 `downstream/chat.go` 的 HTTP 出站埋点**明确豁免并移交本阶段**（该文件在 C 轨独占列）⇒ 本阶段补齐（`trace.NewTracedClient`，与 `ai.go` 等一致）。

## §3 证据（怎么证明真的生效）

| 类别 | 证据 |
|---|---|
| 守卫 | `check_rpc_status.sh` A1~A6 全绿（`PASS: A1 覆盖 proto 40 条全部已登记；清单 43 条 = proto 40 + 已清除 3`）；旧守卫同绿（期望值推导） |
| **负向对照 3 组** | ① 改错证据符号 → A2 FAIL；② `deleted` 改 `live` → A1 FAIL；③ 插未登记标记 → 旧守卫 FAIL。还原后均 exit=0 |
| **CI 真能红** | 临时分支 + draft PR #218（唯一改动 = 故意改坏一行 TSV）⇒ `static-guards` **failure**（annotation: exit code 1），其余 11 job 全 success；正式分支同 job = success。PR 已关、分支已删 |
| 静态 | `go build`/`go vet`/`go test -count=1` **7/7 模块全绿**；`pytest tests/unit` 208 passed；`compileall` OK；`check_residual`/`check_orphan_outputs`/`check_proto_layout` rc=0 |
| **运行时（T-21）** | 7/7 服务 healthy；**Nacos `count:6`**；apisix-seed `6 upstreams + 16 routes`；网关登录 200 |
| **运行时（T-23）** | `smoke_data_layer.py` **24/24 PASS，exit 0**（首跑唯一功能性失败 §4 504 经重跑即过 ⇒ E2E-F-115 冷启动，非回归） |
| **运行时（T-15）** | 5 个服务容器 env **均未设置** `*_TRANSPORT` ⇒ 走 `config.go` 默认 grpc |
| 功能点检 | 登出 200 / `security-questions` 200 / `verify-security-answer` 401（预期拒绝）⇒ **D4 裁定前提的运行时佐证**（找回密码走保留的 `…ByUsername`） |
| H5 复核 | 7 个容器镜像均为本轨 `:cl02-verify`（未覆盖公共 tag） |

**RED 运行暴露并修掉守卫自身 2 个缺陷**（否则是"永真断言"）：
① `deleted` 断言原写 `rpc[[:space:]]+X\(`，而 proto 实际是 `rpc X (`（名与括号间有空格）⇒ **永不命中**；
② `archived` 断言原只看**整文件**是否含 `reserved/deprecated` ⇒ 改为按清单 `proto:line` 取该声明邻域。

## §测试点总表

| # | 测试点 | 结果 |
|---|---|---|
| T-01 | `StreamMessages` 归档落地 | ✅ PASS |
| T-02 | `Logout` 清除落地 | ✅ PASS |
| T-03 | `VerifySecurityAnswer` 清除 | ✅ PASS |
| T-04 | `AnalyzeBatch` 清除 | ✅ PASS |
| T-05 | `MentalHealthHistory` 归档 | ✅ PASS |
| T-06 | `MentalHealthTrigger` 归档 | ✅ PASS |
| T-07 | `MentalHealthTrend` 归档 | ✅ PASS |
| T-08 | 清除类无残留引用 | ✅ PASS |
| T-09 | 状态清单覆盖不重不漏 | ✅ PASS |
| T-10 | `deleted` 不残留 proto | ✅ PASS |
| T-11 | `archived` 必带 deprecated | ✅ PASS |
| T-12 | `live` 必有调用方证据 | ✅ PASS |
| T-13 | 负向对照（守卫必须能 FAIL） | ✅ PASS（3 组） |
| T-14 | HTTP fallback 豁免 | ✅ PASS |
| T-15 | `*_TRANSPORT` 默认 grpc（静态 + 运行时） | ✅ PASS |
| T-16 | 非开关式 HTTP 直连零残留 | ✅ PASS |
| T-17 | 模型服务边界（D7） | ✅ PASS（裁定：不算） |
| T-18 | 决策 4 ADR 加更正记录 | ✅ PASS |
| T-19 | E2E-F-208 说明回填 | ✅ PASS |
| T-20 | 来源计划迁 `legacy-plans/landed/` | ✅ PASS（本文件落地时完成，见 §收口自检 5） |
| T-21 | 清除后全栈起得来 | ✅ PASS |
| T-22 | 接真类端到端有数据 | ✅ N/A（裁定后无接真项） |
| T-23 | 相关功能无回退（§2.4 smoke） | ✅ PASS（24/24） |
| T-24 | 守卫接入 CI 且真能红 | ✅ PASS（CI 实测） |

## §4 结论

**功能结论**：43 条 RPC **每条都有终态**——36 条 live（有调用方证据）、4 条 archived（proto 保留 + 明确不做 + 重启条件）、3 条 deleted（全仓消失）。
**不存在"仍在 proto 里挂着但没人用"的第八条**（穷尽核对：43 条全部定位到调用点）。

**裁定表（7 项 [M]，2026-10-14 用户经 AskUserQuestion 拍板）**：

| # | 事项 | 裁定 | 依据 |
|---|---|---|---|
| D1 | 三条 mental-health | 全部**归档** | ADR chart-contract-alignment §E |
| D2 | `StreamMessages` | **归档** | ADR decision-4-closure §四#3「故意不做」 |
| D3 | `Logout` | **清除**（不接真） | ADR jwt-key-rotation「不做 token 黑名单/主动吊销」 |
| D4 | `VerifySecurityAnswer` + `AnalyzeBatch` | **清除** | 族内冗余 / 无批量业务（动手前已核零调用，删后 grep 复验） |
| D5 | mental-health 的 HTTP 路由 | 与 D1 同进同出（标注未接出） | — |
| D6 | 新旧守卫 | **叠加 + 去掉硬编码** | 消除两处同步负担 |
| D7 | 模型服务（FER/SenseVoice/XTTS） | **不算**服务间调用 | 模型推理服务以 HTTP 暴露属设计 |

**机器结论**：守卫 6 断言（A1~A6）+ 负向对照 **3 组（阶段内常备，见 §3）** + **2 组（第二方核对整改期复现的绕过手法，见 §第二方核对）** + CI 实测能红；7 模块 build/vet/test 全绿；smoke 24/24。

## §4.1 已知偏差（第二方核对指出，如实登记）

| # | 偏差 | 说明 | 处置 |
|---|---|---|---|
| 1 | **RED 与 GREEN 压在同一 commit**（`4134a27`：清单改终态 + 代码改动） | 违反 AGENTS §2.1「先提交失败测试、再提交实现」的分离节奏 ⇒ RED 态无法从 git 历史复现 | 不回溯改写历史（分支已推送、PR 在飞）；**RED 输出已固化在证据档**（[phase2-3-terminal-states.md](evidence/phase2-3-terminal-states.md) §2）。后续阶段应拆两次提交 |
| 2 | 守卫两处**假绿/可绕过**由第二方核对发现（F-1 A4 锚点过期、F-2 注释可绕过） | 说明"自己写的守卫自己验"不可靠 | 已加固 + 补 A6 自检；两个绕过手法均已复现为 FAIL（见 §第二方核对） |

## §5 经验（可复用）

1. **"有 server 实现" ≠ "已接线"**：决策 4 把 `Logout`（client 不在接口、零调用）计入 100% 覆盖，就是按"方法面存在"计数的后果 ⇒ **覆盖面口径必须写清"方法面"还是"生产调用方"**。
2. 🔴 **同名符号 grep 不可靠**：`Register` 撞 Nacos、`SendMessage` 撞 Kafka producer；且 **RPC 名与客户端方法名 14 条不同名** ⇒ 机械断言必须**登记 (文件, 符号)** 再校验，而不是"按名字全仓 grep"。
3. **守卫要先证明自己能红**：本次 RED 运行直接暴露了守卫自身 2 个"永真断言"（deleted 断言因 proto 里的空格永不命中、archived 只看整文件）——**没有负向对照的守卫等于装饰**。
4. **"CI 里能不能红"要真开 PR 验**：本仓 5 个 workflow 只监听 `push/PR → main` ⇒ 推分支不跑 CI；只有开 PR 才能实测门禁。
5. **用户凭印象的结论必须与已收口文档核对**：本轮两处"接真"初选都与 ADR 冲突，按文档改判；**代码注释同样会过期**（可信度：ADR/decisions > 代码 > 注释）。
6. **worktree 构建镜像会缺 gitignored 材料**：`deploy/tls/` 缺失直接让 `ai-svc` 构建 COPY 失败（CL-F-07 的构建期实例）——"worktree build + 共享目录 up"两步法要额外把 TLS 材料带过去。
7. **`/health/ready` 是启动期快照**：服务若早于依赖（postgres）就绪而启动，DB 单次尝试失败 ⇒ repo 永久 nil ⇒ readiness **永久 503 且不自愈** ⇒ **重建服务必须在依赖就绪之后**（否则得到一串假 unhealthy）。已登 **CL-F-16**。
8. **同一 tag 的镜像"后者赢"**：本轨全程用 `:cl02-verify` 后缀（H5），并在验收前用 `docker ps --format '{{.Image}}'` 复核跑的是不是自己的代码。
9. **守卫的锚点必须自检**：断言若依赖"清单里的行号/文件名"这类**外部锚点**，锚点一旦过期，断言就会去校验错误的对象 —— 表现是**删掉真正该守的东西仍 PASS**（假绿），而只改注释字样反而 FAIL（假红）。CL-02 的 A4 就是这样被骗过一轮（第二方核对 F-1），修法是①断言绑定**结构化范围**（按名字解析声明块）②新增 A6 锚点自检把"锚点是否仍指向目标"变成显式断言。**同理：找"调用方"必须排除注释**（F-2/N-1 两轮）。

## §收口自检（逐条过 RUNBOOK §6）

| # | 契约项 | 结论 |
|---|---|---|
| 1 | plan 每个测试点给四值之一 + 证据 | ✅ 24/24（T-22 = N/A 附裁定；T-20 本文件落地时完成） |
| 2 | 回归钉已落地且绿 | ✅ `scripts/check_rpc_status.sh`（6 断言 A1~A6，CI 守卫 27/27）+ `scripts/rpc_status.tsv`；负向对照 3 组 |
| 3 | `report.md` 用模板（起点/实施/证据/结论/经验） | ✅ 本文件 |
| 4 | 账本对账 | ✅ **本阶段名下无未解决 CL-F-xx**（CL-F-05 已销 ✅；新发现 CL-F-16 归 CL-09；CL-F-07/08 仅补实例） |
| 5 | 落地计划迁 `docs/legacy-plans/landed/` | ✅ 已迁（`status: landed` + 修路径引用 + `docs/plans/README.md` 标已落地） |
| 6 | roadmap 状态同步 + 分支收口 | ✅ **已做** —— `.devmode-session` 已释放（07:40）；roadmap CL-02 行 → `done`；PR #214 转 ready → squash 合并 → 删源分支与 worktree（本轮完成） |
| 7 | 第二方核对 | ✅ **已做（独立子代理三轮）**：一轮 `FAIL`（7 条）→ 二轮 `FAIL`（4 条，均为整改引入）→ **三轮 `PASS`**。见 §第二方核对 |

## §第二方核对（独立子代理 · **三轮**）

**方法**：另起独立子代理（general-purpose，**只读**：不改文件/不 commit/不动容器），给它**具体可复现的手法清单**（而不是让它复述我的结论），并要求它"找到问题比给 PASS 有价值"。三轮均在同一子代理上下文内进行（第二轮起只复核整改项）。

### 第一轮：`SECOND-PARTY: FAIL`（7 条 FAIL + 2 条轻微）

| # | 问题 | 处置 |
|---|---|---|
| F-1 | 🔴 **A4 的 archived 断言根本没校验到被归档的 RPC**：清单 `proto:line` 在 proto 增删后**过期**（StreamMessages 指 65/实 70 等 4 条），窗口落在注释/邻居上 ⇒ **删掉真正的 `option deprecated = true;` 仍 PASS**（假绿），只改注释字样反而 FAIL（假红） | ① 重算全部 43 条锚点（修 8 处）② A4 改为**按名字解析 `rpc X { … }` 声明块** ③ **新增 A6 锚点自检**（清单行号必须 == 实际声明行） |
| F-2 | 🔴 **A2「live 必有调用方」可被注释绕过**（符号出现在注释里也算命中）⇒ 注释掉唯一真实调用仍 PASS | A2 改为只在非注释处找命中（后续 N-1 又加固一轮） |
| F-3 | report §收口自检 row 6 标 ✅ 与现状相反（roadmap 仍 in-progress、分支/worktree 仍在） | 改为 ⏳ 待合并时执行（本次合并时落地） |
| F-4 | T-08「清除类无残留」不成立：3 个测试文件仍有**非注释**死 fake | 3 处死 fake 已删；T-08 口径收紧并如实标注"同名但不同层"与 `legacy/` 归档目录 |
| F-5 | 24 点统计自相矛盾（§0 写 PASS 22/未做 1，表格里 T-20 已 ✅） | 统一 **PASS 23 / N/A 1 / 未做 0 / BLOCKED 0** |
| F-6/E8 | 共享文件改动（`deploy/docker-compose.cl02-verify.yml`、`decisions.md`）未登 §四握手区 | 走独立小 PR **#225** 补登（已合并 `ce11df9`） |
| F-7 | 孤儿产出物（`cl02-up-verify.sh` 零引用、新计划未入 README、脚本硬编码绝对路径） | 证据档显式引用 + 脚本改 `CL02_WT`/`CL02_SHARED` 可覆盖 + 新计划入 `docs/plans/README.md` |
| E2/E4/E5 | 守卫头注释"尚未接入 CI"与实际相反 / CL-F-16 缺对照 / RED 与 GREEN 压在同一 commit | 逐条更正；偏差如实登记在 §4.1 |

### 第二轮：`SECOND-PARTY: FAIL`（4 条，**均为整改自身引入**，非"未修"）

| # | 问题 | 处置 |
|---|---|---|
| N-1 | A2 加固只挡"整行注释"，**行尾注释**仍可绕过（`… doSend(id, req) // legacy h.chat.SendMessage( removed`） | A2 改为逐行取"符号之前的部分"，若其中含 `//` 则该行不算命中（**已实测该手法转 FAIL**） |
| N-2 | 证据档"复跑前改这两行"与已可移植的脚本相反 | 改为"可用 `CL02_WT`/`CL02_SHARED` 覆盖" |
| N-3 | §0 引用"§5 经验 9"但该条不存在 | 补第 9 条「守卫的锚点必须自检」 |
| N-4 | 断言数 5 vs 6 未统一 | 全量统一为 A1~A6（6 条）；`phase1-scan.md` 保留带说明的历史注 |

**另：本轮我自查发现 CI 真红（第二方核对未提）**：`check_tdd_gate.sh` 报 `RED: 1/12 commit 违反 TDD` —— 文档收口 commit 里夹了生产代码 `downstream/chat.go` 却无测试。修法 = **重写最近两个 commit**：`b54b182`（文档+守卫+死 fake 清理，不含生产 .go）+ `27defba`（`chat.go` + 新钉子 `chat_http_traced_client_test.go`）⇒ 门禁 GREEN。

### 第三轮：`SECOND-PARTY: PASS`（2026-10-15）

逐项复现确认：N-1 手法 ⇒ A2 FAIL（基线 36 条无误伤）；N-2/N-3/N-4 均已更正；TDD 门禁 GREEN；`27defba` 确认"生产代码 + 测试"同 commit；新钉子的负向对照成立（Transport 换 `http.DefaultTransport` ⇒ 断言失败 `实际 *http.Transport`）。

**第三轮指出的 2 处非阻塞措辞残留**（已清）：§2 表断言列举未含 A6；"负向对照 5 组"与别处"3 组"口径不一（现写清"3 组常备 + 2 组整改期复现"）。

### 核对未能独立验证的项（如实保留，不默认采信自述）

1. **运行时证据**（T-21 7/7 healthy + Nacos `count:6`、T-23 smoke 24/24、T-15 容器 env、功能点检）：**栈已停**且核对者被要求不动容器 ⇒ 未独立复跑，沿用执行者自述 + 证据档。
2. **CI 实跑**（PR #218 的 `static-guards` failure、本分支 success）：核对者无 GitHub 鉴权 ⇒ 未独立复跑；仅静态确认守卫已作为 CI 步骤接线（会拦）。
3. **PR #225 的 §四 握手登记**：不在本分支 diff（按约定不判 FAIL）。
4. **`.devmode-session` 释放 / §3.1 第 3 行**：gitignored + 共享目录，worktree 内不可见。
5. **`protoc` 重生成幂等性**：未重跑工具链。
6. **force-push 后远端一致性**：未与远端逐对象比对。

### 结论

`SECOND-PARTY: PASS`（2026-10-15，三轮：FAIL → FAIL（整改引入）→ **PASS**）。
阶段状态可由 `in-progress` 翻 `done`（roadmap 同步 + PR 转 ready → squash 合并 → 删源分支/worktree）。
**超范围观察**（非本阶段产物，仅记录）：`scripts/check_tdd_gate.sh` 有 5 次 `line 131: [: -: integer expression expected` 的既有小 bug（对 `-` 做算术比较），`rc` 仍为 0、判定正确。
