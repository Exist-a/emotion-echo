# emotion-echo-models · AI 服务实现目录

> **本文档是 3 个 AI 容器（FER / SenseVoice / XTTS）的**实现 +构建 +运行 +排错**主指南**。
> 其他文档（`docs/ai-models/build-guide.md`、`docs/ai-models/vendor-candidates.md`、`docs/ai-models/xtts-decision.md`）都从这里跳转。
> 上一次重大更新：2026-10-17（XTTS 章节按 ADR-2026-09-xtts-v3 校正为"仓内镜像 + 构建期蒸煮模型"；模型位置统一到 D 盘）。

## 一、3 个 AI 容器总览

| 容器 | 职责 | 入口路径 | 实现位置 | 镜像 tag |
|---|---|---|---|---|
| **emotion-echo-fer** | 人脸情绪识别（tflite + Haar cascade） | `POST /analyze` | `emotion-echo-models/FER-tflite/` | `emotion-echo/fer-tflite:v0.1.0` |
| **emotion-echo-sensevoice** | 语音转写 + 7 类情绪识别（funasr + SenseVoiceSmall） | `POST /analyze` | `emotion-echo-models/SV-fastbuild/` | `emotion-echo/sensevoice-fastbuild:v0.1.0` |
| **emotion-echo-xtts** | 语音克隆 TTS（仓内 Coqui TTS server） | `GET /health` · `POST /tts` · `/tts_stream` · `/tts_with_phonemes` | `emotion-echo-models/XTTS/` | `emotion-echo/xtts:v2.0.1` |

**全部位于 `deploy/docker-compose.apps.yml` 的 `profiles: ["ai"]` 之下**，dev 默认不起，需要时显式启用：

```bash
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml --profile ai up -d emotion-echo-fer emotion-echo-sensevoice emotion-echo-xtts
```

ai-svc 通过 `applyEnvOverrides` 自动注入 `FER_BASE_URL` / `SENSEVOICE_BASE_URL` / `XTTS_BASE_URL` 三个环境变量（默认指向容器 DNS 名）——启动后无需任何额外配置。

---

## 二、FER（人脸情绪识别 · tflite 后端）

### 2.1 设计

| 项 | 值 |
|---|---|
| 后端 | tflite-runtime 2.14 + opencv-contrib-python-headless 4.11（**无 tensorflow 依赖**） |
| 模型 | `emotion_model_quantized.tflite`（92KB，fer包内置量化版） |
| 人脸检测 | OpenCV Haar cascade（`cv2.data.haarcascades + haarcascade_frontalface_default.xml`） |
| 输入 | JPEG/PNG 任意尺寸 → 自动检测人脸 → 64×64 灰度送入 tflite |
| 输出 | `{emotion, confidence, scores, source}`，emotion ∈ {angry, anxious, happy, sad, neutral}（与 emotion-llm-service 对齐的 5 类统一情感） |

### 2.2 镜像大小对比

| 镜像 | 大小 |
|---|---|
| **fer-tflite（本方案）** | **538MB disk / 134MB content** |
| 原 emotion-echo/fer（fer + tensorflow）| 12.1GB（**22.5× 体积差**） |

### 2.3 构建

```bash
cd emotion-echo-models/FER-tflite
docker build -t emotion-echo/fer-tflite:v0.1.0 -f Dockerfile .
```

**耗时**：首次 ~30-60 秒（含 1.5MB wheel 下载），有缓存 ~5 秒。

### 2.4 运行验证

```bash
docker run -d --name fer-test -p 8004:8004 emotion-echo/fer-tflite:v0.1.0
sleep 3
curl -fsS http://localhost:8004/health
# {"status":"ok","model_loaded":true,"backend":"tflite+haar"}

# /analyze 空文件：应返回 400
curl -i -X POST http://localhost:8004/analyze -F "file=@/dev/null"
# 400 {"detail":"empty file"}
```

### 2.5 单测

49/49 测试（5 个文件）：`emotion_mapping` / `health_route` / `analyze_route` / `logging_setup` / `metrics_setup`。

