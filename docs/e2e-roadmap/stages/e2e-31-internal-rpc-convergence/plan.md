---
stage: e2e-31
title: 内部 RPC 收敛（BFF→assessment-svc 切回 gRPC + proto 契约扩展 + 死 RPC 逐条裁定）
type: transformation
status: pending
created: 2026-10-08
last-updated: 2026-10-08（建档；**仅只读调研 + 文档，未改代码**）
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策门；执行期 [M] 决策点见 §5
related-findings: [E2E-F-208, E2E-F-213]
---

# E2E-31 内部 RPC 收敛 — 详档（任务书）

> **类型**：transformation —— 把「**决策 4 已定、代码却零调用**」的内部 gRPC 通道恢复到决策状态。
>
> **依据（三重）**：
> ① **决策 4**（跨服务调用 = gRPC + .proto；2026-09-11 收口，见 [adr-2026-09-decision-4-closure.md](../../../architecture/adr/adr-2026-09-decision-4-closure.md)）§四 #10 明文把「**BFF → assessment-svc（5 RPC 全）**」列为**已 gRPC**；
> ② **用户 2026-10-08 对 E2E-30 plan §4 M3 的裁定**：选 **③ 补全改走 gRPC**（用户原话：「得使用 grpc，这是之前定下来的，但是不知道为什么没使用」）；
> ③ 账本 **E2E-F-208**（内部 gRPC「构建了但没人用」15 处）+ 清点文档 [findings/2026-10-08-dead-grpc-inventory.md](../../findings/2026-10-08-dead-grpc-inventory.md)。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§2.1 环境铁律 / §3 六步循环 / §4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)（尤其 **AP-08 架构改动无 ADR**）。
> **前置阶段**：E2E-29 ✅ done、E2E-30 📝 已建档（并存推进；本阶段只碰 RPC 层，**不碰** §2.4 smoke / helm 渲染域）。

---

## 0. 计划期调研（**先摆事实，再排计划**）

> 本节全部为本轮**亲自回读的文件 / 亲自执行的只读命令**（AGENTS.md §〇 文档功课），非引用历史结论。
> **执行环境**：用户 2026-10-08 明示「xtts 暂时停用（已接云端 TTS API）」；本轮**起了 dev 栈做只读复核**（16 容器，BFF `unhealthy`——根因见 §0.1 F4），**未改任何代码、未跑 Playwright**。

**已读实现文件**（≥3）：
`emotion-echo-web-bff/internal/downstream/assessment_grpc.go`（全文 151 行）、`emotion-echo-web-bff/internal/downstream/assessment.go:105-147`、`emotion-echo-web-bff/internal/handler/survey_handler.go:60-320`、`emotion-echo-web-bff/internal/config/config.go:190/316`、`emotion-echo-web-bff/main.go:376-397/526-532`、`emotion-echo-assessment-svc/internal/grpcserver/agent_server.go`（全文 251 行）、`emotion-echo-assessment-svc/internal/types/types.go:12-120`、`emotion-echo-assessment-svc/internal/model/survey.go`（全文）、`proto/agent.proto`（全文）、`emotion-echo-web/app/pages/question/[id].vue:34-52/186-214`。

**已读测试 / 守卫**：`emotion-echo-web-bff/internal/handler/survey_handler_test.go`、`emotion-echo-assessment-svc/internal/grpcserver/agent_server_test.go`（+`_nildeps`/`server_race`）、`emotion-echo-assessment-svc/integration_test/survey_integration_test.go`、`emotion-echo-web-bff/main_grpc_discovery_test.go:91-134`、`emotion-echo-web/e2e/{quiz,survey-scoring,personality}.spec.ts`、`proto/gen.sh`。

**已查 ADR / 决策 / stage**：`adr-2026-09-decision-4-closure.md`（决策 4 收口，§四 #10）、`adr-2026-09-survey-http-bypass.md`（**本阶段要 supersede 的对象**；含 2026-10-08 更正记录）、`adr-2026-09-incremental-rpc-adoption.md`（ADR-18：§B「已有 HTTP 调用找时间迁移 = 非阻塞 backlog」+ §C「proto-first 强制工作流，禁止手写 .pb.go」+ 优先级表 P3「BFF→assessment-svc 3-4 天」）、`docs/architecture/decisions.md` 决策 4 / 40（TTS 主链路 = 云端 CosyVoice2，XTTS 降级离线回退）、`docs/e2e-roadmap/decisions.md` D-44。

