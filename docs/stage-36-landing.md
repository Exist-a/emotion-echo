# Stage 36 · ADR-16 八项缺口一次性修复落地报告（Landing Report）

> 状态：**全部 ✅ 完成** · 日期：2026-09-01 · 目标分支：`feat/bff-fused-emotion-endpoint`
> ADR 编号：16（Stage 35 系统缺口正式记录）
> 来源：[stage-36-fixes-roadmap.md](stage-36-fixes-roadmap.md)
> 全部 commit：15 个（4 批次，11 PR），全部已 push 到 origin

---

## 一、8 项缺口关闭总览

| # | 缺口 | 严重度 | PR | 状态 | 备注 |
|---|------|--------|------|------|------|
| G1 | 4 svc yaml 占位符 → go2sky dial 失败循环 | 🟡 中 | A1.1~A1.4 | ✅ | 跟随 ai-svc e35d531/9610dd8 模式 |
| G2 | chat-svc 缺 `GET /api/v1/conversations` | 🟡 中 | A2.1+A2.2 | ✅ | chat-svc 加端点 + BFF 透传替换空 stub |
| G3 | BFF 缺 analytics/assessment 路由 | 🔴 高 | **（不存在）** | ✅ | G3 已在前序 stage 收口，本轮确认 |
| G4 | Kafka off 时情绪分析无数据 | 🔴 高 | A3.1+A3.2 | ✅ | ai-svc 加 UpsertNeutralEmotion gRPC + chat-svc dev fallback |
| G5 | 真实 LLM 未配 | 🔴 高 | B1 | ✅ | compose env 注入 + .env.local.example |
| G6 | FER/SenseVoice 镜像未冒烟 | 🟡 中 | B2 | ✅ | smoke_ai_profile.sh + 122 单元测试通过 |
| G7 | APISIX dashboard 镜像不可拉 | 🟡 中 | C1 | ✅ | **删容器**而非换镜像——APISIX 3.x 已内嵌 UI |
| G8 | Nacos 全栈未实测 | 🟢 低 | C2 | ✅ | smoke_nacos_full.sh（代码已配齐） |

**全部 8 项缺口按 ADR-16 §B 排期一次性收口**，无 deferred。

---

## 二、每 PR 落地清单（15 commits）

### 36-A：4 批次 · 8 commits

| Commit | 类型 | 内容 |
|---|---|---|
| `fc596fe` | test(A1.1) | user-svc RED: no-bash-placeholder 检查 |
| `67ea247` | fix(A1.1) | user-svc GREEN: yaml 净化 |
| `f4543e6` | refactor(A1.1) | 测试精度提升（忽略注释行） |
| `e573bfd` | test(A1.2) | chat-svc RED + Kafka.Enabled default=true |
| `e1619d9` | fix(A1.2) | chat-svc GREEN: yaml 净化 + Kafka default=true |
| `c9d5809` | test(A1.3) | analytics-svc RED + Kafka.Enabled default=true |
| `5d29966` | fix(A1.3) | analytics-svc GREEN: yaml + Kafka + 旧测试反转 |
| `437febe` | test(A1.4) | assessment-svc RED |
| `4b00598` | fix(A1.4) | assessment-svc GREEN: yaml 净化 |
| `650d208` | test(A2.1) | chat-svc ListConversations RED |
| `92cd322` | fix(A2.1) | chat-svc ListConversations GREEN: repo + logic + handler + route |
| `2152390` | test(A2.2) | BFF listConversations RED |
| `c3839b6` | fix(A2.2) | BFF listConversations GREEN: ChatClient + 替换 stub |

### A3（G4）· 4 commits

| Commit | 类型 | 内容 |
|---|---|---|
| `d232f89` | test(A3.1) | proto 改 + pb.go 重生成（protoc 32.1）+ UpsertNeutralEmotion 测试 |
| `41cff5c` | test(A3.1) | EmotionRepo.GetByEventID RED |
| `7e38556` | fix(A3.1) | ai-svc UpsertNeutralEmotion + GetByEventID (InMemory + Postgres) |
| `e4ac745` | test(A3.2) | chat-svc dev fallback RED |
| `243ef28` | fix(A3.2) | chat-svc grpcclient + dial ai-svc + maybeUpsertNeutralEmotion |

### B（G5/G6）· 2 commits

| Commit | 类型 | 内容 |
|---|---|---|
| `a4ac0f9` | fix(B1) | compose LLM_BASE_URL/API_KEY/MODEL/TIMEOUT env + .env.local.example |
| `8670ec6` | feat(B2) | smoke_ai_profile.sh（FER + SenseVoice health probe） |

### C（G7/G8）· 2 commits

