# emotion-echo-models · AI 服务实现目录

> **本文档是 3 个 AI 容器（FER / SenseVoice / XTTS）的**实现 +构建 +运行 +排错**主指南**。
> 其他文档（`docs/ai-models/build-guide.md`、`docs/ai-models/vendor-candidates.md`、`docs/ai-models/xtts-decision.md`）都从这里跳转。
> 上一次重大更新：2026-09-10（PR-TTS-VENDOR Phase 5+6）。

## 一、3 个 AI 容器总览

| 容器 | 职责 | 入口路径 | 实现位置 | 镜像 tag |
|---|---|---|---|---|
| **emotion-echo-fer** | 人脸情绪识别（tflite + Haar cascade） | `POST /analyze` | `emotion-echo-models/FER-tflite/` | `emotion-echo/fer-tflite:v0.1.0` |
| **emotion-echo-sensevoice** | 语音转写 + 7 类情绪识别（funasr + SenseVoiceSmall） | `POST /analyze` | `emotion-echo-models/SV-fastbuild/` | `emotion-echo/sensevoice-fastbuild:v0.1.0` |
| **emotion-echo-xtts** | 语音克隆 TTS（vendor Coqui） | `POST /voice/{upload,generate,result}` | vendor `ai4all/coqui:latest` | `ai4all/coqui:latest` |

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

- **合成图（无真实人脸）→ source="no-face"**：Haar cascade 对真实人脸敏感，对几何形状不接受。生产传真实照片会命中。
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

## 四、XTTS（语音克隆 TTS · vendor ai4all/coqui）

### 4.1 设计

| 项 | 值 |
|---|---|
| 后端 | vendor `ai4all/coqui:latest`（Coqui TTS server） |
| XTTS v2 模型 | 本地预下（`C:\Users\LENVOV\AppData\Local\Temp\coqui_model`），volume 挂载到容器 `/model` |
| 启动命令覆盖 | `fastapi run app.py --host=0.0.0.0 --port=8003`（vendor 默认 8000，改为 8003 与 ai-svc XTTS_BASE_URL 对齐）|
| API | `POST /voice/upload`（上传参考音）+ `POST /voice/generate`（生成）+ `GET /voice/result`（拉取）|

### 4.2 镜像

**vendor 镜像，无需自建**。Docker Hub 拉 `ai4all/coqui:latest`，本地有 daemon.json 配 aliyun 加速镜像，下载通常 <5 分钟。

### 4.3 模型挂载（关键）

XTTS v2 模型 **2GB** 必须预下并挂载到 `/model`，否则容器启动后从 HuggingFace 下载会失败（dev 网络下 `ssl.SSLEOFError`）。

```bash
# 下载 XTTS v2 模型（9 个文件 ~2GB，stage-59 §十.3.1 验证过的路径）
mkdir -p /c/Users/LENVOV/AppData/Local/Temp/coqui_model
cd /c/Users/LENVOV/AppData/Local/Temp/coqui_model
# 单独下每个文件（分文件下载绕开长连接断链）
for f in config.json vocab.json hash.md5 LICENSE.txt README.md mel_stats.pth speakers_xtts.pth dvae.pth model.pth; do
  curl -L -C - --retry 20 -s -o "$f" \
    "https://hf-mirror.com/coqui/XTTS-v2/resolve/main/$f"
done

# samples 是 vendor 启动时校验用的英文样例
mkdir -p samples
for f in en_sample.wav __samples_; do
  curl -L -C - --retry 20 -s -o "samples/$f" \
    "https://hf-mirror.com/coqui/XTTS-v2/resolve/main/samples/$f"
done
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
docker logs -f emotion-echo-xtts   # 等到 "Uvicorn running on http://0.0.0.0:8003"

# 4. 测试 TTS（容器内）
docker run --rm --network emotion-echo_app-network \
  -v "/c/Users/LENVOV/AppData/Local/Temp/coqui_model/snapshots/master/samples:/samples:ro" \
  python:3.10-slim sh -c "
  pip install requests
  python -c \"
import requests
with open('/samples/en_sample.wav','rb') as f:
    r = requests.post('http://emotion-echo-xtts:8003/voice/upload',
                      files={'audio':('en_sample.wav',f,'audio/wav')},
                      data={'name':'demo'}, timeout=60)
print('upload:', r.status_code, r.text[:100])
r = requests.post('http://emotion-echo-xtts:8003/voice/generate',
                  json={'voice':'demo','text':'Hello from compose TTS.','language':'en'}, timeout=120)
print('generate:', r.status_code)
r = requests.get('http://emotion-echo-xtts:8003/voice/result', timeout=30)
print('result:', r.status_code, 'size:', len(r.content), 'header:', r.content[:16])
\"
"
# 预期：upload 200, generate 200, result 200 (~133KB WAV)
```

### 4.5 已知限制

- **vendor 镜像 `ai4all/coqui` 没有 `/health` 端点**——compose 的 healthcheck 用 socket probe 而不是 HTTP。
- **模型挂载路径跨平台**：默认 `${XTTS_MODEL_PATH:-/c/Users/LENVOV/AppData/Local/Temp/coqui_model}` 是 Git Bash 风格路径。Mac/Linux 用户需要 `export XTTS_MODEL_PATH=/Users/.../coqui_model`。
- **XTTS 端点无 `/health` 返回 404**——不影响业务，ai-svc 不检查 vendor 健康，只用 `/voice/*`。

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

## 七、相关文档

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