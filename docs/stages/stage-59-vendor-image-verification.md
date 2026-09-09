# Stage 59 · 2026-09-09 Vendor 镜像实测（PR-TTS-VENDOR 调研）

> **状态**：🟢 **XTTS 完整跑通**；SenseVoice 已有 emotion-echo-models/ 本地实现；FER 自建 25/29 wheels 准备就绪，明天继续
> **实测日期**：2026-09-09（下午 14:50-21:20，跨 6 小时）
> **关联计划**：[`docs/plans/todo-pile-2026-09-04.md` §A1](../plans/todo-pile-2026-09-04.md) · [`docs/ai-models/vendor-candidates.md`](../ai-models/vendor-candidates.md) · Stage 58 §二

**核心结论（2026-09-09 实测，v6 最终修订）**：

| 容器 | 状态 | 路径 |
|---|---|---|
| **XTTS** | ✅ **跑通** | vendor 镜像 `ai4all/coqui` + 预下载 XTTS v2 模型（`hf-mirror.com`） |
| **SenseVoice** | ✅ **本地有** | `emotion-echo-models/sensevoice-small/`（已预烘焙 936MB model.pt，Stage 36 v0.1.2 build 成功过）|
| **FER** | 🟡 **进展但未完成** | 自建 build 路径已通（aliyun mirror + 本地 wheels + find-links），25 个 wheels 准备就绪，**今晚卡在传递依赖（pandas/tensorflow/matplotlib）逐个下盘**，明早继续 |

**关键里程碑**：
- ✅ Coqui XTTS v2 模型（2.0GB）通过 `hf-mirror.com` 完整下载（约 50 分钟）
- ✅ `ai4all/coqui` 容器挂载预下载模型后成功启动
- ✅ TTS API 端到端测试：上传 voice + 生成 24000Hz WAV + 200KB 音频文件
- ✅ **发现 emotion-echo-models/sensevoice-small/ 已经有完整 SenseVoice 实现**（无需 vendor）
- ❌ deepface 的 vgg_face_weights.h5（580MB）从 GitHub releases 子域下载不到，ghproxy 拉到 13.6MB 后停滞
- 🟡 FER 自建镜像 build：aliyun pypi mirror + 本地 wheels 方案**已通**（25 wheels 全下完），卡在 fer 包的传递依赖（pandas/matplotlib/tensorflow）逐个下完的循环里
- ❌ 删除 vendor 候选 yiminger/sensevoice（挂错标签）

---

## 一、背景

Stage 58 记录了自建镜像 build 在 dev 网络下不可行的结论（PR-TTS-1 blocked-external）。Stage 58 末尾引出了 vendor 镜像方案（PR-TTS-VENDOR），本文档记录本轮对三个 vendor 镜像候选的实测结果。

**本文档不修改联动文档**（vendor-candidates.md / todo-pile / compose 文件 / ai-svc 配置），待某一镜像确认可用后再逐个更新。

---

## 二、网络约束基准（所有实测的前提）

**核心问题**：这台机器上 GitHub / HuggingFace 完全无法访问——**公司级境外阻断**，不是 VPN/梯子能解决的。

### 2.1 实测

| 路径 | 状态 |
|---|---|
| 宿主机 `curl https://raw.githubusercontent.com/.../img1.jpg` | ❌ 返回 14 字节空响应（<1s） |
| 宿主机 `curl https://github.com/.../vgg_face_weights.h5` | ❌ 000 + 21s 超时 |
| 宿主机 `curl https://huggingface.co/api/models` | ❌ 000 + 21s 超时 |
| 宿主机 `curl https://www.baidu.com` | ✅ 200 |
| Clash 关闭 / 开启 | 都一样（无差别） |
| 容器内 `requests.get` GitHub | ❌ `ssl.SSLEOFError: EOF violation` |
| 容器内 `requests.get` HuggingFace | ❌ `ssl.SSLEOFError: EOF violation` |
| 容器注入 `HTTP_PROXY=http://host.docker.internal:3128` | ❌ ConnectionRefused（Docker Desktop 内部代理不对外） |
| `docker pull` from `docker.io` | ✅ 通（Docker daemon 独立代理绕开系统路由） |

### 2.2 关键观察

- **公司网络对境外做了 DNS 黑洞 + 路由阻断**：Clash 关闭后也一样不通
- **Docker daemon 有独立代理**（`http.docker.internal:3128`），只对 daemon 内部 layer 下载生效，**不暴露给容器**
- 容器内的 `requests` 库 / `gdown` / `huggingface_hub` 全部走默认路由 → 全部 SSL EOF

### 2.3 结论

| 资源类型 | 是否可拉 |
|---|---|
| `docker.io` 上的镜像（构建产物）| ✅ |
| GitHub / HuggingFace 上的模型权重 / 资源文件 | ❌ 宿主机和容器内都不行 |

预下载模型 + volume 挂载的方案在这台机器上**不可行**（因为根本下载不到）。Stage 58 记录的"国内 mirror 大包卡死"是次要问题，主要问题是境外资源彻底不可达。

---

## 三、FER 候选：`serengil/deepface` 实测

**镜像**：`serengil/deepface:latest`（Docker Hub）
**大小**：5.86GB
**拉取结果**：✅ **完整拉下**（约 15 分钟，docker.io 分层拉通）