**外部依赖官方文档**：据 ADR-18 §C 的既有约定（`proto/gen.sh` 自陈前置 = `protoc` / `protoc-gen-go` / `protoc-gen-go-grpc` / `grpcio-tools`）；本轮**未复核工具链版本**（不涉版本决策），开工按 §2 前置条件实跑 `bash proto/gen.sh` 验。

**smoke / 运行时探针**：dev 栈已起，**只做了只读探测**（`curl` 网关登录 200 / `psql` 读 survey JSONB 形状）；本阶段主战场是 RPC 层，**全量运行时验证留待开工**（§2 前置条件）。

### 0.0 假设清单（本文假设，与现状对比见 §0.1）

| # | 本文假设 | 依据 / 现状 |
|---|---------|------------|
| 1 | 「gRPC 通道没接线」 | **不成立**：通道**接好了**（`config.go:316` Transport 默认 grpc；`main.go:378` dial + `:393` 装配），是 survey handler 用**恒真旁路**把它绕开了（F1） |
| 2 | 「proto 只是少几个字段」 | **不成立**：是**表达能力不足**——`options` 是 `repeated string` 无法表达 `[{id,text,score}]`，题干键名 `prompt` 与 JSONB 的 `title` 不一致（F2/F5） |
| 3 | 「前端契约可以顺手改」 | **不成立**：前端读取的 `questions`/`options`/`answers` 形状必须**逐字段保持**（F5），proto 设计由它反推 |
| 4 | 「BFF 的 ListResults 只是没人调」 | **不成立**：**客户端方法本身未实现**（`assessment_grpc.go:94/99` 直接 `return fmt.Errorf(... not implemented ...)`）（F3） |
| 5 | 「rest 14 处死 RPC 一并删」 | **不成立**：其中 `StreamMessages`/`AnalyzeBatch`/`PinConversation` 属**决策 4 明文"proto 预留、业务未触发"**，不可与 assessment 链同批处置（F6） |

### 0.1 计划期实测事实表（**只读**；dev 栈 16 容器）

| # | 事实 | 证据（`文件:行号` / 命令输出） |
|---|------|--------------------------|
| **F1** | 🔴 **gRPC 客户端恒被构造却零调用**：`NewAssessmentClient` 在 `Transport==""\|\|"grpc"` 且 `GRPCConn != nil` 时返 gRPC 实现；`config.go:316` 注释「Transport 默认空 → 按 grpc 处理（与决策 4 对齐）」；`main.go:378` dial + `:393` `SetAssessment(GRPCConn: assessmentGRPCConn)`。而 `survey_handler.go:72/121/188/246/295` 的 `if h.assessmentBase != ""` **恒真**（`:190` 默认 `http://localhost:8889` 恒非空）⇒ 5 个 gRPC 分支恒不可达 | `assessment.go:135-137` + `config.go:190/316` + `main.go:378/393` + `survey_handler.go` 5 处 |
| **F2** | 🔴 **proto 表达能力不足**（5 类缺口，与真实 JSONB 逐条对照，见 §0.2 表） | `proto/agent.proto` 全文 vs `psql` 实读 PHQ-9 JSONB |
| **F3** | 🔴 **BFF `ListResults`/`GetResult` 客户端未实现**（stub 直接返 error），而服务端两个 RPC 已实现 | `assessment_grpc.go:94-96` / `:99-101`；`agent_server.go:201/222` |
| **F4** | 🔴 **BFF readiness 硬依赖 profile 门控的 xtts** ⇒ RUNBOOK §2.1 标准命令必产出 `unhealthy` BFF；**级联** `apisix-seed`（`depends_on: web-bff: service_healthy`）**本轮未运行** | `main.go:471-476` readiness targets 含 `xtts`（硬编码，**无配置开关**）；`apps.yml:552-556` xtts `profiles: ["ai"]`；实测 `docker exec ... wget /health/ready` → `rc=8`（503）；`docker inspect apisix-seed` → 未随本轮 `up` 重跑（沿用 etcd 旧配置）。**新登 E2E-F-210** |
| **F5** | ✅ **前端契约（proto 设计的反推依据）**：`questions` 是**数组**；每题用 `q.id`（`"q1"` 键）与 `q.title`（**题干**）；`options` 是 `[{id,text,score}]`；提交体 `{answers:{"q1":<option.score>,…}}` | `question/[id].vue:37-50`（`v-for="option in question.options"` + `option.id`/`option.text`）+ `:186-204`（`answers[q.id] = opt.score`）+ `psql` 实读 JSONB |
| **F6** | 🟡 **15 处死代码的处置**不可一刀切：C+D 类（assessment 链）本轮修；A/B 类多为 proto 预留（决策 4 §四「故意不做」明列 `StreamMessages`/`PinConversation`） | findings §清理选项；`adr-2026-09-decision-4-closure.md` §四 |
| **F7** | ✅ **§2.4 smoke 不可跑（F-209，归 E2E-30）**，本阶段若要用 smoke 验证 survey 需先等 E2E-30 的 L1 | 见账本 E2E-F-209 |
| **F8** | ✅ **网关无 BFF health 路由**（只有 `/user-health`…`/ai-health` + `/apisix-health`）⇒ 健康前置判据需另定 | `apisix/admin/routes` 实测列表；**新登 E2E-F-211** |
| **F9** | ✅ **helm 主 chart 实测 rc=0（3039 行）/ lint 0 failed**（顺带复核，供 E2E-30 用）；**子 chart 实测 23 个**（E2E-30 plan 写 22，属建档期笔误） | `helm template` / `helm lint` / `ls charts/emotion-echo/charts \| wc -l` = 23 |

