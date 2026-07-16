# Emotion-Echo

> 情绪倾诉与心理健康助手后端服务

一个基于 Go + Gin 框架构建的情绪倾诉应用后端，提供用户认证、AI 情绪疏导对话、心理测验、情绪分析报表等功能。

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 语言 | Go 1.21+ |
| Web 框架 | Gin |
| ORM | GORM |
| 数据库 | PostgreSQL 14+ |
| 缓存 | Redis 7+ |
| AI 服务 | Kimi (OpenAI 兼容 API) |
| 任务调度 | cron (内部调度器) |

---

## 快速开始

### 环境要求

- Go 1.21+
- PostgreSQL 14+（推荐用 Postgres.app）
- Redis 7+

### 1. 克隆与依赖

```bash
git clone <repo-url>
cd Emotion-Echo-Gin
go mod download
```

### 2. 数据库准备

```bash
# 创建数据库（默认用户 postgres，无密码）
createdb emotion_echo

# 或使用 psql
psql -U postgres -c "CREATE DATABASE emotion_echo;"
```

> 首次启动时，GORM AutoMigrate 会自动创建所有表结构。

### 3. 配置文件

```bash
cp configs/config.example.yaml configs/config.yaml
```

按需修改 `configs/config.yaml`：
- `database.postgres.password` — 你的 PostgreSQL 密码
- `jwt.secret` — 生产环境务必修改为随机强密码
- `ai.kimi.api_key` — 你的 Kimi API Key（已预填测试 Key）

### 4. 启动服务

```bash
go run ./cmd/server/main.go
```

服务启动后：
- API 地址：`http://localhost:8080/api/v1`
- 健康检查：`http://localhost:8080/health`

### 5. 测试账号

```json
{
  "username": "13800138000",
  "password": "abc123"
}
```

> 前端提交密码时会先做 SHA256 预处理。
>
> **重要安全提示**：生产环境请通过环境变量注入敏感配置，不要直接修改 config.yaml 中的密钥。

---

## 项目结构

```
.
├── cmd/server/           # 服务入口
├── configs/              # 配置文件
├── internal/
│   ├── config/           # 配置加载
│   ├── database/         # PostgreSQL / Redis 连接
│   ├── handler/          # HTTP 处理器（Controller）
│   ├── middleware/       # Gin 中间件
│   ├── models/           # 数据模型（GORM）
│   ├── pkg/              # 公共包（jwt、password、response、errors）
│   ├── repository/       # 数据访问层（DAO）
│   ├── router/           # 路由注册
│   ├── scheduler/        # 定时任务
│   ├── service/          # 业务逻辑层
│   └── workflow/         # 情绪分析工作流
├── migrations/           # 数据库迁移脚本
├── docs/                 # 文档
│   ├── API.md            # 前端对接文档（25个接口）
│   ├── DEVELOPMENT.md    # 开发规范
│   └── BACKEND_API_GAP.md # 接口对齐记录
└── uploads/              # 本地上传目录（头像等）
```

---

## 核心功能

- [x] 用户注册 / 登录 / 登出（JWT + RefreshToken Cookie）
- [x] 验证码登录 / 忘记密码重置
- [x] 用户资料管理与头像上传
- [x] AI 情绪疏导对话（Kimi 流式输出，支持情绪标签）
- [x] 心理测验（SDS 等量表）与结果查看
- [x] 情绪日报 / 周报 / 月报趋势分析
- [x] 微信 OAuth 登录接口（预留，暂不启用）

---

## API 文档

详见 [`docs/API.md`](docs/API.md)：
- 25 个接口完整定义
- 请求/响应示例
- 错误码列表
- 认证方式说明

---

## Docker 部署

项目提供完整的 Docker 支持，可以一键启动所有服务（后端、前端、数据库、大模型）。

详见项目根目录的 `docker-compose.yml`。

### 单独部署后端

```bash
docker build -t emotion-echo-gin .
docker run -p 8080:8080 \
  -e EE_DATABASE_POSTGRES_HOST=postgres \
  -e EE_DATABASE_REDIS_HOST=redis \
  emotion-echo-gin
```

---

## 开发文档

- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) — 项目架构、分层规范、模型定义、开发检查清单
- [`docs/BACKEND_API_GAP.md`](docs/BACKEND_API_GAP.md) — 前后端接口对齐记录

---

## License

MIT
