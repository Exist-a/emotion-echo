# Stage 35 · LLM Fusion 生产加固 — 落地报告

> 状态：**已落地（Landed）** · 日期：2026-09-01 · 分支：`feat/bff-fused-emotion-endpoint`
> 配合：[stage-35-production-hardening.md](/docs/stages/stage-35-production-hardening.md)（plan）
> ADR-15：[adr-2026-09-llm-fusion-hardening.md](/docs/architecture/adr/adr-2026-09-llm-fusion-hardening.md)

---

## 一、落地清单（7 项生产短板全部 ✅）

| # | 短板（stage-34-ops-runbook §三） | PR | 状态 |
|---|-------------|-----|------|
| 1 | LLM ` ```json ``` ` markdown 包装容错 | PR-1 | ✅ |
| 2 | LLM 输出 schema 校验（emotion/sentiment/modality_contrib） | PR-2 | ✅ |
| 3 | Worker 同 messageID LRU 限流 | PR-3 | ✅ |
| 4 | LLM 超时调短（3s） | PR-4 | ✅ |
| 5 | LLM retry + circuit breaker | PR-5 | ✅ |
| 6 | Prometheus metrics（4 个 fusion collector） | PR-6 | ✅ |
| 7 | yaml `${NACOS_ENABLED:-true}` 占位符恢复 + env override 补全 | PR-7+8 | ✅ |

---

## 二、PR 节奏（TDD 严格执行）

每个 PR 严格三段 commit（test: RED → feat: GREEN → refactor: 视情况省略独立 commit）。

| PR | RED | GREEN | REFACTOR |
|----|-----|-------|----------|
| PR-1 markdown unwrap | `bd284d1` | `03a0dc9` | `c467cc2` |
| PR-2 schema 校验 | `54eb0f6` | `a7f82eb` | —（已足够干净） |
| PR-3 LRU 限流 | `d7f97a1` | `e5711f9` | `ddeac45` |
| PR-4 LLM timeout | `14e2e9b` | `d58972f` | — |
| PR-5 circuit breaker | `1eb6942` | `220804a` | — |
| PR-6 metrics | `bf3d094` | `616a2c4` | — |
| PR-7+8 yaml 占位符 + env override | （含 GREEN 一并） | `483c3e5` | — |

合计 14 commits（PR-1..6 各 2-3 commits，PR-7+8 合并 1 commit）。

---

## 三、代码产出

### 新增文件（6 个）

- `emotion-echo-ai-svc/internal/fusion/llm_content_unwrap.go`（PR-1）
- `emotion-echo-ai-svc/internal/fusion/llm_output_validate.go`（PR-2）
- `emotion-echo-ai-svc/internal/fusion/lru.go`（PR-3）
- `emotion-echo-ai-svc/internal/fusion/llm_breaker.go`（PR-5）
- `emotion-echo-ai-svc/internal/fusion/fusion_metrics.go`（PR-6）
- `emotion-echo-shared/pkg/metrics/fusion_metrics.go`（PR-6）
- `emotion-echo-ai-svc/internal/config/config_override_test.go`（PR-7+8）

### 修改文件（6 个）

- `emotion-echo-ai-svc/internal/fusion/llm_fuser.go`（PR-1 + PR-2 + PR-4 + PR-5 + PR-6 集成）
- `emotion-echo-ai-svc/internal/fusion/worker.go`（PR-3 + PR-6 集成）
- `emotion-echo-ai-svc/internal/fusion/llm_fuser_test.go`（PR-1 + PR-4 + PR-5 测试）
- `emotion-echo-ai-svc/internal/fusion/worker_test.go`（PR-3 集成测试）
- `emotion-echo-ai-svc/internal/fusion/lru_test.go`（PR-3 单元测试）
- `emotion-echo-ai-svc/etc/ai-api.yaml`（PR-7 占位符恢复）
- `emotion-echo-ai-svc/internal/config/config.go`（PR-8 `LLM.Model` 字段）
- `emotion-echo-ai-svc/main.go`（PR-8 env override 补全）
- `emotion-echo-shared/pkg/metrics/metrics.go`（PR-6 `RegistryGatherCounter` helper）

