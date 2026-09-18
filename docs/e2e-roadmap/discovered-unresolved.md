---
status: active
priority: high
created: 2026-09-17
last-refresh: 2026-09-18 (E2E-06 解决 E2E-F-09 + E2E-F-19；40 项中 8 项已解决)
type: e2e-discovered-unresolved-ledger
---

# E2E 已发现未解决账本（discovered-unresolved）

> 记录 E2E 阶段实测或建档预探查中发现、但**不属于当前执行阶段范围**未修复的问题。
> 编号 `E2E-F-xx`；确认为运行时 bug 且修复落地后，回填 `docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md`（R-xx 体系）并互相引用。
> 账本只增不删：已解决项标注状态保留记录。

## 账本

### A. 建档预探查（2026-09-17，E2E-F-01~19）

| 编号 | 来源 | 现象 | 根因 | 归属阶段 | 状态 |
|------|------|------|------|---------|------|
| E2E-F-01 | 预探查 | 找回密码/注册的验证码无真实投递渠道，流程仍按手机号短信时代设计 | 项目已改用户名登录；`BFF_DEV_RETURN_CODE=1` 仅 dev 回显 | E2E-07 / E2E-09 | 🟡 方案已定：改为**密保问题**（不可跳过、注册必设、弹框 UI、删除验证码步骤），待实施 |
| E2E-F-02 | 预探查 | 心理测验三层契约错位：提交必 400、结果弹窗"等级"恒空、列表页取数失败报错 | 前端发 `answers` 数组 vs 后端要 `map[string]int`；前端读 `level`/`suggestion` vs 后端回 `riskLevel`；前端读 `data.list` vs 后端回 `{items,total}` | E2E-13 | 🔴 未解决 |
| E2E-F-03 | 预探查 | 现工程量表种子数据不存在，surveys 表为空 | `deploy/db/` 无 INSERT 量表的 SQL；文档声称的 `seed-surveys.sql` 在 git 全历史中不存在 | E2E-13 | 🔴 未解决 |
| E2E-F-04 | 预探查 | "人格测试→心理预测→AI 提示词定制"链路完全不存在 | 量表是症状自评非人格量表；评分无维度/画像；BFF system prompt 写死静态字符串；proto 无画像字段；legacy 挂点 `BuildSurveyContext` 函数体 `return ""` | E2E-14 | 🟡 方案已定（D-02），待实施 |
| E2E-F-05 | 预探查 | 数字人口型是随机轮播假口型，与音频零对齐 | `useTTSPlayer.ts:91-103` 每 150ms 循环切 5 个口型；L38-75 映射表是死代码；XTTS `/tts_with_phonemes` 带时间戳但前端从未调用 | E2E-17 | 🟡 方案已定（D-03），待实施 |
| E2E-F-06 | 预探查 | TTS 流式播放段间存在必然断点 | 500ms debounce 聚合文本；每段新文本先 `stop()` 再重发 HTTP；XTTS 每段一次 `inference_stream` 冷启动 | E2E-17 / E2E-28 | 🟡 方案已定（D-03），待实施 |
| E2E-F-07 | 预探查 | Go 服务结构化日志实际未进 Loki，只有 APISIX access log 被采集 | `promtail-config.yaml` 的 services job 指向 `/var/log/services/*.log`，但 compose 无 volume 挂载，也未用 docker-sd 采 stdout | E2E-21 | 🔴 未解决 |
| E2E-F-08 | 预探查 | Redis 容器空转未使用；分布式限流/登录锁定 Redis 后端是 TODO | `docker-compose.infra.yml` 注释"项目当前未使用 Redis"；`limiter.go:137-140` `RedisLimiterBackend: TODO`；BFF 登录失败锁定 in-memory 单实例 | E2E-18 | 🔴 未解决 |
| E2E-F-09 | 预探查 | 数据库无 schema_migrations 版本表；软删除覆盖不完整；db README 过时 | 迁移靠"幂等 + 每次重放"；软删除仅 users/ai 域，chat 的 deleteconversation 是物理删；README 仍列不存在的 `03-migrate-data.sql` | E2E-06 | ✅ **已解决**（2026-09-18，E2E-06：schema_migrations 表 + migrate.sh 版本追踪 + conversations/messages 软删除 + README 全面更新） |
| E2E-F-10 | 预探查 | analytics mental-health 报表读一张永远为空的表 | `mental_health_assessments` 无任何生产写入方（INSERT 仅出现在测试）；trigger runner 只读已有记录后 marshal 入队 | E2E-15 | 🔴 未解决 |
| E2E-F-11 | 预探查 | Alertmanager 无外部通知渠道，仅 Web UI 聚合 | `alertmanager.yml` 只接 dev-ui 空 receiver | E2E-22 | 🔴 未解决 |
| E2E-F-12 | 预探查 | DLQ 无自动回放工具 | 告警规则注释里的回放都是手工 psql UPDATE；`InMemoryDLQPublisher` 仅测试用 | E2E-24 | 🔴 未解决 |
| E2E-F-13 | 预探查 | gRPC 路径的 trace_id 未注入结构化日志 | `grpcinterceptor/tracing.go` 只做 SkyWalking 上报，未调 `logging.WithTraceID`（HTTP 侧有做） | E2E-21 / E2E-26 | 🔴 未解决 |
| E2E-F-14 | 预探查 | user 页 3 个图表（昼夜/频率/深度）数据为空时整块不渲染且无空态提示 | `chat/user/index.vue:194,206,216` 每图均有 `?.length > 0` 守卫；数据源依赖 analytics 事件链 | E2E-11 | 🔴 未解决 |
| E2E-F-15 | 预探查 | 系统无管理员/角色概念 | `users` 表无 role 列；全仓无 admin 页面/端点 | E2E-06 / E2E-07 | 🟡 已通过 D-01=C 规避 |
| E2E-F-16 | 预探查 | `.mimosa/`（三处）未被 gitignore，持续污染 `git status` | hook 运行时状态目录，`.gitignore` 未覆盖 | E2E-02 | ✅ **已解决**（2026-09-17，commit a44ee4a） |
| E2E-F-17 | 预探查 | `gui-test-screenshots/` 25 个测试截图散落根目录且已被 git 跟踪 | 历次 GUI 测试直接落盘根目录，未归档到 `docs/evidence/` | E2E-02 | ✅ **已解决**（2026-09-17，commit a44ee4a，git mv + 7 处引用更新） |
| E2E-F-18 | 预探查 | 残留空目录与一次性产物：`emotion-echo-web;D`（0 字节，shell 分号误建）、`docker-images-before.txt` | shell 未转义分号建目录；一次性快照未清理 | E2E-02 | ✅ **已解决**（2026-09-17，commit a44ee4a） |
| E2E-F-19 | 预探查 | `users` 表 3 个死字段：`email`（零读写）、`phone`（仅响应回显、零写入→恒 NULL）、`status`（零读写） | `deploy/db/02-create-tables-in-schemas.sql:10-11,17`；详见 [findings §5.2](findings/2026-09-17-pretest-panorama.md) | E2E-06 | ✅ **已解决**（2026-09-18，E2E-06：u001_drop_dead_fields.sql + proto/model/types/logic/grpcserver/BFF 全链路清理） |

