# Stage 35 · 运维手册（Ops Runbook）

> **目标读者**：Stage 35 维护者 / QA / 后续开发者
> **目的**：记录 Stage 35 落地的生产加固项 + 新增的可观测指标 + yaml 占位符
> **配合文档**：[stage-35-landing.md](/docs/stages/stage-35-landing.md)（落地报告）/ [stage-35-production-hardening.md](/docs/stages/stage-35-production-hardening.md)（plan）/ [adr-2026-09-llm-fusion-hardening.md](/docs/architecture/adr/adr-2026-09-llm-fusion-hardening.md)

---

## 一、Stage 35 落地项速查

| 关注点 | 落地文件 | 配置 env | 验证 |
|--------|---------|---------|------|
| LLM markdown 容错 | `fusion/llm_content_unwrap.go` | — | 单元（7 case）+ 集成 |
| LLM schema 校验 | `fusion/llm_output_validate.go` | — | 单元（10 case） |
| Worker LRU 限流 | `fusion/lru.go` + `worker.go` 集成 | `WORKER_LRU_CAPACITY` / `WORKER_LRU_TTL_SECONDS` | 单元（7 case）+ 集成（3 case） |
| LLM 超时 | `fusion/llm_fuser.go` `NewLLMFuser` | `LLM_TIMEOUT=3` | 单元（2 case） |
| Circuit breaker | `fusion/llm_breaker.go` + `llm_fuser.go` 包装 | `LLM_BREAKER_FAIL_THRESHOLD=5` / `LLM_BREAKER_OPEN_SECONDS=30` | 单元（9 case）+ 集成（2 case） |
| Prometheus metrics | `shared/pkg/metrics/fusion_metrics.go` + `fusion/fusion_metrics.go` | — | 单元（3 case） |
| yaml 占位符 + env override | `etc/ai-api.yaml` + `main.go` + `internal/config/config.go` | `LLM_MODEL` / `LLM_TIMEOUT` / `NACOS_*` / `KAFKA_ENABLED` / `SKYWALKING_ENABLED` | 集成（4 case） |

---

## 二、新增 Prometheus 指标

### 2.1 列表（5 个 collector）

| 名称 | 类型 | Labels | 含义 |
|------|------|--------|------|
| `emotion_echo_fusion_llm_call_total` | Counter | `outcome` | LLM 调用计数（success/json_parse_err/timeout/http_5xx/invalid_output/circuit_open/other） |
| `emotion_echo_fusion_llm_latency_seconds` | Histogram | `outcome` | LLM 调用耗时 |
| `emotion_echo_fusion_fallback_total` | Counter | `stage` | fallback 计数（llm_to_late） |
| `emotion_echo_fusion_worker_tick_total` | Counter | `outcome` | Worker tick 计数（ok/error/skipped_lru） |
| `emotion_echo_fusion_lru_stat` | Gauge | `kind` | LRU 状态（size/hits/misses） |

### 2.2 查询示例

```promql
# LLM 成功率
sum(rate(emotion_echo_fusion_llm_call_total{outcome="success"}[5m])) /
  sum(rate(emotion_echo_fusion_llm_call_total[5m]))

# Fallback 率（应 < 5%，否则 LLM provider 异常）
sum(rate(emotion_echo_fusion_fallback_total[5m])) /
  sum(rate(emotion_echo_fusion_worker_tick_total{outcome="ok"}[5m]))

# LLM P99 延迟
histogram_quantile(0.99,
  sum(rate(emotion_echo_fusion_llm_latency_seconds_bucket[5m])) by (le))

# LRU 命中率（应稳定，突变 = Worker 配置变更）
rate(emotion_echo_fusion_lru_stat{kind="hits"}[5m]) /
  (rate(emotion_echo_fusion_lru_stat{kind="hits"}[5m]) +
   rate(emotion_echo_fusion_lru_stat{kind="misses"}[5m]))
```

---

## 三、env 配置参考

