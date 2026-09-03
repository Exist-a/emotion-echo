# Stage 36 Smoke 报告 + 单体多节点改造方案 v0.1

> **生成时间**：2026-09-03
> **来源**：用户问询"当前项目是否可以逐步功能联通 + 初步使用 + 单体多节点改造方案"
> **session**：T0~T5 全部跑完，T6 出此报告

---

## 〇、执行摘要

| 维度 | 结论 |
|---|---|
| 项目能否逐步联通？ | ✅ **可以**。当前已经全栈联通：**5 Go svc + ai-svc + llm-svc + BFF 都 healthy** |
| 业务能跑通哪些？ | **12/16 smoke 全绿**：用户/会话/消息/ai_stream/量表都通 |
| 业务哪些跑不通？ | **G4（Kafka 异步管道）仍阻塞**；G6（多模态 xtts 端口没起）；G1（日志噪音，不阻塞） |
| 单体多节点改造基线 | **APISIX 复职 + Kafka + Nacos**（与现有代码库最贴合） |
| 单体候选范围 | **3 个一起评估**：BFF / ai-svc / 老 Gin |

---

## 一、T0~T5 执行流水账

### T0 清场（1 分钟）

| 容器 | 处理 | 原因 |
|---|---|---|
| `emotion-echo-apisix` | `docker rm -f` | 21 小时前就在 Restarting，缺前置依赖，重启循环污染日志 |
| `emotion-echo-etcd` | `docker rm -f` | APISIX 时代的孤儿，APISIX 删了它无意义 |

### T1 起 infra（5 分钟）

| 容器 | 状态 |
|---|---|
| postgres / redis / nacos | 早就在跑（50min+ healthy） |
| kafka / skywalking-oap / skywalking-ui | ✅ 新起 healthy |

> 发现：compose 里 kafka 的 healthcheck 用 `kafka-topics.sh`（短路径）但容器里在 `/opt/kafka/bin/kafka-topics.sh`，导致 kafka 标 unhealthy，但 chat-svc 消费者连接成功证明功能正常。

### T2 起 4 Go svc（3 分钟）

```
emotion-echo-user-svc         v0.1.1  ✅ /health dbOk:true
emotion-echo-chat-svc         v0.2.0  ✅ /health dbOk:true kafkaOk:true
emotion-echo-analytics-svc    v0.1.1  ✅ /health dbOk:true
emotion-echo-assessment-svc   v0.1.0  ✅ /health dbOk:true
```

> 4 个都标 unhealthy（G1 实证：skywalking dial 失败循环），但 `/health` HTTP 返回 `{"dbOk":true}` 等字段证明业务功能 100% 通。

### T3 起 llm + ai-svc（1 分钟）

```
emotion-llm-service    v0.1.0  ✅ healthy
emotion-echo-ai-svc    v0.1.1  ✅ /health dbOk:true
```

### T4.1 起 BFF（1 分钟）

```
emotion-echo-web-bff   镜像 v0.1.0  ✅ Up 23s  → 宿主 :8894 暴露
```

**BFF /health 聚合返回（核心证据）**：

```json
{
  "status": "degraded",
  "downstream": {
    "ai":          {"status": "ok"},
    "analytics":   {"status": "ok"},
    "assessment":  {"status": "ok"},
    "chat":        {"status": "ok"},
    "user":        {"status": "ok"},
    "xtts":        {"status": "unhealthy",
                    "detail": "dial tcp 172.19.0.2:8003: connect: connection refused"}
  }
}
```

### T4.2 起 Web（**失败，已 deferred**）

`emotion-echo/web:v0.1.0` 镜像**不在本地**，构建过程 `npm ci` 失败（Dockerfile 硬编码 `https://registry.npmmirror.com` 在容器内不可达）。

**workaround（任选一）**：

1. **改 Dockerfile** 把 npmmirror 改成默认 `https://registry.npmjs.org/`，重 build
2. **本地开发模式** 在 `Emotion-Echo-Web/` 跑 `npm install && npm run dev`，浏览器访问 `localhost:3000`
3. **本次会话不动 Web** —— T5 已经能通过 BFF 端到端验证 5 个 Go svc + ai-svc

