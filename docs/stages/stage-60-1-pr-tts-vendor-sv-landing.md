# Stage 60.1 · 2026-09-10 PR-TTS-VENDOR Phase 5 · SenseVoice 真实验证与重建

> **状态**：🟢 **SenseVoice 三语推理 + 错误路径全部通过**
> **日期**：2026-09-10（Stage 60 收尾同日）
> **关联**：[`docs/stages/stage-60-pr-tts-vendor-landing.md`](stage-60-pr-tts-vendor-landing.md) ·
> [`docs/stages/stage-59-vendor-image-verification.md`](stage-59-vendor-image-verification.md) ·
> commit `0fc1d2e` (SV-fastbuild 镜像) + commit `8e09d03` (compose 切换)

**核心结论（修正 Stage 60 §5.1）**：

Stage 60 commit `7979f17` 把 SenseVoice 标为 "✅ 本地仓 emotion-echo-models/sensevoice-small/ 已有完整实现"——这是**未验证的论断**。该论断依据是 Stage 36 当时 build 成功过 v0.1.2，**但本 dev 主机从未复现这次 build**（stage-58 / stage-59 记录了同样的网络阻断）。今天 Phase 5 才真正跑通了从构建到端到端推理的全链路。

---

## 一、本次新增文件

| 路径 | 大小 | 用途 |
|---|---|---|
| `emotion-echo-models/SV-fastbuild/server.py` | 266 行 | SenseVoice 服务实现（继承自 sensevoice-small/server.py，**修复了 LogRecord extra 冲突** +** token 正则补大小写**） |
| `emotion-echo-models/SV-fastbuild/Dockerfile` | 53 行 | 两阶段 build + apt retry loop + `--find-links /build/wheels/` |
| `emotion-echo-models/SV-fastbuild/requirements.txt` | 12 行 | torch==1.13.1+cpu + kaldi-native-fbank + transformers<5 + ... |
| `emotion-echo-models/SV-fastbuild/logging_setup.py` | 42 行 | JSON logger（与 FER-tflite 复用同一结构） |
| `emotion-echo-models/SV-fastbuild/metrics_setup.py` | 75 行 | Prometheus metrics |
| `emotion-echo-models/SV-fastbuild/model.pt` | 936 MB | SenseVoiceSmall 主模型（预烘焙） |
| `emotion-echo-models/SV-fastbuild/am.mvn` | 11 KB | CMVN 统计文件 |
| `emotion-echo-models/SV-fastbuild/chn_jpn_yue_eng_ko_spectok.bpe.model` | 377 KB | SentencePiece 模型 |
| `emotion-echo-models/SV-fastbuild/config.yaml` + `configuration.json` | KB 级 | funasr 模型配置 |
| `emotion-echo-models/SV-fastbuild/wheels/*.whl` | 887 MB | 6 个预下 wheel（torch+cpu、funasr、modelscope、huggingface_hub、gradio、torchaudio，**构建期中间产物，不进入 runtime 镜像**） |

## 二、本次修改文件

| 路径 | 改动 |
|---|---|
| `deploy/docker-compose.apps.yml` `emotion-echo-sensevoice` | `dockerfile: SV-fastbuild/Dockerfile`；`image: emotion-echo/sensevoice-fastbuild:v0.1.0`；环境加 `FUNASR_MODEL_DIR=/app/model`；`start_period: 300s → 90s`；`retries: 5 → 3` |

## 三、依赖收敛的实证过程（为什么不是"凭直觉选版本"）

每一步都是**真实构建/运行报错倒推**得到的，不靠看文档猜：

| 阶段 | 报错（容器内实际输出） | 根因 | 修复 |
|---|---|---|---|
| 初次 pip install | `ResolutionImpossible` 后开始拉 `nvidia_cudnn_cu11 557MB` + `nvidia_cublas_cu11 317MB` | `torch>=1.13,<3.0.0` 解析到 torch 2.14.0+cpu，pip 把 nvidia CUDA 包当依赖拉 | requirements 改 `torch==1.13.1+cpu` |
| 第一次 docker run | `RuntimeError: Form data requires "python-multipart" to be installed.` | FastAPI multipart 上传未声明依赖 | 加 `python-multipart>=0.0.9` |
| 第一次 POST /analyze | `ImportError: torchaudio is not installed and neither is the kaldi-native-fbank fallback backend. FunASR needs one fbank backend for feature extraction.` | funasr 1.4.15 特征提取要 fbank 后端（torchaudio 或 knf） | 用 `kaldi-native-fbank>=1.18` 替代 torchaudio（~1MB vs ~50MB，且无 torch 版本绑定） |
| 首次 POST /analyze trace | `[transformers] Disabling PyTorch because PyTorch >= 2.5 is required but found 1.13.1+cpu` | transformers 5.16 要 torch≥2.5 | pin `transformers<5` |
| 错误日志 `KeyError: Attempt to overwrite 'filename' in LogRecord` | 服务代码 `extra={"filename": ...}` 与 LogRecord 内置字段冲突 | server.py 改为 `extra={"audio_file": ...}` |
| text 字段残留 `<\|en\|><\|Speech\|><\|withitn\|>` | 正则 `[A-Z_]+` 不匹配小写 token | 改为 `[A-Za-z_]+` |

