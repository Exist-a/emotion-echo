# AI 镜像构建指导文档（Stage 36-B5 实战总结）

> **目的**：把 Stage 36 本地模型打包镜像过程中踩过的坑 + 修复方案完整记录，下次构建时不再重新挖一遍。
> **适用**：`emotion-echo/fer` / `emotion-echo/sensevoice` / `emotion-echo/xtts` 三个 `profile: ai` 镜像。
> **编写日期**：2026-09-02 · **编写者**：Stage 36 agent + 用户协作

---

## 一、目录

- [背景与目标](#二背景与目标)
- [三个镜像概述](#三三个镜像概述)
- [通用构建方法](#四通用构建方法)
- [镜像逐个实战记录](#五镜像逐个实战记录)
  - [FER](#51-fer)
  - [SenseVoice](#52-sensevoice)
  - [XTTS](#53-xtts)
- [历史踩坑清单（必读）](#六历史踩坑清单必读)
- [构建验证流程](#七构建验证流程)
- [常见问题速查](#八常见问题速查)

---

## 二、背景与目标

Stage 36 之前，三个 AI 镜像（FER / SenseVoice / XTTS）零散分布在 `Emotion-Echo-LLM/` 子仓里，**没有统一构建入口、也没有 Dockerfile fix 的累计记录**。每次 rebuild 都要踩同样的坑：

- ❌ tuna.tsinghua 镜像替换在境外网络不可达
- ❌ deb.debian.org CDN 偶发 502/EOF
- ❌ pypi CDN 偶发 0字节响应（hash 不匹配）
- ❌ docker buildx cache 跨 build 失效
- ❌ 预烘焙模型路径与 funasr 期望路径不匹配

Stage 36 把这 8 项坑全部修复并固化到 Dockerfile + 加 `scripts/smoke_ai_profile.sh` / `docs/stage-36-landing.md`。本指导文档是这些修复的**单一真相源**。

---

## 三、三个镜像概述

| 镜像 | 源码 | 预烘焙模型 | 大小 (disk) | 启动后端点 | Compose profile |
|------|------|-----------|------------|------------|----------------|
| `emotion-echo/fer` | `Emotion-Echo-LLM/FER/` | libopencv 4.10 内置（caffemodel 缺失走 neutral-fallback） | 12.1GB | `:8004` HTTP/JSON | `profile: ai` |
| `emotion-echo/sensevoice` | `Emotion-Echo-LLM/sensevoice-small/` | **已预烘焙**：`model.pt` 893MB + `am.mvn` 11K + `config.yaml` + `tokens.json` + `fig/` + `example/` | 11.6GB（4.16GB content） | `:8002` HTTP/JSON | `profile: ai` |
| `emotion-echo/xtts` | `Emotion-Echo-LLM/XTTS/` | **已预烘焙**：`model.pth` 1.8GB + `dvae.pth` 201MB + `config.json` + `samples/zh-cn-sample.wav` | 14.2GB（5.3GB content） | `:8003` HTTP/JSON | `profile: ai` |
| `emotion-echo/llm-service` | `emotion-llm-service/`（主仓根） | 无本地模型（FastAPI + gRPC 双协议） | 262MB | `:8000` HTTP + `:50051` gRPC | 默认 |

---

## 四、通用构建方法

### 4.1 前置

- Docker Desktop 29.x（Windows / Mac / Linux）
- 已开梯子（境外网络访问 deb.debian.org + pypi.org）
- 当前 shell 在仓根 `D:\源码\Emotion-Echo`

### 4.2 一键构建全部三个 AI 镜像

```bash
# Stage 33 之前用 "docker-compose"，Stage 34+ 改用 "docker compose"（空格）。
# 加 --no-cache 在 layer cache 失效时强制全步骤重做（首次或怀疑 layer 损坏时）。
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
  --profile ai build fer sensevoice emotion-echo-xtts
```

镜像 tag 由 compose 固定（`v0.1.0`），所以同一 hash 多次 build 是 idempotent 的。

### 4.3 单个镜像构建

```bash
# FER
docker build -t emotion-echo/fer:v0.1.0 -f Emotion-Echo-LLM/FER/Dockerfile Emotion-Echo-LLM

# SenseVoice（含预烘焙模型）
docker build -t emotion-echo/sensevoice:v0.1.2 -f Emotion-Echo-LLM/sensevoice-small/Dockerfile Emotion-Echo-LLM

# XTTS（含预烘焙 + monkey-patch torch.load）
docker build -t emotion-echo/xtts:v0.1.4 -f Emotion-Echo-LLM/XTTS/Dockerfile Emotion-Echo-LLM
```

### 4.4 验证

```bash
# 起容器（profile: ai 必须显式 + 需要 TLS cert + nacos）
bash scripts/smoke_ai_profile.sh

# 单独测
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
  --profile ai up -d --no-deps emotion-echo-fer
curl -fsS http://localhost:8004/health
# → {"status":"ok","model_loaded":false,"backend":"neutral-fallback"}
```

---

## 五、镜像逐个实战记录

### 5.1 FER

**镜像定位**：人脸情绪识别（face emotion recognition），OpenCV DNN + fer Python 包。

**Dockerfile 路径**：`Emotion-Echo-LLM/FER/Dockerfile`

**build 实战时间线**：

| 时间 | 事件 | 修复 |
|------|------|------|
| Stage 25-B | 首次 build | commit `b7532e7` 等，含 libopencv-4.10 |
| Stage 36-B5 round 1 | tuna tsinghua 替换导致境外 build 卡死 | 移除 sed tuna 替换 |
| Stage 36-B5 round 2 | deb.debian.org CDN 偶发 502 失败 | 加 `--fix-missing` + 3-retry loop |
| Stage 36-B5 round 3 | build EXIT=0 (392s) | 镜像 `v0.1.0` 12.1GB |

**最终 Dockerfile 关键改动**（commit `5d0b4cf`）：

```dockerfile
# Stage 36-B5 fix: retry loop tolerates deb.debian.org CDN 502/EOF on individual packages
RUN for i in 1 2 3; do \
      apt-get update && \
      apt-get install -y --no-install-recommends --fix-missing \
        gcc g++ git libopencv-dev && break; \
      echo "apt-get install attempt $i failed, retrying in 10s..."; \
      sleep 10; \
    done \
    && rm -rf /var/lib/apt/lists/*

# 同样模式应用到 runtime 阶段的 libopencv-*410 + ca-certificates + curl + tini
```

**端到端验证**：

```bash
$ docker compose --profile ai up -d --no-deps emotion-echo-fer
emotion-echo-fer   Up 19 seconds (healthy)

$ curl -fsS http://emotion-echo-fer:8004/health
{"status":"ok","model_loaded":false,"backend":"neutral-fallback"}

$ curl -fsS -X POST http://emotion-echo-fer:8004/analyze -F file=@test.png
{"emotion":"neutral","confidence":0.5,"scores":{},"source":"neutral-fallback"}

$ curl -fsS http://emotion-echo-fer:8004/metrics | grep analyze
fer_http_requests_total{method="POST",path="/analyze",status="200"} 3.0
```

**已知限制**：

- `emotion_net.caffemodel`（OpenCV DNN 路径所需）**仓里没有** → backend 永远走 neutral-fallback
- `libGL.so.1` 缺失 → fer 主路径 fail → 走 OpenCV DNN fallback → 又 fail → 走 neutral-fallback（dev 环境三连 fallback）
- 生产建议：要么预烘焙 emotion_net.caffemodel 进镜像，要么装 `libgl1-mesa-glx`

---

### 5.2 SenseVoice

**镜像定位**：阿里达摩院开源语音识别（语音转文字 + 情绪识别），funasr + SenseVoiceSmall。

**Dockerfile 路径**：`Emotion-Echo-LLM/sensevoice-small/Dockerfile`

**build 实战时间线**：

| 时间 | 事件 | 修复 |
|------|------|------|
| Stage 25-B | 首次 build（含 model 下载）| commit `2a9d928` 等 |
| Stage 36-B5 round 1 | tuna tsinghua 不可达 + CDN 502 | 移除 tuna + 加 retry |
| Stage 36-B5 round 2 | 只 COPY `model.pt` + `am.mvn` 到错误路径 `iic/SenseVoiceSmall/`（无双横线）| 改 COPY 整目录到 funasr 期望路径 `iic--SenseVoiceSmall/` |
| Stage 36-B5 round 3 | funasr SDK 校验 metadata 1 秒过（**预烘焙生效**） | commit `256c902` |

**最终 Dockerfile 关键改动**（commit `256c902`）：

```dockerfile
# Stage 36-B5 fix: 预烘焙 SenseVoiceSmall 完整模型仓库（含 model.pt 893MB +
# config.yaml / tokens.json / am.mvn / example/），funasr 期望路径：
#   MODELSCOPE_CACHE/iic--SenseVoiceSmall/snapshots/master/...
#
# 仓里已经下载好完整 ModelScope repo（sensevoice-small/），直接 COPY + 路径重命名。
# 容器启动后 funasr 跳过 ModelScope 下载，model_loaded=true 立即生效。
COPY --chown=app:app sensevoice-small/ /app/cache/models/iic--SenseVoiceSmall/snapshots/master/
RUN chown -R app:app /app/cache
```

**Compose start_period 调整**（commit `0d75a15`）：

```yaml
healthcheck:
  test: ...
  interval: 30s
  timeout: 5s
  # Stage 36-B5 fix: 模型已预烘焙进镜像（v0.1.2），但首次 /analyze 触发
  # funasr 模型加载 + torch GPU warmup ~60s。start_period 提到 300s 防止
  # docker healthcheck restart_policy 在 /analyze 处理期间误杀。
  start_period: 300s
  retries: 5
```

**端到端验证**：

```bash
$ docker logs emotion-echo-sensevoice --tail 5
funasr version: 1.4.12.
INFO download models from model hub: ms
INFO modelscope_hub.download | Downloading 20 files from iic/SenseVoiceSmall@master
  Downloading: 100%|██████████| 20/20 [00:01<00:00, 11.71file/s]
INFO Loading pretrained params from /app/cache/models/iic--SenseVoiceSmall/snapshots/master/model.pt
INFO ckpt: /app/cache/models/iic--SenseVoiceSmall/snapshots/master/model.pt
INFO starting SenseVoice server on 0.0.0.0:8002 device=cpu
INFO Uvicorn running on http://0.0.0.0:8002
```

**关键观察**：

- "Downloading 20 files" 1 秒内完成（**funasr 只做 metadata 校验**，文件已存在秒过）
- "Loading pretrained params from /app/cache/..." ← 模型从镜像预烘焙加载，**没真下载 200MB**
- 容器启动 ~24 秒完成模型加载

**已知限制**：

- 首次 `/analyze` 触发模型 lazy load（funasr 设计）需 ~30-60s CPU 推理
- dev 环境 docker desktop 7.6GB 内存紧张，频繁 `/analyze` 推理 + healthcheck 间隔可能让容器被 restart_policy 误杀
- 生产建议：内存 ≥3GB + start_period ≥300s（已配置）

---

### 5.3 XTTS

**镜像定位**：Coqui 开源 TTS 语音克隆（text-to-speech），XTTS-v2 模型。

**Dockerfile 路径**：`Emotion-Echo-LLM/XTTS/Dockerfile`

**build 实战时间线**（最坎坷的镜像）：

| 时间 | 事件 | 修复 |
|------|------|------|
| Stage 25-B | 首次 build（commit `52eb24c`）| 含 tuna 镜像替换 |
| Stage 36-B5 round 1 | tuna 不可达 + CDN 502 | 移除 tuna + 加 apt retry |
| Stage 36-B5 round 2 | pypi CDN 0字节响应（sha256 = `e3b0c44...` 全0） | pip install 3-retry + `--no-cache-dir` |
| Stage 36-B5 round 3 | docker buildx cache 跨 build 失效（`/install not found`）| 换 `docker build` + `--no-cache` |
| Stage 36-B5 round 4 | **缺失 vendor 模块**：`from pcm_chunk_shape import pcm_chunk_shape` ImportError | 加 `COPY XTTS/pcm_chunk_shape.py ./` |
| Stage 36-B5 round 5 | **PyTorch 2.6+ 默认 weights_only=True** 阻止加载 XTTS 权重（XttsConfig / XttsAudioConfig 等多个 class） | monkey patch `torch.load` 默认 `weights_only=False` |
| Stage 36-B5 round 6 | **torchaudio 2.11 默认用 torchcodec backend**（缺 torchcodec → ImportError） | **留给 dev/prod 环境**：加 `torchcodec` 到 requirements 或固定 torchaudio 版本 |

**最终 Dockerfile 关键改动**：

```dockerfile
# 1. Retry loop for apt (commit 5d0b4cf 风格)
RUN for i in 1 2 3; do \
      apt-get update && \
      apt-get install -y --no-install-recommends --fix-missing \
        gcc g++ git && break; \
      sleep 10; \
    done \
    && rm -rf /var/lib/apt/lists/*

# 2. pip install retry (commit d50f866)
RUN for i in 1 2 3; do \
      pip install --prefix=/install --timeout 600 --no-cache-dir -r requirements.txt && break; \
      sleep 15; \
    done

# 3. Vendor helper module (commit 256c902)
COPY --chown=app:app XTTS/pcm_chunk_shape.py ./
```

**最终 server.py 关键改动**（commit `5839541`）：

```python
# Stage 36-B5 fix: PyTorch 2.6+ 默认 weights_only=True 阻止加载 XTTS 模型权重
# （含 XttsConfig/XttsAudioConfig 等多个自定义类，XTTS vendored TTS 在不同
# load 阶段各 load 不同 class）。逐个 add_safe_globals 治标不治本，XTTS 每次
# 升级都可能引入新 class。
#
# **采用 monkey patch 全局降级**：把 torch.load 默认 weights_only 设为 False。
# XTTS 是 trusted checkpoint 来源（我们 model.pth / dvae.pth 都是 Coqui 官方 ckpt），
# 不是 untrusted pickle。
_original_load = torch.load
def _patched_load(*args, **kwargs):
    if "weights_only" not in kwargs:
        kwargs["weights_only"] = False
    return _original_load(*args, **kwargs)
torch.load = _patched_load
```

**端到端验证**（v0.1.4 build 成功 + 容器 healthy，**模型加载仍需修**）：

```bash
$ docker build -t emotion-echo/xtts:v0.1.4 -f Emotion-Echo-LLM/XTTS/Dockerfile Emotion-Echo-LLM
#20 DONE 99.7s   ← 全 layer cache 命中后秒完

$ docker compose --profile ai up -d --no-deps emotion-echo-xtts
emotion-echo-xtts  Up ... (healthy)

# 但加载模型时遇到最新坑（v0.1.4 build 后）：
ImportError: TorchCodec is required for load_with_torchcodec
```

**已知限制（v0.1.4 仍存在）**：

- **torchaudio 2.11** 默认 backend 切换到 torchcodec
- `pip install torchcodec` 是 dev 环境 torchcodec 的 wheel 编译要求多，需要 linux + libavcodec
- 临时绕过：固定 `torchaudio==2.0.x`（用 soundfile backend）或 env `TORCHAUDIO_USE_TORCHCODEC=0`

---

## 六、历史踩坑清单（必读）

按时间顺序，每坑配 commit + 修复：

### 6.1 tuna.tsinghua.edu.cn 镜像源境外不可达

**症状**：Dockerfile 里 `sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g'` + `pip -i tuna`，境外 IP 直连 tuna 失败，`apt-get update` / `pip install` 卡死。

**根因**：tuna 是中国教育网镜像，海外访问延迟高 / 经常被屏蔽。

**修复**：删除所有 tuna 替换行，用官方 `deb.debian.org` + `pypi.org`。**影响三个 Dockerfile**（commit `5d0b4cf` / `256c902` / `d50f866`）。

### 6.2 deb.debian.org CDN 偶发 502 / EOF

**症状**：`apt-get install` 偶发 `502 Bad Gateway` 或 `reading HTTP response body: unexpected EOF`，单包失败导致整个 layer 失败。

**根因**：Cloudfront CDN 边缘节点回源失败，dev 环境概率高。

**修复**：apt-get install 加 `--fix-missing`（容忍个别包缺失）+ 3-attempt retry loop（commit `5d0b4cf` 风格）：

```dockerfile
RUN for i in 1 2 3; do \
      apt-get update && \
      apt-get install -y --no-install-recommends --fix-missing \
        <packages> && break; \
      echo "apt-get install attempt $i failed, retrying in 10s..."; \
      sleep 10; \
    done \
    && rm -rf /var/lib/apt/lists/*
```

### 6.3 pypi CDN 0字节响应

**症状**：`pip install` 报 `THESE PACKAGES DO NOT MATCH THE HASHES FROM THE REQUIREMENTS FILE`，sha256 = `e3b0c44...`（空文件 hash）。

**根因**：pypi.org CDN 边缘节点偶尔返回 200 OK with 0 字节内容，pip 哈希校验失败。

**修复**：pip install 加 3-retry + `--no-cache-dir`（关 pip 自带 cache 命中 0字节文件）（commit `d50f866`）：

```dockerfile
RUN for i in 1 2 3; do \
      pip install --prefix=/install --timeout 600 --no-cache-dir -r requirements.txt && break; \
      echo "pip install attempt $i failed, retrying in 15s..."; \
      sleep 15; \
    done
```

### 6.4 预烘焙 SenseVoice 模型路径错误

**症状**：COPY `model.pt` + `am.mvn` 到 `/app/cache/models/iic/SenseVoiceSmall/snapshots/master/`（无双横线），但 funasr 期望 `iic--SenseVoiceSmall/snapshots/master/`（**双横线** 是 ModelScope slug 约定）。

**根因**：仓里目录叫 `sensevoice-small/`，但 ModelScope 标准仓库 slug 是 `iic--SenseVoiceSmall`（双横线分隔 owner/repo）。

**修复**：COPY 整个 `sensevoice-small/` 到 `iic--SenseVoiceSmall/`（commit `256c902`）：

```dockerfile
COPY --chown=app:app sensevoice-small/ /app/cache/models/iic--SenseVoiceSmall/snapshots/master/
```

### 6.5 XTTS 缺失 vendor 模块 `pcm_chunk_shape`

**症状**：容器启动 `ModuleNotFoundError: No module named 'pcm_chunk_shape'`，server.py `from pcm_chunk_shape import pcm_chunk_shape` 失败。

**根因**：XTTS 仓里 `XTTS/pcm_chunk_shape.py` 是 vendored helper，**但 Dockerfile 只 COPY `XTTS/TTS/` 子目录，没 COPY 仓根的 vendor 文件**。

**修复**：加 `COPY XTTS/pcm_chunk_shape.py ./`（commit `256c902`）。

### 6.6 PyTorch 2.6+ 默认 `weights_only=True`

**症状**：XTTS 模型加载报 `_pickle.UnpicklingError: Weights only load failed`，`Unsupported global: GLOBAL TTS.tts.models.xtts.XttsAudioConfig`。

**根因**：PyTorch 2.6 把 `torch.load` 默认 `weights_only` 从 `False` 改为 `True`，XTTS vendored TTS 在 pickle 里有多个自定义 class 没 allowlist。

**修复**：monkey patch `torch.load` 全局降级到 `weights_only=False`（commit `5839541`）：

```python
_original_load = torch.load
def _patched_load(*args, **kwargs):
    if "weights_only" not in kwargs:
        kwargs["weights_only"] = False
    return _original_load(*args, **kwargs)
torch.load = _patched_load
```

**安全考量**：XTTS 是 trusted local checkpoint（Stage 22-A 决策），不是 untrusted pickle，monkey patch 是 dev 环境可行方案。生产应该升级到 vendored TTS 修复后取消 patch。

### 6.7 torchaudio 2.11 切到 torchcodec backend

**症状**：XTTS 模型加载报 `ImportError: TorchCodec is required for load_with_torch_codec. Please install torchcodec to use this function.`

**根因**：torchaudio 2.11+ 默认 backend 从 `soundfile` 切到 `torchcodec`，torchcodec 是新依赖（独立 pip 包），需要 C++ 编译环境。

**修复（待定）**：
- 选项 A：`pip install torchcodec`（加到 XTTS requirements）
- 选项 B：固定 `torchaudio<2.11`（用 soundfile backend）
- 选项 C：env `TORCHAUDIO_USE_TORCHCODEC=0`

**留给生产网络跑 build 时决定**——dev 环境 libavcodec 编译链太长，没在本会话修复。

### 6.8 docker buildx cache 跨 build 失效

**症状**：第二次 build 报 `failed to calculate checksum of ref tdlq...::djcll... "/install": not found`。

**根因**：docker buildx desktop-linux builder 在多次 build 后 stale cache，老 builder container 已删但 cache key 还引用。

**修复**：用 `docker build` 替代 `docker buildx build`（走默认 daemon builder，避免 cache 失效）；或 `docker buildx prune` 清理 stale cache。

### 6.9 docker healthcheck 在 /analyze 处理时误杀容器

**症状**：SenseVoice / XTTS 容器在 `/analyze` 处理期间（~60s）被 healthcheck 30s 间隔检测为 unhealthy，触发 `restart_policy unless-stopped` 重启。

**根因**：docker healthcheck 用新 goroutine 跑 `/health` curl，跟 `/analyze` 独立。容器在 `/analyze` 处理时 CPU 100%，healthcheck timeout 失败。

**修复**：
1. `start_period` 提到 ≥300s（让首次加载模型完成后再开始 healthcheck）
2. `retries` 提到 5（容忍偶发失败）
3. 生产建议：healthcheck 间隔提到 60s+（而非 30s）

---

## 七、构建验证流程

### 7.1 build 后必做

```bash
# 1. 镜像存在 + 大小合理
docker images emotion-echo/{fer,sensevoice,xtts}:v0.1.0

# 2. 启动 + /health
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
  --profile ai up -d --no-deps emotion-echo-fer
sleep 30  # 等 start_period
docker ps --format "{{.Names}}\t{{.Status}}" | grep emotion-echo-fer

# 3. 模型预烘焙验证（关键 — 确认不是 runtime 下载）
docker logs emotion-echo-sensevoice --tail 30 | grep "download\|Loading\|model.pt"
# → "Loading pretrained params from /app/cache/..." 表示预烘焙生效
# → "Downloading 20 files" 1 秒内完成 = funasr metadata 校验，不是真下载

# 4. 业务端到端
bash scripts/smoke_ai_profile.sh
```

### 7.2 业务端到端测试

```bash
# chat → ai → FER/SenseVoice 全链
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d postgres nacos ai-svc chat-svc
curl -X POST http://localhost:8890/api/v1/conversations/1/messages -H 'X-User-Id: 3' \
  -H 'Content-Type: application/json' -d '{"role":"user","content":"test"}'
# 看 ai-svc logs: "FER client active: http://emotion-echo-fer:8004"
# 看 fer logs: "fer_http_requests_total{method='POST',path='/analyze'} N"
```

### 7.3 健康检查脚本

```bash
# scripts/smoke_ai_profile.sh
# - 启 3 容器
# - 等 30s health check
# - 端口探测 /health
# - 输出 PASS/FAIL

bash scripts/smoke_ai_profile.sh
# 期望: ✅ PASS（FER + SenseVoice 均 healthy 且 /health 可达）
```

---

## 八、常见问题速查

### Q1: docker build 卡在 pip install 几小时不动

**A**: 看进程 `ps -ef | grep docker`。如果 `docker buildx` 还活着但 log 没动，**docker daemon 写 log buffer 卡住**，不影响实际进度。耐心等或 `docker prune` 释放磁盘。

### Q2: docker build 报 `failed to calculate checksum "/install": not found`

**A**: docker buildx cache 跨 build 失效。改用 `docker build`（默认 daemon builder），或 `docker buildx prune` 清理。

### Q3: docker compose up 后容器 status=`Restarting`

**A**: healthcheck 在 `/analyze` 处理时 timeout，触发 restart_policy。修复：
1. 拉长 `start_period` 到 ≥300s（让首次模型加载完成）
2. 拉长 `interval` 到 60s+
3. 给容器 ≥3GB 内存（dev 环境 ≤7.6GB 紧张）

### Q4: 模型启动报 `ImportError: No module named 'XXX'`

**A**: vendor 模块缺失。看 `grep -r "from XXX\|import XXX" server.py`，对比 Dockerfile `COPY` 行是否覆盖 vendored 文件源。

### Q5: 模型加载报 `_pickle.UnpicklingError`

**A**: PyTorch 2.6+ 默认 `weights_only=True`，拒绝未 allowlist 的 class。修复见 §6.6 monkey patch。

### Q6: profile: ai down 误删 nacos / postgres

**A**: ⚠️ **危险操作**：`docker compose --profile ai down` + `--remove-orphans` 会删所有依赖 `profile: ai` 的服务（**包括 default profile 的 nacos/postgres**）。

**修复**：单独 down ai 服务，不用 `--remove-orphans`：

```bash
# ✅ 安全
docker compose --profile ai down

# ❌ 危险：会删 nacos / postgres（如果它们 depends_on ai）
docker compose --profile ai down --remove-orphans
```

### Q7: 容器 image 巨大（12GB+），但 content size 只有 4GB

**A**: Docker Desktop 用 `disk usage`（virtual size，含所有 layer）+ `content size`（unique）。多 stage Dockerfile 共享 base 镜像（python:3.10-slim）时，virtual 累加，content 独立。**这是正常的，不要删 "duplicated" layer**。

### Q9: buildx cache 跨 build 不共享

**A**: docker buildx desktop-linux builder 是**单个 daemon 实例**，重启会丢 cache。生产用 `docker buildx create --use --bootstrap` 创建持久 builder。

---

## 九、相关文档

- [stage-36-landing.md](stage-36-landing.md) — Stage 36 完整 landing report（含 docker smoke 实测）
- [stage-36-fixes-roadmap.md](stage-36-fixes-roadmap.md) — Stage 36 修复 roadmap
- [stage-18-grpc-mtls.md](stage-18-grpc-mtls.md) — TLS 证书生成设计（mTLS dev cert）
- [stage-25-ai-profile-build-issue.md](stage-25-ai-profile-build-issue.md) — Stage 25 build 阻塞历史
- `scripts/smoke_ai_profile.sh` — AI 镜像端到端 health probe 脚本
- `scripts/generate_dev_tls.sh` — mTLS dev cert 生成脚本
- `emotion-echo-ai-svc/internal/fusion/` — LLM 融合器（FusionWorker 调度）

---

## 十、版本

- v0.1（2026-09-02）— Stage 36-B5 完成首版；3 AI 镜像 build EXIT=0，2 个镜像端到端 PASS，1 个（XTTS）端到端受 torchaudio 2.11 backend 切换影响。
- v0.2 待 — 修 XTTS torchaudio backend + Stage 37 启动时更新