### 0.2 proto ↔ JSONB ↔ 前端 三方对照（**本阶段的核心缺口表**）

| 数据 | proto 现状（`proto/agent.proto`） | 服务端/DB 事实 | 前端需要 | 判定 |
|------|-----------------------------------|----------------|----------|------|
| 量表描述 | `SurveyItem` **无** `description`；`Survey` **无** `description` | `types.SurveyItem.Description` / `model.Survey.Description` 都有 | 列表/详情展示 | **缺字段**（E2E-14 实测 3 量表描述全空） |
| 题干 | `SurveyQuestion.prompt` | JSONB 键是 `title` | `q.title` | **键名不符** |
| 选项 | `repeated string options` | `[{id:int,text:string,score:int}]` | `option.id` / `option.text` / `option.score` | **类型不符**（E2E-13 原始症状） |
| 题号 | 无（BFF 按 index 重编 `q%d`；服务端 `for qid, raw := range` **无序**） | JSONB 键 `"q1"`…`"qN"` | `answers[q.id]` | **顺序/键名不可靠** |
| 作答 | `repeated Answer{question_id int64}` + `oneof` | scorer 期望 `map["q1"]score` | `{answers:{"q1":score}}` | **键语义丢失**（`"q1"`→`1`→`"1"`；option/text 分支被 `hashStringToInt`/取长度占位） |
| 总分 | `SurveyResult.total_score` = **`int32`** | `model.SurveyResult.TotalScore` = `float64` | `totalScore` | **精度截断** |
| 维度分 | `SurveyResult` **无** `factor_scores` | `types.*.FactorScores map[string]float64` | 人格雷达图 `factorScores` | **缺字段**（E2E-F-97） |
| 分数语义 | **无** `score_kind` | `types.*.ScoreKind`（`risk`/`dimension_sum`/`ratio`） | 结果语义标注 | **缺字段**（E2E-F-97） |

> **设计约束（由 F5 + 本表反推）**：proto 的最小充分扩展 = `description` + 题干键名对齐 + **结构化的 `SurveyOption{id,text,score}`** + **键保真的 `map<string,int32> answers`** + `float64` 总分 + `factor_scores`/`score_kind`。用户 2026-10-08 已就其中两处拍板（§5 M1/M2）。

### 0.3 开工复核清单（第一天执行，防止任务书事实表过期）

