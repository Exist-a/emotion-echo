# AI Profile Vendor 镜像候选清单（Stage 58 调研）

> **状态**：🟡 **调研完成，待尝试**
> **调研日期**：2026-09-09
> **关联决策**：[Stage 58 §三 ADR-001 重审](../architecture/decisions.md) + [todo-pile §A1 TTS 缺口](../../plans/todo-pile-2026-09-04.md) + [stage-58-ai-image-build-blocked.md](../../stages/stage-58-ai-image-build-blocked.md)

---

## 一、背景

Stage 36-B5 自建 3 个 AI 模型镜像（FER / SenseVoice / XTTS）路径在 dev 网络环境（daemon.json 国内 mirror + 梯子不稳定）下不可重复：
- **Stage 36 §B2 记录**：XTTS torch 526MB 卡 30+min
- **Stage 58 PR-TTS-1 第 1 轮**：父镜像 `python:3.10-slim` 拉不动（梯子未开，30s timeout exit 124）
- **Stage 58 PR-TTS-1 第 2 轮**：梯子开后父镜像通了，但 FER build 卡在 apt install OpenCV 254MB（仍是国内 mirror 大包卡死）

**核心问题**：自己 build 模型镜像=反模式（企业标准做法是用 vendor 镜像或云 API）。

本文件**记录候选 vendor 镜像 + 评估风险**，作为后续 vendor 尝试阶段的入口。

---

## 二、3 个 AI 模型的候选清单### 2.1 FER（人脸情绪识别）

|候选 | 描述 | 预估大小 | API 形态 | 风险 |
|---|---|---|---|---|
| **`serengil/deepface`** | Docker Hub 9 stars，OpenCV + DeepFace 预装 | ~500MB-1GB | REST API（analyze endpoint） | 中：版本兼容性（依赖 fer 模型权重） |
| **`ghcr.io/serengil/deepface`** | GitHub Container Registry 同源镜像 | 同上 | 同上 | 中：可能与 docker.io 版本不同步 |
| 自建（保留现有）| `emotion-echo-models/FER/Dockerfile` Stage 36 v0.1.0 12.1GB | 12.1GB | `/health` `/metrics` `/analyze` | 高：网络卡 + 大镜像 |

**建议优先级**：**A 候选 `serengil/deepface`**——先验证是否真能 pull + API 与 emotion-echo FER 接口兼容。

### 2.2 SenseVoice（语音情绪）

|候选 | 描述 | 预估大小 | API 形态 | 风险 |
|---|---|---|---|---|
| **`yiminger/sensevoice`** | Docker Hub 3 stars，FastAPI 封装 | ~600MB-1GB | REST API（FastAPI）| 中：API 路径与 emotion-echo SV 可能不一致 |
| **`modelscope/sensevoice-small`** | ModelScope 官方镜像 | ~600MB | REST API | 中：需确认 ModelScope registry 可达性 |
| 自建（保留现有）| `emotion-echo-models/sensevoice-small/Dockerfile` Stage 36 v0.1.2 4.16GB | 4.16GB | `/health` `/metrics` `/analyze` | 高：含 893MB 模型权重预烘焙，build 复杂 |

**建议优先级**：**B 候选 `yiminger/sensevoice`**——FastAPI 风格与 emotion-echo SV 最接近。

### 2.3 XTTS（语音合成）

|候选 | 描述 | 预估大小 | API 形态 | 风险 |
|---|---|---|---|---|
| **`ai4all/coqui`** | Docker Hub 0 stars，Coqui TTS API server | ~1.5GB | REST API（TTS） | 高：stars=0 风险无维护；可能 API 与 emotion-echo XTTS 不一致 |
| **`ghcr.io/coqui-ai/coqui-tts-cpu`** | Coqui 官方 CPU 镜像 | ~1.5GB | REST API | 中：需 `ghcr.io` registry 可达 |
| **阿里云智能语音 RESTful API** | 云端调用，无镜像 | 0（API 调用）| REST API + WebSocket 流式 | 低：成熟服务，已 ADR-001 决策 |
| **OpenAI TTS API** | 境外 fallback | 0（API 调用）| REST API | 低：仅备选 |
| 自建（保留现有）| `emotion-echo-models/XTTS/Dockerfile` Stage 36 v0.1.0 5.3GB | 5.3GB | `/tts` `/tts_stream` | 高：vendor Coqui 已弃，本地 build 失败率高 |