| 变量 | 默认 | 说明 |
|------|------|------|
| `LLM_BASE_URL` | `http://localhost:8000` | LLM 服务地址（OpenAI 兼容协议） |
| `LLM_MODEL` | `deepseek-chat` | 模型名（DeepSeek / gpt-4 / 任意兼容模型） |
| `LLM_TIMEOUT` | `3` | LLM HTTP 超时（Stage 35 PR-4 由 10s 改为 3s） |
| `LLM_API_KEY` / `LLM_INTERNAL_API_KEY` | (空) | Bearer token；空 = 不发 Authorization |
| `LLM_BREAKER_FAIL_THRESHOLD` | `5` | 熔断器连续失败阈值（触发 Open） |
| `LLM_BREAKER_OPEN_SECONDS` | `30` | 熔断器 Open 持续秒数（之后转 HalfOpen） |
| `WORKER_TICK_INTERVAL` | `5` | FusionWorker tick 周期（秒） |
| `WORKER_LRU_CAPACITY` | `1024` | msgID LRU 容量 |
| `WORKER_LRU_TTL_SECONDS` | `240` | msgID LRU TTL（4 分钟） |
| `NACOS_ENABLED` | `false` | Nacos 注册中心开关 |
| `NACOS_ADDR` | `emotion-echo-nacos:8848` | Nacos 地址 |
| `NACOS_NAMESPACE` | `emotion-echo-dev` | Nacos 命名空间 |
| `NACOS_HOT_RELOAD` | `false` | Nacos 配置热更新 |
| `KAFKA_ENABLED` | `true` | Kafka 消费者开关 |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka brokers |
| `SKYWALKING_ENABLED` | `false` | SkyWalking tracer 开关 |
| `SKYWALKING_OAP_ADDR` | `localhost:11800` | SkyWalking OAP 地址 |
| `FER_BASE_URL` / `SENSEVOICE_BASE_URL` / `XTTS_BASE_URL` | (空) | 多模态模型地址；空 = 走 fallback |

> 注：`LLM_BREAKER_*` / `WORKER_*` / `LLM_MODEL` 等 env 通过 main.go `applyEnvOverrides` 注入到 Config 或在 main 内部传给 fusion 构造器。yaml 占位符 `${VAR:-default}` 不会被 go-zero conf 展开。

---

## 四、Stage 34 待补清单 §三 → 全部 ✅

| 等级 | 待补项 | 处理 |
|------|------|------|
| 🔴 高 | LLM markdown 包装容错 | ✅ Stage 35 PR-1 |
| 🔴 高 | LLM 输出 schema 校验 | ✅ Stage 35 PR-2 |
| 🟡 中 | Worker LRU 限流 | ✅ Stage 35 PR-3 |
| 🟡 中 | LLM 超时调短 | ✅ Stage 35 PR-4 |
| 🟡 中 | LLM retry + circuit breaker | ✅ Stage 35 PR-5（熔断器；不重试） |
| 🟡 中 | Prometheus metrics | ✅ Stage 35 PR-6 |
| 🟡 中 | yaml `${NACOS_ENABLED:-true}` 占位符恢复 | ✅ Stage 35 PR-7+8 |

---

## 五、生产部署清单（升级到 Stage 35 时）

### 5.1 必须确认

- [ ] `LLM_BASE_URL` / `LLM_MODEL` 已通过 docker compose env 注入
- [ ] `LLM_TIMEOUT=3` 已设置（默认就是 3，但生产可显式确认）
- [ ] `KAFKA_ENABLED=true`（compose 默认）已确认
- [ ] `NACOS_ENABLED=true` 已确认（如使用 Nacos）

### 5.2 可选

- [ ] Prometheus 已 scrape ai-svc `:8891/metrics`
- [ ] 已建告警规则：
  - LLM 成功率 < 95% → warn
  - Fallback 率 > 5% → page
  - LLM P99 > 2.5s → warn
  - Circuit breaker Open > 1min → page
- [ ] 已在 Grafana 加 LLM latency P50/P99 panel

### 5.3 回滚（如熔断器误判）

```bash
# 调高阈值（更容忍）
docker exec emotion-echo-ai-svc sh -c 'echo "export LLM_BREAKER_FAIL_THRESHOLD=20" >> /etc/profile'

# 强制 close（紧急时绕过熔断）
# 注：当前实现不支持运行时强制 close；最简方案是重启 svc
docker restart emotion-echo-ai-svc
```

---

