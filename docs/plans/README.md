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
| `observability-compose-gap.md` | 新增 | dev compose 可观测性三层补齐（metrics / logs / traces），与 k8s 路径对齐 |
| `db-migration-auto-apply.md` | 新增（2026-09-04） | 13 个服务 migrations 无自动应用机制，dev 库实测一个都没跑过。**本轮已落地**（提交 0d17e85），文件作为方案设计保留 |
| `todo-pile-2026-09-04.md` | 新增（2026-09-04） | 本轮修复过程中实测暴露的未关闭项汇总：TTS 不可用 / 上传未实现 / 多模态不可用 / 文档失真登记 / dev-prod 端口策略 |
| `kafka-reliability-gaps.md` | 新增（2026-09-07） | Kafka 管线 6 项健壮性缺口汇总：dev 模式空跑（ADR-19 未落地）/ ai-svc consumer 缺外层重试 / DLQ 默认 Noop / 无 lag 监控 / 无 Schema Registry / relay 无重试上限 |
| `grpc-inter-service-migration.md` | 新增（2026-09-07） | 后端微服务间调用 HTTP→gRPC 改造：已落地 3 条（chat→ai / ai→llm / BFF→ai），BFF→user/chat/assessment/analytics 仍 HTTP；分 Phase 推进，chat-svc 先行 |
| `observability-testing-gap.md` | 新增（2026-09-07） | 可观测性链路测试完善：metrics 契约 / trace 透传+e2e / 日志结构化 / 回归守护；与 observability-compose-gap.md（基础设施层）互补，本计划聚焦测试层 |

## 写入规范

- 新增计划时文件名用 kebab-case 英文
- 文件顶部加 front-matter：`status: planned` / `priority: high|medium|low`
- 引用 ADR 与相关 stage 时用相对路径
- 完成后迁移到 `legacy-plans/landed/`