| # | 复核项 | 通过标准 |
|---|--------|---------|
| 1 | **F1 复核** | `grep -c 'assessmentBase != ""' survey_handler.go` = 5（未被他人先修） |
| 2 | **F2/F3 复核** | `proto/agent.proto` 仍无 `description`/`SurveyOption`；`assessment_grpc.go:94` 仍返 error |
| 3 | **工具链** | `protoc --version`、`protoc-gen-go --version` 可用；`bash proto/gen.sh` 当前可跑通（rc=0） |
| 4 | **环境基线** | 按 RUNBOOK §2.1 起栈；**注意 F4**——xtts 停用后 BFF 仍可能 `unhealthy`，须先按 §0.1 F4 处置（归 E2E-30）或显式记录该偏差 |
| 5 | **前端契约冻结** | `question/[id].vue` 的 `questions`/`options`/`answers` 形状未变（F5 为反推基线） |
| 6 | **回归钉基线** ✅ **已取（2026-10-08）** | `quiz.spec.ts` / `survey-scoring.spec.ts` / `personality.spec.ts` 全量跑 **48/48 PASS**（2.8m，dev `BASE_URL=http://localhost:3000`）。**过程**：首跑 46/48——`survey-scoring #5` 因固定 `waitForTimeout(2000)` 在慢路由上稳定假红（**E2E-F-213**，已当轮修：改条件等待 `toHaveCount(9,{timeout:30000})`，断言值不变）；页面与后端经探针确认正常（热态 ~9.7s、详情 API 12ms/9 题） |
| 7 | **账本编号** | 新登编号从 **E2E-F-212** 起（E2E-30 已用至 F-211） |

---

## 1. 范围

### 做

- **proto 契约扩展**（`proto/agent.proto`）：`description` + `SurveyOption` + 题干键名对齐 + `map<string,int32> answers` + `float64` 总分 + `factor_scores`/`score_kind`；`bash proto/gen.sh` 重生成（**禁止手写 .pb.go**，ADR-18 §C）
- **assessment-svc 服务端 5 RPC 转换重写**：`agent_server.go` 的 `toProtoSurvey`/`toProtoSurveyResult`/`SubmitSurvey` 键语义/`GetSurveyResult` 字段补全
- **BFF 客户端补全**：实现 `ListResults`/`GetResult`；修 `fromProtoSurvey` 顺序与键名、`SubmitSurvey` 键语义
- **删除恒真 HTTP 旁路**：`survey_handler.go` 5 处 `if h.assessmentBase != ""` + 5 个 `*HTTP` 方法 + `assessmentBase` 字段；personality 画像切回共享 gRPC 客户端（`main.go:526-532`）
- **死 RPC 逐条裁定**：A/B 类 14 处 + E 类 1 处**逐条落账**（保留+注释+守卫 / 删除 / 接线），不得静默留挂
- **ADR**：新 ADR 记录 proto 收敛方案；`adr-2026-09-survey-http-bypass` 转 **superseded**；`docs/architecture/decisions.md` 登记（AP-08）

### 不做（边界）

- **新业务功能**（不新增端点、不改量表内容、不改评分算法）
- **其他 4 条 BFF→svc 链路**（chat/user/analytics/ai）——它们本就可用，不属"死代码"
- **A/B 类里决策 4 明列"故意不做"的**（如 `StreamMessages`/`PinConversation`）：本阶段只做**裁定 + 守卫**，不实现业务
- **ChatService/UserService 等其余服务的 proto 大改**（除非某条裁定明确要求）
- **E2E-30 域**：`smoke_data_layer.py` 基址 / helm 渲染守卫 / §2.4 契约（**归 E2E-30**；本阶段只在需要时消费其产物）
- **xtts 停用本身**（归 E2E-30 / 新决策；本阶段只注意别把 xtts 重新写进任何硬依赖）
- 读/打印 `deploy/.env.local` 内容、密钥进仓（AGENTS §四红线）

---

## 2. 前置条件

| 条件 | 状态 |
|------|------|
| 依赖阶段完成 | E2E-29 ✅ done；E2E-30 与本阶段**无硬依赖**（并存） |
| 环境（dev 模式容器） | 6 应用服务 + infra healthy；**注意 §0.1 F4**（BFF readiness 含 xtts 的偏差须先记录/处置） |
| 数据准备 | 演示账号 `echo` / 种子量表（PHQ-9 / GAD-7 / BIG5）；`psql` 可读 `emotion_echo_assessment.surveys` |
| 工具链 | `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc`（`proto/gen.sh` 前置） |
| 决策门 | 无（用户 2026-10-08 已拍 M1/M2） |

