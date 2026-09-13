---
status: landed
landed: 2026-09-13
owner: User
created: 2026-09-13
related-stages:
  - stage-89-file-understanding-llm-2026-09-13.md（收口报告，含 6 个 PR + e2e 实测 + 残差清单）
  - stage-79-post-file-upload-wiring-2026-09-12.md（文件消息落库 + contentType 全链修复；文件理解登记为后续解锁项）
  - stage-80-llm-chat-completion-rpc-2026-09-12.md（ChatCompletion 流式 RPC）
  - stage-81-bff-llm-grpc-upstream-2026-09-12.md（BFF ai/stream gRPC 上游）
  - stage-87-llm-intent-reclassify-2026-09-13.md（意图重分类；真实 key 验证顺手本期做）
related-adrs:
  - docs/architecture/decisions.md 决策 4（内部 gRPC；抽取在 llm-service 内完成，不新增跨服务 HTTP）
  - docs/architecture/decisions.md 决策 5（Python LLM 微服务）
---

# Plan — 文件理解发给 LLM（文件+提问一起发，会话内持续引用）

## 一、上下文与假设

**目标**：用户上传文件并提问时，LLM 能"看到"文件内容并据此回答；同一会话内后续追问仍能引用文件（用户 2026-09-13 拍板两个产品决策：① 文件+提问一起发；② 会话内持续引用）。

**现状**（2026-09-13 代码实证）：
- 文件消息 = `emotion_echo_chat.messages.content` 存 MinIO 公开 URL + `content_type='file'`，**不触发 AI 流**（`[id].vue:221-242` 注释"文件引用分析依赖 llm-chat-real-pipeline"）；全链无文本抽取代码
- BFF LLM prompt = 硬编码 system + 最后一条用户消息（`ai_stream_handler.go:121-130`），无历史组装
- MinIO：桶匿名可读，BFF `StorageClient` 只有 PutObject/GetObjectURL（无 GetObject）；仅 BFF 有 MinIO client
- 对象 key = `uploads/<uid>-<sha256(filename)[:8]><ext>`，**原始文件名未持久化**

**假设清单**（与现状对比）：
- 假设 DeepSeek API 纯文本（无文件理解接口）→ 走服务端文本抽取路线 ✓（api.deepseek.com，无 file API）
- 假设 pypdf / python-docx 纯 Python 可装 ✓（requirements 现有依赖均为 pip 安装）
- 假设 MinIO 桶匿名可读 → llm-service 直接 HTTP GET ✓（`minio.go:109` GetObjectURL 即公开 URL）
- 假设消息里 URL 是 PublicBaseURL（`http://localhost:9000`）——**llm-service 容器内不可达，必须由 BFF 重写为内部端点**
- gRPC 默认单消息 4MiB、上传上限 20MiB → 抽取文本必须截断，且 proto 只传 {url, name} 不传字节

## 二、交互设计（已拍板）

1. **发送**：选文件（即上传得 url）→ 附件挂输入框 → 用户可输入问题 → 点发送一次完成：
   ① 文件消息落库（content=url, contentType=file, **fileName** 新字段）
   ② 问题文本作为用户消息落库（空则默认"请帮我看看这个文件"）
   ③ 触发 ai/stream
2. **持续引用**：每次 ai/stream，BFF 用 chat gRPC ListMessages 拉最近消息，取最新 ≤2 条 file 消息，
   把 {url(重写为内部端点), name} 放进 ChatCompletionRequest.files → llm-service 拉取+抽取+注入

## 三、实施步骤（TDD，6 PR）

| PR | 范围 | RED | GREEN |
|---|---|---|---|
| 1 | proto：chat.proto SendMessageRequest/Message 加 file_name；emotion_llm.proto 加 FileAttachment{name,url} + repeated files；gen.sh 重生成 | shared pkg 契约测试断言新字段存在（编译失败=红） | proto 修改 + 重生成 Go/Python stubs |
| 2 | chat-svc：migration 006（messages.file_name VARCHAR(255) NULL）+ model/logic/grpcserver 透传 | TestSendMessage_FileName_Persisted 等失败 | 全链透传 + §契约 5 式写入值断言 |
| 3 | BFF：SendMessageReq/MessageView/viewmodel 加 fileName；AIStreamHandler 注入 chat 列表接口收集 file 消息 → URL 重写 → LLMStreamRequest.Files；llm_grpc.go 映射 | fake lister 断言 Files 收集/重写/上限 2 条 | 全链实现 |
| 4 | llm-service：file_context.py（白名单 FILE_FETCH_ALLOWLIST / 限时下载 5s / 按扩展名抽取 / 每文件 6000 字符截断）+ grpc_server ChatCompletion 在 intent 注入后追加 system 上下文；requirements 加 pypdf+python-docx；ClassifyIntent 不碰文件文本 | tests/unit fake HTTP server 断言抽取/截断/白名单拒绝/失败注记 | 模块 + servicer 集成 |
| 5 | 前端：types + message store 透传 fileName；[id].vue/new.vue 附件挂输入框随发送走；ChatFile.vue 真实文件名 | Vitest 组件/store 测试 | 接线 |
| 6 | 容器 e2e + 收口：重建 3 镜像；哨兵字符串 txt/pdf 全链断言；同会话追问验证；Stage 87 真实 key 意图重分类实测；smoke；stage-89 报告 + plan 迁 landed | — | — |

## 四、决策与边界

- **抽取在 llm-service（Python）**：Go 侧 PDF 库弱；抽取与 prompt 组装同处（inject_style 先例）
- **proto 只传 {url,name}**：字节不过 gRPC（4MiB 限制），抽取按需、每次请求现抽（无缓存，residual）
- **SSRF 防护**：llm-service 只 GET 白名单 host（默认 minio 内部端点），http/https only
- **ClassifyIntent 不碰文件文本**（5s 超时保护）
- **不做**：图片/音视频理解（OCR 留 residual）、扫描件 OCR、抽取缓存、AI 回复历史组装（独立 residual）
- HTTP 直连降级路径不注入文件上下文（记日志）
- 新 env：FILE_FETCH_ALLOWLIST / FILE_FETCH_TIMEOUT / FILE_EXTRACT_MAX_CHARS（均有默认值）

## 五、调研依据

- 已读：`ai_stream_handler.go`、`llm_grpc.go`、`upload_handler.go`、`storage/minio.go`、
  `chat_handler.go`、`downstream/chat.go`、`proto/chat.proto`、`proto/emotion_llm.proto`、
  `chat-svc internal/logic/sendmessagelogic.go`、`emotion-llm-service/{chat_completion,grpc_server,intent,intent_llm}.py`、
  `useConversationSender.ts`、`useFileUpload.ts`、`[id].vue`、`ChatFile.vue`、`types/api.ts`
- 已查：stage-79（文件理解 residual 出处）、stage-80/81（LLM 链路）、stage-87（意图重分类 open），
  file-upload-message-extension.md（"发给 Kimi"原始诉求 §4.3）
- 网上信息：DeepSeek API 无文件理解接口（文本 chat.completions）——与 compose 内 LLM_BASE_URL 配置一致，
  本期不引入新外部依赖行为
