# Stage 22 · AI 模型服务容器化（FER / SenseVoice / XTTS）

**日期**：2026-07-15（首版），2026-07-16（状态修正 + Stage 22-B 补充）
**目标**：把 Emotion-Echo 项目里 3 个 AI 模型服务（FER、XTTS、SenseVoice）升级到 Stage 20 标准：
- multi-stage Dockerfile + tini + non-root
- 结构化 JSON 日志
- Prometheus /metrics
- SIGTERM graceful shutdown
- 标准化输出格式
- 集成到 `deploy/docker-compose.apps.yml`（用 profiles 可选启动）
**前置依赖**：Stage 20（emotion-llm-service + ai-svc 模式）。

---

## 一、本阶段总览

| 子阶段 | 内容 | 状态 |
|--------|------|------|
| Stage 22-A.1 | FER（表情识别）容器化 | ✅ |
| Stage 22-A.2 | SenseVoice（语音 ASR + 情绪）容器化 | ✅ |
| Stage 22-A.3 | XTTS（语音克隆 TTS）容器化 | ✅ |
| Stage 22-A.4 | docker-compose 整合 + 端到端验证 | ✅ |
| Stage 22-A.5 | emotion-echo-ai-svc Go 侧客户端接入（aiclient + MultiModalAnalyzer + 18 个单元测试） | ✅ |
| Stage 22-B | env var override + JSON broker 解析（修复 go-zero conf 不支持 `${VAR:-default}` + 容器内网络隔离） | ✅ |

> **📝 状态修正（2026-07-16）**：此文档最初发布时 Stage 22-A.5 标为 ⏳，**实际已在 Stage 23 同期完成**。
> Stage 22-B 是端到端冒烟时（见 [stage-24](stage-24-endpoint-verification-and-bugfix.md)）才发现的 P0 bug —— go-zero conf 不解析 `${VAR:-default}` bash 风格 envsubst，导致容器内 SkyWalking / Postgres / Kafka / LLM 地址全是字面值，必须在 main.go 启动期手动 `os.Getenv` 覆盖。

---

## 二、3 个 AI 服务对照表

| 服务 | 端口 | 模型 | 后端 | CPU 估计内存 | 触发场景 |
|------|------|------|------|--------------|----------|
| **emotion-echo-fer** | 8004 | fer library + OpenCV DNN | fer lib（MTCNN 主）+ OpenCV DNN 备 | ~600MB-1GB | 上传照片分析表情 |
| **emotion-echo-sensevoice** | 8002 | `iic/SenseVoiceSmall` | funasr.AutoModel + torch | ~800MB-1.5GB | 上传音频转写 + 情绪 |
| **emotion-echo-xtts** | 8003 | `AI-ModelScope/XTTS-v2` | Coqui TTS + torch + ModelScope | ~2-3GB | 文本→语音合成 |

3 个加起来 **~3.5-5.5GB RAM**，生产推荐 **8GB+ 内存**；2C2G 机器只能跑核心 4 个服务。

---

## 三、各服务 Dockerfile 设计要点

### FER (Stage 22-A.1)

- 基础镜像 `python:3.10-slim`（OpenCV 4.5 + fer）
- multi-stage builder：装编译工具 → 装 cv2+fer → runtime 仅留 runtime libs
- 两种 backend：`fer` library（MTCNN，主）+ OpenCV DNN（haar+cascade，备）
- 模型：`emotion_net.caffemodel` 不在仓库（~170MB），由本地 mount 或容器 init 时检测
- tini 来自 `apt-get install tini`（glibc 编译版，与 slim 兼容）

### SenseVoice (Stage 22-A.2)

- 基础镜像 `python:3.10-slim`（funasr + torch）
- 模型由 funasr 在 startup 时自动从 ModelScope 下载（`MODELSCOPE_CACHE=/app/cache`）
- 持久化：`emotion-echo-sensevoice-cache` volume，重启不重下模型
- 缓存卷对 XTTS 复用以免重复下载
- `/app/cache` 挂载到 host volume，避免每次重建容器丢失 ~200MB 模型

