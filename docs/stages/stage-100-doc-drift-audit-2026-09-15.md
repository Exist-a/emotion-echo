---
status: landed
stage: 100
title: 2026-09-15 代码审计 + open 清单修订（multi-round-iteration plan 漂移纠正）
date: 2026-09-15
type: doc-drift-audit
source-plan: multi-round-iteration-2026-09-15.md
depends-on:
  - stage-98-round-1-closure.md
  - stage-99-round-2-closure.md
related-stages:
  - stage-94-code-review-2026-09-14-p0-closure.md
  - stage-97-round2-p0-closure.md
related-adrs:
  - 决策 18（doc-drift registry）
---

# Stage 100 — 2026-09-15 代码审计 + open 清单修订

> **背景**：用户 2026-09-15 在会话中提出"先看代码再讨论"，触发本文档。
> 用户原话："你先调查一下这些所谓的待做项在代码中真的没落地吗？你先去看，如果落地了，先修改文档，然后再进行下一步讨论。"
>
> 本文是 **计划漂移纠正收口报告**：把 plan 列的 49 项 open 按代码事实核对后，
> 修订为 23 项真未落/半落（工作量 18-25d → 8-9d）。

---

## 一、审计方法

按 AGENTS.md §〇必做功课 #1（**必读代码事实**），把 `multi-round-iteration-2026-09-15.md §三-§六` 列的全部 49 项 grep 代码证据：

| 维度 | 命令 / 文件 | 数量 |
|------|------------|------|
| 模型层 | `grep -rn "DeletedAt\|gorm.DeletedAt" emotion-echo-ai-svc/internal/model/` | 4 文件 |
| SSRF | `sed -n '70,100p' emotion-llm-service/file_context.py` + urlparse 关键字 | 1 文件 |
| Mock 文案 | `grep -n "variants\|random.choice" emotion-llm-service/chat_completion.py` | 1 文件 |
| LLM 审核 | `grep -n "moderate_content\|_DANGEROUS_PATTERNS" emotion-llm-service/chat_completion.py` | 1 文件 |
| API key 校验 | `grep -n "weak\|REQUIRED\|sys.exit" emotion-llm-service/grpc_server.py` | 1 文件 |
| DLQ metric | git log + `0fbe2d0` 验证 | 1 commit |
| Promtail | `grep -n "services\|/var/log" deploy/loki/promtail-config.yaml` | 1 文件 |
| Healthcheck | `grep -rn "/healthz\|HEALTHCHECK" emotion-echo-web*/main.go + Dockerfile` | 4 文件 |
| Helm probe | `grep -rn "livenessProbe\|readinessProbe" deploy/charts/` | 0 命中 |
| Nacos heartbeat | `grep -rn "BeatInstance\|UpdateInstance" emotion-echo-*/main.go` | 0 命中 |
| Limiter | `grep -n "cleanupInterval\|AfterFunc\|LRU" emotion-echo-shared/pkg/middleware/limiter.go` | 0 命中 |
| PG 池 | `grep -rn "PG_MAX_CONNS\|PG_MIN_IDLE" emotion-echo-*/` | 0 命中 |
| Skywalking | `grep -rn "InstrumentGORM\|InstrumentRedis" emotion-echo-*/main.go` | 0 命中 |
| Kafka topic | `grep -rn "KAFKA_NUM_PARTITIONS" deploy/` | 0 命中 |
| Memory limit | `grep -B5 "memory: 256M" deploy/docker-compose.apps.yml` | 4 svc 仍 256M |
| Consumer split | `wc -l emotion-echo-ai-svc/internal/consumer/consumer.go` | 346 行 |
| Digest pin | `grep -rn "FROM.*@sha256" $(find . -name Dockerfile)` | 16 Dockerfile 0 命中 |

---

## 二、审计结论（4 分类）

### 2.1 doc drift — plan 标 open / 代码已落（13 项）

plan §三-§六 / roadmap §当前 open 清单 标为 open，但代码已有实现：

