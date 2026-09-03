# Stage 35 · LLM Fusion 生产加固（Production Hardening）

> 状态：**进行中（In Progress）** · 日期：2026-09-03 · 目标分支：`feat/bff-fused-emotion-endpoint`（延续 Stage 31-34 节奏）
> ADR 编号：15（LLM Fusion 容错 + 限流 + 可观测策略）— 见 `adr-2026-09-llm-fusion-hardening.md`
> 配合文档：[stage-34-ops-runbook.md](../deployment/runbook/stage-34-ops-runbook.md) §三（待补清单）
> 收口后落地报告：[stage-35-landing.md](/docs/stages/stage-35-landing.md)

---

## 一、动机

Stage 34 完成多模态融合数据通路（face/voice/fused 表 + FusionWorker + BFF `/fused` + analytics 报表），并通过 docker smoke（postgres-only + grpcurl）验证端到端。但 `stage-34-ops-runbook.md §三` 列出 **7 项生产短板**：

| 等级 | 待补项 | 业务影响 |
|------|--------|---------|
| 🔴 高 | LLM ` ```json ``` ` markdown 包装容错 | 真实 LLM（DeepSeek/OpenAI）几乎必返回 markdown 包，代码不解 → 100% 走 fallback |
| 🔴 高 | LLM 输出 schema 校验（必填字段、数值范围） | LLM 偶发返回缺字段或 sentiment_score 越界，被接受后污染下游 |
| 🟡 中 | Worker 同 messageID LRU 限流 | tick 周期内同一 msgID 被多次 fuser 调用，浪费 LLM 配额 |
| 🟡 中 | LLM 超时调短（3–5s） | 当前 10s 默认，1s 慢响应拖垮 Worker |
| 🟡 中 | LLM retry + circuit breaker | 真 LLM 连续失败无降级保护，反复打挂 provider |
| 🟡 中 | Prometheus metrics | 现行 0 观测，生产盲飞 |
| 🟡 中 | yaml `${NACOS_ENABLED:-true}` 占位符恢复 | smoke 临时硬编码 `false` 上生产会断开 Nacos |

Stage 35 把这 7 项**全部关掉**，并补 landing 报告 + 更新 ops runbook。

---

## 二、范围 / 非范围

### ✅ 在范围内
- 仅修改 `emotion-echo-ai-svc/`（主战场），`emotion-echo-shared/pkg/metrics` 扩展新指标 collector
- 配置层恢复 `etc/ai-api.yaml` 占位符 + main.go env override 补全
- 测试：单测 + httptest mock + fake repo + metrics counter 断言

### ❌ 不在范围内（Stage 36+）
- 真实 FER / SenseVoice 集成（待 `profile: ai` 镜像就绪）
- 真实 LLM smoke（需 API key 与网络，本 stage 不强制）
- Stage 33 deferred（Kafka DLQ / DB migration tool / CI/CD / Nacos HA / Redis 共享限流）
- 前端 ECharts 多 series 渲染
- 5s 时窗融合 / 数字人表情驱动

---

## 三、设计总览

| 关注点 | 决策 | ADR 章节 |
|--------|------|----------|
| LLM 输出包装 | `unwrapLLMContent` 三段去包（``` ``` ``` ``` + 空白 + 双重 JSON） | ADR-15 §A |
| LLM 输出校验 | 白名单 emotion + sentiment ∈ [-1,1] + modality_contrib 总和 ≈ 1 | ADR-15 §B |
| 同 msgID 重复融合 | LRU(cap=1024) + TTL=4min | ADR-15 §C |
| LLM 超时 | 默认 3s（可 env 覆盖） | ADR-15 §D |
| LLM 雪崩 | 三态熔断（Closed→Open→HalfOpen），连续 5 失败开 30s；不重试 | ADR-15 §E |
| 可观测 | Prometheus 4 个 collector（LLM call / latency / fallback / worker tick） | ADR-15 §F |
| 配置 | yaml 恢复 `${VAR:-default}` 占位符；main.go `applyEnvOverrides` 补 `LLM_TIMEOUT` 等 | ADR-15 §G |

---

## 四、PR 拆解（8 个）

每个 PR 严格三段 commit（test: RED → feat: GREEN → refactor:），遵循 AGENTS.md §〇。

