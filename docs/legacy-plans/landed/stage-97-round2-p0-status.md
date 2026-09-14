---
status: landed
stage: 97
title: Round 2 Code Review P0 收口（基于工作区待提交草稿）
date: 2026-09-15
landed: 2026-09-15
landed-by: stage-97 (13 commits, 12 PR + docs commit + drift-fix commit)
source-plan: code-review-2026-09-14-round-2.md
depends-on: stage-94, stage-95, stage-96
related-stages:
  - stage-94 (Round 1 P0 收口)
  - stage-95 (Round 2 收口报告)
  - stage-96 (Round 1 P1/P2 残余收口)
---

# Stage 97 — Round 2 P0 收口（基于工作区待提交草稿）

> ⚠️ **本文件是工作区现状核查报告**，不是新规划。Round 2 的 10 个 P0 大部分**已在
> 工作区里完成但未 commit**（working copy 状态）。本 Stage 任务 = **补缺 + 验证 +
> 提交 + 文档同步**。

## 1. 现状核查矩阵（10 项 P0 vs working copy）

| # | 标题 | 工作区状态 | 验证 |
|---|------|-----------|------|
| P0-R2-1 | JWT 双存储（localStorage + 非 HttpOnly cookie） | 🟡 **部分修**：BFF `setAccessTokenCookie` HttpOnly、APISIX seed `cookie: "access_token"`、useApi 优先 cookie；**useApi.ts 中 `localStorage.setItem("access_token", ...)` 残留未删**（需 grep 验证） | 待核 |
| P0-R2-2 | useFaceEmotion 调 orphan `/face/emotion` | ✅ 已修：改调 `/multimodal/analyze`（form-data） | 已核（diff 在工作区） |
| P0-R2-3 | llm-service HTTP 端零鉴权 | ✅ 已修：CORS `*` → 白名单 `credentials=False`；新增 `_check_http_api_key` dependency 挂 `/analyze` | 已核 |
| P0-R2-4 | multimodal_handler 无 body size limit | ✅ 已修：`http.MaxBytesReader(50 << 20)` | 已核 |
| P0-R2-5 | env 名错配 `LLM_INTERNAL_API_KEY` vs `INTERNAL_API_KEY` | ✅ 已修：ai-svc main.go + ai-api.yaml + deployment.yaml 全部改 `INTERNAL_API_KEY` | 已核 |
| P0-R2-6 | `daily_emotion_by_modality_v` 缺 GRANT | ✅ 已修：a004 第 54 行 GRANT；i005 视图已存在 | 已核 |
| P0-R2-7 | messages.conversation_id FK 漂移 | ✅ 已修：02-create-tables-in-schemas.sql 第 79 行补 `REFERENCES ... ON DELETE CASCADE`；01 重复 DDL 全删（仅留 CREATE SCHEMA） | 已核 |
| P0-R2-8 | web Dockerfile prod 阶段无 `USER node` | ✅ 已修：第 30 行 `USER node` | 已核 |
| P0-R2-9 | web Dockerfile `rm -f package-lock.json` | ✅ 已修：改用 `npm ci --omit=optional` 保留 lockfile | 已核 |
| P0-R2-10 | dev compose 不引 env_file | ✅ 已修：apps.yml 顶层加 `x-env-file: &env-file [- .env.local]` 锚点；user-svc/chat-svc/analytics-svc/assessment-svc/llm-service/ai-svc/web-bff 全部 `env_file: *env-file` | 已核 |

**结论：10 个 P0 中 9 个已在工作区完成、1 个（P0-R2-1）部分完成 + 残留 localStorage 待清。**

## 2. 顺手落地但需审计的"非 P0 变更"

工作区还有**远超 P0 范围的改动**——读 diff 后发现，至少涵盖了 **P1-R2（11 项）+ P2-R2（8 项）+ Stage 96 报告的 P1/P2（多项）**。逐项审计：

### 2.1 P1-R2 系列（Round 2 P1，11 项在工作区）

| # | 文件 | 内容 |
|---|------|------|
| P1-R2-1 | useApi.ts | SSE 401 重连无幂等 → sortObjectKeys / sortUrlQuery（实际是 P2-R2-1 "JSON.stringify 顺序"）|
| P1-R2-2 | pages/chat/conversation/[id].vue | marked.parse 自定义 renderer 转义 HTML |
| P1-R2-3 | middleware/auth.global.ts | 白名单 `startsWith` 大小写/前缀绕过 → 改 exact set + `p + "/"` 边界 |
| P1-R2-13 | compose.apps.yml | ai-svc/llm-service memory 512M → 1024M |
| 其他 P1-R2（4-12/14-17） | 未在工作区 |