```bash
cd emotion-echo-models/FER-tflite
docker run --rm -v "$PWD:/app" -w /app python:3.10-slim sh -c "
pip install -r requirements.txt pytest httpx2
python -m pytest tests/unit/ -v
```

### 2.6 已知限制

- **合成图（无真实人脸）→ source="no-face"**：Haar cascade 对真实人脸敏感，对几何形状不接受。**真实人脸图已验证命中 tflite 推理**（2026-09-10，600x574 真人愤怒脸图，emotion=angry, confidence=0.87），详见 §7.1。
- **emotion 7 类原始标签 → 5 类 unified 映射**：`disgust`/`surprise` 映成 `neutral`，`fear` 映成 `anxious`——保持与 emotion-llm-service 对齐。

---

## 三、SenseVoice（语音转写 + 情绪识别）

### 3.1 设计

| 项 | 值 |
|---|---|
| 后端 | funasr 1.4.15 + torch 1.13.1+cpu + kaldi-native-fbank 1.22 |
| 模型 | SenseVoiceSmall（funasr 仓库，iic/SenseVoiceSmall），主模型 936MB 烘焙进镜像，VAD 模型首次启动从 ModelScope 自动下载到 `/app/cache` |
| 输入 | 任意音频（mp3/wav/m4a 等）→ fbank 特征提取 → ASR + 情绪识别 |
| 输出 | `{text, emotion, confidence, raw_text, source}`，emotion ∈ {angry, anxious, happy, sad, neutral} |

### 3.2 镜像大小

| 镜像 | disk | content |
|---|---|---|
| emotion-echo/sensevoice-fastbuild:v0.1.0 | 5.88GB | 2.24GB（其中 936MB 模型 + 1GB torch/funasr/transformers 等） |

### 3.3 构建

**前置条件**：仓内 `emotion-echo-models/SV-fastbuild/wheels/` 必须有 6 个预下 wheel 文件（torch+cpu、torchaudio、funasr、modelscope、huggingface_hub、gradio）。**首次构建需要预下**：

```bash
docker run --rm -v "$(pwd)/emotion-echo-models/SV-fastbuild/wheels:/wheels" python:3.10-slim sh -c "
pip install --no-cache-dir --index-url https://mirrors.aliyun.com/pypi/simple 'pip>=24'
pip download --no-deps --dest /wheels/ \
  --index-url https://pypi.tuna.tsinghua.edu.cn/simple \
  --extra-index-url https://mirrors.aliyun.com/pypi/simple \
  'torch==1.13.1+cpu' 'torchaudio==0.12.1+cpu' \
  'funasr>=1.1.2' 'modelscope' 'huggingface_hub' 'gradio' 2>&1 | tail -5
ls /wheels/ | wc -l   # 应该是 6
"
```

**为什么这样**：dev 网络下 `pypi.org` 不通，`download.pytorch.org` 大包断链（stage-58/59 记录）。清华镜像 + aliyun fallback 是最稳的组合。torch+cpu wheel **必须**用清华镜像（aliyun pypi 没有 PyTorch 专有 wheel）。

然后构建：

```bash
docker build -t emotion-echo/sensevoice-fastbuild:v0.1.0 \
  -f emotion-echo-models/SV-fastbuild/Dockerfile \
  emotion-echo-models/SV-fastbuild
```

**耗时**：首次 ~140 秒（无 cache），有 wheels cache ~47 秒。

### 3.4 运行验证

```bash
docker run -d --name sv-test -p 8002:8002 emotion-echo/sensevoice-fastbuild:v0.1.0
sleep 10   # 等模型加载
curl -fsS http://localhost:8002/health
# {"status":"loading","service":"sensevoice","device":"cpu","model_loaded":false}
# 注意 model_loaded=false 是设计：funasr 懒加载，首次 /analyze 触发 VAD+ASR 模型加载

# 真实推理（仓内 example/zh.mp3）
docker run --rm --network container:sv-test \
  -v "$PWD/emotion-echo-models/sensevoice-small/example:/ex:ro" \
  curlimages/curl:latest \
  -sS -m 120 -X POST http://localhost:8002/analyze \
  -F "file=@/ex/zh.mp3;type=audio/mpeg"
# {"text":"开放时间早上9点至下午5点。","emotion":"neutral","confidence":0.6,...}
```

