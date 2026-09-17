---
status: active
priority: high
created: 2026-09-17
last-refresh: 2026-09-17
type: e2e-discovered-unresolved-ledger
---

# E2E 已发现未解决账本（discovered-unresolved）

> 记录 E2E 阶段实测或建档预探查中发现、但**不属于当前执行阶段范围**未修复的问题。
> 编号 `E2E-F-xx`；确认为运行时 bug 且修复落地后，回填 `docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md`（R-xx 体系）并互相引用。
> 账本只增不删：已解决项标注状态保留记录。

## 账本

| 编号 | 来源 | 发现日期 | 现象 | 根因 | 归属阶段 | 状态 |
|------|------|---------|------|------|---------|------|
| E2E-F-01 | 预探查(建档) | 2026-09-17 | 找回密码/注册的验证码无真实投递渠道，流程仍按手机号短信时代设计 | 项目已改用户名登录；`BFF_DEV_RETURN_CODE=1` 仅 dev 回显；Sprint 112 只改了 UI 文案未改流程 | E2E-02（需改造讨论） | 🔴 未解决 |
| E2E-F-02 | 预探查(建档) | 2026-09-17 | 心理测验三层契约错位：提交必 400、结果弹窗"等级"恒空、列表页取数失败报错 | 前端发 `answers` 数组 vs 后端要 `map[string]int`；前端读 `level`/`suggestion` vs 后端回 `riskLevel`；前端读 `data.list` vs 后端回 `{items,total}` | E2E-08 | 🔴 未解决 |
| E2E-F-03 | 预探查(建档) | 2026-09-17 | 现工程量表种子数据不存在，surveys 表为空 | `deploy/db/` 无 INSERT 量表的 SQL；`docs/stages/stage-8b-assessment-surveys.md` 声称的 `seed-surveys.sql` 在 git 全历史中不存在 | E2E-08 | 🔴 未解决 |
| E2E-F-04 | 预探查(建档) | 2026-09-17 | "人格测试→心理预测→AI 提示词定制"链路完全不存在 | 量表是症状自评（PHQ-9/GAD-7/PSQI）非人格量表；评分为 `TotalScore+RiskLevel+逐题Factors` 无维度/画像；BFF system prompt 写死静态字符串；proto 无画像字段；legacy 唯一挂点 `BuildSurveyContext` 函数体 `return ""` | E2E-09（需改造讨论） | 🔴 未解决 |
| E2E-F-05 | 预探查(建档) | 2026-09-17 | 数字人口型是随机轮播假口型，与音频零对齐 | `useTTSPlayer.ts` L91-103 每 150ms 循环切 5 个口型；L38-75 的 phoneme→口型映射表全文件无引用（死代码）；XTTS `/tts_with_phonemes` 带时间戳但前端从未调用 | E2E-12（需改造讨论） | 🔴 未解决 |
| E2E-F-06 | 预探查(建档) | 2026-09-17 | TTS 流式播放段间存在必然断点 | `useConversationSender` 500ms debounce 聚合文本；`useTTSPlayer` 每段新文本先 `stop()` 再重新发 HTTP 请求；XTTS 每段都是一次新的 `inference_stream` 冷启动 | E2E-12 | 🔴 未解决 |
| E2E-F-07 | 预探查(建档) | 2026-09-17 | Go 服务结构化日志实际未进 Loki，只有 APISIX access log 被采集 | `promtail-config.yaml` 的 services job 指向 `/var/log/services/*.log`，但 compose 无任何 volume 把 Go svc 日志挂到该路径，也未用 docker-sd 采 stdout | E2E-15 | 🔴 未解决 |
| E2E-F-08 | 预探查(建档) | 2026-09-17 | Redis 容器空转未使用；分布式限流/登录锁定 Redis 后端是 TODO | `docker-compose.infra.yml` 注释"项目当前未使用 Redis"；`limiter.go:137-140` `RedisLimiterBackend: TODO`；BFF 登录失败锁定为 in-memory 单实例 | E2E-13 | 🔴 未解决 |
| E2E-F-09 | 预探查(建档) | 2026-09-17 | 数据库无 schema_migrations 版本表；软删除覆盖不完整；db README 过时 | 迁移靠"幂等 + 每次重放"，无法回答"某环境跑过哪些迁移"；软删除仅 users/ai 域，chat 的 deleteconversation 是物理删；`deploy/db/README.md` 仍列不存在的 `03-migrate-data.sql` | E2E-14 | 🔴 未解决 |
| E2E-F-10 | 预探查(建档) | 2026-09-17 | analytics mental-health 报表读一张永远为空的表 | `mental_health_assessments` 无任何生产写入方（INSERT 仅出现在测试）；trigger runner 只读已有记录后 marshal 入队，永不产生新 assessment；`reports` 表同样零写入方 | E2E-10 | 🔴 未解决 |
| E2E-F-11 | 预探查(建档) | 2026-09-17 | Alertmanager 无外部通知渠道，仅 Web UI 聚合 | `alertmanager.yml` 只接 dev-ui 空 receiver（注释写明 prod 演进方向） | E2E-16 | 🔴 未解决 |
| E2E-F-12 | 预探查(建档) | 2026-09-17 | DLQ 无自动回放工具 | 告警规则注释里的回放都是手工 psql UPDATE；`InMemoryDLQPublisher` 仅测试用 | E2E-18 | 🔴 未解决 |
| E2E-F-13 | 预探查(建档) | 2026-09-17 | gRPC 路径的 trace_id 未注入结构化日志 | `grpcinterceptor/tracing.go` 只做 SkyWalking 上报，未调 `logging.WithTraceID`（HTTP 侧 `gin_skywalking.go:17-21` 有做） | E2E-15 / E2E-20 | 🔴 未解决 |
| E2E-F-14 | 预探查(建档) | 2026-09-17 | user 页 3 个图表（昼夜/频率/深度）数据为空时整块不渲染且无空态提示 | `chat/user/index.vue:194,206,216` 每图均有 `?.length > 0` 守卫；数据源 `/user-behavior/*` 依赖 analytics 事件链，与报表 `chartData=[]` 同类风险 | E2E-06 | 🔴 未解决 |
| E2E-F-15 | 预探查(建档) | 2026-09-17 | 系统无管理员/角色概念，"管理员重置密码"无现成载体 | `users` 表无 role 列（唯一 `role` 在 messages 表指 user/assistant）；全仓无 admin 页面/端点 | E2E-02（影响 D-01） | 🔴 未解决 |

## 与 R-xx 体系衔接

- 本账本追踪"E2E 阶段发现"的完整生命周期（发现 → 归属 → 排期 → 修复 → 回填）
- R-xx 体系（`docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md`）是运行时 bug 的权威编号：本账本条目修复落地后，回填 R 系并互相引用
- 建档预探查的 15 项为**建档快照**，实测阶段若有新发现继续追加 E2E-F-16 起
