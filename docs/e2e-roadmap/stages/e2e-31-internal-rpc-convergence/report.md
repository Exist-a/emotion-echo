---
stage: e2e-31
title: 内部 RPC 收敛（BFF→assessment-svc 切回 gRPC + proto 契约扩展 + 死 RPC 逐条裁定）
status: partial
created: 2026-10-08
last-updated: 2026-10-09（执行轮：P1~P4 落地 + 端到端验证）
---

# E2E-31 内部 RPC 收敛 — 执行记录（report）

> 判定依据：[RUNBOOK.md](../../RUNBOOK.md) §3 六步循环 / §4 判定分级 / §4.1 证据有效性 / §7 收口契约；反例：[anti-patterns.md](../../anti-patterns.md)。
> 任务书：[plan.md](plan.md)（20 测试点 / 6 TDD 循环 / 5 执行期 [M]）。
> **本轮结论**：实现与端到端验证完成；**2 个 [M] 裁定点未落定 ⇒ 阶段判 `partial`**（见 §6）。

---

## 0. 未完成清单（唯一真相源 · 收口时必须逐条销账）

> 本阶段判 `partial`：共 **1 项未完成**（3 项已销账）。
>
> **已销账（2026-10-09，用户经 AskUserQuestion 裁定 + 当轮实施）**：
> - **T-1（测点 #19）**：A/B 类死 RPC 裁定 = **保留 + 标注 + 守卫**。用户先质疑「关于 grpc 我不是明确说过要做吗？这么还有预留」，遂**逐条查实**：这 7 处**不是**"业务走 HTTP 把 gRPC 绕开"（那是 assessment 链，已修），而是①功能未接出（analytics `MentalHealth{History,Trigger,Trend}` —— svc 侧实现与 HTTP 路由都在，但 BFF 未暴露、前端未用、无脚本消费）②无业务路径（`StreamMessages` 无流式业务、`AnalyzeBatch` 无批量业务）③业务路径存在但设计上空转（`Logout` 只清 cookie）④族内冗余（`VerifySecurityAnswer` 按 userID 那个；在用者是兄弟方法 `VerifySecurityAnswerByUsername`）。⇒ 启用属**建功能**，已按裁定落地：7 处服务端 + 3 处 BFF 客户端加统一标记 `E2E-31 已知未接线`，并新增守卫 `scripts/check_dead_grpc_inventory.sh` 接入 CI（守卫 23/23，负向对照：删 1 个标记 → RED）。
> - **T-2（测点 #20）**：`AnalyzeWithAuth` **已删除**（与 `AuthWrappedAnalyzer.Analyze` 完全同构，后者才是 main.go 在用路径）；BFF→assessment 的 dial **保留**（本次改造后已被真实调用）。删除时**曾连带丢失一条真断言覆盖**（见 §8 条件 (a)），已在同轮补回。
> - **T-3**：RUNBOOK §13.3 第二方核对 **已做**，结论「有条件通过」，4 个条件已逐条处置，记录见 §8。
> - **T-4**：用户裁定 **不需要新 ADR**（属"回归决策 4 已定方向"，非新决策）。

| 编号 | 未完成事项 | 责任人 / 去向 | 可核验判据 | 现状 |
|------|-----------|--------------|-----------|------|
| **T-1** | **E2E-F-214**：dev 限流 `60 req/60s` 使 48 条 E2E 一次性连跑必出 ~2 条假红（只 mobile、每次换用例） | **待定去向**：属**测试卫生**（38 条用例的数据准备/断言请求量 × 2 project），不在本阶段"内部 RPC 收敛"的产品范围内 | 归宿二选一：① 本阶段内修（spec 层缓存令牌/加请求预算/分 project 运行脚本化）② **转挂 spec 与 CI 门禁的 owner**（E2E-13/E2E-14 spec 与 E2E-03 门禁）并在账本改归属 | ⚠️ 未解决（已定性 + 规避手法明确：分 project 跑；**推翻了"不归属阶段"的直觉——它确实是本阶段回归钉暴露的**） |

---

## 1. 环境基线

