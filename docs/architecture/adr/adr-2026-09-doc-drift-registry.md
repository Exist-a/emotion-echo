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
| 22 | Sprint 1 PR-4c-3 commit msg（928bed2 "实测"小节） | "emotion-echo-user-svc 7 包全 PASS（含 6 个新 reset-password 单测）" + "emotion-echo-web-bff 10 包全 PASS（含 4 个新 reset-password handler 测试）" + "scripts/test_route_contract.sh 0 fail / 27 warn" | (a) **镜像未 rebuild**：`emotion-echo/user-svc:v0.1.0` / `emotion-echo/web-bff:v0.1.0` 是 PR-4c-3 commit 前的旧镜像，跑端到端时 reset-password 路由根本不存在（带 `X-User-Id: 1` 也返 404；不带 token 时 Gin 全局中间件先 abort 返 401 "missing or invalid X-User-Id"——与"中间件拦"假象一致）。(b) **单测只覆盖 logic 层**：`authlogic_test.go TestAuthLogic_ResetPassword_*` 直接调 `l.ResetPassword()`，没经过 handler binding / GinAuthMiddleware，**单测绿 ≠ HTTP 路径通**。(c) **APISIX 白名单漏 `/api/v1/auth/reset-password`**：seed.sh id 110-114 只覆盖 login/register/verification-code/refresh/logout，reset-password 没白名单 → 走 `/api/v1/*` catch-all 被 jwt-auth 401。(d) **BFF 验证码黑洞**：commit msg "未做"小节承诺"前端 console 显示"——但前端代码从未实现 console 输出，dev 模式 e2e 卡在第二步。**修正**（PR-4c-4）：user-svc `internal/handler/auth_handler_test.go` 加 HTTP 端到端测试（复刻 main.go 路由结构 + GinAuthMiddleware 全局注册）+ 重建 user-svc + BFF 镜像 + seed.sh id=115 `/api/v1/auth/reset-password` 白名单 + BFF `verificationCode` 响应 `devCode` 字段（`BFF_DEV_RETURN_CODE=1` 控制，prod 永远关）。**端到端**：注册→拿 devCode→reset-password→DB 验证登录 4 步全绿。修复过程与复现命令详见 [todo-pile § H](/docs/plans/todo-pile-2026-09-04.md)。 | 探测方法错误（同 #6 / #8 / #11 型：单测绿即宣告"实测通过"，未跑过 docker 端到端）+ 未复跑即记录（同 #3 / #21 型：commit msg 自称 Sprint 1 收口，未真起 docker compose 验证）。**教训**：单测（特别是 logic-only 单测，mock 掉 HTTP binding 与 middleware）不能作为"端到端通"的证据；commit msg "实测"小节**必须列可复现命令 + 原始输出**，否则一律视为"自报告"，AGENTS.md §〇 写文档前调研的纪律必须延伸到 commit msg 自验证 | 

