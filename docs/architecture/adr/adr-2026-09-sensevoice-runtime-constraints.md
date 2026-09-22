---
adr: 2026-09-sensevoice-runtime-constraints
title: SenseVoice 模型服务的运行时约束与镜像分发策略
status: accepted
date: 2026-09-22
owners: [ai-svc, emotion-echo-models, deploy]
references: [E2E-16, E2E-F-106, E2E-F-111, E2E-F-112]
supersedes: null
---

# ADR-2026-09 · SenseVoice 模型服务的运行时约束与镜像分发策略

## 上下文（Context）

E2E-16（多模态）开工时，语音输入链路**完全不可用**，排查后确认是**五个互相独立的缺陷叠在一起**，
每一个单独都能让"语音上传"失败，且失败方式都是**静默或误报**（容器报 healthy、BFF 报 503、
前端什么都不显示）。逐条都有实测证据：

| # | 缺陷 | 证据 |
|---|------|------|
| 1 | **torch 与 funasr 版本漂移**：`requirements.txt` 写 `torch==1.13.1+cpu` + `funasr>=1.1.2`（不锁上界）⇒ pip 拉到 funasr 1.4.15（要求 torch≥2.1） | 容器自报 `Disabling PyTorch because PyTorch >= 2.1 is required but found 1.13.1+cpu` + `Models won't be available` |
| 2 | **VAD 模型未烘焙**，在**请求路径**里从 ModelScope 下载（compose 注释自述） | 首次 `/analyze` 先下载再推理，超 ai-svc 的 30s 超时 |
| 3 | **懒加载无预热**：server.py 自述 "first request 30-60s"，而 ai-svc `SenseVoice.Timeout: 30` | 冷启动首个请求**必然**超时 |
| 4 | **healthcheck 只看 HTTP 200**，不看模型是否真加载 | 实测容器 `Up (healthy)` 但每次 `/analyze` 都失败；`depends_on: service_healthy` 全为假绿 |
| 5 | **runtime 阶段缺 ffmpeg**（只在被丢弃的 builder 装了） | 浏览器录的 `webm/opus` 无法解码：`500 inference failed: [Errno 2] No such file or directory: 'ffmpeg'` |

另有一条**部署层约束**（E2E-F-112）：

| 6 | **容器内存限额按旧 torch 估的**（`memory: 1536M  # funasr + torch ~1GB + room`） | 升到 torch 2.1 后峰值超限 ⇒ 被 cgroup 反复杀死（`RestartCount` 每 ~11s 累加，日志永远停在 `ckpt: /app/model/model.pt`）；**同一镜像 `docker run` 无限制则正常** ⇒ 差异 100% 来自限额（实测峰值 **2.622GiB**） |

**为什么长期没被发现**：所有既有验证都走 **WAV + 直接调端点**（torchaudio 经 libsndfile 可解码、
不经浏览器录音状态机），于是 1/3/4/5 全部被绕过；而 2 被 `sensevoice-cache` volume 的"第二次就好了"
掩盖。**只有 IAB 浏览器实测（真实 webm + 真实前端录音路径）才同时暴露它们。**

## 决策（Decisions）

### §A. torch 与 funasr 必须**成对锁定**，wheels 预烘焙

`requirements.txt`：`torch==2.1.0+cpu` + `funasr==1.4.15`（与 `wheels/` 内实际版本一致）。
**升级路径写进注释**：先升 torch（要求 funasr 也升）→ 再升 funasr；**不允许只升一方**。
理由：freelist 式版本范围（`funasr>=1.1.2`）会让镜像构建随时间不可重现——这正是漂移的成因。

### §B. 模型与 VAD 全部**烘焙进镜像**，禁止请求期下载

`fsmn-vad` 四件套（`model.pt` 1.7MB / `am.mvn` / `config.yaml` / `configuration.json`）入库到
`vad_model/`，Dockerfile 烘焙到 `/app/model/vad`，`server.py` 优先用本地目录
（`FUNASR_VAD_DIR`，不存在才回退 hub 名称，兼容未重建的旧镜像）。
理由：**请求路径里做网络下载**与"服务"的语义冲突——它把外部可用性耦合进了接口延迟。