| PR | 主题 | 测试文件 | 涉及源码 |
|----|------|----------|----------|
| **PR-1** | markdown 解包 | `llm_content_unwrap_test.go` | `llm_content_unwrap.go` + `llm_fuser.go` 改造 |
| **PR-2** | schema 校验 | `llm_output_validate_test.go` | `llm_output_validate.go` + `llm_fuser.go` 改造 |
| **PR-3** | Worker LRU | `lru_test.go` + `worker_test.go` 追加 | `lru.go` + `worker.go` 改造 |
| **PR-4** | LLM 超时 | `llm_fuser_test.go` 追加 | `llm_fuser.go` `NewLLMFuser` 默认值 |
| **PR-5** | circuit breaker | `llm_breaker_test.go` | `llm_breaker.go` + `llm_fuser.go` 包装 |
| **PR-6** | metrics | `fusion_metrics_test.go` + `shared/metrics/fusion_metrics_test.go` | `shared/.../fusion_metrics.go` + `llm_fuser.go` + `worker.go` |
| **PR-7** | yaml 占位符 | `yaml_config_test.go`（env override 集成） | `etc/ai-api.yaml` + 注释 |
| **PR-8** | env override 补全 | 复用 PR-7 测试 | `main.go applyEnvOverrides` |

合计 ~870 LoC（含测试与注释）。

---

## 五、TDD 节奏示例（PR-1）

```bash
# 1. RED
# 写 llm_content_unwrap_test.go（5 个 case：纯 JSON / ```json 包裹 / ``` 包裹 / 双重 JSON / 前置自然语言）
go test ./internal/fusion/...  # 红
git commit -m "test(stage-35-pr1): add failing tests for LLM markdown unwrap (RED)"

# 2. GREEN
# 写 unwrapLLMContent 最小实现
go test ./internal/fusion/...  # 绿
git commit -m "feat(stage-35-pr1): implement LLM markdown unwrap to satisfy test (GREEN)"

# 3. REFACTOR
# 抽常量、加注释、改用 strings.TrimPrefix 代替 regex
go test ./internal/fusion/...  # 保持绿
git commit -m "refactor(stage-35-pr1): extract markdown unwrap helpers (REFACTOR)"
```

---

## 六、风险与回滚

| 风险 | 触发条件 | 回滚方案 |
|------|---------|---------|
| Circuit breaker 误判 | 真 LLM 偶发抖动 → 全员 fallback | breaker 阈值 `LLM_BREAKER_FAIL_THRESHOLD=10` 调大 |
| LRU TTL 过短 | 同一 msgID 在 4 分钟内重复融合 | env `WORKER_LRU_TTL_SECONDS=600` 调长 |
| yaml 占位符恢复后 Nacos 真起来 | docker compose 注入 `NACOS_ENABLED=true` 但 Nacos 容器未起 | `STARTUP_STRICT_DEPS=postgres` 配合 `bootstrap.IsRequired("nacos")=false` |
| metrics 高基数 | path label 误用 URL 模板 | 复用 `GinMetricsMiddleware` 用 `c.FullPath()` |
| shared 包变更影响其他 5 svc | fusion metrics 引入新 import panic | `shared/pkg/metrics/fusion_metrics.go` 完全自包含，不依赖 fusion 包 |

---

## 七、验收标准（Definition of Done）

1. ✅ `go test ./...` 在 ai-svc / shared 全绿（fusion 包 ≥90% 覆盖）
2. ✅ `go vet ./...` 全过
3. ✅ docker smoke 3 场景仍通过（postgres + ai-svc + grpcurl /fused）
4. ✅ `emotion_echo_fusion_llm_call_total{outcome="..."}` 在 `/metrics` 可见
5. ✅ `stage-34-ops-runbook.md §三` 7 项全部标记 ✅
6. ✅ ADR-15 在 `docs/architecture-decisions.md` 注册（决策 15）
7. ✅ Branch `feat/bff-fused-emotion-endpoint` 累计 commit +8（PR-1..8）至 +72 ahead of main

---

## 八、不在本次范围

- 不改 BFF / chat-svc / user-svc / analytics-svc
- 不动 proto（fused 协议已是最终版）
- 不动前端
- 不做真实 LLM smoke（待 DeepSeek/OpenAI API key 配齐后单独 stage）
- 不做 K8s / Helm 改动