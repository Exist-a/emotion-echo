# Stage 89 · 2026-09-13 文件理解发给 LLM（文件+提问一起发，会话内持续引用）

> **状态**：🟢 **核心场景全链端到端实测通过**（txt 哨兵 + 追问引用）；PDF 路径已注入但 DeepSeek 行为不一致记为 residual
> **关联**：[`docs/plans/file-understanding-llm.md`](../plans/file-understanding-llm.md)（本期计划 + 调研依据）
> **Plan 迁 landed**：[`docs/legacy-plans/landed/file-understanding-llm.md`](../legacy-plans/landed/file-understanding-llm.md)（本期收口后即迁）

## 核心结论

用户上传文件 + 提问时，DeepSeek **能读到文件内容**并据此回答；同会话后续追问 LLM 仍能看到文件上下文（"会话内持续引用"）。
全链 6 个 PR 严格 RED→GREEN，proto/bff/chat-svc/llm-service/web 五端协同。

## 一、范围

| 层 | 变更 |
|---|---|
| proto | chat.proto `SendMessageRequest.file_name=9` + `Message.file_name=10`；emotion_llm.proto `FileAttachment{url,name}` + `ChatCompletionRequest.files=7` |
| chat-svc | migration 006 `messages.file_name VARCHAR(255)` + 五处映射全链透传（SendMessageReq/MessageView + SendMessage/ListMessages logic + toProtoMessage + grpcserver SendMessage 映射） |
| BFF | SendMessageReq/MessageView.MessageItemVM.fileName 回带；`AIStreamHandler.collectFileAttachments`（≤2 条 file 消息，旧→新排序）+ `fileSourceURL`（PublicBaseURL→内部 MinIO 端点重写）；fileMessageLister 小接口；main.go 把 chat client 接入 |
| llm-service | 新模块 `file_context.py`（URL 白名单 + 限时下载 + txt/md/csv/json/log/pdf/docx 抽取 + 截断，宽 fail）；grpc_server.py 在 intent 注入后追加 system 上下文；requirements 加 pypdf + python-docx；Dockerfile 双阶段 COPY（契约测试先红后绿） |
| web | types/api.ts + message store `sendMessage(content,emotion,clientMsgId,contentType,fileName)` 第五参；`[id].vue` 附件 chip 挂输入框 + 三分支 handleSubmit（附件+文字/附件无文字→默认 prompt/纯文字）；ChatFile 传真实 `:filename` |

## 二、真实容器 e2e（`--profile default` + DeepSeek 真实 key）

| # | 场景 | 预期 | 实测 |
|---|---|---|---|
| 1 | 上传 txt（含哨兵 `Stage89-SENTINEL-e2e真实调用DeepSeek成功...`）→ 文件消息落库 → fileName 回带 → ai/stream | fileName=中文 + DeepSeek 引用哨兵 | ✅ fileName=测试文件.txt，回复："原文是：'Stage89-SENTINEL-...'" |
| 2 | 同会话追问（不再发新文件） | LLM 仍能看到文件 | ✅ Q2 复述："文件里有一句：'Stage89-SENTINEL-追问测试请引用这句'" |
| 3 | 上传 PDF（手工构造含 Tj 文本流）+ 提问 | pypdf 抽出内容 | ⚠️ llm-service 日志显示 `injected 1 attachment(s), 100 chars` 含 sentinel，但 DeepSeek 多次拒引返"我没有真正读取到"——模型行为差异，记 residual |
| 4 | Stage 87 真实 key 意图重分类 | LLM 消歧落库 | ✅ "我在思考一个问题..." → `intent=emotional_support`（18 字符，扩 VARCHAR 后正常） |

## 三、本批揪出的暗坑 + 修

| 坑 | 来源 | 修法 |
|---|---|---|
| **bff ai-stream 文件收集走 c.Request.Context()，缺 x-user-id** | BFF 全局 auth 中间件只把 user ID 注入 gin c.Request.Context() 的 *value*，需要 `session.WithRequestAuth(c)` 才能在 gRPC metadata 里塞 x-user-id | ai_stream_handler.go collectFileAttachments 改用 session.WithRequestAuth |
| **chat-svc `messages.intent VARCHAR(16)` 与白名单 `emotional_support`(18字符) 不一致** | Stage 82 §契约5 暗坑（DDL 设计错了；白名单校验放行但 INSERT 仍 22001） | migration 007 `intent VARCHAR(16)→32`（DROP VIEW/COLUMN/CREATE/GRANT 一气呵成，副作用已在 007 SQL 内） |
| **docker compose 不会自动加载 `.env.local`** | 历史文档误称"自动加载"——Stage 88 收口已修，PR-6 容器重建顺手验证 | 已在 stage-88 文档固化 |

## 四、关键 commit

```
c71423c feat(proto,shared): PR-1 file_name + FileAttachment.files
90abcfb feat(chat): PR-2 file_name 五处映射
fd4cxxx feat(bff): PR-3 fileName + 文件收集注入
6d70xxx feat(llm): PR-4 file_context 抽取模块 + 注入
xx5xxx  feat(web): PR-5 附件挂输入框 + 文件+提问一次发送 + ChatFile 真实文件名
xx7xxx  fix(bff,chat): PR-6 e2e 揪出两暗坑（auth ctx + intent 列扩）
```

## 五、本批未做（open / residual）

| 项 | 说明 | 去向 |
|---|---|---|
| **PDF 路径 DeepSeek 拒读** | 注入报告正常（100 chars 含 sentinel），但模型多次返"没读取到"。可能 system prompt + file_context 拼接位置不适（移到 user 消息尾部或试更明确的指令），或 DeepSeek 对单短文本行判定保守 | Stage 90 优化 file_context 注入位置 + prompt 文案 |
| **AI 多模态（图像/音视频理解）** | DeepSeek API 纯文本，无视觉接口 | 后续可换多模态模型或自建 OCR 服务 |
| **扫描件 PDF OCR** | pypdf 对扫描件 PDF 抽不出文本 | 后续按需接 OCR（XTTS 时代已有同类经验） |
| **抽取结果缓存** | 同会话每次 ai/stream 重抽（≤2 文件，毫秒级）——首次发现已能正确，缓存收益小 | 暂不做 |
| **AI 回复的历史组装** | 当前 system + 最后一条 user；不做整段历史组装（与 file 上线前的限制同型） | 独立 residual（已有计划） |
| **prod 独立 bff-client 证书 / gRPC mTLS** | 纯 prod 事项 | 沿 stage-88 §五 open |

## 六、调研依据

- 已读：`ai_stream_handler.go` / `llm_grpc.go` / `chat.go` / `chat_grpc.go` / `upload_handler.go` / `minio.go` /
  `chat.proto` / `emotion_llm.proto` / `sendmessagelogic.go` / `chat_completion.py` / `intent_llm.py` /
  `useConversationSender.ts` / `[id].vue` / `ChatFile.vue` / `types/api.ts` / `decisions.md` 决策 4/5 /
  `stage-87-llm-intent-reclassify-2026-09-13.md` / `stage-88-llm-nacos-registration-2026-09-13.md`
- 容器实证：pypdf 抽 PDF 文本可解；DeepSeek 真实 stream；followup 复用 file context 仍能命中；
  migration 007 副作用（msg_summary_v 重建）在 compose 启动 db-migrate 时正常通过

---

> 最后更新：2026-09-13 by Stage 89 实施 session
> 关联：file-understanding-llm.md（已迁 landed）、stage-82/87/88、决策 4