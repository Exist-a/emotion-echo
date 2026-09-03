# ADR-15 · LLM Fusion 生产加固策略（2026-09-03）

> **状态**：已审批 · **Stage**：35 · **分支**：`feat/bff-fused-emotion-endpoint`
> **配合**：[stage-35-production-hardening.md](/docs/stages/stage-35-production-hardening.md)

---

## 上下文（Context）

Stage 34 把 LLM 接入 FusionWorker 后，docker smoke 用 `LLM_BASE_URL=""` 只验证了 fallback 路径。真实 LLM 调用面临 4 个不可回避的问题：

1. **包装不稳定**：DeepSeek / OpenAI 在 system prompt 要求"只输出 JSON"时，仍偶发用 ```` ```json...``` ```` 包裹，导致 `json.Unmarshal` 失败
2. **字段不全**：LLM 偶发漏字段（`primary_emotion=""`）、越界（`sentiment_score=1.5`）、`modality_contrib` 总和≠1
3. **雪崩**：provider 抖动时 Worker 会持续重试、堆积、最终拖垮整个 tick 循环
4. **同 msgID 重复**：一个 tick 内 `ListPending` 可能反复列出同一 msg（数据库更新延迟、tick 与 emit 异步），重复调 LLM 浪费配额

同时，Stage 34 smoke 临时把 yaml 的 `Nacos.Enabled` / `Kafka.Enabled` 硬编码 `false` 绕过 go-zero conf 不解析 `${VAR:-default}` 的坑。这个 workaround 上生产会断开 Nacos 注册。

---

## 决策（Decisions）

### §A. LLM 输出 markdown 容错

`internal/fusion/llm_content_unwrap.go::unwrapLLMContent(raw string) string` 三段去包：

1. 检测 `` ```json\n...\n``` `` 模式 → strip 三反引号 + 可选 `json` 语言标记
2. 检测 `` ```\n...\n``` `` 模式（无语言标记）→ strip
3. `strings.TrimSpace`
4. 若仍是 `{...}` 包裹的合法 JSON 字符串 → 反序列化一次再 `MarshalIndent` 回（处理双重 JSON 编码）

**理由**：正则太脆（语言标记 `json` / `JSON` / `Json` 多变），分阶段判定更稳；双重 JSON 是 DeepSeek 偶发行为，必须覆盖。

**放弃方案**：用 OpenAI `response_format: {type: "json_object"}`（部分兼容实现不支持；系统 prompt 已要求但仍偶发包装）。

### §B. LLM 输出 schema 校验

`internal/fusion/llm_output_validate.go::validateLLMOutput(o llmFusedOutput) error` 三条：

- `PrimaryEmotion ∈ {happy, sad, angry, neutral, calm, anxious, surprised, disgusted, fearful}`（白名单常量）
- `SentimentScore ∈ [-1, 1]`
- `ModalityContrib` 非空、各 value ∈ [0, 1]、总和 ∈ [0.99, 1.01]

**理由**：白名单比正则稳；浮点和用 ε 容差；总和严格 1 防止 LLM 乱写权重。

**放弃方案**：用 `go-playground/validator` 标签（要 reflect 解析 struct tag，运行时开销大，且 emotion 白名单要动态配置反而更复杂）。

### §C. 同 msgID LRU 限流

`internal/fusion/lru.go` 纯 LRU（`container/list` + `map[int64]*list.Element`），cap=1024 默认；TTL 默认 4 分钟（> tick 5s × 48）。

**理由**：Worker 5s tick 周期内同一 msgID 出现在 `ListPending` 多次（如 DB upsert 与 list 有时差、stale pending row），不阻就会重复调 LLM。LRU + TTL 既能容错又限制内存。

**放弃方案**：用 `sync.Map`（无 TTL 概念，需要外层定时器清扫）；用 Redis（跨实例限流需要，但 Stage 33 已 deferred，单实例内存足够）。

### §D. LLM 超时

`NewLLMFuser` 默认 `cfg.Timeout = 3 * time.Second`（原 10s）。yaml `LLM.Timeout: 3` 已对齐。

