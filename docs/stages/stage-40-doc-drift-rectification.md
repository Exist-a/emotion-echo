---
status: landed
stage: 40
target: todo-pile-2026-09-04 自查发现 + 全面文档失真修复
date: 2026-09-04
related:
  - docs/architecture/adr/adr-2026-09-doc-drift-registry.md（决策 18）
  - docs/plans/todo-pile-2026-09-04.md（修复对象）
  - docs/architecture/decisions.md（决策 9/11/12 收口）
  - QUICKSTART.md（端口表修正）
  - README.md（"唯一入口"措辞修正）
---

# Stage 40 — todo-pile 自查 + 全面文档失真修复

## 一、缘起

`docs/plans/todo-pile-2026-09-04.md` 是 2026-09-04 当天合并前梳理的"未做项清单"，
列了 A 区（用户路径）3 条 + B 区（架构）4 条 + C 区（文档失真）8 条 + D 区（杂项）6 条。
本 Stage 不做新功能，**只做一件事**：按 AGENTS.md §0.2 + 决策 18 §4.1 功课，
对 todo-pile 的每一条断言做代码实测，并把失真**就地更正**（决策 18 §4.4）。

> 决策 18 §三类型 5 自报告失真的预言——"作者本人就是探测者，结论与事实错位而本人未察觉"——
> 在 todo-pile 的 A1 + B4 上**首次应验**。本 Stage 是该类型的第一个修复案例。

---

## 二、实测范围与方法

按 AGENTS.md §0.2 六步功课：

1. **读相关代码**：本轮读了 12 个文件（BFF `main.go` + `tts_handler.go` + `upload_handler.go`、
   `auth_handler.go`、ai-svc `aiclient/{fer,sensevoice,xtts}.go` + `etc/ai-api.yaml`、
   `docker-compose.apps.yml`、`aiclient/xtts.go` 头注释、`QUICKSTART.md`、
   `decisions.md` 决策 9/11/12/18 + 架构全景图 + 端口表 + 治理层段、
   `README.md` 状态段）
2. **读相关 ADR**：决策 9/11/12 全文 + 决策 18 全文 + 决策 10（Nacos）配置范围段
3. **跑现状 smoke**（部分）：对 todo-pile 已跑过的 smoke 不重复；本轮只读代码 +
   读已有 ADR/stage 文档，**未跑新 smoke**（依据：todo-pile C7 的 smoke 回查是独立待办）
4. **网上信息**：不涉及
5. **列架构假设清单**：见 §四"实测结果矩阵"
6. **回填 commit message**：本次无代码改动，仅文档；修复即落地

---

## 三、修复对象总览

| 类别 | 文件 | 改动性质 | 行数 |
|---|---|---|---|
| 主修复 | `docs/plans/todo-pile-2026-09-04.md` | A1/B4 失真就地更正 + A2 行号校正 + B1 表述精度补正 + C5/C8 范围扩展 | 5 处就地更正块 |
| 主修复 | `docs/architecture/adr/adr-2026-09-doc-drift-registry.md` | §二 增补登记实例 #12（todo-pile A1/B4 自报告失真）+ 实例 #13（todo-pile A2 行号）+ §三 类型 5 新案例 | 1 处表追加 |
| 主修复 | `docs/architecture/decisions.md` | 决策 9/12 末尾各加"关系说明"段；架构全景图加 APISIX 层 + 注明 BFF 是 upstream；端口表加 APISIX 行 + 修正 web-bff 措辞；治理层段 ☐→✅；FER/sensevoice/XTTS 行加 "BASE_URL 默认空" 说明 | 6 处 |
| 主修复 | `QUICKSTART.md` | 第 27 行 BFF 描述 + 第 43 行端口表 + 新增 APISIX :19080 行 + 加 2026-09-04 更正块 | 3 处 |
| 主修复 | `README.md` | 第 176 行"唯一入口"措辞更正 + 第 181 行"APISIX 退役"演进记录注释 | 2 处 |
| **本文档** | `docs/stages/stage-40-doc-drift-rectification.md` | 修复报告 | 新增 |

---