**首次 /analyze 等待 30-60 秒**（funasr 加载 + torch warmup）。

### 3.5 单测

未写单测——SV 是基于 funasr 的薄壳服务，业务逻辑在 funasr 内部。建议未来补：
- `test_emotion_mapping.py`（与 FER-tflite 同结构）
- `test_analyze_route.py`（mock funasr.AutoModel.generate）

### 3.6 常见故障

| 现象 | 根因 | 解决 |
|---|---|---|
| `RuntimeError: Form data requires "python-multipart"` | requirements 漏依赖 | 加 `python-multipart>=0.0.9` |
| `ImportError: torchaudio is not installed and neither is the kaldi-native-fbank fallback` | funasr 1.4 特征提取需要 fbank 后端 | 装 `kaldi-native-fbank>=1.18`（替代 torchaudio，~1MB vs ~50MB） |
| `[transformers] Disabling PyTorch because PyTorch >= 2.5 is required` | transformers 5.x 要 torch≥2.5 | pin `transformers<5` |
| `error from registry: unknown manifest class for application/vnd.oci.empty.v1+json`（**已规避**）| buildx 默认输出 OCI manifest，ACR 个人版不认 | （已规避：SV 走 docker.io 直接 pull）|
| `KeyError: "Attempt to overwrite 'filename' in LogRecord"`（**已修**）| `extra={"filename": ...}` 与 LogRecord 内置字段冲突 | 改为 `extra={"audio_file": ...}` |

---

## 四、XTTS（语音克隆 TTS · 仓内镜像）

### 4.1 设计

| 项 | 值 |
|---|---|
| 后端 | 仓内 `emotion-echo-models/XTTS/server.py`（FastAPI + Coqui-TTS `Xtts`）。vendor `ai4all/coqui` 已弃用——它**没有** `/tts_stream` 端点（见 ADR-2026-09-xtts-v3） |
| XTTS v2 模型 | **构建期蒸煮进镜像**：`Dockerfile` 里 `COPY XTTS/AI-ModelScope/ ./AI-ModelScope/`；运行时 `server.py` 从 `/app/AI-ModelScope/XTTS-v2`（`CACHE_DIR`）加载 —— **不是**运行时挂载 |
| 模型源码位置 | 仓库 `emotion-echo-models/XTTS/AI-ModelScope/XTTS-v2/`（**在 D 盘**；`model.pth` 1.86GB + `dvae.pth` 201MB，已被 `.gitignore` 忽略） |
| 构建上下文 | `emotion-echo-models/`（`docker build -f emotion-echo-models/XTTS/Dockerfile -t emotion-echo/xtts:v2.0.1 emotion-echo-models/`） |
| API | `GET /health` · `POST /tts`（非流式）· `POST /tts_stream`（流式 WAV）· `POST /tts_with_phonemes`（带字符级时间戳，数字人口型用） |

### 4.2 镜像

**仓内镜像，非 vendor**。`emotion-echo-models/XTTS/Dockerfile` 双阶段构建（builder 装 torch / Coqui-TTS / modelscope，runtime 只带 site-packages + vendored TTS 源码 + 模型）。首次 cold build 约 10 分钟，镜像约 11GB。

```bash
docker build -f emotion-echo-models/XTTS/Dockerfile -t emotion-echo/xtts:v2.0.1 emotion-echo-models/
```

### 4.3 模型从哪来（关键 · **不要落到 C 盘**）

XTTS v2 模型 **约 2GB**（`model.pth` 1.86GB + `dvae.pth` 201MB + `mel_stats.pth` + 配置）由 **构建期** 从构建上下文 `emotion-echo-models/XTTS/AI-ModelScope/XTTS-v2/` 蒸煮进镜像；容器启动时**不再下载**。

