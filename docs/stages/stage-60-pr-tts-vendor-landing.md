# Stage 60 · 2026-09-10 PR-TTS-VENDOR Landing · 三 AI 模型落地

> **状态**：🟢 **三模型全部落地，dev `--profile ai` 可用**
> **落地日期**：2026-09-10
> **关联计划**：[`docs/ai-models/vendor-candidates.md §五`](../ai-models/vendor-candidates.md) ·
> [`docs/plans/todo-pile-2026-09-04.md §A1`](../plans/todo-pile-2026-09-04.md) ·
> [`docs/stages/stage-59-vendor-image-verification.md`](stage-59-vendor-image-verification.md)

**核心结论**：

| 模型 | 路径 | 镜像大小 | 端到端状态 |
|---|---|---|---|
| **XTTS** | ✅ vendor `ai4all/coqui:latest` | 11.3GB | 上传 voice + 生成 + 拉取 **133KB WAV** |
| **SenseVoice** | ✅ 仓内本地 `emotion-echo-models/sensevoice-small/` | v0.1.2 4.16GB（已预烘焙 model.pt 893MB） | 继承 Stage 36 已有契约，无 vendor 需求 |
| **FER** | ✅ 新仓 `emotion-echo-models/FER-tflite/` | **538MB disk**（原 12.1GB **缩 22.5×**）| 49/49 单测全绿 + `/health` `model_loaded=true` + `/analyze` 200 |

---

## 一、背景

Stage 58 记录了 dev 网络下自建 AI 镜像不可重复（`stage-58-ai-image-build-blocked.md`）。
Stage 59 实测了 3 个 vendor 镜像候选，发现：

- `ai4all/coqui` ✅ 端到端调通（开启 Clash TUN 后 HuggingFace SSL EOF 解决）
- `yiminger/sensevoice` ❌ 挂错标签，**不是语音情绪识别**
- `serengil/deepface` ⚠️ 可拉但容器内 GitHub SSL EOF（缺 vgg_face_weights.h5 权重）

Stage 60 在 Stage 59 基础上把可用的两条路径（XTTS vendor + SenseVoice 本地）+ FER 备选路径（tflite + Haar，绕开 tensorflow 572MB）落地到仓。

---

## 二、本次改动清单

### 2.1 新增文件

| 路径 | 行数 | 用途 |
|---|---|---|
| `emotion-echo-models/FER-tflite/server.py` | 217 | tflite + Haar 后端实现（与原 FER 契约对齐） |
| `emotion-echo-models/FER-tflite/Dockerfile` | 21 | 镜像构建（基于 `python:3.10-slim`，aliyun mirror） |
| `emotion-echo-models/FER-tflite/requirements.txt` | 9 | tflite-runtime 2.14.0 + opencv-contrib-python-headless + numpy<2 |
| `emotion-echo-models/FER-tflite/logging_setup.py` | 42 | JSON logger（复用原 FER 结构） |
| `emotion-echo-models/FER-tflite/metrics_setup.py` | 75 | Prometheus metrics + middleware（复用原 FER 结构） |
| `emotion-echo-models/FER-tflite/emotion_model_quantized.tflite` | 92KB | tflite FER 模型（fer 包内置量化版） |
| `emotion-echo-models/FER-tflite/haarcascade_frontalface_default.xml` | 1.2MB | OpenCV Haar cascade |
| `emotion-echo-models/FER-tflite/tests/unit/test_emotion_mapping.py` | 67 | EMOTIONS 7 类 + EMOTION_MAPPING 5 类映射 |
| `emotion-echo-models/FER-tflite/tests/unit/test_health_route.py` | 53 | /health 契约 |
| `emotion-echo-models/FER-tflite/tests/unit/test_analyze_route.py` | 105 | /analyze 契约（200/empty/bad image/no-face） |
| `emotion-echo-models/FER-tflite/tests/unit/test_logging_setup.py` | 47 | setup_logging 行为 |
| `emotion-echo-models/FER-tflite/tests/unit/test_metrics_setup.py` | 64 | Prometheus 4 metric 注册 + middleware 行为 |

### 2.2 修改文件

