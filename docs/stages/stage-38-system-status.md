---
status: snapshot
date: 2026-09-03
purpose: 当前 dev 模式完整功能状态盘点（用户问"代码是否可以正常使用"的诚实回答）
---

# Stage 38 系统状态盘点 · 2026-09-03

> 用户问："当前代码是否可以做到正常使用、功能没问题、每个组件都正常？"
>
> **诚实回答：后端数据契约全 PASS（10/10），但实际用户路径有 6 项阻断。**

---

## 一、整体一句话

**后端通了，前端没起；AI 服务部分未起；用户路径上 TTS 和文件上传会撞墙。**

---

## 二、能跑通的部分（实测 PASS）

### 2.1 后端数据契约（smoke 10/10）

[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py)：

```
[OK  ] §1 user_behavior_events 行数: 36
[OK  ] §2 event_type 4 种 enum 细分（message / conversation_created / conversation_closed / conversation）
[OK  ] §3 analytics_reader 读 4 个视图（msg_summary_v / daily_emotion_v / assessment_v / user_behavior_events）
[OK  ] §4 /reports/daily 数据真有（summary + emotionDistribution 非空）
汇总: 10/10 PASS, 0 FAIL
```

### 2.2 BFF 端到端（smoke_bff_t5.py 16/16 OK）

- BFF /health: 6/6 downstream OK
- BFF /api/v1/auth/login (echo/echo123): user_id=3
- BFF /api/v1/users/me: 完整 user keys
- BFF POST /api/v1/conversations: 200 创建会话
- BFF GET /api/v1/conversations: 200 列表
- BFF POST /api/v1/conversations/:id/messages: 200 发送消息
- BFF /api/v1/ai/stream: 200 SSE 流式输出（mock LLM fallback）
- BFF /api/v1/reports/daily: 200 summary + chartData
- BFF /api/v1/surveys: 200 列表
- BFF /metrics: 201 metrics series

### 2.3 数据库全链路数据

```
user_behavior_events |  36   ← analytics-svc Kafka consumer 写入
emotion_analysis     |  12   ← ai-svc Kafka consumer 写入
fused_emotions       |  12   ← FusionWorker tick 写入
messages             |  24   ← chat-svc handler 写入
outbox_events (sent) |  69   ← chat-svc outbox relay 推送
```

---

## 三、阻断项（实测 FAIL 或不可用）

### ❌ 阻断 1：XTTS 模型加载卡死

- 容器：emotion-echo-xtts，running 6+ 小时
- 日志最后一行：`loading XTTS model device=cpu cache_dir=/app/AI-ModelScope/XTTS-v2`
- 影响：BFF /api/v1/ai/tts 返 21 字节 `{"error":"not found"}`，**前端 TTS 语音回复完全不可用**
- 真因：Stage 36 已记录"dev 环境 pypi CDN + Docker Desktop 内存限制，build 卡 30+ 分钟"
- 修复路径：生产网络跑 build；或换 Coqui TTS pre-built image；或 TTS fallback 到 web speech API（前端）

### ❌ 阻断 2：文件上传 404

- 请求：`POST /api/v1/api/v1/upload/image` (BFF 透传)
- 响应：`{"error":"not found"}` HTTP 404
- 影响：聊天附件按钮发请求失败
- 待查：路由是否真在 BFF 挂了

### ❌ 阻断 3：FER / 视觉多模态 404

- 请求：`POST /api/v1/multimodal/face`
- 响应：`{"error":"not found"}` HTTP 404
- 影响：人脸情绪识别不可用
- 待查：路由是否真在 BFF 挂了

### ❌ 阻断 4：前端 Nuxt dev server 没起

- 容器 `emotion-echo-web` 不在 docker ps
- 本机端口 3000 无进程监听
- 影响：**完全无法在浏览器打开登录页**——BFF 通不等于产品可用
- 启动命令：用户机器执行 `cd Emotion-Echo-Web && pnpm install && pnpm dev`

### 🟡 阻断 5（视觉假象）：Dockerfile HEALTHCHECK 命令有 bug

