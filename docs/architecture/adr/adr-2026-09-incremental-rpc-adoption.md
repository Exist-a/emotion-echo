# ADR-18 · 微服务内部调用渐进式 RPC 化（2026-09-03）

> **状态**：已批准 · **类型**：架构演进 · **目标分支**：`main`（所有新 PR 适用）
> **来源**：盘点 feat/bff-fused-emotion-endpoint + ADR-17 时确认的服务间调用现状
> **前置**：ADR-17 chart-contract-alignment（前端 dashboard 渲染塌掉是契约断裂症状）

---

## 上下文（Context）

盘点当前 6 个 Go 微服务（user / chat / ai / analytics / assessment / web-bff）+ 1 个 Python 服务（emotion-llm-service）之间的内部调用协议：

| 调用方 | 被调方 | 协议 | 来源 |
|---|---|---|---|
| BFF → user-svc | HTTP/JSON | stage-30-web-bff 早期下沉 | |
| BFF → chat-svc | HTTP/JSON | 同上 | |
| BFF → analytics-svc | HTTP/JSON | 同上 | |
| BFF → assessment-svc | HTTP/JSON | 同上 | |
| BFF → ai-svc（emotion_query） | **gRPC** | stage-19 ai-svc-grpc-server（早期就走对了）| |
| BFF → emotion-llm-service（LLM 流 + FER/SenseVoice/XTTS）| HTTP + SSE | 跨语言 + LLM 厂商 OpenAI 兼容 | |
| chat-svc → ai-svc（dev fallback） | **gRPC** | stage-36-A3.2 新增 | |
| chat-svc → Kafka → ai-svc/analytics-svc | Kafka（异步管道）| stage-30-C | |

### 现状问题

1. **HTTP/JSON 是主调**（5/7 = 71% 调用是 HTTP）—— 教科书微服务内调用首选应该是 RPC
2. **契约漂移隐患** —— ADR-17 修复的 dashboard 渲染塌掉就是 HTTP/JSON 契约断裂的典型案例：BFF `OK(c, gin.H{"report": r})` 用单数 key 包一层，前端 `reportData.summary` 拿到 undefined，HTTP 200 + 单元测试全绿 = 没暴露
3. **proto 基础设施已就位** —— `proto/` 下两条 `.proto`（emotion_llm.proto / emotion_query.proto）；`emotion-echo-shared/pkg/grpcinterceptor` 已封装 mTLS / retry / tracing；`proto/gen.sh` 一键生成 Go + Python 代码；chat-svc `internal/grpcclient` 已示范了 AIClient 接口 + 真/空实现分离
4. **渐进成本可控** —— 既有 gRPC 经验（emotion_query / stage-36 chat→ai）证明团队已掌握，不需重新学

### 不动的边界

- **BFF → emotion-llm-service 的 LLM 流**保留 HTTP/SSE：LLM 行业标准是 OpenAI 兼容协议，**为了内部一致性硬改 gRPC 是反模式**
- **外部 LLM（DeepSeek/OpenAI）**调 HTTP —— 永远不动
- **前端 → BFF**：HTTP（浏览器跨语言、无 gRPC 浏览器原生支持）

---

## 决策（Decisions）

### §A. 新增服务 / 新增调用一律 gRPC

**适用范围**：自 ADR-18 生效起：
1. 所有新 Go 微服务之间调用 → gRPC（基于现有 proto + shared/pkg/grpcinterceptor）
2. 所有新 Go ↔ Python 内部调用 → gRPC（emotion-llm-service 已支持 gRPC server，见 emotion_llm.proto）
3. **例外清单**（必须遵守）：
   - 浏览器 → BFF（HTTP，无 gRPC 浏览器原生支持）
   - BFF → 外部 LLM / 外部 API（HTTP，OpenAI 兼容标准）
   - 跨 Nacos 服务发现注册的 health check（HTTP，Nacos 标准）
   - Webhook / 回调（HTTP，第三方无法升级）

### §B. 已有 HTTP 调用"找时间迁移"（非阻塞 backlog）

**理由**：现有 5 条 HTTP 链路都是 stage-30 早期产物，工作正常。**全量重写收益 < 风险**（影响 handler test + integration test + smoke 脚本 + docker compose）。**改不改、何时改由后续 stage 决定**，但 ADR-18 把"何时改"的原则定下来：