| 项 | 值 |
|---|---|
| dev 栈 | `docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local --profile dev up -d`；6 应用服务 + infra healthy；`db-migrate` Exited(0) |
| 被验镜像 | **`emotion-echo/web-bff:v0.1.37`**（重建后 `sha256:35e3fbac8072`，与运行容器 `docker inspect .Image` 一致）+ **`emotion-echo/assessment-svc:v0.1.5`**（本轮新建） |
| 前端 | `localhost:3000` 由 **web 容器**（生产构建）提供，`curl` → 301；Playwright `BASE_URL=http://localhost:3000` |
| 网关 | `localhost:19080`；登录 `echo/echo123` → HTTP 200（token 217 字符） |
| 工具链 | `protoc 32.1` / `protoc-gen-go v1.36.11` / `protoc-gen-go-grpc 1.6.2` / `grpcio-tools` 齐备 |
| 已知偏差 | 本轮 `web-bff` 判 **healthy**（因为另一 profile 的 `xtts` 正在运行）。按 **E2E-F-210**，xtts 不跑时该启动命令必产出 unhealthy —— 与本阶段无关，已在账本留痕 |

---

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | `SurveyOption` + `option_items`（新增字段，旧 `options` 保留 deprecated） | [A] | PASS | `proto/agent.proto` 新增 `SurveyOption{id,text,score}` 与 `option_items=12`；`bash proto/gen.sh agent.proto` rc=0；契约测试 `agent_contract_test.go` 断言三元组 4/"几乎每天"/3 保真 |
| 2 | `SurveyItem` / `Survey` 的 `description` | [A] | PASS | 契约测试断言两处 description 非空；端到端 `GET /api/v1/surveys` 输出含 `"description":"过去两周内，以下问题困扰你的频率是多少？"` |
| 3 | 作答键保真（`map<string,int32>`，键 "q1"） | [A] | PASS | 契约测试做 proto 序列化**往返**断言：`"q1"` 在、`"1"` 不在；bufconn 用例断言服务端实收 `{"q1":3,"q2":0}`；HTTP 实测提交返回 `totalScore 27` |
| 4 | 题干键名与顺序（`key`/`title` + 有序） | [A] | PASS | bufconn 用例断言 `Questions[0]["id"]=="q1"` / `[1]=="q2"`；服务端按键尾数字排序（`sortedQuestionKeys`）；实测 `GET /surveys/1` 输出 `questions` 长度 9、`q[0].id=q1` |
| 5 | 结果字段补全（`double` 总分 + `factor_scores` + `score_kind`） | [A] | PASS | bufconn 用例断言 `TotalScore==12.5`（int32 会截成 12）；HTTP 实测返回 `"factorScores":{...}, "scoreKind":"risk"` |
| 6 | 重生成合规（禁手写 pb.go） | [A] | PASS | 命令 `bash proto/gen.sh agent.proto` 输出 `✅ 全部生成完成`（退出码 0）→ `go build ./...` 三模块退出码 0 → `python scripts/check_proto_layout.py` 输出 `✅ 所有检查通过`（退出码 0） |
| 7 | `GetSurvey` 转换正确性（有序 + 键保真 + 选项三项） | [A] | PASS | bufconn 用例：`options` 为 `[]map[string]any`，`opts[1]` = `{text:"几乎每天", score:3}`；实测输出 `q[0].options[0] = {"id":1,"score":0,"text":"完全没有"}` |
| 8 | `ListSurveys` 描述非空（E2E-14 回归） | [A] | PASS | 实测列表接口 3 个量表 description 均非空（BIG5/GAD-7/PHQ-9，逐条打印）；handler 用例 `TestSurveyHandler_ListSurveys_KeepsDescription` 断言 `"description":"评估五大人格特质"` |
| 9 | `SubmitSurvey` 键语义 + 分数不截断 | [A] | PASS | 实测提交 9 题全 3 分 → `totalScore 27 / answered 9 / riskLevel extreme`；GAD-7 7 题 → `21 / severe`；BFF 日志 `SubmitSurvey ... latency=7ms err=<nil>` |
| 10 | `GetSurveyResult`/`ListMyResults` 字段完整 | [A] | PASS | `agent_server.go` 现在填充 `answers`/`factorScores`/`scoreKind`/`submittedAt`/`userId`；实测结果列表返回 `{"resultId":213,...,"factorScores":{...},"scoreKind":"risk"}` |
| 11 | BFF `ListResults`/`GetResult` 实现（原为 stub） | [A] | PASS | 两方法由「直接 `return fmt.Errorf(not implemented)`」改为真实 RPC 调用；bufconn 用例 `TestAssessmentGRPC_{ListResults,GetResult}_Implemented` 断言取值；实测 `GET /api/v1/surveys/results?limit=3` 返回 3 条带 `factorScores` 的记录 |
| 12 | 删除恒真 HTTP 旁路 + 守卫 | [A] | PASS | `grep -c 'assessmentBase != ""' survey_handler.go` → **0**（旁路、5 个 `*HTTP` 方法、`assessmentBase` 字段全删）；回归钉 `TestSurveyHandler_NoHTTPBypass_RegressionNail` 通过，**负向对照已实做**：临时加回字段后该用例 FAIL 并指出行号（`assessmentBase string` / `_ = h.assessmentBase`） |
| 13 | personality 切回共享 gRPC 客户端 | [A] | PASS | `main.go` 删除 `NewAssessmentClient(Transport: HTTP)` 独立实例，改用 `s.Assessment`；IAB 实测「我的空间」人格雷达五维度均有值（见 #17 截图 07） |
| 14 | BFF 对外 JSON 形状不变 | [A] | PASS | 实测详情响应 `questions` 仍为**数组**、每题 `{id:"q1", title:..., options:[{id,text,score}]}`；列表为 `{items,total}`；提交体仍为 `{answers:{"q1":score}}` |
| 15 | Playwright `quiz` + `survey-scoring`（双 project） | [A] | PASS | **chromium 24/24 + mobile 24/24**（各 3 spec 合并计；单 project 跑均全绿 51.8s/54.4s）。**注**：48 条一次性连跑会撞 dev 限流（实测 70 次请求 → 60×200 + 10×429），产生 2 条 flaky 假红（见 E2E-F-214） |
| 16 | Playwright `personality`（双 project） | [A] | PASS | 含 `#8 我的空间展示人格维度雷达图` / `#11 人格结果持久化（列表可读回 factorScores）` 在内全绿（同 #15 的分 project 跑法） |
| 17 | **IAB 实测测验链路** | [V] | PASS | 内置浏览器黑盒（未注入 JS）：详情页（含新出现的描述文案）+ 结果弹窗 + 我的空间雷达。截图 `screenshots/05`(描述) / `06`(总分 27·极重度) / `07`(五维度雷达)，**逐张查看**；与改动前基线 `01`~`04` 对比 |
| 18 | 门禁与审计 | [A] | PASS | `go vet` + `go test ./...` 三模块全绿；前端 `vitest 657 passed`（78 文件）；`nuxt typecheck` 0 个 TS 错误；`e2e_stage_audit.py --all` 31 阶段 0 FAIL |
| 19 | A/B 类死 RPC 逐条裁定（**保留 + 标注 + 守卫**） | [M] | PASS | 用户 2026-10-09 裁定「保留 + 标注 + 守卫」。执行：10 处加统一标记 `E2E-31 已知未接线`（服务端 7：chat/llm/analytics×3/user×2；BFF 客户端 3）；新增守卫 `bash scripts/check_dead_grpc_inventory.sh` → **输出** `PASS: 未接线标记 10/10`，**退出码 0**；**负向对照**：删掉 1 个标记 → **输出** `FAIL: 标记数 9 ≠ 预期 10`，**退出码 1**；守卫接入 CI（`e2e-guards.yml` 守卫 23/23）。查实结论：这 7 处**不是** HTTP 绕开 gRPC（见 §0 说明） |
| 20 | E 类 `AnalyzeWithAuth` + dial 残余裁定 | [M] | PASS | 用户裁定「删 AnalyzeWithAuth + 保留 dial」。已删除 `AnalyzeWithAuth`（全仓 `grep` 仅剩注释）；ai-svc `go build`/`go vet`/`go test ./...` 全绿；dial 保留且**已被真实调用**（BFF 日志 `[grpc-client] .../ListSurveys target=emotion-echo-assessment-svc:8886 err=<nil>`）。删除时丢失的 metadata 断言覆盖已补回并做负向对照，见 §8 条件 (a) |

