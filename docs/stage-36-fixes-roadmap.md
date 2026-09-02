# Stage 36 · 缺口修复路线图（Fixes Roadmap）

> 状态：**待审批（Pending Approval）** · 日期：2026-09-01 · 目标分支：`feat/bff-fused-emotion-endpoint`（沿用）
> ADR-16：[adr-2026-09-known-gaps.md](adr-2026-09-known-gaps.md)
> 来源：[stage-35-system-feasibility.md](stage-35-system-feasibility.md) §五 §六

---

## 一、目标

把 ADR-16 列出的 **8 项已知缺口** 全部修复，每项建立独立 PR + 测试 + 收口文档。

---

## 二、批次划分（基于 ADR-16 §B）

| 批次 | 范围 | 目标 | 预计工作量 |
|------|------|------|------------|
| **Stage 36-A** 立即修 | G1 + G3 | 解日志噪音 + 前端"分析报表/量表"模块上线 | 1-2 天 |
| **Stage 36-B** 高优先 | G2 + G4 | 解前端"会话列表" + 消息自动情绪分析 | 2-3 天 |
| **Stage 36-C** 中优先 | G5 + G6 | 真实 LLM + FER/SenseVoice 集成 | 3-5 天 |
| **Stage 36-D** 低优先 | G7 + G8 | APISIX 镜像切换 + Nacos 全栈验证 | 2-3 天 |

每批次单独 landing doc 收口。

---

## 三、Stage 36-A（立即修，预计 1-2 天）

### PR-A1：4 个 Go svc yaml 占位符修复（G1）

**问题**：user-svc / chat-svc / analytics-svc / assessment-svc / web-bff 的 yaml 都含 `${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}` 字面占位符 → go2sky dial 失败循环。

**修复方案**（复用 Stage 35 PR-7 模式）：
- yaml 字段省略 bool/string 字段 → 走 Config struct tag default
- env override 由 main.go `applyEnvOverrides` 注入（已存在）
- 验证：`SKYWALKING_ENABLED=false` 应禁用 tracer

**TDD 节奏**：
```
PR-A1.1: user-svc yaml + applyEnvOverrides 测试（RED → GREEN → REFACTOR）
PR-A1.2: chat-svc yaml 同上
PR-A1.3: analytics-svc yaml 同上
PR-A1.4: assessment-svc yaml 同上
PR-A1.5: web-bff yaml 同上
```

**验收**：5 个 svc 都启动后 logs 无 "too many colons in address"。

---

### PR-A2：BFF 路由聚合 analytics / assessment（G3）

**问题**：BFF 当前没有把 `/api/v1/reports/*` 和 `/api/v1/assessments/*` 代理到下游 svc，前端这两个模块打不开。

**修复方案**：
- BFF `internal/downstream/` 加 `AnalyticsClient` + `AssessmentClient`（参考 `EmotionQueryClient` 实现）
- BFF `internal/handler/` 加对应路由
- 测试：httptest mock 下游响应 + 验证 BFF 透传 + X-User-Id 转发

**关键路由**：
```
GET  /api/v1/reports/daily?date=2026-09-01       → analytics-svc
GET  /api/v1/reports/weekly                     → analytics-svc
GET  /api/v1/assessments                        → assessment-svc
POST /api/v1/assessments/:scale_id/submit      → assessment-svc
```

**TDD 节奏**：
```
PR-A2.1: AnalyticsClient + 路由（RED → GREEN）
PR-A2.2: AssessmentClient + 路由（RED → GREEN）
PR-A2.3: 集成 smoke：curl /api/v1/reports/daily → 验证 200
```

**验收**：前端"分析报表"和"心理量表"两个模块能正常打开数据。

---

### Stage 36-A 收口

`docs/stage-36-A-landing.md`：包含 PR-A1.1..A2.3 落地报告 + smoke 验证截图 + "Stage 36-A 全部 ✅"。

---

## 四、Stage 36-B（高优先，预计 2-3 天）

### PR-B1：chat-svc 加 list conversations 端点（G2）

**问题**：chat-svc main.go 注释确认"4 路由"只含 POST create + POST/GET messages；缺 `GET /api/v1/conversations` 列表。

