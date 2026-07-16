# Emotion-Echo 完整启动与测试流程

## 目录

1. \[项目架构预览
2. \[快速启动（推荐）
3. \[各服务详细启动步骤
4. \[完整功能测试流程
5. \[常见问题解决

***

## 1. 项目架构预览

```
Emotion-Echo
├── Emotion-Echo-Web (前端 - Nuxt 3 + Element Plus)
├── Emotion-Echo-Gin (后端 - Go + Gin + PostgreSQL + Redis)
└── Emotion-Echo-LLM
    ├── sensevoice-small (语音情绪识别模型)
    ├── XTTS (语音合成模型)
    └── FER (人脸情绪识别模型)
```

服务端口说明：

- **前端**: <http://localhost:3000>
- **后端 API**: <http://localhost:8080>
- **SenseVoice**: <http://localhost:8002>
- **XTTS-v2**: <http://localhost:8003>
- **FER**: <http://localhost:8004>
- **PostgreSQL**: localhost:5432
- **Redis**: localhost:6379

***

## 2. 快速启动（推荐）

### 前置要求

- Node.js 18+
- Go 1.21+
- Docker Desktop 或 Docker Compose
- Python 3.10+ (用于 AI 服务)
- Git

### 步骤 1：克隆项目（如果还没 clone）

```bash
cd d:\源码\Emotion-Echo
```

### 步骤 2：启动数据库服务（PostgreSQL + Redis）

\*\*Windows:

```bash
cd Emotion-Echo-Gin
docker-compose up -d
```

\*\*等待数据库启动完成（约 30 秒）

### 步骤 3：配置后端

```bash
# 复制配置文件
copy configs\config.example.yaml configs\config.yaml

# 编辑 configs\config.yaml，修改以下内容：
# - database.postgres.password （留空即可，docker-compose 配置的是无密码）
# - ai.kimi.api_key (如果需要 AI 功能，填入你的 Kimi API Key)
```

### 步骤 4：启动后端

```bash
# 安装依赖（首次）
go mod download

# 启动后端服务
go run ./cmd/server/main.go
```

后端将在 <http://localhost:8080> 启动

### 步骤 5：启动前端（新开终端）

```bash
cd ..\Emotion-Echo-Web

# 安装依赖（首次）
npm install

# 启动前端开发服务器
npm run dev
```

前端将在 <http://localhost:3000> 启动

### 步骤 6：（可选）启动模型服务

#### SenseVoice 语音情绪识别

```bash
cd ..\Emotion-Echo-LLM\sensevoice-small

# 创建 Python 虚拟环境（首次）
conda create -n sensevoice python=3.10 -y
conda activate sensevoice

# 安装依赖
pip install -r requirements-server.txt

# 启动服务（CPU 模式）
python server.py
```

#### XTTS-v2 语音合成（新开终端）

```bash
cd ..\Emotion-Echo-LLM\XTTS

# 创建 Python 虚拟环境（首次）
conda create -n xtts python=3.10 -y
conda activate xtts

# 安装依赖
pip install -r requirements.txt

# 启动服务
python server.py --device cpu
```

#### FER 人脸情绪识别（新开终端）

```bash
cd ..\Emotion-Echo-LLM\FER

# 创建 Python 虚拟环境（首次）
conda create -n fer python=3.10 -y
conda activate fer

# 安装依赖
pip install -r requirements.txt

# 启动服务
python server.py
```

***

## 3. 各服务详细启动步骤

### A. 数据库服务

**使用 Docker Compose（推荐）：**

```bash
cd Emotion-Echo-Gin

# 启动
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止
docker-compose down
```

\*\*验证：

```bash
# 验证 PostgreSQL
psql -h localhost -p 5432 -U postgres -d emotion_echo -c "SELECT version();"

# 验证 Redis
redis-cli ping
```

### B. 后端服务

**详细启动：**

```bash
cd Emotion-Echo-Gin

# 1. 检查配置
# 确保 configs/config.yaml 已正确配置

# 2. 安装依赖
go mod download

# 3. 启动
go run ./cmd/server/main.go
```

\*\*健康检查：

```bash
curl http://localhost:8080/health
```

\*\*测试账号：

- 手机号：13800138000
- 密码：abc123

### C. 前端服务

**详细启动：**

```bash
cd Emotion-Echo-Web

# 1. 安装依赖
npm install

# 2. 启动开发服务器
npm run dev

# 3. 构建生产版本
npm run build
```

### D. SenseVoice 情绪识别服务

**详细启动：**

```bash
cd Emotion-Echo-LLM\sensevoice-small

# 1. 创建并激活虚拟环境
conda create -n sensevoice python=3.10 -y
conda activate sensevoice

# 2. 安装依赖
pip install -r requirements-server.txt

# 3. CPU 模式启动（推荐测试用）
python server.py --host 0.0.0.0 --port 8002 --device cpu

# GPU 模式（需要 NVIDIA GPU）
python server.py --host 0.0.0.0 --port 8002 --device cuda:0
```

\*\*验证服务：

```bash
# 健康检查
curl http://localhost:8002/health

# 测试情绪识别
curl -X POST http://localhost:8002/analyze \
  -F "file=@example/zh.mp3"
```

***

## 4. 完整功能测试流程

### 测试 1：基础功能验证

**步骤：**

1. 打开浏览器：<http://localhost:3000>
2. 使用测试账号登录：
   - 手机号：13800138000
   - 密码：abc123
3. 检查是否能正常进入首页

### 测试 2：文本对话功能

**步骤：**

1. 登录后点击「开始对话」
2. 创建新会话
3. 输入文本消息，如：「今天心情不太好」
4. 检查 AI 回复是否正常

### 测试 3：语音消息功能（需要启动 SenseVoice）

**步骤：**

1. 在会话页面，点击录音按钮
2. 开始录音，说几句话
3. 点击停止录音
4. 检查是否能正常识别为语音消息
5. 检查语音条是否能正常播放
6. 检查识别文本是否正确
7. 检查情绪识别标签是否显示

### 测试 4：情绪分析报表

**步骤：**

1. 发送多条消息（建议 10 条以上
2. 进入「用户中心」→「情绪报告」
3. 检查日报、周报、月报
4. 检查图表是否正常显示
5. 切换深色模式检查图表适配

### 测试 5：心理测验

**步骤：**

1. 进入「心理测验」
2. 完成一个量表
3. 检查结果是否正常

### 测试 6：深色模式适配

**步骤：**

1. 切换深色/浅色模式
2. 检查各页面元素
3. 特别检查图表渲染
4. 检查会话页面背景

***

## 5. 常见问题解决

### 问题 1：端口被占用

\*\*Windows：

```bash
# 查看端口占用
netstat -ano | findstr :8080
# 或
netstat -ano | findstr :3000

# 杀死进程
taskkill /F /PID <PID号>
```

### 问题 2：数据库连接失败

**检查：**

1. Docker 是否正常运行
2. PostgreSQL/Redis 容器状态

```bash
docker ps
```

### 问题 3：前端无法连接后端

**检查：**

1. 后端是否正常启动（访问 <http://localhost:8080/health>
2. 前端配置的 API 地址是否正确

### 问题 4：SenseVoice 模型下载慢

**解决：**

1. 首次运行时会自动下载模型
2. 也可以手动下载后放到本地
3. 或者使用国内镜像源

### 问题 5：Go 依赖安装慢

**配置国内代理：**

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

### 问题 6：npm 安装慢

**配置国内镜像：**

```bash
npm config set registry https://registry.npmmirror.com
```

***

## 完整启动命令清单

### Windows 完整启动脚本（PowerShell）

\*\*终端 1：数据库

```powershell
cd Emotion-Echo-Gin
docker-compose up -d
```

\*\*终端 2：后端

```powershell
cd Emotion-Echo-Gin
go run ./cmd/server/main.go
```

\*\*终端 3：前端

```powershell
cd Emotion-Echo-Web
npm run dev
```

\*\*终端 4：SenseVoice (可选)

```powershell
cd Emotion-Echo-LLM\sensevoice-small
conda activate sensevoice
python server.py
```

***

## 开发调试

### 后端日志查看

```bash
# 后端会在终端直接输出日志
```

### 前端调试

- 浏览器 F12 打开开发者工具
- 查看 Console 和 Network 标签

### 数据库查看工具

推荐使用：

- DBeaver (免费)
- pgAdmin
- Redis Desktop Manager

***

\*\*祝开发愉快！有问题查看各服务的 README 文档。