### B. 覆盖盲区排查（2026-09-17，E2E-F-20~29）

| 编号 | 来源 | 现象 | 根因 | 归属阶段 | 状态 |
|------|------|------|------|---------|------|
| E2E-F-20 | 盲区排查 | **全仓零 CI**：`.github/` 目录不存在，`docs/ci-workflows/` 的 3 份模板从未执行 | PAT 只有 `repo` scope，GitHub 拒绝 push `.github/workflows/*.yml`。历史审计 `audit-2026-08-31.md:155` 列为 P1"零 CI/CD"，修复项 R-9"落地最小 CI，先于一切新功能"至今未做 | E2E-03 | 🔴 未解决 |
| E2E-F-21 | 盲区排查 | 前端零工程化门槛：无 ESLint/Prettier 配置、`package.json` 无 `lint` script、typecheck 有 **96 处历史错误基线**、无构建产物 smoke、Playwright 仅单 chromium project、无 browserslist | AGENTS.md §2.2 的"合并前 `npm run lint`"是**空条款**（无 eslint 依赖） | E2E-04 | 🔴 未解决 |
| E2E-F-22 | 盲区排查 | `check_docker_digests.sh` **假绿**：只校验 FROM 行格式含 `@sha256:`，而 `Dockerfile.digests.lock` 中 **7 个 digest 是 `sha256:000...000` 占位值**（文件头 `:20-22` 自述） | 检查器只验形式不验实质；沙箱网络不可达 docker.io 无法回填，但缺口被静默掩盖 | E2E-05 | 🔴 未解决 |
| E2E-F-23 | 盲区排查 | **ADR-18 防线全面失效**：10 个校验脚本（路由对齐/视图一致性/env 变量/迁移契约/JWT secret 一致/git 布局/docs 更新/digest/TLS…）全部 CI-shaped 但**全部无人在跑** | 依赖 CI 执行才生效，而 CI 不存在（E2E-F-20）。`check_view_consistency.py` 文档字符串明写"CI 阶段跑：发现 diff 即 fail"——该前提从未成立 | E2E-05 | 🔴 未解决 |
| E2E-F-24 | 盲区排查 | 3 处实测文档失真：① `stage-21-k8s-strategy.md:27` 称 "`deploy/tls/*.key` 提交进 git"，实际未提交（gitignore 已忽略）② `docs/ci-workflows/web-test.yml` 的 `lint` 步骤引用不存在的 script ③ `deploy/env/.env.common` 腐烂（`APISIX_VERSION=3.9.0` vs 实际 3.18.0；`GIN_BACKEND_HOST` 指向已迁 `legacy/` 的 Gin；compose 不引用） | 失真属 ADR-18 已分类的"未复跑即记录/陈旧结论"；③ 未被发现的原因是 `lint_env_vars.sh` 只校验 `.env.local.example`，不校验 `.env.common` | E2E-05 | 🔴 未解决 |
| E2E-F-25 | 盲区排查 | 多实例下 3 处防护**静默失效**：BFF 登录失败锁定（5 次错密码）、验证码 60s 防枚举、APISIX 限流（`policy: local`） | 前两者 in-memory（`auth_handler.go:14,15,66` 注释自述"单实例假设；多实例留 Stage 34+ Redis"）；APISIX `seed.sh:318-325` `policy: local` 每节点各自计数。而决策 3 是"本地 Docker **单机多实例**"，故必须正确 | E2E-20 | 🔴 未解决 |
| E2E-F-26 | 盲区排查 | **无性能/延迟基线**：无压测脚本（k6/locust/vegeta/wrk 全无）、无 p50/p95 目标、无资源预算 | 埋点已就位（fusion histogram、LLM `request_duration_seconds`、gRPC latency 日志）但**无阈值消费**。XTTS 性能图是上游 vendor 自带，非本项目实测 | E2E-28 | 🔴 未解决 |
| E2E-F-27 | 盲区排查 | **无备份/恢复/回滚机制** | 全仓无 `pg_dump`/`pg_restore` 脚本；`migrate.sh` 是幂等重放模型，无 down 脚本；`05-drop-user-oauth.sql` 是**单向破坏性**迁移 | E2E-19 | 🔴 未解决 |
| E2E-F-28 | 盲区排查 | JWT 密钥无轮换机制 | `BFF_JWT_SECRET` 在 `apps.yml:645,709` 与 `seed.sh:61` 用同一默认值 `dev-bff-secret`；轮换需 BFF 与 APISIX **原子一致**否则全部 token 失效。`test_bff_jwt_secret.sh` 是一致性检查而非轮换机制 | E2E-29 | 🔴 未解决 |
| E2E-F-29 | 盲区排查 | 国际化（i18n）完全不存在 | 无 vue-i18n、无 `locales/` 目录，UI 文案硬编码中文；`playwright.config.ts` 写死 `locale: 'zh-CN'`。属产品决策 | **D-04 候选**（不列 E2E 阶段） | 🟡 候选未决 |