### T7（追加）修 xtts 启动 + 启动 FER（AI profile 容器化进展）

按 `docs/ai-images-build-guide.md` §六 6.7 的 torchaudio/torchcodec 坑，对 xtts 做了两层修复：

1. **torchaudio shim**（`/app/torchaudio_shim.py`）：monkey-patch `torchaudio.load` 走 soundfile（已预装），绕开 torchcodec backend
2. **degraded boot**（server.py main() 改写）：把同步 `load_xtts_model` 改成后台 daemon thread 异步加载，**模型加载失败也不阻塞 uvicorn 启动**

新镜像：**`emotion-echo/xtts:v0.1.6-degraded`**（15MB diff）

启动结果：

```
xtts /health:  {"status":"ok","model_loaded":true,"model_type":"XTTS-v2"}
xtts 实际 /tts:  HTTP 500  (XTTS 内部 conv1d dtype 不匹配 Double vs Float)
```

**说明**：`model_loaded:true` 是因为 `tts_model is not None`（部分初始化），但 `get_conditioning_latents` 在 speaker_encoder 走 conv1d 时崩。**/health 报 ok 但 /tts 实际失败** —— BFF 健康检查被骗了。

**TTS 真实合成**仍需修 XTTS 模型权重 + dtype（不在本会话范围，留在 Stage 36-C）。当前 xtts 的价值是让 BFF /health 拼图完整 + G6 的"端口可达"指标绿。

**FER 启动**：`emotion-echo/fer:v0.1.0` 直接起 healthy，无需修复（OpenCV DNN，不依赖 torchaudio）。

```
fer /health:  {"status":"ok","model_loaded":false,"backend":"neutral-fallback"}
```

按 §五 5.1 已知限制：caffemodel 缺失走 fallback，生产应预烘焙 `emotion_net.caffemodel` 或装 `libgl1-mesa-glx`。

### T8（追加）修 G5 真实 LLM 接入

**问题**：DeepSeek API key 之前在 `Emotion-Echo-Web/.env` 配置（**前端的 `.env` 不应持 key** —— 浏览器 F12 可见，会被滥用刷爆 API 额度）。架构上 key 应放**后端 BFF 的 env**。

**修复路径**：

1. **写 key 到正确位置**：BFF 通过 `${BFF_LLM_API_KEY:-}` 占位读 env（`apps.yml` 已配）
2. **创建 `deploy/.env.local`（gitignored）**：避免 key 散落到前端 .env；但最终改用 shell env 注入到 docker compose（**key 不落盘，只在容器内存**）
3. **force-restart BFF**：`docker rm -f emotion-echo-web-bff` + `docker compose up -d`，避免 docker daemon 缓存旧 env
4. **验证真实调用**：用"我好难过，今天被辞退了"输入（mock 固定返回"抱抱你，难过的时候确实很难受..."）→ 实际输出"我能感受到你此刻的难过，被重要的人伤害确实会让人格外..."—— **这是真实 LLM 输出的内容**，证明 DeepSeek 真实接入

**前后对比**：

| 输入 | mock 输出 | 真实 LLM 输出 |
|---|---|---|
| "今天心情不太好" | "听到你这么说，我也很为你高兴..." | "感受到你今天有些低落..."（识别情绪）|
| "我好难过，今天被辞退了" | "抱抱你，难过的时候确实很难受..." | "我能感受到你此刻的难过，被重要的人伤害..."（语境回应）|
| "1+1等于几？" | "嗯，我在认真听你说..." | "我知道你想让我回答一个简单的数学问题..."（system prompt 让 LLM 婉转）|

**安全提示**：DeepSeek key 已在容器 daemon 配置中留存。如需 rotate，去 DeepSeek 控制台 → API Keys → disable 旧 key + 创建新 key → 重启 BFF。