## 四、实测结果矩阵（10 条 todo-pile 条目逐条核验）

| # | 条目 | 文档声明 | 实测事实 | 状态 | 修复 |
|---|---|---|---|---|---|
| A1 | TTS 不可用 | 路由 + 503 正确；`xtts.go` 空即返 nil；容器已删 | 路由对、503 对；`xtts.go` 151 行完整实现（构造时 BaseURL 空才返 nil）；compose `apps.yml:387-389` 注入 `XTTS_BASE_URL:-http://emotion-echo-xtts:8003`；`apps.yml:518` 服务定义仍在 | **部分失真**（根因写错） | A1 段就地更正块 |
| A2 | 文件上传未实现 | 前端单数 vs BFF 复数 + `main.go:232` 注册 | 路径不匹配属实；`main.go:236`（不是 232，232 是 `NewSurveyHandler`）；前端 `useFileUpload.ts:39-48`（不是 42-46）；handler 不论路径对错一律返 502 | **事实成立 + 行号错** | A2 段就地更正块 |
| A3 | 前端 dev server 未起 | 用户必须手工 `pnpm dev` | QUICKSTART/QUICKSTART 都有写 | **事实成立** | 不在本 Stage 修；Sprint 1 加 "5 秒必读" 框 |
| B1 | BFF 8894 端口 | dev 例外 + BFF 不验签 | `apps.yml:602-604` 注释 + `apps.yml:594-596` 注释共同说明 `BFF_TRUST_APISIX=true/false` 两种行为 | **事实成立 + 表述精度问题** | B1 段就地更正块 |
| B2 | Nacos SDK v2.3.5 vs Server v2.4.3 long poll | 路径不匹配 | shared `go.mod:19` SDK v2.3.5；`infra.yml:142-144` Server v2.4.3；项目代码用 SDK 封装，外部不可证伪 | **可推断但不可证伪** | 不在本 Stage 修；B2 不变 |
| B4 | fer/sensevoice/xtts 已删 | Stage 39 §五删除；aiclient 空实现；前端入口已砍 | `apps.yml:437/473/518` 服务定义全部仍在；`aiclient/{fer,sensevoice,xtts}.go` 是 130/118/151 行完整实现；前端 `useFaceEmotion.ts:121` / `useTTSPlayer.ts:180` / `useFileUpload.ts:39-48` 入口仍在 | **完全失真**（决策 18 §三 类型 1 + 5 同型） | B4 段整段重写 |
| C1-C3 | 误诊纠正 | 已合并 | 已合并 | ✅ | 不在本 Stage 修 |
| C4 | Stage 36-D Bug 2 修复从未生效 | `02:118-119` 注释错位 | 已就地修正（02 删裸写、001 用 CONSTRAINT 独占） | **事实成立 + 已自纠** | 不在本 Stage 修 |
| C5 | QUICKSTART "BFF 唯一入口" | 与决策 11/12 矛盾 | QUICKSTART:43 确实这么写；决策 9 / 全景图 / 端口表都仍称 web-bff 为"唯一前端入口"——决策栈内部本身语义矛盾 | **表述精度问题**（决策栈未收口是根因） | QUICKSTART 第 43 行改写 + decisions.md 决策 9/12 关系说明 |
| C6 | quick-login 端点 | 前端期望但 BFF 无 | `pages/login/index.vue:175-185` 注释明写"后端从未实现 quick-login 端点"；BFF `auth_handler.go:5-10` 只注册 5 个端点 | **完全成立** | 不在本 Stage 修；Sprint 1 处理 |
| C7 | dashboard 空 / msg_summary_v | `/reports/daily` 查 `msg_summary_v` | `04-create-views.sql:14-26` 视图定义在；`analytics_handler.go:6,39` 透传；`report_repository.go:165-188` 用 COALESCE 查视图 | **完全成立** | 不在本 Stage 修；C7 是独立待办 |
| C8 | BFF 路由清单无契约测试 | main.go 散装注册，无断言 | `main.go:230-240` 散装；测试目录无完整路由断言 | **完全成立**（且不止 BFF 单边——前端 composables + APISIX seed 也都是清单缺陷） | C8 段范围扩展 |