> 🔴 **模型必须落在 D 盘**（仓库所在盘）。2026-10-17 排查发现宿主 `~/.cache/modelscope/`（C 盘）留了 **2.9GB 的 XTTS-v2 + SenseVoiceSmall 重复副本** —— 仓库里已有同尺寸、同 `hash.md5` 的正本，且**项目任何代码 / compose / 脚本都不读**该宿主缓存，故已删除。**不要在 C 盘（系统盘）留模型缓存。**
>
> 需要让宿主级工具把缓存写到 D 盘时，用环境变量（两个都已被本仓 Dockerfile 使用，名字可对照 `emotion-echo-models/XTTS/Dockerfile` 与 `sensevoice-small/Dockerfile`）：
>
> ```bash
> export MODELSCOPE_CACHE=/d/DockerData/modelscope-cache   # ModelScope
> export HF_HOME=/d/DockerData/hf-cache                    # HuggingFace
> ```
>
> Windows 可用 `setx MODELSCOPE_CACHE "D:\DockerData\modelscope-cache"` 持久化。

把模型补/重下到构建上下文（**历史实测做法**，逐文件下载绕开长连接断链；文件清单与当前目录一致）：

```bash
cd emotion-echo-models/XTTS/AI-ModelScope/XTTS-v2
for f in config.json vocab.json hash.md5 LICENSE.txt README.md mel_stats.pth dvae.pth model.pth; do
  curl -L -C - --retry 20 -s -o "$f" \
    "https://hf-mirror.com/coqui/XTTS-v2/resolve/main/$f"
done
mkdir -p samples
curl -L -C - --retry 20 -s -o "samples/en_sample.wav" \
  "https://hf-mirror.com/coqui/XTTS-v2/resolve/main/samples/en_sample.wav"
```

### 4.4 启动

通过 compose 一次性起：

```bash
# 1. 创建外部网络（如未存在）
docker network create emotion-echo_app-network

# 2. 启动 XTTS
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
  --profile ai up -d --no-deps emotion-echo-xtts

# 3. 等模型加载 + warmup（约 2-3 分钟，看 logs）
docker logs -f emotion-echo-xtts   # 等到 "XTTS model loaded"

# 4. 健康检查（模型已烘焙进镜像 /app/AI-ModelScope/XTTS-v2，无需挂载 /model）
docker exec emotion-echo-xtts python -c "
import urllib.request
print(urllib.request.urlopen('http://localhost:8003/health', timeout=5).read().decode())
"
# 预期：{"status":"ok","model_loaded":true,"model_type":"XTTS-v2"}

# 5. 合成：带字符级时间戳（宿主直连需临时映射 8003；经 BFF→XTTS 无需映射）
curl -sS -m 120 -X POST http://localhost:8003/tts_with_phonemes \
  -H 'Content-Type: application/json' \
  -d '{"text":"你好世界","language":"zh-cn"}'
# 预期：200 + audio(base64) / sample_rate 24000 / phonemes[{char,start,duration}]
```

### 4.5 已知限制

- **compose 的 `${XTTS_MODEL_PATH:-/d/DockerData/coqui_model}:/model` 卷是 vendor 时代残留**：仓内 `server.py` 只读镜像内 `/app/AI-ModelScope/XTTS-v2`（`CACHE_DIR`），**从不读 `/model`** ⇒ 该挂载对当前实现无效（仍指向 D 盘、不占 C 盘空间，待清理；见 §4.3 与 ADR-2026-09-xtts-v3）。
- **模型挂载路径跨平台**：默认 `/d/DockerData/coqui_model` 是 Git Bash 风格路径，Mac/Linux 用户需 `export XTTS_MODEL_PATH=/Users/...`。注意该变量对新镜像**无实际作用**（见上一条）。
- **`/health` 由仓内 `server.py` 提供**（返 `model_loaded` / `model_type`）；compose 的 healthcheck 仍用 TCP socket probe 做启动探活。
- **推理并发与 CPU**：`XTTS_SYNTH_WORKERS`（默认 2）经 `synth_pool` 线程池卸载阻塞推理；`cpus: "8.0"` —— 早期 2 核限额是"TTS 很慢"的根因（E2E-F-132：40 字 188s → 19.9s）。

---

## 五、3 个 AI 容器一起跑