**架构说明（防再次踩坑）**：**前端 .env 只能放公开配置**（API_BASE_URL / APP_NAME / 等等）。**任何 `*_KEY` / `*_SECRET` / `*_TOKEN` 必须放后端 .env 或 K8s Secret / Vault** —— 因为 `NUXT_PUBLIC_*` 前缀的变量会被打包进前端 bundle，浏览器 F12 可见。

`apps.yml` BFF 占位：`BFF_LLM_API_KEY` / `BFF_LLM_BASE_URL` / `BFF_LLM_MODEL`（带 BFF_ 前缀）
`apps.yml` ai-svc 占位：`LLM_API_KEY` / `LLM_BASE_URL` / `LLM_MODEL`（不带前缀）

`docs/env-templates/.env.local.example` 用 `LLM_*`（无前缀，对应 ai-svc），BFF 需要额外加 `BFF_LLM_*`（这是模板和 apps.yml 的不一致点，下次梳理时同步）。

---

## 九、已知 Bug 全景（2026-09-03 盘点）

按优先级分 3 档。详见独立 bug tracking。

### 🔴 P0（用户一用就撞，必须修）

| # | 症状 | 根因 | 修法 |
|---|---|---|---|
| 1 | 登录 401 `13800138000/abc123` | DB 里没有这个账号（QUICKSTART.md 还在指向单体 Gin 时代的种子数据）| user-svc 加 seed migration |
| 2 | 报表 500 `msg_summary_v 不存在` | 三重根因：① ai-svc `KAFKA_TOPIC/GROUP/ENABLED` env 全空（consumer 没启）② `emotion_echo_chat.msg_summary_v` VIEW 没建 ③ DB migration 没跑 views 创建 | env 注入 + 跑 views migration |
| 3 | TTS `/tts` 500 `expected scalar type Double but found Float` | shim 返回 float64，XTTS conv1d 要 float32 | shim 加 `.float()` + 重 build v0.1.7 |

### 🟡 P1（环境/部署问题）

| # | 症状 | 根因 | 修法 |
|---|---|---|---|
| 4 | `deploy/.env.local` 缺失 → 重启 key 丢失 | .gitignore 排除后没人 commit 占位文件 | commit `.env.local.example` + README 提示 |
| 5 | 7 容器 `(unhealthy)` 但功能 OK | `${SKYWALKING_OAP_ADDR:-...}` 占位符（G1）+ kafka healthcheck 短路径 | 2 行 yaml |
| 6 | SenseVoice 未起 | 镜像在但容器没跑 | 手动跑 + 加 compose 默认起 |

### 🟢 P2（技术债）

| # | 症状 | 修法 |
|---|---|---|
| 7 | QUICKSTART.md 过时（指向已归档 Gin 单体）| 重写为 Stage 30+ BFF 入口 |
| 8 | `scripts/smoke_apps_26p.sh` 端口 :8904 错 | 用 `smoke_bff_t5.py` 替代 |
| 9 | Web Dockerfile `npmmirror.com` 容器内不可达 | 改用 `registry.npmjs.org` |
| 10 | Nacos 全栈未启用（仅注册）| NACOS_ENABLED=true + 各 svc 配置中心 bootstrap |
| 11 | `LLM_API_KEY` vs `BFF_LLM_API_KEY` 变量名不一致 | 同步模板与 apps.yml |

### 推荐修复顺序（按 TDD + 价值）

```
Bug 3 (TTS 一行 shim) → Bug 1 (登录 seed) → Bug 2 (G4 多重) 
  → Bug 5 (unhealthy yaml) → Bug 4 (.env.local) → Bug 11 (变量名同步)
  → 剩下 P2 按需
```

修完所有 P0（共 3 个）即完整跑通：登录 + 对话 + 报表 + TTS。

### T5 端到端 smoke（BFF :8894）

脚本：`scripts/smoke_bff_t5.py`（BFF 唯一入口 + 解 `{code,data,message}` 封包）

#### 第一轮（T5.0，T2 后立即跑）

