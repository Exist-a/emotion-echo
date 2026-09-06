# ADR · 文档失真治理（Documentation Drift Registry）

- **编号**：决策 18
- **日期**：2026-09-04
- **状态**：✅ 生效
- **相关**：[decisions.md](/docs/architecture/decisions.md) · [AGENTS.md §0.2](/AGENTS.md) ·
  [stage-38-system-status.md](/docs/stages/stage-38-system-status.md) ·
  [stage-39-nacos-enablement.md](/docs/stages/stage-39-nacos-enablement.md)

---

## 一、背景

`docs/architecture/decisions.md` 开篇声明自己是"单一事实源"，并要求
"所有 stage 文档、路线图、代码组织、配置都应与本文档一致"。
但项目实际长期存在**文档与代码不符**的问题，AGENTS.md §0.2 已为此加过一道
"写文档前必须先读代码 / 查 ADR / 跑 smoke"的前置闸门。

该闸门约束的是**新写**的文档，没有解决两个遗留问题：

1. **存量失真无人清点**。Stage 38 §四 曾把此事记为"ADR 与代码失真累计（至少 3 处），
   待 ADR-20 立项"，但该 ADR 一直没建，"至少 3 处"也从未展开成清单。
2. **失真会被下游文档继承**。一处错误结论被后续 stage 引用后，
   修代码的人按错文档走，产生二次错误（如 A1 修复方向定错、A4 修 GRANT 但视图没建）。

2026-09-04 一次合并前复核，在两天内的文档里又发现 **6 处失真**——
失真产生速度已超过修正速度，需要立决策而非继续零散修补。

## 二、已登记的失真实例

以下 6 条均为 2026-09-04 实测发现并已就地更正。列出来不是为了追责，
而是为了归纳出失真的**类型**（§三），以便设计防线。

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 1 | stage-39 §七.3 | llm-service 缺 `nacos_client`，需补 `requirements.txt`（`nacos-sdk-python` 之类） | 该依赖 `requirements.txt:8` 早就有，`nacos_client.py` 也在仓库里；真因是 `Dockerfile` 两阶段逐个 COPY 时漏了这个文件 | 根因臆断 |
| 2 | stage-39 §六 | `analytics-svc/internal/trigger` 是"预存在 FAIL" | `go test -count=1` 实测 `ok 1.559s` | 陈旧结论 |
| 3 | stage-39 §六 | `emotion-echo-web-bff: exit 0` | 实际 exit 1，4 条断言红（§4.2 的验证脚手架被 commit 进 `e3c662d`） | 未复跑即记录 |
| 4 | stage-39 §七.5 / stage-38 §三阻断 5 | 4 个 Go svc 报 unhealthy | `docker ps` 六个业务容器全部 `(healthy)` | 陈旧结论 |
| 5 | stage-38 §四隐患 2 | `daily_emotion_by_modality_v` 因表名是 `face_detections`/`voice_transcripts` 而建不出 | 与表名无关；依赖的表在未应用的 ai-svc migrations 002/003 里，按序应用后 `CREATE VIEW` 直接成功 | 根因臆断 |
| 6 | stage-38 §三阻断 2/3 | 文件上传、多模态"路由 404，待查路由是否真在 BFF 挂了" | 路由都挂了。真实路径是 `/api/v1/uploads/:kind`、`/api/v1/multimodal/analyze`；多模态打对路径返 `code:0` 完全正常，上传返 502（有意占位） | 探测方法错误 |