```bash
# 1. 启动基础设施（nacos + postgres + redis + kafka + minio + apisix 等 8 个）
docker compose -f deploy/docker-compose.infra.yml up -d

# 2. 启动 5 个业务服务（emotion-echo-*svc）
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d

# 3. 启动 3 个 AI profile 服务
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
  --profile ai up -d emotion-echo-fer emotion-echo-sensevoice emotion-echo-xtts

# 4. 验证
for svc in fer sensevoice xtts; do
  case $svc in
    fer) port=8004 ;;
    sensevoice) port=8002 ;;
    xtts) port=8003 ;;
  esac
  echo "--- $svc ---"
  docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
    --profile ai exec $svc sh -c "wget -q -O - http://localhost:$port/health 2>/dev/null || echo 'no /health'"
done

# 5. ai-svc 自动接通（applyEnvOverrides 注入 *_BASE_URL）
curl http://localhost:8891/api/v1/conversations/1/analyze \
  -H "X-User-Id: 1" \
  -F "image=@test_face.jpg"
```

---

## 六、未来改进方向

| 优先级 | 项 | 工作量 |
|---|---|---|
| 高 | SV 加单测（mock funasr.generate）| 半天 |
| 中 | SV 把 936MB 模型改卷挂载，镜像瘦身 ~1.3GB | 半天 |
| 中 | FER 拆 base 层（虽然已经够小，134MB 不必）| 0 |
| 低 | XTTS 替换为阿里云语音 API（ADR-001 原路径） | 1 天 + API key |
| 低 | SV 导出 ONNX，用 onnxruntime 跑（替代 torch 800MB）| 1 周 |

---

## 七、实测验证记录（2026-09-10）

这一节记录**今天在本机实测过的命令和结果**——下次重 build 或迁移机器时直接复制粘贴即可复现。

### 7.1 FER（本地镜像 `emotion-echo/fer-tflite:v0.1.0`）

```bash
# 启动（端口 8004）
docker run -d --name fer-hc -p 8004:8004 emotion-echo/fer-tflite:v0.1.0

# /health（应返回 model_loaded=true, backend=tflite+haar）
curl -fsS http://localhost:8004/health
# 实际结果：{"status":"ok","model_loaded":true,"backend":"tflite+haar"}

# /analyze 合成脸图（PowerShell 生成 → Windows temp → 挂进容器）
curl -sS -m 30 -X POST http://localhost:8004/analyze \
  -F "file=@/tmp/face.jpg;type=image/jpeg"
# 实际结果：{"emotion":"neutral","confidence":0.5,"scores":{},"source":"no-face"}
# 含义：合成几何形状 Haar 检不到人脸，走 no-face 分支

# /analyze 真实人脸图（600x574 PNG）
curl -sS -m 30 -X POST http://localhost:8004/analyze \
  -F "file=@/tmp/angry_face.png;type=image/png"
# 实际结果：{"emotion":"angry","confidence":0.8687,
#             "scores":{"angry":0.8687,"fear":0.1033,"sad":0.0132,...},
#             "source":"tflite"}
# 含义：tflite 推理路径命中真实人脸，输出 7 类概率分布，置信度 86.87%
```

**FER tflite 推理路径端到端验证通过**（2026-09-10，600x574 真人愤怒脸图，置信度 0.87）。49/49 单测 + 这次真实推理 = 完整覆盖。

### 7.2 SenseVoice（ACR 镜像 `crpi-.../emotion-echo/sensevoice:v0.1.0`）

```bash
# pull（首次约 1.4GB）
docker pull crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com/emotion-echo/sensevoice:v0.1.0

# 启动（端口 8002）
docker run -d --name sv-hc -p 8002:8002 \
  crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com/emotion-echo/sensevoice:v0.1.0
sleep 8

# /health（懒加载设计：model_loaded=false 是预期）
curl -fsS http://localhost:8002/health
# 实际结果：{"status":"loading","service":"sensevoice","device":"cpu","model_loaded":false}

# /analyze 真实音频（仓内 example/zh.mp3，首次 30-60s 加载模型）
curl -sS -m 120 -X POST http://localhost:8002/analyze \
  -F "file=@/ex/zh.mp3;type=audio/mpeg"
# 实际结果：{"text":"开放时间早上9点至下午5点。","emotion":"neutral","confidence":0.6,...}
```