### XTTS (Stage 22-A.3)

- 基础镜像 `python:3.11-slim`（Coqui-TTS 兼容 3.11 较好）
- **特殊点**：项目里有 vendored TTS 源码（`TTS/TTS/`），用 `sys.path.insert(0, "TTS")` 引用
- Dockerfile 必须 `COPY TTS/ ./TTS/` 让 vendor 跟着进镜像
- 模型 `AI-ModelScope/XTTS-v2` 也在仓库里（已 `snapshot_download` 过），Dockerfile COPY 整个 `AI-ModelScope/`
- 系统依赖：`libsndfile1`、`libgomp1`、`libespeak-ng1`（Coqui TTS 必需）
- 模型加载慢，`start_period=180s` 给足时间

---

## 四、3 个服务的统一模式（参考 emotion-llm-service Stage 20）

每个服务都遵守同样的"Stage 20-P0"模式：

### 4.1 文件结构

```
emotion-echo-ai-svc/   # 已完成，可参考
emotion-echo-fer/      # 新
├── server.py
├── logging_setup.py      # JSON 日志（与 llm-service 一致）
├── metrics_setup.py      # Prometheus（与 llm-service 一致）
├── requirements.txt
├── Dockerfile
└── .dockerignore
```

### 4.2 日志（`logging_setup.py`）

复用 emotion-llm-service 的实现，通过环境变量切换：
- `LOG_FORMAT=json`（默认）| `text`
- `LOG_LEVEL=INFO` | `DEBUG` | `WARNING` | `ERROR`

### 4.3 Metrics（`metrics_setup.py`）

按服务定制的 Prometheus 指标：

| 服务 | Counter | Histogram |
|------|---------|-----------|
| FER | `fer_analyze_total{emotion,status}` | `fer_model_inference_seconds`, `fer_http_request_duration_seconds` |
| SenseVoice | `sensevoice_asr_total{emotion,status}` | `sensevoice_asr_inference_seconds`, `sensevoice_http_request_duration_seconds` |
| XTTS | `xtts_synthesis_total{lang,status}`, `xtts_stream_total{lang,status}`, `xtts_phonemes_total{lang,status}` | `xtts_synthesis_seconds{endpoint}` |

通用：
- `{svc}_http_requests_total{method,path,status}`
- `MetricsMiddleware` 自循环跳过 `/metrics`

### 4.4 Graceful Shutdown（Stage 20-1）

`main()` 里用 `uvicorn.Config(timeout_graceful_shutdown=10~20)` + 自定义 signal handler：

```python
loop.add_signal_handler(sig, server.handle_exit, sig, None)
```

tini 在容器内 PID 1 接收 SIGTERM，向 uvicorn 子进程转发，触发 graceful shutdown。

### 4.5 标准化输出

| 服务 | 端点 | 输出 |
|------|------|------|
| FER | `POST /analyze` | `{emotion, confidence, scores, source}` |
| SenseVoice | `POST /analyze` | `{text, emotion, confidence, raw_text, source}` |
| XTTS | `POST /tts` | `{audio:base64, sample_rate, text, language}` |
| XTTS | `POST /tts_stream` | `audio/wav` 流（带 WAV header） |
| XTTS | `POST /tts_with_phonemes` | `{audio:base64, sample_rate, text, language, phonemes, duration}` |

---

## 五、docker-compose 集成（Stage 22-A.4）

### 5.1 Profiles 设计

`deploy/docker-compose.apps.yml` 增加 3 个 service，**默认不启动**（避免 5GB RAM 把小机器压垮）：