#### 2026-09-10 增补（本次会话 #23 #24）

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 23 | `docs/architecture/adr/adr-2026-09-env-profile-strategy.md` 头 4 行（ADR-20）+ `decisions.md` 决策 20 区块 + `README.md` Status 段（停留在 Stage 36/35） | 头 4 行 "🟡 Proposed / 待决策" + "⏸ 未开始（决策落地前不做任何代码/compose 改动）" + README 徽章 `Stage-35--Hardening` + Status 段只到 Stage 36 | (1) `deploy/compose.dev.yml` 61 行（PR-ENV-1 `ac0299e` 2026-09-09 06:31）+ `compose.prod.yml` 66 行（PR-ENV-3 `b89ecab` 06:33）+ `configuration.md` 179 行（PR-ENV-4 `b2ea516` 06:35）+ `apps.yml` 中性化 18 处硬编码（PR-ENV-2 `f400e65` 06:32）；测试脚本 4 项共 43/43 PASS。(2) 仓库已 148 commits ahead of origin/main，stage 文档最新 `stage-60-pr-tts-vendor-landing.md`（XTTS vendor / FER-tflite / SV-fastbuild 三模型落地）。**修正**：ADR-20 头 4 行后追加就地更正块 + decisions.md 决策 20 区块追加就地更正块 + README 顶部徽章改 `Stage-60--PR--TTS--VENDOR` + Status 段后追加 16 行阶段进展表。决策 20 的 owner sign-off 仍未到（保留 Proposed 标签） | 未复跑即记录（同 #3 / #9 / #21 型：决策落地 1 天后 ADR 头 4 行仍未更新）+ 陈旧结论（同 #2 / #4 型：README Status 段停留在 Stage 36，git log 已 Stage 60）。**教训**：ADR 头 4 行的"决策状态 / 实施状态"字段必须**随 commit 同步更新**，建议未来用 pre-commit hook 或 PR-CI 校验 ADR 头 4 行 commit hash 是否指向该 ADR 涉及的 commit |
| 24 | `emotion-echo-web/app/composables/useAIStreamHandler.ts:78` + `useTTSPlayer.ts:175` + `useApi.ts:29,31` | （代码 fallback 字面值 + 文档假设"前端经 APISIX"）| (1) `useAIStreamHandler.ts:78` `const streamUrl = '${runtimeConfig.public.API_BASE_URL \|\| 'http://localhost:8894/api/v1'}${...}'`——fallback 字面值 8894 是**Stage 30 时代的"BFF 直连"残留**；(2) `useTTSPlayer.ts:175` 同款 fallback 8894；(3) `useApi.ts:29,31` fallback 写 `localhost:8080/api/v1`——8080 是 Spring 默认端口，本项目无任何服务监听 8080（user-svc:8888 / chat-svc:8890 / ai-svc:8891 / analytics-svc:8893 / assessment-svc:8889 / web-bff:8894 / apisix:19080 / llm-svc:8000）。**实际产品现状**（已实测）：`nuxt.config.ts:19` 默认 + `.env:5` + `.env.example:24` 都指向 `http://localhost:19080/api/v1`（经 APISIX，cf1c798 之后默认），所以**用户在 `.env` 设好 `NUXT_PUBLIC_API_BASE_URL` 时这3 处 fallback 不触发**；但**任何 docker compose 没起 / .env 漏配的场景**，聊天流 + TTS 播放会无声回退到 8894（BFF 已起可走通）或 8080（HTTP 000 死端口）。**失真类型**：决策 9 vs 决策 11/12 的"BFF 是否唯一入口"长期未收口（`decisions.md` 决策 9 至今仍写 "web-bff 是统一入口"，决策 11/12 写 APISIX 是唯一业务入口——字面冲突 4 个月），fallback 字面值是这套语义未收口的**代码侧残留**。**修正路径**（未做）：把 3 处 fallback 字面值统一改为 `''` 或 throw，强制依赖环境变量；同时把 `decisions.md` 决策 9 末尾加"决策 12 已修正此视角"的正式收口（cf. 决策 9 末尾已有的非正式注释）| 根因臆断（同 #1 / #5 / #11 型：写 fallback 时凭"直觉选个 localhost 端口"，未验证端口是否存在）+ 探测方法错误（同 #6 / #8 型：未在 fallback 路径上跑一次端到端，只看 happy path `.env` 设值的情况）+ 类型 5 自报告（本会话作者本次没复跑 fallback，只读 `.env` + `nuxt.config.ts` 就下结论"前端经 APISIX"，未读 3 个 composable 的 fallback 字面值）。**教训**：产品变更（Stage 30 → Stage 32 入口改回 APISIX）必须做"代码侧残留扫描"——`grep -rn "localhost:[0-9]\+" emotion-echo-web/app/` + `grep -rn "BFF 直连\|直连 BFF" docs/`；本条属于"未做残留扫描"的失真<br>🟢 **修复落地追踪**（2026-09-11 复核）：<br>commit `5028e49 feat(web): apiBaseUrl helper + 6 fail-fast tests (PR-A RED → GREEN)` + `3e7267e refactor(web): 3 处 fallback 字面值移除 · 复用 getApiBaseUrl helper (PR-A GREEN)` 已落地，3 处 composable 现都调 `getApiBaseUrl(useRuntimeConfig())` 抛错而非 fallback 死端口。`decisions.md` 决策 9 vs 11/12 字面冲突亦由 commit `9b03874 docs(decisions): 决策 9 vs 11/12 字面冲突正式收口 (PR-C)` 关闭。QUICKSTART.md:43 端口表 BFF 行措辞也已修。本条 #24 关闭。 |

#### 2026-09-10 再增补（Stage 62 PR-3.4 docker 冒烟期间发现 #28 #29 #30）