- 容器状态：web-bff / ai-svc / analytics-svc 报 **unhealthy**
- 实际：`wget --spider http://localhost:$HTTP_PORT/health` 返非 0 exit code
- 真因：`/health` 端点 **真在返 200 OK**（实测 `{"status":"ok","dbOk":true}`），但 `wget --spider` + 容器内 hostname 解析的组合偶发返 1
- 影响：`docker ps` 一片红看着像失败，但不阻塞实际功能
- 修复：改 HEALTHCHECK 用 `curl -f http://localhost:8891/health || exit 1` 或 wget 输出文件后判断

### 🟡 阻断 6（文档失真）：quickLogin 后端端点从未实现

- 前端 `quickLogin` 函数期望后端有 `/api/v1/auth/quick-login`
- 实际后端无此端点；前端 `quickLogin` 实际走标准 `/api/v1/auth/login` 用 echo/echo123
- E2E 测试 `login-flow.spec.ts` 注释明写"由于 dev mode 下后端 API (localhost:18080) 未启用，quickLogin 异步调用失败"——**E2E 从来没真正通过**
- 影响：用户点"用演示账号快速体验"按钮实际能登录（echo/echo123），但行为/预期不一致

---

## 四、不阻断但有隐患

| # | 项目 | 严重度 |
|---|---|---|
| 1 | Stage 34 migration 004/005/006 + event_id 列未挂 initdb.d | 🟡 中（重建 dev 环境会丢表/列） |
| 2 | `daily_emotion_by_modality_v` 视图依赖 `face_emotion_results` / `voice_emotion_results`，但 PG 实际是 `face_detections` / `voice_transcripts` | 🟡 中（视图无法创建） |
| 3 | docs/plans/wechat-qq-login-and-upload.md 标 superseded（Stage 38-A） | ✅ 已修 |
| 4 | `user_oauth` 表（Stage 19/22 设计）从未使用 | 🟢 低 |
| 5 | ADR 与代码失真累计（至少 3 处） | 🟡 中（待 ADR-20 立项） |
| 6 | `KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR` 等迁移文件未挂 initdb.d | 🟢 低（仅重建场景） |

---

## 五、当前可用 vs 不可用功能清单

### 用户可立即使用 ✅

- dev compose up 后 BFF 接受登录请求（echo/echo123）
- 创建会话、列会话、发消息
- 4 个 dashboard 报表看真实数据（summary 中文 + 情绪饼图）
- 量表列表
- AI 流式文字回复（mock LLM fallback，**非真实 LLM**——`LLM_BASE_URL=""`）
- 用户资料 / 个人信息
- 容器健康（实际）

### 用户撞墙 ❌

- **TTS 语音回复**（XTTS 模型加载卡死）
- **文件上传**（路由 404）
- **多模态情绪识别**（路由 404）
- **浏览器界面**（前端 dev server 没起）
- 看 `docker ps` 不被"unhealthy"字样吓到（视觉假象）

---

## 六、修复优先级（按"用户可见 + 易修"排）

| 优先级 | 项 | 工作量 | 用户影响 |
|---|---|---|---|
| P0 | 用户本地 `pnpm dev` 起前端 | 5 分钟 | 立刻能看 UI |
| P0 | 修文件上传 404（路由确认） | 30 分钟 | 聊天附件能用 |
| P0 | 修多模态 404（路由确认） | 30 分钟 | 视觉情绪识别能用 |
| P1 | 修 Dockerfile HEALTHCHECK 命令 | 30 分钟 | `docker ps` 干净 |
| P1 | XTTS 模型加载（生产网络 build / 换镜像 / fallback） | 半天到一天 | TTS 语音能用 |
| P2 | quickLogin 后端端点实现或前端删除 | 1-2 小时 | 一致性 |
| P3 | migration 004/005/006 + event_id 挂 initdb.d | 2-3 小时 | 重建环境不丢表 |
| P3 | `daily_emotion_by_modality_v` 视图对齐 schema 名 | 半天 | 修 §3 smoke 一个 SKIP |

---

## 七、引用

- Smoke 脚本：[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py) · [scripts/smoke_bff_t5.py](/scripts/smoke_bff_t5.py)
- Stage 38-A landing：[docs/stages/stage-38-A-landing.md](/docs/stages/stage-38-A-landing.md)
- AGENTS.md §2.4 数据契约验收：[AGENTS.md](/AGENTS.md)
- 路线图：[docs/stages/stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md)