| 路径 | 改动 |
|---|---|
| `deploy/docker-compose.apps.yml` | `emotion-echo-fer` 服务 `dockerfile:` 改 `FER-tflite/Dockerfile`，`image:` 改 `emotion-echo/fer-tflite:v0.1.0`，`start_period` 60s → 15s |
| `deploy/docker-compose.apps.yml` | `emotion-echo-xtts` 服务改用 `image: ai4all/coqui:latest`（替换自建），覆盖 `command: ["fastapi", "run", "app.py", "--host=0.0.0.0", "--port=8003"]`，加 `user: root` + volume 挂载 `${XTTS_MODEL_PATH:-/c/Users/LENVOV/AppData/Local/Temp/coqui_model}:/model`，healthcheck 改 socket connect（vendor 无 /health） |
| `docs/ai-models/vendor-candidates.md` | §五 旧结论替换为 §5.1~5.3 三模型落地结论 + ai-svc 调用链说明 |
| `docs/plans/todo-pile-2026-09-04.md` | §A1 追加 Stage 60 closure + 关闭命令 |

### 2.3 ai-svc 端零改动

`emotion-echo-ai-svc/internal/aiclient/xtts.go:46-62` `NewXTTSClient` 在 `BaseURL != ""` 时构造完整客户端，否则返 nil。`main.go:92 applyEnvOverrides` 把 `XTTS_BASE_URL=http://emotion-echo-xtts:8003`（compose 注入）覆盖 `ai-api.yaml` 显式空字符串。**三容器启动后 ai-svc 自动接通，零代码改动**。

---

## 三、关键决策（与文档撰写前做的功课对齐）

### 3.1 已读代码文件（§〇 步骤 ①）

| 文件 | 用途 |
|---|---|
| `emotion-echo-models/FER/server.py` | 291 行原 FER 实现（契约对齐基线） |
| `emotion-echo-models/FER/logging_setup.py` | 42 行 JSON logger（直接复用） |
| `emotion-echo-models/FER/metrics_setup.py` | 75 行 Prometheus metrics（直接复用） |
| `emotion-echo-models/sensevoice-small/Dockerfile` | 已有本地实现目录 |
| `emotion-echo-ai-svc/internal/aiclient/xtts.go` | 151 行 XTTS 客户端（契约基线） |
| `emotion-echo-ai-svc/main.go:86-92` | applyEnvOverrides 实现 |
| `emotion-echo-ai-svc/etc/ai-api.yaml:69-81` | FER/SenseVoice/XTTS BaseURL 默认空 |
| `deploy/docker-compose.apps.yml:387-389, 529-563` | XTTS_BASE_URL/FER_BASE_URL 注入 |
| `docs/ai-models/build-guide.md §5.1` | FER 历史踩坑（emotion_net.caffemodel 缺失） |

### 3.2 已查 ADR / stage / decision（§〇 步骤 ②）

| 文档 | 关联点 |
|---|---|
| `docs/stages/stage-58-ai-image-build-blocked.md` | 锁定自建路径不可行 → 必须 vendor |
| `docs/stages/stage-59-vendor-image-verification.md` | 三 vendor 实测结论（fer/sensevoice/xtts）|
| `docs/ai-models/vendor-candidates.md §五` | 旧结论（截至 2026-09-09）已被本文档替换 |
| `docs/plans/todo-pile-2026-09-04.md §A1` | TTS 缺口在本文档关闭 |
| `docs/plans/todo-pile-2026-09-04.md §A1 失真 #12` | ai-svc 客户端实现已在原位，本文档不重写 |
| `docs/architecture/decisions.md ADR-001` | XTTS 云 API 决策（vendor 路径作为更优替代，本文档覆盖 §A1 重评结论） |

### 3.3 跑过 smoke（§〇 步骤 ③）

| smoke | 结果 |
|---|---|
| `pytest emotion-echo-models/FER-tflite/tests/unit/` | **49/49 全绿** |
| `docker build -t emotion-echo/fer-tflite:v0.1.0 ...` | ✅ 538MB disk（vs 原 12.1GB）|
| `docker run -d -p 8005:8004 emotion-echo/fer-tflite:v0.1.0` + `curl /health` | 200 `{"model_loaded":true,"backend":"tflite+haar"}` |
| `curl -X POST /analyze -F file=@test.jpg` | 200 `{"emotion":"neutral","source":"no-face"}`（合成图无真实人脸） |
| `curl -X POST /analyze` 空文件 | 400 `{"detail":"empty file"}` |
| `curl -X POST /analyze` 非图 bytes | 400 `{"detail":"Invalid image"}` |
| XTTS compose up + `/voice/upload` + `/voice/generate` + `/voice/result` | 200 + 200 + 200 (133KB WAV, RIFF 头正确) |

### 3.4 关键设计选择（§〇 步骤 ⑤ 假设清单）