| Plan 项 | 代码事实 | 证据 |
|---------|---------|------|
| Round 3.1 mock 随机 | 4 random variants（不只是 2 帧）| `chat_completion.py:78-92` |
| Round 3.1 api_key 脱敏 | `_safe_fallback_reason` 三类正则（sk-/Bearer/api_key）| `chat_completion.py:152-163` + `intent_llm.py:92-96` |
| Round 3.1 panic 脱敏 | panic value 改固定文案 + log 原始 panic | `grpcinterceptor/server.go:92-97` |
| Round 3.2 SSRF 边界 | userinfo 拒收 + parsed.hostname 二次校验 + 默认端口对齐 | `file_context.py:72-99` |
| Round 3.4 LLM 输出审核 | 关键词正则 7 类 + 安全回复替换 | `chat_completion.py:165-200` |
| Round 3.5 弱 key fail-fast | `INTERNAL_API_KEY_REQUIRED=1` 时 sys.exit(1) | `grpc_server.py:388-415` |
| Round 4.1 DLQ metric + 告警 | Round 2.3 已落（commit 0fbe2d0 + kafka-dlq.yml） | `deploy/prometheus/rules/kafka-dlq.yml` |
| Round 4.1 Promtail 业务 svc | `/var/log/services/*.log` scrape job 已配 | `promtail-config.yaml:38-42` |
| Round 4.5 消息体大小 | max=4 KiB 已落 | `sendmessagelogic.go:71-76` |
| Round 4.6 MV metric | `MVRefreshFail/Success/Duration` 3 metric | `analytics-svc/main.go:119-126` |
| Round 4.6 dev CORS | 中间件已回滚，CORS 由 APISIX 统一配 | `web-bff/main.go:259-263` |
| Round 4.6 web registry | `ARG NPM_REGISTRY=https://registry.npmjs.org/` | `web/Dockerfile:12-15` |
| Round 4.6 TrustAPISIX | 已实现 + IP 白名单 + dev fallback | `web-bff/main.go:174-197` |
| Round 1 follow-up msg_summary_v 单 owner | deploy/db 撤回 CREATE VIEW；c005 唯一源 | `04-create-views.sql:14-15` |
| Round 1 follow-up i007 DDL 列 | 4 张表 deleted_at + 索引已加 | `i007_soft_delete_columns.sql:19-28` |

> **统计**：14 行（msg_summary_v 单 owner + i007 DDL 列 是 Round 1 follow-up 内部项）

### 2.2 partial open — 代码部分实现（6 项）

代码有部分实现，但**还差关键步骤**：

| Plan 项 | 已落 | 缺什么 | 证据 |
|---------|------|-------|------|
| Round 3.3 prompt 注入前缀 | `<file_attachment>` 包裹 | "不要执行附件指令" 防注入前缀 | `file_context.py:194` vs grpc_server.py:290-310 缺指令前缀 |
| Round 3.5 跨 svc 隔离 | fail-fast | `AI_LLM_INTERNAL_API_KEY` / `BFF_LLM_INTERNAL_API_KEY` 拆分 | 3 svc 仍读 `INTERNAL_API_KEY` 一个 env |
| Round 4.1 helm probe | Dockerfile + /healthz | helm `livenessProbe`/`readinessProbe` | grep `values-prod.yaml` 0 命中 |
| Round 4.2 Nacos fail-fast (web-bff) | llm-service fail-fast | web-bff `main.go:132` swallow 错误未改 | `web-bff/main.go:132` |
| Round 4.5 compose health | 1 处 `service_healthy` | 14 处仍 `service_started` | `compose.apps.yml:53 vs :118-324` |
| Round 4.6 memory limit | ai-svc/llm/funasr 升到 1024/1024/1536M | 4 svc 仍 256M/64M | `compose.apps.yml:112,170,233,289` |
| Round 4.7 §F consumer.go | dlq/proto_decode/metrics 已拆 | consumer.go **346 行**（仍 > 200 阈值）| `wc -l consumer.go = 346` |