### C. CI 模板严格性评审（2026-09-17，E2E-F-30~35）

> 触发：用户要求评审 `docs/ci-workflows/` 的 3 份模板"测试是否严谨"。结论：**不严谨，有 18 处缺陷**。逐条做法已写入 [E2E-03 plan](stages/e2e-03-ci-gate/plan.md) §2.3。

| 编号 | 来源 | 现象 | 根因/证据 | 归属阶段 | 状态 |
|------|------|------|----------|---------|------|
| E2E-F-30 | CI 评审 | Go workflow 不严谨：**Go 版本不匹配**（CI 写死 `1.22`，全部 7 个 `go.mod` 为 `go 1.26.1`）、无 `-race`、无覆盖率强制（AGENTS.md §2.3 底线不可执行）、`GOFLAGS: -mod=mod` 削弱可重现性、无格式检查、无 `timeout-minutes`/`concurrency`/`permissions` | `docs/ci-workflows/go-test.yml`；`go.mod` 实测 1.26.1 × 7 | E2E-03（阶段 2） | 🔴 未解决 |
| E2E-F-31 | CI 评审 | **23 个集成测试永不执行** | `*_integration_test.go` 挂 `//go:build integration`，CI 只跑 `go test ./...`（无 `-tags integration`）；且这些测试需真实 PG/Kafka | E2E-03（阶段 2） | 🔴 未解决 |
| E2E-F-32 | CI 评审 | 4 套测试套件 CI 零覆盖：`emotion-echo-models` **20 个项目 pytest 文件**、`scripts/test_*.py`（4）、`k8s/tests/*_test.go`（6）、`deploy/*.test.js`（1） | 3 份模板均未涉及这些目录 | E2E-03（阶段 2） | 🔴 未解决 |
| E2E-F-33 | CI 评审 | **main 无分支保护** + 仓库为 **public** ⇒ `docs/ci-workflows/README.md` 声称的"任何 test 失败 → PR 不可 merge"**不成立** | GitHub API 原为 `404 Branch not protected`；`visibility: public` | E2E-03（D1）/ E2E-05（措辞校正） | 🟡 **部分解决**（2026-09-17）：已开防强推+防删除+`enforce_admins=true`（实测强推被拒 `GH006`）；**status checks 与 PR 要求待 CI 落地后再开** |
| E2E-F-34 | CI 评审 | LLM workflow **依赖未锁版本** ⇒ 同一 commit 可绿可红 | `emotion-llm-service/requirements.txt` 全用 `>=`（`fastapi>=0.100.0`、`openai>=1.40.0`…），无锁定文件 | E2E-03（阶段 2） | 🔴 未解决 |
| E2E-F-35 | CI 评审 | 前端版本声明缺失 ⇒ CI 版本写死漂移风险 | `package.json` **无 `engines`、无 `packageManager`**，而 CI 写死 `node-version: '20'` / `pnpm version: 9` | E2E-03（阶段 2）/ E2E-04 | 🔴 未解决 |
| E2E-F-36 | E2E-01 实测 | 报表、用户空间等页面**无法上下滑动**（内容溢出时无滚动条） | 待查（疑似 `overflow: hidden` 或 `height: 100vh` 无 `overflow-y: auto`） | E2E-04（全局布局修复） / E2E-11（用户空间） / E2E-15（报表） | 🔴 未解决 |
| E2E-F-37 | E2E-01 实测 | SSR 模式下 **3 个 Playwright spec 因 hydration 时序失败**（dashboard-flow 2 + chat-flow happy-path-2 + login-flow 1） | SSR 渲染的按钮 `visible` 但 Vue click handler 未挂载；需 `waitForLoadState('networkidle')` + `toBeEnabled()` 等 hydration 完成 | E2E-04（Playwright 基础设施规范化） | ✅ **全部修复**（2026-09-17，commits 8100c5f + 03a4361 + 59190a8） |
| E2E-F-38 | E2E-01 实测 | IAB（内置浏览器）**无法通过 `document.cookie` 设置 cookie** | IAB 的 cookie jar 独立于 `document.cookie` API；`Set-Cookie` 响应头可写入但 JS 侧读写受限 | 不归属阶段（IAB 工具限制，非产品 bug） | 🟡 已知限制（E2E 测试改用 Playwright cookie API 绕过） |
| E2E-F-39 | E2E-02 实测 | vitest `useAIStreamHandler.test.ts` **预存失败**：`#app` import 无法解析 | `clientAccessToken.ts:2` 引用 `import { useCookie } from "#app"`，vitest 无 Nuxt `#app` alias 配置 | E2E-03 | ✅ **已解决**（2026-09-17，commit b9ddc85：`tests-app-mock.ts` + vitest alias 修复，47/366 全绿） |
| E2E-F-40 | E2E-03 CI | **Go CI 全部 7 模块测试失败**（go-test #3~7）：`go vet` 报 unreachable code + context.WithCancel leak；`-race` flag 在 CI Go 1.26.1 不可用导致全模块 exit code 1 | 3 个真实代码 bug 已修（1148ae0 + a5cd698）；`-race` 去掉后 go-test #8 全绿 | E2E-03 | ✅ **已定性**（2026-09-18 R-02 #15）：本地验证 `-race` 报 `exit status 0xc0000139`（Windows DLL 错误），确认为**工具链环境问题**而非数据竞争。**处置**：批准永久降级（D-06），残留风险=无（Go 1.26.1 + Windows + CGO_ENABLED=1 组合不支持 race detector）。CI 保持去掉 `-race` |