Stage 62 PR-3.1~3.4 落地后跑 docker 端到端冒烟（`docker compose up -d` 全栈 +
`scripts/grpc_smoke`），gRPC 链路 7/7 PASS，但**顺带实测出 3 个长期存在、
影响"dev 能否正常工作"的独立 bug**。三条都不是本次 gRPC 改动引入，而是历史遗留。

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 28 | `emotion-echo-shared/pkg/discovery/nacos_register.go` Heartbeat（:281）+ Unregister（:191） | b869ff9（PR-1）注释："PR-1 修复：Host 为 0.0.0.0（yaml 默认）会让 Nacos 把实例判 unhealthy；fallback 到本机非 loopback IPv4" | **只修了 Register 一处**。实测时序（重启 user-svc 后每 3s 轮询 Nacos）：T+3s `172.18.0.14:8888`（Register 正确）→ T+6s `0.0.0.0:8888`（Heartbeat 5s tick 覆盖）。全 6 svc 均如此。**后果**：APISIX nacos-discovery 拉到 0.0.0.0 上游 → `connect() failed (111: Connection refused)` → **dev 网关所有请求 502**（`curl :19080/api/v1/auth/login` → 502）。**修复**（commit `3f3a970`）：抽纯函数 `registerHost()`，Register/Unregister/Heartbeat 三处统一调用；新增 3 测试（含源码级契约测试锁住"三处都必须用 helper"）。**验证**：重建 6 svc 后 Nacos 全部真实 IP（172.18.0.12~17），25s 后仍保持；APISIX 登录 200 | 未复跑即记录（同 #3 / #9 / #22 型：b869ff9 只改 Register 一处、未在真实 docker 跑满一个心跳周期（≥5s）就收工） |
| 29 | `deploy/apisix/seed.sh:275` file-logger 插件 | `observability-sprint-b.md §2.2` 要求 seed.sh 主入口路由 plugins 加 file-logger（PR-OBS-1 `89b0117` / PR-OBS-5） | `log_format` 写成 **JSON 字符串**（nginx 风格），但 APISIX 3.18.0（`apache/apisix:3.18.0-debian`）file-logger schema 要求 **object**。实测 apisix-seed 容器 FATAL：`failed to PUT route 100: ... property "log_format" validation failed: wrong type: expected object, got string`。**后果**：**12 条路由一条都没建成**——网关完全空载。**修复**（commit `cb372cd`）：log_format 改 object；seed 日志恢复 `seed complete: 6 upstreams + 12 routes` | 未复跑即记录（同 #3 / #22 / #28 型：PR-OBS 系列文档只写"配置应含哪些插件"，从未写"跑一次确认路由建出来"；APISIX 版本升到 3.18 时 schema 变了也没人复跑） |
| 30 | `deploy/apisix/seed.sh` `put_route_health()`（:446） | `stage-32-apisix-reintroduction` 系列：5 个 health 探针路由"直接打到下游 svc，绕开 BFF 聚合" | 路由只设 `uri` + `upstream_id`，**缺 path rewrite**。请求以 `/user-health` 原样转发下游，而各 svc 的 `gin_auth` 中间件只豁免 `/health` → 全部 401（实测错误串出自 `shared/pkg/middleware/gin_auth.go:31`）。修掉 #29 后 5 个探针全线 401。**修复**（同上 commit `cb372cd`）：`HEALTH_PLUGINS` 加 `"proxy-rewrite": {"uri": "/health"}`；实测 5/5 HTTP 200 | 探测方法错误（同 #6 / #8 / #11 型：路由"设计意图"写在注释里，但**从未有人 curl 过这 5 个端点**验证；#29 掩盖了它——路由压根没建出来时连 401 都看不到） |

**遗留（本条未修，登记待办）**：`/apisix-health`（route 205）无 upstream → 恒返 503。

#### 2026-09-11 增补（Stage 63 收口端到端验证发现 #25 #26 #27）

