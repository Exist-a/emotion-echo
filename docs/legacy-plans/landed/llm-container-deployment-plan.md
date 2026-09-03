---
status: landed
superseded-by: stage-22-ai-services-containerization.md
original-path: .trae/documents/llm-container-deployment-plan.md
original-date: 2026-06-XX
migrated-at: 2026-09-03
round: 2-A
---

# LLM 模型容器化部署计划

## 1. 可行性分析

### 1.1 当前状态评估

| 模型服务 | 现有 Dockerfile | 服务端口 | 依赖状态 | 可行性 |
|---------|----------------|---------|---------|--------|
| **SenseVoice** | ✅ 已存在 | 8002 | 完整 | ✅ 高 |
| **XTTS-v2** | ✅ 已存在 | 8003 | 完整 | ✅ 高 |
| **FER** | ❌ 缺失 | 8004 | 完整 | ✅ 高 |

### 1.2 技术可行性

**已具备的条件：**
- ✅ 所有模型服务都有独立的 `server.py` 入口文件
- ✅ 所有服务都有 `requirements.txt` 依赖清单
- ✅ 所有服务都支持 HTTP API 接口
- ✅ 已有两个服务的 Dockerfile 参考

**潜在挑战：**
- XTTS-v2 需要 GPU 加速才能获得较好性能
- 首次启动需要下载较大的模型文件
- 需要确保网络能够访问 HuggingFace/ModelScope

### 1.3 资源要求

| 服务 | 内存要求 | GPU要求 | 存储要求 | 本地模型状态 |
|------|---------|---------|---------|-------------|
| SenseVoice | 4GB+ | 可选 | ~2GB | ❌ 需要下载 |
| XTTS-v2 | 8GB+ | 推荐 | ~2GB | ✅ 已下载 (1.93GB) |
| FER | 2GB+ | 不需要 | ~500MB | ❌ 需要下载 |

**关键发现：**
- XTTS-v2 模型已存在于本地 `AI-ModelScope/XTTS-v2` 目录，大小约 1.93GB
- 容器化时可以直接复制本地模型，无需重新下载

---

## 2. 部署计划

### 2.1 目标架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     Docker Compose 网络                         │
├────────────┬────────────┬────────────┬────────────┬────────────┤
│  PostgreSQL │   Redis    │  Go后端   │ SenseVoice │   XTTS-v2  │
│  :5432     │  :6379     │  :8080    │  :8002    │  :8003    │
└────────────┴────────────┴────────────┴────────────┴────────────┘
                                    │              │
                                    └──────────────┼────────────┘
                                                   ▼
                                          ┌─────────────┐
                                          │    FER     │
                                          │   :8004    │
                                          └─────────────┘
```

### 2.2 文件修改清单

| 序号 | 文件路径 | 修改内容 | 说明 |
|------|---------|---------|------|
| 1 | `Emotion-Echo-LLM/FER/Dockerfile` | 新建 | 创建 FER 的 Dockerfile |
| 2 | `Emotion-Echo-Gin/docker-compose.yml` | 修改 | 添加三个 LLM 服务 |
| 3 | `Emotion-Echo-Gin/config.yaml` | 修改 | 更新服务地址为容器名 |

### 2.3 步骤详细说明

#### 步骤 1：创建 FER Dockerfile

创建 `Emotion-Echo-LLM/FER/Dockerfile`：

```dockerfile
FROM python:3.10-slim

WORKDIR /app