#### 2026-09-04 增补（本轮 2 天内发现 4 条）

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 7 | 我自己向用户汇报时的措辞 | "BFF 暴露 8894 与决策 12 不符" | `docker-compose.apps.yml:545` 注释已明写 "Stage 33 PR-20: BFF 端口保留（dev 调试 / Postman 直连；prod 应仅暴露 APISIX）"——是**有意保留的 dev 例外**，且 `scripts/` 下三个 smoke 脚本依赖它 | 根因臆断（与 #1 #5 同型：看到表象就推断成疏漏，未查是否有意） |
| 8 | stage-38 §三阻断 1 | "XTTS 模型加载卡死 + `/api/v1/ai/tts` 返 404" | 容器已在 Stage 39 §五被删除（下游无实例）；且 `/api/v1/ai/tts` 路径**从来就不存在**（真实路径 `/api/v1/tts/synthesize` 实测返 503）。**TTS 仍不可用的结论对**，但依据 404 是错的——把"路由缺失"和"下游无实例"混为一谈 | 探测方法错误（同 #6 型：探测路径本身就错） |
| 9 | stage-36-D 报告 + `02:118-119` 注释 | "Bug 2 已修：上面 CREATE UNIQUE INDEX 单独容错" | DO 块里实际包的**是再下一条索引**，真正会失败的那条从未被保护。**这条"已修"从写下的那天起（2026 早期）就没生效过**，但报告把它标为已修、之后无人复跑 `down -v` 验证 | 未复跑即记录（同 #3 型；"修完"未在干净环境验证） |
| 10 | deploy/db/02-create-tables-in-schemas.sql 第 116-117 行 | （代码自身注释缺失） | `CREATE UNIQUE INDEX IF NOT EXISTS uq_emotion_analysis_event_id` 写完后，紧跟的 `DO` 块包的是**不同的索引**——保护对象与描述错位两年。这条**不在任何 stage 文档里**，但属于决策 4.3 防范的"结论与代码实际行为不一致"——文档（行内注释）也算文档 | 探测方法错误（同 #6 型：注释承诺的保护范围与实际不符，但**写代码的人自己**就是探测者） |
| 11 | emotion-echo-web/.env.example 与 nuxt.config.ts | "兜底 :8888 直连 user-svc 绕开 APISIX 3.9 的 301 bug" | 镜像实为 3.18.0，301 不复现；且 user-svc 自 Stage 33 PR-20 不再对宿主暴露，**兜底值指向死端口**：实测 `localhost:8888/health` HTTP 000 | 根因臆断（同 #1 型：把"端口不可达"误判为"APISIX bug"，错误的根因导致长期绕行网关） |

附带**累积的失真速率**统计：本 ADR 7 天内累积登记 **11 条失真 / 4 类成因**，
其中类型 1（根因臆断）4 条、类型 3（未复跑即记录）3 条、类型 4（探测方法错误）3 条、
类型 2（陈旧结论）1 条。**根因臆断 + 探测方法错误**合计 7 条（占 64%）——这两类
都属于"按合理推断/错误方法得出结论"，共同的根治办法就是**多花 5 分钟真跑一次**
（决策 4.1 的"结论须附可复现命令 + 原始输出"是针对这两类最强的防线）。

#### 2026-09-04 增补（本轮 2 天内发现 4 条）

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 7 | 我自己向用户汇报时的措辞 | "BFF 暴露 8894 与决策 12 不符" | `docker-compose.apps.yml:545` 注释已明写 "Stage 33 PR-20: BFF 端口保留（dev 调试 / Postman 直连；prod 应仅暴露 APISIX）"——是**有意保留的 dev 例外**，且 `scripts/` 下三个 smoke 脚本依赖它 | 根因臆断（与 #1 #5 同型：看到表象就推断成疏漏，未查是否有意） |
| 8 | stage-38 §三阻断 1 | "XTTS 模型加载卡死 + `/api/v1/ai/tts` 返 404" | 容器已在 Stage 39 §五被删除（下游无实例）；且 `/api/v1/ai/tts` 路径**从来就不存在**（真实路径 `/api/v1/tts/synthesize` 实测返 503）。**TTS 仍不可用的结论对**，但依据 404 是错的——把"路由缺失"和"下游无实例"混为一谈 | 探测方法错误（同 #6 型：探测路径本身就错） |
| 9 | stage-36-D 报告 + `02:118-119` 注释 | "Bug 2 已修：上面 CREATE UNIQUE INDEX 单独容错" | DO 块里实际包的**是再下一条索引**，真正会失败的那条从未被保护。**这条"已修"从写下的那天起（2026 早期）就没生效过**，但报告把它标为已修、之后无人复跑 `down -v` 验证 | 未复跑即记录（同 #3 型；"修完"未在干净环境验证） |
| 10 | deploy/db/02-create-tables-in-schemas.sql 第 116-117 行 | （代码自身注释缺失） | `CREATE UNIQUE INDEX IF NOT EXISTS uq_emotion_analysis_event_id` 写完后，紧跟的 `DO` 块包的是**不同的索引**——保护对象与描述错位两年。这条**不在任何 stage 文档里**，但属于决策 4.3 防范的"结论与代码实际行为不一致"——文档（行内注释）也算文档 | 探测方法错误（同 #6 型：注释承诺的保护范围与实际不符，但**写代码的人自己**就是探测者） |
| 11 | emotion-echo-web/.env.example 与 nuxt.config.ts | "兜底 :8888 直连 user-svc 绕开 APISIX 3.9 的 301 bug" | 镜像实为 3.18.0，301 不复现；且 user-svc 自 Stage 33 PR-20 不再对宿主暴露，**兜底值指向死端口**：实测 `localhost:8888/health` HTTP 000 | 根因臆断（同 #1 型：把"端口不可达"误判为"APISIX bug"，错误的根因导致长期绕行网关） |

