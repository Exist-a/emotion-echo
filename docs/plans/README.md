---
purpose: 当前有效、未来排期的功能计划
status: Round 1 占位 · Round 2 首批内容迁入
---

# 当前有效的计划

> 这些计划**未被任何 stage 取代**，等待排期与实施。
> 与 `legacy-plans/`（已落地/已偏移/历史价值）不同，这里的文档描述"接下来要做的事"。

## 当前条目（Round 2 迁入）

| 文件 | 来源 | 主题 |
|------|------|------|
| `ai-response-structured.md` | `.trae/documents/ai-response-structured.md` | AI 回复结构化 + Markdown 渲染 |
| `three-vrm-usage-reference.md` | `.trae/documents/three-vrm-usage-reference.md` | Three-VRM API 参考手册 |
| `wechat-qq-login-and-upload.md` | `.trae/documents/微信QQ登录和文件上传实施计划.md` | QQ OAuth + 通用文件上传 |
| `nacos-enablement-dev.md` | 新增 | Nacos dev 模式从"半启用"到"全链路" |
| `observability-compose-gap.md` | 新增（2026-09-04） → 已被取代 | dev compose 可观测性三层补齐。**PR 拆分已被 `observability-sprint-b.md` PR-OBS-1~8 取代**，原文件 §一/§二现状作为参考保留 |
| `db-migration-auto-apply.md` | 新增（2026-09-04） | 13 个服务 migrations 无自动应用机制，dev 库实测一个都没跑过。**本轮已落地**（提交 0d17e85），文件作为方案设计保留 |
| `todo-pile-2026-09-04.md` | 新增（2026-09-04） | 本轮修复过程中实测暴露的未关闭项汇总：TTS 不可用 / 上传未实现 / 多模态不可用 / 文档失真登记 / dev-prod 端口策略 |
| `kafka-reliability-gaps.md` | 新增（2026-09-07） → Sprint A 已落地 | Kafka 管线 6 项健壮性缺口汇总。**Sprint A 已 100% 收口**（10 commit on `feat/chat-dev-event-publisher-a1-1-red`,7/7 服务绿），见 [stages/stage-43-kafka-reliability-sprint-a.md](../stages/stage-43-kafka-reliability-sprint-a.md）。Sprint B §1.4 lag 监控已并入 `observability-sprint-b.md` PR-OBS-7；§1.5 Protobuf 迁移独立 Sprint C 排期 |
| `grpc-inter-service-migration.md` | 新增（2026-09-07）→ 2026-09-11 Sprint C/D/E 全落地 | 后端微服务间调用 HTTP→gRPC 改造：3 条已落地（chat→ai / ai→llm / BFF→ai）+ Sprint C（ctxkey 重构）+ Sprint D（chat-svc 4 RPC 实现）+ Sprint E（user-svc Login/Register proto 扩 + 实现）。**决策 4 核心业务路径 100% gRPC 覆盖（BFF→4 svc handler 调用 21/21 全 gRPC）**。剩余 Sprint F（user-svc ResetPassword/Logout gRPC 化 + ai-svc 业务方法 + chat-svc PinConversation/StreamMessages）+ #32 chat-svc HTTP 500 bug 根因排查 |
| `observability-testing-gap.md` | 新增（2026-09-07） → 已被取代 | 可观测性链路测试完善。**PR 拆分已被 `observability-sprint-b.md` PR-OBS-9~16 取代** |
| `observability-sprint-b.md` | 新增（2026-09-08） → 已落地 | 观测链路 Sprint B 执行源。**16 个 PR-OBS-X + 1 fix = 34 commit 全落地**（2026-09-08），见 [stages/stage-44-observability-sprint-b.md](../stages/stage-44-observability-sprint-b.md)。**PR-OBS-17/18/23/15/19（2026-09-08）§四 B/C 收口**：Stage 45 接口抽象 + Stage 46 HTTP EntrySpan + Stage 47 logging 接入 + Stage 48 err 透传 + Stage 49 gRPC rpc.* tag。**业务功能 6/6 步 100% 收口**。**Stage 50 端到端验证归档**：单元测试 100% PASS + smoke 10/12，具体见 [stages/stage-50-e2e-validation.md](../stages/stage-50-e2e-validation.md)。剩余非业务（sw-oap telemetry / 分支 merge / Nacos / 运维 SQL）见 stage-44 §四 |
| ~~`gozero-removal.md`~~ | 已 landed 2026-09-07 | go-zero 完全移除收尾（决策 1）：conf→shared/pkg/config + logx→slog 下沉 shared + rest.Middleware 类型清零 + goctl 归档；10 个 TDD PR 全部 merged。详见 `legacy-plans/landed/gozero-removal.md` 与 `stages/stage-41-gozero-removal.md` |

## 写入规范

- 新增计划时文件名用 kebab-case 英文
- 文件顶部加 front-matter：`status: planned` / `priority: high|medium|low`
- 引用 ADR 与相关 stage 时用相对路径
- 完成后迁移到 `legacy-plans/landed/`
