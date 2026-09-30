---
stage: e2e-23
title: 健康检查与服务发现
executed: 2026-09-29
status: done
environment: dev 模式（28 容器；compose.dev.yml + --env-file .env.local，profile dev + ai + obs）
---

# E2E-23 执行记录

> **本阶段判 `done`（2026-09-30，用户批准）。** 40 个测试点：**PASS 38 / FAIL 0 / BLOCKED 0 / N/A 2**，
> 其中 30+ 项有**运行时或行为证据**（含破坏性实验与 IAB 浏览器实测）。
> **第二方核对已完成**（2026-09-30，独立跑命令 + 亲自复现负向对照），抽查的 PASS 全部成立；
> 核对方抓出的治理失真（含 3 项**反向失真**——已完成的工作被记成未做）已全部修正，见 §9.1。
>
> **2026-09-30 第三轮复核**（用户追问"这一阶段做了很长时间都没 done，到底哪些地方出问题"触发）
> 查出**四类**与事实相反的陈述并已全部更正，测试点 BLOCKED 由 1 归零：
> ① 报告头与本文件 §2/§0 之外，**§5 账本对账是与 §2、与账本同时相反的第二份副本**；
> ② §0「唯一真相源」里躺着一条**失实条目**（称 #40 需停 Nacos 实跑，而 §2 记 #18~#23 早已全 PASS）；
> ③ 「门禁是否在拦」被误列为本阶段收口障碍——**账本 F-171 自登记起就归 E2E-03**；
> ④ **#40 本身也已全部完成**（`git log -S` 证实 ADR 更正由 `49b5daf` 落地），改判 PASS。
> 未完成项由 **7 项收敛为 1 项**（第三轮复核 + 用户裁定 A + 第二方核对整改后）。
> **归属本阶段且未闭环的账本已归零**（F-107 / F-156 / F-163 全部翻 ✅）——
> 按 RUNBOOK §7 #9 与审计 A5，阶段**已具备判 `done` 的账本条件**，只等用户批准。
> 详见 §0「订正依据」四条。

## 0. 未完成清单（唯一真相源 · 收口时必须逐条销账）

> **本节已清空 —— 2026-09-30 阶段判 `done`。**
>
> 依据 [anti-patterns.md](../../anti-patterns.md) **AP-14**（同一事实只允许一处定义）：
> 本节是本阶段"没做的事"的唯一权威清单。它曾长期是**虚高**的——
> 7 项里有 3 项不属于本阶段（仓库级门禁配置，归 E2E-03）、
> 1 项**失实**（称 #40 需停 Nacos 实跑，而 §2 记录 #18~#23 早已全部 PASS）、
> 1 项**早已完成**（#40 的 ADR 回填，commit `49b5daf` 早已落地）。
> 虚高的清单让本阶段被"自己设的完成条件"卡了很久，而真实遗留只有 4 条。
>
> **本阶段"能不能 done"的机器判据只有两条**（RUNBOOK §7 #9 + 审计 A5）：
> ① §2 的 40 个测试点无 BLOCKED；② §5 账本对账中归属本阶段且未闭环的条目为空。
> **两条均已满足**，故本节清空。

### 销账过程（供下轮对照方法论，不重列结论）

| 轮次 | 未完成项 | 处置 |
|------|---------|------|
| 初始 | 7 项 | — |
| 第三轮复核 | 5 项 | 剔 3 项不属本阶段（门禁配置 → E2E-03）、1 项失实（#40 需停 Nacos 实跑）；#40 经 `git log -S` 核实**早已完成** → BLOCKED 归零 |
| 用户裁定 A + 核对整改 | 1 项 | F-163 按方向 (a) 删除 healthcheck server 侧包装后翻 ✅；F-107/F-156 续约残留 TDD 修完翻 ✅ |
| F-96 实测 | 0 项 | 用户裁定走"冷启动实测"（而非凭推理销账）⇒ **实测挖出真缺陷并当场修掉**，见下 |

### 已移出本阶段（上一版误列在此）

| 上一版编号 | 事项 | 移出去向与理由 |
|-----------|------|---------------|
| 旧 T-1 / T-2 / T-3 | 分支保护实测、`static-guards` 与 `文档守卫总闸` 是否在拦 | **归 E2E-03（CI 门禁）**，账本 E2E-F-171 本身就把归属写作 E2E-03。这是**仓库级门禁配置**问题，与"健康检查与服务发现"这一阶段的测试点无关；把它算作 E2E-23 的未完成项，等于让本阶段被一个它不拥有的问题卡住。**且该结论本身尚未被可靠证据坐实**——见下方"订正依据"第 ③ 条 |
| 旧 T-1（本轮新版） | 测试点 #40 的 ADR 文本回填 | **早已完成**（commit `49b5daf`）。本轮写这一条时**自己也踩了同一个坑**：直接采信账本 F-157 的"ADR 文本回填未做"，写完才去 `git log -S` 核实，发现该更正节早在 2026-09-29 就已落地。⇒ 见下方"订正依据"第 ④ 条 |

> **订正依据（四条，均为可复跑的核查，不是判断）**
> ① **旧 T-6 是失实条目。** 它写"测试点 #40 需批准停 Nacos 做破坏性实验，判据 = #18~#23 实跑"。
> 但同一份文件 §2 记录 **#18/19/20/21/22/23 全部 PASS 且带运行时证据**（retry 序列、停机 100s 后自愈、
> 501 beat 实测），#40 的真实内容是"文档漂移修正"。该破坏性实验早在 2026-09-29
> 用户批准后就跑完了（见 PR #130 评论"更新：破坏性实验轮结果"）。
> **一个自称"唯一真相源"的清单里躺着一个与本文件 §2 直接矛盾的条目**——这正是 AP-14，
> 而上一版配套的新守卫 `check_stage_todo_section.sh` **查不出来**：它只校验"有没有 §0、
> 声明条数与实际是否相等、每条有无责任人与判据"，**不校验条目内容是否为真**。
> 守卫通过 ≠ 清单为真，这是本轮必须记下的教训。
> ② **旧 T-1~T-3 把一个问题的三种说法拆成了三行**（同一件事、同一判据、同一责任人），
> 却让条数从 1 虚增到 3，**放大了"未完成"的观感**。
> ③ **旧 T-1~T-3 的证据链本身不可靠。** 唯一依据是 `mergeable_state`——GitHub 文档里
> `unstable` 的定义是"有非必需检查失败、**分支未受保护**"，而该字段是**惰性重算**的
> 派生值。同一份 §9.9 已记录执行者在**同一问题**上用更好证据连错两次
> （M-1 查错 API 端点、M-2 用被无障碍树裁剪的列表做完整性判断）。
> **在自身误判率已被记录 2 次的情况下，不应再用同一手段下第 3 次否定结论。**
> 正确做法是登录后开 PR 实测或点一次合并——那需要用户，故不作为本阶段条目。
> ④ **账本 F-157 与 §2 #40 的备注都写"ADR 文本回填未做"，两条都过期。** 逐条核实：
> `git log -S "更正（2026-09-29，E2E-23 实测）"` → 命中 commit **`49b5daf`**（提交标题就是
> "docs(e2e-23): #40 文档级联 —— ADR 更正"）；`grep -rn NACOS_REFRESH_MS` → **0 命中**（与更正节所述一致）。
> 另 §2 #40 的阻塞理由"RUNBOOK §2.4 待 #23 验证后才能改"，而 #23 已 PASS、RUNBOOK `:125` 已按 D-30 更正。
> ⇒ **#40 的四项内容全部完成，本轮已由 BLOCKED 改判 PASS，阶段 BLOCKED 归零。**
> **这条订正的元教训**：我在写"订正后的清单"时，**又踩了一次同型坑**（采信一条未核实的账本状态）。
> 正确做法是先 `git log -S` / `grep` 核实再落笔——**账本本身也是待核实的对象，不是事实源**。

### 已销账（本轮做完的，勿重复跟进）

| 事项 | 结论 | 证据 |
|------|------|------|
| **测试点 #40**（阶段唯一 BLOCKED） | ✅ **由 BLOCKED 改判 PASS** | ① `git log -S` 确认 ADR 更正节由 `49b5daf` 落地；② `grep -rn NACOS_REFRESH_MS` 0 命中，与更正节一致；③ RUNBOOK `:125` 已按 D-30 更正；④ 账本 F-107 已带运行时证据翻新 |
| **T-3 重建 ai-svc 镜像 + 复验 F-170 运行时** | ✅ **已完成，有 dev 库上的前后对照** | ① 重建 `emotion-echo/ai-svc:v0.1.9` + `--force-recreate` 该容器（**未重启整栈**），`/health` → `{"status":"ok","dbOk":true}`；② 查明根因是**索引为 partial unique**（`WHERE upload_id <> '__legacy__'`）而旧二进制发的是**不带谓词**的 `ON CONFLICT (upload_id)`；③ **dev 库前后对照**：旧 SQL → `ERROR 42P10 there is no unique or exclusion constraint matching the ON CONFLICT specification`；新 SQL（带 `TargetWhere`）→ `INSERT 0 1`；同 `upload_id` 重复插入 → `INSERT 0 0`（幂等）；落库 1 行且 `primary_emotion` 仍是 `joy` 未被 `sad` 覆盖（`DO NOTHING` 语义正确）；④ 真 Postgres + 真迁移的集成测试 `TestFaceEmotionRepo_Integration_UploadIDDedup` / `TestVoiceEmotionRepo_Integration_UploadIDDedup` 全绿；⑤ 探针行已 `DELETE` 并复验 `leftover = 0` |
| **T-1 账本 F-107 / F-156 的续约可观测性** | ✅ **已完成（TDD + 负向对照）** | ① `nacos_beat.go` 的 `if failCount <= 3` 改为**节流而非封顶**（第 1 次必打、其后每 12 次一次），并在连续失败达 12 次时加一条**说明后果**的日志（该端点已被 Nacos 3.x 移除、心跳实际由 SDK UpdateInstance 承担）；② `nacos_register.go` 的 `_, _ = r.client.UpdateInstance(...)` 改为捕获 error → WARN（节流）+ 恢复 INFO；③ 回归钉 `nacos_renewal_observability_test.go` **5 例全绿**，含性质化断言"不存在长度 ≥ 12 的完全静默窗口"（前 1000 次失败全扫描）；④ **负向对照实测旧代码：49 次失败只打 3 条 WARN、续约错误 0 条、恢复 0 条**；回退修复后 4/4 转 RED；⑤ `go vet` + shared 全模块 `go test ./...` 全绿。**边界**：`>90s` Nacos 宕机未测；HTTP beat 通道**未删除**（Nacos 2.x 下可能可用，本轮只让它可观测）；`-race` 本机跑不了（Windows `0xc0000139`），须 CI 复核 |

| 事项 | 结论 | 证据 |
|------|------|------|
| F-164 坏相对链接 | ✅ 7 处全修 | `decisions.md` / `roadmap.md` 复扫 0 坏链 |
| F-165 图例压圆环 | ✅ 已修**且已浏览器复验** | 截图 `screenshots/41-…png`、`42-…png`；几何断言 9/9 |
| F-167 build tag 盲区 | ✅ 3 处全修，守卫转无条件门禁 7/7 GREEN | `test_integration_tag_compiles.sh` |
| F-168 ai-svc 集成测试 | ✅ 14 红/0 绿 → 全绿 | 共享 fixture 跑真实迁移，68.7s |
| F-169 浏览器复验缺口 | ✅ 已补 | 主机 `pnpm build` + 同源 3000 生产构建 |
| F-170 多模态入库 100% 失败 | ✅ 代码已修（运行时待 T-4） | dev 库实测复现 42P10 → 修后集成测试转绿 |
| AP-11 门禁实测 | ⚠️ **做了，但结论不可采信** | 两次对照 `mergeable_state` 均为 `unstable`；该字段是惰性重算的派生值，且本文件 §9.9 已记录同一问题连错两次（M-1/M-2）⇒ **不作为结论**，问题移交 E2E-03 / E2E-F-171 |

## 1. 环境基线

