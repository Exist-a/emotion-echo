# ADR-16 · Stage 35 系统缺口正式记录（2026-09-01）

> **状态**：已登记 · **类型**：已知缺口（Known Gaps）· **优先级**：见 [stage-36-fixes-roadmap.md](/docs/stages/stage-36-fixes-roadmap.md)
> **来源**：[stage-35-system-feasibility.md](/docs/stages/stage-35-system-feasibility.md) 验证报告（commit `a5e8cd2`）
> **ADR 编号**：16

---

## 上下文（Context）

Stage 35 docker smoke + 业务端到端验证（6 个 Go svc 容器化运行、Postgres 5 schema 联通）发现系统**业务核心链路可用**，但存在 **8 项已知缺口** 影响生产 readiness 与功能完整性。

这些缺口不是"新发现的 bug"，而是：
- 早在 Stage 32/33/34 ADR / runbook 中标记为 deferred
- 在 Stage 35 跑通业务端到端时**首次被实证影响业务功能**

按用户要求，**所有缺口正式纳入修复日程**（不再 deferred 到 Stage 36+），逐项落到 Stage 36 计划。

---

## 缺口清单（8 项）

| # | 缺口 | 严重度 | 业务影响 | 决策依据 |
|---|------|--------|----------|----------|
| G1 | 4 个 Go svc（user/chat/analytics/assessment）的 yaml 仍含 `${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}` 字面占位符 → go2sky dial 失败循环 | 🟡 中 | 日志噪音，**不阻塞** HTTP/gRPC listen 与业务调用 | 与 Stage 35 PR-7 同源问题，ai-svc 已修 |
| G2 | chat-svc 缺 `GET /api/v1/conversations` 列表端点 | 🟡 中 | 前端"会话列表"页面打不开；BFF list 调用返回空 | 设计遗漏（chat-svc main.go 注释确认"4 路由"只含 POST create + POST/GET messages） |
| G3 | BFF 缺 analytics / assessment 路由聚合 | 🔴 高 | 前端"情绪分析报表"与"心理量表"两个核心模块打不开 | Stage 32 BFF 净化时只保 5 个 health check，未加业务代理 |
| G4 | Kafka 异步管道（chat-svc → ai-svc）默认关 → 消息不会自动情绪分析 | 🔴 高 | 用户发消息后，ai-svc 没有自动写 emotion_analysis；前端情绪分析只能"等 Kafka 起来"或"手动 SQL insert" | smoke 模式 KAFKA_ENABLED=false 设计如此，但缺 dev fallback |
| G5 | 真实 LLM endpoint（DeepSeek/OpenAI）未配 → emotion fusion 永远走 late_fuser_weighted | 🔴 高 | Stage 35 PR-1+PR-2 的 markdown 容错 / schema 校验在生产中**未实测**；emotion 永远是 text 单模态加权平均 | 需 API key + 网络 |
| G6 | 多模态（FER/SenseVoice/XTTS）`profile: ai` 镜像未构建 | 🟡 中 | image / audio 多模态情绪不可用，前端只能传 text | Stage 22-A 引入，Stage 34 ops runbook §二.2 标记未验证 |
| G7 | APISIX 网关 + apisix-dashboard 镜像不可拉 | 🟡 中 | 完整 docker compose 全栈跑不起来；dev / prod 只能跑 BFF 直连模式 | Stage 31 PR-12 引入；Stage 34 ops runbook §四 标记镜像不可用 |
| G8 | Nacos 配置中心未启 → 所有 svc 用 env override / yaml 直读 | 🟢 低 | 当前等价于 Nacos 功能（runtime env 注入），但**生产应该用 Nacos**做配置中心化 | Stage 31 PR-09 引入；本次 smoke 主动跳过（`NACOS_ENABLED=false`） |

---

## 决策（Decisions）

### §A. 全部 8 项进入 Stage 36 修复日程

**不再 deferred**。每项都建立独立 PR + 测试 + 收口文档。

### §B. 优先级矩阵（Stage 36 排序）

| 批次 | 项 | 时间窗口 | 阻塞关系 |
|------|---|----------|----------|
| **Stage 36-A 立即修** | G1（yaml 占位符 4 svc）+ G3（BFF 路由）| 1-2 天 | 无，可并行 |
| **Stage 36-B 高优先** | G2（chat-svc list）+ G4（Kafka fallback）| 2-3 天 | G3 完成后前端"列表/聊天"全通 |
| **Stage 36-C 中优先** | G5（真实 LLM）+ G6（FER/SenseVoice）| 3-5 天 | 需外部资源（API key / 模型镜像）|
| **Stage 36-D 低优先** | G7（apisix-dashboard 镜像）+ G8（Nacos 全栈）| 2-3 天 | 与生产 readiness 关联，不阻塞 dev |

### §C. 修复策略统一原则

每项缺口修复遵循：
1. **先 RED test**（AGENTS.md §〇 TDD 强制）
2. **最小 GREEN 实现**
3. **smoke 验证**（docker 容器化跑通，与 Stage 35 同模式）
4. **landing doc 收口**
5. **ADR 更新**（若影响架构决策）

### §D. 不再"deferred"的边界

> Stage 36 之后的所有缺口，**默认进入修复日程**，不再写入 runbook / planning 文档当 deferred。
> 唯一例外：需要外部资源（API key、付费服务）且无 dev 环境的项，标记为 **blocked-external**。

---

## 后果（Consequences）

### ✅ 正向

- 用户/产品视角：所有"看起来能用但实际打不开"的功能都有明确修复时间表
- 工程视角：避免 Stage 36/37 重复出现"还有 N 个坑没修"
- 文档视角：`stage-36-fixes-roadmap.md` 成为后续 sprint 的依据

### ⚠️ 代价

- Stage 36 任务量比预期多（8 项 vs 原 3 项 deferred）
- 部分项（G5/G6/G7）需外部资源（API key / 模型 / 镜像），可能无法在 1-2 周内全部完成
- Stage 36 必须严格按 §B 批次执行，避免被 G5/G6 阻塞而拖延 G1/G2/G3/G4

### ❌ 风险

- 如果 G5（真实 LLM）API key 申请被拒，Stage 36-C 整体降级为"代码完成 + smoke 用 mock LLM"
- G7（apisix-dashboard）镜像上游可能不再维护，需要切换到 fork（如 apache/apisix-dashboard:3.20.0）

---

## 参照（References）

- 来源：[stage-35-system-feasibility.md](/docs/stages/stage-35-system-feasibility.md)
- Stage 35 smoke：[stage-35-smoke-validation.md](/docs/stages/stage-35-smoke-validation.md)
- Stage 34 ops runbook（部分待补项来源）：[stage-34-ops-runbook.md](../../deployment/runbook/stage-34-ops-runbook.md)
- Stage 33 deferred list：[stage-33-landing.md](/docs/stages/stage-33-landing.md) §七
- 修复计划：[stage-36-fixes-roadmap.md](/docs/stages/stage-36-fixes-roadmap.md)
- ADR-15（Stage 35 生产加固）：[adr-2026-09-llm-fusion-hardening.md](/docs/architecture/adr/adr-2026-09-llm-fusion-hardening.md)
- ADR-1~14：[architecture-decisions.md](/docs/architecture/decisions.md)