Stage 63 代码 landed（commit `3d3f0ea`，BFF gRPC 接线启用）后跑 docker 端到端，**单测全绿 ≠ 端到端通**模式再次重演（与 Sprint 1 PR-4c-3 #22 同源）。**3 条新发现全部为 Stage 58/62/63 期间累积的失真**，而非本次引入。

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 25 | `emotion-echo-web-bff/internal/downstream/chat_grpc.go:42,49,217` | 注释"userIDKey 与 HTTP 客户端共享的 ctx key（chat_handler 中间件注入用同一 key）" | `chat_grpc.go:42` 用私有类型 `userIDKey{}`（49 行定义），而 `downstream.WithUserID`（`ai.go:42`）注入的是另一个类型 `userIDCtxKey{}`。**两个 struct{} 类型互不通**→ ctx.Value 永远 miss → metadata x-user-id 永不注入 → chat-svc 拦截器 Unauthenticated。**修复**（本次 commit，集成在 Stage 63 收口 v0.1.2）：chat_grpc.go 三处 `userIDKey{}` → `userIDCtxKey{}`；新加 2 测试 `TestChatGRPCClient_WithUserIDFromDownstream_InjectsMetadata` / `TestChatGRPCClient_NoUserID_NoMetadata` 锁契约；BFF 全包绿 | 注释与代码不符（同 #11 / #24 型：注释写"共享"，实际类型独立）+ 未复跑即记录（#22 / #28 同源） |
| 26 | `emotion-echo-shared/pkg/grpcinterceptor/userid.go:28-30` 注释 | "CtxUserIDKeyType 是 context 中 user id 的 key 类型（与 pkg/middleware.CtxUserIDKey 同义）" + `pkg/middleware/jwt_auth.go:33` 定义 `type CtxUserIDKey struct{}` | grpcinterceptor.CtxUserIDKeyType 与 middleware.CtxUserIDKey 是**两个不同的 struct{} 类型**，注释"同义"是错的。**实际后果**：4 svc gRPC server 的 userid 拦截器（`grpcinterceptor.NewServerUserIDInterceptor`）把 user_id 注入 ctx type=CtxUserIDKeyType{}，但 4 svc 的 logic 文件（user/chat/assessment 共 13 处）读的是 `middleware.CtxUserIDKey{}` → **logic 永远 miss → 业务 401 "missing user id in context"**。**修复路径**（不在本次 PR）：在 shared 顶级新加 `pkg/ctxkey` 包，两边都用 `= type CtxUserIDKey = ctxkey.UserIDKey`，解除循环依赖需先看 shared 包依赖图。本次临时绕路：BFF 默认 Transport="http"（见 #27 同批）端到端恢复 | 类型 5 自报告失真（同 #24 / #25 型：注释自报"同义"，未实测 .(CtxUserIDKeyType{}) 与 .(CtxUserIDKey{}) 是否同 type）<br>🟢 **修复落地追踪**（Sprint C, 2026-09-11）：<br>新建 `emotion-echo-shared/pkg/ctxkey/userid.go` 定义 `type UserID struct{}`；`middleware/jwt_auth.go:32` 改 `type CtxUserIDKey = ctxkey.UserID`；`grpcinterceptor/userid.go:28` 改 `type CtxUserIDKeyType = ctxkey.UserID`。3 个 BFF 跨包等价测试（`TestCtxUserIDKey_TypeAlias` / `_CrossReadWrite` / `_ReverseDirection`）从 RED 转 GREEN。docker 端到端验证：user/assessment/analytics-svc gRPC 路径 200（原 401 "missing user id in context"）。本条 #26 关闭。 |
| 27 | `emotion-echo-chat-svc/internal/grpcserver/chat_server.go:85,89,97,101,113,121` | Stage 58 PR-GRPC-3 commit msg "chat-svc gRPC server 6 RPC 全实现" + `stage-58-q3-followups.md` §一标记完成 | 6 个 RPC（SendMessage / ListMessages / ListConversations / DeleteConversation / PinConversation / StreamMessages）函数体全部 `return nil, status.Error(codes.Unimplemented, "Xxx: PR-GRPC-3 阶段补完")`。**Stage 58 PR-GRPC-3 实质只挂了 server skeleton，未实现任何方法**。**实测**：BFF→chat-svc gRPC 路径全部 Unimplemented。**修复路径**（不在本次 PR）：chat-svc 补 6 RPC 实现（约 1-2 天工作量，可与 grpc-inter-service-migration.md Phase 2 user-svc 合并做）。本次临时绕路：BFF 默认 Transport="http"，HTTP 端 `/api/v1/conversations` 也返 500（chat-svc HTTP 端本身问题，独立修复） | 未复跑即记录（同 #3 / #9 / #22 / #28 型：Stage 58 commit msg 写"全实现"，但从未在 docker 内调过一次该 RPC 验证 Unimplemented 是否被替换）<br>🟢 **修复落地追踪**（Sprint D, 2026-09-11）：<br>chat-svc gRPC 4 RPC 实现（`chat_server.go` SendMessage / ListMessages / ListConversations / DeleteConversation）+ `mapLogicError` 错误码映射 + `toProtoMessage` proto 转换 helper。**PinConversation 保留 Unimplemented 是正确设计**（chat-svc 无 model 字段、无 repo 方法、无 HTTP 端点，加功能需 schema migration + repo 接口扩展，是独立 PR）。**StreamMessages 保留 Unimplemented 是架构判断**（无流式业务，proto 留接口）。4 个 RED 测试 `TestChatServer_*_NotUnimplemented` 转 GREEN。chat-svc v0.1.2 rebuild + docker 端到端 `conversations` 走 gRPC 200。BFF 默认 Transport=grpc 覆盖全部 4 svc，端到端 4/4 业务全 200。`deploy/compose.sprint-c-override.yml` 已删。本条 #27 关闭。 |