```bash
# 默认：只启动 llm-service + ai-svc（约 1GB RAM）
docker compose -f deploy/docker-compose.apps.yml up -d

# 启动 AI profile（含 3 个模型）
docker compose -f deploy/docker-compose.apps.yml --profile ai up -d
```

### 5.2 服务间互访（容器 DNS）

ai-svc 加 env：
```yaml
FER_BASE_URL:        http://emotion-echo-fer:8004
SENSEVOICE_BASE_URL: http://emotion-echo-sensevoice:8002
XTTS_BASE_URL:       http://emotion-echo-xtts:8003
```

ai-svc 内部的 Go client 会用这些 env 调用对应服务。

### 5.3 资源限制

| 服务 | CPU 限制 | 内存限制 | 启动预留 |
|------|----------|----------|----------|
| fer | 1.0 | 1024M | 0.3 核 / 384M |
| sensevoice | 1.5 | 1536M | 0.5 核 / 512M |
| xtts | 2.0 | 3072M | 0.5 核 / 1024M |
| **3 个一起跑** | **3.5 核** | **5.5 GB** | — |

### 5.4 卷共享

```yaml
volumes:
  sensevoice-cache:                            # 命名卷
    name: emotion-echo-sensevoice-cache
```

`emotion-echo-sensevoice` 和 `emotion-echo-xtts` 共享这个 cache 目录（funasr + modelscope 都用）。

---

## 六、端到端验证

### 6.1 启动

```bash
# 1) 启动基础设施
docker compose -f deploy/docker-compose.infra.yml up -d

# 2) 启动应用 + AI 模型
docker compose -f deploy/docker-compose.apps.yml --profile ai up -d --build
```

### 6.2 验证脚本

```bash
python scripts/verify_ai.py
# 期望输出：
# [OK  ] fer        HTTP :8004/health: status=200 backend=fer
# [OK  ] sensevoice HTTP :8002/health: status=200 device=cpu
# [OK  ] xtts       HTTP :8003/health: status=200 n/a
# === Summary: 3/3 AI services healthy ===
```

### 6.3 手动调用

```bash
# FER
curl http://localhost:8004/metrics

# SenseVoice
curl -X POST -F file=@audio.mp3 http://localhost:8002/analyze

# XTTS
curl -X POST -H "Content-Type: application/json" \
    -d '{"text":"你好世界","language":"zh-cn"}' \
    http://localhost:8003/tts | jq .audio | base64 -d > tts.wav
```

---

## 七、踩坑清单（FAQ）

### Q1：Dockerfile build 超时，pip install 找不到包
**A**：镜像内默认 `pypi.org` 被墙，Dockerfile 已设 `PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple`。如果还卡，加 `--timeout 300~600`。

### Q2：tini exec 失败 — `no such file or directory`
**A**：直接用 `apt-get install tini`（slim 自带 glibc 兼容），**不要**从 github 下载静态 glibc 编译版。

### Q3：SenseVoice 启动极慢 / 一直 starting
**A**：funasr 自动从 ModelScope 下载 ~200MB 模型（首次）。`emotion-echo-sensevoice-cache` 卷保存后下次启动就快。cold start 常见 60-120s，所以 `start_period: 120s`。

### Q4：XTTS 容器启动报 `TTS.tts.configs.xtts_config ImportError`
**A**：项目里 vendored `TTS/TTS/` 目录必须 COPY 进镜像。Dockerfile 第 2 阶段 `COPY --chown=app:app TTS/ ./TTS/`。

### Q5：FER 容器跑通但 `model_loaded=false`
**A**：`emotion_net.caffemodel`（~170MB）不在 git 里。需要单独挂载或运行 `python -c "from fer import FER; FER().detect_emotions(...)" ` 触发下载，复制到容器内。

### Q6：docker compose build context 报错 `Cannot find dockerfile`
**A**：`docker-compose.apps.yml` 在 `deploy/` 下，AI 服务的 build context 是 `../Emotion-Echo-LLM`（不是 `../../`）。