| 假设 | 验证方式 | 实测 |
|---|---|---|
| tflite 2.14 与 numpy 2.x 不兼容（_ARRAY_API 缺失） | 容器内 `pip install 'tflite-runtime==2.14.0' 'numpy<2'` | ✅ 落地 `requirements.txt` 已 pin |
| opencv-python-headless 5.x 移除 CascadeClassifier | `hasattr(cv2, 'CascadeClassifier') == False` | ✅ 改用 `opencv-contrib-python-headless` |
| starlette 1.6 TestClient 需 httpx2（不是 httpx）| `pip install httpx2` 验证 | ✅ dev 测试通过 |
| ai4all/coqui 默认 listen 8000，需覆盖 8003 | `docker inspect` 看 Cmd | ✅ compose `command:` 覆盖 |
| ai4all/coqui 无 /health 端点（实测 404）| curl `/health` | ✅ compose healthcheck 改 socket probe |
| ai-svc XTTS 客户端 BaseURL 空时返 nil（构造时降级）| 读 `xtts.go:46-62` | ✅ 无 ai-svc 改动 |
| XTTS 模型目录命名是 `tts_models--multilingual--multi-dataset--xtts_v2/`（非短名）| stage-59 §10.4 实测 | ✅ 挂载路径匹配 |

---

## 四、调用链与契约（不变项）

```
前端 (Nuxt) ─► emotion-echo-apisix ─► emotion-echo-web-bff
                                            │
                                            │ gRPC
                                            ▼
                                    emotion-echo-ai-svc (port 8891/8892)
                                            │
                          ┌─────────────────┼─────────────────┐
                          │                 │                 │
                          ▼                 ▼                 ▼
                  emotion-echo-fer   emotion-echo-sensevoice  emotion-echo-xtts
                  (port 8004)        (port 8002)              (port 8003, vendor)
                  backend:           backend:                  backend:
                  tflite+haar        sensevoice v0.1.2         coqui ai4all
                  538MB disk         4.16GB disk               11.3GB disk
```

三个 AI profile 服务的 env `*_BASE_URL` 由 `applyEnvOverrides` 注入；容器 DNS 在 `app-network` 内可达。

---

## 五、待 owner 决策（不阻塞合并，但建议方向）

1. **dev 是否默认启用 AI profile？**
   - 当前：dev 默认不起 AI 容器（`profiles: ["ai"]`），用户须显式 `--profile ai up`
   - 建议：保留显式开关，但**让 QUICKSTART.md 把 `--profile ai` 列入"5 秒必读"**，避免新人 clone 后发现 TTS 不可用
2. **XTTS 模型挂载路径跨平台问题**
   - 当前：`${XTTS_MODEL_PATH:-/c/Users/LENVOV/AppData/Local/Temp/coqui_model}` 是 Git Bash 风格
   - Mac/Linux 用户需要 export `XTTS_MODEL_PATH=/Users/.../coqui_model`
   - 建议：在 QUICKSTART.md 加跨平台说明
3. **vendor `ai4all/coqui` 长期维护风险**
   - Docker Hub stars=0，无明确维护承诺
   - 建议：监控后续是否能找到 `ghcr.io/coqui-ai/coqui-tts-cpu`（Stage 58 测过被拒）或自建兜底

---

## 六、调研依据（commit message 末尾格式）

| 项 | 来源 |
|---|---|
| `emotion-echo/fer-tflite:v0.1.0` 538MB | `docker images` 实测 |
| 49/49 测试全绿 | `pytest emotion-echo-models/FER-tflite/tests/unit/` 实测 |
| XTTS 133KB WAV 端到端 | curl 实测 `/voice/upload` + `/voice/generate` + `/voice/result` |
| ai-svc 零改动 | 读 `applyEnvOverrides` + `NewXTTSClient` + compose env 注入 |
| vendor `ai4all/coqui` 端到端 | stage-59 §十 + 本次重跑 |
| SenseVoice 本地可用 | stage-59 §四.4 + 仓 `sensevoice-small/` 完整实现 |
| tflite 2.14 numpy 兼容 | 容器内实测 `numpy 2.x → _ARRAY_API not found` |
| opencv-contrib-python-headless 必要 | 实测 `cv2.CascadeClassifier` 在 plain headless 5.x 缺失 |
| ai4all/coqui 端口 8000 | `docker inspect ai4all/coqui:latest` |
| ai4all/coqui 无 /health | curl `/health` 实测 404 |
| Stage 36 v0.1.2 sensevoice build 成功 | `docs/stages/stage-36-landing.md:268-271` |

---

> 最后更新：2026-09-10 by Stage 60 PR-TTS-VENDOR landing session
> 用途：记录三 AI 模型（XTTS vendor / SenseVoice 本地 / FER-tflite 备选）落地到仓与 compose 的全过程 + 决策依据
> 后续：跟进 QUICKSTART.md 跨平台说明 + 是否把 `--profile ai` 作为 dev 默认开关