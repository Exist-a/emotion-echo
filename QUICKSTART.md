# Emotion-Echo 完整启动与测试流程

> Stage 36-D：本文版对应**当前 Stage 30+ BFF 架构**（6 个 Go 微服务 + Python gRPC + BFF 聚合 + 前端 Nuxt + AI profile）。
> 历史单体 Gin 已迁移至 `legacy/emotion-echo-gin/`。

## 目录

1. [项目架构预览](#1-项目架构预览)
2. [快速启动（Docker Compose 推荐）](#2-快速启动docker-compose-推荐)
3. [各服务详细启动步骤](#3-各服务详细启动步骤)
4. [完整功能测试流程](#4-完整功能测试流程)
5. [常见问题解决](#5-常见问题解决)

---

## 1. 项目架构预览

```
Emotion-Echo/
├── Emotion-Echo-Web/                # 前端 Nuxt 3 + Element Plus
├── emotion-echo-user-svc/           # 用户认证 (Go + Gin :8888)
├── emotion-echo-chat-svc/           # 会话与消息 (Go + Gin :8890)
├── emotion-echo-analytics-svc/      # 情绪分析报表 (Go + Gin :8893)
├── emotion-echo-assessment-svc/     # 心理量表 (Go + Gin :8889)
├── emotion-echo-ai-svc/             # AI 编排 (Go + gRPC :8892 + HTTP :8891)
├── emotion-llm-service/             # Python gRPC LLM 推理 (:8000 + :50051)
├── emotion-echo-web-bff/            # 唯一 BFF 入口 (Go + Gin :8894)
├── Emotion-Echo-LLM/                # 多模态 AI profile
│   ├── FER/                         # 人脸情绪 (:8004, profile ai)
│   ├── sensevoice-small/            # 语音情绪 (:8002, profile ai)
│   └── XTTS/                        # 语音合成 (:8003, profile ai)
├── legacy/emotion-echo-gin/         # 已归档的单体 Gin（**不再使用**）
└── deploy/                           # docker-compose 编排
    ├── docker-compose.infra.yml     # PG/Redis/Kafka/Nacos/SW
    └── docker-compose.apps.yml       # 6 Go svc + BFF + emotion-llm-service + AI profile
```

### 服务端口说明（开发模式）

| 服务 | 容器内 | 宿主映射 |
|---|---|---|
| **前端 Web (Nuxt)** | :3000 | <http://localhost:3000> |
| **BFF (唯一入口)** | :8894 | <http://localhost:8894> |
| user-svc | :8888 | (容器内) |
| chat-svc | :8890 | (容器内) |
| analytics-svc | :8893 | (容器内) |
| assessment-svc | :8889 | (容器内) |
| ai-svc (HTTP/gRPC) | :8891 / :8892 | (容器内) |
| emotion-llm-service (HTTP/gRPC) | :8000 / :50051 | (容器内) |
| FER (AI profile) | :8004 | (容器内) |
| SenseVoice (AI profile) | :8002 | (容器内) |
| XTTS (AI profile) | :8003 | (容器内) |
| PostgreSQL | :5432 | (容器内) |
| Redis | :6379 | (容器内) |
| Kafka | :9092 | (容器内) |
| Nacos | :8848 | <http://localhost:8848/nacos> |
| SkyWalking UI | :8080 | <http://localhost:8080> |

---

## 2. 快速启动（Docker Compose 推荐）

### 前置要求

- Docker Desktop 或 Docker Engine + Compose v2
- Go 1.21+（如需本地开发）
- Node.js 18+（如需前端本地 dev）
- Python 3.10+（仅 AI profile 构建时需要）

### 步骤 1：克隆项目

```bash
cd d:\源码\Emotion-Echo
```

### 步骤 2：环境变量（可选，用于真实 LLM）

```bash
# 默认走 mock LLM fallback；接入真实 DeepSeek：
cp deploy/env/.env.local.example deploy/env/.env.local
# 编辑 LLM_API_KEY=sk-... 和 BFF_LLM_API_KEY=sk-...
```

### 步骤 3：启动基础设施

```bash
cd deploy
docker compose -f docker-compose.infra.yml up -d
# 等待 30~60 秒各容器健康
docker compose -f docker-compose.infra.yml ps
# 期望: postgres/redis/kafka/nacos/sw-oap/sw-ui 都 healthy
```

### 步骤 4：启动业务应用

```bash
# 6 Go svc + emotion-llm-service + ai-svc + BFF
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml up -d --no-build

# （可选）启动 AI profile（FER / SenseVoice / XTTS）
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --profile ai up -d --no-build emotion-echo-fer emotion-echo-sensevoice emotion-echo-xtts
```

### 步骤 5：验证联通

```bash
# 1. BFF 健康检查（聚合 6 下游）
curl http://localhost:8894/health
# 期望: {"status":"ok","downstream":{"ai":"ok",...,"xtts":"ok"}}

# 2. 端到端冒烟（16/16 通过为 GREEN）
python scripts/smoke_bff_t5.py

# 3. 容器健康
bash scripts/healthcheck_smoke.sh

# 4. 测试账号就绪
bash scripts/check_seed_users.sh
# 期望: GREEN: echo / echo123 ready
```

---

## 3. 各服务详细启动步骤

### A. 数据库 / 基础设施（已 Stage 26-Q 升级到 PG/Redis/Kafka/SkyWalking/Nacos）

**使用 Docker Compose：**

```bash
cd deploy
docker compose -f docker-compose.infra.yml up -d
docker compose -f docker-compose.infra.yml logs -f
docker compose -f docker-compose.infra.yml down   # 停止
docker compose -f docker-compose.infra.yml down --volumes   # 同时删卷（**会丢数据**）
```

**验证：**
- PostgreSQL：`docker exec -it emotion-echo-postgres psql -U postgres -d emotion_echo -c '\l'`
- Redis：`docker exec -it emotion-echo-redis redis-cli ping`
- Kafka：`MSYS_NO_PATHCONV=1 docker exec emotion-echo-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list`
- Nacos 控制台：<http://localhost:8848/nacos>（nacos/nacos）

### B. 业务微服务（6 Go svc + BFF + emotion-llm-service）

**镜像已预构建，直接 up 即可**：

```bash
cd deploy
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml up -d --no-build
```

**本地开发模式**（需要 go.mod / go.work）：

```bash
cd emotion-echo-user-svc
go mod download
POSTGRES_DSN="host=localhost port=5432 user=postgres password=postgres dbname=emotion_echo sslmode=disable search_path=emotion_echo_user" \
    SKYWALKING_OAP_ADDR=localhost:11800 \
    go run main.go
```

各 svc 的 `etc/<svc>-api.yaml` 是 go-zero 风格（含 listen port + 数据库 DSN 等）。

### C. AI profile（FER / SenseVoice / XTTS）

**预构建镜像已存在**，启动：

```bash
cd deploy
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --profile ai up -d --no-build emotion-echo-fer emotion-echo-sensevoice emotion-echo-xtts
```

**验证：**

```bash
curl http://emotion-echo-fer:8004/health
# {"status":"ok","model_loaded":false,"backend":"neutral-fallback"}
```

**已知限制**（参考 `docs/ai-images-build-guide.md`）：
- FER `emotion_net.caffemodel` 未预烘焙 → 永远 neutral-fallback
- XTTS `model.pth` PyTorch 2.6+ 兼容 → 用 `emotion-echo/xtts:v0.1.9` 已包含 torchaudio shim
- SenseVoice pre-bake 后秒启动（funasr metadata 校验）

---

## 4. 完整功能测试流程

### 测试 1：基础功能验证

**步骤：**
1. 打开浏览器：<http://localhost:3000>（或 curl BFF `/health`）
2. 登录用测试账号：
   - 用户名：`echo`
   - 密码：`echo123`
3. 进入首页

### 测试 2：端到端冒烟（推荐）

```bash
python scripts/smoke_bff_t5.py
```

期望：**16/16 通过**。覆盖 BFF `/health` 聚合 6 下游、登录、当前用户、创建/列表会话、发送消息、AI 流式回复（mock 或真实 LLM）、情绪分析报表、心理量表列表、Prometheus metrics。

### 测试 3：文本对话功能

**步骤：**
1. 登录后点击「开始对话」
2. 创建新会话
3. 输入文本消息，如：「今天心情不太好」
4. 检查 AI 回复是否正常（mock fallback 给共情回复；真实 LLM 给定制回复）

### 测试 4：情绪分析报表

**步骤：**
1. 发送多条消息（建议 10 条以上）
2. 进入「用户中心」→「情绪报告」
3. 检查日报、周报、月报

**前置**：需要 Kafka 异步管道或手动调用 ai-svc `/api/v1/multimodal/analyze` 写 emotion_analysis 表。

### 测试 5：心理测验

**步骤：**
1. 进入「心理测验」
2. 完成一个量表
3. 检查结果

### 测试 6：深色模式适配

**步骤：**
1. 切换深色/浅色模式
2. 检查各页面元素
3. 特别检查图表渲染

---

## 5. 常见问题解决

### Q1: 端口被占用

```bash
netstat -ano | findstr :8894
taskkill /F /PID <PID>
```

### Q2: 数据库连接失败

检查 Docker 是否正常运行 + 容器状态：
```bash
docker ps | grep emotion-echo-postgres
docker logs emotion-echo-postgres
```

### Q3: 容器 (unhealthy) 但 /health 返回 200

参考 `scripts/healthcheck_smoke.sh`。可能原因：
- start_period 太短（已默认 60s）
- wget --spider 用 HEAD 但 /health 只支持 GET（已修）
- kafka healthcheck 短路径找不到二进制（已修）

### Q4: BFF /api/v1/reports/daily 500

参考 `docs/stage-36-smoke-report.md` §九 Bug 2。修法：
- ai-svc KAFKA_TOPIC/GROUP/ENABLED 注入
- emotion_echo_chat.msg_summary_v VIEW 创建
- deploy/db/04-create-views.sql 挂载到 postgres

### Q5: TTS /tts 500 (RuntimeError: Double vs Float)

参考 `docs/stage-36-smoke-report.md` §九 Bug 3 + `docs/ai-images-build-guide.md` §六 6.7。已用 `emotion-echo/xtts:v0.1.9` 修复（torchaudio shim）。

### Q6: 真实 LLM 走 mock

参考 `docs/stage-36-smoke-report.md` §八 T8：
```bash
cp deploy/env/.env.local.example deploy/env/.env.local
# 填 LLM_API_KEY=sk-... 和 BFF_LLM_API_KEY=sk-...
docker compose ... up -d --force-recreate emotion-echo-web-bff emotion-echo-ai-svc
```

### Q7: 浏览器打不开 (http://localhost:3000)

Web 容器构建需要 npm 仓库可达（Dockerfile 默认 npmmirror.com 容器内可能不可达）。**workaround**：
1. 改 Dockerfile：`registry.npmjs.org` 替代 npmmirror
2. 或本地 dev：`cd Emotion-Echo-Web && npm install && npm run dev`

---

## 完整启动命令清单

### Windows 完整启动脚本（PowerShell）

**终端 1：基础设施**
```powershell
cd deploy
docker compose -f docker-compose.infra.yml up -d
```

**终端 2：业务应用**
```powershell
cd deploy
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml up -d --no-build
```

**终端 3：（可选）AI profile**
```powershell
cd deploy
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --profile ai up -d --no-build emotion-echo-fer emotion-echo-sensevoice emotion-echo-xtts
```

**验证：**
```powershell
python scripts/smoke_bff_t5.py
```

---

## 开发调试

### 后端日志查看

```bash
docker logs -f emotion-echo-web-bff
docker logs -f emotion-echo-ai-svc
```

### 数据库查看工具

推荐 DBeaver / pgAdmin / Redis Desktop Manager。

### 链路追踪

SkyWalking UI：<http://localhost:8080>（容器内访问 http://localhost:8080）

### 配置中心

Nacos 控制台：<http://localhost:8848/nacos>（默认账号 nacos/nacos）

---

**祝开发愉快！有问题看 `docs/stage-36-smoke-report.md` + 各 stage 文档。**