环境启动命令（**必须带 `--env-file .env.local` 与 `--profile dev`**，AGENTS.md §四 / RUNBOOK §2.1）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev up -d
```

> `--profile dev` 缺不得（Nacos 声明在 `profiles: ["dev"]`，缺它则 6 服务全部注册失败，E2E-F-108）。
> **不得**带 `--profile ai`（xtts 已按用户 2026-10-08 决定停用）。

---

## 3. 测试点清单（20 个）

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（需截图并被查看）· `[M]` 需人工/设计裁定（必须升级给用户）。详见 [RUNBOOK.md](../../RUNBOOK.md) §4。

### 组 A：proto 契约扩展（6）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 1 | [A] | **`SurveyOption` 消息 + `option_items` 字段**（M1 裁定 = 新增字段） | `SurveyQuestion` 新增 `repeated SurveyOption option_items`（`{id:int32, text:string, score:int32}`）；旧 `options` **保留并标 deprecated**；守卫断言生成物含 `SurveyOption` 类型 |
| 2 | [A] | **`description` 字段** | `SurveyItem` 与 `Survey` 均含 `description`；守卫断言两处（防只补一个） |
| 3 | [A] | **作答键保真**（M2 裁定 = `map<string,int32>`） | `SubmitSurveyRequest.answers` 为 `map<string,int32>`；守卫断言 `"q1"` 类键可往返不丢前缀 |
| 4 | [A] | **题干键名与顺序** | `SurveyQuestion` 含 `key`（`"q1"`）与 `title`；服务端按 key 数值**有序**输出（非 map 遍历） |
| 5 | [A] | **结果字段补全** | `SurveyResult.total_score` 改 `double`；新增 `factor_scores`（`map<string,double>`）与 `score_kind`；`answers`/`submitted_at` 实际被填充 |
| 6 | [A] | **重生成合规** | `bash proto/gen.sh` rc=0；`git diff` 只含生成文件（禁手写 .pb.go）；`go build ./...` 全绿 |

### 组 B：assessment-svc 服务端 5 RPC（4）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 7 | [A] | `GetSurvey` 转换正确性 | 契约测试断言：questions **有序**、`key` 保真、每题含完整 `option_items`（`id`/`text`/`score` 三项非零值），且与 DB JSONB 逐字段一致 |
| 8 | [A] | `ListSurveys` description 非空 | 回归 E2E-14 症状：3 个种子量表的 `description` 经 gRPC 返回非空 |
| 9 | [A] | `SubmitSurvey` 键语义 + 分数**不截断** | 断言写入 DB 的 `answers` 键为 `"q1"`…（非 `"1"`）；`totalScore` 小数不丢；`factorScores`/`scoreKind` 原样回传（回归 E2E-F-97） |
| 10 | [A] | `GetSurveyResult`/`ListMyResults` 字段完整 | 断言 `answers`/`factorScores`/`submittedAt`/`scoreKind` 均有值（当前 server 不填 `answers`/`submittedAt`） |

### 组 C：BFF 客户端补全 + 删旁路（4）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 11 | [A] | **`ListResults`/`GetResult` gRPC 实现** | 两方法不再返 `not implemented`；单测覆盖正常与 not-found 路径 |
| 12 | [A] | **删恒真旁路** | `survey_handler.go` 5 处 `if h.assessmentBase != ""` + 5 个 `*HTTP` 方法 + `assessmentBase` 字段全部移除；守卫断言全仓 `assessmentBase` 零命中 |
| 13 | [A] | **personality 切回共享 gRPC** | `main.go` 不再为画像单建 `Transport: HTTP` 客户端；画像取数走 `s.Assessment`（同一 gRPC 连接）；断言只存在 1 个 assessment 客户端 |
| 14 | [A] | **BFF 对外 JSON 形状不变** | 契约测试断言响应仍为 `questions` **数组** + 每题 `{id, title, options:[{id,text,score}]}` + 提交体 `{answers:{"q1":score}}`（与 F5 基线逐字段一致） |

### 组 D：端到端回归（4）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 15 | [A] | Playwright `quiz.spec.ts` + `survey-scoring.spec.ts` | 双 project（chromium + mobile）**全绿**，与 §0.3 #6 基线等量或更好 |
| 16 | [A] | Playwright `personality.spec.ts` | 全绿；画像注入链路（列表取 factorScores → system prompt）未回归 |
| 17 | [V] | **IAB 实测测验链路** | 真实浏览器：列表（描述可见）→ 答题（选项文本/顺序正确）→ 提交 → 结果（分数/维度/语义）→ 我的空间画像；**截图且被查看** |
| 18 | [A] | 门禁与审计 | `go test ./...` + `go vet` + 前端 vitest/typecheck 全绿；`python scripts/e2e_stage_audit.py --all` 0 FAIL |

### 组 E：其余死 RPC 逐条裁定（2）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 19 | [M] | **A/B 类 14 处逐条落账** | 每条明确处置（保留+注释+「不得新增调用方」守卫 / 删除 / 接线），**不得静默留挂**；决策 4 明列"故意不做"者（`StreamMessages`/`PinConversation`）保留并注明依据 |
| 20 | [M] | **E 类 `AnalyzeWithAuth` + BFF 白 dial 残余** | `ai-svc/analyzer/grpc_analyzer.go:193` 与 assessment 白 dial 的处置裁定并落地（删 / 接线 / 注释+守卫） |

汇总行（执行期填写）：PASS x / FAIL y / BLOCKED z / N/A w

---

## 4. TDD 循环划分（RED→GREEN→REFACTOR）

| 循环 | RED（先行失败测试） | GREEN（最小实现） | 影响面 |
|------|-------------------|------------------|--------|
| **L1** | 契约测试断言 pb.go 含 `SurveyOption` / `answers` 为 map / `description` 存在（当前缺 ⇒ 必红） | 改 `proto/agent.proto` + `bash proto/gen.sh` | `proto/` + shared pb（**禁手写**） |
| **L2** | `GetSurvey` 断言有序 + key 保真 + options 完整（当前无序/丢结构 ⇒ 必红） | `agent_server.go` `toProtoSurvey` 重写（按 key 数值排序） | assessment-svc grpcserver |
| **L3** | `SubmitSurvey` 断言 DB 键为 `"q1"`（当前 `"1"` ⇒ 必红）+ 小数不截断 | `agent_server.go` `SubmitSurvey` + `toProtoSurveyResult` | 同上 |
| **L4** | 守卫断言全仓 `assessmentBase` 零命中（当前 5 命中 ⇒ 必红）+ `ListResults` 不返 error | 删旁路 + 实现两方法 + personality 切 gRPC | BFF handler/downstream/main |
| **L5** | BFF 对外形状契约测试（先钉基线，再保证改造后仍绿） | 转换层调整以保持形状 | BFF 响应契约 |
| **L6** | 守卫：`*.pb.go` 无手写痕迹 + 裁定保留的死 RPC "不得新增调用方"（当前无守卫 ⇒ 必红） | 新增守卫脚本 + 接 CI | `scripts/` + CI |

> **边界提醒**：L1/L4 触及「跨服务协议层」属 RUNBOOK §13.3 架构关键词 ⇒ **必须同批附 ADR + `docs/architecture/decisions.md` 变更**（防 AP-08）。
> L1 改字段属**协议变更**：M1 已定"新增 `option_items` 而非改 5 号字段"⇒ 无 wire-breaking；生成物变更须与 BFF/assessment **同 PR 落地**。

---

## 5. 执行期 [M] 决策点

| # | 决策 | 背景 | 状态 / 备选 |
|---|------|------|------------|
| **M1** | `options` 扩展方式 | `repeated string` 无法表达 `[{id,text,score}]` | ✅ **已裁定（用户 2026-10-08）= 新增 `repeated SurveyOption option_items`，旧字段保留 deprecated**（备选②直接改 5 号字段被否：wire-breaking） |
| **M2** | 作答传输形态 | `Answer.question_id int64` 压掉 `"q1"` 前缀 | ✅ **已裁定（用户 2026-10-08）= `map<string,int32> answers`**（备选②`repeated Answer`+`question_key` 被否：转换双轨复杂） |
| **M3** | A/B 类 14 处逐条处置 | 含 proto 预留（决策 4 明列"故意不做"）与"服务端先实现客户端未接" | ⏳ 执行期逐条升级（测点 #19） |
| **M4** | assessment-svc `:8886` 去留 | BFF 切回 gRPC 后它重新有客户端 | ⏳ 执行期确认：**预期保留**（Nacos 元数据 `grpc_port=8886` 已被 `nacos_boot_test.go` 钉住） |
| **M5** | 迁移窗口 / 回滚策略 | `ASSESSMENT_TRANSPORT=http` 现为唯一生效路径 | ⏳ 执行期定：是否保留该开关一个版本作为回滚位（建议：**保留开关但默认 grpc**，删 HTTP 旁路实现） |

---

## 6. 收口门槛

- [ ] 20 个测试点全部有结论（PASS/FAIL/BLOCKED/N/A 四值，BLOCKED ≤ 1/3）
- [ ] 范围内缺陷走完 TDD（L1~L6 各自 RED→GREEN 记录）
- [ ] **回归钉**：新增/扩展 spec（survey RPC 契约 + BFF 形状契约 + pb.go 守卫），**跑过且绿**
- [ ] 组 D 的 Playwright 三 spec **改动前基线 vs 改动后** 均有记录，且不劣化
- [ ] **IAB 实测**（测点 #17）：截图已产出且**被查看**
- [ ] §5 的 M3/M4/M5 **已升级给用户并落定**
- [ ] **ADR + `docs/architecture/decisions.md`** 已落地；`adr-2026-09-survey-http-bypass` 转 **superseded**
- [ ] **账本对账**：E2E-F-208 逐条翻状态或写明仍挂理由；测点 #19/#20 的裁定逐条落账
- [ ] `python scripts/e2e_stage_audit.py --all` → 0 FAIL；§13.3 第二方核对通过
- [ ] §2.5 收口自检三连 + 残留分支/worktree 清理

---

## 7. 风险与缓解

| 风险 | 应对 |
|------|------|
| **删旁路后前端形状漂移**（最危险） | 先钉 F5 基线形状契约测试（L5），再改造；改造后同测试必须绿；IAB 实测（#17）兜底 |
| **proto 生成物不同步**（只改一边） | pb 为 shared 包，`bash proto/gen.sh` 后 `go build ./...` 必须全绿；L6 守卫防手写 |
| **`total_score` int32→double 静默截断** | 测点 #9 断言小数不丢；对照 `model.SurveyResult` 字段类型 |
| **M3 范围膨胀**（14 处） | 按"决策 4 明列故意不做" vs "真未接线"二分；后者才有实现选项；全部落账不留挂 |
| **xtts 停用与旧文档冲突**（决策 40 / D-44 说 XTTS 是回退） | 本阶段**不碰** TTS 链；仅在涉及 readiness/编排时注意别把 xtts 写回硬依赖（F4 归 E2E-30） |
| **本阶段与 E2E-30 并行改同一文件**（`main.go` / compose） | 双方按 [parallel-tracks](../../../_meta/parallel-tracks.md) 精神串行合并；触碰对方独占列先登记 |
| 计划期未跑 Playwright/未复核工具链版本 | §0.3 开工复核 7 项**逐项跑**（尤其 #3 工具链、#6 基线） |

---

## 8. 引用

- 执行协议：[RUNBOOK.md](../../RUNBOOK.md) · 反例：[anti-patterns.md](../../anti-patterns.md)
- 排期与状态：[roadmap.md](../../roadmap.md) · 账本：[discovered-unresolved.md](../../discovered-unresolved.md)（E2E-F-208 属本阶段）
- 调研产物：[findings/2026-10-08-dead-grpc-inventory.md](../../findings/2026-10-08-dead-grpc-inventory.md)
- 决策 4：[adr-2026-09-decision-4-closure.md](../../../architecture/adr/adr-2026-09-decision-4-closure.md) · [adr-2026-09-incremental-rpc-adoption.md](../../../architecture/adr/adr-2026-09-incremental-rpc-adoption.md)（ADR-18）
- 被取代对象：[adr-2026-09-survey-http-bypass.md](../../../architecture/adr/adr-2026-09-survey-http-bypass.md)
- 契约来源：`proto/agent.proto` + `emotion-echo-web/app/pages/question/[id].vue`（前端反推基线）
- 执行记录（收口时写）：`stages/e2e-31-internal-rpc-convergence/report.md`