> **为什么"凭直觉选版本"不可靠**：funasr 1.4.15 文档说"需要 torchaudio 或 kaldi-native-fbank"，但只在第一次跑真实 /analyze 才会触发 `_fbank_knf` 调用链——所以**没有真实推理就没有这次修复**。

## 五、真实推理证据

`docker run --network container:sv-test curlimages/curl -X POST http://localhost:8002/analyze -F file=@/ex/zh.mp3`：

```json
{
  "text": "开放时间早上9点至下午5点。",
  "emotion": "neutral",
  "confidence": 0.6,
  "raw_text": "<|zh|><|NEUTRAL|><|Speech|><|withitn|>开放时间早上9点至下午5点。",
  "source": "sensevoice"
}
```

| 测试样本 | 文本输出 | emotion | 状态 |
|---|---|---|---|
| `example/zh.mp3`（中文） | "开放时间早上9点至下午5点。" | neutral | ✅ 200 |
| `example/en.mp3`（英文） | "The tribal chieftain called for the boy and presented him with 50 pieces of gold." | neutral | ✅ 200 |
| `example/ja.mp3`（日文） | "うちの中学は弁当制で持っていけない場合は、50 円の学校販売のパンを買う。" | neutral | ✅ 200 |
| 空文件 | — | — | ✅ 400 `empty audio bytes` |

## 六、镜像体积与构建时间

| 指标 | 值 |
|---|---|
| 磁盘占用 | 5.88 GB |
| 内容大小 | 2.24 GB |
| 构建时间（无 cache） | ~140 秒 |
| 构建时间（有 cache，仅 server.py 改动） | ~47 秒 |
| wheels 目录（构建期） | 887 MB |

> 注：2.24 GB 中 model.pt 占 936MB。其余来自 torch+cpu 自身的依赖（numpy / scipy / scikit-learn / numba / librosa 等）。如未来要继续减肥，可考虑：① 把 model.pt 改卷挂载而非烘焙（首启时间+网络换体积）；② 用 ONNX Runtime 跑 SenseVoice 的 ONNX 导出版（体积可降到 600MB 量级）。**两件事本次不做**。

## 七、镜像 registry 推送（待 owner 凭证）

Phase 5 解决了"能 build"，但构建仍需 ~140 秒。**真正的 1-3 秒拉取**需要把镜像推到 registry。

详见 [`docs/plans/image-registry-rollout.md`](../plans/image-registry-rollout.md)（commit 待落地后建）。**当前已经确认**：

1. 阿里云 ACR 个人版可用，国内带宽明显优于 Docker Hub。
2. **阿里云目前没有官方的 ACR MCP server**——通过 `~/.docker/config.json` 配凭证已足够。
3. 待 owner 提供：(地域 / 命名空间 / 用户名 / 仓库密码)。

## 八、调研依据

| 项 | 来源 |
|---|---|
| SenseVoice /analyze trace `funasr/auto/auto_model.py:751 generate` → `_fbank_knf raise _knf_missing_error` | 容器内 `python -c "import funasr.utils.fbank; print(...); raise"` |
| transformers 5.x requires torch≥2.5 | `pip show transformers` 输出 |
| `EMOTION_TOKEN_RE` 漏 `<\|en\|><\|Speech\|><\|withitn\|>` | 实测文本字段含这些 token |
| LogRecord 字段冲突 | `KeyError: Attempt to overwrite 'filename' in LogRecord` 在 stage-23-B 已知 |
| funasr load_utils.py:30 ffmpeg 与 soundfile fallback | 直接读 funasr wheel `__init__.py` + utils/load_utils.py |
| aliyun pypi 没有 torch+cpu wheel | 实测 `curl https://mirrors.aliyun.com/pypi/simple/torch/` |
| 清华镜像有 torch-1.13.1+cpu-cp310-cp310-linux_x86_64.whl | `curl https://pypi.tuna.tsinghua.edu.cn/simple/torch/` 列表 |
| Stage 36 v0.1.2 build 当时成功过 | `docs/stages/stage-36-landing.md` 引用 commit `5d0b4cf` |

---

> 最后更新：2026-09-10 by Stage 60.1 PR-TTS-VENDOR Phase 5 session
> 用途：记录 SV-fastbuild 从 wheels 预下 → 镜像构建 → 真实三语推理 → compose 切换的完整过程，纠正 Stage 60 的"未验证论断"
> 下一步：等 owner ACR 凭证，落地 base 层拆分 + 推送