**统计**：
- 完全失真（事实级错误）：**2 条**（B4、A1 根因链）
- 表述精度问题（决策栈未收口）：**3 条**（B1、A2 行号、C5）
- 事实成立：5 条（A3、C4、C6、C7、C8 — 但 C8 范围需扩展）
- 不可证伪：1 条（B2）

---

## 五、根因归纳

### 5.1 决策 18 §三类型 5 的首次应验

todo-pile A1 + B4 是**作者本人是探测者**的失真典型：
- 看到 dev 默认行为（XTTS 503）→ 推断"容器被删"
- 看到 ai-svc 默认 yaml BaseURL 空 → 推断"aiclient 实现空"
- 看到 todo-pile 自查时 dev 端口不通 → 推断"前端入口已砍"

**没做的事**（5 分钟就能戳破）：
```bash
ls emotion-echo-ai-svc/internal/aiclient/          # → 11 个文件，含 fer/sensevoice/xtts
grep -n "emotion-echo-xtts" deploy/docker-compose.apps.yml  # → 12 处命中
wc -l emotion-echo-ai-svc/internal/aiclient/xtts.go         # → 151
```

**对策**：决策 18 §4.1"结论须附可复现证据"在 todo-pile 起草时未执行——本 Stage 的修复正是补
这个证据链。**给后续 plan 作者的硬要求**：任何 plan 涉及"X 是 Y"的断言，参考本 Stage §四的
实测矩阵格式（断言 / 事实 / 证据 三列）。

### 5.2 决策栈 9/11/12 内部语义未收口

- 决策 9（2026-08-31）：web-bff 是"统一入口"
- 决策 11（2026-09-03）：APISIX 是"网关层"
- 决策 12（2026-09-03）：BFF"宿主机不再直接映射"

三段话单独看都对，但**放在一起**给读者造成困惑：
- "web-bff 是唯一前端入口"（决策 9 / 全景图 / 端口表）vs
- "APISIX 是唯一业务入口"（决策 11）vs
- "BFF 宿主机不再直接映射"（决策 12）vs
- `apps.yml:602-604` 注释"prod 应仅暴露 APISIX，dev 保留 8894"

五处措辞在**视角维度**上分叉：业务入口 / 系统入口 / dev 例外。本 Stage 在决策 9/12
末尾各加"关系说明"段，并改写全景图 + 端口表，把视角维度显式化。

### 5.3 决策 18 §4.1 证据链格式缺少模板

todo-pile 起草时按 §4.1 要求"附可复现命令"是空话——给读者一个空白处让人填，多半没人填。
本 Stage §四的实测结果矩阵（断言 / 事实 / 证据 / 状态 / 修复 五列）可作为后续 plan / stage
文档的标准证据链格式。

---

## 六、未做事项（不在本 Stage 范围）

| 类别 | 条目 | 处理 |
|---|---|---|
| Sprint 1 TDD | A2 前端路径修 + A3 起前端说明 + C5/C6/C8 实质修复 | 见 `todo-pile-2026-09-04.md` §G 排期 |
| Sprint 2 用户拍板 | A1/B4 重建多模态 / A2 对象存储选型 / B1 prod profile | 同上 |
| Sprint 3 跟踪 | B2 Nacos SDK 升级 + C7 Stage 36 dashboard 回查 + D1-D6 杂项 | 同上 |
| Sprint 4 CI 加固 | §0.2 / 决策 18 §4.1 PR 模板复选框 + `scripts/check_route_contract.sh` + `scripts/check_empty_db_repro.sh` | 新计划，留待独立 stage |

---

## 七、调研依据

### 7.1 读过的文件（12 个）