**建议优先级**：**A 候选云 API（阿里云 primary + OpenAI fallback）**——ADR-001 已决策，零镜像下载成本。

---

## 三、实测结论（2026-09-09）

### 3.1 vendor 镜像 pull 实测

| 操作 | 结果 | 详情 |
|---|---|---|
| `docker pull hello-world` | ✅ 成功 | 25.9kB，5/5 layers |
| `docker pull python:3.10-slim` | ✅ 成功 | 197MB，5/5 layers |
| `docker pull serengil/deepface` | ⚠️ **部分卡** | 多个 layer Pull complete 后停，无明确错误 |
| `docker pull ai4all/coqui` | ⚠️ **正在拉** | Pulling fs layer 阶段，4 层等待中 |
| `docker pull yiminger/sensevoice` | ❌ 未测（时间不允许）| — |
| WebFetch hub.docker.com | ❌ **timeout** | 443 端口 10s timeout |
| WebFetch github.com | ❌ **timeout** | 443 端口 10s timeout |

### 3.2 网络诊断

- `~/.docker/daemon.json` 含 3 个国内 mirror（aliyun / USTC / 163）+ `experimental: false`
- 梯子已开（`python:3.10-slim` 拉成功证明 docker.io 通）
- 但**某些 vendor 镜像 layer 卡死**——与 Stage 36 §B2 "国内 mirror 大包 0字节响应"同类问题
- **WebFetch 完全不通**（GitHub / Docker Hub 443 timeout）——可能是梯子只针对部分域名

### 3.3 vendor 实际拉取结果（本文件 §二 镜像列表）

**0 张 vendor 镜像成功落地**。所有候选镜像**最多到 Pulling 阶段，未完整 Pull complete**。

---

## 四、阶段规划：vendor 尝试阶段（建议命名 PR-TTS-VENDOR）

> **目的**：在 Stage 58 后续 sprint 中，**专门留一个阶段**给 vendor 尝试，不与 PR-TTS-1~4 混淆。

### 4.1 阶段触发条件

|条件 | 阈值 |
|---|---|
| 网络 | 至少 docker.io 单层 < 30s 拉通 + WebFetch docker hub 可达 |
| 时间预算 | 单镜像 build/pull + 验证 ≤ 30 分钟 |
| 优先级 | 当 A1 TTS 缺口成为阻塞 PR 时启动 |

### 4.2 阶段 PR 序列（建议）

| PR | 工作内容 | 工作量 | 依赖 |
|---|---|---|---|
| **PR-TTS-VENDOR-1** | `docker pull serengil/deepface` 跑通 + 写 adapter（ai-svc FER client 接 vendor）| 半天 | 网络通 |
| **PR-TTS-VENDOR-2** | `docker pull yiminger/sensevoice` 跑通 + adapter | 半天 | 同上 |
| **PR-TTS-VENDOR-3** | XTTS 接阿里云 API（ADR-001 落地）| 1 天 | 阿里云账号 + key |
| **PR-TTS-VENDOR-4** | 更新 emotion-echo-models/ 为 deprecated 状态 + 保留 git 历史 | 1 小时 | 1-3 完成后 |

### 4.3 阶段准入/退出条件

**准入**：网络恢复 + vendor 镜像能 Pull complete
**退出**：
- 成功：3 模型全部 vendor 化，emotion-echo-models/ 标记 deprecated
- 失败：保留 emotion-echo-models/ + profiles: [ai]（现状，dev 默认不起）

---

## 五、当前结论（截至 2026-09-10 · Stage 60 landing）

### 5.1 三个 AI 模型落地路径（PR-TTS-VENDOR 闭环）