**#25 修复落地证明**：本次 commit `TestChatGRPCClient_WithUserIDFromDownstream_InjectsMetadata` 锁契约 + BFF 全包 `go test ./...` 12/12 包绿 + docker 端到端 `users/me`/`surveys`/`reports/daily` HTTP fallback 200。

**#26 #27 共同教训**：单测全绿（Stage 63 当时）+ gRPC 冒烟 7/7 PASS（Stage 62 PR-3.4 当时）≠ 端到端通。原因共性：**测试只到 mock server 层**（bufconn）或**只到 svc gRPC server 层**（直连），**从未穿过 BFF→4 svc gRPC→svc logic 的完整链路**。本次 Stage 63 收口第一次穿过，发现 #25 #26 #27 三层叠错。

**决策 18 §4.6 提议**：未来"gRPC 链路落地"类 PR 的合并门槛必须包含 **BFF→svc gRPC 真实链路端到端测试**（用 bufconn 模拟 BFF + 真实 svc gRPC server + 真实 svc logic），不能只到 svc gRPC server 健康检查为止。
APISIX 能响应即证明网关存活，但状态码语义误导（应 200）。建议后续改为静态 200 响应或 mock upstream。

#### 2026-09-11 增补（Sprint C 期间发现 #31 #32）

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 31 | `emotion-echo-analytics-svc/internal/logic/mentalhealth_trigger_logic_test.go:105` + `internal/trigger/trigger_queue_test.go:57` | （merge commit `680f1dc` "接收 fix/analytics-mentalhealth-trigger-queue-full"）| 两处都残留 git conflict marker (`<<<<<<< HEAD` / `=======` / `>>>>>>>`)：merge 时未走 `git checkout --ours/--theirs` 或编辑器 resolve，导致 `go test ./...` 直接 syntax error。**影响**：analytics-svc 全包测试一直 FAIL，但 Sprint 1 收口报告未跑 analytics-svc（只跑了 shared/bff/user/chat/assessment 5 个）→ 漏过。**修复**（Sprint C 期间顺手清理）：两处冲突标记删除（两段内容完全相同，"workers=0 不启动 worker"那段）；analytics-svc `go test ./...` 恢复全绿 | 未复跑即记录（同 #3 / #9 / #22 型：merge commit msg 写"接收"，但未跑 `go test` 验证）+ 探测方法错误（同 #11 型：Sprint 1 收口 smoke 只跑部分包，未跑 analytics-svc）。**教训**：merge 冲突必须用编辑器或 `git checkout` resolve，**不能**只 merge 标记分支然后丢冲突 marker；CI 必须有 `grep -rE '<<<<<<< HEAD' --include='*.go'` 检查 |
| 32 | `emotion-echo-web-bff/internal/downstream/chat_grpc.go:42,49,217`（同 #25） | Stage 63 收口报告说"代码 landed + 单测全绿" | Sprint C 期间实测（chat-svc gRPC 走 HTTP 仍返 502）：chat-svc HTTP 端 `/api/v1/conversations` 本身也有问题——list conversations HTTP 处理函数也存在错误（与 gRPC Unimplemented 是不同 bug）。**修复路径**：与 Sprint D 一起修。**本条与 #27 是同源（chat-svc conversations 链路全断），但根因不同（一个 gRPC Unimplemented，一个 HTTP 端 handler bug）**——必须分开修，不能合并 | 探测范围不完整（同 #11 / #28 型：原报告只测了 gRPC 路径 fail，未切 HTTP 路径对比）。**教训**："BFF→svc 全路径"测试必须覆盖**两条传输路径**（gRPC + HTTP），不能只测默认 transport<br>🟡 **状态更新**（Sprint D 收口, 2026-09-11）：<br>Sprint D 修好 #27（chat-svc gRPC 4 RPC 实现），BFF 默认 Transport=grpc 覆盖全部 4 svc，conversations 走 gRPC 200。**HTTP 端 /api/v1/conversations 500 bug 本质未修**——Sprint D 不在本批范围。但因 BFF 不再走 HTTP 触发，bug 被规避。后续如有人设 `CHAT_TRANSPORT=http`（debug/回滚场景），bug 仍存在。**真实根因待查**：观察 chat-svc 容器日志 + logic 单测全绿 → 怀疑 gin_auth middleware 在某条件下返 500 而非 401，需后续 sprint 专门排查。本条 #32 仍 open，记后续。<br><br>🟢 **关闭追踪**（PR-3 `3e07571`, 2026-09-11）：<br>chat-svc v0.1.3 rebuild（PR-2 中间件分层生效 + Sprint D chat-svc gRPC 4 RPC 实现），docker 端到端实测 4 路径：<br>- `/health` 无 X-User-Id → 200（K8s probe 不再 401）<br>- `/metrics` 无 X-User-Id → 200 + prometheus format<br>- `/api/v1/conversations` 无 X-User-Id → 401（业务群 auth 兜底）<br>- `/api/v1/conversations?limit=5` + `X-User-Id: 1` → **200 + `{"list":[],"hasMore":false}`**<br>**bug 未被真实触发**：InMemory repo handler 端到端 5 用例（Happy / Pagination / EmptyUser / InvalidLimit / NoUserID）全绿、Postgres 真库实测 200、logic 单测全绿、5 个 chat_handler 集成测试覆盖。**结论**：#32 的 500 现象源自更早版本镜像（v0.1.0 前后）的中间件/handler 时代 bug，已在多次重构（PR-2 中间件分层 + Sprint D chat-svc gRPC 4 RPC 实现 + PR-3 ListConversations 端到端测试）中无意修复。决策 18 #32 关闭。 |
| 33 | `proto/user.proto` Stage 62 PR-3.1 commit + `emotion-echo-web-bff/internal/downstream/user_grpc.go:79-87` 注释 | Stage 62 PR-3.1 commit msg + plan §一.2 "BFF→user-svc gRPC" + user_grpc.go 注释 "user-svc 暂未暴露对应 gRPC 端点" | **user.proto 只定义 3 RPC（GetMe/UpdateProfile/GetUserById），完全没有 Login/Register/ResetPassword/Logout**——而 user-svc 6 个高频方法中只有 3 个能走 gRPC。BFF 注册/登录路径仍是 HTTP，与"决策 4 内部 svc-to-svc = gRPC"不符。**Sprint E（2026-09-11）修复**：扩 user.proto 加 Login/Register RPC + 生成 stub + user-svc gRPC server 实现 + BFF userGRPCClient.Login/Register 接入。accessToken 仍由 BFF jwt.Manager 签发。**ResetPassword/Logout 仍 HTTP**（Sprint F 跟进）。docker 端到端验证：register → login → users/me 全走 gRPC + 200 | 注释与代码不一致（同 #11 / #24 / #25 型：注释说"暂未暴露"，但从未量化"还差多少"，也不知道扩 proto 是解法）+ 探测方法错误（同 #6 / #8 / #28 型：从未写过 proto stub 生成脚本端到端测试，proto 缺哪几个 RPC 都是猜测）。**教训**：扩 gRPC 覆盖度时**必须先 grep `proto/` 看实际 RPC 定义**，而不是看 server 文件或注释就推断覆盖 |
| 34 | `docs/plans/grpc-inter-service-migration.md §一.2/§一.3`（写于 2026-09-07） | 写"BFF → user-svc/chat-svc/assessment-svc/analytics-svc 仍是 HTTP（5 条）"——把 4 svc 全部标为 HTTP | 截至 2026-09-11：**实际状态已大幅前进**：chat-svc 4/4 handler 调用的方法走 gRPC（Sprint D）、assessment-svc 5/5 全 gRPC、analytics-svc 9 RPC 全 server 实现 + BFF 6/6 handler 调用的方法走 gRPC、user-svc 5/6 方法走 gRPC（Login/Register Sprint E 加, ResetPassword 仍 HTTP）。**plan 文档**与**代码事实**偏差约 2 周工作量 | 未复跑即记录（同 #3 / #9 / #22 型：plan 2026-09-07 写"5 条 HTTP"后 4 天内 Sprint C/D/E 改完大半，plan 未同步刷新）。**教训**：plan 文档写"待实施"清单时**必须带最近一次实测时间戳**，并在每次 sprint 收口时同步刷 plan 状态（本文 Sprint E 段落已刷新 §一.2/§一.3 量化覆盖度 100%/76%/85% 三档）<br>🟢 **状态更新**（Sprint F 收口, 2026-09-11）：<br>user-svc 7/7 方法全走 gRPC（Sprint F 加 ResetPassword + Logout）。plan §一.2/§一.3 覆盖度：核心 21/21=100%，BFF→4 svc 全方法 100%（Logout 算 gRPC 因有 server 实现），所有内部 svc-to-svc ~90%。本条 #34 仍 open 作为"plan 文档需刷新"案例 |
| 35 | Sprint E 收口时（2026-09-11） | Sprint E 报告写"BFF→4 svc 全方法 ~76%" + "ResetPassword/Logout 仍 HTTP" | Sprint F（2026-09-11 当日）补 ResetPassword + Logout gRPC 实现，**BFF→user-svc 7/7 方法全走 gRPC**。本条 = Sprint E 报告本身的"延迟收口"，24 小时内闭环 | 收口报告节奏（同 #28 / #29 / #30 型：决策 4 收口状态变更快，sprint 报告写到落地的时差 = 几小时~1 天）。**教训**：每次 sprint 报告必须带"未来 1-2 sprint 内必做"清单，并在下一次 sprint 第一项就处理；不允许清单跨 sprint 留存 |

