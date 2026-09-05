---
status: snapshot
date: 2026-09-03
revised: 2026-09-04
purpose: 当前 dev 模式完整功能状态盘点（用户问"代码是否可以正常使用"的诚实回答）
revision-note: |
  2026-09-04 复核：本文原 6 项阻断中 3 项为误诊（阻断 2/3 是路径契约不一致而非路由缺失，
  阻断 5 已不复现），§四 隐患 1 反向升级为已发作、隐患 2 撤回。
  各节内已就地标注，勿直接引用未标注的原文结论。
---

# Stage 38 系统状态盘点 · 2026-09-03（2026-09-04 复核修订）

> 用户问："当前代码是否可以做到正常使用、功能没问题、每个组件都正常？"
>
> **原答（2026-09-03）：后端数据契约全 PASS（10/10），但实际用户路径有 6 项阻断。**
>
> **复核后（2026-09-04）：后端数据契约仍 10/10 PASS；实际阻断只有 2 项**——
> 前端 dev server 未起、TTS 不可用。原 6 项里 3 项是误诊或已消失，1 项性质变了
> （文件上传不是路由 404，是后端有意未实现的 502 占位）。
> 同时发现一项原文低估的真问题：dev 库 13 个服务迁移**一个都没应用过**（§四）。

---

## 一、整体一句话

**原文**：后端通了，前端没起；AI 服务部分未起；用户路径上 TTS 和文件上传会撞墙。

**2026-09-04 修订**：后端通且容器全 healthy，前端仍没起（唯一"产品不可用"级阻断）；
AI 本地模型容器（fer / sensevoice / xtts）已在 Stage 39 主动删除，故 TTS 撞墙；
文件上传后端有意未实现；多模态其实是好的，之前是路径打错。
**环境层面另有一个更严重的问题：迁移没有自动应用机制，库随时可能是半成品状态。**

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

> **2026-09-04 更新（现状已变 + 本条的 404 依据同样有误）**
>
> 1. **XTTS 容器已不存在**。Stage 39 §五把它连同 fer / sensevoice / sw-oap / sw-ui
>    一起删除（依据：ai-svc 容器 env 无 `XTTS_BASE_URL`，`aiclient` 空即返回 nil）。
>    所以现在不是"模型加载卡死"，是**下游根本没有实例**。
> 2. **本条写的 `/api/v1/ai/tts` 返 404 不能作为 TTS 不可用的证据**——该路径
>    从来就不存在。BFF 实际注册的是 `POST /api/v1/tts/synthesize` 与
>    `/api/v1/tts/stream`（`tts_handler.go:35-36`）。
> 3. 实测正确路径：`POST /api/v1/tts/synthesize` → **503**（下游 XTTS 不可达），
>    不是 404。BFF `/health` 里 xtts 一项也如实报 `unhealthy`：
>    `Get "http://emotion-echo-xtts:8003/health": context deadline exceeded`。
>
> 结论不变——**TTS 仍然不可用**，但真因是"容器已删 + 镜像构建受阻"，
> 而非"路由缺失"。前端若在调 `/api/v1/ai/tts`，那是另一处需要一并修的路径错。

### ⚠️ 阻断 2：文件上传 404 → **误诊，实为路径不一致 + 后端有意未实现**

- 原记录：请求 `POST /api/v1/api/v1/upload/image` (BFF 透传)，响应 `{"error":"not found"}` HTTP 404
- 原记录待查项："路由是否真在 BFF 挂了"

> **2026-09-04 查清**：路由**挂了**，只是路径名不同。
>
> BFF 实际注册的是 `POST /api/v1/uploads/:kind`（`upload_handler.go:27`，注意是复数
> `uploads`），`main.go:232` 有 `handler.NewUploadHandler().Register(r)`。
> 打正确路径实测：`POST /api/v1/uploads/image` → **502**，body
> `{"message":"uploads not implemented (Stage 31)"}`。
>
> 即 502 是 **Stage 30 T4.58 有意留的占位**——`upload_handler.go` 头部注释写明
> "upload 真支持留给 Stage 31（引入对象存储）。首期统一返回 502，前端可感知
> '未实现' 而不是 404"。
>
> 所以此项要拆成两件事：
> 1. **前端路径写错**（`upload` 单数 + 重复 `/api/v1` 前缀）——前端侧修，成本极低
> 2. **上传后端确实没实现**——需要先做对象存储选型，非 30 分钟能收口