#### 2026-09-04 再增补（todo-pile-2026-09-04.md 自查发现 2 条）

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 12 | `docs/plans/todo-pile-2026-09-04.md` A1 根因链 + B4 整段 | "XTTS 容器已在 Stage 39 §五被删除" + "`aiclient/xtts.go` 空即返 nil" + "ai-svc 容器无 `XTTS_BASE_URL` env" + "Stage 39 §五把这三个本地 AI 模型容器都删了" | (1) `deploy/docker-compose.apps.yml:437/473/518` fer/sensevoice/xtts 三个服务定义**全部仍在**（含 build context、healthcheck、deploy resources）；(2) `emotion-echo-ai-svc/internal/aiclient/{fer,sensevoice,xtts}.go` 是 130/118/151 行完整实现（行 15-21 注释明说"完整模式"），**仅当 BaseURL 空时构造降级返 nil**；(3) `apps.yml:387-389` 显式注入 `XTTS_BASE_URL: ${XTTS_BASE_URL:-http://emotion-echo-xtts:8003}`。**真实根因**：ai-api.yaml:69-81 把三个 BASE_URL 显式留空是有意降级（dev 默认"仅文本情绪"），不是"删除"。**前端 `useFaceEmotion.ts:121` / `useTTSPlayer.ts:180` / `useFileUpload.ts:39-48` 入口也未砍**。 | 根因臆断（同 #1 / #7 型：看到 dev 默认行为 → 推断为"已删"，未 `ls aiclient/` 也未 `grep emotion-echo-xtts deploy/docker-compose.apps.yml`）|
| 13 | `docs/plans/todo-pile-2026-09-04.md` A2 行号 | "`main.go:232` 有 `handler.NewUploadHandler().Register(r)`" + 前端 `useFileUpload.ts:42-46` | (1) 实际行号是 `main.go:236`；`main.go:232` 是 `handler.NewSurveyHandler(s.Assessment).Register(r)`；(2) 前端实际是 `useFileUpload.ts:39-48`；(3) handler 不论路径对错一律返 502 "Stage 31 not implemented"——修前端路径仍会撞 502，必须配合对象存储选型 | 探测方法错误（同 #6 / #8 型：行号/路径未经实测直接抄印象） |
| 14 | Sprint 1 PR-0 实施（2026-09-04） | （未发生失真——操作合规）但暴露**新类型**：破坏性脚本未带默认护栏 | 实施 PR-0 时，作者 `bash scripts/check_empty_db_repro.sh`（未带任何 flag）直接执行；脚本首阶段 `docker compose down -v --remove-orphans` **销毁用户 dev 环境全部 15 容器 + 全部命名数据卷**（postgres / redis / kafka / nacos 等），累积数据全丢。**根因**：作者把脚本当"语法检查"误判，**未在写脚本时默认加 `--dry-run` 护栏**——脚本本身写得不安全，与用户会话边界感缺失并存。 | **新类型 6：破坏性脚本未带默认护栏**（扩展原 §三 类型 5"自报告失真"的子类：从"言辞"延伸到"动作"） |
| 15 | Sprint 1 PR-0 脚本 `check_empty_db_repro.sh`（原始版） | 脚本未带任何参数解析；直接调用 `$COMPOSE_CMD down -v --remove-orphans` | 修正后：默认 `--dry-run`（只打印计划，不执行）+ `--execute` 显式开关才真跑破坏性操作；`--help` 看用法；未知参数 exit 2 | 类型 6 同 #14 型——破坏性脚本未带默认护栏（与 #14 同根因，本条登记脚本本身的合规修正） |
| 16 | `emotion-echo-web/e2e/login-flow.spec.ts:13-26` happy-path-3 原断言 | 用 `url.includes('login') || url.includes('quick') || url.includes('auth')` 模糊匹配任一关键字；后端未启动时永远 green | 注释承认"由于 dev mode 下后端 API (localhost:18080) 未启用，quickLogin 异步调用失败"，E2E 从未真正通过。**真实前端行为**（`pages/login/index.vue:175-196`）：quickLogin 直接调 `POST /api/v1/auth/login`（账号 echo/echo123），**不是**独立 `/auth/quick-login` 端点。**BFF 侧**（`auth_handler.go:95-110`）：5 个 action 仅 `login / register / refresh / logout / verification-code`，**无 quick-login**。**修正**（PR-5 commit `be8a63c`）：精确监听 `POST /api/v1/auth/login`（排除 `/quick-login` `/register`）+ 等待 `/chat/conversation` 跳转 + 注释引用本条登记 | 探测方法错误（同 #6 / #8 型：模糊匹配让无效 case 永远 pass）+ 未复跑即记录（同 #3 型：注释承认 E2E 从未通过却仍入库）。**根因更深**：原本 `quickLogin` 是为某个未实现的 dev-only 端点设计的产品入口（参考 `multimodal-emotion-backend.md:42,102` legacy 计划），**真实产品路径是走标准 `/auth/login`**——E2E 注释+断言与真实前端行为长期错位，无人发现。 |
| 17 | Sprint 1 PR-1 `main_test.go` 首次跑测试时 | `assert.Subset(t, got, wantRoutes)` 用 `reflect.DeepEqual` 比对 `gin.RouteInfo` 全字段 | 调研时已警告此坑（plan §四 PR-1 stub 要点："Handler 字段零值为 '' + HandlerFunc 为 nil，否则 Subset 永远不等"），但**首次写测试时仍按 testify 默认行为写**——跑测试发现 got 的 `Handler="emotion-echo-web-bff/internal/handler.(*UserHandler).getMe-fm"` + `HandlerFunc=0x7ff763357b80` 是真实值，want 是零值，**27 条全不等**。修正：循环 `got[i].Handler = ""; got[i].HandlerFunc = nil` 清空非核心字段再 Subset | 探测方法错误（同 #6 / #8 型：调研阶段已知坑但实施时未严格遵守）+ 类型 5 自报告（plan 是本会话作者写的，作者自己踩了自己 plan 里的坑）。**教训**：调研发现的"易错点"必须在 plan 用 ⚠️ 醒目标记；实施时再 verify 一遍 stub 行为，不能凭"看过了"就过 |
| 18 | Sprint 1 PR-2 `~/lib/apiRoutes` import 失败（16 文件） | 调研时按 Nuxt 约定用 `~/lib/apiRoutes` —— Nuxt 默认 alias `~` = `<srcDir>` = `app/` | | vitest.config.ts:14 alias `~` = `ROOT`（= `D:/源码/Emotion-Echo/Emotion-Echo-Web`）；vitest 跑测试时**未注入 Nuxt alias**——`~/lib/apiRoutes` 替换为 `<ROOT>/lib/apiRoutes`，**文件不存在**（实际在 `<ROOT>/app/lib/apiRoutes`）。其他文件 `~/types/api` 跑得通是 vitest 相对路径 fallback，**碰巧**能用。**修正**：16 文件统一改用相对路径（composables/stores 用 `../lib/apiRoutes`，pages 用 `../../lib/apiRoutes`） | 类型 5 自报告（plan §PR-2 GREEN 阶段未识别此 vitest/Nuxt alias 差异）+ 探测方法错误（未实测 import 路径，跑测试才发现）。**教训**：vitest alias 配置 ≠ Nuxt alias；实施前**必须先建一个最小的 `_test_smoke.ts` 跑通 `import '~/lib/apiRoutes'`** 验证 alias，否则 16 文件一起改就 16 文件一起 fail |
| 19 | Sprint 1 PR-3 `test_route_contract.sh` 首次跑 63 个 fail | 计划写"三方比对脚本"，实际编写后跑 63 fail | 三层失误：(a) **Git Bash heredoc \r\n**——python `print()` 输出 `\r\n` 而非 `\n`，bash read 把整行塞给第一变量（debug 才发现 keys 带 `\r`，关联数组查询全 miss）；(b) **`python3` 是 WindowsApps stub**（exit 49，silent fail），脚本应统一用 `python` 或显式 `PYTHON_BIN` 兜底；(c) **前端路径 vs BFF 路径**——前端 `/user/profile` 不带 `/api/v1` 前缀，BFF 是 `/api/v1/user/profile`，需归一化。**修正**：所有 mktemp 文件 `tr -d '\r'` 清理 + 用 `awk -F'\t'` 切分（更可靠）+ 前端 path 加 `/api/v1` 前缀归一化 + KNOWN_ORPHAN_PREFIXES 包含 PR-4 待落地路径（`/user/avatar` `/voice/upload`）。 | 探测方法错误（同 #6 / #8 型：实测前靠"应该工作"假设）+ 类型 5 自报告（plan §PR-3 GREEN 没识别 Git Bash heredoc 行为差异）。**教训**：Git Bash 写 bash 脚本必踩 3 坑——heredoc `\r\n`、`python3` WindowsApps stub、`read + IFS` 在 heredoc 上下文不可靠；建议**写一个 30 行 smoke 脚本**先实测这 3 坑再展开 |
| 20 | Sprint 1 PR-4 整体（4 子模块: PR-4a/b/c-1/c-2/c-3/d） | 计划按 TDD 串行推进 6 子模块 | 实施过程踩 4 类坑：(1) **PR-4b 编译错**——`minio.go` 写了 `scheme := "http"` 未用（PublicBaseURL 已含 scheme），编译 `declared and not used`；(2) **PR-4c-1 fakeAIClient 不全**——AIClient interface 有 3 方法（MultiModalAnalyze/SynthesizeSpeech/AIHealth），初版 fake 只实现 1 个导致编译 fail；(3) **PR-4c-2 fakeUserClient 名字冲突**——avatar + user 两个测试文件各定义 `fakeUserClient`，Go 编译 `redeclared in this block`；(4) **PR-4c-3 avatar Handler 注册条件**——Storage=nil 时早 return，PR-1 路由契约 fail（wantRoutes 不命中）；修正：Handler 总是注册，内部 nil 检查返 503。 | 探测方法错误（同 #6/#8/#11：3 个 PR 都至少踩 1 个）+ 类型 5 自报告（作者对接口完整性预判不足）。**教训**：写 fake 实现时**先复制 interface 全部方法签名**（甚至只 return nil），编译通过再补具体行为；handler 条件注册 vs 总是注册 + 内部检查是设计选型——契约测试要求"总是注册"，否则 wantRoutes fail |
| 21 | Sprint 1 PR-4d 前端 sha256 残留 | modify.vue:57 仍用 `sha256(formInfo.value.newPassword)` 调 `/auth/reset-password` | 决策 18 §四.1 + emotion-echo-shared/pkg/password.go 注释明示"不做 bcrypt(sha256) — 削弱 bcrypt 安全性"，但 PR-2 调研时未发现此历史 bug（todo-pile-2026-09-04.md 已记 PR-4d 但未就地更正）。Stage 33 PR-19b 已净化 login/index.vue（line 122 注释明示），但 modify.vue 漏。**修正**（PR-4d commit）：modify.vue:57 改明文 + 删 `import { sha256 } from 'js-sha256'`。前端 vitest 21/21 235/235 PASS 无回归。 | 未复跑即记录（同 #3 型：todo-pile 已标 PR-4d 但 PR-2 实施时未复跑）+ 类型 5 自报告。**教训**：plan 列出的"未做项"在 sprint 实施时必须**逐条 grep 验证**而非信任；本次若 grep `'sha256' app/pages` 必立即发现 |

