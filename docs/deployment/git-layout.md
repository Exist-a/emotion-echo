# Emotion-Echo 仓库布局规范

**日期**：2026-07-16
**适用**：本仓库所有协作者（人类 / AI Agent）

---

## 一、仓库拓扑

**单仓 monorepo** —— 所有代码在一个 git 仓库内，**不使用 submodule**。

```
Emotion-Echo/  ←── 唯一仓库（github.com/Exist-a/emotion-echo）
│
├── emotion-echo-shared/          # Go 共享库
├── emotion-echo-ai-svc/          # Go 微服务：AI 编排（gRPC server / Kafka consumer）
├── emotion-echo-chat-svc/        # Go 微服务：会话/消息
├── emotion-echo-analytics-svc/   # Go 微服务：情绪分析报表
├── emotion-echo-assessment-svc/  # Go 微服务：心理测验/量表
├── emotion-echo-user-svc/        # Go 微服务：用户认证
├── emotion-llm-service/          # Python gRPC：LLM 文本情绪分析
│
├── Emotion-Echo-Web/             # 前端 Nuxt 3
├── legacy/emotion-echo-gin/      # 遗留单体（已归档，独立运行）
├── Emotion-Echo-LLM/
│   ├── FER/                      # Python: 人脸情绪识别（Stage 22 容器化）
│   ├── sensevoice-small/         # Python: 语音情绪识别
│   └── XTTS/                     # Python: 语音合成
│       ├── TTS/                  # Coqui XTTS 核心代码
│       ├── Dockerfile            # Stage 22 容器化
│       └── ...
│
├── proto/                        # protobuf 契约
├── deploy/                       # 分布式基础设施编排（docker-compose / APISIX）
├── docs/                         # 架构决策 + stage-X 文档
└── scripts/                      # 运维/验证脚本
```

---

## 二、历史说明

| 日期 | 状态 | 说明 |
|------|------|------|
| 2026-07-16 上午 | monorepo + 4 个 submodule | 初次推送到 GitHub |
| 2026-07-16 下午 | **单仓 monorepo**（当前） | 移除 4 个 submodule，作为普通目录入仓 |

**为什么移除 submodule**：
- 个人项目，跨仓改 PR 成本高
- 5 个微服务共享 `emotion-echo-shared`，跨仓不便
- 前后端同步开发，单仓更高效
- 子仓无独立发布节奏
- GitHub 上 `Emotion-Echo-Web` 和 `Emotion-Echo-Gin` 已删除（合并后不再需要）

迁移快照：`docs/flatten-snapshot-*.json`

---

## 三、克隆 & 工作流

### 3.1 克隆
```bash
git clone https://github.com/Exist-a/emotion-echo.git
cd emotion-echo
```

### 3.2 改代码
直接修改，不需要进任何子仓。

### 3.3 提交
```bash
git add -A
git commit -m "feat(...): ..."
git push origin main
```

---

## 四、不该进仓的内容

已被 `.gitignore` 自动忽略：

- **Node 依赖**：`node_modules/`、`.nuxt/`、`.output/`
- **AI 模型权重**：`*.pth`、`*.onnx`、`*.bin` 等二进制
- **XTTS 模型目录**：`Emotion-Echo-LLM/XTTS/AI-ModelScope/`（2GB 自动下载）
- **运行时数据**：`.postgres/`、`*.db`、`*.sqlite`
- **构建产物**：`dist/`、`build/`、`coverage.html`
- **证书 / 密钥**：`*.pem`、`*.key`、`deploy/certs/`
- **散落的 pb 文件**：`/*.pb.go`（P1-D 规范化前的临时方案）
- **工具链二进制**：`protoc-dist/`、`protoc.zip`

---

## 五、演进约定

任何对仓库布局的修改都必须：

1. 先更新 `docs/git-layout.md`
2. 通过 review 后再执行
3. commit message 引用本文档章节

禁止事项：
- ❌ 重新引入 git submodule（已废弃方案）
- ❌ 拆分多个独立 git 仓库（除非公司化运营）
- ❌ 把 `node_modules/`、模型权重等大文件入仓