### D. E2E-01~06 独立审查发现（2026-09-18，E2E-F-41~59）

> 触发：用户要求"查看推进的代码和文档，看是否合规"。审查结论：**五个已标 done 的阶段无一完整满足 [RUNBOOK.md](RUNBOOK.md) §7 收口契约**，另有 1 个安全级缺陷、2 个功能性破坏、1 个持续红的 CI。
> 错误模式已固化为 [anti-patterns.md](anti-patterns.md)（14 类），补救排期见 [remediation.md](remediation.md)（R-01/R-02/R-03）。

| 编号 | 严重度 | 现象 | 根因/证据 | 归属 | 状态 |
|------|--------|------|----------|------|------|
| E2E-F-41 | 🔴 **安全** | **BFF 密保校验 fail-open**：`Login(username,"dummy")` 当用户存在性探测，真实用户密码非 dummy ⇒ 恒走 `err != nil` ⇒ **恒返回 `success:true`，答案从不校验**。且 BFF 调 `/api/v1/users/verify-security-answer` 而 user-svc 无此路由（HTTP 路径 404） | `bff/internal/handler/auth_handler.go:478-481`、`bff/internal/downstream/user.go:214`、`user-svc/main.go:152-168` | **R-01** | ✅ **已解决**（2026-09-18：user-svc 添加 `/verify-security-answer` 路由 + `VerifySecurityAnswerByUsername` 方法；BFF 改用新方法，不再用 Login 探测用户存在性） |
| E2E-F-42 | 🔴 **功能** | **注册链路断裂**：后端强制 1~2 个密保，前端仍只发 `{username,password,verificationCode}`，全仓 grep `securityQuestions` 零命中 ⇒ 唯一注册入口 100% 返回 400 | `bff/.../auth_handler.go:172-180`、`user-svc/.../authlogic.go:96-102`、`web/app/pages/login/index.vue:147` | **R-01** | ✅ **已解决**（2026-09-18：D-05 决策落地，密保改为可选，注册恢复可用；前端录入 UI 归 E2E-09） |
| E2E-F-43 | 🔴 **阻断** | **测试未同步 → 编译失败**：26 个改动文件零 `_test.go` 变更；`go vet ./...` 实测报 4 个 `unknown field Phone`（model/repository/logic/handler 四处）；BFF avatar 测试的 fake 不满足新 `UserClient` 接口 | `user-svc/internal/{model,repository,logic,handler}/*_test.go`、`bff/internal/handler/avatar_handler_test.go:59`。report 却称"编译验证全通过"——`go build` 不编译测试文件（AP-09） | **R-01** | ✅ **已解决**（2026-09-18：修复 6 处 Phone 引用 + fakeUserClient 接口签名 + gorm.DeletedAt 断言；7 模块 `go vet` 全绿） |
| E2E-F-44 | 🟡 | **Register 非事务**：先建用户再存密保，后者失败则用户行已落库 ⇒ 库中留下"无密保用户"，与"密保不可跳过"相悖 | `user-svc/internal/logic/authlogic.go:119-146` | **R-01** | 🟡 **降级**（2026-09-18：D-05 把密保改为可选后，影响降低——无密保用户仍可正常使用；repository 无事务支持，需架构改动） |
| E2E-F-45 | 🟡 | **迁移 checksum 校验是死代码**：返回码 2 被 `if` 吞掉，`record_migration` 用 `ON CONFLICT DO UPDATE SET checksum=...` 覆盖新校验和 ⇒ "迁移文件被改动"被静默放行，与 `deploy/db/README.md` 行为表直接矛盾 | `deploy/db/migrate.sh:164-173` | **R-01** | ✅ **已解决**（2026-09-18：`check_migration` 返回码 2 正确处理，die 而非忽略） |
| E2E-F-46 | 🟡 | **死字段仍在权威 DDL**：`02-create-tables-in-schemas.sql:10,11,17` 仍有 `phone`/`email`/`status`，与 `chat-svc/migrations/008_p0r27_ddl_drift_test.go` 的"表定义唯一源=02"契约冲突 ⇒ 新引入的漂移源 | 同左 | **R-02** | ✅ **已解决**（2026-09-18 第二方核对：`grep -n "phone\|email" deploy/db/02-create-tables-in-schemas.sql` 零命中；残留 `status` 属 `chat.conversations` / 量表表，为合法业务字段） |
| E2E-F-47 | 🟡 | **演示账号"可删"不成立**：`03-seed-default-users.sql:18-21` 仍硬编码 `echo/echo123` 于 initdb.d（plan:54 明确禁止），而 `cleanup-demo-account.sh:18` 默认清理目标正是 `echo` ⇒ 删了→空卷重建→复活。另：cleanup 漏 6 张含 `user_id` 的表（`ai.emotion_analysis`/`voice_transcripts`/`face_detections`、`assessment.survey_results`/`mental_health_assessments`/`reports`）；DB 不可达时打印 "nothing to clean up" 并 `exit 0`（把连接失败伪装成成功） | 同左 | **R-01 / R-02** | ✅ **已解决**（2026-09-18 第二方核对：initdb 现只种契约账号 `smoke_user`；演示账号 `echo` 归 `deploy/db/seed-demo-account.sh` / `cleanup-demo-account.sh`；cleanup 覆盖 12 张含 `user_id` 表 + `die "Database connection failed"` 非零退出） |
| E2E-F-48 | 🟡 | **孤儿产出物**：`deploy/db/06-create-schema-migrations.sql` 永不执行（initdb.d 只挂 01~05；migrate.sh 发现逻辑是 `*/migrations`，而 `deploy-db/` 下无该子目录），却被 README 与 report 列为交付物 | `deploy/docker-compose.infra.yml:28-33`、`deploy/db/migrate.sh` 发现逻辑 | **R-02** | ✅ **已解决**（2026-09-18 第二方核对：该文件已删除） |
| E2E-F-49 | 🟡 | **`main.go` 注释与代码相反**：注释仍写"Stage 77：失败按 500ms×10 退避重试"，代码已换成单次 `openPostgresDB` ⇒ 在与"数据库改造"无关的维度上做了行为回退 | `user-svc/main.go:84-93` | **R-02** | ✅ **已解决**（2026-09-18 第二方核对：`user-svc/main.go:84` 注释已改为"单次连接，失败降级 nil repo"，与代码一致；`chat-svc/main.go:102` 的退避重试已恢复为 `dbconnect.ConnectWithRetry`） |
| E2E-F-50 | 🔴 | **CI 在 main 持续红且拦不住**：`doc-drift-check` 在 `2cb9e58`/`0644988` 连续红（3 job fail：env 变量 8 项未文档化、6 个占位 digest、migration 顺序 Fail 13）。分支保护 API 实测**无 `required_status_checks` 与 `required_pull_request_reviews` 键**；22 次 run 全 `push`、零 PR ⇒ `docs/ci-workflows/README.md:51`「任何 test 失败 → PR 不可 merge」与 ADR-18 §8.3「修复后才能合并」**均不成立** | GitHub API `/branches/main/protection`、`/actions/runs` | **R-01 / R-03** | 🟡 **部分解决**（2026-09-18：env lint 8 项已补 + migration 顺序添加 analytics-svc 豁免；digest 占位客观无法本地修复，按 D-07 改为显式 WARN） |
| E2E-F-51 | 🔴 | **E2E-04 四个测试点假 PASS**：#4 声称"web-test 已含 typecheck 步骤"——实测该文件只有 `Install deps` + `vitest`（**与代码事实相反**）；#5 build smoke 只"脚本已创建"（断言 `.output/public/index.html` 而该文件不存在）；#6 mobile project 只"配置已新增"未跑过；#7 a11y spec 末段断言被注释掉（永不失败的 soft-assert） | `stages/e2e-04-frontend-engineering/report.md:16,17,26,27`、`.github/workflows/web-test.yml` 全文 | **R-02** | 🟡 **部分解决**（2026-09-18 第二方核对：报告已如实改写为"❌ 假 PASS / ⚠️ 未验证"；**四项能力缺口本身仍存在**——typecheck 未进 web-test、build smoke 未跑、mobile project 未跑、a11y 断言仍被注释） |
| E2E-F-52 | 🔴 | **SSR 切换无决策记录 + 文档大面积失效**：`6c91525`（8 文件 +113/-28）把 `ssr:false→true`，但 `docs/architecture/adr/` 15 个 ADR 无一条涉及渲染模式、`decisions.md` 决策表与变更记录均无条目（违反 AGENTS.md:332）；**7+ 处文档仍写"项目是 SPA"**，最刺眼的是 `e2e-04/plan.md:38`「**不引入 SSR**」与已标 done 并存；切换引入的真实回归（3 个 spec hydration 失败）被归到"Playwright 基础设施规范化"（产品回归按测试问题归档） | 同左 + `docs/architecture/decisions.md` | **R-02** | 🟡 **部分解决（2026-09-18 补齐 ADR）**（文档更正已于前轮完成；**ADR 与 decisions.md 登记本轮补齐**：docs/architecture/adr/adr-2026-09-nuxt-ssr-mode.md + decisions.md 决策 24。剩余：Playwright mobile project / a11y 强制属 E2E-04 阶段 2） |
| E2E-F-53 | 🔴 | **收口契约系统性缺口**：E2E-03 report 非模板（无环境基线/汇总/判定列，21 点中 13 点无结果）；E2E-06 report 缺 4 节且 `[ ] 补 integration test` 未勾选却标 `done`；E2E-01/03/04/05 的 `[V]` 测试点**截图 0 张** | 各 `stages/*/report.md` | **R-02 / R-03** | 🔴 未解决 |
| E2E-F-54 | 🔴 | **账本与 roadmap 状态脱钩**：`E2E-F-21/22/23/24/30` 全部仍挂 🔴 未解决，而其所属 E2E-03/04/05 已标 ✅ done ⇒ 账本失去"单一事实源"作用 | `discovered-unresolved.md` vs `roadmap.md` | **R-02 / R-03** | 🔴 未解决（2026-09-18 第二方核对补充：**双向漂移**——除上述"未解决却标 done"外，E2E-F-46/47/48/49/58/59 已修复却仍挂"未解决"；后者会让审计器 A5 产生**假 FAIL**） |
| E2E-F-55 | 🟡 | **偏离计划无记录**：E2E-03 plan §2.3 的 A4（覆盖率，AGENTS.md §2.3 的 80/90/70 底线）/A5（23 个集成测试）/A6（格式检查）三项在 report 的严格化表里**三行都没有**；且 plan 实列 **23 条**而账本写"18 处"（漏算 5 条）。实测已修 7 条（30%）、未修 14 条（61%） | `stages/e2e-03-ci-gate/report.md` §二、`.github/workflows/*` 实测 | **R-03** | 🔴 未解决 |
| E2E-F-56 | 🟡 | **新增 workflow 回退既有加固标准**：`.github/workflows/doc-drift-check.yml` 的 11 个 job **全部缺 `timeout-minutes`/`concurrency`/`permissions`**——正是 E2E-03 的 A7/A8/A9 且已应用到另外 3 个 workflow | 同左 | **R-03** | 🔴 未解决 |
| E2E-F-57 | 🟡 | **状态与数字多处不一致**：E2E-05 `plan.md` 至今 `status: pending`（roadmap ✅ done / report done）；`roadmap.md:13` 坏链指向 `e2e-07-forgot-password`（实际 `e2e-07-password-recovery`）；E2E-05 report 汇总行留占位符 `PASS x`；同一事实三种结论（`test_migrations_no_service_order.sh`：CI FAIL / ADR-18 记 WARN / report 记 PASS）；计数矛盾（失真"3 处/2 处/4 处"）；typecheck `96` vs `103` 未解释 | 同左 | **R-02 / R-03** | 🟡 **部分解决**（2026-09-18 第二方核对：坏链已修正、`PASS x` 占位符已清）；**仍存活**：E2E-05 `plan.md`=`partial` 而 roadmap/report=`done`（审计器 A9 实测 FAIL），另 `seed.sh` 路由计数见 E2E-F-67 |
| E2E-F-58 | 🟢 | **孤儿代码**：`emotion-echo-web/e2e/helpers/auth.ts` 无任何 spec 引用（grep 零命中），而 E2E-06 测试点 14「spec 依赖已处理」标 PASS；`emotion-echo-web/.git-blame-ignore-revs` 位置错误（放子目录，而文件自身注释按仓库根解析）⇒ 登记不生效 | 同左 | **R-02** | ✅ **已解决**（2026-09-18 第二方核对：`auth.ts` 已删除、`.git-blame-ignore-revs` 已在仓库根）。**遗留**：`emotion-echo-web/e2e/helpers/` 成为**空目录**，`git status` 不可见，`check_residual.sh` 未覆盖 ⇒ 见 E2E-F-62 |
| E2E-F-59 | 🟢 | **同类污染复发**：`deploy/db/migrate.sh;D/` 空目录（shell 分号误建），正是 `E2E-F-18` 已"解决"过的同类问题；空目录不出现在 `git status`，收口自检发现不了 | `deploy/db/` | **R-02 / R-03** | ✅ **已解决**（2026-09-18 第二方核对：`find . -name "*;D"` 零命中） |
| E2E-F-60 | 🔴 | **R-01 #2 实际未通——网关未放行找回密码端点**：`user-svc/main.go:158` 与 BFF `auth_handler.go:458` 两端都已实现 `/api/v1/auth/verify-security-answer`，但 `deploy/apisix/seed.sh:540-546` 的白名单只有 110~115（login/register/verification-code/refresh/logout/reset-password）。该路径落到 route 100（`/api/v1/*`，挂 jwt-auth）⇒ **未登录态的找回密码调用必然 401**。前端经 APISIX `:19080` 调用（`emotion-echo-web/.env:5`），故链路不可达 | `deploy/apisix/seed.sh:540-546,499`、`emotion-echo-web/.env:5` | **R-01** | ✅ **已解决**（2026-09-18：**三层**根因全修——① APISIX 补 `put_auth_route 117`；② BFF gRPC 客户端由 not-implemented 桩改为真实 RPC（proto 新增 VerifySecurityAnswerByUsername 并两端实现）；③ user-svc 拦截器匿名跳过清单补入。端到端实测：错答案 401 / 正确答案 200 / 未知用户 401） |
| E2E-F-61 | 🟡 | **BFF 层密保校验无回归钉**：负向测试只落在 user-svc logic 层（`authlogic_test.go:315-390`，5/5 PASS），而原缺陷所在层 `bff/internal/handler/auth_handler.go:458-497` 在 `auth_handler_test.go` 中**零引用** ⇒ 同类 fail-open 复发时测试抓不到 | `emotion-echo-web-bff/internal/handler/auth_handler_test.go` | **R-01** | ✅ **已解决**（新增 security_answer_handler_test.go 5 用例；且用**负向对照**证明其有效——把 handler 临时回退为旧 fail-open 实现后 WrongAnswer/UnknownUser 两条立即变红） |
| E2E-F-62 | 🟡 | **收口残留物（§2.5 未执行）**：① `chore/trigger-ci` 分支仍在**本地与远端且未并入 main**（为注册 status check 而建，目标却未达成）；② `main` 领先 `origin/main` 1 个 commit（`48ac75b` 未 push）；③ `emotion-echo-web/e2e/helpers/` 空目录残留。三项均不出现在 `git status` 或现版 `check_residual.sh` 的覆盖范围 | `git branch -a`、`git status -sb`、`scripts/check_residual.sh` | **R-03** | ✅ **已解决**（删除残留分支 chore/trigger-ci + 空目录 emotion-echo-web/e2e/helpers/） |
| E2E-F-63 | 🔴 | **`-race` 降级违背 D-06**：账本 E2E-F-40 记"✅ 已定性…残留风险=无（Go 1.26.1 + Windows + CGO_ENABLED=1 组合不支持 race detector）"，唯一证据是**本地 Windows** `0xc0000139`。而 D-06 明文判定"用 Windows DLL 错误解释 **Linux CI** 的 exit 1"**证据链不成立**，并要求 3 步可复现调查（Linux 容器复现 / 空测试验证 / 读 CI 首条错误行）。CI 跑在 Linux，且"批准人"无记录 ⇒ 属 AP-05（用删除关闭需求）+ AP-06（根因臆断） | `decisions.md` §D-06、`discovered-unresolved.md` E2E-F-40、`.github/workflows/go-test.yml:42-43` | **R-01 / R-02** | ✅ **已解决**（D-06 三步调查完成，结论与 R-02 #15 相反：**不是环境问题，是真实数据竞争**——CI 上 shared-test/web-bff 在 -race 下通过，5 个业务模块均报 WARNING: DATA RACE，本地 Linux 容器可复现；5 模块 listener 字段加 sync.RWMutex 修复 + 每模块一条契约测试 + CI 恢复 -race） |
| E2E-F-64 | 🟡 | **审计器 A4 对"诚实陈述缺口"误报，且误报已固化进回归基线**：E2E-04 报告行 #4~#7 证据为"脚本已创建**但** `.output/public/index.html` 不存在"/"配置已新增，**但未实际运行**"，A4 按存在性关键词命中判 FAIL，无法区分"以存在性充当完成证据"与"报告缺口"；`SELFTEST_EXPECT["e2e-04"]` 又把该命中写成期望值。后果：E2E-04 的两个 FAIL（A4+A5）皆属**工具/账本假象**而非真实缺口，会持续污染审计结论 | `scripts/e2e_stage_audit.py`（A4/A4b、`SELFTEST_EXPECT`）、`stages/e2e-04-frontend-engineering/report.md:24-27` | **R-03** | ✅ **已解决**（新增 row_claims_pass() 只审「声称通过」的行；新增 SELFTEST_MUSTNOT 显式登记「e2e-04 不得触发 A4」；新增 A4 合成样本（正例触发/反例不触发）与 row_claims_pass 表驱动 8/8；selftest 全绿） |
| E2E-F-65 | 🟡 | **§13.3 有 2 条断言既无实现也无人工理由**：#8（soft-assert 检测）与 #16（`required_status_checks` 非空）全仓无实现——`grep -rn "soft.assert\|required_status_checks" scripts/ .github/` 零命中。其中 #16 正是仍然存活的 AP-11（实测 `contexts=[]`/`checks=[]`），属最该机械化的那条 | `RUNBOOK.md` §13.3、GitHub API `/branches/main/protection` | **R-03** | ✅ **已解决**（#8 → scripts/check_soft_asserts.sh + 负向测试 5/5 + 白名单 + 接入 CI；#16 → scripts/check_required_checks.py（断言非空 + 禁止把带 paths 过滤的 job 列为 required）。#16 因读分支保护需管理员权限、默认 GITHUB_TOKEN 无权，故列为收口时第二方核对的必跑命令） |
| E2E-F-66 | 🟡 | **TDD 门禁范围漏洞 + 措辞过度**：`check_tdd_gate.sh` 的 `PROD_PATTERNS` 仅含 `\.go$`/`\.vue$`/`\.ts$`，不含 `\.sh$`/`\.py$` ⇒ 新增脚本（`check_residual.sh`、`e2e_stage_audit.py`）一律免检，而脚本类产出正是 AP-13 高发区；绿字却称"所有 5 个 commit 满足 TDD 要求"，实为范围外绿灯 | `scripts/check_tdd_gate.sh:32-37` | **R-03** | ✅ **已解决**（check_tdd_gate.sh 输出限定为「已判定范围内」，并显式枚举 scripts/ 下 48 个无同名负向测试的脚本；未硬性强制以免制造大面积假红，属 R-03 #7 收尾） |
| E2E-F-67 | 🟢 | **`seed.sh` 路由计数文案漂移**：`deploy/apisix/seed.sh:578` 打印"12 routes (1 catch-all + 5 health + 5 auth-whitelist + 1 self-health)"，实为 6 条 auth 白名单（110~115）⇒ 总数应为 13。补 116 号路由时须一并更正 | `deploy/apisix/seed.sh:578` | **R-01** | ✅ **已解决**（末尾计数改为 grep 脚本自身自维护；实测输出 14 routes (1 catch-all + 5 health + 7 auth-whitelist + 1 self-health)，与实际一致） |
| E2E-F-68 | 🔴 **阻断** | **main 写保护自锁死——任何人（含管理员）都无法写入**：分支保护实测 `enforce_admins=true` + `required_pull_request_reviews.required_approving_review_count=1` + `allow_force_pushes=false` + `allow_deletions=false`，而仓库**只有一个协作者**（`Exist-a`，唯一 admin）。GitHub 不允许自我 approve ⇒ **直推被拒**（实测 `git push` 返回 `remote rejected ... Changes must be made through a pull request`）**且 PR 也无法满足 review 要求**。后果：`48ac75b`（R 系列落地文档）与 `3662fae`（第二方核对修正）两个 commit **无法交付**；D-08 只讨论了 `required_status_checks` 的锁死风险，未覆盖"强制 PR + 1 approve + 单协作者"这一更强的锁死形态 | `gh api /branches/main/protection`（`enforce_admins.enabled=true`、`required_approving_review_count=1`）、`gh api /repos/.../collaborators`（仅 1 人）、`git push origin main` 实测 | **R-03** | 🔴 未解决（**已由 PR #2 硬证实**：`gh pr view 2` → `mergeable=MERGEABLE state=BLOCKED review=REVIEW_REQUIRED`，且该 PR 的 18 项 CI **全部 pass** ⇒ 唯一阻塞项就是那个"永远凑不够的 approve"，与 CI 无关；该 PR 同时证明 **PR 触发的 CI 本身可正常工作**，故症结在保护配置而非 CI） |
| E2E-F-69 | 🔴 **安全** | **APISIX 管理员密钥硬编码在公开仓库**：deploy/apisix/seed.sh:60 与 deploy/docker-compose.apps.yml:712 均以 `${APISIX_ADMIN_KEY:-WhZ…}` 形式把**真实默认值**写进源码（仓库为 public）。该 key 可 PUT/DELETE 任意 route 与 upstream，等同网关配置的完全控制权。dev 环境虽只在本机暴露，但「密钥进版本库」本身即违背密钥管理基线 | deploy/apisix/seed.sh:60、deploy/docker-compose.apps.yml:712 | **R-01 / R-03** | 🔴 未解决 |
| E2E-F-70 | 🔴 **阻断（流程）** | **dev 栈镜像陈旧 ⇒ 任何浏览器/curl 验收都会验错对象**：本轮修 fail-open 后 curl 仍返回 200（旧行为），排查发现运行中的 web-bff 镜像构建于修复提交之前（容器 Up 6h、镜像 Created 02:00 UTC，而修复提交在 04:00 之后）。同类风险普遍存在：**先改代码、不重建镜像就直接验收**会得到「改前」的结论（无论结论是「已修复」还是「仍坏」） | docker inspect emotion-echo-web-bff（镜像 Created 时间 vs 代码提交时间） | **R-01 / RUNBOOK** | 🔴 未解决（建议：收口契约增加「验收前记录被验镜像 ID」一步） |
| E2E-F-71 | 🔴 | **gRPC 匿名 RPC 跳过清单漏配**：user-svc/internal/grpcserver/server.go 的 newServiceAwareUserIDInterceptor 跳过清单只有 Login/Register/ResetPassword，漏了 proto 与 HTTP 端都定义为匿名的 VerifySecurityAnswer ⇒ 「契约说匿名、实现要求带身份」，密保校验在 gRPC 路径上被拦截器以 Unauthenticated 拒掉 | emotion-echo-user-svc/internal/grpcserver/server.go:80-90 | **R-01** | ✅ **已解决**（已补入两个方法 + 新增匿名调用测试。本条目同时登记缺陷类别：**匿名清单与 proto 契约之间无机械一致性校验**） |
| E2E-F-72 | 🟡 | **BFF gRPC 客户端存在「未实现」桩**：userGRPCClient.VerifySecurityAnswerByUsername 原为 `return fmt.Errorf("not implemented for gRPC transport, use HTTP")`，而 BFF 默认 transport 即 gRPC ⇒ HTTP 侧两端都通、默认路径是空实现（「端点已补」≠「功能可用」的典型） | emotion-echo-web-bff/internal/downstream/user_grpc.go:170-174 | **R-01** | ✅ **已解决**（改为真实 RPC；并用 bufconn 真 gRPC server 测试钉住，含「必须真的发起 RPC」断言以防桩下假通过） |
| E2E-F-73 | 🔴 | **5 个服务模块同源数据竞争**：grpcserver.(*Server).listener 由 Start() 在后台 goroutine 写入、Addr() 被并发读取，无同步。5 份拷贝（user/chat/ai/analytics/assessment）在 -race 下均报 WARNING: DATA RACE | 各 emotion-echo-*-svc/internal/grpcserver/server.go | **R-01 / R-02** | ✅ **已解决**（加 sync.RWMutex + 每模块一条 server_race_test.go；负向对照：撤锁后该测试报 7 次 DATA RACE、加锁后 0 次） |
| E2E-F-74 | 🟡 | **鉴权判定口径两侧不一致（结构性风险）**：BFF 用 authPathBypass 放行**整个** /api/v1/auth/ 前缀，而 APISIX 是**逐条**白名单 ⇒ 新增 auth action 时两侧必然漂移（E2E-F-60 即此类） | bff/main.go:257 vs deploy/apisix/seed.sh:540-560 | **R-03** | ✅ **已解决（以契约测试兜住）**（check_routes_alignment.sh 契约 3：BFF auth action 集 ⊆ APISIX 白名单。根治方案是让 BFF 也改为逐条白名单，属后续重构） |