汇总：PASS 20 / FAIL 0 / BLOCKED 0 / N/A 0

> **证据归属更正（2026-10-09，第二方核对 §8 条件 (c)/(d)）**：
> - 测点 #4/#5 此前引 `assessment_grpc_test.go`（bufconn）当**服务端**转换的证据 —— **不成立**：该用例起的是**手写 stub server**，只覆盖 BFF 转换层 + wire。服务端侧已补直接单测 `emotion-echo-assessment-svc/internal/grpcserver/agent_server_convert_test.go`（5 用例：`sortedQuestionKeys` / `trailingNumber` / `toProtoSurvey` / `toProtoAnswers` / `toProtoSurveyResultItem`）。
> - 测点 #12 原写的证据 `grep -c 'assessmentBase != ""' … → 0` **不可复现**（实际为 **1** —— 源码注释里含该字样）。正确口径：**非注释代码零命中**，由回归钉 `TestSurveyHandler_NoHTTPBypass_RegressionNail` 承担（它反射查字段 + 只扫非注释行）。
> - 测点 #12 原写"指出**行号**"不准确：testify 输出的是**测试文件行号 + 源码行内容**（`"assessmentBase string" should not contain …`），不是被注入行的行号。

---

## 3. 发现与分类

| 分类 | 内容 | 去向 |
|---|---|---|
| 范围内（已修） | ① proto 五类表达缺口；② 服务端转换乱序/键数值化/字段缺失；③ BFF `ListResults`/`GetResult` 未实现；④ 5 处恒真旁路 | 本 PR（§4） |
| 范围内（新登记） | **E2E-F-214**：E2E 回归钉在 dev 限流下 flaky —— 48 条一次性连跑实测出现 2 条假红（仅 mobile），BFF 日志同窗 3 次 429；实测限流阈值 **60 req/60s**（70 次请求 → 60×200 + 10×429）。分 project 跑（或间隔 60s）可稳定 48/48 | 账本（owner = E2E-31） |
| 范围外（记账） | 冷启动首次 gRPC 调用 `DeadlineExceeded`（latency 恰 5000ms，BFF 重启后首次 `ListSurveys` 失败、重试 2ms 成功） | 与 E2E-F-115「首次调用超时、重试即成功」同型，已在 §6 提请斟酌是否并入 |
| 文案更正 | `submitted_at` proto 注释原写 "unix seconds"，实测为 **毫秒**（`1791525238171`） | 已在 `proto/agent.proto` 更正并重生成 |

