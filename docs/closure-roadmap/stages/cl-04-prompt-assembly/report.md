---
stage: cl-04
title: Prompt 组装层 — 执行记录
type: report
status: in-progress
created: 2026-10-14
last-updated: 2026-10-14（Phase 0 基线 + Phase 1 统一组装层 + **Phase 2 历史注入**（8/16 测试点 PASS）；阶段未收口 —— 余 6 点 + 人可见钉在 Phase 3~7）
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

### 3.7 测试点总表（16 点，逐点状态）

| # | 测试点 | 判定 | 本轮状态 |
|---|---|---|---|
| T-1 | 组装层零回归（逐字等于旧输出） | [A] | ✅ **PASS**（§3.5，golden 字面量 6 例） |
| T-2 | 段顺序固定 + 护栏后置 | [A] | ✅ **PASS**（§3.5，段序 6 例 + 2 条不变量） |
| T-3 | 历史注入生效（时间正序） | [A] | ✅ **PASS**（§3.6 单测 + 运行时 `messages=2→4→6`） |
| T-4 | 单轮不多喂（长度=2） | [A] | ✅ **PASS**（§3.6，5 子例） |
| T-5 | 预算裁剪（至少保 1 轮） | [A] | ✅ **PASS**（§3.6，2 例：裁到 ≤2000 / 单条超预算仍保 1 轮） |
| T-6 | userContent 上界（rune 安全） | [A] | ✅ **PASS**（§3.6，截断 4500→4000 + 未超时逐字透传） |
| T-7 | 参数真下发（Temperature/MaxTokens） | [A] | 未执行（Phase 3） |
| T-8 | 两处调用点收敛（单一组装函数） | [A] | ✅ **PASS**（§3.5，源码扫描 + 2 调用点） |
| T-9 | 配置化 + 三处人格文案一致性 | [A] | 未执行（Phase 4） |
| T-10 | file_context 多附件全输出 | [A] | 未执行（Phase 5） |
| T-11 | 意图注入接回（with_intent） | [A] | 未执行（Phase 5） |
| T-12 | 死路径清零（useAIStream/usePrompt） | [A] | 未执行（Phase 5） |
| T-13 | 契约对齐（无静默丢弃字段） | [A] | 未执行（Phase 6） |
| T-14 | IAB 多轮"记得上文" | [V] | 未执行（Phase 7） |
| T-15 | 负向对照（关历史⇒复现无记忆） | [A] | ✅ **PASS**（§3.6，`NoHistorySignals` 5 子例含"来源报错/去重后为空"⇒ 退回两条） |
| T-16 | MaxTokens/Temperature 取值 | [M] | ✅ **已裁定**（1024 / 0.7，用户 2026-10-14） |

## §4 结论

- **Phase 0 完成**：运行时证据支持"多轮＝无记忆单轮"（3 轮对话 / 会话 6 条消息 / prompt 恒 2 条 / 真实 deepseek 上游）。
- **Phase 1 完成**：统一组装层落地，**行为逐字不变**（T-1 golden 6 例）；两处内联调用点收敛为单一组装点（T-8）。
- **Phase 2 完成**：同会话历史注入（窗口 10/上限 20、预算 2000、当前轮去重、file 不进历史、上界保护）；
  运行时证据 `messages=2→4→6` —— **"多轮＝无记忆单轮"这一核心缺陷已消除**。
- **阶段状态**：`in-progress`（**未收口**）。测试点 **8/16 有 PASS 结论**（T-1/T-2/T-3/T-4/T-5/T-6/T-8/T-15）+ T-16 已裁定；
  余 6 点（T-7/T-9/T-10/T-11/T-12/T-13）+ 人可见钉 T-14 在 Phase 3~7。
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