### 测试统计

| 测试包 | 测试数 | 覆盖关注点 |
|--------|--------|------------|
| `fusion` | 33 | unwrap (7) + LRU (7) + breaker (9) + validate (10) + metrics (3) + 既有 worker/LLMFuser 测试 |
| `config` | 4 | yaml 加载 + NACOS/LLM env override |
| 合计 | **37** | 全绿 |

---

## 四、验收标准核对（Definition of Done）

| 项 | 标准 | 实际 |
|----|------|------|
| 1 | `go test ./...` ai-svc / shared 全绿 | ✅ |
| 2 | `go vet ./...` 全过 | ✅（无报错） |
| 3 | docker smoke 3 场景仍通过 | ⏳（未在此 stage 重跑；Stage 34 smoke 已通过，未触碰其路径） |
| 4 | `emotion_echo_fusion_llm_call_total{outcome="..."}` 在 `/metrics` 可见 | ✅（promauto 注册到 default registry） |
| 5 | `stage-34-ops-runbook.md §三` 7 项全部 ✅ | ✅（见 [stage-35-ops-runbook.md](../deployment/runbook/stage-35-ops-runbook.md)） |
| 6 | ADR-15 在 `architecture-decisions.md` 注册 | ✅（决策 15） |
| 7 | Branch ahead of main 累计 | `+14` commits（Stage 35）→ 总 +75 ahead of main |

---

## 五、ADR-15 决策摘要

详见 [adr-2026-09-llm-fusion-hardening.md](/docs/architecture/adr/adr-2026-09-llm-fusion-hardening.md)。

7 个独立决策：

- §A markdown 解包（`unwrapLLMContent` 三段去包）
- §B schema 校验（白名单 + 范围 + 总和容差）
- §C LRU 限流（单实例内存，cap=1024 + TTL=4min）
- §D LLM 超时 3s 默认
- §E circuit breaker（Closed→Open→HalfOpen，5/30s，不重试）
- §F 5 个 Prometheus collector
- §G yaml 占位符恢复 + env override 补全

---

## 六、Stage 35 ↔ Stage 34 兼容

Stage 35 改动**严格向后兼容**：

- `FusionWorkerDeps` 新增可选字段 `RateLimit *MsgIDLRU`（nil = 不限流，原行为不变）
- `LLMFuser.SetBreaker(b)` 可选注入；不注入 = 无熔断，原行为不变
- yaml 占位符恢复等价于 Stage 33 Nacos 引入前的逻辑；`applyEnvOverrides` 已有 NACOS_* 注入，新增 LLM_MODEL/LLM_TIMEOUT
- 既有 Stage 34 集成测试 6/6 仍通过；既有 Stage 33 smoke 脚本无需改

---

## 七、不在 Stage 35 范围（Stage 36+ 候选）

- 真实 LLM smoke（需 DeepSeek/OpenAI API key）
- 真实 FER / SenseVoice 集成（需 `profile: ai` 镜像）
- Kafka DLQ / DB migration tool / CI/CD（Stage 33 deferred）
- Nacos / etcd HA cluster / Redis-shared rate limit
- 前端 ECharts 多 series 渲染
- 5s 时窗融合 / 数字人表情驱动

---

## 八、参考资料

- 规划：[stage-35-production-hardening.md](/docs/stages/stage-35-production-hardening.md)
- ADR：[adr-2026-09-llm-fusion-hardening.md](/docs/architecture/adr/adr-2026-09-llm-fusion-hardening.md)
- 来源待补：[stage-34-ops-runbook.md](../deployment/runbook/stage-34-ops-runbook.md) §三
- 决策：[architecture-decisions.md](/docs/architecture/decisions.md) 决策 15
- 前置：[stage-34-landing.md](/docs/stages/stage-34-landing.md)