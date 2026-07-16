# Stage 25-A · AI Profile 端到端（部分完成 / 阻塞）

**日期**：2026-07-16
**状态**：⏸ 部分完成（建议拆为 5a + 5b 两个 PR）

---

## 一、已完成（Stage 25-A 代码层）

### 1. verify_stage23_endpoints.py 增强
- 区分 **AI profile LIVE** / **AI profile OFFLINE（降级）** 两种模式
- AI 在线时正确计数（pass 4/4），离线时 TTS 503 视为预期降级
- 输出末尾标注当前模式

### 2. deploy/docker-compose.apps.yml 路径修复
- SenseVoice `build.context`: `../../Emotion-Echo-LLM` → `../Emotion-Echo-LLM`
- 与 FER / XTTS 一致（deploy/ 是 Emotion-Echo 的子目录）

---

## 二、阻塞的两件事

### 阻塞 1：SenseVoice 缺 Dockerfile + server.py

`Emotion-Echo-LLM/sensevoice-small/` 目录**只有**：
- `demo.py`：CLI 脚本（funasr AutoModel 跑本地文件）
- `requirements.txt`、`model.pt (936MB)`
- ❌ **没有** Dockerfile
- ❌ **没有** server.py（HTTP server）

要启用 SenseVoice 容器，必须**先实现**：
- `server.py`：FastAPI + POST `/analyze`（接收音频字节，调 funasr 模型，返回 emotion + text）
- `Dockerfile`：参考 `FER/Dockerfile` 的多阶段 build 模式

这是 Stage 25-A **范围之外**的工作（Stage 22-A 假设 server.py 已存在）。

### 阻塞 2：Docker build 网络超时

```
target emotion-echo-xtts: failed to solve:
failed to fetch anonymous token: connectex
```

拉取 docker.io base image 超时。本地没有 `emotion-echo/fer` 或 `emotion-echo/xtts` 镜像可重用。

可能的原因：
- 当前网络无法访问 `auth.docker.io`
- Docker Hub 速率限制
- 防火墙 / 代理设置

**临时绕过**：可配 Docker Hub 国内镜像（如 `registry.cn-hangzhou.aliyuncs.com`），但需要：
- 用户在 `~/.docker/daemon.json` 配置 registry mirror
- 或用 `docker buildx` 改用其他 registry

---

## 三、Stage 25-A 实际效果（AI offline 状态）

跑 `python scripts/verify_stage23_endpoints.py --ai-svc http://localhost:8891`：

```
=== Summary: 3/4 Stage 23 endpoints healthy [AI profile OFFLINE (降级)] ===
```

3 个 endpoint 健康，1 个（XTTS）降级返 503。这是预期的「降级路径」，不是回归。

---

## 四、Stage 25-B 后续 PR 计划

### PR 1：写 SenseVoice server.py
- 在 `Emotion-Echo-LLM/sensevoice-small/server.py` 实现 FastAPI HTTP server
- POST `/analyze` 接收 multipart audio → 调 funasr → 返回 `{"emotion": "happy", "text": "...", "confidence": 0.9}`
- POST `/health` 健康检查

### PR 2：写 SenseVoice Dockerfile
- 多阶段 build（参考 `FER/Dockerfile`）
- Base: `python:3.10-slim`
- COPY: server.py + requirements.txt + model.pt（仅 COPY，模型忽略看实际情况）

### PR 3：配 Docker registry mirror（解决网络问题）
- 用户配 `~/.docker/daemon.json`：
  ```json
  {
    "registry-mirrors": ["https://registry.cn-hangzhou.aliyuncs.com"]
  }
  ```
- 重启 docker daemon

### PR 4：跑 AI profile 端到端
- `docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --profile ai up -d --build`
- 等待模型加载（FER 60s / SenseVoice 120s / XTTS 180s）
- 跑 `verify_stage23_endpoints.py`，期望 4/4 healthy + `[AI profile LIVE]`

---

## 五、本次 commit 文件清单

| 文件 | 改动 |
|------|------|
| `scripts/verify_stage23_endpoints.py` | 增强：AI live/offline 模式区分 |
| `deploy/docker-compose.apps.yml` | 修复 SenseVoice context 路径 |
| `docs/stage-25-ai-profile-partial.md` | 本文档：阻塞 + 后续计划 |