**理由**：5s tick 周期下，10s 超时意味一次慢 LLM 调用就把整个 tick 卡住；3s 既能覆盖正常 DeepSeek（<2s）又能在抖动时 fail-fast。

### §E. Circuit Breaker（不重试）

`internal/fusion/llm_breaker.go` 三态：

- `Closed`：正常调底层 http；累计连续 N=5 失败 → `Open`
- `Open`：拒绝所有调用，立即返回 `ErrCircuitOpen`；30s 后转 `HalfOpen`
- `HalfOpen`：试 1 次，成功转 `Closed`，失败转 `Open`

**不重试**：retry 会让 Worker 任务堆积；circuit breaker 已经能防雪崩。

**理由**：熔断器比 retry 健康——retry 是"乐观"，熔断是"悲观但节能"。Stage 35 目标是"少打 + 早打"，不是"反复打"。

**配置**：`LLM_BREAKER_FAIL_THRESHOLD=5` / `LLM_BREAKER_OPEN_SECONDS=30`，均 env 覆盖。

### §F. Prometheus Metrics

`emotion-echo-shared/pkg/metrics/fusion_metrics.go` 新增 4 个 collector（自包含、不依赖 fusion 包）：

```go
FusionLLMCallTotal       CounterVec{outcome: success|json_parse_err|timeout|http_5xx|invalid_output|circuit_open|other}
FusionLLMLatencySeconds  HistogramVec{}              // 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
FusionFallbackTotal      CounterVec{stage: llm_to_late|late_to_skip}
FusionWorkerTickTotal    CounterVec{outcome: ok|error|skipped_lru}
```

**理由**：4 个指标覆盖"成功率 / 延迟 / 降级 / Worker 健康"四个维度，足以替代人肉看日志。

**放弃方案**：用 OpenTelemetry（5 个 svc 已统一到 SkyWalking，OTel 重叠）。

### §G. yaml 占位符 + env override 补全

`etc/ai-api.yaml` 把硬编码 `false` 改回 `${VAR:-default}` 占位符，`main.go applyEnvOverrides` 补 5 个 env：

- `LLM_TIMEOUT` → `c.LLM.Timeout`
- `LLM_MODEL` → `c.LLM.Model`
- `LLM_BREAKER_FAIL_THRESHOLD` → 注入 breaker
- `LLM_BREAKER_OPEN_SECONDS` → 同上
- `WORKER_TICK_INTERVAL` → 注入 Worker

**理由**：go-zero conf 不解析 `${VAR:-default}` 是已知坑（commit `b8b344c` 绕过注释），`applyEnvOverrides` 才是 runtime 真相源。让 yaml 显示"真实意图"，让 env 注入负责"实际值"。

---

## 后果（Consequences）

### ✅ 正向
- 真实 LLM 接入不再因包装 / 字段问题 100% fallback
- 雪崩场景下 Worker tick 不再被拖死
- 生产可观测性 0 → 4 个核心指标
- yaml 与代码意图一致，运维无需 hack

### ⚠️ 代价
- 新增 5 个 Go 文件 + 1 个 shared 文件 → 维护面略增
- LRU 单实例内存约 1024 × (16B key + 24B elem) ≈ 40KB，可接受
- circuit breaker 阈值需调优（生产观测后再校准）

### ❌ 不影响
- BFF / 其他 4 Go svc 不变
- proto / frontend 不变
- 既有测试（Stage 34）保持绿（除需小调整的几处 mock 期望）

---

## 参照（References）

- 配合文档：[stage-35-production-hardening.md](/docs/stages/stage-35-production-hardening.md)
- 来源待补清单：[stage-34-ops-runbook.md](../../deployment/runbook/stage-34-ops-runbook.md) §三
- ADR-14（多模态融合算法）：[stage-34-multimodal-fusion.md](/docs/stages/stage-34-multimodal-fusion.md)
- 决策 10/11/12/13：[architecture-decisions.md](/docs/architecture/decisions.md)