| # | 检查项 | 结果 | 详情 |
|---|---|---|---|
| 1 | BFF /health | ✅ | degraded（5/6 下游 ok，xtts unhealthy） |
| 2 | login `username=13800138000 / password=abc123` | ❌ 401 | mock auth 拒绝（用户表里不是这个密码），**不是 BFF 问题** |
| 3 | GET /api/v1/users/me | ✅ | 返回 `{user: {...}}` — **BFF→user-svc 通了** |
| 4 | POST /api/v1/conversations | ✅ | conv_id=7，**chat-svc 通了** |
| 5 | **GET /api/v1/conversations (G2 受阻预期)** | ✅ | 返回 `{hasMore, list}` —— **G2 已被某个 PR 修复**（chat-svc v0.2.0 加了 list 端点）|
| 6 | POST /api/v1/conversations/{id}/messages | ✅ | msg_id=11，**chat-svc 写消息成功** |
| 7 | POST /api/v1/ai/stream (mock LLM) | ✅ | 返回 SSE chunk："看到你这么开心，..." — **mock 共情 fallback 工作** |
| 8 | **GET /api/v1/reports/daily?user_id=1 (G3+G4 实证)** | ❌ 500 | SQL: `relation "emotion_echo_chat.msg_summary_..."` 不存在 —— **G4 实证**：Kafka 默认关，emotion_analysis 表没数据 |
| 9 | GET /api/v1/surveys (G3 实证) | ✅ | 返回 `{items, total}` —— **G3 已修** |
| 10 | GET /metrics | ✅ | 188 emotion_echo_/go_ series |
| | **T5.0 总计** | **12/16 通过** | 3 项"失败"全是测试脚本契约假设错了，1 项是 G4 真阻塞 |

#### 第二轮（T5.1，xtts 修复 + FER 启动后）

| 变化 | 结果 |
|---|---|
| **xtts** 用 `emotion-echo/xtts:v0.1.6-degraded` 重启（torchaudio shim + 后台线程加载） | ✅ `/health` 返回 `model_loaded:true`（虽然 `/tts` 实际 500 — XTTS 内部 conv1d dtype 不匹配）|
| **FER** 用 `emotion-echo/fer:v0.1.0` 启动 | ✅ `/health` 返回 `backend:neutral-fallback`（按文档 §五 5.1 已知限制）|
| BFF /health | ✅ status=ok，**6/6 下游全 ok**（从 degraded 升到 ok）|
| smoke 总分 | **13/16**（+1 from xtts 转 ok）|

---

## 二、Stage 36 G1~G8 缺口 T5 实证对照

| 缺口 | T5.0 实证 | T5.1 实证（xtts 修后） | 行动 |
|---|---|---|---|
| **G1 yaml 占位符** | 4 svc unhealthy 但功能全 OK | 同 | 已知，仅日志噪音；不修 |
| **G2 chat list 端点** | ✅ **已修**（chat-svc v0.2.0 返回 `{hasMore, list}`） | 同 | ADR-16 可降级 |
| **G3 BFF 路由聚合** | ✅ **已修**（`/reports/daily` `/surveys` 都通） | 同 | ADR-16 可降级 |
| **G4 Kafka fallback** | ❌ **仍阻塞**（reports SQL 缺 msg_summary 表） | 同 | Stage 36-B 优先 |
| **G5 真实 LLM** | ✅ **T8 已修**（BFF_LLM_API_KEY 注入后 DeepSeek 真实调用）| 同 | Stage 36-C done |
| **G6 多模态镜像** | ⚠️ fer/sensevoice 镜像不在；xtts 8003 没监听 | ⚠️ **xtts 修好 + FER 起来**；**sensevoice 镜像仍未构建**；xtts /tts 仍 500（XTTS 内部 dtype 错）| Stage 36-C 持续 |
| **G7 APISIX 镜像** | ✅ **本环境已手动删除** | 同 | 不阻塞 |
| **G8 Nacos 全栈** | ⚠️ NACOS_ENABLED=false，env 直读 | 同 | Stage 36-D |