### ⚠️ 阻断 3：FER / 视觉多模态 404 → **误诊，端点完全正常**

- 原记录：请求 `POST /api/v1/multimodal/face`，响应 `{"error":"not found"}` HTTP 404
- 原记录待查项："路由是否真在 BFF 挂了"

> **2026-09-04 查清**：路由挂了且**功能是好的**，原探测打了一个不存在的路径。
>
> BFF 实际注册的是 `POST /api/v1/multimodal/analyze`（`multimodal_handler.go:34`），
> **没有 `/face` 子路由**——模态由请求参数 `kind` 区分（`text|image|audio`），不是路径。
>
> 另一个坑：handler 用 `c.PostForm` 读参数（`multimodal_handler.go:38,46`），
> **只吃 form-data / x-www-form-urlencoded，发 JSON 会报 "kind is required"**。
>
> 实测（form 编码）：
> ```
> $ curl -X POST .../api/v1/multimodal/analyze -F "kind=text" -F "text=..."
> {"code":0,"data":{"kind":"text","emotion":"neutral","confidence":0,
>                   "model":"keyword-stub-v1"},"message":"ok"}
> ```
>
> 所以本项**不是阻断**，是前后端契约不一致（路径 + 编码方式）。前端侧修即可。

### ❌ 阻断 4：前端 Nuxt dev server 没起

- 容器 `emotion-echo-web` 不在 docker ps
- 本机端口 3000 无进程监听
- 影响：**完全无法在浏览器打开登录页**——BFF 通不等于产品可用
- 启动命令：用户机器执行 `cd emotion-echo-web && pnpm install && pnpm dev`

### 🟡 阻断 5（视觉假象）：Dockerfile HEALTHCHECK 命令有 bug

- 容器状态：web-bff / ai-svc / analytics-svc 报 **unhealthy**
- 实际：`wget --spider http://localhost:$HTTP_PORT/health` 返非 0 exit code
- 真因：`/health` 端点 **真在返 200 OK**（实测 `{"status":"ok","dbOk":true}`），但 `wget --spider` + 容器内 hostname 解析的组合偶发返 1
- 影响：`docker ps` 一片红看着像失败，但不阻塞实际功能
- 修复：改 HEALTHCHECK 用 `curl -f http://localhost:8891/health || exit 1` 或 wget 输出文件后判断

> **2026-09-04 更正：已不复现。** `docker ps` 显示 user / chat / analytics / assessment /
> ai / web-bff 六个业务容器全部 `(healthy)`，llm-service 修好后也是 `(healthy)`。
> 本条与 Stage 39 §七.5 同步关闭。

### 🟡 阻断 6（文档失真）：quickLogin 后端端点从未实现

- 前端 `quickLogin` 函数期望后端有 `/api/v1/auth/quick-login`
- 实际后端无此端点；前端 `quickLogin` 实际走标准 `/api/v1/auth/login` 用 echo/echo123
- E2E 测试 `login-flow.spec.ts` 注释明写"由于 dev mode 下后端 API (localhost:18080) 未启用，quickLogin 异步调用失败"——**E2E 从来没真正通过**
- 影响：用户点"用演示账号快速体验"按钮实际能登录（echo/echo123），但行为/预期不一致

---

## 四、不阻断但有隐患

| # | 项目 | 严重度 |
|---|---|---|
| 1 | Stage 34 migration 004/005/006 + event_id 列未挂 initdb.d | 🔴 高（已实际发作，见下） |
| 2 | `daily_emotion_by_modality_v` 视图依赖 `face_emotion_results` / `voice_emotion_results`，但 PG 实际是 `face_detections` / `voice_transcripts` | ✅ 误诊已撤回，见下 |
| 3 | docs/plans/wechat-qq-login-and-upload.md 标 superseded（Stage 38-A） | ✅ 已修 |
| 4 | `user_oauth` 表（Stage 19/22 设计）从未使用 | 🟢 低 |
| 5 | ADR 与代码失真累计（至少 3 处） | ✅ 已立项为决策 18（实测已达 6 处，见下） |
| 6 | `KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR` 等迁移文件未挂 initdb.d | 🟢 低（仅重建场景） |