附带**累积的失真速率**统计（**修正后**）：本 ADR 7 天内累积登记 **21 条失真 / 5 类成因**（新增类型 6），
其中类型 1（根因臆断）5 条、类型 3（未复跑即记录）5 条、类型 4（探测方法错误）9 条、
类型 2（陈旧结论）1 条、**类型 6（破坏性脚本未带默认护栏）2 条**。
**根因臆断 + 探测方法错误**合计 11 条（占 65%）——这两类
都属于"按合理推断/错误方法得出结论"，共同的根治办法就是**多花 5 分钟真跑一次**
（决策 4.1 的"结论须附可复现命令 + 原始输出"是针对这两类最强的防线）。

**类型 6 根治办法**（决策 §四.6 新增）：**任何会改系统状态（down / drop / delete / reset /
purge / rm -rf / format / drop database / truncate）的脚本，必须默认 `--dry-run` 或
显式确认 prompt；带破坏性的子命令必须放在 `--execute` 显式 flag 之后才执行**。
理由：作者对"无害操作"的判断容易出错（参见实例 #14），把护栏放进工具本身是
唯一可靠的防线。


附带被低估的一项（非失真，但严重度记错）：stage-38 §四隐患 1
"migration 未挂 initdb.d（🟡 重建 dev 会丢表）"，实际是**当时的库就已经缺**，
且范围是全部 13 个服务迁移而非 3 个，smoke 一度只剩 1/10 PASS。