## 与 R-xx 体系衔接

- 本账本追踪"E2E 阶段发现"的完整生命周期（发现 → 归属 → 排期 → 修复 → 回填）
- R-xx 体系（`docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md`）是运行时 bug 的权威编号：本账本条目修复落地后，回填 R 系并互相引用
- 建档预探查 19 项 + 覆盖盲区排查 10 项 + CI 模板评审 6 项 + E2E 实测 5 项 + **E2E-01~06 独立审查 19 项** + **R 系列第二方核对 9 项（E2E-F-60~68）** + **修复过程新发现 6 项（E2E-F-69~74）** = **74 项**；实测阶段若有新发现继续追加 `E2E-F-75` 起
- 严重度图例：🔴 阻断/安全 · 🟡 契约缺口 · 🟢 清理项

## 不列入 E2E 阶段的候选（已评估）

| 候选 | 结论 |
|------|------|
| 生产部署（`compose.prod.yml` 空壳 + Helm chart 冻结） | **决策 3 / 决策 23 已明示冻结**，不单列。仅在 E2E-30 保留离线 `helm template`/`lint` 渲染回归项 |
| a11y | 折入 E2E-04（需引入 axe 依赖，不宜纯折入业务阶段） |
| 响应式/移动端 | 折入 E2E-04（仅 2 文件 6 处引用，单列投入产出不匹配） |
| 浏览器兼容 | 折入 E2E-04（补 browserslist + firefox 可选 project） |
| 配置与密钥 | 拆入：digest 假绿→E2E-05；Nacos→E2E-23；JWT 轮换→E2E-29 |
| i18n | 登记为 **D-04 候选**（E2E-F-29），不列阶段 |