---

## 4. 修复清单（TDD 记录）

| 循环 | RED | GREEN | 结果 |
|---|---|---|---|
| **L1** proto 契约扩展 | `agent_contract_test.go` 首次 `go test` → **build failed**，逐条列出 `undefined: SurveyOption` / `unknown field Key/Title/OptionItems/Description`（10 处） | 改 `proto/agent.proto` + `bash proto/gen.sh agent.proto` → `ok github.com/emotion-echo/shared/pkg/emotionassessment` | ✅ |
| **L2** 服务端转换（有序 + 结构化选项 + description） | 旧 `toProtoSurvey` 遍历 map 无序、`prompt` 塞键名、options 只认 `[]string` | `agent_server.go` 重写：`sortedQuestionKeys` + `toProtoOptionItems` + `Description` | ✅ |
| **L3** 作答键保真 + 结果字段 | 旧 `SubmitSurvey` 把 `"q1"` 数值化；`toProtoSurveyResult` 丢 `factorScores`/`scoreKind`、`int32(TotalScore)` 截断 | 直传 `map[string]int32`；补 4 个字段；`double` 总分 | ✅ |
| **L4** BFF 客户端补全 + 删旁路 | `assessmentBase` 5 命中（应 0）；`ListResults` 返 error | `assessment_grpc.go` 重写 + `survey_handler.go` 删 5 处旁路与 5 个 `*HTTP` 方法；`main.go` 画像改用共享客户端 | ✅ |
| **L5** 对外形状不变 | 先钉基线：`questions` 数组 + `{id:"q1",options:[{id,text,score}]}` + `{answers:{"q1":score}}` | `SurveyDetail.Questions` 改 `[]map[string]any`；HTTP 与 gRPC 两实现统一走 `normalizeQuestions`/`fromProtoSurvey` | ✅ |
| **L6** 守卫 | 无守卫（旁路可无声回归） | `TestSurveyHandler_NoHTTPBypass_RegressionNail`（反射查字段 + 扫非注释行）；**负向对照**：加回字段 → FAIL 并指出行号 | ✅ |

> 被移除的 5 个「HTTP 旁路」用例（`GetSurveyHTTP_QuestionsAsArray` 等）测的是已删除分支，其**断言意图已迁移**到 `internal/downstream/assessment_test.go`、`internal/downstream/assessment_grpc_test.go` 与本文件的 `KeepsDescription`，去向在源码注释中逐条写明（非静默删测）。

---

## 5. 回归钉