迁移触发条件（满足任一即应安排迁移）：
1. 该链路出现契约漂移（如 ADR-17 dashboard 那种）；
2. 该链路 P99 latency > 50ms（HTTP/JSON 解码开销占比 > 10%）；
3. 该链路需要 streaming 能力（HTTP 半双流够用即不动）；
4. 该链路上线新服务（或重写现有服务），优先用新协议。

### §C. proto-first 工作流（强制）

新 gRPC 服务必须按以下顺序：

1. **先写 .proto**（提交到 `proto/` 目录，PR review 包括类型设计）
2. **跑 `bash proto/gen.sh`**（生成 Go pb 到 `emotion-echo-shared/pkg/<pkg>/`，Python pb 到 `emotion-llm-service/`）
3. **服务端实现 + 客户端接口**（同时落地，不能先写客户端 stub）
4. **mTLS / tracing / retry** 默认接入（`shared/pkg/grpcinterceptor` 已封装）
5. **integration_test** 用 testcontainers 起真 server + 真 client 跑通

### §D. 客户端实现模式（参考 stage-36 chat-svc grpcclient）

```go
// 接口 + 真实现 + 空实现的"三件套"模式（graceful degradation）
type AIClient interface {
    UpsertNeutralEmotion(ctx, req) error
}

type aigrpcClient struct { conn *grpc.ClientConn }
func NewAIClient(addr string) (AIClient, error) {
    conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(...))
    return &aigrpcClient{conn: conn}, err
}

type NoopAIClient struct{}
func (NoopAIClient) UpsertNeutralEmotion(...) error { return nil }
```

理由：dial 失败 / 服务暂时不可达时调用方仍能正常工作（best-effort）。**不强制做 circuit breaker，但接口与实现必须分离**——便于测试 mock。

### §E. mTLS（stage-36-B3 已落地）+ tracing（stage-13）+ retry（stage-15）默认启用

`shared/pkg/grpcinterceptor` 已封装，新 gRPC 调用**零配置**即可获得：
- mTLS（dev 自签证书由 `scripts/generate_dev_tls.py` 生成）
- SkyWalking OAP 上报（生产）
- Exponential backoff retry（除 idempotent=false 外）

### §F. HTTP 不主动废弃

理由：HTTP 调用仍占大头（5/7），且 HTTP/JSON 对前端 dashboard / admin 调试友好（curl 直接看）。**保持现状共存**，新代码走 gRPC，老代码按 §B 触发条件渐进迁移。

---

## 实施细节（迁移候选优先级）

按"改动小 + 收益高"排序，给后续 stage 排参考：

| 优先级 | 候选迁移 | 预估工作量 | 收益理由 |
|---|---|---|---|
| P1 | chat-svc → analytics-svc（如果加新分析端点）| 1-2 天 | 跨服务 analytics-svc 已经接 gRPC（emotion_query.proto），加新 RPC 是 trivial 增量 |
| P1 | BFF → ai-svc emotion_query（已是 gRPC 但代码可精简）| 0.5 天 | 把 stage-19 的 ChatClient 模式应用到所有 BFF→ai 调用 |
| P2 | BFF → chat-svc（7 个端点）| 5-7 天 | 最高频调用，契约漂移风险最大（ADR-17 dashboard 是间接体现）|
| P2 | BFF → user-svc（auth + profile）| 3-4 天 | 鉴权敏感，gRPC + mTLS 减少 token 泄漏面 |
| P3 | BFF → analytics-svc（6 个端点）| 5-6 天 | 改动面最大，schedule query / aggregation 多 |
| P3 | BFF → assessment-svc（survey + result）| 3-4 天 | 调用频率低 |
| P4 | chat-svc Kafka consumer → ai-svc 调 gRPC（替代 stage-36 同步 fallback）| 1-2 天 | 异步路径统一 |

**未列入**：BFF → emotion-llm-service LLM 流（HTTP/SSE，按 §A 例外保留）。

### 配套改动（任一迁移 PR 必须包含）

1. proto 改动（如有）+ gen.sh 重生成（**禁止手写 .pb.go**）
2. shared/pkg/grpcinterceptor 复用（不重写 retry / mTLS）
3. BFF downstream 包保留 `xxxClient interface`（便于 mock 测试）
4. handler test 改 fake grpc client（`mock_xxx_test.go` 或 inline）
5. integration_test 用 testcontainers 起真 grpc server（参考 stage-31 Nacos testcontainers 模式）
6. docker compose 不变（gRPC 走 8892+ 端口，与 HTTP 端口共存）
7. smoke 脚本（`scripts/smoke_*.sh`）加 grpc CLI 调用验证

