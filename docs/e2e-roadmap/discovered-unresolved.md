---
status: active
priority: high
created: 2026-09-17
last-refresh: 2026-09-17 (29 项；含覆盖盲区排查新增的 10 项)
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
| E2E-F-01 | 预探查 | 找回密码/注册的验证码无真实投递渠道，流程仍按手机号短信时代设计 | 项目已改用户名登录；`BFF_DEV_RETURN_CODE=1` 仅 dev 回显 | E2E-07（+E2E-06 供字段） | 🟡 方案已定（D-01=C 密保问题），待实施 |
| E2E-F-02 | 预探查 | 心理测验三层契约错位：提交必 400、结果弹窗"等级"恒空、列表页取数失败报错 | 前端发 `answers` 数组 vs 后端要 `map[string]int`；前端读 `level`/`suggestion` vs 后端回 `riskLevel`；前端读 `data.list` vs 后端回 `{items,total}` | E2E-13 | 🔴 未解决 |
| E2E-F-03 | 预探查 | 现工程量表种子数据不存在，surveys 表为空 | `deploy/db/` 无 INSERT 量表的 SQL；文档声称的 `seed-surveys.sql` 在 git 全历史中不存在 | E2E-13 | 🔴 未解决 |
| E2E-F-04 | 预探查 | "人格测试→心理预测→AI 提示词定制"链路完全不存在 | 量表是症状自评非人格量表；评分无维度/画像；BFF system prompt 写死静态字符串；proto 无画像字段；legacy 挂点 `BuildSurveyContext` 函数体 `return ""` | E2E-14 | 🟡 方案已定（D-02），待实施 |
| E2E-F-05 | 预探查 | 数字人口型是随机轮播假口型，与音频零对齐 | `useTTSPlayer.ts:91-103` 每 150ms 循环切 5 个口型；L38-75 映射表是死代码；XTTS `/tts_with_phonemes` 带时间戳但前端从未调用 | E2E-17 | 🟡 方案已定（D-03），待实施 |
| E2E-F-06 | 预探查 | TTS 流式播放段间存在必然断点 | 500ms debounce 聚合文本；每段新文本先 `stop()` 再重发 HTTP；XTTS 每段一次 `inference_stream` 冷启动 | E2E-17 / E2E-28 | 🟡 方案已定（D-03），待实施 |
| E2E-F-07 | 预探查 | Go 服务结构化日志实际未进 Loki，只有 APISIX access log 被采集 | `promtail-config.yaml` 的 services job 指向 `/var/log/services/*.log`，但 compose 无 volume 挂载，也未用 docker-sd 采 stdout | E2E-21 | 🔴 未解决 |
| E2E-F-08 | 预探查 | Redis 容器空转未使用；分布式限流/登录锁定 Redis 后端是 TODO | `docker-compose.infra.yml` 注释"项目当前未使用 Redis"；`limiter.go:137-140` `RedisLimiterBackend: TODO`；BFF 登录失败锁定 in-memory 单实例 | E2E-18 | 🔴 未解决 |
| E2E-F-09 | 预探查 | 数据库无 schema_migrations 版本表；软删除覆盖不完整；db README 过时 | 迁移靠"幂等 + 每次重放"；软删除仅 users/ai 域，chat 的 deleteconversation 是物理删；README 仍列不存在的 `03-migrate-data.sql` | E2E-06 | 🔴 未解决 |
| E2E-F-10 | 预探查 | analytics mental-health 报表读一张永远为空的表 | `mental_health_assessments` 无任何生产写入方（INSERT 仅出现在测试）；trigger runner 只读已有记录后 marshal 入队 | E2E-15 | 🔴 未解决 |
| E2E-F-11 | 预探查 | Alertmanager 无外部通知渠道，仅 Web UI 聚合 | `alertmanager.yml` 只接 dev-ui 空 receiver | E2E-22 | 🔴 未解决 |
| E2E-F-12 | 预探查 | DLQ 无自动回放工具 | 告警规则注释里的回放都是手工 psql UPDATE；`InMemoryDLQPublisher` 仅测试用 | E2E-24 | 🔴 未解决 |
| E2E-F-13 | 预探查 | gRPC 路径的 trace_id 未注入结构化日志 | `grpcinterceptor/tracing.go` 只做 SkyWalking 上报，未调 `logging.WithTraceID`（HTTP 侧有做） | E2E-21 / E2E-26 | 🔴 未解决 |
| E2E-F-14 | 预探查 | user 页 3 个图表（昼夜/频率/深度）数据为空时整块不渲染且无空态提示 | `chat/user/index.vue:194,206,216` 每图均有 `?.length > 0` 守卫；数据源依赖 analytics 事件链 | E2E-11 | 🔴 未解决 |
| E2E-F-15 | 预探查 | 系统无管理员/角色概念 | `users` 表无 role 列；全仓无 admin 页面/端点 | E2E-06 / E2E-07 | 🟡 已通过 D-01=C 规避 |
| E2E-F-16 | 预探查 | `.mimosa/`（三处）未被 gitignore，持续污染 `git status` | hook 运行时状态目录，`.gitignore` 未覆盖 | E2E-02 | 🔴 未解决 |
| E2E-F-17 | 预探查 | `gui-test-screenshots/` 25 个测试截图散落根目录且已被 git 跟踪 | 历次 GUI 测试直接落盘根目录，未归档到 `docs/evidence/` | E2E-02 | 🔴 未解决 |
| E2E-F-18 | 预探查 | 残留空目录与一次性产物：`emotion-echo-web;D`（0 字节，shell 分号误建）、`docker-images-before.txt` | shell 未转义分号建目录；一次性快照未清理 | E2E-02 | 🔴 未解决 |
| E2E-F-19 | 预探查 | `users` 表 3 个死字段：`email`（零读写）、`phone`（仅响应回显、零写入→恒 NULL）、`status`（零读写） | `deploy/db/02-create-tables-in-schemas.sql:10-11,17`；详见 [findings §5.2](findings/2026-09-17-pretest-panorama.md) | E2E-06 | 🔴 未解决 |

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

## 与 R-xx 体系衔接

- 本账本追踪"E2E 阶段发现"的完整生命周期（发现 → 归属 → 排期 → 修复 → 回填）
- R-xx 体系（`docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md`）是运行时 bug 的权威编号：本账本条目修复落地后，回填 R 系并互相引用
- 建档预探查 19 项 + 覆盖盲区排查 10 项 = **29 项建档快照**；实测阶段若有新发现继续追加 E2E-F-30 起

## 不列入 E2E 阶段的候选（已评估）

| 候选 | 结论 |
|------|------|
| 生产部署（`compose.prod.yml` 空壳 + Helm chart 冻结） | **决策 3 / 决策 23 已明示冻结**，不单列。仅在 E2E-30 保留离线 `helm template`/`lint` 渲染回归项 |
| a11y | 折入 E2E-04（需引入 axe 依赖，不宜纯折入业务阶段） |
| 响应式/移动端 | 折入 E2E-04（仅 2 文件 6 处引用，单列投入产出不匹配） |
| 浏览器兼容 | 折入 E2E-04（补 browserslist + firefox 可选 project） |
| 配置与密钥 | 拆入：digest 假绿→E2E-05；Nacos→E2E-23；JWT 轮换→E2E-29 |
| i18n | 登记为 **D-04 候选**（E2E-F-29），不列阶段 |