### Q7：2C2G 机器能跑吗
**A**：跑 `llm-service + ai-svc` 行（约 1GB）。**3 个 AI 服务需要 ~5.5GB，建议 8GB+**。小的先砍掉 `xtts`（最重），再砍 `sensevoice`。

### Q8（NEW 2026-07-16）：容器内 ai-svc 找不到 postgres / sw-oap
**A**：compose v2 起，**只有显式声明 `networks:` 字段的 service 才连接该网络**。postgres / skywalking-oap 默认只连 `deploy_default`，与 apps 的 `app-network` 隔离。修复：在 `docker-compose.infra.yml` 给这两个服务加：
```yaml
networks:
  - app-network
```

### Q9（NEW 2026-07-16）：yaml 里 `${VAR:-default}` 没替换
**A**：go-zero conf 不解析 bash 风格 envsubst。**不要**改 yaml 改用 `$VAR` —— yaml 字符串里 `$VAR` 同样不解析。只能在 main.go 用 `os.Getenv` 手动覆盖。详见 [Stage 22-B](#十一stage-22-b-env-var-override)。

---

## 八、Stage 22-A.5（Go 侧接入）✅

### 8.1 设计

`emotion-echo-ai-svc/internal/aiclient/`：

```
aiclient/
├── ai_client.go          # 公共错误 + 共享类型
├── ai_client_test.go     # 11 个单元测试（mock HTTP server）
├── fer.go                # FER 客户端（multipart upload + JSON response）
├── sensevoice.go         # SenseVoice 客户端
├── xtts.go               # XTTS 客户端（返回 base64 WAV）
```

3 个 client 通过 `FER_BASE_URL` / `SENSEVOICE_BASE_URL` / `XTTS_BASE_URL` 环境变量配置。
BaseURL 为空时 client 自动是 `nil`（feature disabled），调用方做 nil-check 即可优雅降级。

### 8.2 调用点

在 `internal/analyzer/analyzer.go` 加多模态路由：

| 输入类型 | 路由 |
|----------|------|
| 文本 | emotion-llm-service（已有）|
| 图像 bytes | FER（新增）|
| 音频 bytes | SenseVoice → 文本 → emotion-llm-service（新增）|

`internal/analyzer/multimodal.go`（新增）实现 `MultiModalAnalyzer`：
- 自动检测 kind（text/image/audio）
- kind=image 时调 `FER.AnalyzeImage`
- kind=audio 时调 `SenseVoice.Analyze`（返回 transcript + emotion）
- AI 服务不可用时降级到 keyword analyzer
- 7 个单元测试覆盖所有降级路径

### 8.3 单元测试覆盖

| 包 | 文件 | 测试数 |
|----|------|--------|
| aiclient | ai_client_test.go | 11 |
| analyzer | multimodal_test.go | 7 |

合计 **18 个** 单测，覆盖：
- 客户端创建 / nil-check / 默认参数
- 成功路径（httptest mock server）
- 失败路径（upstream error / empty bytes）
- 多模态路由分发
- AI 不可用降级
- 鉴权 wrapper

---

## 九、产出物

| 文件 | 作用 |
|------|------|
| Emotion-Echo-LLM/FER/server.py | 升级（JSON 日志 + /metrics + graceful）|
| Emotion-Echo-LLM/FER/logging_setup.py | 新 |
| Emotion-Echo-LLM/FER/metrics_setup.py | 新 |
| Emotion-Echo-LLM/FER/Dockerfile | multi-stage + tini + non-root |
| Emotion-Echo-LLM/FER/requirements.txt | 加 prometheus-client + python-json-logger |
| Emotion-Echo-LLM/FER/.dockerignore | 新 |
| Emotion-Echo-LLM/sensevoice-small/server.py | 同 FER |
| Emotion-Echo-LLM/sensevoice-small/logging_setup.py | 同上 |
| Emotion-Echo-LLM/sensevoice-small/metrics_setup.py | 同上 |
| Emotion-Echo-LLM/sensevoice-small/Dockerfile | 同上（含 funasr cache 卷）|
| Emotion-Echo-LLM/sensevoice-small/requirements.txt | 加新依赖 |
| Emotion-Echo-LLM/sensevoice-small/.dockerignore | 同上 |
| Emotion-Echo-LLM/XTTS/server.py | 同上 |
| Emotion-Echo-LLM/XTTS/logging_setup.py | 同上 |
| Emotion-Echo-LLM/XTTS/metrics_setup.py | 同上 |
| Emotion-Echo-LLM/XTTS/Dockerfile | multi-stage + tini + non-root + 复制 vendored TTS/ |
| Emotion-Echo-LLM/XTTS/requirements.txt | 加新依赖 |
| Emotion-Echo-LLM/XTTS/.dockerignore | 同上（不 COPY 模型）|
| deploy/docker-compose.apps.yml | 加 3 个 service + `profiles: ["ai"]` |
| scripts/verify_ai.py | 端到端冒烟测试 |
| emotion-echo-ai-svc/internal/aiclient/*.go | 3 个 AI 客户端 + 11 tests（Stage 22-A.5）|
| emotion-echo-ai-svc/internal/analyzer/multimodal.go | 多模态路由 + 7 tests |
| emotion-echo-ai-svc/main.go | applyEnvOverrides + applyDefaultFallbacks + JSON broker（Stage 22-B）|
| emotion-echo-ai-svc/internal/handler/multimodal_handler.go | 503 vs 500 区分（Stage 23 + 24）|
| emotion-echo-ai-svc/internal/logic/aihealthlogic.go | 并行 3 个 AI 探活（Stage 23）|

---

## 十、Stage 23+ 候选

> 详见 [stage-25-roadmap.md](stage-25-roadmap.md)。

- Stage 23：AI 服务对外 HTTP 网关（3 个 endpoint）— ✅ 完成
- Stage 24：端到端验证 + 6+ bug 修复 — ✅ 完成
- Stage 25：本周推荐路径（git init / proto 规范化 / 其他 svc metrics / 前端集成 / K8s 化）
- Stage 26+：TTS 流式 / chat-svc 串联 / Web E2E / DB 迁移 / CI/CD

---

## 十一、Stage 22-B（env var override）

### 11.1 背景

docker compose 注入：
```yaml
environment:
  POSTGRES_DSN: ${POSTGRES_DSN:-host=postgres ...}
  SKYWALKING_OAP_ADDR: ${SKYWALKING_OAP_ADDR:-localhost:11800}
  KAFKA_BROKERS: ${KAFKA_BROKERS:-localhost:9092}
```

ai-svc 的 `etc/ai-api.yaml`：
```yaml
Postgres:
  DSN: ${POSTGRES_DSN:-host=localhost ...}
SkyWalking:
  OAPAddr: ${SKYWALKING_OAP_ADDR:-localhost:11800}
Kafka:
  BrokersCSV: ${KAFKA_BROKERS:-localhost:9092}
```

**问题**：go-zero conf 不解析 `${VAR:-default}` 这种 bash 风格 envsubst。
- yaml 里整串被当成字面值
- 日志里看到 `address ${SKYWALKING_OAP_ADDR:-localhost:11800}` 完全没替换

### 11.2 修复

[main.go:75-103](file:///d:/源码/Emotion-Echo/emotion-echo-ai-svc/main.go#L75-L103) 添加：

```go
// applyEnvOverrides reads OS env vars and patches c.* fields.
// Precedence: env > yaml.
func applyEnvOverrides(c *config.Config) {
    if v := os.Getenv("POSTGRES_DSN"); v != "" {
        c.Postgres.DSN = v
    }
    if v := os.Getenv("KAFKA_BROKERS"); v != "" {
        c.Kafka.BrokersCSV = v
    }
    if v := os.Getenv("SKYWALKING_OAP_ADDR"); v != "" {
        c.SkyWalking.OAPAddr = v
    }
    // ... LLM / FER / SenseVoice / XTTS ...
}

// applyDefaultFallbacks fills safe defaults when both yaml and env are empty.
func applyDefaultFallbacks(c *config.Config) {
    if c.Postgres.DSN == "" {
        c.Postgres.DSN = "host=localhost port=5432 user=postgres password=postgres dbname=emotion_echo sslmode=disable search_path=emotion_echo_ai"
    }
    // ... kafka / skywalking / llm ...
}
```

[main.go:143](file:///d:/源码/Emotion-Echo/emotion-echo-ai-svc/main.go#L143) 在 `main()` 调用：

```go
conf.MustLoad(*configFile, &c)
applyEnvOverrides(&c)
applyDefaultFallbacks(&c)
```

### 11.3 Kafka broker 解析升级

[main.go:389-412](file:///d:/源码/Emotion-Echo/emotion-echo-ai-svc/main.go#L389-L412) `kafkaBrokers()` 支持 JSON 数组形式：

```go
func kafkaBrokers(csv string) []string {
    csv = strings.TrimSpace(csv)
    // 1) JSON array form
    if strings.HasPrefix(csv, "[") {
        var arr []string
        if err := json.Unmarshal([]byte(csv), &arr); err == nil {
            return arr
        }
    }
    // 2) CSV form
    csv = strings.Trim(csv, `"[]`)
    parts := strings.Split(csv, ",")
    // ...
}
```

支持输入：
- `"emotion-echo-kafka:9092"`（单 broker）
- `"kafka1:9092,kafka2:9092"`（CSV）
- `'["kafka1:9092","kafka2:9092"]'`（compose yaml list 单引号 → JSON array）

### 11.4 容器内网络修复

[deploy/docker-compose.infra.yml](file:///d:/源码/Emotion-Echo/deploy/docker-compose.infra.yml) 给 postgres 和 skywalking-oap 显式加 `app-network`：

```yaml
postgres:
  ...
  networks:
    - app-network

skywalking-oap:
  ...
  networks:
    - app-network
```

原因：docker compose v2 起，service 必须显式声明 `networks:` 字段才会连该网络。否则只连默认 `deploy_default`，与 apps 的 `app-network` 完全隔离。

### 11.5 验证

Stage 24 跑完 `verify_stage23_endpoints.py`：

```
[OK] GET /api/v1/ai/health → 200
   - FER/SenseVoice/XTTS 都是 down（AI profile 未启）但 enabled=true
[OK] POST /api/v1/multimodal/analyze (kind=text) → emotion=happy
[OK] POST /api/v1/multimodal/analyze (kind=audio) → emotion=neutral (降级)
[OK] POST /api/v1/tts/synthesize → 503 (XTTS 未启)
=== Summary: 3/4 Stage 23 endpoints healthy ===
```

**Stage 22-B 修复前**：`/api/v1/ai/health` 返 200 但所有项都报 `dial tcp: missing port in address` —— 因为 OAPAddr 是字面值 `${SKYWALKING_OAP_ADDR:-localhost:11800}`，端口解析失败。
**Stage 22-B 修复后**：DNS 解析走容器网络，正确报 `lookup emotion-echo-sw-oap` —— 错误更准确，前端可区分。

---

**当前进度（2026-07-16 更新）**：
- ✅ Stage 22-A.1 / A.2 / A.3 / A.4 完成
- ✅ Stage 22-A.5（Go 侧客户端）完成
- ✅ Stage 22-B（env override + 网络修复）完成
- ✅ Stage 23（HTTP gateway endpoint）完成
- ✅ Stage 24（端到端验证 + bug 修复）完成

下一步见 [stage-25-roadmap.md](stage-25-roadmap.md)。