### §C. **启动预热**（而非懒加载）

`@app.on_event("startup")` 加载模型；失败**不 crash-loop**（记 ERROR，`/health` 保持
`model_loaded=false`，由 healthcheck 判 unhealthy）。
理由：懒加载 + 客户端 30s 超时 = 首个请求必然失败，是设计层面的错误而非调优问题。

### §D. healthcheck 必须校验**真实就绪**，不得只看 HTTP 200

healthcheck 断言 `model_loaded == true`；`start_period` 180s（预热约 25-35s + 余量）。
理由：只看 200 的健康检查给出的是**假绿**，会让 `depends_on: service_healthy`、
运维重启策略、以及一切"健康即就绪"的推断同时失效。同型问题在 `user-svc` 也存在
（`repository not initialized (degraded start)` 时仍报 healthy，见 E2E-F-96）。

### §E. runtime 必须含 **ffmpeg**

runtime 阶段 apt 加 `ffmpeg`（原本只在 builder 有）。
理由：浏览器 `MediaRecorder` 的实际产物是 `audio/webm;codecs=opus`，torchaudio 解不了 webm。
**这是"接口契约"问题**：服务的输入格式必须覆盖调用方的真实产物，而不是测试夹具的格式。

### §F. 容器内存限额按**模型峰值**定，并计入宿主 WSL 预算

compose sensevoice `memory: 3072M`（原 1536M）。实测模型加载后 **2.622GiB / 3GiB（87%）**。
并且：宿主 `.wslconfig` 限 WSL2 至 6GB 时，`sw-oap` 一个就吃 2GB ⇒ 加载 893MB 模型会 OOM
拖垮 Docker Desktop（表现为"一测这个镜像 docker 就出问题"）。**全栈 + 该模型不适合放在 6GB 里**。

### §G. ACR 双层分发：app 层构建**必须传 `BASE_TAG`**

`sendvoice-base` / `sensevoice` 双层同 tag；`push-to-acr.sh` 的 app 构建补
`--build-arg BASE_TAG="${TAG}"`。
理由：`Dockerfile.app` 的 `FROM` 默认 `v0.1.0`，不传参时会**用旧 base 构建新版本 tag**——
看似升级、实则没升（本次若不修，`v0.1.1` 会挂在旧 base 上，白修）。

## 后果（Consequences）

**正面**
- 语音链路端到端可用（实测：`/analyze` 0.84s 返回 transcript；`/voice/upload` 200 + audioUrl 匿名可读；
  IAB 浏览器实测语音气泡渲染 + `<audio src>` 指向 MinIO 实名对象）
- 健康检查恢复语义（healthy ⟺ 模型就绪），`depends_on` / 重启策略重新可信
- 镜像构建可重现（版本成对锁定）

**负面 / 代价**
- 镜像更大（`sensevoice:v0.1.1` 含 ffmpeg + VAD + 893MB 模型）
- 稳态内存占用 **2.6GB**，全栈在 6GB WSL 下放不下 ⇒ 需要宿主放宽 `.wslconfig` 或错峰运行
  （本次实测：停掉 `skywalking-oap/ui` 后可用内存 1.1GB → 3.2GB）
- torch 升级会牵动 funasr（成对升级约束），升级窗口比单组件更窄

**遗留（不在本 ADR 范围）**
- `push-to-acr.sh` 的登录检查在 Windows/Git Bash 下误报（`${HOME}` 注入 Windows Python），
  本次以等价 buildx 命令绕过，脚本修法待下一轮
- `user-svc` 同型的"降级启动仍报 healthy"（E2E-F-96）、BFF 的 Nacos/gRPC 连接不重解析
  （E2E-F-107）—— 均属"健康与连接语义"家族，归各自阶段