**修复方案**：
- BFF 已有"chat list 调用"逻辑但 chat-svc 不支持 → 加端点
- `chat-svc/internal/handler/` 加 `ListConversationsHandler`
- 实现：按 user_id 查 conversations 表，分页（limit + offset），返回 ConversationList

**TDD 节奏**：
```
PR-B1.1: chat-svc ListConversationsHandler（RED → GREEN）
PR-B1.2: BFF list conversations 去掉空 fallback（GREEN）
PR-B1.3: smoke 验证：curl GET /api/v1/conversations → 200 + list
```

**验收**：前端"会话列表"页面显示真实数据（不再空）。

---

### PR-B2：消息自动情绪分析 dev fallback（G4）

**问题**：Kafka off 时消息不会自动写 emotion_analysis，前端情绪分析功能无法演示。

**修复方案**：双轨：
1. **保留 Kafka 主路径**（生产用）
2. **加 synchronous fallback**：chat-svc 发消息成功后，如果 Kafka 配置为"dev mode"（`KAFKA_ENABLED=false` 且 `DEV_SYNC_EMOTION=true`），chat-svc 直接调 ai-svc gRPC `GetEmotionByMessage` 或直接 INSERT 一条 emotion_analysis 占位

**简化方案**（更直接）：
- chat-svc 发消息成功后，**直接 INSERT 一条 `emotion_analysis` 占位行**（primary_emotion="neutral", confidence=0），让前端能立刻查
- 异步 Kafka consumer 路径保留（生产正常）

**TDD 节奏**：
```
PR-B2.1: chat-svc SendMessageHandler 加 dev fallback emotion write
PR-B2.2: smoke 验证：发消息 → DB emotion_analysis 自动有占位行
```

**验收**：发消息后立即能 `GetEmotionByMessage(msgID)` 返回 neutral 占位。

---

### Stage 36-B 收口

`docs/stage-36-B-landing.md`：G2 / G4 全 ✅ + 前端"会话列表 / 情绪分析"两个核心流程可演示。

---

## 五、Stage 36-C（中优先，预计 3-5 天）

### PR-C1：真实 LLM 集成（G5）

**问题**：当前 `LLM_BASE_URL=""` → emotion fusion 永远走 late_fuser_weighted。Stage 35 PR-1/PR-2 的容错 / 校验逻辑生产中未实测。

**前置条件**：
- DeepSeek API key（注册即送）
- 或 OpenAI API key（付费）

**修复方案**：
1. 配 `LLM_BASE_URL=https://api.deepseek.com`、`LLM_MODEL=deepseek-chat`、`LLM_API_KEY=...`
2. docker compose `.env.local` 加这三个变量
3. smoke 验证：发消息 → 看 FusionWorker tick 日志 "method=llm"（不再是 late_fuser_weighted）

**TDD 节奏**：
```
PR-C1.1: docker-compose.yml 加 LLM_* env 注入（doc only）
PR-C1.2: smoke 跑：real LLM 调通，记录日志 / metrics / 返回
PR-C1.3: 验证 markdown 容错 + schema 校验生效
```

**验收**：FusionWorker log 显示 `method=llm`，且 `emotion_echo_fusion_llm_call_total{outcome="success"}` 增加。

---

### PR-C2：FER / SenseVoice Python 模型镜像（G6）

**问题**：`profile: ai` 镜像未构建，image / audio 多模态不可用。

**前置条件**：
- `Emotion-Echo-LLM/FER/` + `Emotion-Echo-LLM/sensevoice-small/` Python 模型
- Dockerfile（应在 Emotion-Echo-LLM 下）

**修复方案**：
1. 构建 `emotion-echo/fer:v0.1.0` + `emotion-echo/sensevoice:v0.1.0` 镜像
2. 启动：`docker compose --profile ai up -d fer sensevoice`
3. ai-svc 配置 `FER_BASE_URL=http://emotion-echo-fer:8004` 等