- **启动命令**：dev 栈在本轮开工前已运行（28 容器），本轮按需 `--force-recreate` 单服务重建
- **dev 覆盖项声明**：`BFF_DEV_RETURN_CODE=1`（验证码回显）、`BFF_TRUST_APISIX=true`、CORS 含 localhost
- **本轮验证范围**：仅 dev 配置。prod 语义（`compose.prod.yml`）本机无法运行，相关结论均标 N/A 并写明理由
- **Nacos 注册基线**：`count: 6`（chat / ai / assessment / analytics / web-bff / user）
- **容器状态**：结束时 6 个 Go 服务 + APISIX + 6 个观测服务全部 `(healthy)`

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 6 服务 `/health` 可达且 schema 一致 | `[A]` | PASS | `docker exec emotion-echo-web-bff sh -c 'wget -qO- ...'` 五个服务均返 `{"status":...,"dbOk":...}`，仅 `:8894` 宿主可达 | 宿主直连 5 个端口返 `000`（端口未映射，见 plan §0 F-k）⇒ 探针须走容器网络 |
| 2 | 停 Postgres ⇒ user-svc `status` 变化 | `[A]` | PASS | 停 postgres 后 `wget -qO- :8888/health` → `{"status":"degraded",...,"dbOk":false}` | 修复前为 `"ok"`（`healthlogic.go:41` 硬编码），RED 实测 expected `degraded` / actual `ok` |
| 3 | 同上验证 analytics / assessment / ai | `[A]` | PASS | `go test ./internal/logic/ -run TestHealthLogic -count=1` 三服务均 `ok`（各 1.4~1.6s），表驱动覆盖 ok/degraded | 原缺陷同为硬编码（各 `:40`） |
| 4 | BFF 下游全挂 ⇒ HTTP 码与 `status` | `[A]` | PASS | `TestHealthHandler_D29LivenessAlwaysOK` + `TestHealthReadyHandler_D29ReadinessReflectsDownstream`（4 子用例全绿） | 修复前 degraded 却恒 200（`health_handler.go:109`） |
| 5 | `/health` 覆盖 Redis 依赖 | `[A]` | PASS | **运行时前后对照**：基线 `deps={nacos:ok,redis:ok}`；**停 Redis** → `status=degraded`、`redis=unhealthy`（`dial tcp: lookup emotion-echo-redis: i/o timeout`）、**ready → 503**；恢复 → 双 ok、ready 200、容器 healthy | 修法：拆出 `buildRedisClient()` 由 main 持有，**authLockStore 与 /health 探针共用同一 client**（第一版自建 client 会多开连接池且永不关闭，已修） |
| 6 | BFF 未注册 Nacos ⇒ `/health` 报 degraded | `[A]` | PASS | `NacosRuntime.registered atomic.Bool`：注册成功后置 true、`Close()` 置 false；探针接入 `/health` 的 `deps.nacos`；Nacos 未启用时**不注册该探针**（避免误伤无 Nacos 的部署）。单测 6 例 + 负向对照 | "BFF 掉出注册"属破坏性场景，**运行时未实测**（如实记录） |
| 7 | chat-svc `EventPublisher` 是真连还是仅判 nil | `[A]` | PASS | `healthlogic.go:43-52` 锁死现状并加注释：「此处只判非 nil，并不真连 Kafka」 | 现状 = **仅判 nil**。plan §2 A2 要求"二选一不得留模糊"，本轮选择**明确记录现状**而非改造真连（后者需 Kafka 连接探测设计，超出本轮） |
| 8 | `/health/ready` 存在且不通返 503 | `[A]` | PASS | `go test ./pkg/middleware/ -run HealthReadyRoute -count=1 -v` → 6 子用例 `PASS`；`go test ./internal/handler/ -run D29Readiness` → `ok 0.632s` | |
| 9 | compose healthcheck 指向 ready | `[A]` | PASS | `bash scripts/test_healthcheck_readiness.sh` → `PASS: 25  FAIL: 0` + `GREEN`（6 compose + 1 一次性容器 + 6 Helm 静态 readiness + 6 Helm 静态 liveness + 6 Helm 渲染）；`docker inspect emotion-echo-user-svc --format '{{json .Config.Healthcheck.Test}}'` → `["CMD-SHELL","wget --quiet ... /health/ready || exit 1"]` | 负向对照：改回 `/health` → RED |
| 10 | `seed.sh` 自带探针在依赖降级下行为正确 | `[A]` | PASS | **运行时前后对照**：正常态 5 个 `upstream OK: .../health/ready`；**停 Postgres 后** `FATAL: upstream emotion-echo-web-bff:8894/health/ready not healthy` → **exit=2 中止**（改前探恒 200 的 liveness，**这一步会通过**）；恢复后 11 处 OK | 原探针打 `/health`（liveness 恒 200）⇒ **结构上不可能发现降级**，形同虚设。已改指 `/health/ready` |
| 11 | user-svc / chat-svc 补齐 `/health` handler 测试 | `[A]` | PASS | `go test ./internal/handler/ -run D29Readiness -count=1` → `ok`；`go test ./internal/logic/ -run TestHealthLogic -count=1` → `ok 0.576s` | 二者此前**零** `/health` 测试 |
| 12 | 5 服务 gRPC health service 名正确 | `[A]` | PASS | `bash scripts/test_grpc_health_shutdown.sh` → `PASS: 30  FAIL: 0` + `GREEN：5 个服务的 gRPC health 均会在停机时翻转为 NOT_SERVING`；`cd emotion-echo-ai-svc && go test ./internal/grpcserver/ -run TestGrpcHealth -count=1` → `ok  emotion-echo-ai-svc/internal/grpcserver  25.686s` | **🔴 第二方核对推翻原判定，已 TDD 修**：原判"名正确"是错的 —— 5 个服务注册的 per-service 名是 `emotion.User` / `emotion.Chat` / `emotion.Analytics` / `emotion.Assessment` / `emotion.AI`，**这些名字在 proto 里根本不存在**（真实全名 `emotion_user.v1.UserService` 等，取自 `emotion-echo-shared/*_grpc.pb.go` 的 `ServiceName`）。后果：真实 gRPC 客户端用真名 `Check()` 拿到 `NOT_FOUND`，**per-service 健康实际不可查询**；原测试用同一字面量断言 ⇒ **自证循环、永远绿**。处置：守卫加第 6 条（RED `PASS: 25 FAIL: 5`）→ 改 5 个 `server.go`（GREEN `30/0`）→ 补 `TestGrpcHealth_RegisteredNameMatchesProtoServiceName`（负向对照：改回 `emotion.AI` 立即 FAIL）|
| 13 | 优雅停机翻 `NOT_SERVING` | `[A]` | PASS | `--- PASS: TestGrpcHealth_FlipsToNotServingOnShutdown (5.01s)`，起真实 gRPC server + shared healthcheck 客户端 | 修复前**无任何 NOT_SERVING 写入**（plan §0 F-d） |
| 14 | 恢复后翻回 `SERVING`（`Resume()`） | `[A]` | N/A | — | **2026-09-30 核实后事实已更正**：原记"plan B1 要求接线、实际只接了 `Shutdown()`"**与代码不符**—— ① 5 个服务的 gRPC 端**根本不用** `shared/pkg/healthcheck` 的 server 侧，它 import 的是上游 `google.golang.org/grpc/health`；② 承载 `Shutdown()`/`Resume()` 的那个类型**在生产里一次都没被实例化**（全仓 grep 只命中它自己包的测试）；③ 故两者**都没有生产调用方**。plan B1 想要的"停机翻 NOT_SERVING"**早已由 `MarkShuttingDown()` 达成**。**用户 2026-09-30 裁定方向 (a)：整个 server 侧包装已删除**（含 `Resume()`），client 测试改为对上游 server 跑，回归钉 `scripts/test_healthcheck_no_dead_server.sh` 6/6 |
| 15 | `ai-svc` 客户端按状态分流 | `[A]` | PASS | **定案：只做启动期门禁，请求期不分流**。依据 `grpc_analyzer.go:91-99`：`NewGRPCAnalyzer` 内一次 `WaitForReady`，不通过则关连接返错；此后业务 RPC 不再查 health。测试：NOT_SERVING 时构造必须失败 + 对照组（否则可能因"连不上"假通过）+ 负向对照（绕过门禁立即红） | 不一定是缺陷（每请求探一次代价高），但**必须写进文档**，否则运维会误以为"health 翻 NOT_SERVING ⇒ 客户端自动绕开" |
| 16 | web-bff 无 gRPC server 属设计现状 | `[A]` | N/A | — | 陈述性测试点，无可断言行为。已记入 plan §2 B3 |
| 17 | 6 服务注册齐全（`count:6`） | `[A]` | PASS | `curl .../ns/service/list?...namespaceId=emotion-echo-dev` → `count: 6` | |
| 18 | 停 Nacos ⇒ 重启 BFF ⇒ 自愈（**F-107 复现**） | `[A]` | PASS | `docker logs emotion-echo-web-bff` 实测 retry 序列 `attempt 1/10 → 5/10`（退避 2→4→8→16→30s 与代码一致）；Nacos 恢复后日志 `19:02:51 attempt 2/10 → 19:02:56 Starting web-bff`（**5s 内自愈**）；`curl /ns/service/list` → `count: 6`；网关 login → 400（路由通） | F-107 启动期修复获**运行时证据**，账本已翻 |
| 19 | 5 服务 fail-fast 退出码 | `[A]` | PASS | `docker logs emotion-echo-user-svc` 实测 `boot failed (fatal): [nacos] WaitForNacos: context deadline exceeded` ×5；`docker inspect` → `RestartCount=2`（on-failure 拉起）；Nacos 恢复后回 `(healthy)`、重新注册 | **新发现（plan H4 获答）**：Nacos 长期宕机时构成**崩溃-重启打鸣**（fatal 每 ~60s 一次）——dev 可接受，prod 需注意 compose restart 策略 |
| 20 | 运行期掉线 ⇒ 重注册 | `[A]` | PASS | **干净实验**：记录 6 服务 `StartedAt` → 停 Nacos 100s → 起 Nacos 90s → `count: 6`，全程**未重启任何业务服务** | **计划期"必 FAIL"预判被推翻**（AP-06 自纠）：grep 无显式重连代码，但漏了两条隐性通道——SDK gRPC 自动重连 + `Heartbeat()` watcher 每 5s `UpdateInstance`（upsert）。边界：watcher `_, _ =` 吞错误，SDK 若死透则静默失效；>90s 的宕机未测 |
| 21 | 心跳协议统一 | `[A]` | PASS | BFF 日志实测 `BeatInstance failed ... beat HTTP 501: no such api:POST:/nacos/v1/ns/instance/beat` ×9（Nacos 3.x 无该端点）→ 每次静默降级 SDK；5 服务日志 0 次同类告警（一直走 SDK） | **新发现**：BFF 的 HTTP beat 协议**从未生效过**——功能上等价（都靠 SDK），但 ① beat 代码是死的 ② `failCount>3` 后连 WARN 都不打（`nacos_beat.go` 的 `if failCount <= 3`）③ 观测盲区 |
| 22 | `NACOS_REQUIRED` prod 实情 | `[A]` | PASS | 全仓 grep → **0 命中**；`compose.prod.yml` 为 ADR-20 空壳占位（故意不填值） | 结论：**从未被任何编排声明过** ⇒ 促成 D-31 |
| 23 | Nacos 重启 ⇒ APISIX 节点自动跟随 | `[A]` | PASS | `docker restart emotion-echo-nacos` → 服务重注册 `count: 6` → **未重跑 seed、未碰 Admin API** → 网关 login 400（路由通）。反向：停 user-svc 75s 后 Nacos 实例数→0 | **D-30 落定**：RUNBOOK §2.4「重建服务后必须重跑 apisix-seed」判为**误导性文档**并已更正。边界：user 路由在实例摘除后仍 401（非 503）⇒ APISIX 摘除有滞后，属 F-154/E2E-25 的主动健康检查范围 |
| 24 | `GetConfig` 首帧失败仍能继续 | `[A]` | PASS | `go test . -run TestBootNacos_Ops -count=1` → `ok emotion-echo-chat-svc 1.083s`（4 例）：首帧失败**仍成功启动**、注册照常、ops 保持启动值；首帧成功则真应用；敏感 key 被清洗；`deps.ops` 为 nil 时退化为"只记录不应用" | 依据：`IsHardBootError` 把 `Register`/`WaitForNacos` 归 hard，**GetConfig 不在其列** ⇒ 缺配置是正常状态（新环境还没推过 ops） |
| 25 | APISIX 补 healthcheck | `[A]` | PASS | `docker ps` → `emotion-echo-apisix Up (healthy)`；探针命令**双向实测**（通→0、不通→1） | 该镜像内 wget/curl/nc/busybox **全缺**，只能用 bash /dev/tcp 测 9080 |
| 26 | 观测栈 + skywalking 补 healthcheck | `[A]` | PASS | **16 个常驻服务全部补齐**：`bash scripts/test_obs_healthchecks.sh` → `PASS: 18  FAIL: 0`（16 常驻 + 2 一次性）；`docker ps` 逐个确认 grafana/loki/prometheus/alertmanager/kafka-exporter/promtail/**sw-oap/sw-ui/obs-mock-receiver** 均 `Up (healthy)` | **首轮遗漏 skywalking-oap/ui + obs-mock-receiver 三个**（plan D2 点名），由第二方核对 C-6 抓出并补：端点先逐个实测可达才写入（OAP 无 `SW_HEALTH_CHECKER` 故用 `:1234/metrics`；UI 探 `/`；receiver 探 `/received`） |
| 27 | **db-migrate 冷启动 `Exited(0)`** | `[A]` | PASS | **前后对照**：修复前 `ExitCode 1` + `FATAL: Postgres 30s 内未就绪`；修复后 `ExitCode 0` + 日志以 `全部迁移应用完成，共 31 个文件` 结尾 | **F-151 闭环**，账本已翻状态 |
| 28 | `migrate.sh` 有负向测试 | `[A]` | PASS | `scripts/test_migrate_pg_wait.sh` 4/4；负向对照：删掉递增退避 → RED | |
| 29 | `dev-up.sh` 批次等待语义 | `[A]` | PASS | `bash scripts/test_devup_batch_waits.sh` → `PASS: 3  FAIL: 0` + `GREEN`；抽出 `wait_healthy` 实机跑四条路径（redis/apisix/postgres/BFF）→ 全部 `exit=0`、0 秒返回 | 挖出**更深缺陷**：`wait_healthy` 对设了 `container_name` 的服务恒失效（见 §3） |
| 30 | chat-svc 4 个 Outbox 参数可热更 | `[A]` | PASS | 运行时实测：**不重启服务**推 Nacos → `[hot-reload] … changed, 86 bytes` → `ops applied via hot-reload: max_attempts=21 sent_retention=6d dead_retention=8d cleanup=300s`；删配置重启后回落 yaml 默认 `100/7/30/3600`。单测 5 例，负向对照（relay 忽略 Ops → 立即红） | 附带修出 `CleanupIntervalS` 也是死配置（ticker 启动时固化，改了不生效）；根因修正：F-155 原文「HotReload 全线关闭」实为**编排层显式压制**（`apps.yml:144` 的 `NACOS_HOT_RELOAD: "false"` 覆盖 yaml，因 env 优先于 yaml），非「没人设」 |
| 31 | ai-svc 9 个参数可热更 | `[A]` | PASS | **运行时 9/9**：不重启推 Nacos → `ops applied via hot-reload: llm=11s fer=33s sv=55s xtts=1m39s lang=ja speed=1.35 retries=7 breaker=12/1m15s` | 途中抓到 P1 的**误判缺陷**：`llm_timeout`/`kafka_max_retries` 被敏感词根误删（词根含 `llm`/`kafka`）⇒ 拆成凭据类/组件类，组件类仅点号命名空间下判定 |
| 32 | analytics-svc `MaxRetries` 可热更 | `[A]` | PASS | **运行时**：启动 `max_retries=3` → 不重启推 8 → `ops applied via hot-reload: max_retries=8` | 修法：consumer 与 handler 改为**共享同一个 atomic 容器**（此前是构造期拷贝两份 int，热更只改到一份） |
| 33 | P1 敏感字段白名单（负向断言） | `[A]` | PASS | `go test ./pkg/configcenter/ -count=1` → `ok`：`SanitizeOpsContent` 剔除敏感 key（含嵌套 `db:` 下 `password` 子项）、保留合法参数；**负向对照**：禁用词根匹配 → 5 个子用例立即红。**运行时**：ai-svc 启动日志 `剔除敏感 key: [...]` 与真实 ops 配置正常应用 | ⚠️ 本行曾误记 BLOCKED（commit `4d4b1f9` 已实现，第二方核对抓出该**反向失真**）；实施期还抓到并修掉一个**误判**——词根含 `llm`/`kafka` 把 `llm_timeout`/`kafka_max_retries` 这类合法参数也剔除了 |
| 34 | P2 `LLM.Timeout` 接线 bug | `[A]` | PASS | `go test . -run LLMFuser -count=1` → `ok`：`TestNewLLMFuserForConfig_TimeoutReachesHTTPClient`（5s/30s 实际到达 `http.Client`）+ `TestLLMFuser_HTTPTimeout_DefaultFallback`（0 时兜底 3s）。实现：`main.go` 抽出 `newLLMFuserForConfig(...)` 真正传 `Timeout`；新增 `HTTPTimeout()` 使"生效值"可被断言；**负向对照**：去掉 `* time.Second`（变 5 纳秒）→ 立即红 | ⚠️ 本行曾误记 BLOCKED（`4d4b1f9` 已修），第二方核对抓出 |
| 35 | P3 `analytics` 重试默认值 | `[A]` | PASS | `go test ./internal/config/ -count=1` → `ok`：`SetDefaults` 后 `Kafka.MaxRetries == 3`、非零值不被覆盖。实现：`config.go` 补 `if c.Kafka.MaxRetries == 0 { = 3 }`（与 ai-svc 同名字段对齐）。**边界如实记录**：0 被视为"未设置"⇒ 无法用该参数关闭重试，已写进代码注释 | ⚠️ 本行曾误记 BLOCKED（`4d4b1f9` 已修），第二方核对抓出 |
| 36 | user/assessment 零候选如实记账 | `[A]` | PASS | plan §2.4 完整盘点表（`user-svc/internal/config/config.go:55-87`、`assessment-svc:49-83` 全文回读） | D-32 已拍板；**未硬造参数** |
| 37 | `smoke_health_discovery.py` | `[A]` | PASS | `python scripts/smoke_health_discovery.py` → `PASS: 14 check(s)`；加 `--with-chaos` → `PASS: 16 check(s)`（含停 Postgres 验 `/health/ready` 返 `HTTP/1.1 503`、恢复后回 200） | 走 `docker exec` 进容器网络内探（宿主侧 4 个服务端口未映射，直连返 000） |
| 38 | Playwright spec + **IAB 详细测试** | `[A]`+`[V]` | PASS | ① `npx playwright test e2e/health-discovery.spec.ts` → `8 passed (2.4s)`（chromium 4 + mobile 4）；负向对照：断言反转 → 1 failed。② **IAB 浏览器实测**（真实用户路径，非 API 直调）：在登录页点「用演示账号快速体验」→ 成功跳转 `/chat/conversation/new`；侧边栏真实路由为 `/chat/conversation` `/question` `/chat/user` `/chat/setting`；访问 `/chat/user`（我的空间）→ **用户信息 + 三张图表全部渲染出真实数据**（近30天对话频次峰值 60、互动深度指标、昼夜模式环形图）+ 人格画像五维度有值。截图 `screenshots/38a-iab-chat-conversation-list.png`、`38b-iab-my-space-real-charts.png`（**已查看**） | **这条证据的价值**：图表有真实数据 ⇒ 浏览器 → APISIX 网关 → BFF → **analytics-svc / assessment-svc** 多跳全部打通，是 curl 层证不到的（curl 不穿前端 origin 与 CORS）。途中踩到 IAB 的 locator click 超时（memory 已记该限制），改用 CUA 坐标点击 |
| 39 | APISIX Admin 页面截图 | `[V]` | PASS | `screenshots/39-apisix-admin-upstreams-6.png`（已查看）：Upstreams 页 `1-6 of 6 items`，user/chat/assessment/analytics/ai/web-bff 六个 upstream | **计划期假设被推翻**：原写"节点非空"，实测 Admin API 的 `nodes` **恒为 0** —— discovery 型 upstream 的节点在请求时动态解析、不 materialize 到 Admin API（`/apisix/admin/upstreams/{id}/discovery` 同样返 0 节点）。**节点可用性的真证据是实际请求**（网关 `/api/v1/users/me` 返 401 而非 503），已由 #37/#38 覆盖 |
| 40 | 文档漂移修正 | `[A]` | PASS | **2026-09-30 第三轮复核改判（原 BLOCKED）**：四项内容逐条核实全部完成 —— ① `git log -S "更正（2026-09-29，E2E-23 实测）"` 命中 `49b5daf`（提交标题即"#40 文档级联 —— ADR 更正"），`adr-2026-09-nacos-reintroduction.md:110-111` 两条承诺已加删除线 + 更正表；② `grep -rn NACOS_REFRESH_MS` → 0 命中，与更正节"未实现"一致；③ RUNBOOK `:125` 的「重建任何服务后必须重跑 apisix-seed」已按 D-30 划删除线更正；④ 账本 F-107 已带运行时证据（#18 retry 序列 + 5s 自愈）翻新 | 属 F 组 |

汇总：**PASS 38 / FAIL 0 / BLOCKED 0 / N/A 2**

> ✅ **2026-09-30 第三轮复核：BLOCKED 归零。** 原唯一 BLOCKED 的 #40 经四项逐条核实**全部完成**，
> 已改判 PASS（证据见 §2 该行）。本表此前的 "PASS 37 / BLOCKED 1" 与
> 报告头、roadmap、PR #130 正文**四处同步更新**。
>
> **BLOCKED = 0 意味着 RUNBOOK §4 的 1/3 红线已无争议**——阶段判 `partial` 的理由不再是测试点，
> 而只剩 §5 里**归属本阶段的两条账本残留**（F-107 / F-156 的同批代码）与一条待裁定（F-163）。
>
> N/A 2 项（#14 `Resume()` 语义不适用、#16 陈述性）—— 理由见各自行备注，**未用 N/A 掩盖任何一项**。

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| 4 服务 `/health` 的 `Status` 硬编码 `"ok"` | 范围内 | 修复 commit `3034f52` |
| `/health/ready` 会被两处鉴权中间件 401 拦掉 | 范围内 | 修复 commit `52d0a4e` |
| BFF 探出 degraded 仍返 200 | 范围内 | 修复 commit `10e494c` |
| compose healthcheck 打 liveness ⇒ readiness 形同虚设 | 范围内 | 修复 commit `c230095` |
| gRPC health 永不翻转（`healthSrv` 是局部变量） | 范围内 | 修复 commit `5cab18c` |
| `NACOS_REQUIRED` 全仓零声明 | 范围内 | 修复 commit `1579038`（D-31） |
| `db-migrate` `Exited(1)`（F-151） | 范围内 | 修复 commit `f748d61` |
| **11 个常驻服务零 healthcheck（含网关 APISIX）** | 范围内 | 修复 commit `997111e`（7 个） |
| **`wait_healthy` 对设了 `container_name` 的服务恒失效** | 范围内 | 修复 commit `b5cea46` |
| 多个守卫脚本用 `python3`（本机不存在）⇒ 假绿 | 范围内 | 修复 commit `1579038` |
| APISIX upstream 无主动健康检查（`checks` 段） | **范围外** | 账本 E2E-F-154，归 E2E-25 |
| 5 服务 Nacos ops 配置"读了扔" | 范围外 | 账本 E2E-F-155，E 组处理 |
| 账本 F-107 描述与代码事实相反 | 范围内（部分） | 账本 E2E-F-156，**待 #18 运行时复现后才可翻状态** |
| ADR 两条承诺无实现落点 | 范围内（部分） | 账本 E2E-F-157，**待 #13/#14 结论落地** |
| skywalking-oap/ui、obs-mock-receiver 仍无 healthcheck | 范围外 | 本轮未登记（非本阶段目标） |
| `ai-svc` 的 `LLM.Timeout` 是死配置 | 范围外（E 组 P2） | 账本 F-159（见 §5） |
| `analytics-svc` `SetDefaults` 漏 `Kafka.MaxRetries` | 范围外（E 组 P3） | 账本 F-160（见 §5） |
| `configcenter.sensitivePrefixes` 只拦 dataId 不拦 YAML 内 key | 范围外（E 组 P1） | 账本 F-158（见 §5） |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| `3034f52` | 4 服务 `status` 说真话 | 4 份 `healthlogic_contract_test.go`，RED 实测 `expected "degraded" / actual "ok"` |
| `52d0a4e` | `/health/ready` 免鉴权 | `health_endpoint_auth_test.go`，RED 实测 `/health/ready` 无 header 返 401 |
| `10e494c` | 6 服务 readiness 端点 | `health_ready_handler_test.go`（user）、`health_ready_test.go`（bff）；接线守卫 RED 实测 `undefined: HealthReadyHandler` |
| `c230095` | compose healthcheck 改指 ready | `test_healthcheck_readiness.sh` RED 后 GREEN；负向对照 5/1 |
| `5cab18c` | gRPC health 停机翻转 | `server_health_transition_test.go` RED 实测 `MarkShuttingDown undefined` |
| `1579038` | NACOS_REQUIRED 编排声明 | `test_nacos_required_declared.sh` RED 实测 1/3 |
| `f748d61` | db-migrate 等 PG | `test_migrate_pg_wait.sh` RED 实测 2/4 |
| `997111e` | 7 服务补 healthcheck | `test_obs_healthchecks.sh` RED（守卫为新增，无存量缺陷） |
| `b5cea46` | dev-up 分批等待 | `test_devup_batch_waits.sh` RED 实测 4 条错配 |

**负向对照清单**（每条都实跑过，证明断言有约束力）：

| 守卫 | 负向操作 | 结果 |
|------|---------|------|
| `test_healthcheck_readiness.sh` | 8888 改回 `/health` | RED（5 PASS / 1 FAIL） |
| `test_grpc_health_shutdown.sh` | 删掉 chat-svc 的 `MarkShuttingDown()` 调用 | RED（2 条 FAIL） |
| `health_routes_wiring_test.go` | 删掉 ai-svc 的 `r.GET("/health/ready")` | RED |
| `test_nacos_required_declared.sh` | 删掉 dev 的 `NACOS_REQUIRED: "0"` | RED |
| `test_migrate_pg_wait.sh` | 删掉等待间隔递增逻辑 | RED |
| `test_obs_healthchecks.sh` | 删掉 grafana 的 healthcheck | RED（14 PASS / 1 FAIL） |
| `test_devup_batch_waits.sh` | `emotion-echo-$name` 回退为 `$name` | RED |
| `healthlogic_contract_test.go` | `if !dbOK` → `if false && !dbOK` | RED（2 个测试） |

> 最后一条的记录方式：第一次尝试直接改回字面量 `"ok"`，被编译器先拦下
> （`declared and not used: status`），**测试根本没跑到断言**。改用保留变量
> 的方式重做才算真正的行为级负向对照。**编译错误不等于负向对照成功。**

## 5. 账本对账（RUNBOOK §7 #9）

> ⚠️ **本表上一版与 §2、与账本同时相反**（记着"F-158/159/160 未修""F-155 未开工""F-151 部分解决"，
> 而三者实际均已闭环）。第二方核对的 C-1 只更正了 §2 与账本，**漏掉了本表**。
> 本次按账本逐条回读后重写，**本表是 §5 的唯一副本**。

| 账本条目 | 归属 | 本轮处理 | 状态 |
|---------|------|---------|------|
| E2E-F-107 | E2E-23 | **已闭环**（第三轮复核）：启动期 retry 序列实测 + 5s 自愈；运行期由 #20 证明本就成立；**残留的"watcher 吞掉续约错误"已于同日 TDD 修完**（`_, _ =` → 捕获 error + WARN/节流/恢复 INFO，负向对照实测旧代码 49 次失败只打 3 条 WARN、续约错误 0 条日志） | ✅ 已解决 |
| E2E-F-151 | E2E-23 | **已闭环**：① `db-migrate` `ExitCode 1→0` 有运行时前后对照；② 观测栈那一半已由 §2 #26 覆盖——`test_obs_healthchecks.sh` **18/18 GREEN**，含 skywalking-oap/ui 与 obs-mock-receiver（第二方核对 C-6 曾点名这三个遗漏，本阶段补齐） | ✅ 已解决 |
| E2E-F-154 | E2E-25 | 本阶段范围外，仅记录；#23 的反向验证（实例摘除后路由仍 401 非 503）补充了其证据 | 🔴 未解决（归 E2E-25） |
| E2E-F-155 | E2E-23 | **已全量闭环**：chat 4 + ai 9 + analytics 1 共 14 个参数运行时逐项验证生效；`user-svc`/`assessment-svc` 按 D-32 **不硬造参数** | ✅ 已解决 |
| E2E-F-156 | E2E-23 | **主体闭环**：F-107 描述与代码相反的失真已用运行时证据修正；#20 推翻了"无重注册"的预判。**残留的两点均已于同日 TDD 修完**：`if failCount <= 3` → 节流（每 12 次一次）+ 12 次时打"通道长期不可用"通知；watcher 吞错误 → 捕获并按节流记 WARN + 恢复 INFO。回归钉 5/5（含"不存在长度 ≥ 12 的完全静默窗口"的性质断言） | ✅ 已解决 |
| E2E-F-157 | E2E-23 | #13/#14 已落地（commit `5cab18c`）；**ADR `:110-111` 两条承诺的文本回填已由 `49b5daf` 落地**（`git log -S` 核实），`NACOS_REFRESH_MS` 全仓 0 命中与更正节一致 | ✅ 已解决 |
| E2E-F-158 / 159 / 160 | E2E-23 | **均已修**（commit `4d4b1f9`）：P1 敏感字段词根拆分 + P2 `LLM.Timeout` 真正接线 + P3 `SetDefaults` 补 `Kafka.MaxRetries`；§2 #33/#34/#35 已判 PASS 并附负向对照 | ✅ 已解决 |
| E2E-F-163 | E2E-23 | **事实已更正并已落地**：原记"只接了 `Shutdown()`"与代码不符——5 个服务用的是上游 health server，承载 `Shutdown()`/`Resume()` 的类型**生产零实例化**。**用户 2026-09-30 裁定方向 (a)，整个 server 侧包装已删除** | 🟠 待最终归档确认 |

**归属本阶段且未闭环的账本 = 0 条。** F-107 / F-156 的续约可观测性残留已于 2026-09-30 TDD 修完翻 ✅；
F-163 经核实事实有误（包装类型生产零实例化）并按用户裁定方向 (a) 整体删除后翻 ✅。

⇒ 按 RUNBOOK §7 #9 与审计 A5，**账本条件已满足**；阶段仍标 `partial` 唯一原因是 RUNBOOK §7 #10
   "执行者不得自行宣布 done" —— **等用户批准**。
⇒ **上一版把 F-151/155/157/158/159/160 也算作未闭环，是本阶段被判 `partial` 的假理由**；
真实理由自始至终只有 F-107 / F-156 / F-163 三条，而它们的工作量是**几个小改动 + 一句用户批准**，
不是"做了很久还没 done"。**三条本轮全部做完 ⇒ 现在真的只差用户一句批准。**

## 6. 回归钉

- 新增 `scripts/test_healthcheck_readiness.sh`（**25/25**；初版 7 项，复核轮加了 Helm 静态 12 项 + 渲染 6 项）
- 新增 `scripts/test_grpc_health_shutdown.sh`（25/25）
- 新增 `scripts/test_nacos_required_declared.sh`（3/3）
- 新增 `scripts/test_migrate_pg_wait.sh`（4/4）
- 新增 `scripts/test_obs_healthchecks.sh`（**18/18**；16 常驻 + 2 一次性。⚠️ 第二方核对实测：**它只断言 `healthcheck:` 键存在，不校验探针端点** —— 把 grafana 探针端口改到永远不通的 `:9999`，守卫仍报 GREEN。端点正确性目前靠人工一次性验证，不是回归钉）
- 新增 `scripts/test_devup_batch_waits.sh`（3/3）
- 新增 `scripts/_extract_compose_block.py`、`scripts/_check_devup_batches.py`（守卫辅助，非独立门禁）
- Go 单测：5 份 `healthlogic_contract_test.go` + 1 份 `health_ready_handler_test.go` + 1 份 `health_ready_test.go` + 1 份 `server_health_transition_test.go` + 1 份 `health_endpoint_auth_test.go` + 1 份 `health_routes_wiring_test.go`
- Playwright spec：`emotion-echo-web/e2e/health-discovery.spec.ts`（4 用例 × 2 project，`8 passed`）

**全量回归**：7 个 Go 模块 `go test ./...` + `go vet ./...` 全绿。

## 7. 已知债与未验证项（如实记录）

1. **`-race` 本机无法执行**（Windows 报 `0xc0000139`，DLL 加载错误）。按 E2E-21 教训，不用 Windows 错误解释 CI 行为。`MarkShuttingDown` 用了 `RWMutex` 保护 `healthSrv`，逻辑上无竞争，但**未经 -race 实测**，须 CI 复核。
2. **`ai-svc/internal/grpcserver` 整包测试耗时 76s**，超 RUNBOOK §11 的 5s 护栏。对照实测：单跑既有测试 `TestGetEmotionByMessage_HappyPath` 也需 5.6s，慢因来自既有 `startTestServer` 的 cleanup 里 `GracefulStop` 等待 2s，**非本轮新增测试引入**。本轮未擅自重构。
3. **未实机跑完整 `dev-up.sh`** —— 它会重建 19 容器栈，且当前栈正在运行、多个服务镜像已在本轮改过，贸然重跑风险高于收益。改为抽出 `wait_healthy` 单独实机验证四种命名路径。**此项未完成，不假装验过。**
4. **`grpcurl` 无法探测 gRPC** —— 容器不注册 gRPC reflection（`failed to query for service descriptor`）。改用仓库内 `shared/pkg/healthcheck` 客户端（生产同一套封装）。
5. **APISIX healthcheck 只能测端口** —— 镜像内无任何 HTTP 客户端，`resty` 跑不通，且未启用 healthcheck/public-api 插件（`/apisix/status` 实测 404）。端口通**不反映路由是否可用**，后者归 E2E-25。

## 8. 待决策 / 升级项

- **无阻塞性待决策**。D-29 / D-31 / D-32 已于 2026-09-29 由用户拍板。
- **D-30 保持待实测**（非用户决策项，由测试点 #23 出结论后自动落定）。
- **无阻塞性待决策**。破坏性实验（停 Nacos / 停 Postgres / 停 Redis）已经用户在 2026-09-29 授权并全部执行完毕（#5/#6/#10/#18/#19/#20/#21/#23）。
- `Resume()` 接线（#14）与 skywalking 侧 healthcheck（#26 范围内）两项属"计划要求但未做"，已在 §2 与 §3 如实标注，**建议下一轮补做或由用户裁定降级**。

## 9. 收口自检

- [x] **第三轮复核顺带抓到一个真门禁漏洞并已修**：`e2e_stage_audit.py` 的 `parse_ledger()`
      对格数 < 6 的行**静默 `continue`**，而账本有 4 条**跨行的 Markdown 表格行**
      （F-165/166/167/171）⇒ 这 4 条对审计器完全不可见。
      **要害不是 A8 误报编号不连续，而是 A5**——A5 是"阶段判 done 前账本须对账干净"的
      唯一执行者，它也看不见 ⇒ **阶段可以带着一条未解决的账本判 done 而门禁全绿**。
      本阶段当时**侥幸**没被绕过（那 4 条都不归 E2E-23）——**靠运气不是机制**。
      修法 + 回归钉 `scripts/test_audit_ledger_parser.sh`（5/5 GREEN，含 2 条负向对照），
      已接入 `e2e-guards.yml` 第 10 项；**顺带清掉 23 个阶段共 23 条 A8 误报**。
      修的过程中又踩了两个坑，都已固化进守卫（见该脚本文件头）：
      ① `command -v python3` 在 Windows 命中 Store 别名桩，**存在却静默不执行**，
      导致守卫有两条断言空跑成 PASS；② "解析结果为空 ⇒ PASS" 是弱断言，
      python 片段抛异常时 stdout 同样为空，**异常会被读成 PASS**。
      故守卫改为：解释器必须实跑出版本号才认；每条断言先校验退出码再解释输出。

- [x] **第三轮复核（2026-09-30）：§0 与 §5 均查出与事实相反的陈述并已更正** —— §0 的旧 T-6 是失实条目、
      旧 T-1~T-3 误归属本阶段；§5 把 F-151/155/158/159/160 记作未闭环而它们实际均已闭环。
      未完成项由 7 项收敛到 1 项（且账本未闭环项归零），详见 §0「订正依据」与 §5 表头。
- [x] `git status` 干净（改动均已提交）
- [x] `main` 与 `origin/main` 无 ahead/behind（本轮改动全在 feature 分支）
- [x] 无残留已合并分支（`git branch --merged main` 仅 main）
- [x] `python scripts/e2e_stage_audit.py --all` → 30 阶段 0 FAIL
- [x] 账本对账完成（§5）
- [x] **§13.3 #12 相对链接可达**（plan 3 条、report 0 条，脚本解析 0 坏链）
- [x] **§13.3 #17 残留扫描** `bash scripts/check_residual.sh` → GREEN
      （核对前为 RED：`.e2e23-probe-server.py;D` 空目录是实施期探针遗留，已删）
- [x] **第二方核对已完成**（2026-09-30，见 §9.1）
- [x] **CI 5 workflow 全绿**（最终 `d6cbad7`：go-test / llm-test / web-test /
      doc-drift-check / e2e-guards 全 `success`；过程记录见 §9.2、§9.3 的 G-2b、§9.5）
- [x] **第三轮复核（用户追问"核实了吗"触发）**：§9.4 四组新验证 +
      §9.5 第二方独立复核全量重验 40 个测试点 + 本轮 5 项修复
- [x] **8 个静态守卫全部 exit=0**（healthcheck 25/0、grpc 30/0、
      integration-tag 有条件 GREEN：新增损坏 0、已知债 2 见 E2E-F-167）
- [x] **文档级联扫描已完成**（**10 组 / 17 个 .md**，见 §9.3）
- [x] **§2.5 分支纪律**：`git status -sb` 与 origin 无 ahead/behind；
      `git branch --merged main` 仅 main；本会话未新增 worktree

> ⚠️ **自检项自身曾漏项**：本节原写"账本对账完成，**5 条**未解决项"而 §5 实列 7 行，
> 且缺 #12/#17 两项 —— 由第二方核对（C-10）抓出并补正。

## 9.1 第二方核对记录（2026-09-30）

**核对方式**：独立子代理，任务书明写"执行者自证一律不可信、须独立跑命令"。
核对者**独立复跑**了 smoke（14/16）、Playwright（8 passed）、4 服务 health logic、
ai-svc grpcserver/ops、analytics config、shared configcenter、**6 个守卫（逐个捕获退出码）**、
审计器，并**亲自复现 3 组负向对照**（healthlogic / compose 探针 / gRPC 守卫 → 均 RED，
改后全部 `git checkout --` 还原，工作树核对前后均干净）。

### 核对结论（原文要点）

> 执行者报的 34 个 PASS，我亲自复跑的部分**全部成立** …… **问题不在"假 PASS"，
> 而在报告的文本层与账本层大面积陈旧、与代码事实相反** —— 包括一组反向失真
> （已完成的工作被标成 BLOCKED/未修）。

### 抓出的问题与处置

| # | 问题 | 严重度 | 处置 |
|---|------|--------|------|
| C-1 | **反向失真**：#33/#34/#35（P1/P2/P3）在 report 记 BLOCKED「未实施」、账本 F-158/159/160 记 🔴，但 commit `4d4b1f9` **已实现且测试绿** | 🔴 | 已改 PASS（附复跑证据）+ 三条账翻 ✅ |
| C-2 | report 与 roadmap 内**同一文件三套互斥结论**（表格说完成、正文说未做；§10 把已完成项列为下轮） | 🔴 | 已逐处清理，全文单套结论；§10 重写 |
| C-3 | 账本 F-107 仍 🔴 而 report §5 称 🟡（报告与账本脱钩） | 🔴 | 已按 §5 翻 🟡 |
| C-17 | `.e2e23-probe-server.py;D` 空目录残留（§13.3 #17 实测 RED；被 `.gitignore` 遮蔽故 `git status` 看不见） | 🔴 | 已删，`check_residual.sh` 转 GREEN |
| C-4 | `check_adr_gate.sh` 是**空转门禁**（`git ls-tree` 判整棵树含 ADR 路径，本仓永远命中） | 🟠 | 已记账（下轮修）；本阶段以**新建健康契约 ADR** 补实质 |
| C-5 | 6 守卫 + smoke 在 `.github/workflows/` **零引用** ⇒ 不能拦任何东西 | 🟠 | 已加 `e2e-guards.yml`（6 静态守卫入 CI，**如实标注"仅报告不拦"**）；smoke 需运行栈故留本地 |
| C-6 | plan D2 点名的 `skywalking-oap`/`ui`/`obs-mock-receiver` 未补 healthcheck，而 #26 仍判 PASS | 🟠 | 已在 §2/§3 与 §10 如实标注为"降格 PASS"，建议下轮补做 |
| C-7 | #14 `Resume()` 未接线（plan B1 要求），以 N/A 收口 | 🟠 | 已在 §10 列为待裁定项（按 §4.2 须走"降级 + 批准 + 账本记录"） |
| C-8 | `test_nacos_required_declared.sh` 的 prod 分支只校验注释存在 | 🟡 | 如实记录；该守卫在 CI 中会打印命中的是注释行 |
| C-9 | #40 判 BLOCKED 但 3/4 内容已完成 | 🟡 | 已在 §2 备注列明已完成部分 |
| C-10 | §9 自检项漏 #12/#17 且计数错 | 🟡 | 本节已补正 |

### 核对者明示"无法核实"的项（如实转述）

- **item 16**（required_status_checks）：无管理员 token，`check_required_checks.py` SKIP
- **#18/19/20/21/23 破坏性 Nacos 实验**：未复跑（需停机 ~100–190s，会扰动他人工作），仅做文本自洽性与间接核验
- **#5 停 Redis / #30~#32 Nacos 热更运行时实验**：核实了接线实体（`curl :8894/health` 实返 `deps:{nacos:ok,redis:ok}`），但未重新推送配置做行为对照
- **report §7 已知债 2/4**（grpcserver 整包 76s、grpcurl 无 reflection）未独立复跑

**执行者确认**：上述"无法核实"项**不做"已核实"声称**；破坏性实验的原始日志在 §2 各行证据列，
可由任何人按命令复现。

## 9.2 CI 检出：两处 AP-09（改实现未同步测试）——执行者自曝

`92805ad` 推送后 CI 5 个 workflow 中 **`go-test` 红**（其余 4 个 success）。
逐步定位：

| 步骤 | 命令 | 结果 |
|------|------|------|
| 1 | Actions API 查 run 列表 | `go-test` failure，其余 4 个 success |
| 2 | run → jobs API | 仅 `test (emotion-echo-web-bff)` 失败，失败步骤 = `go vet` |
| 3 | 本地复现 `go vet ./...` | `main_test.go:171: not enough arguments in call to registerRoutes` |
| 4 | 补参数后 `go test ./...` | `TestRegisterRoutes_WithEmotionQ` 失败：`should have 39, but has 40` |

**根因**：本阶段改了 BFF 的 `registerRoutes` 签名（新增 `handler.HealthDeps`）并新增了
`GET /health/ready` 路由，但**没有同步更新 `main_test.go`** —— 正是
[anti-patterns.md](../../anti-patterns.md) **AP-09（改实现不改测试）** 的教科书案例。
两个具体漂移：

1. 5 处 `registerRoutes(...)` 调用少传第 8 个参数 → **测试文件编译不过**；
2. `wantRoutes` 白名单缺 `GET /health/ready` → **路由契约测试计数差 1**。

**为什么本地没发现**：本阶段 Go 侧的验证一直以 `go build` + 守卫脚本为主，
**没有对 5 个服务逐个跑 `go test ./...` 全量**；`go build` 按定义不编译 `_test.go`。

**处置**：`main_test.go` 5 处调用补 `stubHealthDeps()`（零值 `HealthDeps`，
`HasDeps()==false`，路由装配行为与改动前一致），`wantRoutes` 补 `GET /health/ready`，
并把该文件 `gofmt -w`（此前缩进已被本阶段早前的误操作打乱，gofmt -l 一直有输出）。
`go vet ./...` + `go test ./...` 全绿。

**留下的教训**（已并入本轮收口动作）：守卫脚本绿 ≠ 代码绿；**凡改动跨包签名或路由表，
必须 `go vet ./...`（会编译测试文件）而非 `go build`**，这正是 AP-09 硬规则第 2 条。

## 9.3 收口后文档级联扫描（2026-09-30，catch → 修 → 再抓两处真缺口）

§13.3 只要求"级联修改文档"，但没给"扫哪些"的清单。本轮用独立子代理做了一次
**只读全仓漂移扫描**（专挑与本阶段 12 条新事实矛盾的陈述），结果分两类。

### 已更正的文档级联（**10 组 / 17 个 .md 文件**）

> ⚠️ **本节初版写"10 个文件"，是执行者自己数错了**：`10` 是下表的**组数**，
> 实际改动的 `.md` 是 **17** 个（`git show --stat e1c9ab0 | grep -c '\.md$'` 实测）。
> 同一组里的多个文件（如 decisions.md 与 microservices.md 同时改了汇总表和验证段）被并成一行，
> 于是"组数"被当成了"文件数"——正是 anti-patterns **AP-14（数字与事实不一致）**。

| 文件 | 陈旧内容 | 更正 |
|------|----------|------|
| `docs/learn/08-probes-and-security.md` | 「我们项目**把所有探针都指向 `/health`**（浅）以简化；生产建议分两个端点」 | 整节重写为 D-29 双端点现状 + K8s 侧常见落地遗漏；示例 YAML 的 readinessProbe 同步 |
| `docs/ci-workflows/README.md` | 文件清单只有 3 个 workflow；整段前提是「PAT 无 workflow scope 故存模板」；「**任何 test 失败 → PR 不可 merge**」 | 重写为 5 个 workflow 的权威说明；**删掉那句"不可 merge"**（正是 anti-patterns AP-11 本身禁止的表述，而本仓新加的 `e2e-guards.yml` 头里就写着"仅报告不拦"）；补 CI 查询的正确姿势（旧 Statuses API 对本仓恒空） |
| `docs/e2e-roadmap/roadmap.md:34` vs `:112` | 同一文件 L34「E2E-23 🟡 详档待建档」与 L112「partial，PASS 37，23 commit」自相矛盾 | 激活块改为「当前 E2E-24 / 上一阶段 E2E-23 partial」；E2E-22 块里「db-migrate 范围外归 E2E-23」补闭环标记 |
| `docs/architecture/decisions.md` 决策 10 汇总表 | 「健康检查 = grpc health 探活（5s/次连续 3 次摘除）」「服务发现 = 客户端定时拉取 + watch（30s 间隔）」—— 两条**从未落地**的承诺，勘误只落在 Nacos ADR 没落到汇总表 | 两行按实际机制重写（`/health`+`/health/ready`+`MarkShuttingDown()`；`Heartbeat()` 续约，无 `NACOS_REFRESH_MS` 轮询），并标出原文是未落地承诺 |
| `docs/architecture/decisions.md` / `microservices.md` 验证段 | 6 条 `curl .../health` 当验收手段 | 全改 `curl -i .../health/ready` —— `/health` 恒 200，用它验收等于什么都没验 |
| `docs/architecture/microservices.md:177` | 「每个 svc 暴露 `/health`，返回 dbOk / kafkaOk」 | 补 `/health/ready` + BFF 另加 redis/nacos |
| `deploy/configuration.md` / `stage-35-ops-runbook.md` | `NACOS_HOT_RELOAD` 默认 false，理由写「SDK↔server 有 bug」 | 注明 chat/analytics/ai 现为 `true`、原理由已不成立，并写明 **env 优先于 yaml**（本阶段排查出的真因） |
| `QUICKSTART.md` Q3 | 「容器 unhealthy 但 /health 返回 200」的答案停在旧根因 | 改为"这是 D-29 的设计使然"，第一步就是换 `/health/ready`；补 503 输出样例与 `/health/ready` 只支持 GET 等新坑 |
| `docs/learn/11-compose-to-k8s.md` | compose→k8s 的 healthcheck→3 探针映射是无脑 1:1；三探针全打 `/health` | 示例按双端点改写，并加一段说明**这个映射不是 1:1**（readiness 摘流量、liveness 才重启） |
| `docs/stages/stage-31-landing.md` / `observability-compose.md` / `docker-compose.md` / `stage-34-ops-runbook.md` / `decomposition-plan.md` | 排障指向 `etc/*.yaml`（env 优先，指向 yaml 会误判）；验收判据停留在 6 容器；固定 30×2s 等 PG（与 E2E-F-151 同型反模式） | 逐条更正；`stage-34` 的等待循环改成递增退避 + 超时非零退出 |
| `docs/ci-workflows/{go-test,llm-test}.yml`（2 个**模板副本**） | 头部仍写「本文件存放在 `docs/ci-workflows/` 而非 `.github/workflows/`」——该前提 2026-09-24 已解除，是不实陈述；两份内容已与真实 workflow 分叉却无任何免责标注（`web-test.yml` 早有 HISTORICAL 标注，另两份没有） | 按 `web-test.yml` 的既有格式补 HISTORICAL TEMPLATE 头 + "如有冲突以 `.github/workflows/` 为准" |

### 扫描顺带抓出的两处**真缺口**（不是文档问题）

| # | 缺口 | 后果 | 处置 |
|---|------|------|------|
| G-1 | **Helm 侧 6 个服务的 `readinessProbe` 仍打 `/health`** —— 本阶段只改了 compose | **生产（K8s）里 DB 挂掉时 Pod 不会被摘出 Endpoints，继续接流量，且零报错**（探针返 200 判定"健康"）。liveness/readiness 分离在生产等于没做 | TDD 修：先把断言加进 `scripts/test_healthcheck_readiness.sh`（RED 6 FAIL）→ 改 6 份 chart（GREEN 19/19）→ 负向对照（把 user-svc 改回 `/health` 立即 `FAIL 1 / RED`）|
| G-2b | `test_route_contract.sh` 里 `WEB_API_ROUTES` 写的是 `Emotion-Echo-Web/...`，仓库实名是 `emotion-echo-web/...`（全小写） | **本地 Windows 绿、CI ubuntu 红**：Windows 文件系统大小写不敏感，脚本照常跑通；接进 Actions 后第一步就 `missing source` exit 1。**"本地全绿"不能替代跨平台验证** | 改小写 + 在脚本里写明这条坑（E2E-23 新增的 7 个守卫中唯一一个有大小写依赖的） |
| G-2 | `scripts/test_route_contract.sh` 变红：`BFF route GET /health/ready NOT covered by APISIX` | 该脚本**不在 6 个 CI 守卫之列**，所以本阶段加路由时它静默变红、无人发现（AP-10 孤儿守卫的变体：守卫存在但没接进任何执行路径） | 修三处：① 把 `/health/ready` 显式加入该脚本的基础设施路径白名单（**只加这一条，不改成"跳过所有非 `/api/v1`"**，那会放过任何拼错前缀的路径）；② 更正脚本头部三处陈旧计数（27→37 主路径、`main.go:214-246`→`main.go:430`）并指向 `main_test.go` 的 `wantRoutes` 为单一事实源；③ **把它接进 `.github/workflows/e2e-guards.yml`（现 7 个守卫）**——光修脚本不接线，下次加路由还会静默变红 |

### 复核轮（2026-09-30，被用户追问"中间态和结果都核实了吗"触发的自查）

上一节的所有结论都只有**静态检查**背书。复核时逐条追问"这个验证本身可靠吗"，抓出 4 处问题：

| # | 问题 | 处置 |
|---|------|------|
| V-1 | **改了 6 份 Helm 模板却从未 `helm template` 渲染过** —— 静态 grep 只证明"文本里有 `/health/ready`"，证明不了 YAML 没被改坏 | 实测 6 个 chart 全部 RENDER OK 且 readiness/liveness 路径正确；并把 `helm template` **固化进守卫**（守卫 19 → 25 项），helm 缺失时**报红并注明"本项未验证"**（不静默跳过 —— 本阶段 `python3` 假绿的同型教训）；负向对照（注入坏模板）确认守卫会红 |
| V-2 | **liveness 断言没做过负向对照** —— 只验证过 readiness 侧 | 注入（把 chat-svc 的 liveness 改成 `/health/ready`）→ `FAIL 1 / rc=1`；还原后 `19/0 / rc=0`，工作树干净 |
| V-3 | 本节写「已更正的文档级联（**10 个文件**）」，实际是 **10 组 / 17 个 .md** —— 组数被当成了文件数（AP-14） | 已更正，并在小节头写明更正原因与复核命令 `git show --stat e1c9ab0 \| grep -c '\.md$'` |
| V-4 | 账本 E2E-F-164 写「10 处坏相对链接」，其中 2 条是 `/docs/...` **root-absolute** 路径 —— 它们在 GitHub 上**完全有效**；且未验证是否 pre-existing | 更正为 **7 处**（decisions.md 6 + roadmap.md 1），并**对 `origin/main` 跑同一扫描确认 7 条全部 pre-existing、非本轮引入** |

顺带抓出第 5 处：`docs/ci-workflows/{go-test,llm-test}.yml` 两份**模板副本**头部仍写
「本文件存放在 `docs/ci-workflows/` 而非 `.github/workflows/`」——该前提 2026-09-24 已解除，
是不实陈述；两份内容也已与真实 workflow 分叉却无任何免责标注（`web-test.yml` 早有 HISTORICAL
标注，另两份没有）。已按 `web-test.yml` 的既有格式补 HISTORICAL TEMPLATE 头。

**教训**：G-1 说明"改了 dev 编排"不等于"改了健康契约"——契约的适用面是**所有部署形态**。
G-2 说明"守卫写好了"不等于"守卫在跑"——`test_route_contract.sh` 早于本阶段存在，
但因为没接进 CI，本阶段的一次路由新增就能让它悄悄变红。
G-2b 说明**本地绿 ≠ CI 绿**：本项目开发机是 Windows（大小写不敏感），
脚本里一处路径大小写写错能躲过所有本地检查，一进 ubuntu runner 立刻死。
这也解释了为什么前 6 个守卫从 `92805ad` 起就在 CI 跑得好好的，
而新加的第 7 个第一次跑就红——**不是新守卫写得差，是它第一次离开了 Windows**。
这两条都是 anti-patterns 里已有条目的复现（AP-10 / AP-01），已按原条目处置，未新开账本条目。

## 9.4 收尾复核轮（2026-09-30，用户追问"中间态和结果都核实了吗"）

上一条消息只报了"门禁全绿"，但**没交代这些门禁本身覆盖了什么、没覆盖什么**。
复核把"我说已验证"逐条拆开重跑，抓出 5 处问题（详见 §9.3 复核轮表 V-1~V-5）。
本节记录**为补齐覆盖面而新做的四组验证**。

### ① Helm 契约：渲染产物级验证（补 V-1 的窟窿）

| 验证 | 命令 | 结果 |
|------|------|------|
| 6 个 chart 能否渲染 | `helm template <c> charts/.../<c>` | 6/6 **RENDER OK** |
| 渲染产物里探针路径**精确值** | `yaml.safe_load` 取 `Deployment.spec.template.spec.containers[].readinessProbe.httpGet.path` | 6/6 `== /health/ready`（**精确相等**，不是前缀包含） |
| liveness 仍为浅探针 | 同上，取 `livenessProbe.httpGet.path` | 6/6 `== /health`（startupProbe 同为 `/health`） |
| chart 静态合法性 | `helm lint` × 6 | 6/6 `0 chart(s) failed` |

> **为什么必须是 YAML 解析而不是 grep**：守卫里用 `case "$r_path" in /health/ready*)` 是
> **前缀匹配**，`/health/ready-but-wrong` 也能过。已把守卫的渲染断言改成
> 去掉行内注释后**精确字符串比较**。

### ② D-29 运行时契约（本轮新做，非引用旧结论）

在真实 dev 栈上做**前 / 中 / 后**三段对照（停依赖 → 观察 → 恢复）：

| 阶段 | `GET /health` | `GET /health/ready` |
|------|--------------|---------------------|
| 基线（全绿） | **200** `status=ok` `deps{nacos:ok, redis:ok}` | **200** `status=ok` 同 deps |
| **停 Redis** | **200**（仍 200，符合"liveness 恒 200"）`status=**degraded**` `deps.redis=unhealthy` + `detail` | **503** `status=**degraded**` |
| 再停 Postgres | 200 `degraded` | 503 `degraded` |
| 恢复 | 200 `status=ok` | 200 `status=ok` |

**这就是 D-29 的定义性行为**：`/health` 不因依赖故障而变红（避免探针误杀引发全站重启），
但 `status` 说真话；`/health/ready` 承载 200/503。`deps` 字段带 `detail` 而非裸 bool ——
**排障时能直接看到 `dial tcp: lookup emotion-echo-redis: i/o timeout`**，不必翻日志。

### ③ IAB 浏览器复验（重跑，不引用旧截图）

栈在我接手前有 10 个 infra 容器被外部终止（同刻 `Exited(255)`，postgres/nacos/redis/kafka/etcd/
grafana/loki/minio/prometheus/alertmanager），BFF 因 Nacos 不可达而 unhealthy 并在重试
`WaitForNacos`（attempt 1→3/10）。按 §八 登记 `deploy/.devmode-session` 后重启
（`nacos` 带 `profiles: ["dev"]`，普通 `up -d` **不会**起它 —— 这是个容易漏的点），
22 容器全 healthy 后再测。

`/chat/user` 复验（演示账号 Echo User / ID 1，**登录态真实**）：

| 图表 | 客观证据（canvas backing store 直读） | 视觉结论 |
|------|----------------------------------------|----------|
| 昼夜使用模式 | canvas 399×432，着色像素 **23.2%** | donut 四段 + 图例，**图例与圆环重叠**（见 E2E-F-165） |
| 近30天对话频次 | 着色像素 **14.9%** | 面积图，2026-09-04~09-14，峰值 ~60，真实时序 |
| 互动深度指标 | 着色像素 **12.9%** | 柱状图 平均轮数/总消息/第三指标，真实数值 |
| 人格维度（雷达） | 着色像素 **6.2%**，纵向内容覆盖 **71.5%**（第 40~348 行 / 432） | 5 轴标签齐全，**完整无裁剪** |

> **我自己推翻了一次假缺陷**（已记 E2E-F-166）：按 `clip` 截雷达图时看到底部大片空白，
> 初判"被截断"；用 canvas 逐行像素统计证伪 —— 越界部分是我给的 `clip` 矩形**超出视口下沿**
> 被填成卡片背景。加高视口到 1680×1900 后截图完全正常。
> 附带发现：本布局里 **`window.scrollTo` 无效**（滚动容器是内层 div）。

截图：[`screenshots/40-verify-my-space-4-charts-real-data.png`](screenshots/40-verify-my-space-4-charts-real-data.png)

### ④ 门禁是否真能拦合并：从"无法核实"变成"已核实"

`GET /repos/{owner}/{repo}/branches/main/protection` 返 **401**（需管理员），
`scripts/check_required_checks.py` 因此只能 SKIP —— 这是我上一条消息里"无法核实"的那项。
本轮改用**公开可读**的 `GET /repos/Exist-a/emotion-echo/rules/branches/main`，返回 **`[]`**
⇒ `main` 上**零条分支规则** ⇒ `required_status_checks` 必为空。
**CI 全红也不阻止合并**，这是有正面证据的结论（已回填账本 E2E-F-162）。

### ⑤ 记忆纠错：python3 "遗留地雷"是错的

我此前记录"仓内另有 4 个脚本仍用 `python3`，未修，属遗留债"。**复核证明这条不成立**：
6 个 `.py` 的 `#!/usr/bin/env python3` **从不生效**（它们一律被 `python <file>` 调用，shebang 无人使用），
逐个跑全部正常；两个 `.sh` 也已有 `PYTHON_BIN` 绝对路径 + `command -v python3 || python` 兜底链。
已订正记忆。残留的真问题只有：`PYTHON_BIN` 默认值是本机绝对路径，且这两个脚本不在 CI 里。

## 9.5 第二方独立复核（2026-09-30，40 个测试点全量重验）

独立子代理，任务书明写"执行者自证一律不可信、须独立跑命令"，共 113 次工具调用 / 104 分钟。

### 总判

**PASS 成立 25 项 / 判定存疑或证据不足 9 项 / 复现不了 6 项**

**没有抓到"文件已创建"式的假 PASS**（AP-01 零命中），37/0/1/2 的算术经其独立统计确认正确。
但抓到 **2 处真 soft assert（守卫自身假绿）**、**1 处结论相反**、多处数字与事实不一致。

### 已处置（本轮修完）

| # | 问题 | 严重度 | 处置 |
|---|------|--------|------|
| V-6 | **#12 结论相反（本阶段自造）**：注册的 per-service 名 `emotion.AI` 等 5 个名字 **proto 里不存在**；真实客户端 `Check()` 得 `NOT_FOUND`；原测试用同一字面量 ⇒ 自证循环永远绿 | 🔴 | 守卫加第 6 条（RED 5 FAIL）→ 5 个 `server.go` 改 proto 真名（GREEN 30/0）→ 补 `TestGrpcHealth_RegisteredNameMatchesProtoServiceName`（负向对照 FAIL）。**报告 §2 #12 判定已更正** |
| V-7 | **我的守卫 compose 段是子串 glob**（`case ... in *"/health/ready"*`）：核对者把 URL 改成 `/health/readyXYZ`，守卫仍报 GREEN | 🟠 | 改**精确 URL 相等**。负向对照：注入 `/health/readyXYZ` → `FAIL 1 / rc=1`；还原 `rc=0`。现与 Helm 段同一标准 |
| V-8 | **`-tags integration` 构建编译不过且无人发现**：核对过程中 `gofmt` 暴露出 `ai-svc/integration_test/dlq_integration_test.go` 语法错误 —— 查 git 确认是**本阶段 commit `81da12d` 手误**（`}()` → `}(, nil)` 且漏了 `Consume` 新增的第 8 参数）。带 `//go:build integration` ⇒ `go test ./...` / `go vet ./...` / CI **全部跳过**，所以全绿 | 🔴 | ① 修好 ai-svc；② 顺藤查出 **user-svc 与 web-bff 也早已编译不过**（pre-existing，user-svc 那处正是 anti-patterns AP-09 当例子引用的 `unknown field Phone`）；③ 新增守卫 `scripts/test_integration_tag_compiles.sh`（**ratchet 语义**）接入 `e2e-guards.yml` 第 8 项；④ 账本 E2E-F-167 |
| V-9 | #9 证据写 `PASS: 7 FAIL: 0`（加 Helm 前的旧值，实为 25）、§6 写 `7/7` 与 `15/15`（实为 25/25、18/18）、§10 写"4 个 workflow 零引用"（守卫已全部接入） | 🟡 | 三处数字全部更正 |
| V-10 | #26 观测守卫**只断言 `healthcheck:` 键存在，不校验端点** —— 核对者把 grafana 探针端口改到永远不通的 `:9999`，守卫仍 GREEN | 🟡 | 如实标注进 §6 与 §2 #26 备注：**端点正确性目前靠人工一次性验证，不是回归钉**。不修（修它要"对运行中容器发请求"，已超静态守卫范畴），记入待办 |

### 我复核后**驳回**的一条（不能照单全收）

核对者称「`e2e-guards.yml` 没装 helm ⇒ 该守卫在 GitHub runner 上**必然 FAIL**，与'CI 5 workflow 全绿'冲突」。
**这条不成立**：Actions API 实测 `4efec2b` 的 `e2e-guards` = `success`，而守卫在无 helm 时 `exit 1` ——
两者只能同时成立于"runner 自带 helm"（GitHub `ubuntu-latest` 镜像确实预装）。
**未加 helm 安装步骤不是缺陷**；但为消除隐式依赖，仍显式加了 `actions/setup-go`（第 8 个守卫需要），
helm 保持使用 runner 预装版并在守卫里保留"缺失即判红"的显式分支。

### 核对者复现不了的（如实转述，不做"已核实"声称）

- **#2 / #5 / #10**（停 Postgres、停 Redis 的降级行为）：破坏性实验未复跑。
  **注**：本轮我已亲自做过等价的运行时验证（停 Redis + Postgres → `/health` 200+`degraded`、
  `/health/ready` 503，恢复回 ok，见 §9.4 ②），但那是**换时点的独立复现**，不等于它复现了原测试点。
- **#18~#23**（Nacos 自愈/重注册/beat 501/APISIX 跟随）：需停起 Nacos 100~190s，未做。
- **#30~#32**（热更 14 参数运行时）：需推 Nacos 配置，会改动共享配置中心，未做。
  核对者判"接线完整、可热更的结论成立"但"运行时实测"复现不了。
- **#27**（db-migrate `Exited(0)`）：需重建容器，未做。
- **#38**（IAB 浏览器）：未重跑浏览器，但**看了截图**并确认数据真实（见 §9.4 ③，本轮我自己重跑了）。

### 核对者独立复现成功的（可作为独立佐证）

`#17` Nacos `count:6`（6 个名字与报告完全一致）、`#22` `NACOS_REQUIRED` dev 段（并确认 prod 段只命中注释行，
即 C-8 描述准确）、`#25` APISIX 镜像内**无任何 HTTP 客户端**（`which wget curl nc busybox` 全空）、
`#39` 截图 6 个 upstream 名字全对。核对者对 `test_grpc_health_shutdown.sh` / `test_migrate_pg_wait.sh` /
`test_devup_batch_waits.sh` / `test_route_contract.sh` 四个守卫的自评是"**不能假绿**"，并各做了负向对照。

## 9.6 收尾轮：把"记账未修"的四项真正做掉（2026-09-30）

前几轮抓到的问题里有 4 项只写了账本没动。本轮按"能修就修"处理。

| 项 | 处置 | 证据 |
|----|------|------|
| **F-167 剩余 2 个 pre-existing 模块** | ✅ **修完**。`user-svc` 集成测试对着**已删除的 schema** 写（model 早无 `Phone`/`Email`/`Status`，repo 早无 `GetByPhone`；反过来 model 有的 `config` 列 DDL 里却缺）⇒ 编译不过，且真跑会 42703。`web-bff` 的 `NewAIStreamHandler` 签名早已改为 `(cfg config.Config)`，fake 缺 `XTTSPhonemes` 与 `UserClient` 6 个方法、`ChatClient` 1 个方法。**改完之后真跑**：`go test -tags integration` user-svc 与 web-bff **全绿**。守卫的 `KNOWN_BROKEN` 随之清空，从 ratchet 转成**无条件门禁**（7/7 GREEN） | 修 user-svc 时又暴露一层：修好编译后**真跑**才发现建表 DDL 缺 `config` 列（`42P03`）—— 改一层、跑一次、再暴露下一层，这正是"编译通过 ≠ 能跑"的实证 |
| **V-10 观测守卫不校验端点** | ✅ 修完。把 16 个探针的**关键串（端口+路径）**固化成基准表，守卫从"查 `healthcheck:` 键存在"升级为"探针必须含基准串"。基准来源是 2026-09-30 在**真实运行的栈上逐个实测可达**，不是照抄文档 | 负向对照：把 grafana 探针端口改成永远不通的 `:9999`（正是第二方演示的场景）→ `FAIL 1 / rc=1`；还原 `rc=0` |
| **F-165 图例与圆环重叠** | ✅ 代码已修（TDD）。`legend` 由 `orient:'vertical', left:'left'` 改为横排底部，`center`/`radius` 配套收小。新增 2 条**几何断言**（把 option 换算成圆环外接矩形与图例占位矩形，要求不相交；另加一条"不得压到标题带"） | RED：`2 failed \| 6 passed` → GREEN：`9 passed`；`typecheck` 中我的文件 0 错误。**另用真实 ECharts 6 离屏排版做引擎级对照**，方向一致（修复前相交、修复后不相交） |
| **F-164 坏相对链接** | 🟡 部分。`roadmap.md` 那 1 条已在本轮修正；`decisions.md` 的 6 条指向端侧轨（Lane O）独占文件，按 AGENTS.md §八 不得触碰 | 账本已改为 7 条并标注 pre-existing（对 `origin/main` 跑同一扫描确认） |

### 未能完成的一项（如实记账，不粉饰）

**F-165 的浏览器渲染复验没做成。** 代码、单元测试、引擎级对照都到位了，但**没拿到浏览器截图**：
`emotion-echo-web` 是生产构建镜像（`node .output/server/index.mjs`，无 bind mount），
源码改动必须重建镜像才可见；而本机 `docker build emotion-echo-web` **必然失败**
（`@oxc-parser/binding-linux-x64-musl` 安装超时，memory `docker-build-frontend-workaround` 有记录）。
退而用本地 `nuxt dev` 后页面能出 SSR 骨架，但 **0 个 canvas**（图表不渲染），无法取证。
已恢复环境（停 dev、重启 web 容器、删 devmode 锁），并记为 **E2E-F-169**。
**按"未验证"记账，不按"已修且已验"记账。**

### 本轮新发现并升级给用户的一项

**E2E-F-168：ai-svc 集成测试 14 个全红、0 个通过。** 这些测试编译是过的（F-167 已修），
所以这轮第一次真跑才暴露：根因是**每个测试文件各自手写一份不完整的建表 DDL**
（全目录只建了 `emotion_analysis` 一张表，`voice_transcripts` / `fused_emotions` / `face_detections` 全缺）。
**修法本身是设计决策**（共享 fixture 跑真实迁移 vs 逐文件补 DDL），按 AGENTS.md §八 第 4 条不由执行者自决，已升级。

## 9.7 用户裁定后的执行轮（2026-09-30）

用户对 4 项待裁定给了结论：F-168 走**共享 fixture 跑真实迁移**；F-165 的验证缺口
**想办法解决**、阶段**暂不收尾**；required status checks 要操作流程。本节记录执行结果。

### F-168 已解决，并顺带抓出一个真生产 bug（E2E-F-170）

按裁定新增 `emotion-echo-ai-svc/integration_test/testdb_test.go` 的 `newAIDB`，
按**生产顺序**执行 `deploy/db/01-create-schemas.sql` → `02-create-tables-in-schemas.sql`
→ `emotion-echo-ai-svc/migrations/i*.sql`（排序）。测试库从此与生产同源；
两个旧 helper 改为委托共享 fixture（`grpc_health` 那份自带残缺 DDL 也并入）。
**找不到任何一份 SQL 一律 `t.Fatal` 而非 `t.Skip`** ——「没检查到」不能算「检查通过」。

第一次跑：**14 红 → 5 红**（基础表齐了）。逐个修的过程中抓出 **E2E-F-170**：

> 🔴 **多模态（face/voice）情绪入库 100% 失败。**
> `i006` / `i008` 把唯一索引改成 **partial** 形式
> （`CREATE UNIQUE INDEX ... ON ...(upload_id) WHERE upload_id <> '__legacy__'`），
> 而仓储的 `ON CONFLICT (upload_id) DO NOTHING` **不带谓词** ⇒ PostgreSQL 推不出冲突目标
> ⇒ `42P10`。**这不是"重复插入时报错"，是每次 INSERT 都在解析期失败**；
> 前面那道 `SELECT` 早退出只能避开重复插入，避不开这个。
> **已在运行中的 dev 库上直接复现**（face 与 voice 均报 42P10；`emotion_analysis` 的
> `event_id` 侧因另有非 partial 约束而不受影响）。

修法：`clause.OnConflict` 补 **`TargetWhere`**（不是 `Where` —— GORM 把 `TargetWhere`
拼在冲突目标列**之后、动作之前**，而 `Where` 拼在动作**之后**，那是给 `DO UPDATE` 用的，
用错会 42601；这个坑我也踩了一次，第一次写成 `Where` 直接语法错）。
另外 `TestDailyEmotionByModalityView_Integration` 的 INSERT 补上 `event_id`
——真实 schema 把它设成 NOT NULL，而这条 INSERT 是照着旧的、不完整的测试库 DDL 写的。

**结果：`go test -tags integration` 由 14 红/0 绿 变为全绿（68.7s）。**

> 这正是"共享 fixture 跑真实迁移"这条路线换来��的核心价值：
> **测试库与生产不同源时，`ON CONFLICT` 这类"依赖索引形态"的 bug 对测试完全不可见。**
> 手抄 DDL 的测试库带的是非 partial 约束，所以永远测不到这条路径。

⚠️ 修复只落在代码上；**运行中的 ai-svc 镜像仍是旧二进制**，需重建后多模态写入才恢复。

## 9.8 F-165 浏览器复验（用户裁定"想办法解决"后完成，2026-09-30）

§9.6 记为"未完成"的那一项，本轮做成了。**两个障碍的真实原因都和原先记的不一样**：

| 障碍 | 原记 | 实测真因 |
|------|------|----------|
| web 镜像构建失败 | 记忆 `docker-build-frontend-workaround` 说是 npm 装 `@oxc-parser/binding-linux-x64-musl` 超时 | **`@mlc-ai/web-llm` 根本没装** ⇒ 主机 `pnpm build` 直接报 `Rollup failed to resolve import "@mlc-ai/web-llm"`。这同时解释了 §9.6 里"`nuxt dev` 起来但 0 个 canvas"——**不是 dev 模式的问题，是整个客户端 bundle 加载失败** |
| 换端口跑构建产物 | 以为换端口即可 | **CORS 取不到数据**：`localhost:3001` 不在 APISIX 的 `CORS_ALLOW_ORIGINS` 白名单，页面内 fetch 直接 `Failed to fetch`（memory `apisix-cors-origin-and-seed-override` 记过这个坑） |

**最终路径**：装上缺失依赖 → 主机 `pnpm build` 成功（7.52 MB）→ **停掉 web 容器**、
用 `node .output/server/index.mjs` 在 **3000** 上跑**同一份生产构建**（origin 与白名单对齐）→ 浏览器复验。

**复验结果**（视口 1680×1080，登录态真实）：四图全部渲染真实数据；
昼夜使用模式的图例已是**底部 2×2 横排、完全在圆环之外**，无任何文字压色块。
截图：[`41-f165-legend-no-overlap-verified.png`](screenshots/41-f165-legend-no-overlap-verified.png)（整页）
+ [`42-f165-donut-canvas-closeup.png`](screenshots/42-f165-donut-canvas-closeup.png)（该图 canvas 特写）。

顺带查明一件事：卡片高度是 `vhToPx(40)` = **40vh**（不是固定 300px）。
我先前用 1900px 超高视口量到的 760px 高卡片与中间大片空白，是**视口造成的、不是回退** ——
换回 1080px 常规视口后 canvas 为 292×432，版面正常。

环境已恢复（停 node 进程、重启 web 容器、删 devmode 锁）。
`package.json` / `pnpm-lock.yaml` 被 `pnpm add` 改动过（把包从 `optionalDependencies` 挪到
`devDependencies`），已 `git checkout` 回退 —— **那是端侧轨的设计决定，不该由本轨顺手改**。

## 9.9 门禁收口：汇总门禁 + 本轮问题落档（2026-09-30）

### 门禁本身：从 33 条点名降到 10 条

用户配好 33 条 required checks 后，我复核发现两个**结构性**隐患（详见
[ADR-2026-09-e2e-23-gate-aggregation](../../../architecture/adr/adr-2026-09-e2e-23-gate-aggregation.md)）：

1. 其中 23 条是 `doc-drift-check` 的**中文 job 名**，GitHub 精确匹配 ——
   谁改一个字的门禁就**静默失效**，不报错、不拦。
2. 33 条要人肉同步，新增检查不会自动纳入。

已加 `doc-drift-gate`（`needs` 全部 23 个检查 + `if: always()`）与
`doc-drift-needs-sync`（校验 gate 的 `needs` 覆盖完整性，且**独立于 gate 跑**）。
分支保护改为只填 `文档守卫总闸`，共 **10 条**（gate 1 + go-test 7 + llm/web/e2e-guards 各 1 - gate 替代了 23 条中除被 gate 覆盖外的全部）。

守卫 `scripts/check_doc_drift_gate_needs.sh` 的两组负向对照已实测：
漏一个 job → `rc=1` 并报出漏项；留一个幽灵 job → `rc=1` 并报出幽灵引用；还原 → `rc=0`。
汇总脚本的判定逻辑也在本地实跑过三种输入：全成功 `rc=0` / 有 failure `rc=1` / 有 skipped `rc=1`
（**skipped 刻意判为不通过** —— 否则"检查被跳过"会被 GitHub 当成通过）。

### 本轮我自己的两次误判（如实落档）

同一件事——"门禁到底配没配好"——我连错两次，**都是同一类错误：把"我没看到"当成"它不存在"**。

| # | 我的做法 | 实际 | 根因 |
|---|---------|------|------|
| M-1 | 查 `GET /repos/{o}/{r}/rules/branches/main`，拿到 `[]` 就断言"门禁没生效" | 门禁**早已生效** | 该端点**只返回 Rulesets（仓库规则集）**，不含经典分支保护。用户建的是经典保护（URL `/settings/branch_protection_rules/83308753`）。**拿错了尺子** |
| M-2 | 用 computer-use 枚举"已保存的 required checks"，稳定得 26 条，据此断言"缺 7 条 go-test" | **33 条一条不缺** | Edge 的无障碍树会**按优先级裁剪长列表**，列表上部的条目根本不出现在树里。我用**被裁剪的证据**做完整性判断 |

M-1 已更正到 `discovered-unresolved.md` 的 E2E-F-162（此前记的是"无法核实"，实际当时就能核实，
只是我查错了地方）。M-2 由用户截图直接推翻。

**这两条不是小事**：它们与本阶段反复在治的病同源 ——
anti-patterns **AP-01（把"文件已创建"当"已验证"）** 的近亲，
以及 [anti-patterns.md](../../anti-patterns.md) 反复警告的
"根因未验证就下结论"。已记入 memory `invalid-probe-evidence-pattern` 与
`emotion-echo-pr-workflow-mechanics`。

**自曝第三处**：写完上面这段"相对路径写错"的记录后，我给新 ADR 加的第一条链接
**当场就写错了同一类错**（从 `stages/e2e-23-health-discovery/` 出发用了 `../../`，
只到 `docs/e2e-roadmap/`，应�� `../../../`）。已由坏链扫描当场抓出并修掉。
顺带修掉 `discovered-unresolved.md` 里一条 pre-existing 的同型坏链
（`../../architecture/adr/adr-2026-09-client-object-url-bff-proxy.md`）。
**这条记录的教训比我预想的具体**：把教训写进文档，并不会自动让我不再犯；
真正抓住它的是**每次改完都跑一遍坏链扫描**这个机械动作。

**沉淀下来的判据**（本轮新增，值得写进规程）：
- 任何"清点清单得 N 条、所以缺 M 条"的推论，**必须先自证枚举是完整的**；
- 反向同样成立："我看到了 N 条"**不能**推出"只有 N 条"。

### 自曝第四处：`git add -A` 误提交临时文件（已被门禁盲区放过）

提交 `cf67113` 时我用 `git add -A`，把此前 API 查询的临时响应文件 `.r.json`
**一并提交进了仓库**。本轮 `c51bcda` 已删除。

- **内容是否敏感**：逐项核过 —— 文件里 12 处 `token|key|secret|password` 匹配全是
  GitHub API 的字段名 `keys_url`；31 处 40+ 字符长串是 **git commit SHA** 与中文提交信息。
  **无任何真实凭据**。但它本就不该进版本库。
- **为什么没人发现**：`check_residual.sh` 原先只扫 `*;D` 空目录与"无末尾换行"；
  `check_orphan_outputs.sh` 只看 `scripts/` 与 `workflows/` 的引用关系。
  **根目录的散落临时文件是两个门禁的共同盲区** —— 又是同一族问题：
  "守卫在，但它守的不是那块地方"。
- **已修**：`check_residual.sh` 增加第 3 项扫描，用 `git ls-files` 判定仓库根下
  `.r*/.c*/.ci*/.api*/.resp*/.tmp*/.probe*` 形态的 `.json/.log/.txt/.out`。
  负向对照：造一个 `.ci-tmp.json` → `rc=1` 并报出；删除 → `GREEN`。

### 另一处真相更正：经典保护没有公开可读的 API

我先前在 `memory` 里写"用 `GET /rules/branches/main` 就能核实门禁"，**那是错的** ——
该端点覆盖不到经典保护。经典保护只能靠
`/branches/main/protection`（需管理员 token，我拿不到）或**开 PR 实测能否被拦**来验证。
已订正 memory。

## 9.10 AP-11 门禁实测：做完了，结论是**门禁没有在拦**

这一节记录我亲手做的红线实测（两次对照），以及它暴露出的问题。

### 怎么做的

用现成的 **PR #130**（9-29 建的，head 跟着本分支走）做载体。
"门禁生效"与"门禁没生效"在 `mergeable_state` 上是**可区分**的：

| mergeable_state | 含义 |
|-----------------|------|
| `blocked` | 合并被分支保护挡住（有**必需**检查红/未完成） |
| `unstable` | 只有**非必需**检查红，**仍可合并** |
| `clean` | 一切正常 |

所以判据是：故意让某条检查红，看它翻成 `blocked` 还是 `unstable`。
光看 `clean` **不能证明门禁存在** —— 没有门禁时也是 `clean`。

### 两次对照的实测结果

| # | 故意弄红的检查 | 是否在我给的 12 条里 | mergeable_state | 结论 |
|---|---------------|-------------------|-----------------|------|
| 1 | `static-guards`（e2e-guards，8 个守卫） | 是 | **`unstable`** | **不在必需检查里** |
| 2 | `视图定义一致性` → 经 `文档守卫总闸` 传播 | 门禁本身是 | **`unstable`** | **`文档守卫总闸` 也不在必需检查里** |

两次都连续轮询 14~16 次（约 7 分钟）且 SHA 正确，状态稳定不变。

### 结论

**当前 main 的 required status checks 没有在起拦截作用。**
两次实测证明：不只是 `static-guards` 漏了，我判定为"已加上"的那两条
（`文档守卫总闸`、`汇总门禁 needs 覆盖校验`）**同样没在拦**。

这与截图里"它们出现在列表中"矛盾。三种可能，我无法从我这侧区分（经典保护
无公开只读 API，我也没有管理员 token）：

1. 保存没真正落盘（横幅出现过，但仍需实测确认）；
2. 名字有肉眼不可见的差异（`文档守卫总闸` 前后是否有空格/全角字符）——
   GitHub 是**精确字符串匹配**；
3. 规则没有应用到 main（规则卡显示 "Currently applies to 1 branch"，但那是保存时的状态）。

**只有开 PR 试合并能最终判定**，而这一步我已经用两次对照把"CI 红"这一侧做完了。

### 顺带修正我自己两处操作失误

- 第一次回退用 `git checkout -- <file>`，那是从**索引**恢复，而索引里已是坏版本 ⇒
  **回退没生效**，导致第一次对照时 `static-guards` 仍在红、结论差点被污染。
  正确做法是 `git checkout HEAD~N -- <file>` 或 `git revert`。
- 第二次想用 `残留物扫描` 当对照，但 `*;D` 目录是 `.gitignore:153` **故意忽略**的
  （那是 shell 误建的残留类，见 E2E-F-18）⇒ 那个文件根本进不了 git、CI 里自然绿，
  对照没做成。改用"直接改一个 doc-drift 脚本"才成功。

两次失误都已修正，临时改动已全部还原（`4e02485`），全量门禁 15 项 + 审计器 0 FAIL。

## 10. 下轮建议

> ⚠️ **本节已降级为指针。** 未完成事项的**唯一真相源是 [§0 未完成清单](#0-未完成清单唯一真相源--收口时必须逐条销账)**
> （含每项的责任人、可核验的完成判据、关联账本号）。
> 本节此前维护着同一批事实的第二份副本，已按 [anti-patterns.md](../../anti-patterns.md)
> **AP-14**（同一事实只允许一处定义）删除，避免两处漂移。

原列的 4 项（C 组破坏性实验 / F 组回归钉 / E 组 14 参数 / #5#6）**均已于本轮完成**。

### 移入 §0 的两项历史事项

| 事项 | 去向 | 说明 |
|------|------|------|
| C-4 `check_adr_gate.sh` 空转门禁 | **仍未做，但不在本阶段范围** | 它是**空转**（`git ls-tree` 判整棵树是否含 ADR 路径，本仓永远命中 ⇒ 永远绿）。本轮已用**独立的** `adr-2026-09-health-check-contract.md` 让 §13.3 #15 有实质内容可校验，但**空转门禁本身没修**。归 **E2E-03 / R-03**（机制建设），不在 E2E-23 账上 ⇒ 未列入 §0 |
| 健康契约独立 ADR | **已建** | `docs/architecture/adr/adr-2026-09-health-check-contract.md`（本轮建），无需再裁定 |

### 若 T-1~T-7 全部销账后的下轮起点

E2E-24 消息链路（outbox→Kafka→consumer→DLQ 全链 + 重试/死信/回放）——
但按 [roadmap.md](../../roadmap.md) 排期，它在第七批；E2E-25（网关 APISIX）带
**F-137** 这条必须先治理的前置。启动前先读 `RUNBOOK.md` 的收口契约。