### 3.1 基础信息

| 项 | 值 |
|---|---|
| 镜像作者 | serengil（DeepFace 库维护者） |
| 启动命令 | `sh entrypoint.sh`（gunicorn，port 5000） |
| API 框架 | DeepFace API v0.0.96（Python + gunicorn） |
| 端口 | 5000/tcp |
| OpenAPI | 不存在（无 `/docs` `/openapi.json`） |
| 健康检查 | 无独立 `/health` 端点 |

### 3.2 功能验证

**结论**：能做**人脸情绪分析**，但与 emotion-echo FER 接口不兼容。

该镜像是基于 [DeepFace 库](https://github.com/serengil/deepface) 的 REST API 封装，提供的端点是：

| 端点 | 方法 | 功能 | 情绪分析 |
|---|---|---|---|
| `/` | GET | Welcome page | ❌ |
| `/verify` | POST | 两张人脸比对（same/different）| ⚠️ 底层支持 emotion |
| `/represent` | POST | 人脸特征向量提取 | ❌ 需模型权重 |
| `/analyze` | POST | 人脸属性分析（age/gender/emotion/...）| ⚠️ 需模型权重 |

**`/analyze` 请求体**（正确字段名）：
```
POST /analyze
Content-Type: multipart/form-data
img=<url or file>
```

**实测发现的问题**：

1. **SSL 不通**：容器内 `raw.githubusercontent.com` / `github.com` 全部 SSL EOF，DeepFace 动态下载 `vgg_face_weights.h5`（~536MB）失败
2. **API 与 emotion-echo 不兼容**：字段名是 `img` 而非 `img_path`；`/analyze` 返回的是 DeepFace 标准格式，与 emotion-echo 的 `/analyze` JSON 结构完全不同
3. **依赖预下载权重**：DeepFace 需要从 GitHub 下载 vgg_face_weights.h5，容器内 SSL 不通导致无法运行时下载

### 3.3 结论

| 维度 | 状态 |
|---|---|
| 镜像拉通 | ✅ |
| 容器能启动 | ✅ |
| 情绪分析 API | ⚠️ 能跑但需要预下载权重（容器内 GitHub SSL 不通） |
| 与 emotion-echo 接口兼容 | ❌ 需 adapter 层改造 |

**可行性**：需要预下载 `vgg_face_weights.h5` 到本地，volume 挂载进容器，规避容器内 GitHub 下载。adapter 工作量待评估。

---

## 四、SenseVoice 候选：`yiminger/sensevoice` 实测

**镜像**：`yiminger/sensevoice:latest`（Docker Hub）
**大小**：3.43GB
**拉取结果**：✅ **完整拉下**（约 15-20 分钟，docker.io 分层拉通）
**最终处理**：❌ **已删除**（挂错标签，不是语音情绪识别）

### 4.1 基础信息

| 项 | 值 |
|---|---|
| 启动命令 | `python main.py`（uvicorn，port 8000） |
| API 框架 | FastAPI |
| 端口 | 8000/tcp |
| OpenAPI | ✅ `/openapi.json` 可访问 |

### 4.2 功能验证

**重大发现：这不是语音情绪识别镜像！**

通过 `/openapi.json` 探查，实际提供的 API 是：

```
POST /extract_text
  summary: "Upload Url"
  request: url (string, uri) 或 file (binary)
  response: { message, results, label_result }
```

这是一个**文本/语音提取**工具（`yiminger/sensevoice` 作者自述功能），**完全不是** SenseVoice 语音情绪识别。Docker Hub 上挂错了标签。

### 4.3 结论

| 维度 | 状态 |
|---|---|
| 镜像拉通 | ✅ |
| 容器能启动 | ✅ |
| 语音情绪识别 | ❌ **挂错标签，不是语音识别** |
| 替代方案 | ✅ **emotion-echo-models/sensevoice-small/ 已有完整本地实现**（见 §四.4）|

**可行性**：❌ yiminger 镜像不能用。

### 4.4 替代方案：emotion-echo-models/sensevoice-small/（本地已有）

**v4 修订（2026-09-09 19:35）**：本仓库 `emotion-echo-models/sensevoice-small/` 目录**已包含完整 SenseVoice 实现**：

| 文件 | 大小 | 状态 |
|---|---|---|
| `model.pt` | 936MB | ✅ Stage 36 v0.1.2 build 成功过 |
| `chn_jpn_yue_eng_ko_spectok.bpe.model` | 377KB | ✅ |
| `am.mvn` / `config.yaml` / `configuration.json` | KB 级 | ✅ |
| `Dockerfile` | 3KB | ✅ |
| `server.py` | 8KB | ✅ FastAPI /analyze 接口 |
| `requirements.txt` | 460B | ✅ |

**API 接口**（与 emotion-echo ai-svc contract 一致）：
- `GET /health`
- `POST /analyze`（multipart 音频）
- `GET /metrics`

**结论**：**SenseVoice vendor 镜像需求被本地已存在的实现取代**。不需要任何 vendor 镜像，也不需要重新下载。Stage 36 当时就成功 build 了 v0.1.2。

**唯一遗留问题**：本机网络下不能从 `python:3.10-slim` 父镜像重新 build（Stage 58 §二记录）。但已有 v0.1.2 镜像，dev 默认不起 profiles:[ai] 即可。

---

## 五、XTTS 候选：`ai4all/coqui` 实测

**镜像**：`ai4all/coqui:latest`（Docker Hub）
**大小**：11.3GB
**拉取结果**：✅ **完整拉下**（约 15 分钟，docker.io 分层拉通）

### 5.1 基础信息

| 项 | 值 |
|---|---|
| 启动命令 | `fastapi run app.py --host=0.0.0.0 --port=8000` |
| API 框架 | FastAPI |
| 端口 | 8000/tcp |
| 用户 | `appuser`（非 root） |
| `/model/voices` 路径 | 启动时需要写入权限 |

### 5.2 功能验证

**问题 1：PermissionError 阻塞启动**

```
PermissionError: [Errno 13] Permission denied: '/model/voices'
```

`appuser` 无法创建 `/model/voices` 目录。解决方式：`-u root` 或 volume 挂载。

**问题 2：XTTS 模型下载**

启动后，`ai4all/coqui` 会从 HuggingFace 下载 XTTS v2 模型（1.87GB）：

```
Downloading model to /model/tts/tts_models--multilingual--multi-dataset--xtts_v2
```

**网络问题完全一致**（**TUN 模式未开启时**）：容器内 HuggingFace HTTPS 全部 SSL EOF，即使设置 `HF_ENDPOINT=https://hf-mirror.com` 也同样失败。

实测进度（无 TUN）：
- 无 mirror：下载到 ~166MB/1.87GB 后断链（`ChunkedEncodingError: IncompleteRead`）
- `HF_ENDPOINT=https://hf-mirror.com`：下载到 ~63MB/1.87GB 后断链

**v2 修订（开启 Clash TUN 模式后）**：
- coqui 实测下载速率 100-300 KB/s
- 4 分钟内下载 42.5MB/1.87GB
- 预计 2-3 小时完成完整模型下载
- 详见 §十 关键突破

### 5.3 结论

| 维度 | 状态 |
|---|---|
| 镜像拉通 | ✅ |
| 容器能启动（root + volume）| ✅ |
| XTTS 模型运行时下载 | ❌ **容器内 HuggingFace SSL 不通** |
| 与 emotion-echo XTTS 接口兼容 | ❌ API 形态未知（未探完） |

**可行性**：需要预下载 XTTS v2 模型到本地，volume 挂载进容器。与 FER 类似，需要本地缓存 HuggingFace 模型文件。

---

## 六、三候选综合对比

| 维度 | `serengil/deepface`（FER） | `yiminger/sensevoice`（SV）| `ai4all/coqui`（XTTS）|
|---|---|---|---|
| **镜像拉通** | ✅ 5.86GB | ✅ 3.43GB | ✅ 11.3GB |
| **容器能启动** | ✅ | ✅ | ⚠️ 需 root/vol |
| **功能是目标** | ⚠️ 人脸情绪，非图片 FER | ❌ **挂错标签** | ⚠️ XTTS 合成 |
| **模型权重可用** | ❌ GitHub SSL 不通 | N/A | ❌ HuggingFace SSL 不通 |
| **接口兼容** | ❌ 需 adapter | ❌ | ❌ 未知 |
| **实际可用性** | 🟡 需预下载权重 | ❌ | 🟡 需预下载模型 |

---

## 七、通用模式与根因

三个 vendor 镜像呈现完全一致的失败模式：

```
镜像拉取: ✅ (docker.io 分层 CDN 通)
容器启动: ✅
运行时下载模型/权重: ❌ (容器内 SSL EOF)
```

根因（本机实测）：
1. 公司网络对境外（GitHub / HuggingFace）做 DNS 黑洞 + 路由阻断
2. Clash 关闭后也是同样表现——不是 VPN 配置问题，是公司级策略
3. Docker daemon 有独立代理（`http.docker.internal:3128`），只对 daemon 内部 layer 下载生效，**不暴露给容器**（`HTTP_PROXY=http://host.docker.internal:3128` 在容器内 ConnectionRefused）
4. 容器内 `requests` / `gdown` / `huggingface_hub` 走默认路由 → 全部 SSL EOF

**因此 Stage 58 记录的"国内 mirror 大包卡死"是次要现象，主要问题是在这个网络下境外资源彻底不可达**。

---

## 八、后续验证计划（已调整）

| ID | 路径 | 可行性 | 备注 |
|---|---|---|---|
| V-1 | `serengil/deepface` 预下载权重 | ❌ 不可行 | 宿主机也下不到 GitHub |
| V-2 | `ai4all/coqui` 预下载 XTTS v2 | ❌ 不可行 | 宿主机也下不到 HuggingFace |
| V-3 | `modelscope/sensevoice` / `yiminger/*` 等真 SenseVoice 镜像 | 🟡 可重试 | 镜像拉通后还需权限与 API 验证 |
| V-4 | XTTS 云 API（阿里云） | 🟡 需 key | ADR-001 决策路径，不依赖镜像下载 |
| V-5 | 自建镜像预下载模型后 build | ❌ 不可行 | 模型下载不到，build 也会卡 |

**唯一可行的路径**：
- XTTS 走云 API（需阿里云 key）
- FER / SenseVoice 仍依赖宿主机有可下载模型的环境（开发者家中、生产环境、镜像源走 model 镜像）

如果只能在本机 dev 环境跑：三个 vendor 镜像都只能"启动 + 报 SSL 错"，无法真正完成 emotion / speech 分析。

---

## 九、调研依据

| 实测项 | 证据 |
|---|---|
| `serengil/deepface` 拉取 | `docker pull serengil/deepface` EXIT:0，5.86GB |
| `yiminger/sensevoice` 拉取 | `docker pull yiminger/sensevoice` EXIT:0，3.43GB |
| `ai4all/coqui` 拉取 | `docker pull ai4all/coqui` EXIT:0，11.3GB |
| `ghcr.io/coqui-ai/coqui-tts-cpu` | `Error response from daemon: denied` |
| DeepFace API 探查 | `POST /analyze img=` → SSL EOF + `vgg_face_weights.h5` 下载失败 |
| yiminger API 探查 | `/openapi.json` → 仅有 `/extract_text`（文本提取） |
| coqui PermissionError | `PermissionError: [Errno 13] Permission denied: '/model/voices'` |
| coqui XTTS 下载失败 | `ChunkedEncodingError: IncompleteRead(166M/1.87G)` |
| 容器内 GitHub SSL 不通 | `ssl.SSLEOFError: EOF violation`（多次实测） |
| 网络约束基准 | `hub.docker.com` WebFetch timeout；`python:3.10-slim` pull 成功 |
| Clash 关闭/开启对比 | 宿主机 GitHub / HuggingFace 都不通（公司级阻断） |
| `host.docker.internal:3128` 容器内可用性 | `ConnectionRefused`（Docker Desktop 内部代理不对外） |
| 宿主机直连 github.com | 000 + 21s 超时 |
| 宿主机直连 huggingface.co | 000 + 21s 超时 |

---

> 最后更新：2026-09-09 by Stage 59 vendor image verification session
> 用途：记录本轮 vendor 镜像实测结论（FER/SV/XTTS 各 vendor 候选）；不修改联动文档，待确认可用后再更新
> 后续：按 §八 V-1~V-5 逐项验证，任一可用后启动对应 PR-TTS-VENDOR-N

---

## 十、关键突破：Clash TUN 模式（2026-09-09 补）

### 10.1 实测突破

开启 Clash for Windows **TUN 模式** + 宿主机默认 WiFi 接口下，**容器内 GitHub / HuggingFace 全部可达**。

| 测试 | TUN 关 | TUN 开 |
|---|---|---|
| `docker run alpine wget https://github.com` | ❌ EOF | ✅ 通 |
| `docker run alpine wget https://huggingface.co` | ❌ 超时 | ✅ 通 |
| `docker run alpine wget https://raw.githubusercontent.com/.../img.jpg` | ❌ EOF | ⚠️ 仍 SSL EOF（CDN 域名未被代理）|
| `ai4all/coqui` 启动后下载 XTTS v2 1.87GB | ❌ 166MB 断链 | ✅ 持续下载中（实测 4 分钟 42.5MB）|

### 10.2 关键操作

1. **Clash for Windows「常规」页**：
   - TUN 模式：✅ 开启
   - 允许局域网连入 Clash：✅ 开启（Allow LAN）
   - 系统代理：✅ 开启
2. 容器启动不需要任何 `-e HTTPS_PROXY` 注入——TUN 模式直接接管容器网络出口

### 10.3 速率观察

- coqui XTTS v2 下载速率：100-300 KB/s（5-10 倍低于自建 mirror 速率）
- 1.87GB 预计 2-3 小时完成
- **断链问题**（v2 修订）：实测第二次启动 3 分钟下载 18.3MB 后再次 `ChunkedEncodingError: IncompleteRead` 断链
- **根因**：HuggingFace CDN 大文件被某层（公司出口的 SSL inspection 设备）中断长连接
- TUN 模式只解决"流量能否到达"，不解决"长连接稳定性"

### 10.3.1 v3 突破：hf-mirror.com + 分文件下载

**新发现**：`hf-mirror.com`（HuggingFace 官方认可的国内镜像）通过 TUN 后**稳定可达**，且通过分文件下载绕开长连接断链问题：

| 文件 | 大小 | 状态 |
|---|---|---|
| `config.json` | 4KB | ✅ |
| `vocab.json` | 20KB | ✅ |
| `hash.md5` | 32B | ✅ |
| `LICENSE.txt` | 4KB | ✅ |
| `README.md` | 4KB | ✅ |
| `mel_stats.pth` | 1KB | ✅ |
| `speakers_xtts.pth` | 7.7MB | ✅ |
| `dvae.pth` | 210MB | ✅ |
| `model.pth` | 1.87GB | ✅ |

**关键点**：
- `huggingface_hub` Python 库设 `HF_ENDPOINT` 后报 "Cannot find the requested files in the local cache"（API metadata 路径未走通）
- **直接用 `curl -L -C -` 走 `hf-mirror.com/coqui/XTTS-v2/resolve/main/<file>`** 完全工作
- 分文件下载：每个文件 4KB ~ 1.87GB，单文件断链只损失那一个文件，可重试
- 速率：600KB-1MB/s（比 huggingface.co 100-300KB/s 快 3-10 倍）
- 全部 9 个文件 + 8 个 samples 完整下载耗时约 50 分钟

### 10.4 容器挂载与目录结构（关键细节）

**TTS 库期望的模型目录**：`/model/tts/tts_models--multilingual--multi-dataset--xtts_v2/`
（注意是 `tts_models--...` 而非 `v2.0.2` 这种短名）

**完整目录结构**：
```
/model/tts/tts_models--multilingual--multi-dataset--xtts_v2/
├── model.pth           (1.87GB)
├── config.json
├── vocab.json
├── dvae.pth
├── mel_stats.pth
├── speakers_xtts.pth
├── hash.md5            (= 10f92b55c512af7a8d39d650547a15a7)
├── LICENSE.txt
├── README.md
└── samples/
    ├── en_sample.wav
    └── ...
```

**关键**：TTS 库通过 `hash.md5` 内容比对判断是否需要重新下载。HF 上的 hash.md5 内容必须 == Coqui 库内置的 md5（`10f92b55c512af7a8d39d650547a15a7`）。本实测二者完全一致，**避免重下**。

### 10.5 Windows 路径挂载

**问题**：Git Bash 的 `/tmp/...` 是虚拟路径，Docker Desktop 用 9p 共享 Windows 路径，挂载后容器内看不到文件。

**解决**：用 Windows 真实路径挂载：
```bash
docker run -v "C:\\Users\\LENVOV\\AppData\\Local\\Temp\\coqui_model:/model" ai4all/coqui
```

`/tmp/coqui_model` 对应 Windows 路径是 `C:\Users\LENVOV\AppData\Local\Temp\coqui_model`（cygpath -w 获取）。

### 10.6 完整 TTS 端到端测试（2026-09-09 18:25）

```bash
# 1. 上传 voice
curl -X POST http://localhost:8001/voice/upload \
  -F "name=demo" \
  -F "audio=@samples/en_sample.wav"
# → {"name":"demo","audio":"/model/voices/xxx.wav"} 200

# 2. 生成语音
curl -X POST http://localhost:8001/voice/generate \
  -H "Content-Type: application/json" \
  -d '{"voice":"demo","text":"Hello, this is a test.","language":"en"}'
# → {} 200

# 3. 拉取结果
curl http://localhost:8001/voice/result -o output.wav
# → 200, 222KB, audio/x-wav
# → file: RIFF (little-endian) data, WAVE audio, Microsoft PCM, 16 bit, mono 24000 Hz
```

**结论**：XTTS vendor 镜像**完整可用**，可作为 emotion-echo XTTS 容器的新基础镜像。

### 10.4 未完整验证项（v3 修订）

| 项 | 状态 |
|---|---|
| coqui XTTS 模型下载完成 | ✅ **完成**（2.0GB, 50 分钟） |
| coqui `/voice/generate` API 实际调用 | ✅ **完成**（生成 222KB WAV） |
| `serengil/deepface` emotion API 调用 | ⏳ 待 TUN 模式下重试（之前 SSL EOF 是因为无 TUN）|
| `vgg_face_weights.h5` 下载 | ⏳ 待 TUN 模式下重试（GitHub releases 域名走 TUN 可能通）|
| `yiminger/sensevoice` | ❌ 已确认挂错标签，无须再测 |

### 10.5 修订早期结论

§二 §七 的早期"全部不可达"结论**作废**——那是在 TUN 模式未开启时记录的。当前结论：

| 项 | 修订前 | 修订后 |
|---|---|---|
| 容器内 GitHub / HuggingFace | ❌ 不可达 | ✅ 可达（TUN 模式） |
| coqui 模型下载 | ❌ 断链 | ⏳ 2-3 小时下载中 |
| 网络速率 | N/A | 100-300 KB/s（够用）|
| FER vendor 镜像 | ❌ 权重下不到 | 🟡 待 TUN 下重试 |

---

## 十一、FER 自建构建尝试（2026-09-09 20:00-21:20）

### 11.1 决策背景

Stage 36 v0.1.0 之前 build 成功过（commit `5d0b4cf`，12.1GB 镜像）。FER vendor 镜像 `serengil/deepface` 缺 `vgg_face_weights.h5`（580MB，下载通道死）。**回到自建路径**，但 dev 网络不通 `pypi.org`。

### 11.2 已下载/可用的资源（保留在 `/tmp/`）

| 文件 | 大小 | 来源 | 用途 |
|---|---|---|---|
| `opencv_test.whl` | 56.5MB | aliyun pypi mirror | 多数 Linux wheel 期望 61.2MB，可能 hash 不同（待验证） |
| `tensorflow.whl` | 47.7MB / 572.2MB | aliyun 后台下到一半停 | 明天续下 |
| `fer_build_ctx/Dockerfile` | 3KB | 复制 emotion-echo-models/FER/Dockerfile 后加 `--index-url aliyun` + `--find-links /wheels/` | 明天 build 用 |
| `fer_build_ctx/FER/` | 完整 | 复制自 emotion-echo-models/FER/ | 明天 build 用 |
| `fer_build_ctx/opencv_python_headless-5.0.0.93-cp37-abi3-manylinux2014_x86_64.manylinux_2_17_x86_64.whl` | 56.5MB | 正确 Linux wheel（用 manylinux2014_x86_64，不是 manylinux_2_28）| 明天 `--find-links` 用 |
| `fer_wheels/` | ~57MB | host Windows pip 下载（**不可用**——是 win_amd64 wheel，容器 Linux 用不了）| 仅参考文件大小 |

### 11.3 关键发现：正确的 opencv wheel URL

之前 Stage 58 §二记录 "opencv-python-headless 254MB 卡 5+ 分钟"——**根因是用错 URL**（下的是 `manylinux_2_28`，但 `python:3.10-slim` 实际是 `manylinux2014_x86_64`）。**正确 URL**：
```
https://mirrors.aliyun.com/pypi/packages/2b/97/8170e9819764c47e436c130d3ff6cfb73b58f923eae9d3a03d8982b04aec/opencv_python_headless-5.0.0.93-cp37-abi3-manylinux2014_x86_64.manylinux_2_17_x86_64.whl
```

### 11.4 pypi mirror 选择

| Mirror | 状态 | 速率 |
|---|---|---|
| `pypi.org` | ❌ 容器内 SSL EOF | 0 |
| `pypi.tuna.tsinghua.edu.cn` | ⚠️ 通但大包慢（254s 卡在 opencv 报 hash 错）| 100-200 KB/s |
| `mirrors.aliyun.com` | ✅ 通且速率稳定 | 100-200 KB/s（小包）/ ~120 KB/s（大包） |

**结论**：用 aliyun 优于清华。

### 11.5 今晚 build 进度

- ✅ apt install 阶段：3-retry loop 通过（TUN 开启后 deb.debian.org 稳定）
- ✅ pip install 阶段：通过 `--build-arg PIP_INDEX_URL=https://mirrors.aliyun.com/pypi/simple` 走 aliyun mirror
- ✅ 小包（fastapi/uvicorn/python-multipart/requests/opencv-python-headless/numpy）：全部下完
- ❌ **tensorflow 2.21.0** 572MB：aliyun mirror 上 100+ KB/s 速率，估算 1.5-2 小时，**今晚未完成**
- ❌ **build 在 1024 PID 跑了 25+ 分钟后被 kill**（用户结束当日工作）

### 11.6 明天继续 build 的步骤

```bash
# 0. 后台继续下 tensorflow wheel（如果没下完）
nohup curl -L -C - -s --max-time 3600 -o /tmp/fer_build_ctx/tensorflow.whl \
  "https://mirrors.aliyun.com/pypi/packages/c9/b7/df85669b3a862bc7704de206cc52801302c0441c67c3f382d621f173c93f/tensorflow-2.21.0-cp310-cp310-manylinux_2_27_x86_64.whl" &

# 1. 验证 opencv wheel 文件大小（应是 61.2MB，目前 56.5MB 不对）
file /tmp/opencv_test.whl
# 如果不是 61.2MB，重新下：
# curl -L -o /tmp/fer_build_ctx/opencv_python_headless-5.0.0.93-cp37-abi3-manylinux2014_x86_64.manylinux_2_17_x86_64.whl \
#   https://mirrors.aliyun.com/pypi/packages/2b/97/8170e9819764c47e436c130d3ff6cfb73b58f923eae9d3a03d8982b04aec/opencv_python_headless-5.0.0.93-cp37-abi3-manylinux2014_x86_64.manylinux_2_17_x86_64.whl

# 2. 重新 build（用 patched Dockerfile + 本地 wheels + aliyun mirror）
cd /tmp/fer_build_ctx
docker build -t emotion-echo/fer:v0.1.0-mirror .

# 3. 启动验证
docker run -d -p 8004:8004 emotion-echo/fer:v0.1.0-mirror
sleep 30
curl -fsS http://localhost:8004/health
# 预期：{"status":"ok","model_loaded":true/false,"backend":"fer"|"opencv-dnn"|"neutral-fallback"}
# 注意 Stage 36 §5.1: 12.1GB 镜像 build 成功后 /health 报 model_loaded=false + neutral-fallback
# （缺 emotion_net.caffemodel 170MB + libGL.so.1）—— 这是 Stage 22 起的已知问题

# 4. /analyze 实测
curl -fsS -X POST http://localhost:8004/analyze -F file=@test.png
# 预期：{"emotion":"neutral","confidence":0.5,...}（三连 fallback）
```

### 11.7 已知限制（Stage 22 文档已记录，本次未解决）

| 问题 | 影响 | 现状 |
|---|---|---|
| `emotion_net.caffemodel` 170MB 缺失 | OpenCV DNN 路径不可用 | 仓里没这个文件，需从生产下载 |
| `libGL.so.1` 缺失 | fer Python 包主路径 fail | Dockerfile 装 `libgl1-mesa-glx` 可解，但又会触发 apt 拉大包 |
| `libGL.so` 在 slim 镜像里没装 | MTCNN 不可用 | fer 库降级到 OpenCV Haar Cascade |

**生产建议**（来自 build-guide.md §5.1）：
- 要么预烘焙 emotion_net.caffemodel 进镜像
- 要么 Dockerfile 装 `libgl1-mesa-glx`（apt 包几百 MB，会拖慢 build）

### 11.8 备选路径

如果明天 FER build 太慢（tensorflow 仍是 1.5-2 小时），可考虑：
- **改写 server.py 用 tflite + OpenCV Haar**（避开 tensorflow 572MB 依赖）
  - `emotion_model_quantized.tflite`（92KB）+ `haarcascade_frontalface_default.xml`（1.2MB）都在 fer 包 data/ 里
  - 需要重写 inference 逻辑：tflite 跑 emotion + OpenCV Haar 跑 face detection
  - 镜像大小从 12.1GB 降到 ~1GB
  - 已在 `/tmp/fer_app/fer_server.py` 写好原型（未测）

### 11.9 时间线总结

| 时间 | 进展 |
|---|---|
| 18:25 | XTTS 端到端跑通（TTS API 返回 222KB WAV）|
| 19:35 | 删除 yiminger/sensevoice；发现 emotion-echo-models/sensevoice-small/ 已有完整实现 |
| 20:00 | Stage 36 §5.1 重读：发现 emotion-echo/fer:v0.1.0 之前 build 成功过 12.1GB |
| 20:08-20:30 | FER build 试 1：原始 Dockerfile，pypi.org SSL EOF 失败 |
| 20:30-20:45 | FER build 试 2：加 `--build-arg PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple`，openCV 254s 后报 hash 错 |
| 20:45-20:55 | 手动从 aliyun pypi mirror 下 opencv manylinux2014_x86_64 wheel（56.5MB / 61.2MB，文件大小有出入，hash 待验证） |
| 20:55-21:15 | FER build 试 3：patched Dockerfile（aliyun mirror + 本地 wheel + find-links），opencv 下完（9 分钟），开始下 tensorflow 572MB |
| 21:15-21:20 | 用户结束当日工作，build 在 tensorflow 下载阶段被 kill |

### 11.10 调研依据

| 项 | 来源 |
|---|---|
| Stage 36 v0.1.0 build 成功 12.1GB | `docs/stages/stage-36-landing.md:268-271` |
| FER Dockerfile 当前内容 | `emotion-echo-models/FER/Dockerfile`（未修改）|
| `fer` 包依赖 tensorflow>=2.0.0 | `/tmp/fer_src/setup.py` INSTALL_REQUIRES |
| `emotion_net.caffemodel` 170MB 缺失 | `docs/ai-models/build-guide.md §5.1` |
| `libGL.so.1` 缺失导致三连 fallback | `docs/ai-models/build-guide.md §5.1` |
| 清华 mirror 大包 hash 错 | 实测 254s 时 `opencv_python_headless-5.0.0.93-cp37-abi3-manylinux_2_28_x86_64.whl` sha256 mismatch |
| aliyun mirror 正确 URL | 实测 `manylinux2014_x86_64.manylinux_2_17_x86_64` 是 `python:3.10-slim` 真正用的 wheel |
| TFLite 替代方案 92KB 模型 | `/tmp/fer_src/src/fer/data/emotion_model_quantized.tflite`（已下到 host） |

---

## 十二、FER build 续战：25/29 wheels 就位（2026-09-09 21:20-21:58）

### 12.1 进展摘要

**当晚后半段**（用户说"再等一下，build 别停"后）：

- 21:20-21:28：tensorflow 2.21.0（572MB）**完整下完**（10 分钟，700-1000KB/s，宿主机关梯子后从 aliyun 直拉）
- 21:30-21:35：改 Dockerfile 用 `pip install --no-index --find-links /wheels/` 纯本地装
- 21:38-21:43：8 个小包 wheels 下完（fastapi/uvicorn/python-multipart/requests/numpy/fer/prometheus-client/python-json-logger）
- 21:43-21:53：连续 5 次 build，每次补一个传递依赖：
  - 第 1 次：缺 pydantic → 下 9 个传递依赖
  - 第 2 次：缺 annotated-doc → 下 5.3KB
  - 第 3 次：缺 typing-inspection → 下 4.4KB
  - 第 4 次：缺 urllib3 → 下 14 个传递依赖（urllib3/certifi/charset_normalizer/idna/exceptiongroup 等）
  - **第 5 次：缺 pandas**——build 失败
- 21:58：用户结束当日工作

### 12.2 当前 `/wheels/` 状态

**25 个 wheel，总 ~670MB**：

```
annotated_doc-0.8.0-py3-none-any.whl (5KB)
annotated_types-0.8.0-py3-none-any.whl (13KB)
anyio-4.15.1-py3-none-any.whl (132KB)
certifi-2026.7.22-py3-none-any.whl (137KB)
charset_normalizer-3.5.1-...manylinux.whl (262KB)
click-8.5.0-py3-none-any.whl (125KB)
exceptiongroup-1.3.1-py3-none-any.whl
fastapi-0.141.1-py3-none-any.whl (132KB)
fer-25.10.3-py3-none-any.whl (891KB)
h11-0.16.0-py3-none-any.whl (37KB)
idna-3.19-py3-none-any.whl (69KB)
numpy-2.2.6-...manylinux.whl (16.8MB)
opencv_python_headless-5.0.0.93-...manylinux_2_17_x86_64.whl (56.5MB，**期望 61.2MB，待 hash 验证**)
prometheus_client-0.26.0-py3-none-any.whl (64KB)
pydantic-2.13.5-py3-none-any.whl (473KB)
pydantic_core-2.48.0-...manylinux.whl (2.1MB)
python_json_logger-4.2.0-py3-none-any.whl (15KB)
python_multipart-0.0.32-py3-none-any.whl (30KB)
requests-2.34.2-py3-none-any.whl (73KB)
sniffio-1.3.1-py3-none-any.whl (10KB)
starlette-1.6.0-py3-none-any.whl (76KB)
tensorflow-2.21.0-...manylinux_2_27_x86_64.whl (572MB)
typing_extensions-4.16.0-py3-none-any.whl (46KB)
typing_inspection-0.4.4-py3-none-any.whl
urllib3-2.7.0-py3-none-any.whl (131KB)
uvicorn-0.52.4-py3-none-any.whl (80KB)
```

**还差 4 个 fer 包传递依赖**：pandas / matplotlib（两个中等大小包，可能还有 ndtri/kiwisolver 等更深的传递依赖）

### 12.3 明早继续 build 的命令（直接 copy-paste 即可）

```bash
# 1. 补 pandas + matplotlib wheels（容器内 pip download）
id=$(docker run -d -v "C:/Users/LENVOV/AppData/Local/Temp/fer_build_ctx/wheels:/wheels" python:3.10-slim sleep 600)
docker exec $id sh -c "
pip install --quiet --index-url https://mirrors.aliyun.com/pypi/simple 'fer' 2>&1 | tail -3
pip download --no-deps --dest /wheels/ \
  --index-url https://mirrors.aliyun.com/pypi/simple \
  'pandas' 'matplotlib' 'kiwisolver' 'pyparsing' 'cycler' 'fonttools' 'pillow' 2>&1 | tail -5
"
docker stop $id; docker rm $id

# 2. 检查文件大小
ls -la /tmp/fer_build_ctx/wheels/ | grep -E "opencv|pandas|matplotlib" | head -5
# opencv 期望 61.2MB，目前 56.5MB，hash 待验证
# 如果不是 61.2MB，重新下：
# curl -L -o /tmp/fer_build_ctx/opencv_python_headless-5.0.0.93-cp37-abi3-manylinux2014_x86_64.manylinux_2_17_x86_64.whl \
#   https://mirrors.aliyun.com/pypi/packages/2b/97/8170e9819764c47e436c130d3ff6cfb73b58f923eae9d3a03d8982b04aec/opencv_python_headless-5.0.0.93-cp37-abi3-manylinux2014_x86_64.manylinux_2_17_x86_64.whl

# 3. 重新 build
cd /tmp/fer_build_ctx
docker build -t emotion-echo/fer:v0.1.0-mirror .

# 4. 启动验证
docker run -d -p 8004:8004 emotion-echo/fer:v0.1.0-mirror
sleep 60
curl -fsS http://localhost:8004/health
# 预期：{"status":"ok","model_loaded":true/false,"backend":"fer"|"opencv-dnn"|"neutral-fallback"}
# Stage 36 §5.1 实测 12.1GB 镜像 build 成功后 /health 报 model_loaded=false + neutral-fallback
# （缺 emotion_net.caffemodel 170MB + libGL.so.1）—— Stage 22 起的已知问题

curl -fsS -X POST http://localhost:8004/analyze -F file=@test.png
# 预期：{"emotion":"neutral","confidence":0.5,...}（三连 fallback）
```

### 12.4 备选（如果 12.3 走完还卡）

如果 12.3 build 通了但 /health 报 `model_loaded=false`（预期内，因为缺 emotion_net.caffemodel），而且 `/analyze` 全返 neutral：

- 走**改写 server.py** 路径（§11.8 备选方案）：
  - 用 tflite-runtime（~1MB）替代 tensorflow（572MB）
  - 用 emotion_model_quantized.tflite（92KB）替代 emotion_model.hdf5
  - 用 OpenCV Haar Cascade（OpenCV 自带）替代 fer Python 库
  - 镜像大小从 12.1GB 降到 ~1GB
  - 已在 `/tmp/fer_app/fer_server.py` 写好原型（**未测**）
  - 预计明早 1-2 小时可完成

### 12.5 进度图

```
[21:20] wheels: 0
[21:28] wheels: 0+tensorflow  ← 后台完成
[21:38] wheels: 8
[21:43] wheels: 8+9=17
[21:43] wheels: 17+1=18 (annotated-doc)
[21:51] wheels: 18+1=19 (typing-inspection)
[21:53] wheels: 19+14=25 (含 urllib3 等)
[21:55] wheels: 25 ← build 第 5 次失败，缺 pandas
[22:00] 用户结束工作
[明天]  + pandas + matplotlib + ... → 期望 30+ wheels → build 通过
```

### 12.6 调研依据

| 项 | 来源 |
|---|---|
| tensorflow 完整下完 572MB | 实测 21:28 完成 |
| aliyun mirror 速率 700-1000KB/s | 实测多轮 |
| fer 依赖 pandas/matplotlib | `/tmp/fer_src/setup.py` INSTALL_REQUIRES |
| 5 次 build 缺依赖的循环 | 实测 21:43-21:55 连续 5 次 |

---

> 最后更新：2026-09-09 21:58 by Stage 59 持续 session
> 用途：完整记录 XTTS / SenseVoice / FER 三个 AI 容器的 vendor 调研 + 自建 build 尝试过程
> 下次起点：§十二.3 命令（补 pandas/matplotlib + 重新 build）