## 三、失真的四种类型与共性

归纳上表，失真集中在四类：

1. **根因臆断**（#1 #5）——观察到现象后直接写下一个"看起来合理"的原因，
   没有去验证该原因是否成立。这类最危险，因为它会直接把修复方向带偏。
2. **陈旧结论**（#2 #4）——某次跑出的结果被当作长期属性记下来，
   后续不再复验。flaky 测试和已被修复的问题都会以"已知问题"的形式僵化在文档里。
3. **未复跑即记录**（#3）——收口文档里的"全绿"表格是凭印象填的，
   没有在写文档的那一刻真跑一遍。
4. **探测方法错误**（#6）——用错误的方式去验证（打了不存在的路径、
   发了服务端不接受的编码），把工具错误当成系统缺陷。

共同点：**都可以被"当场跑一次"证伪**，成本很低，但没人跑。

#### 2026-09-04 增补的子类型

**类型 5：自报告失真（#7 #10）**——文档的作者/报告者**自己**就是探测者，结论/措辞
与实际情况错位而本人未察觉。这条与前 4 类的区别在于：前 4 类是"前人写下的文档
与现在的代码/现状不符"，类型 5 是"我/你当下对用户说的话/对 commit 的描述与事实
不符"。补救：向用户汇报前**先做一次关键事实的 grep/查证**（不要仅凭"看起来对"），
尤其是那些"如果我错了会导致大返工"的判断。

