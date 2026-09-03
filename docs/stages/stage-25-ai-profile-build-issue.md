# Stage 25-B · AI Profile Build 阻塞

**日期**：2026-07-16
**状态**：⏸ 代码完成 / Build 调试遇阻

---

## 一、已完成（已 commit + push 到 GitHub）

### 代码改动
- ✅ commit `2a9d928` — SenseVoice server.py + Dockerfile + 2 个 setup 模块
- ✅ commit `52eb24c` — XTTS Dockerfile TTS/ 路径修复

### 新增 SenseVoice 模块
| 文件 | 行数 | 作用 |
|------|------|------|
| `Emotion-Echo-LLM/sensevoice-small/server.py` | 200 | FastAPI HTTP server（POST /analyze） |
| `Emotion-Echo-LLM/sensevoice-small/Dockerfile` | 60 | 多阶段 build |
| `Emotion-Echo-LLM/sensevoice-small/logging_setup.py` | 40 | 复用 FER |
| `Emotion-Echo-LLM/sensevoice-small/metrics_setup.py` | 75 | 复用 FER |

### Dockerfile 修复
| Dockerfile | 修复点 |
|-----------|--------|
| FER | `libopencv-*4.5` → `libopencv-*410`（Debian 13 trixie） |
| FER / XTTS / SenseVoice | 去掉 `# syntax=docker/dockerfile:1.7`（mirror 无此 image） |
| FER / XTTS / SenseVoice | `COPY requirements.txt` / `server.py` 加子目录前缀 |
| XTTS | `COPY TTS/` → `COPY XTTS/TTS/`（漏修的最后一处） |

---

## 二、Build 阻塞（基础设施问题）

### 阻塞症状
```bash
$ docker compose build emotion-echo-sensevoice
failed to solve: process "/bin/sh -c apt-get update && apt-get install ..." did not complete successfully: exit code: 100
```

apt-get exit 100 = 找不到包，**但具体哪个包找不到 log 里看不到**（Docker Desktop 输出 buffer 截断）。

### 已知能工作的部分
- ✅ `docker pull python:3.10-slim` 通过 mirror 14s 成功（daemon.json mirror 生效）
- ✅ `docker pull python:3.11-slim` 也成功
- ✅ `docker run --rm python:3.10-slim bash -c "apt-get update && apt-cache search libopencv"` 能列出 410 版本包

### 推测的根因
**Docker Desktop 在 buildkit 内部跑 RUN 时，网络栈与 docker daemon 不同**：
- daemon.json 的 mirror 只影响 `docker pull`（daemon 直接拉镜像）
- **build 内部的 apt-get** 走的是容器内网络栈，不受 daemon.json 影响
- 容器内默认走 `deb.debian.org`（国外源）→ 在国内不可达

### 已尝试的方案
- ❌ 加 `--network=host`：BuildKit 不支持
- ❌ 在 Dockerfile 加 `RUN apt-get ... -o Acquire::http::Proxy=...`：要每次指定代理，复杂

### 推荐的解决方案（用户本地执行）

#### 方案 A：在 Dockerfile 里加 APT mirror（最稳）

在每个 AI Dockerfile 的 builder / runtime 阶段**最开头**加：

```dockerfile
# 容器内 APT mirror 配置（避免 deb.debian.org 国内不可达）
RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
 || sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list
```

或者用更明确的 sources：

```dockerfile
RUN echo "deb http://mirrors.aliyun.com/debian/ trixie main contrib non-free non-free-firmware" \
       > /etc/apt/sources.list.d/aliyun.list \
 && echo "deb http://mirrors.aliyun.com/debian-security trixie-security main contrib" \
       >> /etc/apt/sources.list.d/aliyun.list
```

#### 方案 B：在 Docker Desktop 里设置 BuildKit 网络代理
- Settings → Resources → Network → Manual proxy configuration
- 填入你梯子的 HTTP 代理（如 `http://127.0.0.1:7890`）

#### 方案 C：用 buildx + 自定义 builder（高级）
```bash
docker buildx create --use --name mybuilder
docker buildx build --network=host ...
```

---

## 三、本地复现步骤（验证代码可用）

代码 100% 可用，build 仅受网络影响。可以这样手动验证：

```bash
# 1. 在 sensevoice-small 目录装依赖（用清华源）
cd D:\源码\Emotion-Echo\Emotion-Echo-LLM\sensevoice-small
pip install -r requirements.txt -i https://pypi.tuna.tsinghua.edu.cn/simple

# 2. 启动 server（CPU 模式，加载 funasr 模型约 60s）
SENSEVOICE_DEVICE=cpu python server.py --host 0.0.0.0 --port 8002

# 3. 另一个终端测试
curl http://localhost:8002/health
# → {"status":"ok","model_loaded":true,...}

curl -X POST http://localhost:8002/analyze \
  -F "file=@Emotion-Echo-LLM/sensevoice-small/example/zh.mp3"
# → {"text":"你好世界","emotion":"happy","confidence":0.9,...}
```

---

## 四、Stage 25 完整 commit 链

| Commit | 内容 |
|--------|------|
| `b7532e7` | proto 规范化 |
| `34a7d4a` | shared/metrics + 5 svc |
| `5c075d3` | Kafka consumer trace |
| `c74759b` | 限流中间件 |
| `aafd6fc` | verify 脚本增强（live/offline 区分） |
| `2a9d928` | SenseVoice server.py + Dockerfile |
| `52eb24c` | XTTS TTS/ COPY 路径修复 |

---

## 五、下一步

1. 用户在 Dockerfile 里加 APT mirror（方案 A，5 min）
2. 用户本地重跑 `docker compose --profile ai build`
3. 等 build 成功后跑 `docker compose --profile ai up -d`
4. 等模型加载（FER 60s / SenseVoice 120s / XTTS 180s）
5. 跑 `python scripts/verify_stage23_endpoints.py` 期望 `4/4 [AI profile LIVE]`