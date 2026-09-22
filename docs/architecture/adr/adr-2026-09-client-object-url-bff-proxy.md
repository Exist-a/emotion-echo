# ADR-2026-09: 客户端对象 URL 走网关相对路径（BFF 反代），不向客户端下发存储绝对地址

- **状态**：已采纳（2026-09-22）
- **关联**：账本 E2E-F-113（触发实测）、E2E-F-116（存量 avatar/uploads 同型债）、PR #59、E2E-16 plan §5 测试点 2/3/4a

## 背景

E2E-F-113 实测（2026-09-22）：`voice_handler` 返回的 `audioUrl` 由 `storage.GetObjectURL()` 用
`PublicBaseURL`（dev = `http://localhost:9000`）拼成**绝对地址**下发给前端。三视角对比：

| 视角 | 结果 |
|------|------|
| 宿主浏览器（Docker 9000 端口转发仅对宿主生效） | 200 —— **碰巧**可达 |
| web 容器内部 `curl localhost:9000` | `000` 立即 RST（容器自己没监听 9000） |
| docker 网络内部 `curl emotion-echo-minio:9000` | 200（容器 DNS 正常） |

后果：`<audio>` 在非宿主浏览器 / 任何未做宿主端口转发的部署下 `readyState=0`，语音气泡静默不可回放。
同型问题在 avatar / uploads 响应中**同样存在**（`PublicBaseURL` 绝对地址），宿主浏览器可用纯属端口转发巧合。

## 决策

1. **面向客户端 fetch/播放的对象 URL 一律返回网关相对路径**（如 `/api/v1/voice/audio/<filekey>`），
   由 BFF 提供反代端点从 MinIO **流式读取**（`storage.StorageClient.GetObject(ctx, key)` →
   `io.ReadCloser` + 真实 `Content-Type`/`Content-Length` → `io.Copy` 给 writer）。
2. **`PublicBaseURL` 只作 BFF 内部用途**（构造日志/内部引用），不作为"客户端可达"的依据；
   任何客户端需要 fetch 的对象地址不得使用存储服务的绝对地址。
3. 反代端点与其它 `/api/v1/*` 同走 APISIX `jwt-auth`，对象访问继承网关鉴权/限流；
   端点自带防御：key 为**单段**路径参数（gin `:filekey`）、拒 `..`/`/`/空（handler 层 400 + 路由层 404 双层）、
   对象不存在返 **404**（绝不 200 + 空 body，避免 `<audio>` 静默 `readyState=0`）、
   storage 未配置返 503（与"看似成功但回放空白"语义一致）。

## 后果

- **全环境一致**：宿主 / 远程协作 / 生产走同一路径，host 与存储部署解耦，无需按环境切 `PublicBaseURL`。
- **每类新对象需要一个反代端点**：voice 已落地（PR #59）；avatar / uploads 仍是 `PublicBaseURL`
  绝对地址下发 ⇒ 同型存量债登记 **E2E-F-116**（归 E2E-27 对象存储阶段迁移）。
- 验收口径升级：任何对象 URL 的测试点必须在**发起者视角**断言可达（E2E-16 plan §5 测试点 2/3/4a
  双视角 curl + `readyState ≥ 1`），仅 `curl -I 宿主` 200 不算通过（AP-01 第四次复发的根治条款）。

## 增补（2026-09-22）：对象 URL 在消息中的持久化契约

语音消息落库（E2E-16 测试点 #5，PR #62）把本 ADR 的决策 1 延伸到存储层：

1. **消息行持久化网关相对 URL**：`messages.content = /api/v1/voice/audio/<filekey>`、
   `content_type = 'audio'`（不存 `PublicBaseURL` 绝对地址——否则又把 host 可达性问题
   写进数据库）。
2. **加载契约**：`loadMoreMessages` 对 `contentType==='audio'` 行做 `audioUrl = content`
   映射——后端行没有 `audioUrl` 字段，气泡渲染条件是 `item.audioUrl`，不映射则
   "落库成功但刷新后气泡照样消失"（与 F-113 同族的可达性/呈现断链）。
3. **绑定契约**：语音消息的 `ai/stream messageId` 必须用落库返回的服务端 id
   （`face_emotion_results.message_id` / 融合按 message_id 取行，绑随机 clientMsgId 会取空）。

已知外观项：会话侧栏 `lastMessage` 刷新后显示该 URL（与既有文件消息同型）。