### 7.3 XTTS（仓内 `emotion-echo/xtts:v2.0.1`，端口 8003）

```bash
# 模型已蒸煮进镜像（构建上下文 emotion-echo-models/XTTS/AI-ModelScope/，在 D 盘）
# 无需任何模型挂载；详见 §4.2 / §4.3

# 启动（compose 已指定 image 与 CMD，这里单跑等价形态）
docker run -d --name xtts-hc -p 8013:8003 \
  emotion-echo/xtts:v2.0.1

# 等模型加载（XTTS v2 torch warmup ≈ 2-3 分钟）
sleep 150
docker exec xtts-hc python -c "
import urllib.request
print(urllib.request.urlopen('http://localhost:8003/health', timeout=5).read().decode())
"
# 实际结果：{"status":"ok","model_loaded":true,"model_type":"XTTS-v2"}

# 合成端到端（带字符级时间戳）
curl -sS -m 120 -X POST http://localhost:8013/tts_with_phonemes \
  -H "Content-Type: application/json" \
  -d '{"text":"你好世界","language":"zh-cn"}'
# 实际结果：200 + audio(base64) / sample_rate 24000 / phonemes[{char,start,duration}] / duration
#   （per-char 等分近似：last.start + last.duration == duration）
```

### 7.4 完整 profile 启动 + 验证（一键脚本）

```bash
# 启动
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
  --profile ai up -d emotion-echo-fer emotion-echo-sensevoice emotion-echo-xtts

# 等待 startup（XTTS 最慢，约 2.5 分钟）
sleep 180

# 验证三容器健康
for svc in fer sensevoice xtts; do
  port=$([ "$svc" = "fer" ] && echo 8004 || ([ "$svc" = "sensevoice" ] && echo 8002 || echo 8003))
  echo "--- $svc :$port ---"
  docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
    --profile ai exec $svc sh -c "timeout 5 wget -q -O - http://localhost:$port/health 2>/dev/null || echo '(no /health or timeout)'"
done
# 实际结果：fer/sensevoice 返回 200 JSON，xtts 返回 '(no /health or timeout)' 是预期（vendor 无 /health 端点）
```

---

## 八、相关文档

- [`docs/ai-models/build-guide.md`](../docs/ai-models/build-guide.md) — 旧版构建指南（Stage 36），已 deprecated，本文档取代
- [`docs/ai-models/vendor-candidates.md`](../docs/ai-models/vendor-candidates.md) — 3 模型 vendor 候选调研记录（含 yiminger/sensevoice 挂错标签的发现）
- [`docs/ai-models/xtts-decision.md`](../docs/ai-models/xtts-decision.md) — XTTS 选型决策记录（云 API vs vendor）
- [`docs/ai-models/xtts-integration.md`](../docs/ai-models/xtts-integration.md) — XTTS 集成细节
- [`docs/stages/stage-59-vendor-image-verification.md`](../docs/stages/stage-59-vendor-image-verification.md) — 3 vendor 镜像实测记录（5h 跨 2026-09-09 下午）
- [`docs/stages/stage-60-pr-tts-vendor-landing.md`](../docs/stages/stage-60-pr-tts-vendor-landing.md) — PR-TTS-VENDOR 三模型落地文档
- [`docs/stages/stage-60-1-pr-tts-vendor-sv-landing.md`](../docs/stages/stage-60-1-pr-tts-vendor-sv-landing.md) — SV 真实推理验证（4/4 通过）
- [`docs/plans/todo-pile-2026-09-04.md §A1`](../docs/plans/todo-pile-2026-09-04.md) — TTS 缺口 closure 记录

### 内部参考（**非指导文档**，仅留作历史）

- `docs/operations/acr-push.md` — Aliyun ACR 推送尝试记录（**已不推荐使用**：5 次 push 都失败，ACR 个人版不兼容 vendor OCI manifest）。Fer/SenseVoice 的 docker push ACR 走通但 XTTS 不行，dev 镜像拉取路径走 docker.io 即可
- `scripts/push-to-acr.sh` — ACR 推送脚本（**同上**，FER/SV 路径可用但 XTTS 不支持；当前未启用）