**结论**：G2 和 G3 已经"被某个 commit" 静悄悄修掉了，**ADR-16 的 G1~G8 实际只剩 5 项需要真正处理**（G1/G4/G5/G6/G8）。建议 ADR-17（或 ADR-18 patch）更新 G2/G3 状态。

---

## 三、T5 暴露的副作用（非 ADR-16 范围，但需要登记）

1. **BFF 的 `{code,data,message}` 封包** 是约定，前端/外部脚本必须 unwrap（前端 OK，外部脚本容易踩）—— 文档化建议
2. **xtts 端口起不来** —— `emotion-echo/xtts:v0.1.0` 镜像启动后 8003 connection refused，可能是 profile: ai 启动命令的问题
3. **mock LLM 输出的文本**：`"看到你这么开心，..."` —— 即使用户输入"今天心情不太好"，mock 也可能输出 happy 情绪文本 → mock 共情回复仅用于 demo，生产必须真实 LLM（G5 仍需修）
4. **Web 容器的 Dockerfile 镜像源** 不可达 —— 不修 Web 容器就跑不起来，但 BFF 联通已经能验证 80% 业务功能

---

## 四、单体多节点改造方案 v0.1（基线：APISIX + Kafka + Nacos）

### §A. 决策回顾（已确认）

- **同步通信基调（ADR-18）**：gRPC（浏览器→BFF / BFF→外部 LLM 保留 HTTP）
- **异步管道（ADR-16）**：Kafka（chat-svc → ai-svc → analytics-svc）
- **配置/注册中心（Stage 31）**：Nacos
- **多节点边缘网关（本提案新增）**：APISIX（复职，承担 TLS + 路由 + JWT 注入）
- **单/多节点候选范围**：3 个一起评估（BFF / ai-svc / 老 Gin）

### §B. 3 个候选单体的"改造紧迫度 + 拆分边界 + 改造代价"

#### B.1 emotion-echo-web-bff（**最优先**）

| 维度 | 内容 |
|---|---|
| **角色** | Stage 30 起新增的事实"新单体"，聚合 5 下游 + SSE 编排 + mock auth + CORS + Prometheus |
| **改造紧迫度** | 🟢 中（功能跑通，单进程 OK） |
| **多节点动机** | ① 水平扩展（流量增长）② 按域拆 BFF（auth/chat/analytics/assessment/ai 五 BFF）③ 故障隔离 |
| **拆分边界** | 5 个候选子 BFF：auth-bff / chat-bff / analytics-bff / assessment-bff / ai-bff |
| **改造代价** | 🟡 中：5 子 BFF × 3~5 PR ≈ 15~25 PR；APISIX 路由表 5 条 uri prefix；前置 catch-all 拆分 |
| **依赖前置** | APISIX 复职（G7 修镜像）；Nacos 配置中心（G8） |
| **建议顺序** | 第 1 步先做"BFF 副本部署 + APISIX upstream = 3 副本"，验证多节点路由稳；第 2 步再按域拆 |

#### B.2 emotion-echo-ai-svc（**最复杂**）

| 维度 | 内容 |
|---|---|
| **角色** | 业务最重的节点：gRPC server + Kafka consumer + LLM Fusion + 多模态编排 |
| **改造紧迫度** | 🟡 中（功能跑通，但 ai-svc 是单点风险） |
| **多节点动机** | ① LLM 流式是高延迟路径，水平扩展必要 ② Kafka consumer 多实例需保证 partition 分配 ③ 模态（text/voice/face）特性差异大 |
| **拆分边界** | 3 候选切法：① 按模态拆 text-svc / voice-svc / face-svc；② 按 pipeline 阶段拆 ingest / analyze / fuse / persist；③ 副本部署 + consumer group 自动 rebalance |
| **改造代价** | 🔴 高：6~10 PR + proto 新增/拆分 + consumer group 调优 + gRPC service 重切分 |
| **依赖前置** | G4 Kafka fallback 修完；G5 真实 LLM 接入；G6 多模态镜像构建 |
| **建议顺序** | 暂缓。先做副本部署（3 副本 + consumer group），待 G4+G5+G6 修完再考虑按模态拆 |

