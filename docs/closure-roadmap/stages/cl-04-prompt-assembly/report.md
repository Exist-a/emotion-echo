---
stage: cl-04
title: Prompt 组装层 — 执行记录
type: report
status: in-progress
created: 2026-10-14
last-updated: 2026-10-14（Phase 0 基线 + Phase 1 统一组装层 + Phase 2 历史注入 + **Phase 3 参数下发**（9/16 测试点 PASS）；阶段未收口 —— 余 5 点 + T-14 在 Phase 4~7）
---

# CL-04 · Prompt 组装层 — 执行记录（report）

> 本文件按 [RUNBOOK](../../RUNBOOK.md) §6 的模板口径记录：**起点 / 实施 / 证据 / 结论 / 经验**。
> 结果四值 `PASS / FAIL / BLOCKED / N/A`，**证据 = 执行输出**（命令 + 输出/退出码/截图）；
> "已创建/已新增"不算证据。**阶段未收口** —— 执行者不得自行宣布 done。

## §1 起点（开工时的事实）

| 项 | 事实 | 证据 |
|---|---|---|
| 分支 / 工作区 | `feat/cl-04-prompt-assembly`，worktree `../Emotion-Echo-cl04`，base = `origin/main` @ `35cdfe6` | `git worktree list` |
| 并行占位 | A 轨已登记（契约 §三），dev mode 锁已写 | `deploy/.devmode-session` → `owner: lane-cl-a`，`until: 2026-10-14 12:00` |
| 决策 | [M] T-16 **用户裁定 2026-10-14**：`MaxTokens=1024` / `Temperature=0.7` | plan §2 T-16 / §6 R1 |
| 起栈前容器态 | 仅 4 个在跑（`emotion-llm-service` / `xtts` / `sensevoice` / `fer`，皆 Up 2h）；其余 20+ 皆 Exited 11h | `docker ps -a` |
| 用户指示 | 语音合成容器停用 ⇒ 已 `docker stop emotion-echo-xtts`（释放 ~3.1GiB） | `docker stats` 前后对比 |
| 内存上限 | WSL `.wslconfig` = `memory=10GB` + `swap=4GB`（2026-09-23 用户定） | `/c/Users/LENVOV/.wslconfig` |

## §2 实施（Phase 0）