**TDD 节奏**：
```
PR-C2.1: emotion-echo/fer Dockerfile（RED：build 失败）
PR-C2.2: emotion-echo/sensevoice Dockerfile（同上）
PR-C2.3: ai-svc smoke：上传图片 → emotion 非 neutral
```

**验收**：上传图片 → face_emotion_results 表写入非 neutral emotion。

---

### Stage 36-C 收口

`docs/stage-36-C-landing.md`：G5 / G6 全 ✅ + 真实融合 + 多模态可用。

---

## 六、Stage 36-D（低优先，预计 2-3 天）

### PR-D1：APISIX 全栈（G7）

**问题**：`apache/apisix-dashboard:3.18.0-alpine` 镜像不可拉。

**修复方案**：
1. 尝试 `apache/apisix-dashboard:3.20.0-alpine` 或 `release` tag
2. 若都不行：本地 `git clone` + docker build 自定义镜像
3. 启动全栈 docker compose，验证 APISIX :19080 + dashboard :9000 可用

**验收**：完整 docker compose up -d 后 APISIX dashboard 可登录。

---

### PR-D2：Nacos 配置中心（G8）

**问题**：smoke 主动跳过 Nacos，但生产应该用。

**修复方案**：
1. 启动 nacos 容器（`docker compose -f deploy/docker-compose.infra.yml up -d nacos`）
2. 5 个 Go svc 配 `NACOS_ENABLED=true` + `NACOS_ADDR=emotion-echo-nacos:8848`
3. 验证：每个 svc 在 Nacos 控制台看到注册 entry + 健康

**验收**：Nacos 控制台 5 个 svc 都已注册且健康。

---

### Stage 36-D 收口

`docs/stage-36-D-landing.md`：G7 / G8 全 ✅ + 全栈 docker compose 跑通。

---

## 七、Stage 36 整体验收

| 项 | 标准 |
|---|------|
| 8 个 ADR-16 缺口 | 全部 ✅ |
| 所有 PR | RED→GREEN→REFACTOR 三段 commit |
| smoke 验证 | 每个批次完成后立即跑 |
| landing doc | 每个批次单独收口 + Stage 36 总结 |
| architecture-decisions.md | 注册决策 16 + 任何中间变更 |
| branch ahead of main | 累计 commit + 8~15（视 PR 拆解粒度）|

---

## 八、不在 Stage 36 范围

- K8s manifests 完善（Stage 33 deferred）
- Helm charts 完善（同上）
- CI/CD pipeline（Stage 33 deferred）
- Kafka DLQ（Stage 33 deferred）
- DB migration tool（Stage 33 deferred）
- Nacos / etcd HA cluster
- Redis 共享限流（Stage 33 deferred）

> 这些继续作为 **deferred**，等 Stage 36 收口后单独评估。

---

## 九、参考资料

- ADR-16：[adr-2026-09-known-gaps.md](adr-2026-09-known-gaps.md)
- 来源：[stage-35-system-feasibility.md](stage-35-system-feasibility.md)
- Stage 35 smoke：[stage-35-smoke-validation.md](stage-35-smoke-validation.md)
- Stage 35 landing：[stage-35-landing.md](stage-35-landing.md)
- Stage 35 plan：[stage-35-production-hardening.md](stage-35-production-hardening.md)
- architecture-decisions：[architecture-decisions.md](architecture-decisions.md)

---

## 十、Post-36 增量（不属 ADR-16 8 项缺口，新发现的契约问题）

盘点 `feat/bff-fused-emotion-endpoint` 分支时发现图表三层契约全错位，
HTTP 200 但 dashboard 页面渲染塌掉。详见：[adr-2026-09-chart-contract-alignment.md](adr-2026-09-chart-contract-alignment.md)

- 分支：`fix/chart-contract-alignment`（基于 `fix/stage-36-post-test-cleanup` 0b58bd5）
- 4 commits: 90a3338 + 1590f24 + 789dd4b + 044cb58
- 解决：4 dashboard 页面真实渲染 / BFF presentation 层 / summary rule-based / wordCount 删除 / alias 解析
- 留给 stage-37：dev fallback user_behavior_events / 5 端点接入前端 / analytics_reader GRANT / LLM summary / 数据建模 bug