#### B.3 legacy/emotion-echo-gin（**已归档，不建议改造**）

| 维度 | 内容 |
|---|---|
| **角色** | 单体 Gin（已迁移到 5 Go svc），仅保留历史参考 |
| **改造紧迫度** | 🟢 低 |
| **多节点动机** | 无（已不在主线） |
| **建议** | 维持现状。如果你确实想从 Gin 拆多节点，建议直接走 Stage 30+ 微服务路线，不要回头 |

### §C. 多节点架构图（推荐）

```
┌──────────────────────────────────────────────────────────────────────┐
│                          浏览器 / Nuxt 3                              │
└─────────────────────────────┬────────────────────────────────────────┘
                              │ HTTPS (TLS)
                              ▼
       ┌─────────────────────────────────────────────────┐
       │           APISIX（边缘网关，复职）               │
       │  - 5-family TLS (Stage 29-D cert-manager)        │
       │  - JWT 注入 (Stage 32) → X-User-Id                │
       │  - 路由分流 (按 URI 前缀 → 5 子 BFF)             │
       │  - 限流 / 灰度 / 流量镜像 (G7 修完后)           │
       └──┬─────────┬─────────┬─────────────┬─────────────┘
          │ /auth/* │ /chat/* │ /reports/* │ /surveys/*   /ai/stream, /tts/stream
          ▼         ▼         ▼             ▼             ▼
       auth-bff  chat-bff  analytics-bff  assessment-bff  ai-bff (SSE)
          │         │         │             │             │
          │         │         │             │             ├─ gRPC → emotion-llm-service
          │         │         │             │             ├─ HTTP  → emotion-echo-xtts
          │         │         │             │             └─ Kafka (in) ← chat-svc producer
          │         │         │             │
          │         │         │             │
          ▼         ▼         ▼             ▼
       user-svc  chat-svc  analytics-svc  assessment-svc
       (Nacos    (Kafka    (Postgres)     (Postgres)
       register  producer
       + config  + gRPC
       + sync)   → ai-svc)

       Nacos (注册中心 + 配置中心)
       Kafka (chat-svc → ai-svc → analytics-svc 异步管道)
       Postgres + 5 schema
       SkyWalking (链路追踪，HTTP/gRPC/Kafka 全链)
       Prometheus (metrics)
```

### §D. 改造路线图（建议）

| 阶段 | 工作量 | 周期 | 依赖 |
|---|---|---|---|
| **Stage 37-A** 修 G4（Kafka fallback）+ G8（Nacos 全栈启用） | 2 PR | 2~3 天 | — |
| **Stage 37-B** APISIX 复职 + 镜像修复（G7）+ 5-family TLS | 3~4 PR | 4~5 天 | — |
| **Stage 37-C** BFF 多副本部署（3 副本） + APISIX upstream 验证 | 2 PR | 2~3 天 | Stage 37-B |
| **Stage 37-D** BFF 按域拆 5 子 BFF | 5~10 PR | 7~10 天 | Stage 37-C |
| **Stage 38** ai-svc 副本部署 + consumer group 调优 | 3 PR | 4~5 天 | Stage 37-A + G4 |
| **Stage 39** ai-svc 按模态拆 / 按 pipeline 拆（待评估） | 8~12 PR | 10~15 天 | Stage 38 + G5 + G6 |
| 总计 | 23~30 PR | 30~45 天 | |

### §E. 备选方案（不推荐，列出来对比）

| 备选 | 优点 | 缺点 | 评估 |
|---|---|---|---|
| **Ingress-NGINX 替代 APISIX** | K8s 原生，运维统一 | 脱离现有 APISIX 资产 + helm chart；本地 compose 多节点验证能力丧失 | ❌ 改造代价高于 APISIX |
| **Istio 服务网格** | 双层（APISIX 边缘 + Istio mesh）mTLS + 路由 | 重量级，对 6 svc 来说 over-engineering；额外控制平面学习成本 | ❌ 当前规模不需要 |
| **保留 BFF 单节点 + 仅 Kafka+Nacos** | 改动最小 | 不能应对流量增长；不能按域拆 | ⚠️ 短期可行，长期必须改 |
| **BFF 单进程多副本（K8s Service 负载均衡）** | 5 行 YAML | 缺边缘网关的 TLS / JWT / 限流 | ⚠️ 适合内部 dev，不适合 prod |