```
emotion-echo-web-bff/main.go                                    # 行 230-240 registerRoutes 散装
emotion-echo-web-bff/internal/handler/tts_handler.go             # 行 35-36 路由注册
emotion-echo-web-bff/internal/handler/upload_handler.go          # 行 26-30 handler 写死 502
emotion-echo-web-bff/internal/handler/auth_handler.go            # 行 5-10 5 个 auth 端点
emotion-echo-ai-svc/internal/aiclient/xtts.go                    # 151 行 + 头注释行 15-21
emotion-echo-ai-svc/internal/aiclient/fer.go                     # 130 行 + 头注释行 14-21
emotion-echo-ai-svc/internal/aiclient/sensevoice.go              # 118 行
emotion-echo-ai-svc/etc/ai-api.yaml                              # 行 69-81 BaseURL 留空 + 注释
deploy/docker-compose.apps.yml                                   # 行 387-389 BASE_URL env + 行 437/473/518 服务定义 + 行 574-604 BFF
QUICKSTART.md                                                    # 行 27 BFF 描述 + 行 43 端口表
README.md                                                        # 行 176 "唯一入口" + 行 181 "APISIX 退役"
docs/architecture/decisions.md                                   # 决策 9/11/12/18 + 架构全景图 + 端口表 + 治理层段
```

### 7.2 查过的 ADR / Stage 文档

```
docs/architecture/decisions.md（决策 9 / 10 / 11 / 12 / 18 全文）
docs/architecture/adr/adr-2026-09-doc-drift-registry.md（决策 18 §二 11 条 + §三 4 类）
docs/architecture/adr/adr-2026-09-nacos-reintroduction.md（决策 10 配置范围）
docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md（决策 17）
docs/stages/stage-38-system-status.md（2026-09-04 复核段）
docs/stages/stage-39-nacos-enablement.md（2026-09-04 复核段 + §五删除记录）
docs/plans/todo-pile-2026-09-04.md（修复对象）
```

### 7.3 跑过的现状

本 Stage 是纯文档修复，未跑 smoke；§四的"实测事实"全部来自代码静态阅读 +
行号实测（`grep -n` / `wc -l` / `ls`）。已在 §五.1 列出戳破 B4 的 3 条命令。

---

## 八、修复统计

| 维度 | 数量 |
|---|---|
| 涉及文件 | 6 个（todo-pile + 决策 18 + decisions + QUICKSTART + README + 本文档） |
| 失真登记（决策 18 §二） | +2 条（实例 #12、#13） |
| 就地更正块 | 5 处（todo-pile A1/B4/A2/B1/C5/C8 各 1 处） |
| 决策栈关系说明 | 2 处（决策 9、决策 12 末尾） |
| 架构全景图 / 端口表 / 治理层段 | 3 处（decisions.md 内） |
| 行号校正 | 2 处（A2 main.go 232→236；useFileUpload.ts 42-46→39-48） |
| 措辞更新 | 4 处（QUICKSTART 第 27/43 行；README 第 176/181 行；decisions.md 端口表 web-bff/FER 行） |
| 新增决策条目 | 0（仅修订现有决策的"关系说明"段） |
| 新增 stage 文档 | 1（本文件） |

---

> 本次 Stage 完成时间：2026-09-04
> 预计 PR 数：1（合并 commit，覆盖 5 文件改动）
> 收口条件：todo-pile-2026-09-04.md / decisions.md / 决策 18 / QUICKSTART / README 五处就地更正完成 +
> 决策 18 §二 实例 #12、#13 登记完成 + 本文件落地

---

## 九、给后续 Stage 作者的"决策 18 §4.1 证据链模板"

本 Stage §四实测结果矩阵沉淀的可复用模板：

```markdown
| # | 条目 | 文档声明 | 实测事实 | 状态 | 修复 |
|---|---|---|---|---|---|
```

五列含义：
- **条目**：本计划/本 Stage 涉及的某条断言
- **文档声明**：原始文档写的内容
- **实测事实**：本作者用 5 分钟 grep/读/wc 实测的代码现状（必须给路径:行号）
- **状态**：[✓ 完全成立] / [✗ 完全失真] / [△ 部分失真 / 表述精度] / [? 不可证伪]
- **修复**：就地更正块的 commit message 摘要或下一段 anchor

**模板使用硬要求**：
- "实测事实"列每条至少一条 `路径:行号` 证据（决策 18 §4.1）
- "状态"列 ✗ / △ 项必须有"修复"列
- 不可证伪项（?）必须在"修复"列说明为何无法静态验证 + 是否需要运行时 smoke