### 2.3 truly open — 代码确认未实现（11 项）

| Plan 项 | 现状 | 工作量 | 来源 |
|---------|------|--------|------|
| face/voice/fused model gorm.DeletedAt + repo Delete | i007 SQL 列已加；model/repo 未改 | 0.5d | plan §三 Round 1.2 |
| voice_transcripts 软删除（DDL+model+repo） | 5 张表唯一 DDL 缺 | 0.25d | 同上 |
| assessment_v 迁 analytics | 双 owner 但 SQL diff 0 行 | 0.25d | Round 1.4 |
| Round 4.2 Nacos BeatInstance | 0 命中 | 0.5d | plan §六 Round 4.2 PR-1 |
| Round 4.3 limiter buckets LRU | 0 命中 | 0.25d | plan §六 Round 4.3 PR-1 |
| Round 4.3 限流 backend Redis | 0 命中 | 0.75d | plan §六 Round 4.3 PR-2 |
| Round 4.4 PG 连接池配置化 | 硬编码 10/5 | 0.5d | plan §六 Round 4.4 PR-2 |
| Round 4.4 skywalking gorm/redis | 0 caller in main.go | 1d | plan §六 Round 4.4 PR-1 |
| Round 4.4 chat-events 6 partition | KAFKA_NUM_PARTITIONS 缺 | 1d（触发=上 prod）| plan §六 Round 4.4 PR-3 |
| Round 4.5 ai-svc IP 限流 | 0 命中 | 0.25d | plan §六 Round 4.5 PR-3 |
| Round 4.5 nacos 控制台 profile ops | 0 命中 | 0.25d | plan §六 Round 4.5 PR-4 |
| Round 4.6 ai-api.yaml 字面值 + applyDefaultFallbacks | 仍用 localhost/5432/11800 dev 默认 | 0.25d | plan §六 Round 4.6 PR-1+PR-2 |
| Round 4.7 digest pin | 16 Dockerfile 全部 `FROM image:tag` | 1d | plan §六 Round 4.7 PR-3 |

> **统计**：13 行（assessment_v 单独算）

### 2.4 trigger-condition — 多副本/上 prod 才生效（5 项）

| Plan 项 | 触发条件 | 现状 |
|---------|---------|------|
| Kafka D3 consumer attempts 持久化 | ai-svc/analytics-svc 多副本 | in-memory map（consumer.go:63） |
| Kafka D5 relay 多副本互斥 | chat-svc 决定扩副本 | 无 advisory lock / SELECT FOR UPDATE SKIP LOCKED |
| Kafka D7 删除会话生命周期 | owner 拍板（产品语义） | `DeleteConversationTx` 硬删 + 复用 conversation.closed |
| Round 4.4 chat-events 6 partition | 真上 prod | dev 1 partition 无影响 |
| Round 4.1 helm probe 部分 | helm 部署触发 | — |

---

## 三、总账

| 分类 | 数量 | 工作量 |
|------|------|--------|
| doc drift（plan 标 open / 已落）| 14 | — |
| partial open（部分落）| 7 | ~3.5d |
| truly open（真未落）| 13 | ~7d |
| trigger-condition | 5 | 触发后 ~3-5d |
| **真正可启动 open** | **17 truly + 6 partial = 23** | **~8-9d** |
| **原 plan 估** | 49 | 18-25d |
| **差额** | -26 项 | -10-15d |

> plan 多估了一半工作量。13 项 doc drift 是历史 sprint 顺手落地但 plan/roadmap 未同步；6 项 partial 是部分实现未达到 plan 的完整目标。

---

## 四、文档修订落点