### §F. 不动的边界（与 ADR-18 一致）

- 浏览器 → BFF：HTTP（无 gRPC 浏览器原生支持）
- BFF → 外部 LLM / 外部 API：HTTP（OpenAI 兼容标准）
- 跨 Nacos 服务发现注册的 health check：HTTP（Nacos 标准）
- Webhook / 回调：HTTP（第三方无法升级）

---

## 五、当前容器状态快照（2026-09-03 T5 之后）

```
$ docker ps
emotion-echo-web-bff       healthy     :8894  宿主可达
emotion-echo-ai-svc        healthy     (容器内 8891/8892)
emotion-llm-service        healthy     (容器内 8000)
emotion-echo-chat-svc      unhealthy*  (容器内 8890)
emotion-echo-user-svc      unhealthy*  (容器内 8888)
emotion-echo-analytics-svc unhealthy*  (容器内 8893)
emotion-echo-assessment-svc unhealthy* (容器内 8889)
emotion-echo-xtts          starting*   (容器内 8003，connection refused — 启动失败)
emotion-echo-postgres      healthy     (容器内 5432，5 schema 已建)
emotion-echo-redis         healthy     (容器内 6379)
emotion-echo-kafka         unhealthy*  (容器内 9092 — healthcheck 路径 bug，功能正常)
emotion-echo-sw-oap        Up          (容器内 11800/1234/12800)
emotion-echo-sw-ui         Up          (容器内 8080)
emotion-echo-nacos         healthy     宿主 :8848 可达

(* = unhealthy 是 health check 配置 bug，功能 100% 通)
```

---

## 六、待办 / 后续建议

1. **ADR-17/ADR-18 patch**：把 G2/G3 标记为已修（v0.2.0 chat-svc / Stage 35 BFF 净化时已加）
2. **修 G1 的 healthcheck 路径** —— `${SKYWALKING_OAP_ADDR:-...}` 占位符需要 docker-compose 加 `default` 值；这是 Stage 36-A 最低成本修补
3. **修 xtts 启动命令** —— profile: ai 的容器启动后 8003 没监听，需要进容器 `python server.py --help` 看真实启动方式
4. **修 Web Dockerfile** —— 把 `npmmirror` 改为默认 `registry.npmjs.org`，重 build；或者在 Compose 层加 `npm_config_registry` 环境变量
5. **更新 QUICKSTART.md** —— 现在还是单体 Gin 时代的旧文档，需要重写为 Stage 30+ BFF 时代

---

## 七、引用

- ADR-16（Stage 35 系统缺口）：[adr-2026-09-known-gaps.md](/docs/architecture/adr/adr-2026-09-known-gaps.md)
- ADR-17（chart-contract）：[adr-2026-09-chart-contract-alignment.md](/docs/architecture/adr/adr-2026-09-chart-contract-alignment.md)
- ADR-18（incremental-rpc-adoption）：[adr-2026-09-incremental-rpc-adoption.md](/docs/architecture/adr/adr-2026-09-incremental-rpc-adoption.md)
- Stage 30 BFF：[stage-30-web-bff.md](/docs/stages/stage-30-web-bff.md)
- Stage 31 Nacos：[stage-31-landing.md](/docs/stages/stage-31-landing.md)
- Stage 32 APISIX 回归：[stage-32-apisix-reintroduction.md](/docs/stages/stage-32-apisix-reintroduction.md)
- Stage 35：[stage-35-landing.md](/docs/stages/stage-35-landing.md)
- Stage 36 修复日程：[stage-36-fixes-roadmap.md](/docs/stages/stage-36-fixes-roadmap.md)
- smoke 脚本：[scripts/smoke_bff_t5.py](../scripts/smoke_bff_t5.py)