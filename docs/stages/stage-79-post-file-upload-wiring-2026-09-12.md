# Stage 79 — file-upload-message-extension 收口（装配层接线 + contentType 全链路修复）

> 日期：2026-09-12
> 来源：Stage 78 排期重排第 1 项 → 用户指示"按推荐进行"
> 计划：[docs/legacy-plans/landed/file-upload-message-extension.md](../legacy-plans/landed/file-upload-message-extension.md)（本批迁 landed）
> 性质：TDD 修复批次（RED eb29c27/7e25b2e → GREEN f994b92/d177e17/3fcb0fa/b574d4a），6 commit

## 一、调研结论（计划 vs 代码）

零件基本全备、唯独没装配：BFF `/api/v1/uploads/{image,video,file}`（Stage 58 完整实现）、
前端 `useFileUpload.ts`（+5 测试全绿）、`ChatFile.vue` 三态渲染、`apiRoutes` 主路径转正——
全部已存在。残余是装配线：类型收窄、`handleAttachment` 空实现、消息列表没挂 ChatFile、
store 硬编码 contentType。

## 二、e2e 实测揪出的深链 bug（本批核心价值）

"上传 → 发文件消息"首次真实 e2e 即抓到 **contentType 在响应链 5 处静默丢失**——
DB 落库 file、前端收到 text，且 ListMessages 同样丢失。逐层定位（5 处）：

| # | 层 | 缺口 | 修复 |
|---|---|---|---|
| 1 | BFF 绑定 | `SendMessageReq` tag snake_case（content_type/emotion_tag/client_msg_id），前端 camelCase 三字段**一直被静默丢弃**——Stage 33 幂等键从未真正到过 chat-svc | tag 改 camelCase（`d177e17`） |
| 2 | proto | `Message` 响应消息无 content_type 字段 | 加 content_type=8 + regen（`3fcb0fa`） |
| 3 | chat-svc | `toProtoMessage` 不回带 | 透传补齐 |
| 4 | chat-svc | logic 响应组装（SendMessage/ListMessages）漏 ContentType | 补齐（`b574d4a`） |
| 5 | BFF | `fromProtoMessage` 不映射 + `toMessageItemVM` **硬编码 "text"** | 透传 + `cmp.Or(…, "text")` 兜底 |

> 根因范式与 AGENTS.md §2.4 警告完全一致：各层单测各自用"自己那一层"的形状构造
> 输入（BFF 单测用 snake_case body、前端单测不存在），单层全绿但跨层契约断裂。
> 端到端 e2e 是唯一能抓到此类 bug 的手段。

## 三、前端装配（GREEN f994b92）

- `types/api.ts`：contentType 扩展 `image/file/video`（保留 `img` 兼容旧数据）；
  补导出 Stage 58 起就缺失的 `UploadResult/FileType/UploadProgress/FileUploadConfig`
  + `FaceEmotionResult`（vite 类型擦除掩盖，typecheck 才暴露）——项目类型错误 106→96
- `stores/message.ts`：`sendMessage` 参数化 contentType（默认 text 向后兼容）
- `[id].vue`：`handleAttachment` 从 console.log 接为 隐藏 input → useFileUpload 上传 →
  文件消息落库；消息列表挂 `ChatFile` 分支；文件消息不走 AI 流（文件引用分析依赖
  `llm-chat-real-pipeline.md`，已登记为后续解锁项）
- `useFaceEmotion.ts`：类型补导出后暴露的 2 个潜在错误顺修

## 四、验收

- TDD：RED（store 透传 / BFF camelCase 绑定 / chat-svc toProtoMessage / BFF toMessageItemVM
  / chat-svc logic roundtrip，5 组测试先红）→ 全绿
- 全量：web vitest **24 文件 250 测试 PASS**；chat-svc/web-bff/shared/user-svc
  go vet 0 err + test 全绿
- dev 栈 e2e（经 APISIX 网关）：登录 → 上传 `/uploads/file`（MinIO OK）→
  camelCase 发文件消息 → **SEND 响应 contentType=file** → **LIST 响应 file** ✓
  （修复前的历史行保持 text，属预期兼容）
- 回归：`smoke_data_layer.py` 11/11 + §契约 7 6/6
- web/web-bff/chat-svc 镜像重建 4/4 OK + 滚动重启 healthy

## 五、本批未做（open）

| 项 | 说明 |
|---|---|
| llm-chat-real-pipeline PR-1/PR-2 | 下一推荐项（stage-78 顺序 2）；文件"发给 Kimi 带文件引用"依赖它 |
| web 历史 typecheck 错误 96 处 | charts/DigitalHuman/FaceCamera 等遗留，非本批引入（stash 对照实证） |
| Kafka P3 / §1.4 | 不变 |
| intent-classification（重写版）| 被 llm 链路解锁后启动 |

## 六、调研依据

- 已读：web-bff/{internal/handler/chat_handler.go,viewmodel.go,upload_handler.go,
  internal/downstream/chat.go,chat_grpc.go}、chat-svc/{internal/logic/sendmessagelogic.go,
  listmessageslogic.go,internal/grpcserver/chat_server.go,internal/types/types.go}、
  proto/chat.proto、web/{app/types/api.ts,app/stores/message.ts,
  app/composables/{useFileUpload,useConversationSender,useApi}.ts,
  app/pages/chat/conversation/[id].vue,app/components/ChatFile.vue}
- 已查：stage-78 核查报告、plan 原文、git log（Stage 58/33 相关注册与幂等历史）
- 命令证据：e2e curl 全链路输出（upload OK → DB psql content_type=file → SEND/LIST 响应 file）；
  vitest 250/250；go test 全模块绿；smoke 11/11 + §契约 7 6/6；stash 对照 typecheck 差值
- 关联：stage-78 顺序 1、AGENTS.md §2.4（跨层契约教训再次实证）

---

> 最后更新：2026-09-12 by Stage 79 实施 session
> 关联：file-upload-message-extension.md（已迁 landed）、llm-chat-real-pipeline.md（下一步）