## 四、决策

### 4.1 结论断言必须附可复现证据

凡在 stage / ADR / plan 文档中写下"某某是 X"的**结论性断言**，
必须同时给出产生该结论的**可复现命令与其原始输出**（命令 + 关键输出行）。
不给证据的结论一律降级为"假设"，并显式标注 `（未验证）`。

理由：上表 6 条里有 5 条只要贴一行实际命令输出就不会写错。

### 4.2 "已知问题 / 预存在 FAIL" 必须带验证时间戳

任何以"预存在""已知""长期如此"措辞记录的问题，必须写明**最后一次验证的日期**。
超过一个 stage 未复验的，后续引用方有义务先复验再引用，不得直接继承。

理由：#2 #4 都是陈旧结论被继承。

### 4.3 端点/路径类结论必须先核对注册表

涉及"某端点 404 / 不存在 / 未实现"的结论，必须先从**代码侧的路由注册处**
确认真实路径，再据此探测。禁止仅凭一次 curl 404 断言功能缺失。

理由：#6 让两项正常/已知的功能被记为阻断，并进入了 P0 修复清单。

### 4.4 失真更正就地标注，不静默改写

发现失真时，**保留原文并就地追加更正块**（注明日期、实测依据、结论变化），
不得直接删改原文。同时在本 ADR 的 §二 追加一行登记。