# 安装系统依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    git \
    libopencv-dev \
    && rm -rf /var/lib/apt/lists/*

# 复制依赖文件
COPY requirements.txt .

# 安装 Python 依赖
RUN pip install --no-cache-dir -r requirements.txt

# 复制应用代码
COPY . .

# 暴露端口
EXPOSE 8004

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import requests; requests.get('http://localhost:8004/health', timeout=5)" || exit 1

# 启动命令
CMD ["python", "server.py", "--host", "0.0.0.0", "--port", "8004"]
```

#### 步骤 2：更新 docker-compose.yml

扩展现有配置，添加三个 LLM 服务：

```yaml
services:
  # ... 现有 postgres 和 redis 服务 ...
  
  sensevoice:
    build: ./Emotion-Echo-LLM/sensevoice-small
    container_name: emotion-echo-sensevoice
    restart: unless-stopped
    ports:
      - "8002:8002"
    environment:
      - PYTHONUNBUFFERED=1
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8002/health"]
      interval: 30s
      timeout: 10s
      start_period: 120s
      retries: 3
    depends_on:
      - redis

  xtts:
    build: ./Emotion-Echo-LLM/XTTS
    container_name: emotion-echo-xtts
    restart: unless-stopped
    ports:
      - "8003:8003"
    environment:
      - PYTHONUNBUFFERED=1
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8003/health"]
      interval: 30s
      timeout: 10s
      start_period: 180s
      retries: 3

  fer:
    build: ./Emotion-Echo-LLM/FER
    container_name: emotion-echo-fer
    restart: unless-stopped
    ports:
      - "8004:8004"
    environment:
      - PYTHONUNBUFFERED=1
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8004/health"]
      interval: 30s
      timeout: 10s
      start_period: 60s
      retries: 3
```

#### 步骤 3：更新配置文件

修改 `Emotion-Echo-Gin/config.yaml`，使用容器名作为服务地址：

```yaml
ai:
  emotion:
    enabled: true
    base_url: "http://sensevoice:8002"
  face:
    enabled: true
    fer_base_url: "http://fer:8004"
    weights:
      face: 0.5
      voice: 0.3
      text: 0.2
```

---

## 3. 风险评估与应对

### 3.1 风险清单

| 风险 | 等级 | 影响 | 应对措施 |
|------|------|------|---------|
| GPU 不可用 | 中 | XTTS 性能下降 | 使用 CPU 模式启动 |
| 模型下载失败 | 高 | 服务无法启动 | 手动下载模型或配置代理 |
| 端口冲突 | 低 | 服务启动失败 | 检查并修改端口映射 |
| 内存不足 | 中 | 服务崩溃 | 增加容器内存限制 |

### 3.2 降级方案

```bash
# 如果没有 GPU，使用 CPU 模式
# 修改 docker-compose.yml 中 xtts 的启动命令
command: ["python", "server.py", "--host", "0.0.0.0", "--port", "8003", "--device", "cpu"]
```

---

## 4. 部署验证

### 4.1 启动命令

```bash
# 进入项目目录
cd Emotion-Echo-Gin

# 构建并启动所有服务
docker-compose up -d --build

# 查看日志
docker-compose logs -f

# 检查服务状态
docker-compose ps
```

### 4.2 健康检查

| 服务 | 检查命令 | 预期结果 |
|------|---------|---------|
| SenseVoice | `curl http://localhost:8002/health` | `{"status":"ok"}` |
| XTTS-v2 | `curl http://localhost:8003/health` | `{"status":"ok"}` |
| FER | `curl http://localhost:8004/health` | `{"status":"ok"}` |

### 4.3 功能测试

```bash
# 测试语音情绪识别
curl -X POST http://localhost:8002/analyze -F "file=@test_audio.wav"

# 测试语音合成
curl -X POST http://localhost:8003/tts \
  -H "Content-Type: application/json" \
  -d '{"text":"你好","language":"zh-cn"}'

# 测试人脸情绪识别
curl -X POST http://localhost:8004/analyze -F "file=@test_face.jpg"
```

---

## 5. 预期结果

完成部署后，系统架构如下：

| 服务 | 容器名 | 端口 | 状态 |
|------|--------|------|------|
| PostgreSQL | emotion-echo-postgres | 5432 | ✅ |
| Redis | emotion-echo-redis | 6379 | ✅ |
| Go 后端 | emotion-echo-backend | 8080 | ✅ |
| SenseVoice | emotion-echo-sensevoice | 8002 | ✅ |
| XTTS-v2 | emotion-echo-xtts | 8003 | ✅ |
| FER | emotion-echo-fer | 8004 | ✅ |

---

## 6. 后续优化建议

1. **模型缓存**：使用 Docker 卷挂载模型目录，避免重复下载
2. **环境变量配置**：将配置项转为环境变量
3. **监控告警**：添加 Prometheus 监控指标
4. **日志收集**：配置 ELK 日志收集
5. **自动扩缩容**：根据负载自动调整容器数量