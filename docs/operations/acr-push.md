# ACR 镜像推送与发布流程

> **范围**：本项目 SenseVoice 镜像（`sensevoice-base` + `sensevoice`）推到阿里云 ACR 个人版的完整工作流。
> **关联**：[`scripts/push-to-acr.sh`](../../scripts/push-to-acr.sh) ·
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

## 七、仓库总览

私有 namespace `emotion-echo` 下当前4 个镜像仓库：

| 仓库 | 来源 | 内容大小 | 备注 |
|---|---|---|---|
| `sensevoice-base` | 我们（built） | 502MB | Python + torch + funasr + knf + transformers |
| `sensevoice` | 我们（built on top of base） | 1.37GB | 业务代码 + 936MB 模型 |
| `fer-tflite` | 我们（built） | 134MB | tflite + Haar cascade 后端，无 tensorflow |
| `xtts` | vendor ai4all/coqui 重 tag | 3.69GB | Coqui TTS server，已固化 command override |

**总占 ACR 存储**：~5.7GB（不含 docker.io 缓存层）。ACR 个人版免费档够用。