## 六、容器启动故障速查（Stage 35 新增）

| 现象 | 原因 | 修复 |
|------|------|------|
| `error: yaml: invalid character` 在 Nacos/Kafka/SkyWalking bool 字段 | go-zero conf 不解析 `${VAR:-default}` 字面 `${` | 已 Stage 35 修复：main.go applyEnvOverrides 注入 env；yaml 占位符保留即可 |
| `emotion_echo_fusion_llm_call_total` 在 `/metrics` 看不到 | promauto 注册到 default registry，需确保 `/metrics` endpoint 是 `promhttp.Handler()`（Stage 25-E 已统一） | 确认 ai-svc/main.go 注册 `/metrics` 用 sharedmetrics.PromHTTPHandler() |
| LLM 命中率 100%（永远命中）| LRU TTL 过长 + Worker tick 周期短，导致 stale row 永远命中 | 调小 `WORKER_LRU_TTL_SECONDS=60` |
| Circuit breaker 一直 Open | provider 真挂 / 阈值过低 | 拉 `emotion_echo_fusion_llm_call_total{outcome="http_5xx"}` 看具体；调高 `LLM_BREAKER_FAIL_THRESHOLD=20` |
| LLM P99 > 3s（持续 timeout） | provider 真慢 | 调高 `LLM_TIMEOUT=5`（注意会拖慢 Worker tick） |

---

## 七、Stage 35 关键路径

```
1. Fusion Worker tick（每 WORKER_TICK_INTERVAL 秒）
   ↓
   PendingLister.ListPending → 候选 messageIDs
   ↓
   对每个 candidate:
     ├─ LRU.Touch(msgID) 命中 → RecordWorkerTick("skipped_lru") + skip
     │
     └─ 未命中：
        ├─ EmotionRepo / FaceRepo / VoiceRepo 拼 ModalitySnapshot
        │
        ├─ LLMFuser.Fuse:
        │   ├─ CircuitBreaker.Allow() == false → ErrCircuitOpen → RecordLLMCall("circuit_open")
        │   │
        │   └─ true → HTTP POST /v1/chat/completions
        │       ├─ 5xx → RecordLLMCall("http_5xx") + RecordFailure
        │       ├─ timeout → RecordLLMCall("timeout") + RecordFailure
        │       └─ 200 OK → unwrapLLMContent + validateLLMOutput
        │           ├─ JSON parse 失败 → RecordLLMCall("json_parse_err") + RecordFailure
        │           ├─ validate 失败 → RecordLLMCall("invalid_output") + RecordFailure
        │           └─ OK → RecordLLMCall("success") + RecordSuccess + Return *FusedEmotion
        │
        ├─ 失败 fallback：
        │   ├─ ErrCircuitOpen / err != nil → RecordFallback("llm_to_late")
        │   └─ LateFuser.Fuse → WeightedLateFuser
        │
        └─ FusedEmotionRepo.Upsert（Postgres ON CONFLICT message_id DO UPDATE）
           └─ LRU.Add(msgID)（记录已融合，防下个 tick 重复）

2. Prometheus scrape /metrics
   ↓
   4 个 fusion collector：
     - emotion_echo_fusion_llm_call_total{outcome=...}
     - emotion_echo_fusion_llm_latency_seconds{outcome=...}
     - emotion_echo_fusion_fallback_total{stage=...}
     - emotion_echo_fusion_worker_tick_total{outcome=...}
     - emotion_echo_fusion_lru_stat{kind=...}
```

---

## 八、相关 git 历史

| Commit | 含义 |
|--------|------|
| `bd284d1` / `03a0dc9` / `c467cc2` | PR-1 markdown unwrap (RED/GREEN/REFACTOR) |
| `54eb0f6` / `a7f82eb` | PR-2 schema 校验 |
| `d7f97a1` / `e5711f9` / `ddeac45` | PR-3 LRU 限流 |
| `14e2e9b` / `d58972f` | PR-4 LLM 3s 超时 |
| `1eb6942` / `220804a` | PR-5 circuit breaker |
| `bf3d094` / `616a2c4` | PR-6 metrics |
| `483c3e5` | PR-7+8 yaml 占位符 + env override |

分支：`feat/bff-fused-emotion-endpoint`