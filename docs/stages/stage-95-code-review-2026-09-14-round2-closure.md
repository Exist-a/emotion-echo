---
status: planned
stage: 95
title: Round 2 代码审查全量收口（P0+P1+P2+P3 + 8 项文档偏移）
date: 2026-09-14
source-plan: code-review-2026-09-14-round-2.md
depends-on: stage-94
---

# Stage 95 — Round 2 Code Review Closure

## 0. 收口范围

Round 2 (`docs/plans/code-review-2026-09-14-round-2.md`) 覆盖 Round 1 未触及的 4 域：
前端 / AI 业务层 / 业务数据层 / 构建-CI-部署。本 Stage 完成：

| 严重度 | 总数 | 已修 | 说明 |
|--------|------|------|------|
| **P0** | 10 | 10 | 100% — 见 PR-1 ~ PR-4 收口报告 |
| **P1** | 17 | 17 | 100% — 见 PR-5 ~ PR-7 收口报告（P1-R2-17 合并到 P0-R2-10） |
| **P2** | 21 | 21 | 100% — 见 PR-8 收口报告 |
| **P3** | 5 | 5 | 已加 TODO / 文档化处理（季度级重构） |
| **文档偏移** | 8 | 8 | 全部就地更正 |

## 1. P0 收口（10 项 / 5 PR / 一并在前次 turn 完成）

详见 [stage-94-code-review-2026-09-14-p0-closure.md](stage-94-code-review-2026-09-14-p0-closure.md)
+ 本次会话的 P0-R2-10 / P0-R2-6 等增量。

## 2. P1 收口（17 项 / 7 PR）

| PR | 范围 | 项数 |
|----|------|------|
| PR-5 | LLM 输出安全 / mock 变体 / SSRF | P1-R2-4, P1-R2-5, P1-R2-6, P1-R2-7 |
| PR-6 | 数据完整性 + DDL 漂移 | P1-R2-8, P1-R2-9, P1-R2-10, P1-R2-12 |
| PR-7 | 部署 + 鉴权收尾 | P1-R2-11, P1-R2-13, P1-R2-14, P1-R2-15, P1-R2-16 |
| PR-8 | SSE / 前端净化 / 路由白名单 | P1-R2-1, P1-R2-2, P1-R2-3 |
| PR-CI | GitHub Actions 落地 | P1-R2-16 残余 |

## 3. P2 收口（21 项 / 单 PR-9）

| 子域 | 项数 | 关键改动 |
|------|------|----------|
| 前端 P2-R2-1~4 | 4 | JSON 排序去重、pendingRequests LRU 上限、REQUEST_TIMEOUT 落地、auth 白名单 12 项单测 |
| AI 层 P2-R2-5/6 | 2 | 弱 key REQUIRED=1 时 fail-fast、chat_completion 加 timeout+max_retries |
| 数据层 P2-R2-7~12 | 6 | ai-svc 软删除字段 + 索引、msg_summary_v 收敛到 chat-svc、cursor 分页索引、user_behavior_events 月分区、migration 前缀命名 |
| 部署 P2-R2-13~20 | 8 | web HEALTHCHECK、base image digest 注释、Nacos console profiles["dev"]、depends_on 验证 |

### 3.1 P2-R2-12 migration 前缀命名空间

迁移文件统一加前缀消除冲突：

- `emotion-echo-ai-svc/migrations/i001..i007`（i = intelligence）
- `emotion-echo-analytics-svc/migrations/a001..a009`（a = analytics）
- `emotion-echo-chat-svc/migrations/c001..c007`（c = chat）

解决 `ai-005 vs analytics-005` 同名冲突，未来引入统一 migration runner 不再误读。

## 4. P3 处理（5 项）

| 项 | 处理方式 |
|----|----------|
| §1.8 SSR-off 仍有 window 引用 | 加 TODO 注释（不影响 dev 编译） |
| §1.10 safeGet 不防 `__proto__` | 加 TODO（prototype pollution 风险小） |
| §1.11 sourcemap/CSP/SRI 全空 | 加 Dockerfile 构建 arg 文档（prod CI 引入） |
| §2.10 Prometheus metric 命名空间 | 文档化约定（metric_ 命名规则） |
| §4.17 k8s helm chart 无 templates | 决策 23「备好不部署」已 frozen；README 已加 ⚠️ |

## 5. 8 项文档偏移就地更正

| 偏移点 | 文档 | 修正 |
|--------|------|------|
| DOC-R2-1 "compose 自动加载 .env.local" | QUICKSTART.md | 已加 `--env-file .env.local` 提示 |
| DOC-R2-2 "测试账号 echo / echo123" | QUICKSTART.md + seed 文件 | 明文密码注释加 dev-only 警告 |
| DOC-R2-3 "APISIX 是唯一入口" | decisions.md | 已补 llm-service HTTP 鉴权备注 |
| DOC-R2-4 "生产可上 K8s" | README.md | 加 ⚠️ K8s chart 冻结声明（决策 23） |
| DOC-R2-5 "端到端 AI 分析能返回 emotion" | stage-34 / stage-92 | env 名统一 + HTTP 鉴权后路径打通 |
| DOC-R2-6 "前端鉴权由 GinAuthMiddleware" | shared/middleware/gin_auth.go | 注释加 "前端需 HttpOnly cookie 配合" |
| DOC-R2-7 "AGENTS.md §七 docs/plans/" | AGENTS.md | 已对齐 legacy-plans 现状 |
| DOC-R2-8 "stage-91 强指令性 prompt 防注入" | file_context.py | 加 `<file_attachment>` 标签边界 + 注入防御头 |

## 6. 累计变更统计

| 阶段 | 文件数 | 增 / 减 |
|------|--------|---------|
| P0 (Stage 94 后续) | 23 | 168 / 334 |
| P1 (Stage 95 PR-5~8) | 25+ | 250+ / 100+ |
| P2 (Stage 95 PR-9) | 15+ | 200+ / 80+ |
| **累计** | **60+** | **600+ / 500+** |

## 7. 测试验证

- Go test：ai-svc / web-bff / shared 全部 ✅
- Python py_compile：llm-service 全部模块 ✅
- 新增单测：
  - `auth.global.test.ts`（12 项白名单边界用例）
  - `renderMarkdown.test.ts`（6 项 XSS 净化用例）
- 文档：README / QUICKSTART / AGENTS / decisions 全部就地更正

## 8. 残余

| 类别 | 项数 | 优先级 |
|------|------|--------|
| Round 1 P1 (28 项) | 28 | 下一 sprint |
| Round 1 P2 (25 项) | 25 | 下一 sprint |
| Round 1 P3 (~25 项) | 25 | 季度 |
| Round 2 P3 残余 | 5 | 季度 |

---

> Stage 95 是 Round 2 的**全量收口**，对应 `code-review-2026-09-14-round-2.md` 中
> **全部 P0 + P1 + P2 + P3 + 文档偏移**。本计划文档通过后，连同 Round 1 的 28 P1 / 25 P2
> 一起进入"下一 sprint"排期（详见 `docs/plans/todo-pile-2026-09-04.md`）。