| 模型 | 路径 | 镜像 | 状态 |
|---|---|---|---|
| **XTTS** | ✅ **vendor `ai4all/coqui:latest`** | 11.3GB（vendor 提供） | 已接入 `deploy/docker-compose.apps.yml`；端到端调通（upload/generate/result 200，133KB WAV） |
| **SenseVoice** | ✅ **本地仓 `emotion-echo-models/sensevoice-small/`** | `emotion-echo/sensevoice:v0.1.0`（compose build） | Stage 36 v0.1.2 镜像仓内已预烘焙 893MB `model.pt` + am.mvn + config；vendor 镜像 `yiminger/sensevoice` 挂错标签不可用 |
| **FER** | ✅ **本地新仓 `emotion-echo-models/FER-tflite/`** | `emotion-echo/fer-tflite:v0.1.0`（538MB disk，**比原 12.1GB 缩 22.5×**）| tflite + Haar cascade 后端，绕开 tensorflow 572MB；49/49 单元测试全绿 |

### 5.2 关键决策变更（vs 2026-09-09 旧结论）

| 旧结论（§五 截至 2026-09-09） | 新结论（Stage 60/60.1） | 原因 |
|---|---|---|
| "Vendor 镜像 0 张完整 pull 成功" | ✅ 3 张 vendor/本地实现可拉/可 build | 开启 Clash TUN 模式后境外资源可达（stage-59 §十）|
| "XTTS 走云 API（ADR-001）" | ✅ 改走 vendor `ai4all/coqui`（docker.io aliyun 加速拉取） | vendor 镜像端到端调通（133KB WAV 输出）；云 API 仍保留为 future fallback（`docs/ai-models/xtts-decision.md`） |
| "FER 自建卡死 + 重 30+ 分钟" | ✅ tflite 备选路径，build < 1min | 原型实测验证后正式落地为 `FER-tflite/` 独立目录（镜像 538MB disk） |
| "SenseVoice 走 yiminger vendor" | ✅ 改走仓内本地实现（`SV-fastbuild/`） | yiminger 镜像挂错标签，仓内 `sensevoice-small/` 已完整可用；funasr+wheels预下+TUN 实现 |

### 5.3 实施入口（实施者从 emotion-echo-models/README.md 开始）

> 本文件是**调研选型记录**（"为什么是这个方案"）。具体怎么 build / 怎么跑 / 怎么排错，看：
> **[`emotion-echo-models/README.md`](../../emotion-echo-models/README.md)** —— 3 个 AI 容器的主实施指南（FER-tflite / SV-fastbuild / XTTS vendor 三节）。

### 5.3 ai-svc 调用链已通

| 模型 | compose 服务名 | ai-svc env | 端到端契约 |
|---|---|---|---|
| FER | `emotion-echo-fer` (port 8004) | `FER_BASE_URL=http://emotion-echo-fer:8004` | POST /analyze → `{emotion, confidence, scores, source}` |
| SenseVoice | `emotion-echo-sensevoice` (port 8002) | `SENSEVOICE_BASE_URL=http://emotion-echo-sensevoice:8002` | 同上 |
| XTTS | `emotion-echo-xtts` (port 8003, vendor) | `XTTS_BASE_URL=http://emotion-echo-xtts:8003` | POST /tts → base64 WAV |

ai-svc `applyEnvOverrides` (`main.go:86`) 在容器 DNS 可达时自动接通，**前端 TTS 按钮 dev 模式启用 AI profile 即可用**。

---

## 六、调研依据

| 项 | 来源 |
|---|---|
| docker search 结果 | `docker search --limit 8 --filter "is-official=true" fer` 等实测 |
| Stage 36-B5 历史 | `docs/stages/stage-36-fixes-roadmap.md §B2` + commit `5d0b4cf`/`256c902`/`d50f866` |
| Stage 58 PR-TTS-1 第 1 轮 | `scripts/test_ai_image_build_precheck.sh` 15/16 PASS |
| Stage 58 PR-TTS-1 第 2 轮 | `docs/stages/stage-58-ai-image-build-blocked.md` 第 2 轮实测 |
| ADR-001 XTTS 云端化 | `docs/ai-models/xtts-decision.md` §二 决策 |
| 企业级 vendor 实践 | Stage 58 v2 plan §PR-TTS-1 风险条款 |

---

> 最后更新：2026-09-09 by Stage 58 v2 plan 协作 session
> 用途：vendor 镜像候选清单 + PR-TTS-VENDOR 阶段入口
> 后续：当网络恢复或决定接云 API 时，按 §四 阶段 PR 序列推进