> **2026-09-04 更正：隐患 1 升级为已发作，隐患 2 撤回**
>
> **隐患 1 不是"重建 dev 才会丢"，当时的 dev 库就已经缺。** 合并 main 前跑 §2.4 smoke，
> `POST /conversations/{id}/messages` 返 500，chat-svc 日志
> `column "client_msg_id" of relation "messages" does not exist (SQLSTATE 42703)`。
> 排查发现 `deploy/docker-compose.infra.yml:28-31` 只挂了 `deploy/db/01~04`，
> 而 `emotion-echo-{ai,analytics,chat}-svc/migrations/` 下 13 个 .sql **一个都没被应用过**：
> 缺 `messages.client_msg_id` 列、缺 `analytics_reader` 角色、缺 `msg_summary_v` 等视图、
> 缺 `user_behavior_events.event_id` 列。补第一个迁移后 smoke 一度只剩 1/10 PASS，
> 按依赖顺序全部应用（全部幂等，无报错）后恢复 10/10 PASS。
>
> 范围也比原记录大：不止 Stage 34 的 004/005/006，而是**全部 13 个服务迁移**。
> 根因是没有任何自动应用机制，不是"某几个文件漏挂"；真正的修法（纳入 initdb.d
> 或引入 migrate 工具）**尚未做**，留作独立待办。
>
> **隐患 2 是误诊，撤回。** `daily_emotion_by_modality_v` 建不出来与表名无关：
> 它依赖的 `face_emotion_results` / `voice_emotion_results` 就在未被应用的
> `emotion-echo-ai-svc/migrations/002`、`003` 里。按顺序应用 002/003 后再跑 005，
> `CREATE VIEW` 直接成功。`face_detections` / `voice_transcripts` 是另一组表，与该视图无关。

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

> **2026-09-04 更正后的清单**（原清单里 3 条依据有误，见 §三）
>
> 真正撞墙的只剩两条：
> - **浏览器界面** —— 前端 dev server 仍未起，3000 端口无监听。这是唯一
>   "产品完全不可用"级别的项。
> - **TTS 语音回复** —— XTTS 容器已删（Stage 39 §五），`/api/v1/tts/synthesize`
>   返 503。
>
> 从撞墙清单移出：
> - **多模态情绪识别** → 移到"可用"。`/api/v1/multimodal/analyze` 实测返
>   `code:0`，功能正常，原 404 是打错路径。
> - **`docker ps` unhealthy** → 移到"可用"。六个业务容器现全部 `(healthy)`。
>
> 性质改变：
> - **文件上传** → 不是"路由 404"，是后端有意未实现（502 占位，等对象存储）
>   + 前端路径写错（`upload` vs `uploads`）。

---

## 五之二、前后端路径契约不一致清单（2026-09-04 新增）

§三 的三条误诊有同一个根源：**前端/文档使用的路径与 BFF 实际注册的路径不一致**。
BFF 对未匹配路径统一返 `{"error":"not found"}` 404（`main.go` 的 `r.NoRoute`），
所以任何路径写错都长得像"功能没实现"。完整对照：

| 前端/文档在用 | BFF 实际注册 | 打对路径后的真实结果 |
|---|---|---|
| `/api/v1/upload/image` | `/api/v1/uploads/:kind` | 502（有意占位，未实现） |
| `/api/v1/multimodal/face` | `/api/v1/multimodal/analyze`（模态走 `kind` 参数） | 200 `code:0` 正常 |
| `/api/v1/ai/tts` | `/api/v1/tts/synthesize`、`/api/v1/tts/stream` | 503（XTTS 已删） |

额外注意：`/api/v1/multimodal/analyze` 用 `c.PostForm` 取参，**发 JSON 会被拒**，
必须 form-data / x-www-form-urlencoded。

建议后续：给 BFF 补一份路由清单契约测试（断言注册路径集合），
并让前端 API 层集中定义路径常量，避免逐处硬编码再次漂移。

---

## 六、修复优先级（按"用户可见 + 易修"排）

