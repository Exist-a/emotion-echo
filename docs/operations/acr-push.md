# ACR 镜像推送与发布流程

> **⛔ 部分已弃用 / 仍可参考**：本文档记录的是把 3 个 AI 镜像推到阿里云 ACR 个人版的实验记录。
>
> **当前结论**（2026-09-11 commit `8702615`）：
>
> | 镜像 | ACR 状态 | 实际使用 |
> |---|---|---|
> | `sensevoice-base` / `sensevoice` / `fer-tflite` | ✅ 已推 ACR（§一 ~ §五流程仍有效） | compose 拉 ACR（§五.1） |
> | XTTS vendor `ai4all/coqui` | ❌ 未推 ACR（§7.1） | docker.io pull（aliyun 加速） |
> | SV-fastbuild `wheels/` | ❌ 未推 ACR（§7.2）+ ❌ 未进 git（filter-repo） | 本地预下（`scripts/download-sv-wheels.sh`）|
>
> 实施指南见 [`emotion-echo-models/README.md`](../../emotion-echo-models/README.md)。
> SV-fastbuild wheels 本地管理流程见 [§7.2](#72-sv-fastbuild-wheels本地管理不在-git-不在-acr)。
>
> ---
> 范围（历史）：本项目 SenseVoice 镜像（`sensevoice-base` + `sensevoice`）推到阿里云 ACR 个人版的完整工作流。
> 关联：[`scripts/push-to-acr.sh`](../../scripts/push-to-acr.sh) ·
> [`scripts/download-sv-wheels.sh`](../../scripts/download-sv-wheels.sh) ·
> [`docs/stages/stage-60-1-pr-tts-vendor-sv-landing.md`](../../stages/stage-60-1-pr-tts-vendor-sv-landing.md)

## 一、架构：为什么拆 base + app 两层

| 层 | 包含 | 大小 | 何时重建 |
|---|---|---|---|
| **`sensevoice-base`** | python:3.10-slim + torch+cpu + funasr + kaldi-native-fbank + transformers + prometheus-client + fastapi + uvicorn + ... | ~500 MB | Python 依赖变更（罕见） |
| **`sensevoice`** | 在 base 上叠加：业务代码（server.py 等）+ 936 MB SenseVoiceSmall 模型 + 配置 | ~1.4 GB | 代码改动 / 模型改动 |

**收益**：日常只重建应用层（秒级），base 一次推完几个月不动。

## 二、推送前置：每个开发者手动做一次

### 2.1 一次性：在 ACR 控制台创建命名空间 + 仓库

进入 ACR 个人版实例，创建：

| 命名空间 | 仓库 | 访问权限 |
|---|---|---|
| `emotion-echo` | `sensevoice-base` | 私有 |
| `emotion-echo` | `sensevoice` | 私有 |

仓库**必须先建**，否则 docker push 会报 `insufficient_scope`。

### 2.2 一次性：在 ACR 控制台设置 Registry 登录密码

控制台 → 个人版实例 → **访问凭证** → **设置 Registry 登录密码**（首次需要）。

> ⚠️ 这条密码**和阿里云账号登录密码无关**，是 ACR 单独的凭证。忘了就重置（重置会让所有现有凭证失效）。

### 2.3 一次性：在本机 docker login

在**你的真终端**（不是 IDE 集成终端——后者可能是 non-TTY）执行：

```bash
echo "$ACR_PASSWORD" | docker login \
  crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com \
  -u YOUR_USERNAME \
  --password-stdin
```

成功后 docker daemon 会把凭证存到 `~/.docker/config.json` 的 auths 段（base64，不是明文，但请**仍视作敏感**）。

> 💡 **凭证传递规范**：密码**不**出现在 shell history、commit、文档、对话、本仓库脚本里。任何需要密码的地方都由用户在自己终端里 `--password-stdin` 注入，daemon 接管后其余流程自动带凭证。

### 2.4 验证 login 状态

```bash
python -c "
import json
c = json.load(open('$HOME/.docker/config.json'))
target = 'crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com'
print('logged in' if target in c.get('auths', {}) else 'NOT logged in')
"
```

应输出 `logged in`。

## 三、推送：日常一条命令

```bash
./scripts/push-to-acr.sh v0.1.0          # 推 base + app 两层
./scripts/push-to-acr.sh --base-only v0.2.0  # 只重建 base
./scripts/push-to-acr.sh --app-only        # 只重建 app（默认 tag）
./scripts/push-to-acr.sh --skip-push       # 只 build，不 push（本地调试）
```

脚本做的事：

1. **检查 login**：读 `~/.docker/config.json` 验证有目标 registry 的 auth 条目，没有则报错并提示如何 login
2. **build + push base**：用 `buildx build` + `--provenance=false` + `--output type=image,oci-mediatypes=false`（**绕过 ACR 个人版对 OCI manifest 的不支持**，详见 §四）
3. **build + push app**：依赖上一步的 base tag

预计耗时：base 3-5 分钟（首次含 wheel 下载），应用层 ~10 分钟（含 1.4GB 推送）。

## 四、坑：ACR 个人版 + docker buildx OCI manifest 不兼容

### 4.1 现象

```bash
$ docker push registry.../sensevoice-base:v0.1.0
33fd700e5760: Layer already exists
...
error from registry: unknown manifest class for application/vnd.oci.empty.v1+json
```

### 4.2 根因

Docker 26.x 起 buildx 默认输出 OCI 格式 manifest（`application/vnd.oci.image.manifest.v1+json` + 配套的 `application/vnd.oci.empty.v1+json` 空清单）。

ACR 个人版**只支持 Docker schema 2 manifest**（`application/vnd.docker.distribution.manifest.v2+json`），不支持 OCI 空清单。

### 4.3 修复（已写入 `scripts/push-to-acr.sh`）

```bash
docker buildx build \
  --provenance=false \                              # 关掉 attestation
  --output type=image,oci-mediatypes=false \        # 强制 schema 2 manifest
  ...
```

两个 flag **缺一不可**：
- 只加 `oci-mediatypes=false` → `cannot export attestations with "oci-mediatypes=false"`
- 只加 `--provenance=false` → 仍可能产 OCI manifest

### 4.4 公开渠道是否有解

我搜了 ACR + buildx + OCI manifest 相关 issue/blog，**没有公开的 ACR 个人版 + buildx OCI manifest 兼容性问题讨论**。最可能的解释：阿里云 ACR EE（企业版）才完整支持 OCI 规范，个人版默认只做 schema 2。

如果未来升级到 ACR EE，可移除这两个 flag。

## 五、消费：从 ACR 拉镜像（compose / 手动）

### 5.1 compose 当前配置（已切到 ACR）

[`deploy/docker-compose.apps.yml`](../../deploy/docker-compose.apps.yml) 三个 AI 服务 image 已切到 ACR：

```yaml
emotion-echo-sensevoice:
  image: crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com/emotion-echo/sensevoice:v0.1.0

emotion-echo-fer:
  image: crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com/emotion-echo/fer-tflite:v0.1.0

emotion-echo-xtts:
  image: crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com/emotion-echo/xtts:v0.1.0
  command: ["fastapi", "run", "app.py", "--host=0.0.0.0", "--port=8003"]
  volumes:
    - ${XTTS_MODEL_PATH:-/c/Users/LENVOV/AppData/Local/Temp/coqui_model}:/model
```

`docker compose --profile ai up -d` 会自动 pull。

### 5.2 手动拉

```bash
docker pull crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com/emotion-echo/sensevoice:v0.1.0
docker pull crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com/emotion-echo/fer-tflite:v0.1.0
docker pull crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com/emotion-echo/xtts:v0.1.0
```

### 5.3 切换回本地 build

如果将来 ACR 出问题需要回退，把 compose 那段 `image:` 改回 Phase 5 的 build 块：

```yaml
build:
  context: ../emotion-echo-models
  dockerfile: SV-fastbuild/Dockerfile
image: emotion-echo/sensevoice-fastbuild:v0.1.0
```

（这条已在 compose 文件的注释里写明。）

## 六、变更日志

| 日期 | 变更 | 作者 |
|---|---|---|
| 2026-09-10 | 初版：base + app 两层结构 + OCI manifest workaround | Stage 60.1 session |
| 2026-09-10 | FER-tflite 镜像推到 ACR；compose emotion-echo-fer 切换到 ACR | Phase 6 followup |
| 2026-09-10 | XTTS vendor镜像推到 ACR（3.69GB）；compose emotion-echo-xtts 切换到 ACR | Phase 6 followup |
| 2026-09-11 | 顶部状态表修正（XTTS / SV-fastbuild 状态准确化）| commit 8702615 |
| 2026-09-11 | 新增 §7.2 SV-fastbuild wheels 本地管理 + scripts/download-sv-wheels.sh | commit 8702615 |
| 2026-09-11 | git-filter-repo 从历史删除 wheels/ + force-push；filter-repo 重写 675 commit SHA | commit 50f2a3d |

## 七、仓库总览

私有 namespace `emotion-echo` 下当前**3 个镜像仓库**：

| 仓库 | 来源 | 内容大小 | 备注 |
|---|---|---|---|
| `sensevoice-base` | 我们（built） | 502MB | Python + torch + funasr + knf + transformers |
| `sensevoice` | 我们（built on top of base） | 1.37GB | 业务代码 + 936MB 模型 |
| `fer-tflite` | 我们（built） | 134MB | tflite + Haar cascade 后端，无 tensorflow |

**总占 ACR 存储**：~2GB。

### 7.1 XTTS 为什么不在 ACR

XTTS vendor `ai4all/coqui:latest`（3.69GB）**未上 ACR**，仍走 docker.io pull（aliyun 加速）。

**已知失败模式**（2026-09-10 实测5 次）：

1. 直接 `docker push` → 卡在 `pushing layers`，ACR 端无 manifest
2. `docker buildx build` 加 `--provenance=false --output type=image,oci-mediatypes=false` → `exporting manifest` 完成，但 `pushing layers` 仍卡
3. 重启 Docker daemon + 重试 → 同症状
4. kill 僵死进程多次 → daemon 内部 lock 不释放

**根因（2026-09-10 阿里云官方文档确认）**：
> 从2024 年04 月起，新创建的 **ACR 企业版**实例才支持 OCI 的 Image 和 Distribution 规范 v1.1.0。ACR **个人版不支持 OCI manifest**。

`ai4all/coqui` 是 buildkit 产物（Comment: `buildkit.dockerfile.v0`），其 layer blob 实际可能混用 docker schema 2 + OCI 格式。buildx 重导出能改 manifest type，但 layer 本身的兼容问题 ACR 个人版仍可能在某些 chunk 上拒绝 PATCH。

**未来可重试的场景**：
- 升级到 ACR 企业版（费用约 ¥100/月）
- 自建 `registry:2` 容器（个人版镜像）
- vendor 升级后再次尝试（ai4all/coqui 可能改用纯 schema 2）

**当前决策**：XTTS 走 docker.io，阶段 5（commit `8e09d03` 的早期版本）已验证 133KB WAV 端到端跑通，足以支撑 dev 模式使用。

### 7.2 SV-fastbuild wheels：本地管理（不在 git 不在 ACR）

SV-fastbuild 镜像构建依赖 6 个本地 `.whl`（合计 ~231 MB），其中 `torch-1.13.1+cpu-cp310-cp310-linux_x86_64.whl` 单文件 **190 MB**，超过 GitHub 单文件 100 MB 限制。

**历史**：commit `98e01ed` / `8e418a6` 把 wheels 直接 commit 进 git（commit 留言提 "ACR layer caching"，但实际未接 ACR 推送脚本 `scripts/push-to-acr.sh`）。后果是 push 被 GitHub 远端拒，错误 GH001: file exceeds 100.00 MB。

**当前状态（commit `8702615` 后）**：

| 存储位置 | 状态 |
|---|---|
| **GitHub 远端 main** | ❌ 无（filter-repo 从历史彻底清除，新 commit 只含 `.gitignore` + `.gitkeep`）|
| **ACR** | ❌ 无（`scripts/push-to-acr.sh` 未含 SV-fastbuild 块）|
| **本地 `emotion-echo-models/SV-fastbuild/wheels/`** | ✅ 6 个 .whl + .gitignore + .gitkeep |

**为什么不上 ACR**：

1. ACR 个人版不支持 OCI manifest，SV-fastbuild Dockerfile 是 buildkit 产物（`Dockerfile` line 2 注释提到 "fastbuild"），与 §7.1 XTTS 同源问题
2. ACR 1.4GB + 190MB torch wheel 单 layer 已超个人版 quota
3. ACR 设计是"运行时拉镜像"语义，wheels 是**构建期中间产物**（注释 `Dockerfile` line 25: "构建期中间产物，不进入 runtime 镜像"），不该作为独立镜像分发

**为什么不上 git**：

1. GitHub 单文件 100 MB 红线
2. 模型 + wheels 加起来 `emotion-echo-models/` ~5.9 GB，超过 LFS 免费 1 GB 配额
3. LFS 化能解决本次 push，但下一轮再加新大文件（如 `XTTS-v2/model.pth` 893MB）又会撞墙
4. 个人开发为主，单机构建 → **本地一份 wheels 文件 + build 时复用**最简单

**首次/重建流程**：

```bash
# 1. clone 后，wheels/ 目录里只有 .gitignore + .gitkeep，6 个 .whl 缺失
ls emotion-echo-models/SV-fastbuild/wheels/
# 输出: .gitignore  .gitkeep  （没 .whl）

# 2. 跑下载脚本（首次 ~3 分钟，主要时间在 torch 190MB）
bash scripts/download-sv-wheels.sh

# 3. 验证 wheels 完整
ls emotion-echo-models/SV-fastbuild/wheels/*.whl | wc -l
# 输出: 6

# 4. build SV-fastbuild 镜像（Dockerfile COPY wheels/ + --find-links 正常用）
docker build -t emotion-echo/sensevoice-fastbuild:test \
  -f emotion-echo-models/SV-fastbuild/Dockerfile \
  emotion-echo-models/SV-fastbuild/
```

**脚本设计要点（见 `scripts/download-sv-wheels.sh` 文件头注释）**：

- **不用 pip download**：pip 26+ 走 PyTorch CPU index 时 torch==1.13.1+cpu 目录已被 PyTorch 官方删除（403），但 S3 文件还在；pip 找不到候选版本 → 失败
- **curl 直拉 6 个 .whl**：torch 用 `download.pytorch.org/whl/cpu/<file>` 直链，其他 5 个用 `pypi.org/simple/<package>/` 解析优先选 linux x86_64
- **重试 + 大小校验**：torch 必须 >= 100MB，其他 >= 1KB，避免下载残缺
- **可切镜像**：`PYTORCH_INDEX` / `PIP_INDEX_URL` 环境变量覆盖

**何时需要重跑**：

- requirements.txt 变更（加/升/降依赖）
- 现有 wheel 文件损坏或缺失
- 切换 Python 版本（3.10 → 3.11 等）

**未来可重试的场景**：

- ACR 企业版（¥100/月）+ 加 SV-fastbuild 块到 `scripts/push-to-acr.sh`（避免每次本地预下）
- LFS 付费扩容（$5/月 50GB），全部 `emotion-echo-models/` 走 LFS
- vendor 升级（torch >= 2.x）后普通 PyPI simple 能解析，pip download 路径重新可用

**Refs**：
- `scripts/download-sv-wheels.sh`（端到端测试 funasr wheel byte-identical）
- commit `8702615` feat(scripts): SV-fastbuild 本地 wheels 下载脚本
- commit `50f2a3d` chore(models): SV-fastbuild/wheels/ 重新加 .gitignore + .gitkeep
- filter-repo 操作（force-push，~675 commit SHA 重写，详见 AGENTS.md 2026-09-11 session log）

调研依据 (AGENTS.md §〇):
- 读代码: SV-fastbuild/Dockerfile (COPY wheels/ + --find-links 硬依赖);
          scripts/push-to-acr.sh (无 SV-fastbuild 块);
          .gitattributes (无 .whl LFS 规则);
          `emotion-echo-models/` du -sh 实测 5.9GB;
          `torch==1.13.1+cpu` PyTorch 官方 index 目录 403 但 S3 文件 200
- 查 ADR: docs/architecture/decisions.md (无相关决策)
- 跑现状: git push GH001 失败 + filter-repo 重写 + curl 直链 199MB 下载成功
- 网上信息: download.pytorch.org/whl/cpu/ 老版本目录索引被删但 S3 文件保留