### 2.2 P2-R2 系列（Round 2 P2，工作区 8 项）

| # | 文件 | 内容 |
|---|------|------|
| P2-R2-1 | useApi.ts | JSON.stringify 顺序敏感 → sortObjectKeys + sortUrlQuery |
| P2-R2-2 | useApi.ts | pendingRequests Map 内存泄漏 → size 上限 + FIFO 清理 |
| P2-R2-3 | useApi.ts | （部分）请求合并 AbortSignal |
| P2-R2-8 | 04-create-views.sql | msg_summary_v 三处定义漂移 → 04 删除 msg_summary_v，唯一源收敛到 chat-svc c005 |
| P2-R2-13 | compose 镜像 tag | （需 grep 验证） |
| P2-R2-16 | web Dockerfile | HEALTHCHECK 加 `wget` |
| P2-R2-18 | web-bff | （需 grep 验证） |
| P2-R2-19 | web Dockerfile | 注释提示 pin digest（**仅注释，未实施**）|

### 2.3 Stage 96 报告的 P1/P2（工作区顺手做的）|

| 标签 | 文件 | 内容 |
|------|------|------|
| P1-2 | consumer.go (ai + analytics) | DLQ sw8 header 透传 |
| P1-15 | dlq.go (ai + analytics) | 3 次指数退避重试 |
| P2-5 | grpcinterceptor/server.go | panic counter |
| P2-18 | grpcinterceptor/server.go | panic 消息脱敏（"internal error" 固定文案）|
| P2-13 | analytics config.go | MaxRetries 字段对齐 ai-svc |
| DOC-9 | llm-service main.py | Nacos fail-fast（NACOS_REQUIRED）|
| P0-R2-3 联动 | apisix/seed.sh | jwt-auth cookie = "access_token" |
| P1-3 | apisix/seed.sh | file-logger trace_id 字段 |
| P1-4 | promtail-config.yaml | /var/log/services/*.log 采集 |
| 其它 | 01/02 schema DDL 重整、migration 重命名加前缀（i/a/c）|

## 3. 必须做的"补缺 + 验证"清单（Stage 97 实际工作）

### 3.1 P0-R2-1 补缺（RED 测试 + 清残留）
- [ ] grep `localStorage.setItem("access_token"` 是否在 useApi.ts / stores/user.ts 仍有残留
- [ ] 加 useApi.ts 单测：登录后 setAccessTokenCookie 不写 localStorage
- [ ] 加 useApi.ts 单测：刷新页面后从 cookie 恢复 token
- [ ] **清残留**（如有）

### 3.2 TDD 验证（每项 P0 修复补单测，否则不允许合并）

| P0 | 现有测试 | 需补 |
|---|---------|------|
| P0-R2-3 | 无 | llm-service `test_main.py` 加 `/analyze` 无 Internal-API-Key → 401（key 配时）|
| P0-R2-4 | 无 | ai-svc `multimodal_handler_test.go` 加 body 超 50MB → 400/413 |
| P0-R2-5 | 无 | ai-svc `main_test.go`（若存在）或 `config_test.go` 加 `applyEnvOverrides` `INTERNAL_API_KEY` 注入断言 |
| P0-R2-6 | 无 | analytics `004_security_test.go` 已存在？加 `daily_emotion_by_modality_v` SELECT 断言 |
| P0-R2-7 | 无 | （DDL，无法单测；契约 smoke §5 验证）|
| P0-R2-8 | 无 | （Dockerfile 层，docker build + docker run --user 验证）|
| P0-R2-9 | 无 | （lockfile 保留可由 `docker build` 验证；不删 lockfile 不重 install 即可）|
| P0-R2-10 | 无 | compose 启动时验证 env_file 注入（`docker inspect` env 列表）|

### 3.3 §2.4 数据契约 smoke（合并前必跑）
- 启动 dev compose（带 `--env-file .env.local`）
- 跑 `scripts/smoke_data_layer.py` 验证 §契约 1-6 全绿
- 重点：§契约 3（`analytics_reader` 视图可读含 `daily_emotion_by_modality_v`）

### 3.4 文档同步（§九 收口时同步）
- [ ] `code-review-2026-09-14-round-2.md` front-matter 改 `status: landed`
- [ ] `docs/plans/README.md` Round 2 索引移到 landed
- [ ] 新建 `docs/stages/stage-97-round2-p0-closure.md` 收口报告
- [ ] `roadmap.md` §"当前 open 清单"刷新（清掉 Round 2 P0 + 加 Stage 97 落地记录）
- [ ] `decisions.md` 决策 11/12 末尾补 "llm-service HTTP 端仅 dev 调试，prod 必鉴权" 备注
- [ ] `QUICKSTART.md` 删明文 password "echo123"（P1-R2-11）+ 改 "compose 自动加载 .env.local" → "必须 `--env-file .env.local`"
- [ ] `README.md` 删 "compose 自动加载 .env.local" 措辞

## 4. PR 拆分（按 Round 2 §5 + 合并修复分组）

| PR | 范围 | 工作量 | 涉及文件 |
|---|------|--------|----------|
| **PR-1** | LLM 链路兜底 | 已完成 | emotion-echo-ai-svc/main.go + ai-api.yaml + emotion-llm-service/main.py + deployment.yaml |
| **PR-2** | multimodal_handler DoS 防护 | 已完成 | emotion-echo-ai-svc/internal/handler/multimodal_handler.go |
| **PR-3** | DB DDL 漂移修复（FK + 01/02 拆分 + GRANT + msg_summary_v 收敛）| 已完成 | deploy/db/01/02/03/04 + ai-svc migrations/i* + analytics migrations/a* |
| **PR-4** | web 容器 USER + lockfile + HEALTHCHECK | 已完成 | emotion-echo-web/Dockerfile |
| **PR-5** | compose env_file 锚点 + 镜像标签 | 已完成 | docker-compose.apps.yml + docker-compose.ai.yml |
| **PR-6** | JWT 双存储重写（cookie-only）| 🟡 需补缺 | BFF auth_handler + useApi.ts + stores/user.ts + useAIStream.ts + apisix/seed.sh |
| **PR-7** | useFaceEmotion orphan API 修复 | 已完成 | useFaceEmotion.ts + apiRoutes.ts |
| **PR-8** | 顺手 Round 1 P1/P2 + Round 2 P1/P2 | 已完成 | （多项） |
| **PR-9** | docs + Stage 97 收口报告 | 待写 | docs/ 下 |

## 5. 风险点（先评估再提交）

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| PR 范围膨胀（一个 PR 含 50+ 文件改动）| 高 | 按 Round 2 §5 拆 9 个 PR（每 PR ≤ 8 文件 + 单测 ≥ 1 文件）|
| 改 schema DDL 后 dev 库缺视图 | 中 | `migrate.sh` 重跑 + smoke §契约 3 + 4 验证 |
| 改 web Dockerfile lockfile 触发 linux-musl 兼容（Stage 74 已知）| 中 | 已用 `--omit=optional` 绕过；首次构建若失败回退到 `npm install --no-package-lock --omit=optional` |
| JWT cookie-only 破坏现有 session | 中 | refresh_token 流程不变；前端 store 加 migration shim（清 localStorage）|
| llm-service `/analyze` 鉴权后 dev 用户调试 AI 功能断 | 低 | dev compose 默认 `INTERNAL_API_KEY` 空 → 鉴权跳过 |
| APISIX cookie: "access_token" 与现 Authorization JWT 双链路并存 | 中 | seed.sh 已配；老客户端先用 Authorization，新客户端逐步切 cookie |
| Redis sentinel / WebSocket 重连等不在本 PR 范围 | - | 列入 backlog |

## 6. 成功标准（Stage 97 完成定义）

- [ ] 9 个 PR 全部 merged 到 main
- [ ] §2.4 §契约 1-6 全绿（含 `daily_emotion_by_modality_v` GRANT 实证）
- [ ] `go test ./...` + `pytest` 全绿
- [ ] dev compose 启动后端到端跑通：登录 / AI stream / 报表查询
- [ ] `code-review-2026-09-14-round-2.md` 改 `status: landed`
- [ ] `docs/stages/stage-97-round2-p0-closure.md` 收口报告
- [ ] `git status` 干净 / `git status -sb` main ahead=behind=0
- [ ] Round 2 P0 全部从 open 清单移除

## 7. 不在本 Stage 范围

- Round 2 P1（17 项）—— 下一 sprint 单独排期
- Round 2 P2（21 项）—— 工作区已顺手修 8 项，其余 13 项下一 sprint
- Round 2 P3（5 项）—— 季度
- Kafka pipeline 待决策（kafka-pipeline-pending-decisions.md D1-D8）—— 独立 sprint
- todo-pile C6/D5 残余 —— 1-2h + 半天

> **本计划基于代码事实**（working copy diff 全核 + 6 个关键文件 + 8 个 PR 范围 + 25+ 文件改动）。
> 调研依据 = `git diff HEAD --stat`（50+ 文件）+ `grep`/`Read` 复核每个 P0 修复路径。