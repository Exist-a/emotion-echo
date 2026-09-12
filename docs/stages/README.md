---
purpose: 系统演进历史
status: Round 1 占位 · Round 3 全部迁入
---

# 系统演进历史（Stage 0 → 36+）

> 每个 Stage 是一次完整、独立、可回滚的演进单元。
> 每份 stage 文档包含目标、commit 清单、验证报告、ADR 引用。

## 阅读建议

- **新人**：先看 Stage 35（生产加固）→ Stage 30（BFF 上线）→ Stage 0~5（基础）
- **找某次变更**：直接按 stage 编号定位
- **想了解某个决策**：`architecture/adr/` + 对应 stage 文档

## 索引

Round 3 完成后此索引将自动生成（按 stage 编号）。

| 阶段 | 主题 | 文档数 |
|------|------|-------|
| Stage 0~9 | 单体 → 微服务演进 | 10 |
| Stage 10~19 | gRPC 化 | 10 |
| Stage 20~25 | 容器化 + AI 接入 | ~15 |
| Stage 26~30 | BFF + Kafka + LLM | ~25 |
| Stage 31~36 | Nacos + APISIX + 多模态 + 生产加固 | ~30 |
| Stage 37~39 | data layer 4A bug 修复 + dev APISIX + Nacos 真启用 | 8 |
| **Stage 40** | **todo-pile 自查 + 全面文档失真修复（决策 18 治理）** | **1** |
| Stage 41~50 | go-zero 全量移除 + 容器 TZ + Kafka Sprint A + 观测链路 Sprint B 收口（44~49）+ e2e 验证 | 11 |
| Stage 51~60 | Nacos 修复 / roadmap Z / Kafka·mentalhealth 批次合并 / Q3 后续 24 commit / TTS vendor 三 AI 模型 | 12 |
| Stage 61~70 | gRPC 化决策 4 收口 + BFF gRPC wiring + bug 集中清理 + DevEventPublisher/seed + Kafka P1 + 路由对齐 | 11 |
| Stage 71~80 | BFF 路由对齐 + chat Pin RPC + Kafka Protobuf 迁移 + 技术债收尾 + Nacos PR-1~3 + Postgres 守卫 + file-upload + llm pipeline PR-1 | 10 |
| Stage 81~85 | llm-chat-real-pipeline PR-2/3a/3b（BFF gRPC 上游 + 意图分类/落库/日报饼图）+ events 测试契约修复 + 趋势报告意图维度 | 5 |

> 注：41 以后按主题分段统计（2026-09-12 更新至 Stage 85）；每期 stage 的"本批未做（open）"
> 表 + [architecture/roadmap.md](../architecture/roadmap.md) §下一步行动 是待办的权威来源。