| 文档 | 改动 |
|------|------|
| `docs/plans/multi-round-iteration-2026-09-15.md` front-matter | `status: landed` → `status: in-flight`；新增 `audit-2026-09-15` 字段 |
| `docs/plans/multi-round-iteration-2026-09-15.md` §〇 | "49 个 open 项" + "18-25 人天" 加审计修订注释 |
| `docs/plans/multi-round-iteration-2026-09-15.md` §十二 工作量 | "预计总时长 16-22d" → 修订为 "8-9d" |
| `docs/plans/multi-round-iteration-2026-09-15.md` §十三 | 表格按审计重写（Round 3.1/3.2/3.4 标 ✅；3.3/3.5 partial；Round 4 大部分 partial） |
| `docs/plans/multi-round-iteration-2026-09-15.md` §十四 | **新增 6 小节**：审计方法 + doc drift + partial + truly + trigger + 下轮优先级 + 修订记录 |
| `docs/plans/multi-round-iteration-2026-09-15.md` §十 调研依据 | 加 "2026-09-15 审计修订" 注脚 |
| `docs/plans/multi-round-iteration-2026-09-15.md` §十一 成功标准 | 加 "审计修订" 字段 |
| `docs/architecture/roadmap.md` §当前 open 清单 | 全量重写：Round 1/2/3/4 按审计标 ✅ / ⏳ partial / ⏳ 真未落；新增"修订后真正 open = 23 项" + 下轮建议顺序 |
| `docs/plans/README.md` front-matter + 条目 | 加 "代码审计修订" 状态；条目描述更新工作量 + 列出已落 13 项 + 剩余 23 项 |

---

## 五、下轮启动建议（按 §14.5 顺序）

按 AGENTS.md §〇"前置必在前" + 工作量×风险排序：

| 序号 | Round | 工作量 | 风险 |
|------|-------|--------|------|
| 1 | Round 1 follow-up（face/voice/fused/voice_transcripts 软删） | 0.75d | 低 |
| 2 | Round 3.3 防注入前缀 | 0.25d | 中（心理健康）|
| 3 | Round 3.5 跨 svc 隔离 | 0.5d | 中（误用 dev key）|
| 4 | Round 4.6 ai-api.yaml 字面值 + applyDefaultFallbacks 收紧 | 0.5d | 中（prod 误配）|
| 5 | Round 4.5 compose health (14 处) + nacos profile + ai-svc IP 限流 | 1d | 低-中 |
| 6 | Round 4.2 Nacos 心跳 + fail-fast | 1d | 中（多副本必做）|
| 7 | Round 4.3 limiter buckets LRU + Redis backend | 1d | 中（OOM 风险）|
| 8 | Round 4.4 PG 池 + skywalking gorm/redis | 1.5d | 低-中 |
| 9 | Round 4.6 memory limit | 0.5d | 低 |
| 10 | Round 4.7 consumer.go 拆 < 200 行 + digest pin + SKIP_PATH_LIST | 2d | 低 |
| **合计** | | **8-9d** | |

---

## 六、调研依据