---

## 后果（Consequences）

### ✅ 正向

- 新增服务天然 contract-safe（proto 是单一真相源）
- 性能：gRPC HTTP/2 多路复用 + 二进制序列化，估算整体 latency 降 30-50%
- 调试体验：grpcurl / grpc-cli / 现有 skywalking 都对 gRPC 一等支持
- 团队已有 gRPC 经验（emotion_query / chat-svc dev fallback），迁移 learning curve 低

### ⚠️ 代价

- 全量迁移耗时长（按上表 P2+P3 约 3-4 周专门重构）
- HTTP/JSON 调试便利性丢失（不能直接 curl 看响应）—— 但 grpc-cli / grpcurl 补上
- 跨服务调用失败模式变（gRPC 错误码 vs HTTP status），handler 层 status 映射需重写

### ⚠️ 风险

- **proto 兼容性**：新增 field 容易，删除 / 重命名 field 是破坏性升级——必须用 `reserved` 字段保留旧号
- **mTLS 证书管理**：stage-36-B3 已有 dev 证书生成脚本；prod 证书轮转由 ops runbook 处理
- **grading 升级陷阱**：proto 文件必须用 reserved 字段保留旧号；否则客户端旧版本会 panic
- **跨语言一致性**：Python emotion-llm-service 用 grpcio 生成 stub；不能与 Go 端字段名错位

---

## 备选方案（已拒绝）

### 备选 1：全量一次性 RPC 化（拒绝）

立刻把 5 条 HTTP 链路全转 gRPC。
- 工作量：3-4 周专门重构 + 全量回归
- 风险：影响当前 stage-36 全部收口；smoke / integration test 全部需要重写
- 收益：仅 30-50% latency 优化
- **拒绝理由**：当前 stage-36 → stage-37 backlog 里多项缺口优先级更高（dev fallback / 5 端点接入 / LLM summary）；一次性改造 ROI 差

### 备选 2：彻底拥抱 REST + OpenAPI 严格契约（拒绝）

继续 HTTP/JSON，但引入 OpenAPI 3.0 strict + 自动化契约测试。
- 工作量：1 周搭脚手架 + 7 个服务补 spec
- 风险：OpenAPI 工具链（Redocly / Spectral）学习成本 + CI 卡 spec drift
- 收益：契约保护仅及于"字段对齐"，不及 gRPC 的 streaming / deadline / metadata
- **拒绝理由**：gRPC 已就位（proto + grpcinterceptor + 团队经验），选 OpenAPI 是倒退

### 备选 3：HTTP/JSON + Avro / MessagePack 二进制（拒绝）

维持 HTTP 但序列化改二进制。
- 工作量：每条链路各 1-2 天
- 风险：HTTP 调试工具链全部失效（curl / Postman 不能直接看 body）
- 收益：包体小，延迟低于 JSON
- **拒绝理由**：gRPC 已含全部收益 + 更好生态

---

## 待办（迁移路线图）

按上表优先级排进后续 stage（不是本 ADR 强制，但供排期参考）：

[ ] **stage-37** 起新 gRPC 服务先做（`P1` 候选 1）
[ ] **stage-38** BFF → chat-svc migration（`P2` 工作量最大，先做）
[ ] **stage-39** BFF → user-svc + analytics-svc migration（`P2/P3`）
[ ] **stage-40** 收尾：所有 HTTP 内部调用迁移完成；HTTP 仅保留 §A 例外清单

每个 stage 完成后更新本 ADR"已迁移清单"。

---

## 验收

| 维度 | 验收方式 |
|---|---|
| ADR 可发现 | `docs/adr-2026-09-*.md` 文件命名一致 |
| 与已有 ADR 一致 | ADR 编号连续（17 = chart-contract；18 = 本 ADR）|
| 新增服务遵守 | PR review 必查 `proto/` 改动；缺 proto → reject |
| 老迁移不阻塞 | 上述 stage-37/38/39/40 是建议，不强制时间 |
| 文档同步 | stage-37 路线图草案引用本 ADR §A |