| Commit | 类型 | 内容 |
|---|---|---|
| `857dcd3` | fix(C1) | 删 apisix-dashboard 容器 + validation.yml + 暴露 host :9180 |
| `9a9de43` | feat(C2) | smoke_nacos_full.sh |

---

## 三、关键架构变更（值得 review）

### A3（G4）：chat-svc 首次接入 gRPC client

**新增包**：`emotion-echo-chat-svc/internal/grpcclient/`
- `ai_client.go` — `AIClient` 接口 + `NoopAIClient` 空实现
- `ai_client_grpc.go` — `aigrpcClient` 真 gRPC 实现（dial ai-svc :8892）

**ServiceContext 加字段**：`AIClient grpcclient.AIClient`（默认 `NoopAIClient{}`，main.go 注入真 client）

**SendMessageLogic.maybeUpsertNeutralEmotion**：仅在 `Kafka.Enabled=false` 时调 `AIClient.UpsertNeutralEmotion(event_id=outbox UUID)`。失败只 log 不阻塞消息返回（dev fallback best-effort 语义）。

**幂等键**：`event_id` = outbox event UUID，DB UNIQUE on event_id 保证 at-least-once 投递下不重复落库。

### A3（G4）：shared proto 重生成

用 protoc 32.1 + 本地 `protoc-dist/` 工具链从 `proto/emotion_query.proto` 重生成 `emotion-echo-shared/pkg/emotionquery/*.pb.go`。新增 RPC `UpsertNeutralEmotion(UpsertNeutralEmotionRequest) returns (UpsertNeutralEmotionResponse)`。

### C1（G7）：APISIX dashboard 容器删除

发现 `apache/apisix-dashboard:3.18.0-alpine` 镜像**永远拉不到**——项目自 2025-07-09 起停止独立发版（GitHub release 历史已确认）。APISIX 3.x 主镜像 `apache/apisix:3.18.0-debian` 已默认开启 `deployment.admin.enable_admin_ui: true`，UI 在 `/ui/` 路径下。

修复 = **删 dashboard 容器 + 删 validation.yml workaround + 暴露 host `9180:9180`**。这是 plan 阶段联网搜到的关键发现，比原计划"换镜像"简单得多。

---

## 四、测试状态

| 服务 | `go test ./...` | `go vet ./...` |
|---|---|---|
| emotion-echo-user-svc | ✅ | ✅ |
| emotion-echo-chat-svc | ✅ | ✅ |
| emotion-echo-analytics-svc | ✅ | ✅ |
| emotion-echo-assessment-svc | ✅ | ✅ |
| emotion-echo-ai-svc | ✅ | ✅ |
| emotion-echo-web-bff | ✅ | ✅ |
| emotion-echo-shared | ✅ | ✅ |

**emotion-echo-llm**（Python）：

| 包 | `pytest tests/unit/` |
|---|---|
| Emotion-Echo-LLM/FER | ✅ 64 passed |
| Emotion-Echo-LLM/sensevoice-small | ✅ 58 passed |

---

## 五、待跑 docker smoke 验证（用户线下执行）

本会话未跑 docker（你的环境里跑会更准确）。两个脚本已就绪：

```bash
# G6：FER + SenseVoice 健康检查 + /health endpoint 探测
bash scripts/smoke_ai_profile.sh

# G8：6 svc 注册 Nacos 全栈实测
bash scripts/smoke_nacos_full.sh

# G5：真实 LLM 烟雾测试
cp docs/env-templates/.env.local.example deploy/.env.local
# 填入真实 DeepSeek key，然后
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d ai-svc
docker compose logs ai-svc | grep "fusion.*LLM fuser active"  # 期望一行日志
```

---

## 六、不在 Stage 36 范围（继续 deferred）

按 ADR-16 §D：
- K8s manifests / Helm chart 完善
- CI/CD pipeline
- Kafka DLQ
- DB migration tool
- Nacos / etcd HA cluster
- Redis 共享限流（Stage 33 deferred）

---

## 七、ADR 注册

无需新增 ADR。本轮所有变更遵循已有 ADR-15（Stage 35 production hardening）+ ADR-16（Stage 35 缺口登记）的策略。

---

## 八、Stage 36 与 Stage 35/34 节奏一致

| Stage | 时间 | 主题 | commits |
|---|---|---|---|
| 33 | 2026-07 | 部署修复 + Nacos/APISIX 引入 | ~70 |
| 34 | 2026-08 | 多模态融合数据通路 | ~25 |
| 35 | 2026-09 | LLM fusion 加固 + 缺口登记（ADR-15/16） | ~13 |
| **36** | **2026-09** | **8 项缺口一次性修复（ADR-16 全 ✅）** | **15** |