| 优先级 | 项 | 工作量 | 用户影响 |
|---|---|---|---|
| P0 | 用户本地 `pnpm dev` 起前端 | 5 分钟 | 立刻能看 UI |
| P0 | ~~修文件上传 404（路由确认）~~ → 前端改路径为 `/api/v1/uploads/:kind` | 10 分钟 | 从 404 变成可感知的 502"未实现" |
| P0 | ~~修多模态 404（路由确认）~~ → 前端改路径 `analyze` + 改 form 编码 | 30 分钟 | 多模态情绪识别能用（后端本就正常） |
| P1 | ~~修 Dockerfile HEALTHCHECK 命令~~ | — | **已不复现**，容器全 healthy |
| P1 | XTTS 重建（生产网络 build / 换镜像 / 前端 fallback web speech） | 半天到一天 | TTS 语音能用 |
| P1 | 文件上传后端实现（需先做对象存储选型） | 1-2 天 | 聊天附件真正可用 |
| P2 | quickLogin 后端端点实现或前端删除 | 1-2 小时 | 一致性 |
| P1 | ~~migration 挂 initdb.d~~ → migrations 自动应用机制 | 2-3 小时 | **升级为 P1**：dev 库实测 13 个迁移全未应用，环境不可重建。✅ **2026-09-04 已落地**（commit 0d17e85）—— 加 `emotion-echo-db-migrate` init 容器，5 个业务 svc 依赖其 service_completed_successfully，14 条迁移契约全绿；**但**修这个 bug 的过程中发现 initdb 链本身有 3 处静默断点（详见 §八），不是单纯加个容器就完事。 |
| — | ~~`daily_emotion_by_modality_v` 视图对齐 schema 名~~ | — | **误诊撤回**，应用 ai-svc 002/003 后视图正常创建 |

---

## 八、2026-09-04 实测：initdb 链本身有三处静默断点

修迁移容器的过程中，**`down -v` 全新起**才暴露出 initdb 链是脆的——任何一条 DDL
报错都会**静默掐断后续所有脚本**。三个具体断点：

1. **`emotion_analysis` 表被两个文件重复定义且列不一致**。
   `01-create-schemas.sql:121` 建表时**没有** `event_id`，`02:117` 的
   `CREATE UNIQUE INDEX (event_id)` 因此报 "column does not exist"，中断整条
   initdb 链 → 03 种子用户和 04 视图都不执行 → 全新环境无 echo 测试账号 → 登录 401。
   修复：01 补 `event_id` 列，两边列定义保持一致。

2. **Stage 36-D "Bug 2 已修"实际包错语句**。`02:118-119` 注释白纸黑字说"上面
   CREATE UNIQUE INDEX 单独容错"，但实际 `DO` 块里包的是再下一条索引——真正会
   失败的那条**从未被保护**。这条"已修"从写下的那天起（2026 早期）就没生效过，
   但 Stage 36-D 报告把它标为已修，之后无人 `down -v` 验证。属于决策 18 §三
   "未复跑即记录"。修复：把 02 里那条同名索引删掉，让 `ai-svc/migrations/001`
   独占以 `CONSTRAINT` 形式创建（与 GORM OnConflict 匹配），并把两处注释对齐。

3. **`emotion-echo-ai-svc/migrations/001` 守卫只看 `pg_constraint`，不看 `pg_class`**。
   同名对象可能以索引形式存在（旧版 02 历史上建过同名 UNIQUE INDEX）。
   守卫放行后 ADD CONSTRAINT 报 `relation "...", already exists`，迁移失败。
   修复：守卫放宽到同时检查 `pg_class`（即使 ② 已修，仍为兼容已存在该索引的环境）。

**教训**：迁移不能只靠"代码看起来对"。**空库场景必须真跑一次**——而 initdb.d 的
"数据卷为空才执行"特性恰恰让这种验证被忽略。任何会修改 schema 的迁移/视图/角色
变更，都应该有一次空库验证 + 一条契约测试断言"重新建库后 smoke 仍 10/10"。

本节记录的全部修复已并入 `0d17e85` 提交。`db-migration-auto-apply.md` plan 已
落地，但**反向结论**同样重要：**plan 落地的真正验收不是"加个 init 容器"，而是
"down -v 后 smoke 10/10"**。

---

## 九、引用

- Smoke 脚本：[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py) · [scripts/smoke_bff_t5.py](/scripts/smoke_bff_t5.py)
- Stage 38-A landing：[docs/stages/stage-38-A-landing.md](/docs/stages/stage-38-A-landing.md)
- AGENTS.md §2.4 数据契约验收：[AGENTS.md](/AGENTS.md)
- 路线图：[docs/stages/stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md)