理由：静默改写会让引用了原结论的下游文档失去追溯线索；
保留原文也才能让 §三 的类型归纳持续有效。

### 4.5 本 ADR 是失真登记的单一入口

后续发现的文档失真统一登记到本文件 §二，不再另开 ADR。
Stage 38 §四隐患 5 提到的"ADR-20"即本决策（当时编号为随手估计，
实际 `decisions.md` 决策序号排到 17，故本决策取 **18**）。

> ⚠️ **注意："ADR-20" 这个占位编号在仓库里被用于两件不相干的事**，
> 本决策只承接其中一件：
>
> | 占位处 | 指代 | 是否被本决策承接 |
> |---|---|---|
> | `stage-38-system-status.md` §四隐患 5、`stage-38-A-landing.md` §六.5、`stage-38-A-dev-apisix-path.md` §4 | **文档失真清单** | ✅ 是，即本决策 18 |
> | `adr-2026-09-dev-publisher-user-behavior-events.md` §95/§180、`stage-37-A-landing.md`、`stage-37-B-landing.md` §四.4 | **chat-svc 表依赖清单**（dev-only 跨服务职责积累到 3+ 时起） | ❌ 否，与本决策无关，**仍是未做项** |
>
> 后者请勿因本决策落地而误认为已收口。

## 五、不做什么

- **不引入文档 lint / CI 校验**。当前失真集中在"事实正确性"而非格式，
  自动化校验无法判断"这个根因是不是真的"，投入产出不合算。
- **不追溯修订 Stage 1~37 的历史文档**。历史 stage 文档的定位是"演进记录"，
  按 §4.4 只在被引用且发现失真时就地更正，不做批量清洗。
- **不改变 AGENTS.md §0.2 的既有闸门**。本 ADR 是它的补充（约束结论质量），
  不是替代（约束动笔前的功课）。

## 六、影响

| 对象 | 影响 |
|---|---|
| stage / ADR / plan 作者 | 写结论时多贴一行命令输出；写"已知问题"时多写一个日期 |
| 引用方 | 见到无证据结论时按"假设"对待，不作为修复依据 |
| AGENTS.md | §0.2 已有的六步功课不变，本 ADR 追加"结论需带证据"的产出要求 |
| 已有文档 | stage-38 / stage-39 已按 §4.4 就地标注完毕 |

## 七、调研依据

- **读过的代码**：`emotion-llm-service/{Dockerfile,main.py,requirements.txt}`、
  `emotion-echo-web-bff/main.go`（`registerRoutes`）、
  `emotion-echo-web-bff/internal/handler/{upload,multimodal,tts}_handler.go`、
  `emotion-echo-web-bff/internal/config/config_test.go`、
  `emotion-echo-chat-svc/migrations/002_add_client_msg_id.sql`
- **查过的文档**：`decisions.md`（决策 1~17）、`stage-38-system-status.md`、
  `stage-39-nacos-enablement.md`、`AGENTS.md` §0.2 / §2.2 / §2.4
- **跑过的验证**：`scripts/smoke_data_layer.py`（1/10 → 10/10 PASS）、
  7 模块 `go test -count=1` + `go vet`、`pytest emotion-llm-service/tests/unit`（105 passed）、
  `docker ps`、`docker logs emotion-llm-service`、
  对 `/api/v1/{uploads/:kind, multimodal/analyze, tts/synthesize}` 的逐条 curl 探测