附带**累积的失真速率**统计（**Stage 62 后**）：本 ADR **9 天内累积登记 27 条失真 / 5 类成因**（新增类型 6），
其中类型 1（根因臆断）6 条、类型 3（未复跑即记录）9 条、类型 4（探测方法错误）12 条、
类型 2（陈旧结论）2 条、**类型 6（破坏性脚本未带默认护栏）2 条**。
**根因臆断 + 探测方法错误**合计 13 条（占 48%）——这两类
都属于"按合理推断/错误方法得出结论"，共同的根治办法就是**多花 5 分钟真跑一次**
（决策 4.1 的"结论须附可复现命令 + 原始输出"是针对这两类最强的防线）。

> **2026-09-10 观察（值得单列）**：#28 / #29 都是"修了一处、漏了同源的另几处"。
> #28 是 Register 修了、Heartbeat/Unregister 漏；
> #29 是文档写了"要加 file-logger"、没跑验证；
> #30 是路由写出来了、没人 curl 过。
> 三者共同点：**修复动作覆盖不完整 + 缺少"跑一次"的收尾**。
> 这与 AGENTS.md §〇.2「⑥ 写完后回填」要求的"commit message 末尾列调研依据"
> 是同一层防护——**本次冒烟能一次性抓出 3 条，正是因为跑了真实 docker 端到端**。

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

**类型 5：自报告失真（#7 #10 #14 #15 #18 #22 #24）**——文档的作者/报告者**自己**就是探测者，结论/措辞
与实际情况错位而本人未察觉。这条与前 4 类的区别在于：前 4 类是"前人写下的文档
与现在的代码/现状不符"，类型 5 是"我/你当下对用户说的话/对 commit 的描述与事实
不符"。补救：向用户汇报前**先做一次关键事实的 grep/查证**（不要仅凭"看起来对"），
尤其是那些"如果我错了会导致大返工"的判断。

**类型 5 子类型 #24（2026-09-10 增）：代码侧残留扫描未做**——产品架构变更
（Stage 30 → Stage 32 入口从 BFF 改回 APISIX）后，**未做"代码侧残留扫描"**
（`grep -rn "localhost:[0-9]\+" emotion-echo-web/app/` + `grep -rn "直连 BFF" docs/`），
导致 fallback 字面值 / 文档措辞的旧假设继续存在。补救：每次架构入口或端口变更，
**必须**额外跑一次"残留扫描"——这是 plan 文档里从未列过的隐含纪律。

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