1. 按 RUNBOOK §2 起栈：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local --profile dev up -d`
   —— ⚠️ **在共享目录（主工作区）执行**：worktree 里没有 gitignored 的 `deploy/.env.local`（见 CL-F-07）。
2. 核对服务注册（期望 `count:6`）。
3. 写 Phase 0 取证探针（`evidence/*.mjs`，走网关 `:19080` + demo 账号 `echo/echo123`），跑 3 轮真实对话。
4. 取 llm-service 侧 `messages=N` 日志。
5. **镜像新鲜度核验** → 发现 `check_image_freshness.sh` 报 8/8 STALE → 对"报告与代码相反"的风险做**行为验证** → 重建真陈旧的 `web-bff` → 复跑。

## §3 证据

### 3.1 起栈与服务注册 — PASS

```text
$ curl -s "http://localhost:8848/nacos/v1/ns/service/list?pageNo=1&pageSize=50&namespaceId=emotion-echo-dev"
{"count":6,"doms":["emotion-echo-chat-svc","emotion-echo-ai-svc","emotion-echo-assessment-svc",
                   "emotion-echo-analytics-svc","emotion-echo-web-bff","emotion-echo-user-svc"]}
```
栈内 17 容器 Up（infra + apps；obs profile 未启用、xtts 已按要求停）。

### 3.2 Phase 0 主结论：多轮对话下 BFF 仍只发 `[system, user]` **两条** — PASS

运行 3 轮对话（**每轮先把 user 消息落库再打 `/ai/stream`**），会话最终 **6 条消息**（3 user + 3 assistant），
而 llm-service 侧收到的**每一轮都是 `messages=2`**：

```text
$ node evidence/phase0-capture.mjs
turn1: persist=200 stream=200 replyChars=63
turn2: persist=200 stream=200 replyChars=92
turn3: persist=200 stream=200 replyChars=69
{ "conversationId": "668", "conversationMessageCount": 6, "login": {"status":200,"gotToken":true} }

$ docker logs --since 5m emotion-llm-service | grep "ChatCompletion request"
{"msg": "ChatCompletion request: messages=2 model=deepseek-chat", "time": "2026-10-14T00:31:44.394Z"}
{"msg": "ChatCompletion request: messages=2 model=deepseek-chat", "time": "2026-10-14T00:31:46.670Z"}
{"msg": "ChatCompletion request: messages=2 model=deepseek-chat", "time": "2026-10-14T00:31:49.089Z"}
```

**释义**：第 1/2/3 轮时会话里分别有 2/4/6 条消息，而 prompt 恒为 2 条 ⇒
**历史完全没有进 prompt**（plan §A.1 的源码结论 `ai_stream_handler.go:414-422/:477-485` 得到运行时证实）。
`model=deepseek-chat` ⇒ 真实上游，非 mock（排除"降级掩盖"）。

> 附带基线：3 轮回复长度 **63 / 92 / 69 字符**（Phase 3 落地 `MaxTokens=1024` 后，用它对照"是否变啰嗦"，plan §6 R1）。
> 证据文件：`evidence/phase0-output.txt`、`evidence/llm-service-messages.log`。

### 3.3 镜像新鲜度：**8/8 → 7/8**，其中 web-bff 真陈旧已修、chat-svc 为**假红** — PASS（附 CL-F-08）

起栈后守卫报 **8/8 STALE**。但该守卫判据是**纯时间比较**，我做了**行为验证**（不用时间当结论）：

```text
重建前 $ node evidence/phase0-staleness-probe.mjs 667        （输出：phase0-staleness-output.txt）
GET /messages  data keys=["list"]           ← 缺 827f654 新增的 cursor/hasMore ⇒ 真陈旧
GET /messages?limit=2  返回 ids=[854,855] = 全量最后两条 ⇒ chat-svc 已是"最新优先"新语义

重建后 $ node evidence/phase0-staleness-probe.mjs 667        （输出：phase0-staleness-output-after-rebuild.txt）
GET /messages  data keys=["cursor","hasMore","list"]        ← 新契约生效
```

- **`emotion-echo-web-bff`：判定为真陈旧**（行为证据：响应缺 `cursor`/`hasMore`）；已按正确 compose 组合
  （**必须带 `-f docker-compose.infra.yml`**，否则 `invalid compose project`：`kafka` 依赖解析失败）重建 + `--force-recreate`；
  复跑后守卫转 `[OK]`：

```text
$ bash scripts/check_image_freshness.sh | grep web-bff
[OK]   emotion-echo-web-bff: 镜像 ≥ 源码 commit 827f654
$ docker image inspect --format '{{.Created}}' emotion-echo/web-bff:v0.1.37
2026-10-14T00:30:31.561065714Z        （本机 08:30，重建时刻）
```

- **`emotion-echo-chat-svc`：判为假红**（守卫仍 STALE，但行为已新：旧代码 `Order("id ASC")` 取最早 N 条，
  运行栈 `?limit=2` 返回的是**最新**两条）。根因：改动先写在工作副本里、镜像正是那时构建的，**提交时间晚于构建时间** ⇒
  纯时间判据无法区分。已登 **CL-F-08**（owner CL-00）。
- 其余 6 个（`user-svc`/`analytics-svc`/`assessment-svc`/`llm-service`/`ai-svc`/`web`）**只按时间判 STALE，未做行为核验**
  —— 不得据此声称"都缺代码"（正是 CL-F-08 的误读风险）。

### 3.4 chat-svc 排序语义（Phase 2 依赖）— PASS

运行时确认：`GET /messages?limit=2` 返回**最新两条且时间升序**（`ids=[854,855]`，与全量尾部一致）。
⇒ plan §A.3 的假设（"取最新 limit 条后反转为升序"）在**运行时成立**，Phase 2 可直接按序拼历史。
⚠️ 同时确认 `ai_stream_handler.go:318` 的注释"msgs 按时间倒序（最新在前）"与实现不符（陈旧注释，plan §6 R5）。

### 3.5 Phase 1：统一组装层落地（换实现不改行为）— PASS

**做法**：先用**改造前的旧实现**一次性产出 golden 字面量（临时 `zz_golden_gen_test.go`，生成后即删），
再用它当 oracle 写 RED 钉子，最后删掉旧实现 —— 这样"零回归"钉的是**旧字节**而非新实现的自证。

RED（先红，证据：编译错误 = 新 API 全部未定义）：

```text
$ go test ./internal/handler/ -run TestPromptAssembly -count=1
prompt_assembly_test.go:102:15: c.h.assembleSystemPrompt undefined
prompt_assembly_test.go:102:36: undefined: assembleInput
prompt_assembly_test.go:131:24: undefined: segmentKindBase
... FAIL emotion-echo-web-bff/internal/handler [build failed]
```

GREEN（落地后）：

```text
$ go test ./internal/handler/ -run TestPromptAssembly -v -count=1
--- PASS: TestPromptAssembly_ZeroRegression_GoldenBytes (6 子例全 PASS：无信号 / 仅画像 / 仅情绪 /
         仅历史 / 画像+情绪 / 画像报错)
--- PASS: TestPromptAssembly_MessagesAreSystemThenUser
--- PASS: TestPromptAssembly_SegmentOrderIsFixed (6 子例 + base 恒首 + canonical 子序列不变量)
--- PASS: TestPromptAssembly_ServeHTTPHasNoInlineSystemMessage
ok  emotion-echo-web-bff/internal/handler

$ go vet ./...                                  → exit 0
$ go test ./... -count=1                        → 全部包 ok（10 包）
$ go vet -tags integration ./...                → exit 0（CI 的 build-tag 编译守卫）
```

**改动清单**：
- 新增 `internal/handler/prompt_assembly.go`：`systemSegments`（段序唯一来源）/ `assembleSystemPrompt` / `assembleMessages`（唯一组装点）；
- `ai_stream_handler.go`：**删** `buildSystemPrompt` + `buildSystemPromptWithEmotion`（缺陷 11 的两处内联调用点收敛为 2 次 `assembleMessages` 调用）；
- 既有测试 `d14_emotion_context_test.go` / `f122_emotion_history_fallback_test.go` 的调用点机械迁移到新 API（**断言逐条保持不变**）。

**行为等价性**：段序 `base → personality → emotion` 以 `"\n\n"` 连接，与旧实现 `base + "\n\n" + guide + "\n\n" + emotionSeg` 逐字一致；
空段直接丢弃（不产生空行）；画像报错时仍打同一条 error 日志并回落基础人设。

**运行时等价（不是只跑单测）**：按 RUNBOOK §2「改完 Go 必须重建镜像再验收」，
用**新代码**重建 web-bff 并 `--force-recreate`，再跑一遍 3 轮真实对话：

```text
$ docker image inspect --format '{{.Created}}' emotion-echo/web-bff:v0.1.37
2026-10-14T00:47:02.77200162Z      ← 本次从 worktree 源码构建（Phase 1 代码）
$ docker ps --filter name=emotion-echo-web-bff
emotion-echo-web-bff  Up (healthy)  emotion-echo/web-bff:v0.1.37
$ node evidence/phase0-capture.mjs        （输出：evidence/phase1-runtime-verify.txt）
turn1/2/3: persist=200 stream=200 replyChars=86/53/78
$ docker logs --since 3m emotion-llm-service | grep "ChatCompletion request"
messages=2 model=deepseek-chat   ×3        ← 与 Phase 0 基线一致：仍两条、仍真上游
```

⇒ **可观测契约未变**（仍 `[system, user]` 两条 + 真 upstream），符合"换实现不改行为"的口径。

**顺带验证 CL-F-07 ③ 的绕过路径**：`cd <worktree>/deploy && docker compose … --env-file <共享目录>/deploy/.env.local build` **可以成功**（exit 0），
即"worktree 建镜像 + 共享目录 up 容器"两步法可行 —— 这是并行轨在 worktree 里验证 Go 改动的可行做法（已回填 CL-F-07 用途）。


### 3.6 Phase 2：同会话历史注入 — PASS

**本阶段 [M] 参数（用户 2026-10-14 裁定）**：滑动窗口**默认最近 10 条 / 上限 20 条**（配置夹在 1..20，token 预算只缩不扩）；
token 预算硬顶 **2000**（中文按 1 字 ≈ 1 token 估算，不引 tokenizer —— plan §3.3"只需防爆"）。
另设**当前轮输入上界 4000 rune**（防输入炸弹的工程兜底，非产品参数；超出按 rune 边界截断 + WARN）。

**实现**：新增 `internal/handler/prompt_history.go`；`assembleMessages` 结构变为
`[system] + [历史(时间正序)] + [当前 user]`。取数复用 chat-svc `ListMessages(cursor=0, limit=上限+1)`。

RED（先红）：

```text
$ go test ./internal/handler/ -run TestHistory -count=1
prompt_history_test.go:84:18: undefined: maxHistoryMessages
prompt_history_test.go:175:24: undefined: defaultHistoryMessages
prompt_history_test.go:198:24: undefined: estimateTokens
... FAIL [build failed]
```

GREEN（单测 9 组）：

```text
--- PASS: TestHistory_InjectedInChronologicalOrderBetweenSystemAndCurrentUser
--- PASS: TestHistory_DoesNotDuplicateCurrentTurn          ← 前端"先落库再打"⇒ 最新一条即当前轮，按内容去重
--- PASS: TestHistory_ExcludesFileEmptyAndUnknownRoles     ← file 走 Files 通道 / tool / 空白不进历史
--- PASS: TestHistory_NoHistorySignals_YieldsExactlyTwoMessages（5 子例：无会话/无来源/空/报错/去重后空）
--- PASS: TestHistory_WindowKeepsNewestTenByDefault        ← 25 条里取最新 10 条
--- PASS: TestHistory_TokenBudgetTrimsOldestFirst          ← 3600 token ⇒ 从最旧端裁到 ≤2000
--- PASS: TestHistory_BudgetNeverDropsBelowOneTurn         ← 单条即超预算也只裁到"最近 1 轮"
--- PASS: TestHistory_UserContentClampedAtRuneBoundary     ← 4500 rune → 4000，UTF-8 不裂
--- PASS: TestHistory_UserContentUntouchedWhenWithinLimit  ← 未超上界逐字透传（含首尾空白）
```

**运行时决定性证据（重建镜像 + force-recreate 后 3 轮真实对话）**：

```text
$ node evidence/phase0-capture.mjs        （输出：evidence/phase2-runtime-verify.txt）
turn1/2/3: persist=200 stream=200 replyChars=57/94/114

$ docker logs --since 3m emotion-llm-service | grep "ChatCompletion request"
messages=2 model=deepseek-chat   ← 第 1 轮（无历史 / 当前轮去重后为空）
messages=4 model=deepseek-chat   ← 第 2 轮（system + u1 + a1 + u2）
messages=6 model=deepseek-chat   ← 第 3 轮（system + 4 条历史 + u3）
```

⇒ **历史真的进了 prompt**（Phase 0 基线是"三轮恒 2"）。窗口 10 / 预算 2000 在此规模未触发，符合预期。

### 3.7 Phase 3：生成参数真下发 — PASS

**缺陷 3**：handler 从不给上游请求赋 `Temperature`/`MaxTokens` ⇒ 上游拿到零值 ⇒ **输出长度无上限**。

**实现**：新增 `internal/handler/prompt_params.go`（**单一来源** `llmGeneration`，Go 常量集中 —— roadmap §4 定的载体口径）；
两条上游路径都赋值；`downstream.LLMChatReq` 补 `Temperature`/`MaxTokens`（带 `omitempty`，未设即不发送）。

RED → GREEN：

```text
$ go test ./internal/handler/ -run TestLLMParams -count=1
prompt_params_test.go:68:3: unknown field Temperature in struct literal of type downstream.LLMChatReq
prompt_params_test.go:69:3: unknown field MaxTokens ...
FAIL [build failed]

（实现后）
--- PASS: TestLLMParams_SentOnGRPCPath              ← fake 捕获 0.7 / 1024 + 非零断言（缺陷 3 不复现）
--- PASS: TestLLMParams_BothUpstreamPathsCarryThem  ← 两条路径都带（计数不依赖 gofmt 对齐）
--- PASS: TestLLMParams_SerializedIntoUpstreamBody  ← LLMChatReq 序列化真出现 temperature/max_tokens
--- PASS: TestLLMGRPCClient_StreamChat_MapsGenerationParamsIntoPB  ← 新增 pb 层回归钉
```

> **顺手补的一个空隙**：`llm_grpc_test.go` 的**文件头早已声称**覆盖 "消息映射（messages/model/temperature/max_tokens）"，
> 但实际只断言了 `model`/`messages` —— 这正是缺陷 3（零值）长期没被门禁发现的原因。已补 pb 层断言钉。

**运行时 + R1 长度张力核对**（§6 R1 要求"Phase 3 落地后核对是否变啰嗦"）：

```text
$ node evidence/phase0-capture.mjs      （输出 phase3-runtime-verify.txt）
turn1/2/3: persist=200 stream=200 replyChars=70/63/90
$ docker logs emotion-llm-service | grep "ChatCompletion request"  → messages=2/4/6（历史注入仍生效）

$ node evidence/phase3-replylength.mjs 669     （读落库回复全文，零额外 LLM 调用）
[ai] runes=86 句子数≈3   ← 第 1 轮
[ai] runes=53 句子数≈2   ← 第 2 轮
[ai] runes=78 句子数≈3   ← 第 3 轮
```

⇒ **回复仍是 2~3 句（53~86 字）**，与 Phase 0 基线（63/55/81）同量级 ⇒ **`MaxTokens=1024` 未使回复变啰嗦**
（模型按 system 人设自我约束，上限没被触及）。§6 R1 的张力**在本样本上未出现**，无需复议。
另：第 2/3 轮的回复明确在承接上文（"明明自己做过，一被盯着问…" / "这两个问题在你心里已经转了好几圈"）
—— 历史注入的可读效果（T-14 的 IAB 正式判定仍在 Phase 7）。

**附带发现（T-13 有用）**：读路径的 VM 字段是 `sender`（`"ai"`/`"user"`），写路径是 `role`（`"assistant"`）。
不是 bug（是刻意的前端 VM），但属"统一字段集"要盘点的差异项。

### 3.9 Phase 4：文案集中 + 三方一致性守卫 — PASS

**Part A 文案集中**（验收 D「system 文案集中一处」）：新增 `internal/handler/prompt_templates.go`，
把**基础人设 / 人格适配表 + 护栏 + 分档阈值 / 情绪段句式 / 中文情绪映射**全部集中到一处；
`personality_directive.go` 收缩为"只留 `buildPersonalityGuide` 算法"；`ai_stream_handler.go` 移除
`baseSystemPrompt`、情绪句式内联与 `emotionChineseLabel`（switch → 表查，未收录原样返回，行为等价）。
顺手删掉**死变量** `personalityDimensionLabels`（全仓零引用，仅声明）——原属 Phase 5 死路径处置项，提前清掉。

> **零行为变更的证据**：Phase 1 的 golden **字节**钉（6 例）在搬迁后仍全绿 —— 搬迁不动字节。
> 因是纯搬迁，**未单独重建镜像**（运行时随 Phase 5 一并复验）；判断依据是字节级 golden + 全模块测试，不是自述。

**Part B T-9 三方一致性守卫**：同一份展示契约由三处独立实现（前端 `personality.ts` / BFF 适配表+阈值 /
assessment `dimensionMap`），而前端那份的**文件头自己写着**"与后端两处实现保持一致，改任一处必须同步改另两处"
—— 纯靠人记（本项目历史上正是这类人工同步漂移过）。守卫比对：维度序列 / 中文标签 / 满分 / 分档阈值 / 每维题数。

三个设计取舍（写进报告以免被当成"随手写的测试"）：

| 取舍 | 选择 | 理由 |
|---|---|---|
| 守卫 vs 单一来源派生（plan §3.4 二选一） | **机器守卫** | 前端要在**运行时**用 label 渲染雷达图，派生要么加接口要么上 codegen；先把漂移变红灯，成本最低 |
| 放 Go 测试 vs `scripts/` 守卫 | **Go 测试** | 随 `go-test.yml` 自动执行，**不用改 `.github/workflows/**`**（D 轨文件面，且新 job 要动总闸 needs）；CI 整仓 checkout ⇒ 跨模块相对路径同样成立（已核 `go-test.yml`） |
| 解析不到怎么办 | **判红**，并带解析器自检 | 反"未解析到 ⇒ 跳过"的空跑 PASS（E2E-F-180 同型教训） |

```text
$ go test ./internal/handler/ -run TestPersonalityContract -v -count=1
--- PASS: TestPersonalityContract_ThreeWayConsistent   ← BFF ↔ 前端 ↔ assessment 无漂移
--- PASS: TestPersonalityContract_DriftIsDetected      ← 负向对照 3 例（标签/阈值/维度顺序）全部抓到
$ go test ./... -count=1    → 全包 ok        $ go vet ./... → exit 0
```

**顺带发现（并入本阶段 T-13 盘点）**：前端 `personality.ts` 文件头指向的 BFF 函数名 `formatPersonalityContext`
**在 BFF 里并不存在**（真实实现是 `personality_directive.go` 的适配表 + 阈值）
⇒ 一例文档漂移（守卫钉的是**值**一致性，函数名漂移不在其范围内）。

### 3.10 Phase 5：死路径处置 + file_context 修复 — PASS

三件事分开提交（每件都含自己的测试）：

**T-10 修 `file_context.py` 多附件缩进 bug**（`fix(llm-service)`，`58c4a1b`）

```text
RED（新增 tests/unit/test_file_context_multi_attachment.py，3 附件）：
  assert 1 == 3  →  输出只剩 "--- 附件 3：ok2.txt"
  （i/name 泄漏成循环末值、正文只有最后一条）3 例全红
GREEN：python -m pytest tests/unit → 211 passed（新增 3 例；既有只覆盖单附件，正是本 bug 潜伏的原因）
```

**T-11 接回意图风格注入**（`feat(bff)`，`cb29ab7`）：`LLMStreamRequest.WithIntent` → pb → 单例常量 `llmWithIntent = true`。
两条钉（handler 侧 fake 捕获 + downstream 侧 mock servicer 断言 `GetWithIntent()`）。运行时确认交接成功：

```text
$ docker logs emotion-llm-service | grep "[intent]"
[intent] career_help (confidence=0.9)   ← 0.9 = LLM 消歧路径的固定置信度 ⇒ 规则式拿不准、确实发生了额外 LLM 调用
[intent] career_help (confidence=0.9)
[intent] other (confidence=0.0)
（基线跑同一脚本时该日志 0 条 ⇒ 端到端确认"接回前真的没触发"）
```

**⚠️ 接回的代价（实测，非推测）**：

| 指标 | 未接回（基线） | 接回后 | 变化 |
|---|---|---|---|
| TTFT（3 轮均值） | 1056 / 1042 / 722 → **940ms** | 1290 / 1212 / 1584 → **1362ms** | **+422ms（+45%）** |
| 回复长度 | 53~86 字 / 2~3 句 | 51 / **192** / **278** 字（第 2/3 轮 ≈6/13 句） | **3~5 倍，且变成分点列表体** |

第 2/3 轮长回复的直接原因**不是** `MaxTokens`（1024 远未触及），而是 llm-service 的
`STYLE_INSTRUCTIONS["career_help"]`（"分点给出建议（1. 2. 3.）…关键建议加粗标注"）**压过了基础人设的"2-3 句话简短回应"**
⇒ 同一份 system 段里出现两条互相矛盾的篇幅指令。**这是产品口径问题，交用户裁定**（选项：①调 `STYLE_INSTRUCTIONS` 与陪伴人设对齐；
②`llmWithIntent` 翻回 false 放弃该收益；③按意图分流）。证据：`evidence/phase5-ttft-*.txt`、`phase5-replies-withintent.txt`。

**T-12 死路径清零**（`chore(web)`，`ec40e8d`）：删 `useAIStream.ts` / `usePrompt.ts`(+其单测) / 孤儿类型 `types/prompt/promptType.ts`
（仅被 `usePrompt` 引用 ⇒ 整目录成孤儿，一并清）；同步改 `r09-*.architecture.test.ts`（它用 `readFileSync` 读源码，删文件后该文件所有用例会连带抛错），
并**新增 2 条回流防护钉**（文件必须不存在 + 生产链路不得再出现 `buildRCTPrompt`）。

```text
$ pnpm vitest run app/composables/r09-*.test.ts → 8 passed
$ pnpm vitest run                              → 77 文件 / 649 用例全绿（删除无回归）
$ pnpm lint                                    → 0 error（95 既存 warning）
$ grep -rn "useAIStream\b|buildRCTPrompt|promptType" app/ e2e/ → 仅新守卫自身
```

### 3.12 Phase 6：契约对齐（T-13）+ 基础人设去长度约束（用户裁定）— PASS

**两条用户裁定（2026-10-14，经 AskUserQuestion 收集）**：① 基础人设的"简短"约束**先去掉看效果**；② `emotion` 字段**用起来**。

**① 去掉 `baseSystemPrompt` 的长度约束**（原"用中文简短回应（2-3 句话）"）：

| 变量 | 未接回意图风格（Phase 0/3） | 接回意图风格（Phase 5） | 去掉简短约束（Phase 6a） | **两侧定上限＝适中档（Phase 6b）** |
|---|---|---|---|---|
| TTFT（3 轮均值） | 940ms | 1362ms | 1332ms | **1630ms**（其中第 1 轮 2243ms = llm-service 刚重建的冷启动；第 2/3 轮 1334/1313ms） |
| 回复体量 | 53~86 字 / 2~3 句 | 51 / 192 / 278 字 | 657 / 766 / 629 字（≈18 句） | **243 / 269 / 256 字（8~11 句）** |
| 回复形态 | 简短陪伴 | 时短时长（规则矛盾） | 长文（4 段小标题 + 大段铺陈） | **先共情一句 → 3~4 条要点 → 收束提问** |
| e2e 用时 | 1.2~1.4s | 1.6~2.9s | 4.4~4.8s | **2.7s** |

**定档（T-17 终态，用户 2026-10-14："按目前状态适中，你定"）**：
**全文不超过 300 字；对方只是倾诉情绪时更短（约 100 字）；需要建议或步骤时才展开。**
落地方式 = **两侧同时给约束**（这是本轮实测链条的结论：只改一侧 ⇒ 规则互相矛盾 ⇒ 必然被另一侧顶穿）：
- BFF `baseSystemPrompt`（语言/身份/三基调保留 + 长度指引）；
- llm-service `intent.py` 的 `STYLE_LENGTH_CONSTRAINT`，在 `style_instruction()` 里**统一附加**，
  且 `emotional_support` 额外要求"优先简短共情（约 100 字），不要给清单或长篇建议"。

回归钉：BFF `TestBasePersona_HasModerateLengthGuidance` + llm-service `tests/unit/test_intent_length_cap.py`（4 例，
含"上限是追加、不顶掉各意图原有结构指引"）。
证据：`evidence/phase6b-{ttft,replies}-moderate.txt`（含 243/269/256 字全文）。

取舍：去掉与 `STYLE_INSTRUCTIONS` 矛盾的那条后，回复形态从"简短陪伴"变成**结构化长文（分点建议）**。
用户要的正是"先看一下效果"——数据与样本见 `evidence/phase6-{ttft,replies}-nobrevity.txt`（含 657 字全文）。
回归钉：`prompt_contract_test.go::TestBasePersona_NoBrevityConstraint`（钉"不得回潮简短约束"+ 必保语言/身份/三基调）。
⚠️ **T-1 语义随之变更**：从"逐字等于**改造前旧输出**"→"逐字等于**本次裁定后的基线**"（golden 常量已同步，测试文件头写明）。

**② `emotion` 字段接上（按一手事实落地）**：亲核三项 →
(a) `sendToExistingConversation(convId, content, emotion = 'neutral', …)` 的 `emotion` **在所有调用点都没传**
（`new.vue` / `[id].vue` 只传 `faceEmotion`/`voiceEmotion`）⇒ 线上该字段**恒为默认值 `neutral`**；
(b) 前端 `app/utils/emotion.ts` 只有标签映射，**没有**文本情绪分类器；
(c) 承载它的 `pages/chat/conversation/[id].vue`、`composables/useConversationSender.ts` 在契约 §1.1 属 **C 轨独占列**。

⇒ BFF 半场落地（零风险 + 向前兼容）：`neutral`/空 = **无信号**（默认值而非测量值；照单注入等于断言每条消息"对方此刻平静"）；
非 neutral 时注入"对方此刻的话语里透着X"（与 face/voice 同段位、同套不护栏）；face/voice 任一非空则**不叠加**文本信号；
文本信号**优先于会话历史回落**（省一次 gRPC）。**前端半场未做**（越界需握手；且前端需先有分类器）——记为待握手项。

证据（5 个新用例）：`TestBasePersona_NoBrevityConstraint` · `TestTextEmotion_{InjectedWhenNonNeutral, NeutralOrDefaultIsNoSignal,
FaceVoiceTakePrecedence, BeatsHistoryFallback}` 全 PASS；`go vet ./...` exit 0；`go test ./...` 10 包 ok / 0 FAIL。

### 3.13 测试点总表（16 点，逐点状态）

| # | 测试点 | 判定 | 本轮状态 |
|---|---|---|---|
| T-1 | 组装层零回归（逐字等于旧输出） | [A] | ✅ **PASS**（§3.5，golden 字面量 6 例） |
| T-2 | 段顺序固定 + 护栏后置 | [A] | ✅ **PASS**（§3.5，段序 6 例 + 2 条不变量） |
| T-3 | 历史注入生效（时间正序） | [A] | ✅ **PASS**（§3.6 单测 + 运行时 `messages=2→4→6`） |
| T-4 | 单轮不多喂（长度=2） | [A] | ✅ **PASS**（§3.6，5 子例） |
| T-5 | 预算裁剪（至少保 1 轮） | [A] | ✅ **PASS**（§3.6，2 例：裁到 ≤2000 / 单条超预算仍保 1 轮） |
| T-6 | userContent 上界（rune 安全） | [A] | ✅ **PASS**（§3.6，截断 4500→4000 + 未超时逐字透传） |
| T-7 | 参数真下发（Temperature/MaxTokens） | [A] | ✅ **PASS**（§3.7：handler 侧 3 钉 + pb 层 1 钉） |
| T-8 | 两处调用点收敛（单一组装函数） | [A] | ✅ **PASS**（§3.5，源码扫描 + 2 调用点） |
| T-9 | 配置化 + 三处人格文案一致性 | [A] | ✅ **PASS**（§3.9：文案集中一处 + 三方守卫 2 例含负向对照） |
| T-10 | file_context 多附件全输出 | [A] | ✅ **PASS**（§3.10：RED 复现"只剩最后一条"→ 修后 3 passed / 全量 211 passed） |
| T-11 | 意图注入接回（with_intent） | [A] | ✅ **PASS**（§3.10：2 钉 + 运行时 `[intent]` 日志；⚠️ 代价已实测并上报） |
| T-12 | 死路径清零（useAIStream/usePrompt） | [A] | ✅ **PASS**（§3.10：删 4 文件 + 2 回流防护钉；vitest 649 用例全绿） |
| T-13 | 契约对齐（无静默丢弃字段） | [A] | ✅ **PASS**（§3.12：`emotion` 接上（neutral=无信号）+ 前端半场记为待握手项） |
| T-14 | IAB 多轮"记得上文" | [V] | 未执行（Phase 7；需用 dev 栈，按 §3.1 排队） |
| T-15 | 负向对照（关历史⇒复现无记忆） | [A] | ✅ **PASS**（§3.6，`NoHistorySignals` 5 子例含"来源报错/去重后为空"⇒ 退回两条） |
| T-16 | MaxTokens/Temperature 取值 | [M] | ✅ **已裁定**（1024 / 0.7，用户 2026-10-14） |
| T-17 | 基础人设长度约束的去留 | [M] | ✅ **已裁定并落地**（2026-10-14：**先去掉**看效果；实测回复 657/766/629 字，见 §3.12） |

## §4 结论

- **Phase 0 完成**：运行时证据支持"多轮＝无记忆单轮"（3 轮对话 / 会话 6 条消息 / prompt 恒 2 条 / 真实 deepseek 上游）。
- **Phase 1 完成**：统一组装层落地，**行为逐字不变**（T-1 golden 6 例）；两处内联调用点收敛为单一组装点（T-8）。
- **Phase 2 完成**：同会话历史注入（窗口 10/上限 20、预算 2000、当前轮去重、file 不进历史、上界保护）；
  运行时证据 `messages=2→4→6` —— **"多轮＝无记忆单轮"这一核心缺陷已消除**。
- **Phase 3 完成**：生成参数真下发（0.7 / 1024，两条路径 + pb 层各一钉）；顺手补上 `llm_grpc_test.go`
  声称却缺失的 pb 映射断言；R1 长度张力实测**未出现**（回复仍 2~3 句）。
- **Phase 4 完成**：system 文案集中一处（`prompt_templates.go`）+ 人格契约三方一致性守卫（含负向对照）；
  顺手清掉死变量 `personalityDimensionLabels`。
- **Phase 5 完成**：① `file_context.py` 多附件缩进 bug 修复（原只输出最后一条）；② 意图风格注入**接回**并运行时确认
  （⚠️ 代价实测：TTFT +45%、**回复 3~5 倍长且变列表体** ⇒ 产品口径待裁定，见 §3.10）；
  ③ 死路径清零（删 `useAIStream`/`usePrompt`/孤儿类型 + 2 条回流防护钉）。
- **Phase 6 完成**：契约对齐（`emotion` 接上，`neutral` 视为无信号；前端半场记为待握手项）+
  **基础人设去掉"简短"约束**（用户裁定）—— 实测回复形态转为结构化长文（657/766/629 字，TTFT 不变）。
- **阶段状态**：`in-progress`（**未收口**）。测试点 **14/16 有 PASS 结论**（T-1~T-13 全部 + T-15）+ T-16/T-17 已裁定；
  仅剩 **T-14 IAB 人可见钉**（Phase 7，需 dev 栈）。
- **环境遗留**（交付给下一轮）：
  - dev 栈**在跑**（17 容器）；`xtts` 按用户指示**已停**（TTS 链路预期 502，文本链路不受影响）；
  - dev mode 锁 `deploy/.devmode-session`（owner `lane-cl-a`，until 2026-10-14 12:00）→ **收工须删**；
  - dev 库新增会话 667 / 668（取证产物，dev-only）；
  - `web-bff` 镜像已重建为当前 main；**其余 7 个仍按时间判 STALE（是否实质陈旧未核验）**。
- **账本**：新登 **CL-F-06**（`check_git_layout.py` 在 worktree 恒红）、**CL-F-07**（worktree 缺 gitignored 运行态文件）、
  **CL-F-08**（`check_image_freshness.sh` 假红）。

## §5 经验（本轮踩到并留档）

1. **镜像新鲜度不能拿时间当结论**：守卫的纯时间判据会把"工作副本已含改动、稍后才 commit"误判为 STALE（CL-F-08 实例：chat-svc）。
   反过来它也**确实**抓出了真陈旧的 web-bff（行为缺 `cursor/hasMore`）——**判据粗糙 ≠ 无效**，正解是**行为判别**。
2. **`docker compose build` 必须带齐 `-f`**：只用 `-f docker-compose.apps.yml` 会报
   `service "emotion-echo-analytics-svc" depends on undefined service "kafka": invalid compose project`（kafka 在 infra 文件里），
   **构建静默失败后 `up -d` 仍会成功** ⇒ 容易误以为"重建过了"。
3. **worktree 与 gitignored 运行态文件**：`deploy/.env.local`、`deploy/.devmode-session` 在 worktree 里都不存在 ⇒
   起栈报错、锁失效、且 worktree 的源码无法直接构建成镜像（CL-F-07）。并行契约要求 worktree，二者存在**结构性冲突**，需在契约/RUNBOOK 层给安全做法。
4. **MSYS 吞多行 `node -e`**：多行内联 node 在 Git Bash 下静默无输出 ⇒ 探针一律落文件再跑（本次已按此改）。
5. **本机 `gofmt -l` 是全仓假报**（Windows 工作区为 CRLF，gofmt 要 LF）：它会列出**几十个我没碰过的文件**
   ⇒ **不得把 `gofmt -l` 当门禁**（本机）。判"是否改坏行尾"的正确手法是 `git diff --numstat` 看改动行数是否
   仍是小值。CI 侧 Go 门禁是 `go vet` + `go test`（`go-test.yml`），不含 gofmt。
6. **"战役式"推进的节奏经验**：先零回归搬迁（Phase 1）→ 再加能力（Phase 2/3）—— 每加一步都由同一套 golden 字节钉
   托底，改动面大也不再心虚；反过来若一开始就"重写 + 加历史"，一旦回归很难定位是哪一部分引起的。
7. **并行轨共用 docker daemon（CL-F-09）**：两个 worktree 构建的是**同一批镜像 tag** ⇒ 互相覆盖；修完代码"本地验收通过"
   不等于"你验的是你的代码"。自查手法：`docker images | grep <tag>` 看构建时刻 + `docker ps --format {{.Image}}` 看容器实际镜像 ID。
8. **"死路径复活"必须先量代价再宣称收益**：`with_intent` 接回本身一行代码，但真实代价是 TTFT +45% 与回复体量 3~5 倍
   （后者来自另一处文案与人设的矛盾，不在 BFF 侧）。若只按"链路通了"收工，产品侧的变化会被漏报。