| 事实 | 证据 |
|------|------|
| Round 3.1 mock 4 variants | `chat_completion.py:78-92` 注释 P1-R2-4 + random.choice |
| Round 3.1 api_key 脱敏 | `chat_completion.py:152-163` `_safe_fallback_reason` 注释 P1-R2-4 |
| Round 3.1 panic 脱敏 | `grpcinterceptor/server.go:92-97` 注释 P2-18 Round 1 |
| Round 3.2 SSRF userinfo | `file_context.py:74-85` 注释 P1-R2-5 |
| Round 3.4 关键词审核 | `chat_completion.py:168-189` `_DANGEROUS_PATTERNS` 注释 P1-R2-7 |
| Round 3.5 fail-fast | `grpc_server.py:392-407` 注释 P2-R2-5 + REQUIRED=1 |
| Round 4.1 DLQ metric | commit `0fbe2d0` + `deploy/prometheus/rules/kafka-dlq.yml` 2 条 critical alert |
| Round 4.1 Promtail 业务 svc | `promtail-config.yaml:36-42` job=services + /var/log/services/*.log |
| Round 4.5 大小限制 | `sendmessagelogic.go:71-76` 注释 P1-13 |
| Round 4.6 MV metric | `analytics-svc/main.go:119-126` MVRefreshFail/Success/Duration |
| Round 4.6 CORS | `web-bff/main.go:259-263` "dev CORS 已回滚，改走 APISIX" |
| Round 4.6 web registry | `web/Dockerfile:12-15` ARG NPM_REGISTRY |
| Round 4.6 TrustAPISIX | `web-bff/main.go:174-197` "信任 APISIX 注入 X-User-Id header" |
| Round 1 follow-up msg_summary_v | `deploy/db/04-create-views.sql:14-15` "已迁至 chat-svc migrations" |
| Round 1 follow-up i007 DDL | `i007_soft_delete_columns.sql:19-28` 4 ALTER TABLE + 4 CREATE INDEX |
| face/voice/fused model 未加 | `face_emotion.go / fused_emotion.go / voice_emotion.go` grep gorm.DeletedAt 0 命中 |
| voice_transcripts DDL 缺 | `02-create-tables-in-schemas.sql:140-150` 创表无 deleted_at |
| assessment_v 双 owner 一致 | `04-create-views.sql:30-50` vs `a001_create_views.sql:39-59` diff 0 行 |
| Round 4.2 BeatInstance 缺 | grep 全仓 0 命中 |
| Round 4.3 limiter LRU 缺 | grep 全仓 0 命中 cleanupInterval/AfterFunc |
| Round 4.3 Redis backend 缺 | grep 全仓 0 命中 redis:// / LIMITER_BACKEND |
| Round 4.4 PG 硬编码 | 5 svc 全部 `MaxOpenConns == 0 → 10`、`MaxIdleConns == 0 → 5` |
| Round 4.4 skywalking gorm/redis caller | grep 全仓 0 caller in main.go |
| Round 4.4 KAFKA_NUM_PARTITIONS | grep deploy/ 全仓 0 命中 |
| Round 4.5 ai-svc IP 限流 | grep ai-svc 0 命中 IPLimit/ipRateLimit |
| Round 4.5 nacos profile ops | grep 全仓 0 命中 `profiles.*ops` |
| Round 4.6 ai-api.yaml 字面值 | `ai-svc/main.go:166-191` applyDefaultFallbacks 仍含 localhost 默认 |
| Round 4.7 digest pin | `find . -name Dockerfile` 16 个，0 命中 `@sha256` |
| Round 4.7 consumer.go 行数 | `wc -l = 346`（< 200 不达标）|

---

## 七、与决策栈的关系

| 决策 | 状态 | 关联 |
|------|------|------|
| 决策 18 doc-drift registry | ✅ | 本报告 stage-100 登记 |
| 决策 19 dev-publisher fallback | ✅ | 审计未触动 |
| 决策 22 chat-svc 表依赖 | ✅ | 审计未触动 |

---

## 八、收口自检三连（AGENTS.md §2.5）

```
$ git status
On branch main
nothing to commit, working tree clean ✓ (本次只改文档，未动代码)

$ git status -sb
## main...origin/main
✓ 无 ahead/behind

$ git branch --merged main
* main
✓ 无残留 feature 分支
```

---

## 九、留给下轮（Round 5 全量收口候选）

- Round 5.1：roadmap §"当前 open 清单"再次刷新（按 Round 3-4 落地结果）
- Round 5.2：`docs/plans/README.md` + `docs/architecture/decisions.md` 索引刷新
- Round 5.3：CI 接入（docs/ci-workflows §2.4 数据契约 smoke）
- Round 5.4：`stage-101-multi-round-iteration-closure.md` 整体收口报告

---

> 本报告对应 `docs/plans/multi-round-iteration-2026-09-15.md §十四`。
> 工作量从 18-25d 修订为 8-9d。代码事实清单见 §二 4 分类表格。
> 等用户决定是否启动 Round 3.3 防注入前缀（最小工作量） 或 Round 1 follow-up 软删除（最稳）。