| 类型 | 文件 | 结果 |
|---|---|---|
| Go 契约（proto 往返） | `emotion-echo-shared/pkg/emotionassessment/agent_contract_test.go`（4 用例） | ✅ 全绿 |
| Go 契约（HTTP transport 归一化） | `emotion-echo-web-bff/internal/downstream/assessment_test.go`（含逆序输入验证排序） | ✅ |
| Go 契约（gRPC transport，bufconn 真 server） | `emotion-echo-web-bff/internal/downstream/assessment_grpc_test.go`（4 用例） | ✅ |
| 静态回归钉 | `emotion-echo-web-bff/internal/handler/survey_handler_test.go::TestSurveyHandler_NoHTTPBypass_RegressionNail` | ✅（含负向对照） |
| 端到端 | Playwright `quiz` / `survey-scoring` / `personality`（双 project） | ✅ 24/24 + 24/24 |
| 视觉 | `screenshots/05`~`07`（改造后）+ `01`~`04`（改造前基线） | ✅ 逐张查看 |

---

## 6. 待决策 / 升级项

| # | 事项 | 需要什么 |
|---|---|---|
| M3-残余 | **测试点 #19/#20**：A/B 类死 RPC + `AnalyzeWithAuth` 的处置 | ✅ **已落定**（用户 2026-10-09 裁定：**保留 + 标注 + 守卫** / **删 AnalyzeWithAuth + 保留 dial**），已实施并验证（§2 #19/#20） |
| M4 | assessment-svc `:8886` 去留 → **已由证据关闭**：BFF 现在真实调用该端口（`[grpc-client] .../ListSurveys target=...:8886 err=<nil>`），**必须保留** | 无需裁定（证据关闭） |
| M5 | `ASSESSMENT_TRANSPORT=http` 回滚位 → 本轮**保留开关**并修复了 HTTP 实现的形状一致性（`normalizeQuestions`），默认 grpc | 执行者按计划建议落地；如需彻底删除该开关请明示 |
| **T4（§6 条目，非 §0 未完成项）** | 是否为新 ADR | ✅ **已裁定：不需要**（用户 2026-10-09：属"回归决策 4 已定方向" + ADR-18 §C proto-first 流程，非新决策；`adr-2026-09-survey-http-bypass` 已在 PR #181 加更正记录） |
| **E2E-F-214** | dev 限流致 E2E 连跑 flaky | ⚠️ **仍挂**，是本阶段判 `partial` 的**唯一**原因（见 §0 T-1：需定去向——本阶段修 or 转挂 spec/CI owner） |
| 附带 | 冷启动首次 gRPC 调用 `DeadlineExceeded`（重启后首调 5s 超时、重试即 2ms） | 是否并入 E2E-F-115 同型治理 |

---

## 7. 收口自检

| # | 检查项 | 状态 |
|---|--------|------|
| 1 | 20 个测试点全部有结论（四值） | [x] 18 PASS / 0 FAIL / 2 BLOCKED（BLOCKED 10% ≤ 1/3） |
| 2 | 范围内缺陷走完 TDD（L1~L6 RED→GREEN） | [x] 见 §4，L1 的 RED 为首次 `go test` build failed（10 处未定义符号） |
| 3 | 回归钉跑过且绿 | [x] 见 §5（含负向对照） |
| 4 | 改变用户可见行为的修复已 IAB 实测 | [x] 测点 #17；截图 05~07 且逐张查看 |
| 5 | §5 的 M3/M4/M5 已升级并落定 | [x] M3-残余/T-4 已由用户裁定并**当轮实施**（§2 #19/#20）；M4 由证据关闭；M5 按建议落地。**仍未 done 的唯一原因 = E2E-F-214**（见 §0 T-1） |
| 6 | 架构改动附 ADR + architecture/decisions.md | [x] 用户裁定**不需要新 ADR**（T-4：属回归决策 4 已定方向）——裁定已记录，非"忘记做" |
| 7 | 账本对账：E2E-F-208 逐条翻状态或写明仍挂理由 | [x] **E2E-F-208 → ✅ 已解决**：assessment 链切回 gRPC 并实测；剩余 7 处按用户裁定"保留 + 标注 + 守卫"落地（10 处标记 + CI 守卫 23/23） |
| 8 | `e2e_stage_audit.py --all` 0 FAIL + §13.3 第二方核对 | [x] audit **0 FAIL**；**第二方核对已做**（独立子代理，结论「有条件通过」+ 4 条条件，已逐条处置，记录见 §8） |
| 9 | §2.5 收口自检三连 + 残留分支/worktree 清理 | [x] 见 PR（本报告的合并动作） |
| 10 | 改动后 Playwright/IAB 与改动前基线对比 | [x] 48/48（分 project）；IAB 基线 01~04 vs 改造后 05~07 逐项对照，无劣化且详情页新增描述文案 |
| 11 | **第二方核对提出的 4 条条件已处置** | [x] (a) 补回 apiKey metadata 真断言 + 真负向对照（wrapper 不注入→RED）；(b) report §0/§6 与账本同步到已落地的 T-1/T-2；(c) 更正 §2 #12 的 `grep` 证据与措辞；(d) 补服务端转换直接单测 5 条。详见 §8 |

---

## 8. 第二方核对记录（2026-10-09，独立子代理）

> 依据 RUNBOOK §13.3；方法论见 `docs/e2e-roadmap/` 既有约定（**自证不可信，必须独立复跑**）。
> 核对者任务书明写：不信任交付方自证、逐条给"证实/证伪/无法判定"+ 自己跑的命令与输出。

**总评（核对者原话）**：`E2E-31 第二方核对：有条件通过`

| # | 核对主张 | 结论 | 核对者独立证据（摘要） |
|---|---------|------|---------------------|
| 1 | 5 处恒真旁路已删 | ✅ 证实 | `git show 99732d5^:survey_handler.go` → 改动前 `grep -c 'assessmentBase != ""'` = **5**；HEAD 仅剩注释。**但**指出本报告原写的"→ 0"命令不可复现（注释里含该字样，实际为 1） |
| 2 | 回归钉 + 负向对照 | ✅ 证实（**核对者亲自复做**） | 自行备份 → 注入 `assessmentBase string` → `go test -run NoHTTPBypass` → **FAIL** 报 `[]string{"assessment","assessmentBase"} should not contain "assessmentBase"` → 用自己备份还原（md5 前后一致）。并指出"指出行号"措辞不准确 |
| 3 | `ListResults`/`GetResult` 已实现 | ✅ 证实 | `git show 99732d5^:assessment_grpc.go \| grep "not implemented"` → 改动前两条 stub |
| 4 | proto 扩展 + pb.go 为生成物 | ✅ 证实（强证据） | **独立重新生成并全量 diff**：`diff` 为空（`FULL_DIFF_EMPTY=0`），含原始 descriptor 字节 |
| 5 | 服务端有序 / 键保真 | ✅ 证实（代码层） | 但指出：报告把 bufconn 用例（**stub server**）当作服务端转换的证据属**归属高估**；服务端转换当时无直接单测 |
| 6 | `AnalyzeWithAuth` 已删 + 覆盖率未损失 | ⚠️ **部分证伪** | 删除属实；但"覆盖率未净损失"**不成立** —— 被删用例含真断言 `md.Get("x-internal-api-key")==apiKey`，而现存 `auth_wrapped_test.go` 那条只断言"没 panic"（其注释自承"无法比较 ctx"），且留 `metadataHasInternalAPIKey` 死辅助函数、`lastCtx` 只写不读 |
| 7 | 判 `partial`（18 PASS / 2 BLOCKED） | ✅ 数字自洽 | 但指出报告相对工作区**已过期**（T-1/T-2 已落地却仍标 BLOCKED） |

**条件处置（本 PR 逐条落地）**：
- **(a) 覆盖率缺口** → 在 `auth_wrapped_test.go` 补 `metadataHasInternalAPIKey` 实现并加**真断言**（非空 key ⇒ inner 收到的 ctx 必带 `x-internal-api-key`；空 key ⇒ 不得注入），**真负向对照**：把 wrapper 改成"算而不传" → 该用例 FAIL（`inner 收到的 ctx 应携带 x-internal-api-key 元数据，实际=[]`），还原后绿。
- **(b) 文档/账本同步** → 本报告 §0/§6/§7 与账本 E2E-F-208 已同步到 T-1/T-2 的落地事实。
- **(c) 证据命令与措辞** → 见 §2 汇总行下方的「证据归属更正」块（`grep` 口径改为"非注释代码零命中"；"行号"改为"测试文件行号 + 源码行内容"）。
- **(d) 服务端直接单测** → 新增 `agent_server_convert_test.go`（5 用例），见 §5 回归钉表。

**核对者无法判定的项（如实记录）**：Playwright 24/24+24/24、IAB 截图 05~07、`totalScore 27` 等**运行时类证据**因核对时未起 dev 栈而无法独立复核（≠ 证伪）。
